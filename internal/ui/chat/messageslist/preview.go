package messageslist

import (
	"image"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/ayn2op/arikawa/v3/discord"
	md "github.com/ayn2op/arikawa/v3/markdown"
	"github.com/ayn2op/discordo/internal/ui/emojis"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/text"
	"github.com/yuin/goldmark/v2/ast"
)

type previewSource struct {
	proxy         discord.URL
	width, height uint
	// url is the media at its full size, and name what it is called.
	url, name string
}

func (m Model) previewSources(message discord.Message) []previewSource {
	var sources []previewSource
	for _, attachment := range message.Attachments {
		proxy := attachment.Proxy
		if isVideo(attachment) {
			// A video cannot be drawn, but Discord's media proxy gives its first frame as a picture, which is shown
			// as Discord shows it, and the video plays when it is clicked.
			proxy = videoFrame(proxy)
		}
		sources = append(sources, previewSource{proxy, attachment.Width, attachment.Height, attachment.URL, attachment.Filename})
	}

	embeds := message.Embeds
	if !m.cfg.UI.MessagesList.Embeds.Enabled {
		embeds = nil
	}
	for _, embed := range embeds {
		switch {
		case embed.Image != nil:
			sources = append(sources, previewSource{embed.Image.Proxy, embed.Image.Width, embed.Image.Height, embed.Image.URL, embed.Title})
		// Image and GIF embeds carry the media itself as the thumbnail.
		case embed.Thumbnail != nil && (embed.Type == discord.ImageEmbed || embed.Type == discord.GIFVEmbed):
			proxy := embed.Thumbnail.Proxy
			// A Tenor GIF, which Discord sends as a video with a still thumbnail, is shown as the GIF it also is, so
			// that it plays.
			if gif := tenorGIF(embed); gif != "" {
				proxy = gif
			}
			sources = append(sources, previewSource{proxy, embed.Thumbnail.Width, embed.Thumbnail.Height, embedMediaURL(embed), embed.Title})
		// A video, such as a YouTube link, shows its thumbnail, and plays when it is clicked.
		case embed.Thumbnail != nil && embed.Video != nil:
			sources = append(sources, previewSource{embed.Thumbnail.Proxy, embed.Thumbnail.Width, embed.Thumbnail.Height, embedVideoURL(embed), embed.Title})
		}
	}

	// Non-image files have no dimensions.
	return slices.DeleteFunc(sources, func(source previewSource) bool { return source.width == 0 || source.height == 0 })
}

// isVideo reports whether attachment is a video.
func isVideo(attachment discord.Attachment) bool {
	if strings.HasPrefix(attachment.ContentType, "video/") {
		return true
	}
	switch strings.ToLower(path.Ext(attachment.Filename)) {
	case ".mp4", ".webm", ".mov", ".mkv", ".m4v":
		return true
	}
	return false
}

// videoFrame returns the address of the first frame of the video at proxy, on Discord's media proxy.
func videoFrame(proxy discord.URL) discord.URL {
	u, err := url.Parse(string(proxy))
	if err != nil {
		return proxy
	}
	query := u.Query()
	query.Set("format", "jpeg")
	u.RawQuery = query.Encode()
	return discord.URL(u.String())
}

// tenorGIF returns the GIF of a Tenor embed, or "". Tenor names a file of each format by its ID and a code of five
// characters: its video media.tenor.com/<id>AAAPo/<name>.mp4 is also the GIF media.tenor.com/<id>AAAAC/<name>.gif.
func tenorGIF(embed discord.Embed) discord.URL {
	if embed.Type != discord.GIFVEmbed || embed.Video == nil {
		return ""
	}
	u, err := url.Parse(embed.Video.URL)
	if err != nil || u.Host != "media.tenor.com" {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || len(parts[0]) <= 5 {
		return ""
	}
	name := strings.TrimSuffix(parts[1], path.Ext(parts[1]))
	return "https://media.tenor.com/" + parts[0][:len(parts[0])-5] + "AAAAC/" + name + ".gif"
}

// embedVideoURL returns what plays the video of embed: the page it was linked as, for a site such as YouTube, whose
// player URL is not the video, which mpv plays from the page with yt-dlp; or else the video file itself.
func embedVideoURL(embed discord.Embed) string {
	if embed.Provider != nil && embed.URL != "" {
		return embed.URL
	}
	if embed.Video != nil && embed.Video.URL != "" {
		return embed.Video.URL
	}
	return embed.URL
}

// embedMediaURL returns the media an image or GIF embed shows: its video for a GIF, which Discord sends as one, or
// else its full-size image.
func embedMediaURL(embed discord.Embed) string {
	if embed.Video != nil && embed.Video.URL != "" {
		return embed.Video.URL
	}
	if embed.Thumbnail != nil && embed.Thumbnail.URL != "" {
		return embed.Thumbnail.URL
	}
	return embed.URL
}

// LoadImages returns the command that loads the images of the shown messages, or nil.
func (m Model) LoadImages() tview.Cmd {
	msg := images.RequestedMsg{Squares: map[discord.URL]uint{}, Previews: map[discord.URL]image.Point{}}
	for index := range m.items {
		item := &m.items[index]
		if item.separator != nil {
			continue
		}

		if author := item.message.Author; m.showsAvatar(author) {
			msg.Squares[avatarURL(author)] = uint(m.cfg.UI.MessagesList.Avatars.Height)
		}

		m.addEmojis(msg.Squares, item)
		if !m.cfg.UI.MessagesList.Attachments.Preview {
			continue
		}

		for _, source := range m.previewSources(item.message) {
			msg.Previews[source.proxy] = image.Pt(int(source.width), int(source.height))
		}
	}

	if len(msg.Squares) == 0 && len(msg.Previews) == 0 {
		return nil
	}

	return cmd.Task(func() tview.Msg { return msg })
}

// addEmojis adds the custom emoji of the reactions to the item's message and in its content to squares.
func (m Model) addEmojis(squares map[discord.URL]uint, item *messageItem) {
	for _, reaction := range item.message.Reactions {
		if m.cfg.UI.MessagesList.Reactions.Enabled && reaction.Emoji.IsCustom() {
			squares[emojis.URLFromID(reaction.Emoji.ID)] = emojis.Rows
		}
	}

	root := m.contentRoot(item)
	if root == nil {
		return
	}

	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		emoji, ok := node.(*md.Emoji)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}

		if id, err := discord.ParseSnowflake(emoji.ID); err == nil {
			squares[emojis.URLFromID(discord.EmojiID(id))] = emojis.Rows
		}

		return ast.WalkContinue, nil
	})
}

// showsAvatar reports whether messages by user are drawn beside their avatar.
func (m Model) showsAvatar(user discord.User) bool {
	avatars := m.cfg.UI.MessagesList.Avatars
	return avatars.Preview && avatars.Height != 0 && !m.blocked(user)
}

func avatarURL(user discord.User) discord.URL {
	return user.AvatarURLWithType(discord.PNGImage) + "?size=128"
}

// emojiLine returns the image of the custom emoji with id as a line of spans, or nil if it is not loaded.
func (m Model) emojiLine(id string) []text.Span {
	snowflake, err := discord.ParseSnowflake(id)
	if err != nil {
		return nil
	}

	emoji, ok := m.images[emojis.URLFromID(discord.EmojiID(snowflake))]
	if !ok {
		return nil
	}

	if lines := emoji.Lines(); len(lines) > 0 {
		return lines[0]
	}

	return nil
}
