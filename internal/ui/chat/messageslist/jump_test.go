package messageslist

import (
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func TestDiscordLink(t *testing.T) {
	for _, test := range []struct {
		link      string
		channelID discord.ChannelID
		messageID discord.MessageID
		ok        bool
	}{
		{"https://discord.com/channels/1/2/3", 2, 3, true},
		{"https://ptb.discord.com/channels/@me/2", 2, 0, true},
		{"https://canary.discord.com/channels/1/2", 2, 0, true},
		{"https://example.com/channels/1/2/3", 0, 0, false},
		{"https://discord.com/invite/abc", 0, 0, false},
	} {
		channelID, messageID, ok := discordLink(test.link)
		if channelID != test.channelID || messageID != test.messageID || ok != test.ok {
			t.Errorf("discordLink(%q) = %v %v %v", test.link, channelID, messageID, ok)
		}
	}
}

func TestChannelLinksJump(t *testing.T) {
	links := channelLinks(discord.Message{GuildID: 1, Content: "see <#5> and <#6>, and <#5> again"})
	if len(links) != 2 || links[0] != "https://discord.com/channels/1/5" {
		t.Fatalf("links = %v", links)
	}
	jump, ok := openURL(links[1]).(cmd.Task)
	if !ok {
		t.Fatal("a link into Discord is not opened in discordo")
	}
	if got := jump(); got != (JumpMsg{ChannelID: 6}) {
		t.Fatalf("opening it = %#v, want a jump to channel 6", got)
	}
}

func TestLinkClick(t *testing.T) {
	screen, err := tcell.NewTerminfoScreenFromTty(vt.NewMockTerm(vt.MockOptSize{X: 20, Y: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)

	links := &linkCells{urls: map[[2]int]string{}}
	content := text.Rich(text.Span{Text: "see "}, text.Span{Text: "here", Style: tcell.StyleDefault.Url("https://example.com")})
	w := linkClicks{Widget: content, links: links}
	area := tview.Rectangle{Width: 20, Height: 2}
	w.Draw(screen, area)

	click := func(x int) tview.Msg {
		return w.Handle(tview.MouseMsg{EventMouse: tcell.NewEventMouse(x, 0, tcell.ButtonPrimary, tcell.ModNone), Action: tview.MouseLeftClick}, area)
	}
	if got := click(5); got != openLinkMsg("https://example.com") {
		t.Fatalf("clicking the link = %#v", got)
	}
	if _, ok := click(1).(openLinkMsg); ok {
		t.Fatal("clicking plain text opened a link")
	}
}
