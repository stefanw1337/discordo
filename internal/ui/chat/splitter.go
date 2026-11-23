package chat

import (
	"log/slog"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
)

// Sidebar widths, in percent of the screen, that dragging its edge and the keys keep it within.
const (
	minSidebarPercent = 5
	maxSidebarPercent = 60
	sidebarStep       = 2
)

// sidebarDragMsg is a drag of the right edge of the server list: its start, where it is, or its end.
type sidebarDragMsg struct {
	start, end bool
	percent    int
}

// splitter wraps the panes, and turns a press on the right edge of the server list, and what the mouse does until
// it is let go, into a drag that sets its width.
type splitter struct {
	tview.Widget
	// percent is the width of the server list, and dragging whether its edge is being dragged.
	percent  int
	dragging bool
}

func (s splitter) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	mouse, ok := msg.(tview.MouseMsg)
	if !ok || area.Width <= 0 {
		return s.Widget.Handle(msg, area)
	}
	x, _ := mouse.Position()
	percent := max(min((x-area.X+1)*100/area.Width, maxSidebarPercent), minSidebarPercent)
	if s.dragging {
		switch mouse.Action {
		case tview.MouseLeftUp:
			return sidebarDragMsg{end: true, percent: percent}
		case tview.MouseMove, tview.MouseLeftDown:
			return sidebarDragMsg{percent: percent}
		}
		return nil
	}
	// The edge is the right border of the server list's box.
	edge := area.X + area.Width*s.percent/100 - 1
	if mouse.Action == tview.MouseLeftDown && (x == edge || x == edge+1) {
		return sidebarDragMsg{start: true, percent: s.percent}
	}
	return s.Widget.Handle(msg, area)
}

// dragSidebar handles a drag of the server list's edge, saving the width it is left at.
func (m *Model) dragSidebar(msg sidebarDragMsg) {
	switch {
	case msg.start:
		m.draggingSidebar = true
	case msg.end:
		m.draggingSidebar = false
		m.setSidebarWidth(msg.percent)
	default:
		m.cfg.Sidebar.WidthPercent = msg.percent
	}
}

// setSidebarWidth sets the width of the server list, in percent, and saves it.
func (m *Model) setSidebarWidth(percent int) {
	m.cfg.Sidebar.WidthPercent = max(min(percent, maxSidebarPercent), minSidebarPercent)
	if err := config.SetValue(m.cfg.Path, []string{"sidebar", "width_percent"}, m.cfg.Sidebar.WidthPercent); err != nil {
		slog.Error("failed to save the width of the server list", "err", err)
	}
}
