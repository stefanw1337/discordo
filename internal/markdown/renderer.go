package markdown

import (
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	md "github.com/ayn2op/arikawa/v3/markdown"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/text"
	"github.com/yuin/goldmark/v2/ast"
)

type Renderer struct {
	cfg *config.Config
}

const codeBlockIndent = "    "

func NewRenderer(cfg *config.Config) *Renderer {
	return &Renderer{cfg: cfg}
}

// RenderText returns the spans of the markdown node. emoji returns the image of the custom emoji with an ID as a line of spans, or nil if it has none.
func (r *Renderer) RenderText(source []byte, node ast.Node, base tview.Style, emoji func(id string) []text.Span) []text.Span {
	walker := &walker{renderer: r, source: source, emoji: emoji, builder: new(Builder), styles: []tview.Style{base}}
	_ = ast.Walk(node, walker.walk)
	return walker.builder.Finish()
}

// walker holds the state of one RenderText walk.
type walker struct {
	renderer *Renderer
	source   []byte
	emoji    func(id string) []text.Span

	builder   *Builder
	styles    []tview.Style
	linkDepth int
	listDepth int
	// linkDest is the destination of the link being walked, or "" outside of one.
	linkDest string
}

func (w *walker) walk(node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		return w.enter(node), nil
	}

	w.exit(node)
	return ast.WalkContinue, nil
}

func (w *walker) enter(node ast.Node) ast.WalkStatus {
	theme := w.renderer.cfg.Theme.MessagesList
	switch node := node.(type) {
	case *ast.Heading:
		w.builder.Write(strings.Repeat("#", node.Level)+" ", w.style())
	case *ast.Text:
		text := string(node.Value.Bytes(w.source))
		// A link whose text is its own destination shows like an autolink.
		if w.linkDest != "" && text == w.linkDest {
			text = w.renderer.linkText(text)
		}
		w.builder.Write(text, w.style())
		switch {
		case node.HardLineBreak():
			w.builder.NewLine()
			w.builder.NewLine()
		case node.SoftLineBreak():
			w.builder.NewLine()
		}
	case *ast.CodeBlock:
		w.builder.NewLine()
		w.renderer.renderCodeBlock(w.builder, w.source, node, w.style())
	case *ast.AutoLink:
		url := string(node.Destination.Bytes(w.source))
		w.builder.Write(w.renderer.linkText(url), tview.MergeStyle(w.style(), theme.URLStyle.Style).Url(url))
	case *ast.Link:
		url := string(node.Destination.Bytes(w.source))
		w.linkDest = url
		w.linkDepth++
		w.pushStyle(tview.MergeStyle(w.style(), theme.URLStyle.Style).Url(url))
	case *ast.List:
		w.builder.NewLine()
		w.listDepth++
	case *ast.ListItem:
		w.builder.Write(strings.Repeat("  ", w.listDepth-1)+listMarker(node), w.style())
	case *md.Inline:
		if w.renderer.shouldMask(node) {
			w.builder.Write("[spoiler]", w.style())
			return ast.WalkSkipChildren
		}

		w.pushStyle(applyInlineAttr(w.style(), node.Attr, w.linkDepth > 0))
	case *md.Mention:
		w.builder.Write(mentionText(node), tview.MergeStyle(w.style(), theme.MentionStyle.Style))
	case *md.Emoji:
		w.writeEmoji(node)
	case *md.Timestamp:
		w.builder.Write(node.Format(time.Now()), w.style().Reverse(true))
	}

	return ast.WalkContinue
}

func (w *walker) exit(node ast.Node) {
	switch node := node.(type) {
	case *ast.Heading:
		w.builder.NewLine()
	case *ast.ListItem:
		// A list that the item ends with already ended the line.
		if _, ok := node.LastChild().(*ast.List); !ok {
			w.builder.NewLine()
		}
	case *ast.Link:
		w.linkDest = ""
		w.linkDepth--
		w.popStyle()
	case *ast.List:
		w.listDepth--
	case *md.Inline:
		if !w.renderer.shouldMask(node) {
			w.popStyle()
		}
	}
}

// linkText returns what a link to url shows: the file name when ShortenFileLinks is set and url names a file, otherwise url.
// The link itself still opens url.
func (r *Renderer) linkText(url string) string {
	if !r.cfg.UI.MessagesList.ShortenFileLinks {
		return url
	}

	if name, ok := FileLinkName(url); ok {
		return name
	}
	return url
}

