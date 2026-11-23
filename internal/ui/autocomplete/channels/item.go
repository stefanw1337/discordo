// Package channels completes mentions of channels, as #name.
package channels

import (
	"slices"
	"strings"
	"unicode"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/ui/autocomplete"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
	"github.com/sahilm/fuzzy"
)

// Title is the autocomplete's title for channels.
const Title = "Channels"

type item struct {
	name, category string
}

func (i item) Insert() string { return "#" + i.name }

func (i item) Line(map[discord.URL]images.Widget) []text.Span {
	line := []text.Span{{Text: "# " + i.name}}
	if i.category != "" {
		line = append(line, text.Span{Text: "  " + i.category, Style: tcell.StyleDefault.Dim(true)})
	}
	return line
}

// Mentionable returns the channels of guildID that can be mentioned and that the user can see, in their order.
func Mentionable(s *ningen.State, guildID discord.GuildID) []discord.Channel {
	if !guildID.IsValid() {
		return nil
	}
	all, err := s.Cabinet.Channels(guildID)
	if err != nil {
		return nil
	}
	channels := make([]discord.Channel, 0, len(all))
	for _, channel := range all {
		if channel.Type == discord.GuildCategory || channel.Name == "" {
			continue
		}
		if !s.HasPermissions(channel.ID, discord.PermissionViewChannel) {
			continue
		}
		channels = append(channels, channel)
	}
	slices.SortStableFunc(channels, func(a, b discord.Channel) int { return a.Position - b.Position })
	return channels
}

// Items returns up to limit channels of the guild of channel matching query, best first, or the first of them if
// query is empty.
func Items(s *ningen.State, channel *discord.Channel, query string, limit int) []autocomplete.Item {
	channels := Mentionable(s, channel.GuildID)
	names := make([]string, len(channels))
	for i, c := range channels {
		names[i] = c.Name
	}

	matches := fuzzy.Find(query, names)
	if query == "" {
		for i, name := range names {
			matches = append(matches, fuzzy.Match{Str: name, Index: i})
		}
	}

	items := make([]autocomplete.Item, 0, min(limit, len(matches)))
	for _, match := range matches {
		if len(items) == limit {
			break
		}
		c := channels[match.Index]
		category := ""
		if c.ParentID.IsValid() {
			if parent, err := s.Cabinet.Channel(c.ParentID); err == nil {
				category = parent.Name
			}
		}
		items = append(items, item{name: c.Name, category: category})
	}
	return items
}

// Find returns the channel of guildID called name, which mentions as #name, ignoring case, or nil. A name typed
// without the emoji and separator many servers put before their channels' names, as sc-news for 📰|sc-news, finds
// the one channel so named.
func Find(s *ningen.State, guildID discord.GuildID, name string) *discord.Channel {
	channels := Mentionable(s, guildID)
	for _, channel := range channels {
		if strings.EqualFold(channel.Name, name) {
			return &channel
		}
	}
	var found *discord.Channel
	for _, channel := range channels {
		if strings.EqualFold(plainName(channel.Name), name) {
			if found != nil {
				return nil
			}
			found = &channel
		}
	}
	return found
}

// plainName returns name without what comes before its first letter or digit, such as an emoji and a separator.
func plainName(name string) string {
	return strings.TrimLeftFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
