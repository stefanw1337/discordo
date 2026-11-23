package guildstree

import (
	"strings"
	"unicode"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/layout"
	"github.com/ayn2op/tview/spacer"
	"github.com/ayn2op/tview/text"
	"github.com/ayn2op/tview/tree"
	"github.com/gdamore/tcell/v3"
)

// The server bar, which guild_icons = "bar" shows as Discord does, lists the servers by their icons, large, down the
// left of the tree, which shows the channels of the one open.
const (
	guildIconsBar = "bar"
	// barRows is how tall an icon of the bar is, three rows: 60 pixels in Windows Terminal.
	barRows = 3
	// barWidth is the width of the bar: the pill that marks the server open or unread, its icon, and a space.
	barWidth = 1 + 2*barRows + 1
)

// initialsBackground is the color behind the initials of a server without an icon, which tells them from its name.
var initialsBackground = tcell.NewRGBColor(54, 57, 63)

// initials returns the initials of name, as Discord shows for a server without an icon: the first letter or digit of
// each word, at most most of them.
func initials(name string, most int) string {
	var out []rune
	for _, word := range strings.Fields(name) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == most {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

// initialsSpan returns the initials of name drawn width cells wide on their background, in place of an icon.
func initialsSpan(name string, width int) text.Span {
	label := initials(name, width)
	label += strings.Repeat(" ", max(width-len([]rune(label)), 0))
	return text.Span{Text: label, Style: tcell.StyleDefault.Background(initialsBackground).Foreground(tcell.ColorWhite).Bold(true)}
}

// isIconSpan reports whether span is part of an icon: a sixel marker, or initials.
func isIconSpan(span text.Span) bool {
	return isMarker(span.Style) || span.Style.GetBackground() == initialsBackground
}

// barEntries returns the nodes the bar lists: direct messages, then each server, those in folders too, in the order
// they were dragged into, which is kept in discordo alone, and else in Discord's.
func (m Model) barEntries() []*tree.Node {
	servers := m.barServers()
	if m.treeState != nil && len(m.treeState.Order) > 0 {
		byKey := make(map[string]*tree.Node, len(servers))
		for _, node := range servers {
			byKey[nodeKey(node)] = node
		}
		ordered := make([]*tree.Node, 0, len(servers))
		for _, key := range m.treeState.Order {
			if node, ok := byKey[key]; ok {
				ordered = append(ordered, node)
				delete(byKey, key)
			}
		}
		// Servers joined since keep Discord's order, after those placed.
		for _, node := range servers {
			if _, ok := byKey[nodeKey(node)]; ok {
				ordered = append(ordered, node)
			}
		}
		servers = ordered
	}
	if m.dmRootNode != nil {
		return append([]*tree.Node{m.dmRootNode}, servers...)
	}
	return servers
}

// moveInBar moves the entry of the bar at from to where the one at to is, and keeps the order.
func (m *Model) moveInBar(from, to int) {
	entries := m.barEntries()
	// Direct messages stay first.
	if from <= 0 || to <= 0 || from >= len(entries) || to >= len(entries) || from == to {
		return
	}
	moved := entries[from]
	entries = append(entries[:from:from], entries[from+1:]...)
	entries = append(entries[:to], append([]*tree.Node{moved}, entries[to:]...)...)
	order := make([]string, 0, len(entries)-1)
	for _, entry := range entries[1:] {
		order = append(order, nodeKey(entry))
	}
	m.treeState.setOrder(order)
}

// barServers returns the servers in Discord's order, those in folders too.
func (m Model) barServers() []*tree.Node {
	entries := []*tree.Node{}
	for _, node := range m.root.Children() {
		switch node.Reference().(type) {
		case discord.GuildID:
			entries = append(entries, node)
		case folderNode:
			for _, child := range node.Children() {
				if _, ok := child.Reference().(discord.GuildID); ok {
					entries = append(entries, child)
				}
			}
		}
	}
	return entries
}

// barRoot returns the node of the server whose channels the tree shows: the one the highlighted node is in, or the
// one last opened, or direct messages.
func (m Model) barRoot() *tree.Node {
	if m.current != nil {
		for _, node := range m.root.PathTo(m.current) {
			switch node.Reference().(type) {
			case discord.GuildID, dmNode:
				return node
			}
		}
	}
	if m.barNode != nil {
		for _, entry := range m.barEntries() {
			if entry == m.barNode {
				return entry
			}
		}
	}
	return m.dmRootNode
}

// openInBar opens node, a server or direct messages, in the tree, loading its channels the first time, and
// highlights its first channel.
func (m *Model) openInBar(node *tree.Node) {
	if node == nil {
		return
	}
	if len(node.Children()) == 0 {
		switch ref := node.Reference().(type) {
		case discord.GuildID:
			m.expandGuild(node, ref)
		case dmNode:
			m.expandPrivateChannels(node)
		}
	}
	node.Expand()
	m.barNode = node
	if children := node.Children(); len(children) > 0 {
		m.setCurrentNode(children[0])
	} else {
		m.setCurrentNode(node)
	}
}

// openAdjacent opens the server before or after the open one in the bar.
func (m *Model) openAdjacent(previous bool) {
	entries := m.barEntries()
	open := m.barRoot()
	for i, entry := range entries {
		if entry != open {
			continue
		}
		next := i + 1
		if previous {
			next = i - 1
		}
		if next >= 0 && next < len(entries) {
			m.openInBar(entries[next])
			// The bar is scrolled to keep the server opened near its top.
			if next < m.barOffset || next > m.barOffset+3 {
				m.barOffset = max(next-3, 0)
			}
		}
		return
	}
}

// barOpenMsg opens the entry of the bar at index; barScrollMsg scrolls the bar by entries. barPressMsg is a press on
// an entry, and barReleaseMsg the release that follows: on the same entry it opens it, and on another it moves the
// pressed one there.
type (
	barOpenMsg    int
	barScrollMsg  int
	barPressMsg   int
	barReleaseMsg int
)

// barEntry is a server as the bar draws it.
type barEntry struct {
	icon     [][]text.Span
	name     string
	open     bool
	unread   bool
	mentions int
}

// bar is the server bar.
type bar struct {
	tview.Widget
	entries []barEntry
	offset  int
}

// barView returns the bar as it is now.
func (m Model) barView() bar {
	open := m.barRoot()
	nodes := m.barEntries()
	entries := make([]barEntry, len(nodes))
	for i, node := range nodes {
		entry := barEntry{open: node == open, unread: m.isUnread(node)}
		switch ref := node.Reference().(type) {
		case discord.GuildID:
			entry.name = m.guildNames[ref]
			entry.mentions = m.guildMentions(ref)
			if image, ok := m.images[m.guildIcons[ref]]; ok && m.showIcons() {
				entry.icon = image.Lines()
			}
		case dmNode:
			entry.name = "Direct Messages"
		}
		entries[i] = entry
	}
	return bar{Widget: spacer.New().Width(layout.Fixed(barWidth)).Height(layout.Fill), entries: entries, offset: m.barOffset}
}

func (b bar) Draw(screen tview.Screen, area tview.Rectangle) {
	pill := tcell.StyleDefault.Foreground(tcell.ColorWhite)
	// Scrolled no further than the bar stays full, as the wheel can scroll it past its end.
	shown := max((area.Height+1)/(barRows+1), 1)
	offset := max(min(b.offset, len(b.entries)-shown), 0)
	for i := offset; i < len(b.entries); i++ {
		y := area.Y + (i-offset)*(barRows+1)
		if y+barRows > area.Y+area.Height {
			break
		}
		entry := b.entries[i]
		// The pill: whole beside the server open, a dot beside one unread, red with its mentions.
		for row := range barRows {
			switch {
			case entry.open:
				screen.Put(area.X, y+row, "▌", pill)
			case entry.mentions > 0 && row == barRows-1:
				screen.Put(area.X, y+row, "●", tcell.StyleDefault.Foreground(tcell.ColorRed))
			case entry.unread && row == barRows/2:
				screen.Put(area.X, y+row, "•", pill)
			}
		}

		x := area.X + 1
		if len(entry.icon) >= barRows && allMarkers(entry.icon) {
			for row := range barRows {
				col := x
				for _, span := range entry.icon[row] {
					col += tview.Print(screen, span.Text, col, y+row, area.X+area.Width-col, tview.HorizontalAlignmentLeft, span.Style)
				}
			}
			continue
		}
		// A server without an icon, or whose icon is not loaded, shows its initials, as in Discord.
		span := initialsSpan(entry.name, 2*barRows)
		label := strings.TrimRight(span.Text, " ")
		screen.FillArea(x, y, 2*barRows, barRows, ' ', span.Style)
		tview.Print(screen, label, x, y+barRows/2, 2*barRows, tview.HorizontalAlignmentCenter, span.Style)
	}
}

func (b bar) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	mouse, ok := msg.(tview.MouseMsg)
	if !ok || !area.Contains(mouse.Position()) {
		return msg
	}
	switch mouse.Action {
	case tview.MouseLeftDown, tview.MouseLeftUp:
		_, y := mouse.Position()
		shown := max((area.Height+1)/(barRows+1), 1)
		index := max(min(b.offset, len(b.entries)-shown), 0) + (y-area.Y)/(barRows+1)
		if index >= len(b.entries) {
			index = len(b.entries) - 1
		}
		if mouse.Action == tview.MouseLeftDown {
			return barPressMsg(index)
		}
		return barReleaseMsg(index)
	case tview.MouseScrollUp:
		return barScrollMsg(-1)
	case tview.MouseScrollDown:
		return barScrollMsg(1)
	}
	return msg
}

// allMarkers reports whether every span of lines is a sixel marker, which an icon that can be drawn sharply is.
func allMarkers(lines [][]text.Span) bool {
	for _, line := range lines {
		for _, span := range line {
			if !isMarker(span.Style) {
				return false
			}
		}
	}
	return true
}

// updateBar handles the messages of the bar.
func (m Model) updateBar(msg tview.Msg) (Model, tview.Cmd, bool) {
	switch msg := msg.(type) {
	case barOpenMsg:
		entries := m.barEntries()
		if int(msg) < len(entries) {
			m.openInBar(entries[msg])
		}
		return m, nil, true
	case barPressMsg:
		m.barPressed = int(msg)
		return m, nil, true
	case barReleaseMsg:
		pressed := m.barPressed
		m.barPressed = -1
		entries := m.barEntries()
		switch {
		case pressed < 0 || pressed >= len(entries):
		case pressed == int(msg):
			m.openInBar(entries[pressed])
		default:
			m.moveInBar(pressed, int(msg))
		}
		return m, nil, true
	case barScrollMsg:
		m.barOffset = max(min(m.barOffset+int(msg), len(m.barEntries())-1), 0)
		return m, nil, true
	}
	return m, nil, false
}
