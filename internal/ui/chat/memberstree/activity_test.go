package memberstree

import (
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
)

func TestActivityText(t *testing.T) {
	for _, test := range []struct {
		name       string
		activities []discord.Activity
		want       string
	}{
		{"nothing", nil, ""},
		{"game", []discord.Activity{{Type: discord.GameActivity, Name: "Star Citizen"}}, "Playing Star Citizen"},
		{"music", []discord.Activity{{Type: discord.ListeningActivity, Name: "Spotify"}}, "Listening to Spotify"},
		{"custom status first", []discord.Activity{
			{Type: discord.GameActivity, Name: "Star Citizen"},
			{Type: discord.CustomActivity, State: "afk", Emoji: &discord.Emoji{Name: "💤"}},
		}, "💤 afk"},
		{"custom emoji left out", []discord.Activity{{Type: discord.CustomActivity, State: "hi", Emoji: &discord.Emoji{ID: 5, Name: "pepe"}}}, "hi"},
		{"empty custom status", []discord.Activity{{Type: discord.CustomActivity}, {Type: discord.WatchingActivity, Name: "YouTube"}}, "Watching YouTube"},
	} {
		if got := activityText(test.activities); got != test.want {
			t.Errorf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}
