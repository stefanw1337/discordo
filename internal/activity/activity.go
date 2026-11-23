// Package activity finds which of the programs the user lists is open, to show it as the user's activity, as
// Discord does for games. It looks only at programs that have a window, and only when asked to.
package activity

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
)

// Interval is how often the open windows are looked at.
const Interval = 15 * time.Second

// Window is a window that is open: the name of its program's file and its title.
type Window struct {
	Exe   string
	Title string
}

// Types are the kinds of activity, as they are written in the config, in the order the programs page cycles them.
var Types = []string{"playing", "listening", "watching", "competing"}

// Find returns the first of programs that has a window open, and that window's title.
func Find(programs []config.Program, windows []Window) (config.Program, string, bool) {
	for _, program := range programs {
		for _, window := range windows {
			if strings.EqualFold(window.Exe, program.Exe) {
				return program, window.Title, true
			}
		}
	}
	return config.Program{}, "", false
}

// Exes returns the names of the programs that have a window open, each once, in the order they were found.
func Exes(windows []Window) []string {
	var exes []string
	seen := map[string]bool{}
	for _, window := range windows {
		key := strings.ToLower(window.Exe)
		if window.Exe == "" || seen[key] {
			continue
		}
		seen[key] = true
		exes = append(exes, window.Exe)
	}
	return exes
}

// Name returns a name to show for the program with the file exe until it is renamed: its name without the extension.
func Name(exe string) string {
	return strings.TrimSuffix(exe, filepath.Ext(exe))
}

// New returns the activity of program, open since start, whose window has the title title.
func New(program config.Program, title string, start time.Time) discord.Activity {
	name := program.Name
	if name == "" {
		name = Name(program.Exe)
	}
	activity := discord.Activity{
		Name:       name,
		Type:       activityType(program.Type),
		CreatedAt:  discord.UnixMsTimestamp(start.UnixMilli()),
		Timestamps: &discord.ActivityTimestamps{Start: discord.UnixMsTimestamp(start.UnixMilli())},
	}
	if program.ShowTitle {
		// A title that only repeats the name, as a program's main window often has, says nothing more.
		if title = strings.TrimSpace(title); title != "" && !strings.EqualFold(title, name) {
			activity.Details = truncate(title, 128)
		}
	}
	return activity
}

func activityType(name string) discord.ActivityType {
	switch name {
	case "listening":
		return discord.ListeningActivity
	case "watching":
		return discord.WatchingActivity
	case "competing":
		return discord.CompetingActivity
	}
	return discord.GameActivity
}

// truncate cuts s to at most n runes, as Discord refuses longer details.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
