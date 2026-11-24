package profilecard

import (
	"fmt"
	"strings"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
)

// activityLines returns what the user is doing, as the profile in Discord shows it: for each activity a heading,
// such as "Listening to Spotify", then what it tells, such as the song, its artists and album, and how far it is.
func activityLines(activities []discord.Activity, now time.Time) [][2]string {
	var lines [][2]string
	// The custom status comes first, whole, as the members list cuts it to its width.
	for _, a := range activities {
		if a.Type != discord.CustomActivity {
			continue
		}
		status := a.State
		if a.Emoji != nil && !a.Emoji.ID.IsValid() && a.Emoji.Name != "" {
			status = strings.TrimSpace(a.Emoji.Name + " " + status)
		}
		if status != "" {
			lines = append(lines, [2]string{"", status})
		}
	}
	for _, a := range activities {
		heading := ""
		switch a.Type {
		case discord.GameActivity:
			heading = "Playing " + a.Name
		case discord.StreamingActivity:
			heading = "Streaming " + a.Name
		case discord.ListeningActivity:
			heading = "Listening to " + a.Name
		case discord.WatchingActivity:
			heading = "Watching " + a.Name
		case discord.CompetingActivity:
			heading = "Competing in " + a.Name
		default:
			continue
		}
		lines = append(lines, [2]string{heading, ""})
		details := []string{a.Details}
		if a.Type == discord.ListeningActivity {
			// Spotify gives the artists in the state, separated by semicolons, and the album as the picture's text.
			details = append(details, by(a.State))
			if a.Assets != nil && a.Assets.LargeText != "" && a.Assets.LargeText != a.Details {
				details = append(details, "on "+a.Assets.LargeText)
			}
		} else {
			details = append(details, a.State)
		}
		for _, detail := range details {
			if detail = strings.TrimSpace(detail); detail != "" && detail != "by" {
				lines = append(lines, [2]string{"", detail})
			}
		}
		if progress := progress(a.Timestamps, now); progress != "" {
			lines = append(lines, [2]string{"", progress})
		}
	}
	return lines
}

func by(artists string) string {
	if artists == "" {
		return ""
	}
	return "by " + strings.ReplaceAll(artists, "; ", ", ")
}

// progress returns how far into a song it is, as 1:23 / 3:45, or how long something has gone on, as "for 1:02:03".
func progress(timestamps *discord.ActivityTimestamps, now time.Time) string {
	if timestamps == nil || timestamps.Start == 0 {
		return ""
	}
	start := time.UnixMilli(int64(timestamps.Start))
	elapsed := max(now.Sub(start), 0)
	if timestamps.End != 0 {
		total := time.UnixMilli(int64(timestamps.End)).Sub(start)
		return clock(min(elapsed, total)) + " / " + clock(total)
	}
	return "for " + clock(elapsed)
}

func clock(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
