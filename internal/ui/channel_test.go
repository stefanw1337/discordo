package ui

import (
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
)

func TestSortGuildChannelsVoiceLast(t *testing.T) {
	channels := []discord.Channel{
		{ID: 1, Type: discord.GuildVoice, Position: 0},
		{ID: 2, Type: discord.GuildText, Position: 2},
		{ID: 3, Type: discord.GuildStageVoice, Position: 1},
		{ID: 4, Type: discord.GuildText, Position: 1},
	}
	SortGuildChannels(channels)
	want := []discord.ChannelID{4, 2, 1, 3}
	for i, channel := range channels {
		if channel.ID != want[i] {
			t.Fatalf("order = %v, want %v", ids(channels), want)
		}
	}
}

func ids(channels []discord.Channel) []discord.ChannelID {
	out := make([]discord.ChannelID, len(channels))
	for i, channel := range channels {
		out[i] = channel.ID
	}
	return out
}
