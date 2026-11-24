package composer

import (
	"context"
	"fmt"
	"github.com/ayn2op/discordo/internal/ui"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/utils/json/option"
	"github.com/ayn2op/arikawa/v3/utils/sendpart"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/autocomplete/channels"
	"github.com/ayn2op/discordo/internal/ui/autocomplete/emojis"
	"github.com/ayn2op/discordo/internal/ui/autocomplete/mentions"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ncruces/zenity"
	"golang.design/x/clipboard"
)

// RefreshAutocompleteMsg refreshes the autocomplete.
type RefreshAutocompleteMsg struct{}

// EditLastMsg asks to edit the user's last message.
type EditLastMsg struct{}

// SentMsg reports that the composer sent a message.
type SentMsg struct{}

// GIFSearchMsg asks for GIFs of Query to pick one to send, as "/gif Query" typed in the composer does.
type GIFSearchMsg struct{ Query string }

// gifQuery returns the query of a "/gif" command, and whether text is one.
func gifQuery(text string) (string, bool) {
	if text == "/gif" {
		return "", true
	}
	query, ok := strings.CutPrefix(text, "/gif ")
	return strings.TrimSpace(query), ok
}

// SendGIF sends the GIF page url, replying as the message being written would.
func (m *Model) SendGIF(url string) tview.Cmd {
	m.setText(url, len(url))
	return m.send()
}

// PostedMsg reports that the composer started Post, a thread of a forum.
type PostedMsg struct{ Post discord.Channel }

// PostFailedMsg reports that Post, whose first message is Text, failed to start, and why.
type PostFailedMsg struct {
	Post Post
	Text string
	Err  error
}

type editorMsg string

// edit is a text of the composer and the cursor in it.
type edit struct {
	text   string
	cursor int
}

// editMsg is the text and cursor that typing in the composer left.
type editMsg edit

func editLastMessage() tview.Cmd {
	return cmd.Task(func() tview.Msg { return EditLastMsg{} })
}

type imagePastedMsg []byte

// PasteImage attaches the image on the clipboard, if there is one, as when ctrl+v is pressed in a terminal that keeps
// it for itself and pastes only text, as Windows Terminal does.
//
// The key may also reach discordo, as when the terminal is set to leave it alone: an image is then pasted once.
func (m *Model) PasteImage() tview.Cmd {
	return m.pasteImageOnce()
}

// pasteImageOnce pastes the clipboard's image, unless it was pasted just now.
func (m *Model) pasteImageOnce() tview.Cmd {
	now := time.Now()
	if now.Sub(m.lastImagePaste) < pasteDedupe {
		return nil
	}
	m.lastImagePaste = now
	return pasteImage()
}

// pasteDedupe is how soon after a paste of an image another is taken to be the same key press, seen twice.
const pasteDedupe = 500 * time.Millisecond

func pasteImage() tview.Cmd {
	return cmd.Task(func() tview.Msg {
		data, err := clipboard.Read(context.Background(), clipboard.FmtImage)
		if err != nil {
			slog.Error("failed to read from clipboard", "err", err)
			return nil
		}
		return imagePastedMsg(data)
	})
}

type filesPickedMsg struct {
	channelID discord.ChannelID
	files     []sendpart.File
}

func (m Model) pickFiles() tview.Cmd {
	selectedChannel := m.channel
	if selectedChannel == nil {
		return nil
	}
	channelID := selectedChannel.ID

	return cmd.Task(func() tview.Msg {
		paths, err := zenity.SelectFileMultiple()
		if err != nil {
			slog.Error("failed to open file dialog", "err", err)
			return nil
		}

		files := openFiles(paths)
		if len(files) == 0 {
			return nil
		}

		return filesPickedMsg{channelID: channelID, files: files}
	})
}

