package chat

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/discordo/internal/activity"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
)

// sharedActivity is what the user is shown doing, from the programs they list: which program, since when, and the
// activity last sent, to send only changes.
type sharedActivity struct {
	// generation is raised whenever the windows should be looked at anew, so that the looks scheduled before stop.
	generation int
	exe        string
	start      time.Time
	sent       *discord.Activity
}

// activityScannedMsg is the windows found open by the look of generation.
type activityScannedMsg struct {
	generation int
	windows    []activity.Window
}

// scanActivity looks at the windows that are open after delay, if sharing is enabled, and stops the looks scheduled
// before.
func (m *Model) scanActivity(delay time.Duration) tview.Cmd {
	m.activity.generation++
	if !m.cfg.Activity.Enabled {
		// Others stop seeing the program at once when sharing is turned off.
		return m.sendActivity(nil)
	}
	generation := m.activity.generation
	return cmd.Task(func() tview.Msg {
		time.Sleep(delay)
		return activityScannedMsg{generation: generation, windows: activity.Windows()}
	})
}

func (m *Model) activityScanned(msg activityScannedMsg) tview.Cmd {
	if msg.generation != m.activity.generation || !m.cfg.Activity.Enabled {
		return nil
	}
	var shown *discord.Activity
	if program, title, ok := activity.Find(m.cfg.Activity.Programs, msg.windows); ok {
		if program.Exe != m.activity.exe {
			m.activity.exe, m.activity.start = program.Exe, time.Now()
		}
		a := activity.New(program, title, m.activity.start)
		shown = &a
	} else {
		m.activity.exe = ""
	}
	next := m.scanActivity(activity.Interval)
	return cmd.Batch{m.sendActivity(shown), next}
}

// sendActivity shows others the user doing a, or nothing if a is nil, unless that is what they see already.
func (m *Model) sendActivity(a *discord.Activity) tview.Cmd {
	if sameActivity(m.activity.sent, a) {
		return nil
	}
	m.activity.sent = a
	presence := gateway.UpdatePresenceCommand{Status: m.presenceStatus()}
	presence.Activities = m.customStatus()
	if a != nil {
		presence.Activities = append(presence.Activities, *a)
	}
	state := m.state
	return cmd.Task(func() tview.Msg {
		if err := state.SendGateway(context.Background(), &presence); err != nil {
			slog.Error("failed to send activity", "err", err)
		}
		return nil
	})
}

func sameActivity(a, b *discord.Activity) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name && a.Type == b.Type && a.Details == b.Details && a.CreatedAt == b.CreatedAt
}

// presenceStatus is the status to send with an activity, so that sending one does not change it: the status of the
// config, or the one the user has.
func (m *Model) presenceStatus() discord.Status {
	if status := m.cfg.Status; status != "" && status != discord.UnknownStatus {
		return status
	}
	me, _ := m.state.Cabinet.Me()
	if me == nil {
		return discord.OnlineStatus
	}
	presence, _ := m.state.PresenceStore.Presence(0, me.ID)
	switch {
	case presence == nil || presence.Status == "" || presence.Status == discord.UnknownStatus:
		return discord.OnlineStatus
	case presence.Status == discord.OfflineStatus:
		// Connected and seen offline means invisible, which must stay so.
		return discord.InvisibleStatus
	}
	return presence.Status
}

// customStatus is the custom status the user has, which is sent with an activity so that it is kept.
func (m *Model) customStatus() []discord.Activity {
	me, _ := m.state.Cabinet.Me()
	if me == nil {
		return nil
	}
	presence, _ := m.state.PresenceStore.Presence(0, me.ID)
	if presence == nil {
		return nil
	}
	var custom []discord.Activity
	for _, a := range presence.Activities {
		if a.Type == discord.CustomActivity {
			custom = append(custom, a)
		}
	}
	return slices.Clip(custom)
}
