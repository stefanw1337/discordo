package chat

import (
	"log/slog"
	"slices"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/ayn2op/arikawa/v3/utils/ws"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
)

// Calls of direct messages and groups, which Discord does not document: CALL_CREATE when one starts, CALL_UPDATE
// when who is rung changes, CALL_DELETE when it ends. See https://docs.discord.food/resources/voice#call-object.
func init() {
	gateway.OpUnmarshalers.Add(
		func() ws.Event { return new(callCreateEvent) },
		func() ws.Event { return new(callUpdateEvent) },
		func() ws.Event { return new(callDeleteEvent) },
	)
}

// call is a call of a direct message or group.
type call struct {
	ChannelID discord.ChannelID `json:"channel_id"`
	MessageID discord.MessageID `json:"message_id"`
	// Ringing are the users being rung.
	Ringing []discord.UserID `json:"ringing"`
}

type (
	callCreateEvent struct{ call }
	callUpdateEvent struct{ call }
	callDeleteEvent struct {
		ChannelID discord.ChannelID `json:"channel_id"`
	}
)

func (*callCreateEvent) Op() ws.OpCode           { return 0 }
func (*callCreateEvent) EventType() ws.EventType { return "CALL_CREATE" }
func (*callUpdateEvent) Op() ws.OpCode           { return 0 }
func (*callUpdateEvent) EventType() ws.EventType { return "CALL_UPDATE" }
func (*callDeleteEvent) Op() ws.OpCode           { return 0 }
func (*callDeleteEvent) EventType() ws.EventType { return "CALL_DELETE" }

// incomingCall is a call ringing the user.
type incomingCall struct {
	channelID discord.ChannelID
	// from is who is calling, or the group's name.
	from string
}

// updateCall keeps up with whether c rings the user, telling them when it starts to.
func (m *Model) updateCall(c call) tview.Cmd {
	me, _ := m.state.Cabinet.Me()
	rung := me != nil && slices.Contains(c.Ringing, me.ID)
	if !rung {
		if m.ringing != nil && m.ringing.channelID == c.ChannelID {
			m.ringing = nil
		}
		return nil
	}
	if m.ringing != nil && m.ringing.channelID == c.ChannelID {
		return nil
	}

	from := "Someone"
	if channel, err := m.state.Cabinet.Channel(c.ChannelID); err == nil {
		switch {
		case channel.Name != "":
			from = channel.Name
		case len(channel.DMRecipients) > 0:
			from = channel.DMRecipients[0].DisplayOrUsername()
		}
	}
	m.ringing = &incomingCall{channelID: c.ChannelID, from: from}
	if !m.cfg.Notifications.Enabled || m.cfg.Status == discord.DoNotDisturbStatus {
		return nil
	}
	return cmd.Task(func() tview.Msg { return cmd.Notify{Title: "Incoming call", Body: from + " is calling you"} })
}

// callFooter tells of the call ringing the user, and how to answer it.
func (m Model) callFooter() string {
	if m.ringing == nil {
		return ""
	}
	keybinds := m.cfg.Keybinds
	return "📞 " + m.ringing.from + " is calling — " + keybinds.AnswerCall.Keybind.Help().Key + " answer, " +
		keybinds.DeclineCall.Keybind.Help().Key + " decline"
}

// answerCall joins the call ringing the user.
func (m *Model) answerCall() tview.Cmd {
	if m.ringing == nil {
		return nil
	}
	channelID := m.ringing.channelID
	m.ringing = nil
	return m.guildsTree.JoinCall(channelID)
}

// declineCall stops the call ringing the user, as Discord's decline does.
func (m *Model) declineCall() tview.Cmd {
	if m.ringing == nil {
		return nil
	}
	channelID := m.ringing.channelID
	m.ringing = nil
	me, _ := m.state.Cabinet.Me()
	if me == nil {
		return nil
	}
	client := m.state.Client
	return cmd.Task(func() tview.Msg {
		body := struct {
			Recipients []discord.UserID `json:"recipients"`
		}{[]discord.UserID{me.ID}}
		if err := client.FastRequest("POST", api.EndpointChannels+channelID.String()+"/call/stop-ringing", httputil.WithJSONBody(body)); err != nil {
			slog.Error("failed to decline call", "channel_id", channelID, "err", err)
		}
		return nil
	})
}

// friendRequestArrived tells the user of a friend request sent to them.
func (m *Model) friendRequestArrived(relationship discord.Relationship) tview.Cmd {
	if relationship.Type != discord.IncomingFriendRequest {
		return nil
	}
	if m.overlay == overlayFriendRequests {
		m.friendRequests.Open()
	}
	if !m.cfg.Notifications.Enabled || m.cfg.Status == discord.DoNotDisturbStatus {
		return nil
	}
	name := relationship.User.DisplayOrUsername()
	key := m.cfg.Keybinds.ToggleFriendRequests.Keybind.Help().Key
	return cmd.Task(func() tview.Msg {
		return cmd.Notify{Title: "Friend request", Body: name + " wants to be your friend (" + key + " in discordo)"}
	})
}
