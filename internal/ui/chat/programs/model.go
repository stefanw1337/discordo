// Package programs is the page that picks the programs shown as the user's activity, names them, and orders them.
package programs

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/ayn2op/discordo/internal/activity"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	uipicker "github.com/ayn2op/discordo/internal/ui/picker"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/keybind"
	"github.com/ayn2op/tview/layout"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/text"
	"github.com/ayn2op/tview/textinput"
	"github.com/gdamore/tcell/v3"
)

const title = "Activity programs (enter rename · ctrl+t kind · ctrl+o window title · ctrl+d remove · ctrl+↑↓ order)"

// actions are the page's own keys, and what each does to the selected program.
var actions = []struct {
	key    keybind.Keybind
	action action
}{
	{keybind.New("ctrl+t"), actionType},
	{keybind.New("ctrl+o"), actionTitle},
	{keybind.New("ctrl+d"), actionRemove},
	{keybind.New("ctrl+up"), actionUp},
	{keybind.New("ctrl+down"), actionDown},
}

type action int

const (
	actionType action = iota
	actionTitle
	actionRemove
	actionUp
	actionDown
)

// program refers to the ith listed program; add to a program that is open and not listed.
type (
	program int
	add     string
)

// Model is the page of the programs.
type Model struct {
	list uipicker.Model
	cfg  *config.Config
	// open are the programs that have a window open, which are looked at only while sharing is enabled.
	open []string
	// renaming is the index of the program being renamed, or -1.
	renaming   int
	name       string
	nameCursor int
	err        error
}

func NewModel(cfg *config.Config) Model {
	return Model{list: uipicker.NewModel(cfg, title), cfg: cfg, renaming: -1}
}

// windowsMsg is the programs found open.
type windowsMsg []string

// ChangedMsg reports that the programs were changed and saved.
type ChangedMsg struct{}

// CancelMsg closes the page.
type CancelMsg struct{}

// Open lists the programs, and looks at the windows that are open if sharing is enabled.
func (m *Model) Open() tview.Cmd {
	m.renaming, m.err, m.open = -1, nil, nil
	m.list.SetItems(m.items())
	if !m.cfg.Activity.Enabled {
		return nil
	}
	return cmd.Task(func() tview.Msg { return windowsMsg(activity.Exes(activity.Windows())) })
}

func (m Model) listed(exe string) bool {
	return slices.ContainsFunc(m.cfg.Activity.Programs, func(p config.Program) bool { return strings.EqualFold(p.Exe, exe) })
}

func (m Model) items() picker.Items {
	dim := tcell.StyleDefault.Dim(true)
	bold := tcell.StyleDefault.Bold(true)
	var items picker.Items
	for i, p := range m.cfg.Activity.Programs {
		kind := p.Type
		if kind == "" {
			kind = "playing"
		}
		details := " " + p.Exe + " · " + kind
		if p.ShowTitle {
			details += " · window title shown"
		}
		name := p.Name
		if name == "" {
			name = activity.Name(p.Exe)
		}
		items = append(items, picker.Item{
			Content:    text.Rich(text.Span{Text: name, Style: bold}, text.Span{Text: details, Style: dim}).Wrapping(text.WrappingNone),
			FilterText: name + " " + p.Exe, Reference: program(i),
		})
	}
	for _, exe := range m.open {
		if m.listed(exe) {
			continue
		}
		items = append(items, picker.Item{
			Content:    text.Rich(text.Span{Text: "+ add "}, text.Span{Text: exe, Style: bold}, text.Span{Text: " (open now)", Style: dim}).Wrapping(text.WrappingNone),
			FilterText: exe, Reference: add(exe),
		})
	}
	note := "Only the programs listed here are ever shown. Others see the first of them that has a window open."
	switch {
	case m.err != nil:
		note = "Could not save: " + m.err.Error()
	case !m.cfg.Activity.Enabled:
		note = "Turn on \"Share what I'm doing\" in the settings to see the programs that are open and add them."
	}
	return append(items, picker.Item{Content: text.Rich(text.Span{Text: note, Style: dim}).Wrapping(text.WrappingNone), Reference: nil})
}

// save writes the programs to the config file and reports the change.
func (m *Model) save() tview.Cmd {
	programs := make([]map[string]any, 0, len(m.cfg.Activity.Programs))
	for _, p := range m.cfg.Activity.Programs {
		entry := map[string]any{"exe": p.Exe, "name": p.Name, "show_title": p.ShowTitle}
		if p.Type != "" {
			entry["type"] = p.Type
		}
		programs = append(programs, entry)
	}
	m.err = config.SetValue(m.cfg.Path, []string{"activity", "programs"}, programs)
	if m.err != nil {
		slog.Error("failed to save activity programs", "err", m.err)
	}
	m.list.ReplaceItems(m.items())
	return cmd.Task(func() tview.Msg { return ChangedMsg{} })
}

