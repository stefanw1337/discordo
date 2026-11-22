package guildstree

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview/tree"
)

// newFavoritesModel returns a tree that keeps its favorites in a temporary directory, with guild 1 "Drammen" and its
// text channel 2 "general", category 3 and voice channel 4 in the state.
func newFavoritesModel(t *testing.T) Model {
	t.Helper()
	s := ningen.FromState(state.NewWithStore("", defaultstore.New()))
	me := discord.User{ID: 9}
	guild := discord.Guild{ID: 1, Name: "Drammen", OwnerID: me.ID}
	if err := errors.Join(
		s.MyselfSet(me, false),
		s.GuildSet(&guild, false),
		s.MemberSet(guild.ID, &discord.Member{User: me}, false),
		s.RoleSet(guild.ID, &discord.Role{ID: discord.RoleID(guild.ID)}, false),
		s.ChannelSet(&discord.Channel{ID: 2, GuildID: 1, Name: "general", Type: discord.GuildText}, false),
		s.ChannelSet(&discord.Channel{ID: 3, GuildID: 1, Name: "Text", Type: discord.GuildCategory}, false),
		s.ChannelSet(&discord.Channel{ID: 4, GuildID: 1, Name: "Talk", Type: discord.GuildVoice}, false),
	); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	m := NewModel(&cfg, s)
	m.favorites = loadFavorites(filepath.Join(t.TempDir(), favoritesFileName))
	m.reset()
	return m
}

func TestModelToggleFavorite(t *testing.T) {
	m := newFavoritesModel(t)
	general := tree.NewNode("general").SetReference(discord.ChannelID(2))

	m.toggleFavorite(general)
	if m.favoritesRoot == nil || m.root.Children()[0] != m.favoritesRoot {
		t.Fatal("the favorites are not listed first")
	}
	children := m.favoritesRoot.Children()
	if len(children) != 1 || children[0].Reference() != favoriteRef(2) {
		t.Fatalf("favorites = %v, want general", children)
	}

	t.Run("kept in the file", func(t *testing.T) {
		if got := loadFavorites(m.favorites.path).ids; len(got) != 1 || got[0] != 2 {
			t.Fatalf("saved favorites = %v, want [2]", got)
		}
	})

	t.Run("not a category or voice channel", func(t *testing.T) {
		for _, id := range []discord.ChannelID{3, 4} {
			m.toggleFavorite(tree.NewNode("").SetReference(id))
		}
		if len(m.favorites.ids) != 1 {
			t.Fatalf("favorites = %v, want only general", m.favorites.ids)
		}
	})

	t.Run("the channel's own node is still found by its ID", func(t *testing.T) {
		if m.findNodeByRef(discord.ChannelID(2)) == children[0] {
			t.Fatal("the favorite node stands in for the channel's own")
		}
	})

	t.Run("removed from the favorite itself", func(t *testing.T) {
		m.current = children[0]
		m.toggleFavorite(children[0])
		if m.favoritesRoot != nil || len(m.favorites.ids) != 0 {
			t.Fatal("the favorite was not removed")
		}
		if m.current != m.root {
			t.Fatal("the removed favorite is still highlighted")
		}
	})
}

func TestLoadFavorites(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if got := loadFavorites(filepath.Join(t.TempDir(), "none.json")).ids; len(got) != 0 {
			t.Fatalf("favorites = %v, want none", got)
		}
	})
}
