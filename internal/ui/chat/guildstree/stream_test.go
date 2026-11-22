package guildstree

import (
	"errors"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/stream"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/tree"
)

// newStreamModel returns a tree with voice channel 2 of guild 1 shown, where user 5 streams and user 6 does not.
func newStreamModel(t *testing.T) (Model, *tree.Node, *tree.Node) {
	t.Helper()
	s := ningen.FromState(state.NewWithStore("", defaultstore.New()))
	me := discord.User{ID: 9}
	talk := discord.Channel{ID: 2, GuildID: 1, Name: "Talk", Type: discord.GuildVoice}
	if err := errors.Join(
		s.MyselfSet(me, false),
		s.GuildSet(&discord.Guild{ID: 1, OwnerID: me.ID}, false),
		s.ChannelSet(&talk, false),
		s.VoiceStateSet(1, &discord.VoiceState{GuildID: 1, ChannelID: 2, UserID: 5, SelfStream: true}, false),
		s.VoiceStateSet(1, &discord.VoiceState{GuildID: 1, ChannelID: 2, UserID: 6}, false),
	); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	m := NewModel(&cfg, s)
	m.favorites = loadFavorites(t.TempDir() + "/favorites.json")
	channelNode := tree.NewNode("Talk").SetReference(talk.ID)
	streamer := tree.NewNode("streamer").SetReference(discord.UserID(5))
	other := tree.NewNode("other").SetReference(discord.UserID(6))
	channelNode.SetChildren([]*tree.Node{streamer, other})
	m.root.AddChild(channelNode)
	m.channelNodes[talk.ID] = channelNode
	return m, streamer, other
}

func TestModelStream(t *testing.T) {
	m, streamer, other := newStreamModel(t)

	t.Run("marks who streams", func(t *testing.T) {
		if got := m.streamMarkers(discord.VoiceState{UserID: 5, SelfStream: true}); got != m.cfg.Icons.VoiceStreaming {
			t.Fatalf("markers = %q, want %q", got, m.cfg.Icons.VoiceStreaming)
		}
		if got := m.streamMarkers(discord.VoiceState{UserID: 6}); got != "" {
			t.Fatalf("markers = %q, want none", got)
		}
	})

	t.Run("knows the stream's key", func(t *testing.T) {
		key, _, ok := m.streamOf(streamer)
		if !ok || key != stream.NewKey(1, 2, 5) {
			t.Fatalf("streamOf = %q %v", key, ok)
		}
		if _, _, ok := m.streamOf(other); ok {
			t.Fatal("a member who does not stream has a stream")
		}
		m.current = streamer
		if len(m.watchKeybinds()) != 1 {
			t.Fatal("the watch keybind is not offered on a streamer")
		}
	})

	for _, test := range []struct {
		name string
		node *tree.Node
	}{
		{"who does not stream", other},
		{"outside the voice channel joined", streamer},
	} {
		t.Run("does not watch "+test.name, func(t *testing.T) {
			task, ok := m.toggleWatch(test.node).(cmd.Task)
			if !ok {
				t.Fatal("no reason was given")
			}
			if _, ok := task().(ui.DialogMsg); !ok {
				t.Fatal("the reason is not shown")
			}
			if m.watcher.Status().Watching {
				t.Fatal("a stream is being watched")
			}
		})
	}
}
