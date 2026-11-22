package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/utils/ws"
	"github.com/ayn2op/arikawa/v3/voice/voicegateway"
	"github.com/disgoorg/godave"
)

const (
	// joinTimeout is how long Discord has to answer a request to watch with the stream's server.
	joinTimeout = 20 * time.Second
	// jitterDepth is how many packets after a missing one are awaited before it is given up on: enough for the
	// packets of a keyframe to come in a different order.
	jitterDepth = 128
	// pliInterval is the least time between two requests for a keyframe.
	pliInterval = 500 * time.Millisecond
)

// Status is what is being watched, if anything, and how it is going.
type Status struct {
	Key Key
	// Watching reports whether a stream is being connected to or watched.
	Watching bool
	// Playing reports whether video is being played.
	Playing bool
	// Codec is the codec of the video being played, once it is.
	Codec string
	// Err is why watching the last stream ended, if it ended on its own.
	Err error
}

// AudioSink plays the audio of the stream watched, along with that of the voice channel it is in.
type AudioSink interface {
	// PlayStream plays an Opus packet with the sequence number seq, which userID sent with ssrc.
	PlayStream(ssrc uint32, userID discord.UserID, seq uint16, packet []byte) error
	// StopStream stops playing what was played.
	StopStream()
}

// Watcher watches one stream at a time.
type Watcher struct {
	state  *state.State
	dave   godave.SessionCreateFunc
	player PlayerConfig
	// audio plays the stream's audio, or is nil to play none.
	audio AudioSink

	mutex  sync.Mutex
	status Status
	cancel context.CancelFunc

	updates chan struct{}
}

// NewWatcher returns a watcher that encrypts end to end with sessions from dave, plays video with player and audio
// with audio, if it is not nil.
func NewWatcher(state *state.State, dave godave.SessionCreateFunc, player PlayerConfig, audio AudioSink) *Watcher {
	return &Watcher{state: state, dave: dave, player: player, audio: audio, updates: make(chan struct{}, 1)}
}

// Updates sends whenever the status changes.
func (w *Watcher) Updates() <-chan struct{} { return w.updates }

func (w *Watcher) Status() Status {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.status
}

func (w *Watcher) setStatus(key Key, change func(*Status)) {
	w.mutex.Lock()
	if w.status.Key != key {
		w.mutex.Unlock()
		return
	}
	change(&w.status)
	w.mutex.Unlock()

	select {
	case w.updates <- struct{}{}:
	default:
	}
}

// Watch stops watching the stream being watched, if any, and starts watching the one with key, which the user must
// be in the voice channel of.
func (w *Watcher) Watch(key Key) {
	w.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	w.mutex.Lock()
	w.cancel = cancel
	w.status = Status{Key: key, Watching: true}
	w.mutex.Unlock()
	w.setStatus(key, func(*Status) {})

	go func() {
		err := w.watch(ctx, key)
		if ctx.Err() != nil {
			err = nil
		}
		if err != nil {
			slog.Error("watching a stream ended", "stream_key", key, "err", err)
		}
		w.setStatus(key, func(s *Status) { *s = Status{Key: key, Err: err} })
	}()
}

