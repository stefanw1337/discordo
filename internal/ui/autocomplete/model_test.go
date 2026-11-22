package autocomplete

import (
	"github.com/ayn2op/discordo/internal/ui/images"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/tview/text"
)

// item is an item drawn as its text.
type item string

func (i item) Insert() string { return string(i) }

func (i item) Line(map[discord.URL]images.Widget) []text.Span {
	return []text.Span{{Text: string(i)}}
}

func TestNewModel(t *testing.T) {
	t.Run("shows no items", func(t *testing.T) {
		if m := NewModel(nil); m.Visible() {
			t.Fatal("a new list shows items")
		}
	})
	t.Run("selects nothing", func(t *testing.T) {
		if _, ok := NewModel(nil).Selected(); ok {
			t.Fatal("a new list selected an item")
		}
	})
}

func TestModelUpdate(t *testing.T) {
	m := NewModel(nil)
	t.Run("items select the first", func(t *testing.T) {
		m, _ = m.Update(SetItemsMsg{"Emojis", []Item{item("😀"), item("😃")}})
		if item, ok := m.Selected(); !ok || item.Insert() != "😀" {
			t.Errorf("selected %v, %v, want the first item", item, ok)
		}
	})
	t.Run("no items hide the list", func(t *testing.T) {
		if m, _ = m.Update(SetItemsMsg{}); m.Visible() {
			t.Error("a hidden list shows items")
		}
	})
}
