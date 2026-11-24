package chat

import (
	"fmt"
	"time"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
)

// forwardReference is a forward of a message, as Discord's client sends it: a reference of type 1.
type forwardReference struct {
	Type      int               `json:"type"`
	MessageID discord.MessageID `json:"message_id"`
	ChannelID discord.ChannelID `json:"channel_id"`
	GuildID   discord.GuildID   `json:"guild_id,omitempty"`
}

// startForward opens the channels picker to pick where to forward message.
func (m *Model) startForward(message discord.Message) {
	m.openPicker()
	m.forwarding = &message
	m.channelsPicker.SetTitle("Forward to (enter sends)")
}

// endForward stops forwarding and gives the channels picker its title back.
func (m *Model) endForward() {
	m.forwarding = nil
	m.channelsPicker.SetTitle("Channels")
}

// forward forwards message to the channel target.
func (m *Model) forward(message discord.Message, target discord.ChannelID) tview.Cmd {
	guildID := message.GuildID
	if !guildID.IsValid() {
		if channel, err := m.state.Cabinet.Channel(message.ChannelID); err == nil {
			guildID = channel.GuildID
		}
	}
	body := struct {
		Reference forwardReference `json:"message_reference"`
		Nonce     string           `json:"nonce"`
	}{
		Reference: forwardReference{Type: 1, MessageID: message.ID, ChannelID: message.ChannelID, GuildID: guildID},
		Nonce:     fmt.Sprint(discord.NewSnowflake(time.Now())),
	}
	state := m.state
	return cmd.Task(func() tview.Msg {
		if err := state.FastRequest("POST", api.EndpointChannels+target.String()+"/messages", httputil.WithJSONBody(body)); err != nil {
			return ui.ErrorDialog("forward the message", err)
		}
		return nil
	})
}
