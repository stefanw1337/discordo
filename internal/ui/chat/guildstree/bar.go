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

// barEntries returns the nodes the bar lists: direct messages, then each server, those in folders too, in order.
func (m Model) barEntries() []*tree.Node {
	entries := []*tree.Node{}
	if m.dmRootNode != nil {
		entries = append(entries, m.dmRootNode)
	}
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

// barOpenMsg opens the entry of the bar at index; barScrollMsg scrolls the bar by entries.
type (
	barOpenMsg   int
	barScrollMsg int
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
	for i := b.offset; i < len(b.entries); i++ {
		y := area.Y + (i-b.offset)*(barRows+1)
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
	case tview.MouseLeftDown:
		_, y := mouse.Position()
		index := b.offset + (y-area.Y)/(barRows+1)
		if index < len(b.entries) {
			return barOpenMsg(index)
		}
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
	case barScrollMsg:
		m.barOffset = max(min(m.barOffset+int(msg), len(m.barEntries())-1), 0)
		return m, nil, true
	}
	return m, nil, false
}
