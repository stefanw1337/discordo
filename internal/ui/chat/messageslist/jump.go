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

// openMentionsMsg asks to open what the selected message's mentions of channels and links into Discord lead to, as
// clicking the message again does.
type openMentionsMsg struct{}

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

// openMentions opens what the selected message's mentions of channels and links into Discord lead to, asking which
// if there are several.
func (m Model) openMentions() tview.Cmd {
	message, ok := m.selectedMessage()
	if !ok {
		return nil
	}
	links := channelLinks(*message)
	for _, link := range m.selectedURLs() {
		if _, _, ok := discordLink(link); ok {
			links = append(links, link)
		}
	}
	switch len(links) {
	case 0:
		return nil
	case 1:
		return openURL(links[0])
	}
	return m.showAttachmentsList(links, nil, nil)
}