// FileLinkName returns the file name at the end of raw's path, and whether raw looks like a link to a file:
// it has a host, its path does not end in a slash, and its last path segment has an extension of 1 to 7 letters or digits.
func FileLinkName(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	// A path ending in a slash names a directory, not a file.
	if err != nil || parsed.Host == "" || parsed.Path == "" || strings.HasSuffix(parsed.Path, "/") {
		return "", false
	}

	name, err := url.PathUnescape(path.Base(parsed.Path))
	if err != nil || name == "" || name == "." || name == "/" {
		return "", false
	}

	ext := path.Ext(name)
	if len(ext) < 2 || len(ext) > 8 || name == ext {
		return "", false
	}
	for _, r := range ext[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "", false
		}
	}
	return name, true
}

func (w *walker) writeEmoji(node *md.Emoji) {
	emojis := w.renderer.cfg.UI.MessagesList.Emojis
	if image := w.emoji(node.ID); image != nil {
		w.builder.WriteSpans(image...)
		if !emojis.ShowEmojiName {
			return
		}

		w.builder.Write(" ", w.style())
	}

	w.builder.Write(emojis.Name(node.Name), tview.MergeStyle(w.style(), w.renderer.cfg.Theme.MessagesList.EmojiStyle.Style))
}

func (w *walker) style() tview.Style {
	return w.styles[len(w.styles)-1]
}

func (w *walker) pushStyle(style tview.Style) {
	w.styles = append(w.styles, style)
}

func (w *walker) popStyle() {
	if len(w.styles) > 1 {
		w.styles = w.styles[:len(w.styles)-1]
	}
}

func (r *Renderer) renderCodeBlock(builder *Builder, source []byte, node *ast.CodeBlock, base tview.Style) {
	var code strings.Builder
	for _, line := range node.Value.Segments() {
		code.Write(line.Bytes(source))
	}

	language, _ := node.Language(source)
	r.writeCodeBlock(builder, code.String(), strings.TrimSpace(language), base)
}

// writeCode writes code in style, starting each line after the first with the code block indent in base.
func writeCode(builder *Builder, code string, style, base tview.Style) {
	for index, line := range strings.Split(code, "\n") {
		if index > 0 {
			builder.NewLine()
			builder.Write(codeBlockIndent, base)
		}

		if line != "" {
			builder.Write(line, style)
		}
	}
}

// shouldMask reports whether node is a spoiler shown as "[spoiler]" in place of what it hides.
func (r *Renderer) shouldMask(node *md.Inline) bool {
	return r.cfg.Markdown.MaskSpoilers && node.Attr&md.AttrSpoiler != 0
}

// listMarker returns what goes before item: its number in an ordered list, counted from the list's start, or a dash.
func listMarker(item *ast.ListItem) string {
	list := item.Parent().(*ast.List)
	if !list.IsOrdered() {
		return "- "
	}

	number := list.Start
	for sibling := item.PreviousSibling(); sibling != nil; sibling = sibling.PreviousSibling() {
		number++
	}
	return strconv.Itoa(number) + ". "
}

func mentionText(node *md.Mention) string {
	switch {
	case node.Channel != nil:
		return "#" + node.Channel.Name
	case node.GuildUser != nil:
		name := node.GuildUser.DisplayOrUsername()
		if member := node.GuildUser.Member; member != nil && member.Nick != "" {
			name = member.Nick
		}
		return "@" + name
	case node.GuildRole != nil:
		return "@" + node.GuildRole.Name
	default:
		return ""
	}
}

func applyInlineAttr(style tview.Style, attr md.Attribute, inLink bool) tview.Style {
	if attr&md.AttrBold != 0 {
		style = style.Bold(true)
	}
	if attr&md.AttrItalics != 0 {
		style = style.Italic(true)
	}
	if attr&md.AttrUnderline != 0 {
		style = style.Underline(true)
	}
	if attr&md.AttrStrikethrough != 0 {
		style = style.StrikeThrough(true)
	}
	// Link labels like `hash` should still look like links, not highlighted blocks.
	if attr&md.AttrMonospace != 0 && !inLink {
		style = style.Reverse(true)
	}

	return style
}
