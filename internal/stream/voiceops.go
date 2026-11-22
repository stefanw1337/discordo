package stream

import (
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/utils/ws"
	"github.com/ayn2op/arikawa/v3/voice/voicegateway"
)

// opUnmarshalers are the events of the voice gateway of a stream: those of a voice connection, with video.
var opUnmarshalers = ws.NewOpUnmarshalers(
	func() ws.Event { return new(readyEvent) },
	func() ws.Event { return new(sessionDescriptionEvent) },
	func() ws.Event { return new(videoEvent) },
	func() ws.Event { return new(sessionUpdateEvent) },
	func() ws.Event { return new(voicegateway.HelloEvent) },
	func() ws.Event { return new(voicegateway.HeartbeatAckEvent) },
	func() ws.Event { return new(voicegateway.SpeakingEvent) },
	func() ws.Event { return new(voicegateway.ResumedEvent) },
	func() ws.Event { return new(voicegateway.ClientsConnectEvent) },
	func() ws.Event { return new(voicegateway.ClientDisconnectEvent) },
	func() ws.Event { return new(voicegateway.DAVEPrepareTransitionEvent) },
	func() ws.Event { return new(voicegateway.DAVEExecuteTransitionEvent) },
	func() ws.Event { return new(voicegateway.DAVEPrepareEpochEvent) },
	func() ws.Event { return new(voicegateway.MLSExternalSenderEvent) },
	func() ws.Event { return new(voicegateway.MLSProposalsEvent) },
	func() ws.Event { return new(voicegateway.MLSAnnounceCommitEvent) },
	func() ws.Event { return new(voicegateway.MLSWelcomeEvent) },
)

// streamInfo describes a video stream, a simulcast layer, of a voice connection.
type streamInfo struct {
	Type    string `json:"type,omitempty"`
	RID     string `json:"rid,omitempty"`
	Quality int    `json:"quality,omitempty"`
	Active  bool   `json:"active"`
	SSRC    uint32 `json:"ssrc,omitempty"`
	RTXSSRC uint32 `json:"rtx_ssrc,omitempty"`
}

// identifyCommand is Opcode 0. It identifies with the voice server of a stream, which server_id names.
type identifyCommand struct {
	ServerID  string            `json:"server_id"`
	ChannelID discord.ChannelID `json:"channel_id"`
	UserID    discord.UserID    `json:"user_id"`
	SessionID string            `json:"session_id"`
	Token     string            `json:"token"`
	// Video and Streams tell that the connection is one with video, as a stream's is.
	Video                  bool         `json:"video"`
	Streams                []streamInfo `json:"streams"`
	MaxDAVEProtocolVersion int          `json:"max_dave_protocol_version"`
}

func (*identifyCommand) Op() ws.OpCode           { return 0 }
func (*identifyCommand) EventType() ws.EventType { return "" }

// codecInfo is a codec offered in Select Protocol: its name, kind and the payload types it is sent with.
type codecInfo struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Priority       int    `json:"priority"`
	PayloadType    uint8  `json:"payload_type"`
	RTXPayloadType uint8  `json:"rtx_payload_type,omitempty"`
	Encode         bool   `json:"encode"`
	Decode         bool   `json:"decode"`
}

// Payload types of the codecs offered. 96 is kept for probe packets.
const (
	payloadOpus    = 120
	payloadH264    = 101
	payloadH264RTX = 102
	payloadVP8     = 103
	payloadVP8RTX  = 104
)

// offeredCodecs are the codecs that can be watched in, H.264 first, as Discord's clients default to it.
var offeredCodecs = []codecInfo{
	{Name: codecOpus, Type: "audio", Priority: 1000, PayloadType: payloadOpus, Decode: true},
	{Name: codecH264, Type: "video", Priority: 1000, PayloadType: payloadH264, RTXPayloadType: payloadH264RTX, Decode: true},
	{Name: codecVP8, Type: "video", Priority: 2000, PayloadType: payloadVP8, RTXPayloadType: payloadVP8RTX, Decode: true},
}

