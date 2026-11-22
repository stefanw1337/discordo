package guildstree

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview/tree"
)

const (
	favoritesNodeName = "Favorites"
	favoritesFileName = "favorites.json"
)

// favoritesNode is the reference of the node that lists the favorite channels, above the direct messages.
type favoritesNode struct{}

// favoriteRef is the reference of a favorite channel's node under the favorites node. It is not a discord.ChannelID,
// so that the channel's own node, under its guild, is the one found by its ID.
type favoriteRef discord.ChannelID

// favorites are the channels marked favorite, in the order they were marked, kept in a file.
type favorites struct {
	path string
	ids  []discord.ChannelID
}

// loadFavorites reads the favorites kept at path. A file that does not exist yet holds none.
func loadFavorites(path string) *favorites {
	f := &favorites{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f
	}
	if err != nil {
		slog.Error("failed to read favorites", "path", path, "err", err)
		return f
	}

	var file struct {
		ChannelIDs []discord.ChannelID `json:"channel_ids"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		slog.Error("failed to parse favorites", "path", path, "err", err)
		return f
	}
	f.ids = file.ChannelIDs
	return f
}

func defaultFavoritesPath() string {
	return filepath.Join(config.Dir(), favoritesFileName)
}

func (f *favorites) contains(id discord.ChannelID) bool {
	return slices.Contains(f.ids, id)
}

// toggle marks the channel with id favorite, or unmarks it if it is, and saves the change.
func (f *favorites) toggle(id discord.ChannelID) error {
	if index := slices.Index(f.ids, id); index != -1 {
		f.ids = slices.Delete(f.ids, index, index+1)
	} else {
		f.ids = append(f.ids, id)
	}
	return f.save()
}

func (f *favorites) save() error {
	data, err := json.MarshalIndent(struct {
		ChannelIDs []discord.ChannelID `json:"channel_ids"`
	}{f.ids}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f.path, data, 0o644)
}

// refreshFavorites shows the favorite channels the state knows under the favorites node, which is shown first while
// there are any, keeping it expanded or collapsed.
func (m *Model) refreshFavorites() {
	clear(m.favoriteNodes)
	expanded := m.favoritesRoot == nil || m.favoritesRoot.Expanded()
	if m.favoritesRoot != nil {
		m.root.RemoveChild(m.favoritesRoot)
		if m.current == m.favoritesRoot || isFavorite(m.current) {
			m.current = m.root
		}
		m.favoritesRoot = nil
	}

	var children []*tree.Node
	for _, id := range m.favorites.ids {
		channel, err := m.state.Cabinet.Channel(id)
		if err != nil {
			continue
		}

		text := m.channelText(*channel)
		if guild, err := m.state.Cabinet.Guild(channel.GuildID); err == nil {
			text += " - " + guild.Name
		}
		node := tree.NewNode(text).SetReference(favoriteRef(id)).SetIndent(m.cfg.Sidebar.Indents.Channel)
		setNodeNameStyle(node, m.channelNodeStyle(*channel))
		m.favoriteNodes[id] = node
		children = append(children, node)
	}
	if len(children) == 0 {
		return
	}

	m.favoritesRoot = tree.NewNode(favoritesNodeName).
		SetReference(favoritesNode{}).
		SetExpandable(true).
		SetExpanded(expanded).
		SetChildren(children)
	m.root.SetChildren(slices.Insert(m.root.Children(), 0, m.favoritesRoot))
}

// refreshFavorite restyles the favorite node of the channel, if it is one.
func (m Model) refreshFavorite(channel discord.Channel) {
	node := m.favoriteNodes[channel.ID]
	if node == nil {
		return
	}

	text := m.channelText(channel)
	if guild, err := m.state.Cabinet.Guild(channel.GuildID); err == nil {
		text += " - " + guild.Name
	}
	setNodeText(node, text)
	setNodeNameStyle(node, m.channelNodeStyle(channel))
}

// toggleFavorite marks the channel of node favorite, or unmarks it, if node is a channel that can be opened.
func (m *Model) toggleFavorite(node *tree.Node) {
	var id discord.ChannelID
	switch ref := node.Reference().(type) {
	case favoriteRef:
		id = discord.ChannelID(ref)
	case discord.ChannelID:
		channel, err := m.state.Cabinet.Channel(ref)
		if err != nil || channel.Type == discord.GuildCategory || isVoice(channel.Type) {
			return
		}
		id = ref
	default:
		return
	}

	if err := m.favorites.toggle(id); err != nil {
		slog.Error("failed to save favorites", "path", m.favorites.path, "err", err)
	}
	m.refreshFavorites()
}

func isFavorite(node *tree.Node) bool {
	if node == nil {
		return false
	}
	_, ok := node.Reference().(favoriteRef)
	return ok
}
