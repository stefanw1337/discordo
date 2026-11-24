// Package picker holds what discordo's pickers share.
package picker

import (
	"reflect"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/scrollbar"
)

// Model is a picker of items shown in a box titled title.
type Model struct {
	items       picker.Items
	searchState picker.SearchState
	cfg         *config.Config
	title       string
}

func NewModel(cfg *config.Config, title string) Model {
	return Model{searchState: picker.NewSearchState(), cfg: cfg, title: title}
}

// SetItems replaces the items and clears the query.
func (m *Model) SetItems(items picker.Items) {
	m.items = items
	m.searchState.Reset()
}

// ReplaceItems replaces the items with the same ones changed, such as drawn with new images, keeping the query.
func (m *Model) ReplaceItems(items picker.Items) { m.items = items }

// Visible returns the matches shown, and a few more.
func (m Model) Visible() picker.Items {
	return m.searchState.Visible(m.items, m.cfg.Picker.Height)
}

// Selected returns the selected item.
func (m Model) Selected() (picker.Item, bool) {
	return m.searchState.Selected(m.items)
}

// Select selects the ith match.
func (m *Model) Select(i int) { m.searchState.Select(i) }

// Reset clears the query.
func (m *Model) Reset() { m.searchState.Reset() }

func (Model) Init() tview.Cmd { return nil }

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	if msg, ok := msg.(changeMsg); ok {
		m.searchState = picker.SearchState(msg)
	}
	return m, nil
}

// View shows the picker, turning the selected item into the message onSelect returns and canceling with onCancel.
func (m Model) View(onSelect func(picker.Item) tview.Msg, onCancel tview.Msg) tview.Widget {
	cfg := m.cfg
	bar := scrollbar.New().
		SymbolSet(cfg.UI.ScrollBar.SymbolSet.SymbolSet).
		Style(cfg.Theme.ScrollBar.TrackStyle.Style).
		ThumbStyle(cfg.Theme.ScrollBar.ThumbStyle.Style)
	p := picker.New(m.items, &m.searchState).
		ScrollBar(bar, cfg.UI.ScrollBar.Visibility.ScrollBarVisibility).
		Keybind(pickerKeybind(cfg.Keybinds.Picker)).
		ListKeybind(ui.SelectionKeybind(cfg.Keybinds.Picker.SelectionKeybinds)).
		OnChange(func(s *picker.SearchState) tview.Msg { return changeMsg(*s) }).
		OnSelect(onSelect).
		OnCancel(onCancel)
	clicks := clickWidget{Widget: p, cursor: cursorOf(m.searchState), items: m.items, onSelect: onSelect}
	return ui.Box(clicks, cfg, true).Title(ui.Label(m.title, cfg.Theme.Title, true))
}

// queryHeight is the query and the line under it, above the items.
const queryHeight = 2

// clickWidget picks an item clicked once it is selected: the first click selects it, the next picks it, as enter does.
type clickWidget struct {
	tview.Widget
	// cursor is the match selected before the click.
	cursor   int
	items    picker.Items
	onSelect func(picker.Item) tview.Msg
}

func (w clickWidget) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	handled := w.Widget.Handle(msg, area)
	mouse, ok := msg.(tview.MouseMsg)
	if !ok || mouse.Action != tview.MouseLeftDown || w.onSelect == nil {
		return handled
	}
	if _, y := mouse.Position(); y < area.Y+queryHeight {
		return handled
	}
	change, ok := handled.(changeMsg)
	if !ok {
		return handled
	}
	state := picker.SearchState(change)
	if cursorOf(state) != w.cursor {
		return handled
	}
	item, ok := state.Selected(w.items)
	if !ok {
		return handled
	}
	if selected := w.onSelect(item); selected != nil {
		return selected
	}
	return handled
}

// cursorOf returns the match selected in state, which the picker keeps to itself.
func cursorOf(state picker.SearchState) int {
	return int(reflect.ValueOf(state).FieldByName("cursor").Int())
}
