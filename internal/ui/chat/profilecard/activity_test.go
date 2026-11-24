package profilecard

import (
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
)

func TestActivityLines(t *testing.T) {
	start := time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC)
	song := discord.Activity{
		Type: discord.ListeningActivity, Name: "Spotify", Details: "ODD YOUTH", State: "A; B",
		Assets:     &discord.ActivityAssets{LargeText: "The Album"},
		Timestamps: &discord.ActivityTimestamps{Start: discord.UnixMsTimestamp(start.UnixMilli()), End: discord.UnixMsTimestamp(start.Add(225 * time.Second).UnixMilli())},
	}
	lines := activityLines([]discord.Activity{{Type: discord.CustomActivity, State: "hi"}, song}, start.Add(83*time.Second))
	want := [][2]string{{"Listening to Spotify", ""}, {"", "ODD YOUTH"}, {"", "by A, B"}, {"", "on The Album"}, {"", "1:23 / 3:45"}}
	if len(lines) != len(want) {
		t.Fatalf("lines = %v", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %v, want %v", i, lines[i], want[i])
		}
	}
}
