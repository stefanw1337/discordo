// Package stream watches a Go Live stream: it asks Discord for it on the main gateway, connects to the voice server
// that carries it, and plays the video it receives with mpv.
//
// Discord does not document Go Live. This follows the user documentation at https://docs.discord.food.
package stream

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/utils/ws"
)

func init() {
	gateway.OpUnmarshalers.Add(
		func() ws.Event { return new(CreateEvent) },
		func() ws.Event { return new(ServerUpdateEvent) },
		func() ws.Event { return new(DeleteEvent) },
	)
}

// Key is the stream key of a user's stream in a voice channel: "guild:<guild>:<channel>:<user>", or
// "call:<channel>:<user>" in a DM or group DM.
type Key string

// NewKey returns the key of the stream of userID in channelID, in guildID if it is valid.
func NewKey(guildID discord.GuildID, channelID discord.ChannelID, userID discord.UserID) Key {
	if guildID.IsValid() {
		return Key("guild:" + guildID.String() + ":" + channelID.String() + ":" + userID.String())
	}
	return Key("call:" + channelID.String() + ":" + userID.String())
}

// Parse returns the guild, if any, channel and owner of the stream with key k.
func (k Key) Parse() (guildID discord.GuildID, channelID discord.ChannelID, userID discord.UserID, err error) {
	parts := strings.Split(string(k), ":")
	ids := make([]uint64, 0, 3)
	switch {
	case len(parts) == 4 && parts[0] == "guild":
		parts = parts[1:]
	case len(parts) == 3 && parts[0] == "call":
		parts = parts[1:]
	default:
		return 0, 0, 0, fmt.Errorf("invalid stream key %q", k)
	}
	for _, part := range parts {
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid stream key %q: %w", k, err)
		}
		ids = append(ids, id)
	}

	if len(ids) == 3 {
		return discord.GuildID(ids[0]), discord.ChannelID(ids[1]), discord.UserID(ids[2]), nil
	}
	return 0, discord.ChannelID(ids[0]), discord.UserID(ids[1]), nil
}

// WatchCommand is Opcode 20 of the gateway. It asks to watch the stream with StreamKey, which answers with a
// CreateEvent and a ServerUpdateEvent, or a DeleteEvent if it cannot be watched.
type WatchCommand struct {
	StreamKey Key `json:"stream_key"`
}

func (*WatchCommand) Op() ws.OpCode           { return 20 }
func (*WatchCommand) EventType() ws.EventType { return "" }

// StopCommand is Opcode 19 of the gateway. It stops watching, or streaming, the stream with StreamKey.
type StopCommand struct {
	StreamKey Key `json:"stream_key"`
}

func (*StopCommand) Op() ws.OpCode           { return 19 }
func (*StopCommand) EventType() ws.EventType { return "" }

// CreateEvent is the STREAM_CREATE dispatch event. It tells of a stream that is watched or started.
type CreateEvent struct {
	StreamKey Key `json:"stream_key"`
	// RTCServerID is the server_id to identify with on the voice server of the stream.
	RTCServerID string           `json:"rtc_server_id"`
	Region      string           `json:"region"`
	ViewerIDs   []discord.UserID `json:"viewer_ids"`
	Paused      bool             `json:"paused"`
}

func (*CreateEvent) Op() ws.OpCode           { return 0 }
func (*CreateEvent) EventType() ws.EventType { return "STREAM_CREATE" }

// ServerUpdateEvent is the STREAM_SERVER_UPDATE dispatch event. It gives the voice server of a stream to connect
// to, and the token to identify with there.
type ServerUpdateEvent struct {
	StreamKey Key    `json:"stream_key"`
	Token     string `json:"token"`
	Endpoint  string `json:"endpoint"`
}

func (*ServerUpdateEvent) Op() ws.OpCode           { return 0 }
func (*ServerUpdateEvent) EventType() ws.EventType { return "STREAM_SERVER_UPDATE" }

// DeleteEvent is the STREAM_DELETE dispatch event. It tells that a stream ended, or could not be watched, and why.
type DeleteEvent struct {
	StreamKey Key `json:"stream_key"`
	// Reason is why, such as "user_requested", "stream_ended", "stream_full" or "unauthorized".
	Reason      string `json:"reason"`
	Unavailable bool   `json:"unavailable"`
}

func (*DeleteEvent) Op() ws.OpCode           { return 0 }
func (*DeleteEvent) EventType() ws.EventType { return "STREAM_DELETE" }