// droppedFiles returns the files whose paths text is, as a terminal pastes the files dragged onto it, each quoted if
// it holds a space, or nil if text is anything else.
func droppedFiles(text string) []string {
	text = strings.TrimSpace(text)
	// Only text that starts as a path is looked at, for typing not to look for files at every key.
	if !looksLikePath(text) {
		return nil
	}
	if text == "" || strings.ContainsAny(text, "\r\n") {
		return nil
	}
	var paths []string
	for text != "" {
		var path string
		if quote := text[0]; quote == '"' || quote == '\'' {
			end := strings.IndexByte(text[1:], quote)
			if end < 0 {
				return nil
			}
			path, text = text[1:end+1], text[end+2:]
		} else {
			path, text, _ = strings.Cut(text, " ")
		}
		text = strings.TrimSpace(text)
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || !filepath.IsAbs(path) {
			return nil
		}
		paths = append(paths, path)
	}
	return paths
}

// looksLikePath reports whether text starts as an absolute path does, quoted or not: C:\, \\server or /.
func looksLikePath(text string) bool {
	text = strings.TrimLeft(text, "\"'")
	switch {
	case strings.HasPrefix(text, "/"), strings.HasPrefix(text, `\\`):
		return true
	case len(text) >= 3 && text[1] == ':' && (text[2] == '\\' || text[2] == '/'):
		return true
	}
	return false
}

// uploadLimit is the largest file the user may send in the channel, in bytes: Discord's limit without Nitro, unless
// the user's Nitro or the server's boosts raise it.
func (m Model) uploadLimit() int64 {
	const mb = 1024 * 1024
	limit := int64(10 * mb)
	if me, err := m.state.Cabinet.Me(); err == nil {
		switch me.Nitro {
		case discord.NitroFull:
			limit = 500 * mb
		case discord.NitroClassic, discord.NitroBasic:
			limit = 50 * mb
		}
	}
	if m.channel != nil && m.channel.GuildID.IsValid() {
		if guild, err := m.state.Cabinet.Guild(m.channel.GuildID); err == nil {
			switch {
			case guild.NitroBoost >= 3:
				limit = max(limit, 100*mb)
			case guild.NitroBoost == 2:
				limit = max(limit, 50*mb)
			}
		}
	}
	return limit
}

// attachFiles attaches files, except those larger than the user may send, which are closed and told of, as Discord
// would refuse the whole message.
func (m *Model) attachFiles(files []sendpart.File) tview.Cmd {
	limit := m.uploadLimit()
	var tooLarge []string
	for _, file := range files {
		if f, ok := file.Reader.(*os.File); ok {
			if info, err := f.Stat(); err == nil && info.Size() > limit {
				_ = f.Close()
				tooLarge = append(tooLarge, fmt.Sprintf("%s (%d MB)", file.Name, info.Size()/(1024*1024)))
				continue
			}
		}
		m.attach(file.Name, file.Reader)
	}
	if len(tooLarge) == 0 {
		return nil
	}
	err := fmt.Errorf("%s is larger than the %d MB you may send here", strings.Join(tooLarge, ", "), limit/(1024*1024))
	return cmd.Task(func() tview.Msg { return ui.ErrorDialog("attach", err) })
}

// openFiles opens the files at paths, skipping those that fail to open.
func openFiles(paths []string) []sendpart.File {
	files := make([]sendpart.File, 0, len(paths))
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			slog.Error("failed to open file", "path", path, "err", err)
			continue
		}

		files = append(files, sendpart.File{Name: filepath.Base(path), Reader: file})
	}

	return files
}

func closeFiles(files []sendpart.File) cmd.Task {
	return cmd.Task(func() tview.Msg {
		for _, file := range files {
			closeFile(file)
		}

		return nil
	})
}

func closeFile(file sendpart.File) {
	closer, ok := file.Reader.(io.Closer)
	if !ok {
		return
	}

	if err := closer.Close(); err != nil {
		slog.Error("failed to close file", "err", err)
	}
}

