// Package mentionsinbox lists the messages that recently mentioned the user, across every server and DM, and jumps to the one picked.
package mentionsinbox

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	uipicker "github.com/ayn2op/discordo/internal/ui/picker"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
)

// limit is how many mentions are asked for, the most Discord's inbox shows at once.
const limit = 25

type Model struct {
	uipicker.Model
	cfg   *config.Config
	state *ningen.State
	// request numbers the loads, so that the answer to one replaced by another is dropped.
	request int
}

func NewModel(cfg *config.Config, state *ningen.State) Model {
	return Model{Model: uipicker.NewModel(cfg, "Mentions"), cfg: cfg, state: state}
}

// Open shows that the mentions are loading and asks Discord for them.
func (m *Model) Open() tview.Cmd {
	m.request++
	m.SetItems(picker.Items{notice("Loading...")})
	request, client := m.request, m.state.Client
	return cmd.Task(func() tview.Msg {
		messages, err := fetch(client)
		return loadedMsg{request: request, messages: messages, err: err}
	})
}

// fetch asks Discord for the newest messages that mentioned the user, their roles, @everyone or @here.
func fetch(client *api.Client) ([]discord.Message, error) {
	endpoint := api.EndpointMe + "/mentions?limit=" + strconv.Itoa(limit) + "&roles=true&everyone=true"
	return client.RequestJSON[[]discord.Message]("GET", endpoint)
}

type loadedMsg struct {
	request  int
	messages []discord.Message
	err      error
}

// SelectedMsg asks to show the message with MessageID in ChannelID.
type SelectedMsg struct {
	ChannelID discord.ChannelID
	MessageID discord.MessageID
}

type CancelMsg struct{}

var _ tview.Model[Model] = Model{}

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	if msg, ok := msg.(loadedMsg); ok {
		if msg.request != m.request {
			return m, nil
		}

		switch {
		case msg.err != nil:
			slog.Error("failed to load mentions", "err", msg.err)
			m.SetItems(picker.Items{notice("Could not load mentions: " + msg.err.Error())})
		case len(msg.messages) == 0:
			m.SetItems(picker.Items{notice("No recent mentions")})
		default:
			items := make(picker.Items, len(msg.messages))
			for i, message := range msg.messages {
				items[i] = m.item(message)
			}
			m.SetItems(items)
		}
		return m, nil
	}

	var command tview.Cmd
	m.Model, command = m.Model.Update(msg)
	return m, command
}

// notice returns an item that only says text, and does nothing when picked.
func notice(s string) picker.Item {
	return picker.Item{Content: text.New(s).Style(tcell.StyleDefault.Dim(true))}
}

// item returns message as an item: when, who, where and what, found by any of them.
func (m Model) item(message discord.Message) picker.Item {
	where := m.where(message)
	content := strings.Join(strings.Fields(message.Content), " ")
	if content == "" && len(message.Attachments) != 0 {
		content = "[" + message.Attachments[0].Filename + "]"
	}

	dim := tcell.StyleDefault.Dim(true)
	spans := []text.Span{
		{Text: message.Timestamp.Time().Local().Format("02 Jan 15:04") + " ", Style: dim},
		{Text: message.Author.DisplayOrUsername(), Style: tcell.StyleDefault.Bold(true)},
		{Text: " in " + where + ": ", Style: dim},
		{Text: content, Style: tcell.StyleDefault},
	}
	return picker.Item{
		Content:    text.Rich(spans...).Wrapping(text.WrappingNone),
		FilterText: message.Author.Username + " " + message.Author.DisplayOrUsername() + " " + where + " " + content,
		Reference:  SelectedMsg{ChannelID: message.ChannelID, MessageID: message.ID},
	}
}

// where returns the channel and guild of message, as far as the state knows them.
func (m Model) where(message discord.Message) string {
	channel, err := m.state.Cabinet.Channel(message.ChannelID)
	if err != nil {
		return "a channel you cannot see"
	}

	where := ui.ChannelToString(*channel, m.cfg.Icons, m.state)
	if guild, err := m.state.Cabinet.Guild(channel.GuildID); err == nil {
		where += " (" + guild.Name + ")"
	}
	return where
}

// View shows the picker in a box titled Mentions.
func (m Model) View() tview.Widget {
	return m.Model.View(func(item picker.Item) tview.Msg {
		selected, ok := item.Reference.(SelectedMsg)
		if !ok {
			return nil
		}
		return selected
	}, CancelMsg{})
}
