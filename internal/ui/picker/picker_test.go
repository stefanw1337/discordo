package picker

import (
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
)

type picked struct{ ref any }

func click(x, y int) tview.MouseMsg {
	return tview.MouseMsg{EventMouse: tcell.NewEventMouse(x, y, tcell.ButtonPrimary, tcell.ModNone), Action: tview.MouseLeftDown}
}

func TestClickSelectsThenPicks(t *testing.T) {
	cfg := config.Default()
	m := NewModel(&cfg, "Test")
	m.SetItems(picker.Items{
		{Content: text.New("one"), FilterText: "one", Reference: 1},
		{Content: text.New("two"), FilterText: "two", Reference: 2},
		{Content: text.New("three"), FilterText: "three", Reference: 3},
	})
	area := tview.Rectangle{X: 0, Y: 0, Width: 40, Height: 12}
	view := func() tview.Widget {
		return m.View(func(item picker.Item) tview.Msg { return picked{item.Reference} }, nil)
	}

	// The row of "two" is the first row whose click selects anything other than the first item.
	row := -1
	for y := range area.Height {
		if change, ok := view().Handle(click(5, y), area).(changeMsg); ok {
			state := picker.SearchState(change)
			if item, _ := state.Selected(m.items); item.Reference == 2 {
				row = y
				m, _ = m.Update(change)
				break
			}
		}
	}
	if row < 0 {
		t.Fatal("no row selects the second item")
	}
	if got, ok := view().Handle(click(5, row), area).(picked); !ok || got.ref != 2 {
		t.Fatalf("second click = %#v, want the second item picked", got)
	}
	// A click on another row selects it without picking.
	if _, ok := view().Handle(click(5, row+1), area).(picked); ok {
		t.Fatal("a click on another row picked it at once")
	}
}