func (m *Model) sendTyping() tview.Cmd {
	// Nobody sees typing in a forum.
	if !m.cfg.TypingIndicator.Send || m.post != nil {
		return nil
	}

	now := time.Now()
	if now.Before(m.typingUntil) {
		return nil
	}
	m.typingUntil = now.Add(TypingDuration)

	selectedChannel := m.channel
	if selectedChannel == nil {
		return nil
	}
	channelID := selectedChannel.ID
	return cmd.Task(func() tview.Msg {
		if err := m.state.Typing(channelID); err != nil {
			slog.Error("failed to send typing indicator", "err", err)
		}
		return nil
	})
}

func (m *Model) send() tview.Cmd {
	selectedChannel := m.channel
	if selectedChannel == nil {
		return nil
	}

	text := strings.TrimSpace(m.text)
	if text == "" && len(m.sendMessageData.Files) == 0 {
		return nil
	}

	// "/gif cats" searches GIFs instead of being sent; what is replied to is kept for the GIF.
	if query, ok := gifQuery(text); ok && m.editing == nil && m.post == nil && len(m.sendMessageData.Files) == 0 {
		m.setText("", 0)
		return cmd.Task(func() tview.Msg { return GIFSearchMsg{Query: query} })
	}

	typed := text
	text = m.processText(selectedChannel, []byte(text))
	data := *m.sendMessageData
	data.Files = slices.Clone(data.Files)

	editing, post := m.editing, m.post
	m.typingUntil = time.Time{}
	m.reset()
	if post != nil {
		m.endPost()
		data.Content = text
		return m.startPost(*post, typed, data)
	}

	sent := cmd.Task(func() tview.Msg { return SentMsg{} })
	return cmd.Batch{sent, cmd.Task(func() tview.Msg {
		defer closeFiles(data.Files)()
		if editing != nil {
			m.editMessage(*editing, text)
			return nil
		}

		data.Content = text
		m.sendMessage(selectedChannel.ID, data)
		return nil
	})}
}

// startPostData is what starts a post: its title, tags and first message.
type startPostData struct {
	Name        string              `json:"name"`
	AppliedTags []discord.TagID     `json:"applied_tags,omitempty"`
	Message     api.SendMessageData `json:"message"`
}

func (data startPostData) NeedsMultipart() bool {
	return len(data.Message.Files) > 0
}

func (data startPostData) WriteMultipart(body *multipart.Writer) error {
	return sendpart.Write(body, data, data.Message.Files)
}

// startPost starts post with message as its first message, which was typed as typed. If it fails, typed is given back.
func (m Model) startPost(post Post, typed string, message api.SendMessageData) tview.Cmd {
	return cmd.Task(func() tview.Msg {
		defer closeFiles(message.Files)()
		data := startPostData{Name: post.Title, AppliedTags: post.Tags, Message: message}
		var thread discord.Channel
		url := api.EndpointChannels + post.Forum.ID.String() + "/threads"
		if err := sendpart.POST(m.state.Client.Client, data, &thread, url); err != nil {
			return PostFailedMsg{Post: post, Text: typed, Err: err}
		}
		return PostedMsg{Post: thread}
	})
}

func (m Model) editMessage(message discord.Message, text string) {
	data := api.EditMessageData{Content: option.SomeNullable(text)}
	if _, err := m.state.EditMessageComplex(message.ChannelID, message.ID, data); err != nil {
		slog.Error("failed to edit message", "err", err)
	}
}

func (m Model) sendMessage(channelID discord.ChannelID, data api.SendMessageData) {
	if _, err := m.state.SendMessageComplex(channelID, data); err != nil {
		slog.Error("failed to send message in channel", "channel_id", channelID, "err", err)
	}
}

func (m *Model) complete() tview.Cmd {
	if item, ok := m.autocomplete.Selected(); ok {
		_, name := m.query()
		m.replace(m.cursor-len(name)-1, m.cursor, item.Insert()+" ")
	}

	return m.showCompletions("", nil)
}

