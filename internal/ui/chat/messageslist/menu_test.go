package messageslist

import (
	"slices"
	"strings"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
)

// runTask returns what command, a task, sends, or fails if it is not one.
func runTask(t *testing.T, command tview.Cmd) tview.Msg {
	t.Helper()
	task, ok := command.(cmd.Task)
	if !ok {
		t.Fatalf("command = %#v, want a task", command)
	}
	return task()
}

func TestModelMenu(t *testing.T) {
	ml := newTestModel(t, testConfig(t))
	ml.setMessages([]discord.Message{{ID: 2, ChannelID: 1, Author: discord.User{ID: 5, Username: "kari"}, Content: "hei"}})
	ml.setCursor(ml.messageIndex(-1, 1))

	t.Run("lists what can be done", func(t *testing.T) {
		items, ok := runTask(t, ml.showMenu()).(ShowMenuMsg)
		if !ok {
			t.Fatal("the menu was not shown")
		}
		var labels []string
		for _, item := range items {
			labels = append(labels, item.Label)
		}
		keybinds := ml.cfg.Keybinds.MessagesList
		for _, want := range []string{keybinds.Reply.Help().Desc, keybinds.OpenProfile.Help().Desc, keybinds.YankContent.Help().Desc} {
			if !slices.Contains(labels, want) {
				t.Errorf("menu %v lacks %q", labels, want)
			}
		}
	})

	t.Run("an action does what its key does", func(t *testing.T) {
		_, command := ml.Update(ActionMsg{ml.cfg.Keybinds.MessagesList.Reply.Keybind})
		reply, ok := runTask(t, command).(ReplyMsg)
		if !ok || reply.Message.ID != 2 || reply.Mention {
			t.Fatalf("got %#v, want a reply to message 2", reply)
		}
	})

	t.Run("right-clicking a message asks for its menu", func(t *testing.T) {
		ml.clearSelection()
		area := tview.Rectangle{Width: 60, Height: 10}
		click := tview.MouseMsg{EventMouse: tcell.NewEventMouse(5, 1, tcell.Button2, tcell.ModNone), Action: tview.MouseRightClick}
		got := ml.View().Handle(click, area)
		if got != menuMsg(0) {
			t.Fatalf("got %#v, want the menu of the message", got)
		}

		var command tview.Cmd
		*ml, command = ml.Update(got)
		if selected, ok := ml.selectedMessage(); !ok || selected.ID != 2 {
			t.Fatal("the right-clicked message was not selected")
		}
		if _, ok := runTask(t, command).(ShowMenuMsg); !ok {
			t.Fatal("the menu was not shown")
		}
	})
}

func TestModelMedia(t *testing.T) {
	ml := newTestModel(t, testConfig(t))
	image := discord.Attachment{ID: 1, Filename: "clipboard.png", URL: "https://cdn.discordapp.com/attachments/1/2/clipboard.png?ex=1", Proxy: "https://media.discordapp.net/attachments/1/2/clipboard.png", ContentType: "image/png", Width: 800, Height: 600}
	message := discord.Message{ID: 2, ChannelID: 1, Author: discord.User{ID: 5}, Attachments: []discord.Attachment{image}}
	ml.setMessages([]discord.Message{message})

	t.Run("the attachment is named by its file name", func(t *testing.T) {
		var got string
		for _, span := range ml.renderMessage(message, nil, tcell.StyleDefault, true) {
			got += span.Text
		}
		if !strings.Contains(got, "clipboard.png") || strings.Contains(got, "https://") {
			t.Fatalf("message = %q, want the file name without its URL", got)
		}
	})

	t.Run("clicking the preview shows the full image", func(t *testing.T) {
		preview := clickable{Widget: text.New("preview"), msg: viewMediaMsg{url: image.URL, name: image.Filename}}
		area := tview.Rectangle{Width: 20, Height: 2}
		press := tview.MouseMsg{EventMouse: tcell.NewEventMouse(1, 1, tcell.ButtonPrimary, tcell.ModNone), Action: tview.MouseLeftDown}
		if got := preview.Handle(press, area); got != (viewMediaMsg{url: image.URL, name: image.Filename}) {
			t.Fatalf("got %#v, want the full image asked for", got)
		}
	})

	t.Run("the full-size URL is the attachment's own", func(t *testing.T) {
		sources := ml.previewSources(message)
		if len(sources) != 1 || sources[0].url != image.URL || sources[0].name != image.Filename {
			t.Fatalf("sources = %+v", sources)
		}
		if !ml.hasMedia(message) {
			t.Fatal("the image is not media to view")
		}
	})
}
