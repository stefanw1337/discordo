package chat

import (
	"testing"

	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/spacer"
	"github.com/gdamore/tcell/v3"
)

func press(action tview.MouseAction, x int) tview.MouseMsg {
	return tview.MouseMsg{EventMouse: tcell.NewEventMouse(x, 5, tcell.ButtonPrimary, tcell.ModNone), Action: action}
}

func TestSplitter(t *testing.T) {
	area := tview.Rectangle{Width: 200, Height: 50}
	s := splitter{Widget: spacer.New(), percent: 20}

	// The server list is 40 wide: its right border is column 39.
	if got := s.Handle(press(tview.MouseLeftDown, 39), area); got != (sidebarDragMsg{start: true, percent: 20}) {
		t.Fatalf("pressing the edge = %#v, want the drag to start", got)
	}
	if got := s.Handle(press(tview.MouseLeftDown, 100), area); got == (sidebarDragMsg{start: true, percent: 20}) {
		t.Fatal("pressing elsewhere started a drag")
	}

	s.dragging = true
	if got := s.Handle(press(tview.MouseMove, 59), area); got != (sidebarDragMsg{percent: 30}) {
		t.Fatalf("dragging to column 59 = %#v, want 30%%", got)
	}
	if got := s.Handle(press(tview.MouseLeftUp, 199), area); got != (sidebarDragMsg{end: true, percent: maxSidebarPercent}) {
		t.Fatalf("letting go at the far right = %#v, want the most there is", got)
	}
}
