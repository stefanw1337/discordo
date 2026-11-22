// Package messagemenu lists what can be done to a message, as right-clicking it shows, and does the one picked.
package messagemenu

import (
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/chat/messageslist"
	uipicker "github.com/ayn2op/discordo/internal/ui/picker"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
)

type Model struct {
	uipicker.Model
}

func NewModel(cfg *config.Config) Model {
	return Model{Model: uipicker.NewModel(cfg, "Message")}
}

// SetItems lists items, each with the key that does it too.
func (m *Model) SetItems(items []messageslist.MenuItem) {
	pickerItems := make(picker.Items, len(items))
	for i, item := range items {
		content := text.Rich(
			text.Span{Text: item.Label},
			text.Span{Text: "  " + item.Key, Style: tcell.StyleDefault.Dim(true)},
		).Wrapping(text.WrappingNone)
		pickerItems[i] = picker.Item{Content: content, FilterText: item.Label, Reference: item.Action}
	}
	m.Model.SetItems(pickerItems)
}

// SelectedMsg asks to do Action to the selected message.
type SelectedMsg struct{ Action messageslist.ActionMsg }

type CancelMsg struct{}

var _ tview.Model[Model] = Model{}

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	var command tview.Cmd
	m.Model, command = m.Model.Update(msg)
	return m, command
}

// View shows the menu in a box titled Message.
func (m Model) View() tview.Widget {
	return m.Model.View(func(item picker.Item) tview.Msg {
		action, ok := item.Reference.(messageslist.ActionMsg)
		if !ok {
			return nil
		}
		return SelectedMsg{Action: action}
	}, CancelMsg{})
}