type (
	selectedMsg struct{ ref any }
	actionMsg   action
	nameMsg     struct {
		name   string
		cursor int
	}
	submitNameMsg struct{}
	cancelNameMsg struct{}
)

var _ tview.Model[Model] = Model{}

func (Model) Init() tview.Cmd { return nil }

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	programs := &m.cfg.Activity.Programs
	switch msg := msg.(type) {
	case windowsMsg:
		m.open = msg
		m.list.ReplaceItems(m.items())
		return m, nil
	case selectedMsg:
		switch ref := msg.ref.(type) {
		case program:
			m.startRename(int(ref))
		case add:
			// A program is added under the name of its file, and named at once.
			*programs = append(*programs, config.Program{Exe: string(ref), Name: activity.Name(string(ref))})
			m.list.SetItems(m.items())
			command := m.save()
			m.startRename(len(*programs) - 1)
			return m, command
		}
		return m, nil
	case nameMsg:
		m.name, m.nameCursor = msg.name, msg.cursor
		return m, nil
	case submitNameMsg:
		if m.renaming >= 0 && m.renaming < len(*programs) {
			(*programs)[m.renaming].Name = strings.TrimSpace(m.name)
		}
		m.renaming = -1
		return m, m.save()
	case cancelNameMsg:
		m.renaming = -1
		return m, nil
	case actionMsg:
		item, ok := m.list.Selected()
		if !ok {
			return m, nil
		}
		ref, ok := item.Reference.(program)
		if !ok {
			return m, nil
		}
		i := int(ref)
		p := &(*programs)[i]
		switch action(msg) {
		case actionType:
			p.Type = activity.Types[(slices.Index(activity.Types, kindOf(*p))+1)%len(activity.Types)]
		case actionTitle:
			p.ShowTitle = !p.ShowTitle
		case actionRemove:
			*programs = slices.Delete(*programs, i, i+1)
			m.list.SetItems(m.items())
		case actionUp:
			if i == 0 {
				return m, nil
			}
			(*programs)[i-1], (*programs)[i] = (*programs)[i], (*programs)[i-1]
			m.list.Select(i - 1)
		case actionDown:
			if i == len(*programs)-1 {
				return m, nil
			}
			(*programs)[i+1], (*programs)[i] = (*programs)[i], (*programs)[i+1]
			m.list.Select(i + 1)
		}
		return m, m.save()
	}
	var command tview.Cmd
	m.list, command = m.list.Update(msg)
	return m, command
}

func kindOf(p config.Program) string {
	if p.Type == "" {
		return "playing"
	}
	return p.Type
}

func (m *Model) startRename(i int) {
	p := m.cfg.Activity.Programs[i]
	m.renaming, m.name = i, p.Name
	m.nameCursor = len(m.name)
}

// View shows the list, or the name being typed.
func (m Model) View() tview.Widget {
	if m.renaming >= 0 && m.renaming < len(m.cfg.Activity.Programs) {
		input := textinput.New(m.name, m.nameCursor).
			Focused(true).
			OnInput(func(name string, cursor int) tview.Msg { return nameMsg{name, cursor} }).
			OnSubmit(submitNameMsg{})
		label := fmt.Sprintf("Name others see for %s (enter saves, esc cancels)", m.cfg.Activity.Programs[m.renaming].Exe)
		return ui.Box(renameWidget{input: input, cancel: m.cfg.Keybinds.Picker.Cancel.Keybind}, m.cfg, true).Title(ui.Label(label, m.cfg.Theme.Title, true))
	}
	list := m.list.View(func(item picker.Item) tview.Msg {
		if item.Reference == nil {
			return nil
		}
		return selectedMsg{item.Reference}
	}, CancelMsg{})
	return listWidget{list}
}

// listWidget gives the page's own keys to the page before the picker takes them for its query.
type listWidget struct{ tview.Widget }

func (w listWidget) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	if key, ok := msg.(tview.KeyMsg); ok {
		for _, a := range actions {
			if keybind.Matches(key, a.key) {
				return actionMsg(a.action)
			}
		}
	}
	return w.Widget.Handle(msg, area)
}

// renameWidget is the name being typed, one line tall.
type renameWidget struct {
	input  textinput.Widget
	cancel keybind.Keybind
}

func (renameWidget) Size() (width, height layout.Length) { return layout.Fill, layout.Fixed(1) }

func (w renameWidget) Layout(limits layout.Limits) layout.Size { return w.input.Layout(limits) }

func (w renameWidget) Draw(screen tview.Screen, area tview.Rectangle) { w.input.Draw(screen, area) }

func (w renameWidget) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg {
	if key, ok := msg.(tview.KeyMsg); ok && keybind.Matches(key, w.cancel) {
		return cancelNameMsg{}
	}
	return w.input.Handle(msg, area)
}