// selectProtocolCommand is Opcode 1. It tells the voice server where to send UDP to, how to encrypt it, and the
// codecs that can be decoded.
type selectProtocolCommand struct {
	Protocol string                          `json:"protocol"`
	Data     voicegateway.SelectProtocolData `json:"data"`
	Codecs   []codecInfo                     `json:"codecs"`
}

func (*selectProtocolCommand) Op() ws.OpCode           { return 1 }
func (*selectProtocolCommand) EventType() ws.EventType { return "" }

// readyEvent is Opcode 2. It gives the connection's SSRC and the voice server's UDP address.
type readyEvent struct {
	SSRC    uint32       `json:"ssrc"`
	IP      string       `json:"ip"`
	Port    int          `json:"port"`
	Modes   []string     `json:"modes"`
	Streams []streamInfo `json:"streams"`
}

func (*readyEvent) Op() ws.OpCode           { return 2 }
func (*readyEvent) EventType() ws.EventType { return "" }

// sessionDescriptionEvent is Opcode 4. It gives the key that UDP is encrypted with, and the codecs to send in.
type sessionDescriptionEvent struct {
	Mode                string   `json:"mode"`
	SecretKey           [32]byte `json:"secret_key"`
	AudioCodec          string   `json:"audio_codec"`
	VideoCodec          string   `json:"video_codec"`
	DAVEProtocolVersion uint16   `json:"dave_protocol_version"`
}

func (*sessionDescriptionEvent) Op() ws.OpCode           { return 4 }
func (*sessionDescriptionEvent) EventType() ws.EventType { return "" }

// videoEvent is Opcode 12. Received, it tells the SSRCs a user sends audio and video with. Sent, it announces the
// connection's own, which a viewer must do once before it is sent video.
type videoEvent struct {
	UserID    discord.UserID `json:"user_id,omitempty"`
	AudioSSRC uint32         `json:"audio_ssrc"`
	VideoSSRC uint32         `json:"video_ssrc"`
	RTXSSRC   uint32         `json:"rtx_ssrc"`
	Streams   []streamInfo   `json:"streams"`
}

func (*videoEvent) Op() ws.OpCode           { return 12 }
func (*videoEvent) EventType() ws.EventType { return "" }

// primary returns the SSRC and RTX SSRC of the video the event tells of: those of its best active stream, or its
// top-level ones.
func (e videoEvent) primary() (ssrc, rtx uint32) {
	best := -1
	for _, stream := range e.Streams {
		if stream.SSRC != 0 && stream.Active && stream.Quality > best {
			best, ssrc, rtx = stream.Quality, stream.SSRC, stream.RTXSSRC
		}
	}
	if ssrc == 0 {
		ssrc, rtx = e.VideoSSRC, e.RTXSSRC
	}
	// An RTX SSRC that is not given is the one after the primary.
	if rtx == 0 && ssrc != 0 {
		rtx = ssrc + 1
	}
	return ssrc, rtx
}

// sessionUpdateEvent is Opcode 14. It changes the codecs of the session.
type sessionUpdateEvent struct {
	AudioCodec string `json:"audio_codec,omitempty"`
	VideoCodec string `json:"video_codec,omitempty"`
}

func (*sessionUpdateEvent) Op() ws.OpCode           { return 14 }
func (*sessionUpdateEvent) EventType() ws.EventType { return "" }

// mediaSinkWantsCommand is Opcode 15. It asks for the remote SSRCs wanted, from 0 for none to 100 for the best
// quality, by SSRC or "any".
type mediaSinkWantsCommand map[string]int

func (*mediaSinkWantsCommand) Op() ws.OpCode           { return 15 }
func (*mediaSinkWantsCommand) EventType() ws.EventType { return "" }
