package guildstree

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview/tree"
)

// treeStateFileName is the file, beside the config file, that keeps which folders, servers and categories are open
// and which server the bar has open, so that the tree is as it was left when discordo is started again.
const treeStateFileName = "tree.json"

type treeState struct {
	path string
	// Expanded holds, by the key nodeKey gives, whether each node was left open.
	Expanded map[string]bool `json:"expanded"`
	// Bar is the key of the server the bar had open.
	Bar string `json:"bar,omitempty"`
}

// treeStatePath returns where the state is kept: beside the config file read, or nowhere if there was none to read,
// as in tests.
func treeStatePath(cfg *config.Config) string {
	if cfg.Path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.Path), treeStateFileName)
}

func loadTreeState(path string) *treeState {
	if path == "" {
		return &treeState{Expanded: map[string]bool{}}
	}
	state := &treeState{path: path, Expanded: map[string]bool{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state
	}
	if err != nil {
		slog.Error("failed to read tree state", "path", path, "err", err)
		return state
	}
	if err := json.Unmarshal(data, state); err != nil {
		slog.Error("failed to parse tree state", "path", path, "err", err)
	}
	if state.Expanded == nil {
		state.Expanded = map[string]bool{}
	}
	return state
}

// nodeKey returns the key the state keeps node under, or "" for a node whose state is not kept.
func nodeKey(node *tree.Node) string {
	switch ref := node.Reference().(type) {
	case discord.GuildID:
		return "guild:" + ref.String()
	case discord.ChannelID:
		if node.Expandable() {
			return "channel:" + ref.String()
		}
	case folderNode:
		return "folder:" + strconv.FormatInt(int64(ref.id), 10)
	case dmNode:
		return "dm"
	}
	return ""
}

// restore opens or closes node as it was left, and reports whether it was left open, or false if it is not known.
func (s *treeState) restore(node *tree.Node) (expanded, known bool) {
	if s == nil {
		return false, false
	}
	expanded, known = s.Expanded[nodeKey(node)]
	if known {
		node.SetExpanded(expanded)
	}
	return expanded, known
}

// record notes whether each node under root that has a kept state is open, and saves it if that changed.
func (s *treeState) record(root *tree.Node, bar *tree.Node) {
	if s == nil || s.path == "" {
		return
	}
	expanded := maps.Clone(s.Expanded)
	root.Walk(func(node, _ *tree.Node) bool {
		// A server whose channels were never loaded keeps the state it was left in.
		if _, ok := node.Reference().(discord.GuildID); ok && len(node.Children()) == 0 {
			return true
		}
		if key := nodeKey(node); key != "" {
			expanded[key] = node.Expanded()
		}
		return true
	})
	barKey := ""
	if bar != nil {
		barKey = nodeKey(bar)
	}
	if maps.Equal(expanded, s.Expanded) && barKey == s.Bar {
		return
	}
	s.Expanded, s.Bar = expanded, barKey

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		slog.Error("failed to make the folder of the tree state", "err", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		slog.Error("failed to save tree state", "path", s.path, "err", err)
	}
}
