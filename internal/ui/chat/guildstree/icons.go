package guildstree

import (
	"log/slog"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/discordo/internal/ui/sixel"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/text"
	"github.com/ayn2op/tview/tree"
)

// Guild icons are shown before the names of guilds, one row tall, in terminals that draw images sharply. A folder shows
// the icons of its first guilds, as Discord does.
const (
	guildIconsBoth = "both"
	guildIconsIcon = "icon"
	guildIconsName = "name"
	// folderIcons is how many of its guilds' icons a folder shows.
	folderIcons = 4
)

// showIcons reports whether guild icons are shown: they are not asked to be left out, and images are drawn sharply,
// as half blocks would draw an icon a row tall with four pixels.
func (m Model) showIcons() bool {
	return m.cfg.UI.GuildsTree.GuildIcons != guildIconsName && sixel.Enabled()
}

// iconURL returns the icon of the guild, or "" if it has none.
func iconURL(guild discord.Guild) discord.URL {
	if guild.Icon == "" {
		return ""
	}
	return guild.IconURLWithType(discord.PNGImage) + "?size=64"
}

// LoadImages returns the command that asks for the icons of the guilds not loaded yet, or nil.
func (m Model) LoadImages() tview.Cmd {
	if !m.showIcons() {
		return nil
	}
	request := images.RequestedMsg{Squares: map[discord.URL]uint{}}
	for _, url := range m.guildIcons {
		if _, ok := m.images[url]; !ok && url != "" {
			request.Squares[url] = 1
		}
	}
	slog.Info("loading guild icons", "count", len(request.Squares), "guilds", len(m.guildIcons))
	if len(request.Squares) == 0 {
		return nil
	}
	return cmd.Task(func() tview.Msg { return request })
}

// icon returns the spans that draw the icon of guildID, or nil if it is not shown.
func (m Model) icon(guildID discord.GuildID) []text.Span {
	if !m.showIcons() {
		return nil
	}
	image, ok := m.images[m.guildIcons[guildID]]
	if !ok {
		return nil
	}
	lines := image.Lines()
	if len(lines) == 0 {
		return nil
	}
	// An image that could not be drawn with sixels would be half blocks, which a row cannot show.
	for _, span := range lines[0] {
		if !sixel.IsMarker(span.Style) {
			return nil
		}
	}
	return lines[0]
}

// nameIndex returns the index of the span of node's name, after its icons.
func nameIndex(spans []text.Span) int {
	for index, span := range spans {
		if !sixel.IsMarker(span.Style) {
			return index
		}
	}
	return len(spans)
}

// setGuildSpans sets the text of the node of a guild: its icon, then its name unless only the icon is shown.
func (m Model) setGuildSpans(node *tree.Node, guild discord.Guild) {
	m.guildIcons[guild.ID] = iconURL(guild)
	m.guildNames[guild.ID] = guild.Name
	m.setGuildSpansStyled(node, guild.ID, m.guildNodeStyle(guild.ID))
}

// setGuildSpansStyled sets the text of the node of guildID, whose name is styled with style.
func (m Model) setGuildSpansStyled(node *tree.Node, guildID discord.GuildID, style tview.Style) {
	icon := m.icon(guildID)
	mentions := mentionsLabel(m.guildMentions(guildID))
	name := m.guildNames[guildID] + mentions
	if len(icon) != 0 {
		if m.cfg.UI.GuildsTree.GuildIcons == guildIconsIcon {
			name = mentions
		} else {
			name = " " + name
		}
	}
	node.SetSpans(append(icon, text.Span{Text: name, Style: style})...)
}

// setFolderSpans sets the text of the node of a folder: the icons of its first guilds, then its name, which a folder
// without one shown by its icons goes without.
func (m Model) setFolderSpans(node *tree.Node, name string, style tview.Style) {
	var icons []text.Span
	for _, child := range node.Children() {
		if len(icons) >= folderIcons*2 {
			break
		}
		guildID, ok := child.Reference().(discord.GuildID)
		if !ok {
			continue
		}
		if icon := m.icon(guildID); len(icon) != 0 {
			icons = append(icons, icon...)
			icons = append(icons, text.Span{Text: " "})
		}
	}
	if len(icons) != 0 {
		icons = icons[:len(icons)-1]
		if name == defaultFolderName || m.cfg.UI.GuildsTree.GuildIcons == guildIconsIcon {
			name = ""
		} else {
			name = " " + name
		}
	}
	node.SetSpans(append(icons, text.Span{Text: name, Style: style})...)
}

// refreshIcons sets the text of every guild and folder node again, as the icons shown changed.
func (m Model) refreshIcons() {
	for guildID, node := range m.guildNodes {
		spans := node.Spans()
		if index := nameIndex(spans); index < len(spans) {
			m.setGuildSpansStyled(node, guildID, spans[index].Style)
		}
	}
	for _, node := range m.root.Children() {
		if folder, ok := node.Reference().(folderNode); ok {
			m.setFolderSpans(node, folder.name, folder.style)
		}
	}
}

// setImages keeps the images that were loaded, and shows the icons among them.
func (m Model) setImages(changed images.ChangedMsg) {
	clear(m.images)
	for url, image := range changed {
		m.images[url] = image
	}
	m.refreshIcons()
}
