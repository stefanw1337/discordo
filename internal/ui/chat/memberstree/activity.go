package memberstree

import (
	"github.com/ayn2op/arikawa/v3/discord"
)

// activityText returns what Discord shows under a member's name: their custom status, else what they are doing,
// as "Playing Star Citizen", or "".
func activityText(activities []discord.Activity) string {
	for _, activity := range activities {
		if activity.Type != discord.CustomActivity {
			continue
		}
		text := activity.State
		if activity.Emoji != nil && !activity.Emoji.ID.IsValid() && activity.Emoji.Name != "" {
			// A custom emoji cannot be drawn in a terminal; a Unicode one is a character.
			text = activity.Emoji.Name + " " + text
		}
		if text != "" {
			return text
		}
	}
	for _, activity := range activities {
		switch activity.Type {
		case discord.GameActivity:
			return "Playing " + activity.Name
		case discord.StreamingActivity:
			if activity.Details != "" {
				return "Streaming " + activity.Details
			}
			return "Streaming " + activity.Name
		case discord.ListeningActivity:
			return "Listening to " + activity.Name
		case discord.WatchingActivity:
			return "Watching " + activity.Name
		case discord.CompetingActivity:
			return "Competing in " + activity.Name
		}
	}
	return ""
}
