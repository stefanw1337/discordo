package guildstree

import (
	"path/filepath"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview/tree"
)

func TestTreeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tree.json")
	state := loadTreeState(path)

	root := tree.NewNode("")
	folder := tree.NewNode("f").SetReference(folderNode{id: 7}).SetExpanded(true)
	guild := tree.NewNode("g").SetReference(discord.GuildID(1)).SetExpandable(true)
	category := tree.NewNode("c").SetReference(discord.ChannelID(2)).SetExpandable(true).SetExpanded(true)
	unloaded := tree.NewNode("u").SetReference(discord.GuildID(3)).SetExpandable(true)
	guild.AddChild(category)
	folder.AddChild(guild)
	root.AddChild(folder)
	root.AddChild(unloaded)
	guild.SetExpanded(true)
	folder.SetExpanded(false)
	state.record(root, guild)

	// Started again: the same nodes, made closed, are opened and closed as they were left.
	again := loadTreeState(path)
	for _, test := range []struct {
		node *tree.Node
		want bool
	}{
		{tree.NewNode("").SetReference(folderNode{id: 7}).SetExpanded(true), false},
		{tree.NewNode("").SetReference(discord.GuildID(1)).SetExpandable(true), true},
		{tree.NewNode("").SetReference(discord.ChannelID(2)).SetExpandable(true), true},
	} {
		if expanded, known := again.restore(test.node); !known || expanded != test.want || test.node.Expanded() != test.want {
			t.Errorf("%s: expanded = %v (known %v), want %v", nodeKey(test.node), expanded, known, test.want)
		}
	}
	if _, known := again.restore(tree.NewNode("").SetReference(discord.GuildID(3)).SetExpandable(true)); known {
		t.Error("a server whose channels were never loaded was recorded")
	}
	if again.Bar != "guild:1" {
		t.Errorf("bar = %q, want guild:1", again.Bar)
	}
}

func TestEmptyCategoryLeftOut(t *testing.T) {
	s := ningen.FromState(state.NewWithStore("", defaultstore.New()))
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg, s)
	category := discord.Channel{ID: 10, GuildID: 1, Type: discord.GuildCategory, Name: "___hidden___"}
	// A channel the user cannot see, as no permissions are known here.
	hidden := discord.Channel{ID: 11, GuildID: 1, Type: discord.GuildText, ParentID: 10}
	s.State.Call(&gateway.GuildCreateEvent{Guild: discord.Guild{ID: 1}, Channels: []discord.Channel{category, hidden}})

	guild := tree.NewNode("g").SetReference(discord.GuildID(1))
	m.createChannelNodes(guild, []discord.Channel{category, hidden})
	if len(guild.Children()) != 0 {
		_, err := s.Cabinet.Channel(10)
		t.Fatalf("children = %d (its children %d, err %v), want the category without visible channels left out", len(guild.Children()), len(guild.Children()[0].Children()), err)
	}
}