// Stop stops watching.
func (w *Watcher) Stop() {
	w.mutex.Lock()
	cancel := w.cancel
	w.cancel = nil
	w.mutex.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ErrStreamEnded is returned once the stream that was watched ends.
var ErrStreamEnded = errors.New("the stream ended")

// watch asks Discord for the stream with key, connects to its server and plays it until ctx ends or it does.
func (w *Watcher) watch(ctx context.Context, key Key) error {
	guildID, channelID, ownerID, err := key.Parse()
	if err != nil {
		return err
	}

	created := make(chan *CreateEvent, 1)
	servers := make(chan *ServerUpdateEvent, 4)
	deleted := make(chan *DeleteEvent, 1)
	removeHandler := w.state.AddHandler(func(event any) {
		switch event := event.(type) {
		case *CreateEvent:
			if event.StreamKey == key {
				trySend(created, event)
			}
		case *ServerUpdateEvent:
			if event.StreamKey == key {
				trySend(servers, event)
			}
		case *DeleteEvent:
			if event.StreamKey == key {
				trySend(deleted, event)
			}
		}
	})
	defer removeHandler()

	if err := w.state.Gateway().Send(ctx, &WatchCommand{StreamKey: key}); err != nil {
		return fmt.Errorf("failed to ask to watch the stream: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.state.Gateway().Send(stopCtx, &StopCommand{StreamKey: key}); err != nil {
			slog.Warn("failed to stop watching a stream", "stream_key", key, "err", err)
		}
	}()

	var create *CreateEvent
	var server *ServerUpdateEvent
	timeout := time.NewTimer(joinTimeout)
	defer timeout.Stop()
	for create == nil || server == nil {
		select {
		case create = <-created:
		case server = <-servers:
		case event := <-deleted:
			return deleteError(event)
		case <-timeout.C:
			return errors.New("Discord did not answer the request to watch the stream")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if server.Endpoint == "" {
		return errors.New("the stream has no server yet")
	}

	me, err := w.state.Me()
	if err != nil {
		return err
	}
	identify := identifyCommand{
		ServerID:  create.RTCServerID,
		ChannelID: channelID,
		UserID:    me.ID,
		SessionID: w.sessionID(guildID, me.ID),
		Token:     server.Token,
		Video:     true,
		Streams:   []streamInfo{{Type: "screen", RID: "100", Quality: 100}},
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	gateway := newVoiceGateway(server.Endpoint, identify, w.dave)
	r := &receiver{watcher: w, key: key, gateway: gateway, owner: ownerID, needKeyframe: true, ended: make(chan struct{}), received: make(chan struct{})}
	defer r.stop()

	ops := gateway.connect(ctx)
	var ready *readyEvent
	for {
		select {
		case op, ok := <-ops:
			if !ok {
				if err := gateway.gateway.LastError(); err != nil {
					return err
				}
				return ErrStreamEnded
			}
			if err := r.handle(ctx, op, &ready); err != nil {
				return err
			}
		case event := <-deleted:
			return deleteError(event)
		case <-r.ended:
			return r.err
		case <-ctx.Done():
			return nil
		}
	}
}

// sessionID returns the session of the user's voice state, which a stream's connection identifies with.
func (w *Watcher) sessionID(guildID discord.GuildID, userID discord.UserID) string {
	if guildID.IsValid() {
		if voiceState, err := w.state.VoiceState(guildID, userID); err == nil && voiceState.SessionID != "" {
			return voiceState.SessionID
		}
	}
	return w.state.Ready().SessionID
}

func deleteError(event *DeleteEvent) error {
	switch event.Reason {
	case "", "user_requested", "stream_ended":
		return ErrStreamEnded
	case "stream_full":
		return errors.New("the stream is full")
	case "unauthorized":
		return errors.New("you are not allowed to watch the stream")
	default:
		return fmt.Errorf("the stream ended: %s", event.Reason)
	}
}

func trySend[T any](ch chan T, v T) {
	select {
	case ch <- v:
	default:
	}
}

// receiver connects the media of a stream and plays its video.
type receiver struct {
	watcher *Watcher
	key     Key
	gateway *voiceGateway
	owner   discord.UserID

	transport *transport
	// videoSSRC and rtxSSRC are what the owner of the stream sends video and repairs with, and audioSSRC audio.
	videoSSRC, rtxSSRC, audioSSRC atomic.Uint32

	// The fields below are used by the goroutine that receives.
	jitter       *jitterBuffer
	codec        string
	player       *player
	needKeyframe bool
	lastPLI      time.Time

	ended chan struct{}
	err   error
	once  sync.Once
	// receiving reports whether receive was started, which closes received once it returns.
	receiving bool
	received  chan struct{}
}

// handle acts on an op of the stream's voice gateway. ready is the Ready event once it came.
func (r *receiver) handle(ctx context.Context, op ws.Op, ready **readyEvent) error {
	switch data := op.Data.(type) {
	case *ws.CloseEvent:
		return fmt.Errorf("the stream's voice server closed the connection: %w", data)
	case *readyEvent:
		*ready = data
		addr := fmt.Sprintf("%s:%d", data.IP, data.Port)
		dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		transport, err := dialTransport(dialCtx, addr, data.SSRC)
		cancel()
		if err != nil {
			return err
		}
		r.transport = transport

		mode, err := chooseMode(data.Modes)
		if err != nil {
			return err
		}
		return r.gateway.send(ctx, &selectProtocolCommand{
			Protocol: "udp",
			Data:     voicegateway.SelectProtocolData{Address: transport.address, Port: transport.port, Mode: mode},
			Codecs:   offeredCodecs,
		})
	case *sessionDescriptionEvent:
		if r.transport == nil || *ready == nil {
			return errors.New("the stream's session was described before it was ready")
		}
		if err := r.transport.useKey(data.Mode, data.SecretKey); err != nil {
			return err
		}

		// A viewer announces its own SSRCs, sending no video, before it is sent any.
		streams := make([]streamInfo, len((*ready).Streams))
		for i, s := range (*ready).Streams {
			s.Active = false
			streams[i] = s
		}
		if err := r.gateway.send(ctx, &videoEvent{AudioSSRC: (*ready).SSRC, Streams: streams}); err != nil {
			return err
		}
		if err := r.gateway.send(ctx, &mediaSinkWantsCommand{"any": 100}); err != nil {
			return err
		}
		if !r.receiving {
			r.receiving = true
			go r.receive()
		}
	case *videoEvent:
		if data.UserID != r.owner {
			return nil
		}
		if data.AudioSSRC != 0 {
			r.audioSSRC.Store(data.AudioSSRC)
		}
		ssrc, rtx := data.primary()
		if ssrc == 0 {
			return nil
		}
		if r.videoSSRC.Swap(ssrc) != ssrc {
			r.rtxSSRC.Store(rtx)
			return r.gateway.send(ctx, &mediaSinkWantsCommand{"any": 100, fmt.Sprint(ssrc): 100})
		}
		r.rtxSSRC.Store(rtx)
	}
	return nil
}

// receive reads the stream's media and plays its video until the transport closes or playing fails.
func (r *receiver) receive() {
	defer close(r.received)
	buf := make([]byte, maxPacketSize)
	for {
		packet, err := r.transport.readRTP(buf)
		if err != nil {
			r.end(nil)
			return
		}
		if err := r.handlePacket(packet); err != nil {
			r.end(err)
			return
		}
	}
}

// primaryPayload returns the payload type of the media that RTX with payloadType repairs.
func primaryPayload(payloadType uint8) uint8 {
	switch payloadType {
	case payloadH264RTX:
		return payloadH264
	case payloadVP8RTX:
		return payloadVP8
	}
	return payloadType
}

func codecOf(payloadType uint8) (string, depacketizer) {
	switch payloadType {
	case payloadH264:
		return codecH264, h264Depacketizer{}
	case payloadVP8:
		return codecVP8, vp8Depacketizer{}
	}
	return "", nil
}

// handlePacket plays the audio of the stream's owner, and puts a video packet of theirs in order and plays the
// frames it completes.
func (r *receiver) handlePacket(packet rtpPacket) error {
	if audio := r.audioSSRC.Load(); audio != 0 && packet.ssrc == audio {
		r.playAudio(packet)
		return nil
	}

	video, rtx := r.videoSSRC.Load(), r.rtxSSRC.Load()
	switch {
	case video == 0:
		return nil
	case packet.ssrc == rtx:
		repaired, err := unwrapRTX(packet, video, primaryPayload(packet.payloadType))
		if err != nil {
			return nil
		}
		packet = repaired
	case packet.ssrc != video:
		// Video of others is not played.
		return nil
	}

	codec, depacketizer := codecOf(packet.payloadType)
	if depacketizer == nil {
		return nil
	}
	if codec != r.codec {
		r.codec, r.jitter, r.needKeyframe = codec, newJitterBuffer(jitterDepth), true
		if r.player != nil {
			r.player.stop()
			r.player = nil
		}
	}

	for _, f := range r.jitter.push(packet) {
		if err := r.playFrame(f, depacketizer); err != nil {
			return err
		}
	}
	return nil
}

// playFrame plays a frame of video, from a keyframe on: after a loss, until the next keyframe, frames are dropped
// rather than shown broken, and a keyframe is asked for.
func (r *receiver) playFrame(f frame, depacketizer depacketizer) error {
	if f.afterLoss {
		r.needKeyframe = true
	}

	data, keyframe, err := depacketizer.frame(f.payloads)
	if err != nil {
		r.needKeyframe = true
		r.requestKeyframe()
		return nil
	}
	if r.needKeyframe && !keyframe {
		r.requestKeyframe()
		return nil
	}

	plain, err := r.gateway.decrypt(r.owner, data)
	if err != nil {
		// The end-to-end keys may not be agreed on yet.
		slog.Debug("failed to decrypt a stream's frame", "err", err)
		r.needKeyframe = true
		return nil
	}
	r.needKeyframe = false

	if r.player == nil {
		player, err := startPlayer(r.watcher.player, r.codec, "Discord stream")
		if err != nil {
			return err
		}
		r.player = player
		codec := r.codec
		r.watcher.setStatus(r.key, func(s *Status) { s.Playing, s.Codec = true, codec })
		go func() {
			<-player.done
			// The player was closed, so the stream is no longer watched.
			r.end(nil)
		}()
	}
	// A player that stopped reading was closed, which ends watching through its done channel.
	_ = r.player.write(plain, f.timestamp)
	return nil
}

// playAudio plays an Opus packet of the stream, once its end-to-end encryption is removed.
func (r *receiver) playAudio(packet rtpPacket) {
	sink := r.watcher.audio
	if sink == nil || packet.payloadType != payloadOpus {
		return
	}
	opus, err := r.gateway.decrypt(r.owner, packet.payload)
	if err != nil {
		slog.Debug("failed to decrypt a stream's audio", "err", err)
		return
	}
	if err := sink.PlayStream(packet.ssrc, r.owner, packet.sequence, opus); err != nil {
		slog.Debug("failed to play a stream's audio", "err", err)
	}
}

// requestKeyframe asks the stream's owner for a keyframe, at most every pliInterval.
func (r *receiver) requestKeyframe() {
	video := r.videoSSRC.Load()
	if video == 0 || r.transport == nil || time.Since(r.lastPLI) < pliInterval {
		return
	}
	r.lastPLI = time.Now()
	if err := r.transport.sendPLI(video); err != nil {
		slog.Debug("failed to ask for a keyframe", "err", err)
	}
}

// end stops receiving, with err as why if it is not nil.
func (r *receiver) end(err error) {
	r.once.Do(func() {
		r.err = err
		close(r.ended)
	})
}

func (r *receiver) stop() {
	if sink := r.watcher.audio; sink != nil {
		defer sink.StopStream()
	}
	if r.transport != nil {
		r.transport.close()
	}
	// The player is the receiving goroutine's until it returns.
	if r.receiving {
		<-r.received
	}
	if r.player != nil {
		r.player.stop()
	}
}
