package mentionsinbox

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/ningen/v3"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s := ningen.FromState(state.NewWithStore("", defaultstore.New()))
	if err := errors.Join(
		s.GuildSet(&discord.Guild{ID: 1, Name: "Drammen"}, false),
		s.ChannelSet(&discord.Channel{ID: 2, GuildID: 1, Name: "general", Type: discord.GuildText}, false),
	); err != nil {
		t.Fatal(err)
	}
	return NewModel(cfg, s)
}

func TestModelUpdate(t *testing.T) {
	message := discord.Message{
		ID: 3, ChannelID: 2, Author: discord.User{Username: "kari"}, Content: "hei  @stefan,\nser du dette?",
		Timestamp: discord.NewTimestamp(time.Now()),
	}

	t.Run("lists the mentions", func(t *testing.T) {
		m := newTestModel(t)
		m.Open()
		m, _ = m.Update(loadedMsg{request: m.request, messages: []discord.Message{message}})
		items := m.Visible()
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if got, want := items[0].Reference, (SelectedMsg{ChannelID: 2, MessageID: 3}); got != want {
			t.Fatalf("reference = %v, want %v", got, want)
		}
		for _, want := range []string{"kari", "general (Drammen)", "hei @stefan, ser du dette?"} {
			if !strings.Contains(items[0].FilterText, want) {
				t.Errorf("filter text %q lacks %q", items[0].FilterText, want)
			}
		}
	})

	t.Run("drops a replaced load", func(t *testing.T) {
		m := newTestModel(t)
		m.Open()
		stale := m.request
		m.Open()
		m, _ = m.Update(loadedMsg{request: stale, messages: []discord.Message{message}})
		if items := m.Visible(); len(items) != 1 || items[0].Reference != nil {
			t.Fatal("a replaced load was shown")
		}
	})

	t.Run("says when there are none or it failed", func(t *testing.T) {
		for _, msg := range []loadedMsg{{}, {err: errors.New("offline")}} {
			m := newTestModel(t)
			m.Open()
			msg.request = m.request
			m, _ = m.Update(msg)
			if items := m.Visible(); len(items) != 1 || items[0].Reference != nil {
				t.Fatalf("got %v, want one notice", items)
			}
		}
	})
}
