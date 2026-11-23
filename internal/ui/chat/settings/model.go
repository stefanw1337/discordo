// Package settings is a page of discordo's settings, each changed by picking it, which saves it to the config file.
package settings

import (
	"log/slog"

	"github.com/ayn2op/discordo/internal/config"
	uipicker "github.com/ayn2op/discordo/internal/ui/picker"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
)

// Choice is one value a setting can have, as it is shown and as it is saved.
type Choice struct {
	Label string
	Value any
}

// Setting is a setting the page changes, by moving it to its next choice.
type Setting struct {
	Label string
	// Key is the setting's place in the config file: the names of its tables, then its own.
	Key     []string
	Choices []Choice
	Get     func(*config.Config) any
	Set     func(*config.Config, any)
	// Live is set for a setting that is applied at once. The others are applied when discordo is started again.
	Live bool
}

// textSetting and boolSetting make the Get and Set of a setting of the field field returns.
func textSetting(field func(*config.Config) *string) (func(*config.Config) any, func(*config.Config, any)) {
	return func(cfg *config.Config) any { return *field(cfg) },
		func(cfg *config.Config, value any) { *field(cfg) = value.(string) }
}

func boolSetting(field func(*config.Config) *bool) (func(*config.Config) any, func(*config.Config, any)) {
	return func(cfg *config.Config) any { return *field(cfg) },
		func(cfg *config.Config, value any) { *field(cfg) = value.(bool) }
}

var onOff = []Choice{{"on", true}, {"off", false}}

func newSetting(label string, key []string, choices []Choice, live bool, get func(*config.Config) any, set func(*config.Config, any)) Setting {
	return Setting{Label: label, Key: key, Choices: choices, Live: live, Get: get, Set: set}
}

// Settings returns the settings the page changes.
func Settings() []Setting {
	g, s := textSetting(func(c *config.Config) *string { return &c.UI.GuildsTree.GuildIcons })
	guildIcons := newSetting("Server list", []string{"ui", "guilds_tree", "guild_icons"},
		[]Choice{{"icons and names", "both"}, {"icons only", "icon"}, {"server bar, large icons", "bar"}, {"names only", "name"}}, true, g, s)
	g, s = textSetting(func(c *config.Config) *string { return &c.Timestamps.Format })
	timeFormat := newSetting("Time", []string{"timestamps", "format"},
		[]Choice{{"24-hour (23:07)", "15:04"}, {"12-hour (11:07PM)", "3:04PM"}}, false, g, s)
	g, s = textSetting(func(c *config.Config) *string { return &c.DateSeparator.Format })
	dateFormat := newSetting("Date", []string{"date_separator", "format"},
		[]Choice{{"18.11.2026", "02.01.2006"}, {"18 November 2026", "2 January 2006"}, {"November 18, 2026", "January 2, 2006"}}, false, g, s)
	g, s = textSetting(func(c *config.Config) *string { return &c.UI.MessagesList.ImageProtocol })
	images := newSetting("Images", []string{"ui", "messages_list", "image_protocol"},
		[]Choice{{"automatic", "auto"}, {"sharp (sixel)", "sixel"}, {"kitty", "kitty"}, {"blocks", "halfblocks"}}, false, g, s)

	widthGet := func(c *config.Config) any { return c.Sidebar.WidthPercent }
	widthSet := func(c *config.Config, value any) { c.Sidebar.WidthPercent = value.(int) }
	sidebarWidth := newSetting("Server list width", []string{"sidebar", "width_percent"},
		[]Choice{{"10%", 10}, {"15%", 15}, {"20%", 20}, {"25%", 25}, {"30%", 30}}, true, widthGet, widthSet)

	settings := []Setting{guildIcons, sidebarWidth, timeFormat, dateFormat, images}
	for _, b := range []struct {
		label string
		key   []string
		field func(*config.Config) *bool
	}{
		{"Hide muted channels", []string{"ui", "guilds_tree", "hide_muted_channels"}, func(c *config.Config) *bool { return &c.UI.GuildsTree.HideMutedChannels }},
		{"Expand folders", []string{"ui", "guilds_tree", "auto_expand_folders"}, func(c *config.Config) *bool { return &c.UI.GuildsTree.AutoExpandFolders }},
		{"Short file links", []string{"ui", "messages_list", "shorten_file_links"}, func(c *config.Config) *bool { return &c.UI.MessagesList.ShortenFileLinks }},
		{"Mouse", []string{"mouse"}, func(c *config.Config) *bool { return &c.Mouse }},
		{"Open in Windows Terminal", []string{"windows_terminal"}, func(c *config.Config) *bool { return &c.WindowsTerminal }},
	} {
		g, s := boolSetting(b.field)
		settings = append(settings, newSetting(b.label, b.key, onOff, false, g, s))
	}
	return settings
}

