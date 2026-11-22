package guildstree

import (
	"errors"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/stream"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/keybind"
	"github.com/ayn2op/tview/tree"
	dave "github.com/thomas-vilte/dave-go/session"
)

// StreamMsg reports that what is being watched, or how it is going, changed.
type StreamMsg struct{}

func newWatcher(m Model) *stream.Watcher {
	player := stream.PlayerConfig{Path: m.cfg.Stream.Player, Args: m.cfg.Stream.PlayerArgs}
	return stream.NewWatcher(m.state.State, dave.CreateFunc(), player, m.voiceSession)
}

// listenStream waits for the watcher's status to change.
func (m Model) listenStream() tview.Cmd {
	return cmd.Task(func() tview.Msg {
		<-m.watcher.Updates()
		return StreamMsg{}
	})
}

// watching returns the user whose stream is being watched, or 0 if none is.
func (m Model) watching() discord.UserID {
	status := m.watcher.Status()
	if !status.Watching {
		return 0
	}
	_, _, userID, err := status.Key.Parse()
	if err != nil {
		return 0
	}
	return userID
}

// streamMarkers returns what shows that the user that state is of streams, and that their stream is watched.
func (m Model) streamMarkers(state discord.VoiceState) string {
	switch {
	case state.UserID == m.watching():
		return m.cfg.Icons.VoiceWatching
	case state.SelfStream:
		return m.cfg.Icons.VoiceStreaming
	}
	return ""
}

// streamOf returns the key of the stream of the voice member of node, and whether they are streaming.
func (m Model) streamOf(node *tree.Node) (stream.Key, *discord.Channel, bool) {
	userID, channel := m.voiceMember(node)
	if channel == nil {
		return "", nil, false
	}
	state, err := m.state.VoiceState(channel.GuildID, userID)
	if err != nil || !state.SelfStream {
		return "", channel, false
	}
	return stream.NewKey(channel.GuildID, channel.ID, userID), channel, true
}

// toggleWatch watches the stream of the voice member of node, or stops watching it if it is being watched.
func (m Model) toggleWatch(node *tree.Node) tview.Cmd {
	key, channel, ok := m.streamOf(node)
	if channel == nil {
		return nil
	}
	if !ok {
		return cmd.Task(func() tview.Msg { return ui.ErrorDialog("watch", errors.New("they are not streaming")) })
	}

	status := m.watcher.Status()
	if status.Watching && status.Key == key {
		m.watcher.Stop()
		return nil
	}
	if m.voiceSession.Status().ChannelID != channel.ID {
		err := errors.New("join " + channel.Name + " first, with " + m.cfg.Keybinds.GuildsTree.ToggleVoice.Help().Key)
		return cmd.Task(func() tview.Msg { return ui.ErrorDialog("watch", err) })
	}

	m.watcher.Watch(key)
	return nil
}

// streamChanged shows the new status of the stream watched, and why it ended if it ended on its own.
func (m Model) streamChanged() tview.Cmd {
	if channelID := m.voiceSession.Status().ChannelID; channelID.IsValid() {
		if channel, err := m.state.Cabinet.Channel(channelID); err == nil {
			m.showVoiceMembers(channel.GuildID)
		}
	}

	listen := m.listenStream()
	status := m.watcher.Status()
	if status.Err == nil || errors.Is(status.Err, stream.ErrStreamEnded) {
		return listen
	}
	err := status.Err
	return cmd.Batch{listen, cmd.Task(func() tview.Msg { return ui.ErrorDialog("watch the stream", err) })}
}

// watchKeybinds returns the keybind that watches the stream of the highlighted voice member, if they are streaming.
func (m Model) watchKeybinds() []keybind.Keybind {
	key, _, ok := m.streamOf(m.current)
	if !ok {
		return nil
	}
	watch := m.cfg.Keybinds.GuildsTree.WatchStream.Keybind
	if status := m.watcher.Status(); status.Watching && status.Key == key {
		watch = watch.WithHelp(watch.Help().Key, "stop watching")
	}
	return []keybind.Keybind{watch}
}

// streamFooter returns what is being watched, or "".
func (m Model) streamFooter() string {
	status := m.watcher.Status()
	if !status.Watching {
		return ""
	}
	name := "a stream"
	if guildID, _, userID, err := status.Key.Parse(); err == nil {
		if member, err := m.state.Cabinet.Member(guildID, userID); err == nil {
			name = member.User.DisplayOrUsername()
			if member.Nick != "" {
				name = member.Nick
			}
		}
	}
	if !status.Playing {
		return "Connecting to " + name + "'s stream..."
	}
	return "Watching " + name + " (" + status.Codec + ")"
}
