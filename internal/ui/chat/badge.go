package chat

import (
	"time"

	"github.com/ayn2op/discordo/internal/winterm"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
)

// badgeInterval is how often the taskbar's badge is brought up to date.
const badgeInterval = 2 * time.Second

// badgeTickMsg brings the taskbar's badge up to date, and asks for the next.
type badgeTickMsg struct{}

// updateBadge shows the unread mentions on the taskbar's button of the terminal, unless badges are turned off, and
// does it again in a while.
func (m *Model) updateBadge() tview.Cmd {
	mentions := 0
	if m.cfg.UI.Badges {
		mentions = m.guildsTree.Mentions()
	}
	if mentions != m.badgeMentions {
		m.badgeMentions = mentions
		winterm.SetBadge(winterm.Badge{Mentions: mentions})
	}
	return cmd.Task(func() tview.Msg {
		time.Sleep(badgeInterval)
		return badgeTickMsg{}
	})
}