// current returns the index of the choice setting has in cfg, or -1 if it has another value.
func current(setting Setting, cfg *config.Config) int {
	value := setting.Get(cfg)
	for i, choice := range setting.Choices {
		if choice.Value == value {
			return i
		}
	}
	return -1
}

// Model is the settings page.
type Model struct {
	uipicker.Model
	cfg      *config.Config
	settings []Setting
	// restart is set once a setting that is applied on the next start was changed.
	restart bool
	err     error
}

func NewModel(cfg *config.Config) Model {
	return Model{Model: uipicker.NewModel(cfg, "Settings (enter changes)"), cfg: cfg, settings: Settings()}
}

// Open lists the settings as they are.
func (m *Model) Open() {
	m.restart, m.err = false, nil
	m.Model.SetItems(m.items())
}

func (m Model) items() picker.Items {
	items := make(picker.Items, 0, len(m.settings)+1)
	dim := tcell.StyleDefault.Dim(true)
	for i, setting := range m.settings {
		value := "custom"
		if index := current(setting, m.cfg); index >= 0 {
			value = setting.Choices[index].Label
		}
		spans := []text.Span{{Text: setting.Label + ": "}, {Text: value, Style: tcell.StyleDefault.Bold(true)}}
		if !setting.Live {
			spans = append(spans, text.Span{Text: "  (on restart)", Style: dim})
		}
		items = append(items, picker.Item{Content: text.Rich(spans...).Wrapping(text.WrappingNone), FilterText: setting.Label, Reference: i})
	}
	note := "Saved to " + m.cfg.Path
	if m.err != nil {
		note = "Could not save: " + m.err.Error()
	} else if m.restart {
		note = "Saved. Start discordo again for the settings marked (on restart)."
	}
	items = append(items, picker.Item{Content: text.Rich(text.Span{Text: note, Style: dim}).Wrapping(text.WrappingNone), FilterText: "", Reference: -1})
	return items
}

// selectedMsg asks to move the setting at index to its next choice.
type selectedMsg int

// ChangedMsg reports that the setting with Key was changed, and applied if it is Live.
type ChangedMsg struct {
	Key  []string
	Live bool
}

// CancelMsg closes the page.
type CancelMsg struct{}

var _ tview.Model[Model] = Model{}

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	if index, ok := msg.(selectedMsg); ok {
		if int(index) < 0 || int(index) >= len(m.settings) {
			return m, nil
		}
		setting := m.settings[index]
		next := setting.Choices[(current(setting, m.cfg)+1)%len(setting.Choices)]
		setting.Set(m.cfg, next.Value)
		m.err = config.SetValue(m.cfg.Path, setting.Key, next.Value)
		if m.err != nil {
			slog.Error("failed to save setting", "key", setting.Key, "err", m.err)
		}
		m.restart = m.restart || !setting.Live
		m.Model.ReplaceItems(m.items())
		changed := ChangedMsg{Key: setting.Key, Live: setting.Live}
		return m, cmd.Task(func() tview.Msg { return changed })
	}
	var command tview.Cmd
	m.Model, command = m.Model.Update(msg)
	return m, command
}

// View shows the page in a box titled Settings.
func (m Model) View() tview.Widget {
	return m.Model.View(func(item picker.Item) tview.Msg {
		index, ok := item.Reference.(int)
		if !ok || index < 0 {
			return nil
		}
		return selectedMsg(index)
	}, CancelMsg{})
}
