package messageslist

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
)

// JumpMsg asks to open ChannelID, at MessageID if it is valid: what a mention of a channel, a thread or a post of a
// forum, or a link to a message on discord.com, leads to.
type JumpMsg struct {
	ChannelID discord.ChannelID
	MessageID discord.MessageID
}

var channelMentionRegex = regexp.MustCompile(`<#(\d+)>`)

// discordLink returns the channel and message, if any, that a link into Discord, such as
// https://discord.com/channels/<guild>/<channel>/<message>, leads to.
func discordLink(link string) (discord.ChannelID, discord.MessageID, bool) {
	u, err := url.Parse(link)
	if err != nil {
		return 0, 0, false
	}
	switch strings.TrimPrefix(strings.TrimPrefix(u.Host, "www."), "ptb.") {
	case "discord.com", "canary.discord.com", "discordapp.com":
	default:
		return 0, 0, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "channels" {
		return 0, 0, false
	}
	channelID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	var messageID uint64
	if len(parts) >= 4 {
		messageID, _ = strconv.ParseUint(parts[3], 10, 64)
	}
	return discord.ChannelID(channelID), discord.MessageID(messageID), true
}

// channelLinks returns links into Discord for the channels message mentions, in the order it mentions them.
func channelLinks(message discord.Message) []string {
	guild := "@me"
	if message.GuildID.IsValid() {
		guild = message.GuildID.String()
	}
	var links []string
	seen := map[string]bool{}
	for _, match := range channelMentionRegex.FindAllStringSubmatch(message.Content, -1) {
		if !seen[match[1]] {
			seen[match[1]] = true
			links = append(links, "https://discord.com/channels/"+guild+"/"+match[1])
		}
	}
	return links
}

// jumpTo returns the command that opens where link leads into Discord.
func jumpTo(channelID discord.ChannelID, messageID discord.MessageID) tview.Cmd {
	return cmd.Task(func() tview.Msg { return JumpMsg{ChannelID: channelID, MessageID: messageID} })
}

// linkCells are the cells of the messages drawn last that are part of a link, by their place on the screen, so that a
// click on a link opens it.
type linkCells struct {
	urls map[[2]int]string
}

// linkClicks draws the messages, noting where their links are, and turns a click on one into opening it.
type linkClicks struct {
	tview.Widget
	links *linkCells
}

func (w linkClicks) Draw(screen tview.Screen, area tview.Rectangle) {
	w.Widget.Draw(screen, area)
	if w.links == nil {
		return
	}
	clear(w.links.urls)
	for y := area.Y; y < area.Y+area.Height; y++ {
		for x := area.X; x < area.X+area.Width; x++ {
			if _, style, _ := screen.Get(x, y); style != (tview.Style{}) {
				if _, url := style.GetUrl(); url != "" {
					w.links.urls[[2]int{x, y}] = url
				}
			}
		}
	}
}

// openLinkMsg asks to open the link clicked.
type openLinkMsg string

func (w linkClicks) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	if mouse, ok := msg.(tview.MouseMsg); ok && w.links != nil && mouse.Action == tview.MouseLeftClick && area.Contains(mouse.Position()) {
		x, y := mouse.Position()
		if url, ok := w.links.urls[[2]int{x, y}]; ok {
			return openLinkMsg(url)
		}
	}
	return w.Widget.Handle(msg, area)
}
