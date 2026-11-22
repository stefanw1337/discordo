package messageslist

import (
	"slices"

	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/keybind"
	"github.com/gdamore/tcell/v3"
)

// ActionMsg asks the list to do what keybind does, as if it were pressed, such as from the message menu.
type ActionMsg struct{ keybind keybind.Keybind }

func (ActionMsg) isMsg() {}

// is reports whether other is the keybind the action is of. A keybind is told by its keys, as its help may be reworded.
func (a ActionMsg) is(other keybind.Keybind) bool {
	return len(a.keybind.Keys()) != 0 && slices.Equal(a.keybind.Keys(), other.Keys())
}

// menuMsg asks for the menu of the message at an index, which was right-clicked.
type menuMsg int

func (menuMsg) isMsg() {}

// MenuItem is an action of the message menu: what it does, the key that does it too, and the message that does it.
type MenuItem struct {
	Label, Key string
	Action     ActionMsg
}

// ShowMenuMsg asks to show the actions that can be taken on the selected message.
type ShowMenuMsg []MenuItem

// showMenu asks to show the menu of the selected message: what its keybinds can do to it, and copying it.
func (m Model) showMenu() tview.Cmd {
	message, ok := m.selectedMessage()
	if !ok {
		return nil
	}

	keybinds := m.cfg.Keybinds.MessagesList
	actions, manage, attachments := m.messageHelp(*message)
	all := slices.Concat(actions, manage, attachments, []keybind.Keybind{keybinds.YankContent.Keybind, keybinds.YankURL.Keybind, keybinds.YankID.Keybind})
	var items ShowMenuMsg
	for _, k := range all {
		// An unbound keybind cannot be told from another.
		if len(k.Keys()) == 0 {
			continue
		}
		items = append(items, MenuItem{Label: k.Help().Desc, Key: k.Help().Key, Action: ActionMsg{k}})
	}
	if len(items) == 0 {
		return nil
	}
	return cmd.Task(func() tview.Msg { return items })
}

// rightClickable is a list that turns a right click on an item into a menuMsg for it.
type rightClickable struct {
	tview.Widget
}

func (w rightClickable) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	mouse, ok := msg.(tview.MouseMsg)
	if !ok || mouse.Action != tview.MouseRightClick || !area.Contains(mouse.Position()) {
		return w.Widget.Handle(msg, area)
	}

	// The list tells which item a press is on by selecting it.
	x, y := mouse.Position()
	press := tview.MouseMsg{EventMouse: tcell.NewEventMouse(x, y, tcell.ButtonPrimary, mouse.Modifiers()), Action: tview.MouseLeftDown}
	if index, ok := w.Widget.Handle(press, area).(listMsg); ok {
		return menuMsg(index)
	}
	return msg
}
