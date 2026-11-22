package stream

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/utils/ws"
	"github.com/ayn2op/arikawa/v3/voice/voicegateway"
	"github.com/disgoorg/godave"
)

// voiceGatewayVersion is the version of the voice gateway connected to, which has video and DAVE.
const voiceGatewayVersion = "9"

// gatewayOpts reconnect a stream's voice gateway only on errors that do not end the stream.
var gatewayOpts = ws.GatewayOpts{
	ReconnectDelay:        func(try int) time.Duration { return time.Duration(2+2*try) * time.Second },
	FatalCloseCodes:       []int{4003, 4004, 4006, 4009, 4011, 4012, 4014, 4016, 4017, 4021, 4022},
	ReconnectAttempt:      3,
	AlwaysCloseGracefully: true,
}

// voiceGateway is the websocket of a stream's voice server. It identifies, keeps the DAVE session in step and sends
// on what the watcher needs.
type voiceGateway struct {
	gateway  *ws.Gateway
	identify identifyCommand
	dave     godave.Session

	mutex sync.Mutex
	ready bool
	// seq is the sequence number of the last event received.
	seq int64
}

func newVoiceGateway(endpoint string, identify identifyCommand, dave godave.SessionCreateFunc) *voiceGateway {
	if dave == nil {
		dave = godave.NewNoopSession
	}

	url := "wss://" + strings.TrimSuffix(endpoint, ":80") + "/?v=" + voiceGatewayVersion
	g := &voiceGateway{
		gateway:  ws.NewGateway(ws.NewWebsocket(ws.NewCodec(opUnmarshalers), url), &gatewayOpts),
		identify: identify,
	}
	g.dave = dave(slog.Default(), godave.UserID(identify.UserID.String()), (*daveCallbacks)(g))
	g.dave.SetChannelID(godave.ChannelID(identify.ChannelID))
	identify.MaxDAVEProtocolVersion = g.dave.MaxSupportedProtocolVersion()
	g.identify = identify
	return g
}

// connect opens the websocket and returns the events it receives that the watcher acts on, until it closes.
func (g *voiceGateway) connect(ctx context.Context) <-chan ws.Op {
	return g.gateway.Connect(ctx, (*voiceGatewayHandler)(g))
}

func (g *voiceGateway) send(ctx context.Context, event ws.Event) error {
	return g.gateway.Send(ctx, event)
}

// decrypt removes the end-to-end encryption of a frame that userID sent.
func (g *voiceGateway) decrypt(userID discord.UserID, frame []byte) ([]byte, error) {
	out := make([]byte, g.dave.MaxDecryptedFrameSize(godave.UserID(userID.String()), len(frame)))
	n, err := g.dave.Decrypt(godave.UserID(userID.String()), frame, out)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

type daveCallbacks voiceGateway

func (g *daveCallbacks) SendMLSKeyPackage(b []byte) error {
	return g.gateway.Send(context.Background(), new(voicegateway.MLSKeyPackageCommand(b)))
}

func (g *daveCallbacks) SendMLSCommitWelcome(b []byte) error {
	return g.gateway.Send(context.Background(), new(voicegateway.MLSCommitWelcomeCommand(b)))
}

func (g *daveCallbacks) SendReadyForTransition(id uint16) error {
	return g.gateway.Send(context.Background(), &voicegateway.DAVETransitionReadyCommand{TransitionID: id})
}

func (g *daveCallbacks) SendInvalidCommitWelcome(id uint16) error {
	return g.gateway.Send(context.Background(), &voicegateway.MLSInvalidCommitWelcomeCommand{TransitionID: id})
}

type voiceGatewayHandler voiceGateway

func (g *voiceGatewayHandler) OnOp(ctx context.Context, op ws.Op) bool {
	if op.Sequence != 0 {
		g.mutex.Lock()
		g.seq = op.Sequence
		g.mutex.Unlock()
	}

	switch data := op.Data.(type) {
	case *voicegateway.HelloEvent:
		g.gateway.ResetHeartbeat(data.HeartbeatInterval.Duration())
		// A stream's connection is not resumed: one that drops is identified again.
		if err := g.gateway.Send(ctx, &g.identify); err != nil {
			g.gateway.SendErrorWrap(err, "failed to identify with the stream's voice server")
			g.gateway.QueueReconnect()
		}
	case *readyEvent:
		g.mutex.Lock()
		g.ready = true
		g.mutex.Unlock()
	case *sessionDescriptionEvent:
		g.dave.OnSelectProtocolAck(data.DAVEProtocolVersion)
	case *voicegateway.ClientsConnectEvent:
		for _, id := range data.UserIDs {
			g.dave.AddUser(godave.UserID(id.String()))
		}
	case *voicegateway.ClientDisconnectEvent:
		g.dave.RemoveUser(godave.UserID(data.UserID.String()))
	case *voicegateway.DAVEPrepareTransitionEvent:
		g.dave.OnDavePrepareTransition(data.TransitionID, data.ProtocolVersion)
	case *voicegateway.DAVEExecuteTransitionEvent:
		g.dave.OnDaveExecuteTransition(data.TransitionID)
	case *voicegateway.DAVEPrepareEpochEvent:
		g.dave.OnDavePrepareEpoch(data.Epoch, data.ProtocolVersion)
	case *voicegateway.MLSExternalSenderEvent:
		g.dave.OnDaveMLSExternalSenderPackage(*data)
	case *voicegateway.MLSProposalsEvent:
		g.dave.OnDaveMLSProposals(*data)
	case *voicegateway.MLSAnnounceCommitEvent:
		g.dave.OnDaveMLSPrepareCommitTransition(data.TransitionID, data.Commit)
	case *voicegateway.MLSWelcomeEvent:
		g.dave.OnDaveMLSWelcome(data.TransitionID, data.Welcome)
	}
	return true
}

func (g *voiceGatewayHandler) SendHeartbeat(ctx context.Context) {
	g.mutex.Lock()
	seq := g.seq
	g.mutex.Unlock()

	heartbeat := voicegateway.HeartbeatCommand{T: time.Now().UnixMilli(), SeqAck: seq}
	if err := g.gateway.Send(ctx, &heartbeat); err != nil {
		g.gateway.SendErrorWrap(err, "stream heartbeat failed")
		g.gateway.QueueReconnect()
	}
}

func (g *voiceGatewayHandler) Close() error {
	return g.dave.Close()
}