func (m *Model) showAutocomplete() tview.Cmd {
	trigger, name := m.query()
	if trigger == 0 || m.channel == nil {
		return m.showCompletions("", nil)
	}

	guildID, limit := m.channel.GuildID, int(m.cfg.UI.Autocomplete.Limit)
	if trigger == '@' {
		items, loadImages := mentions.Items(m.cfg, m.state, m.channel, name, limit)
		return cmd.Batch{m.showCompletions(mentions.Title, items), loadImages, m.searchMember(guildID, name)}
	}
	if trigger == '#' {
		return m.showCompletions(channels.Title, channels.Items(m.state, m.channel, name, limit))
	}

	items, loadImages := emojis.Items(m.cfg, m.state, guildID, name, limit)
	return cmd.Batch{m.showCompletions(emojis.Title, items), loadImages}
}

// searchMember asks Discord for members matching name; they arrive later as a gateway event.
func (m *Model) searchMember(guildID discord.GuildID, name string) tview.Cmd {
	if name == "" || !guildID.IsValid() {
		return nil
	}

	key := guildID.String() + " " + name
	if _, ok := m.memberSearchCache[key]; ok {
		return nil
	}
	// If searching for "ab" returns less than SearchLimit, then "abc" would not return anything new because we already searched everything starting with "ab".
	// This will still be true even if a new member joins because arikawa loads new members into the state.
	if count, ok := m.memberSearchCache[key[:len(key)-1]]; ok && count < m.state.MemberState.SearchLimit {
		m.memberSearchCache[key] = count
		return nil
	}

	now := time.Now()
	// Rate limit on our side because we can't distinguish between a successful search and SearchMember not doing anything because of its internal rate limit that we can't detect
	if m.lastSearch.Add(m.state.MemberState.SearchFrequency).After(now) {
		return nil
	}

	m.lastSearch = now
	nonce := memberSearchNonce + key
	return cmd.Task(func() tview.Msg {
		if err := m.state.SendGateway(context.Background(), &gateway.RequestGuildMembersCommand{
			GuildIDs:  []discord.GuildID{guildID},
			Query:     option.Some(name),
			Presences: m.state.MemberState.RequestPresences,
			Limit:     m.state.MemberState.SearchLimit,
			Nonce:     nonce,
		}); err != nil {
			slog.Error("failed to search guild members", "err", err, "guild_id", guildID, "query", name)
		}
		return nil
	})
}

func (m Model) openEditor() tview.Cmd {
	if m.cfg.Editor == "" {
		return cmd.Task(func() tview.Msg {
			slog.Warn("Attempt to open file with editor, but no editor is set")
			return nil
		})
	}
	text, cfg := m.text, m.cfg
	return cmd.Suspend(func() tview.Msg {
		return runEditor(cfg, text)
	})
}

// runEditor opens text in the editor and returns what it was edited to.
func runEditor(cfg *config.Config, text string) tview.Msg {
	file, err := os.CreateTemp("", tmpFilePattern)
	if err != nil {
		slog.Error("failed to create tmp file", "err", err)
		return nil
	}

	name := file.Name()
	defer func() {
		if err := os.Remove(name); err != nil {
			slog.Error("failed to remove tmp file", "err", err)
		}
	}()

	_, _ = file.WriteString(text)
	_ = file.Close()

	cmd, err := cfg.EditorCommand(name)
	if err != nil {
		slog.Error("failed to create editor command", "err", err)
		return nil
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		slog.Error("failed to run command", "args", cmd.Args, "err", err)
		return nil
	}

	edited, err := os.ReadFile(name)
	if err != nil {
		slog.Error("failed to read tmp file", "name", name, "err", err)
		return nil
	}

	return editorMsg(strings.TrimSpace(string(edited)))
}
