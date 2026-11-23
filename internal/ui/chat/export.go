package chat

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/ayn2op/discordo/internal/account"
	"github.com/ayn2op/discordo/internal/ui/chat/settings"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ncruces/zenity"
)

// exportAccount writes an export of the account beside the config file, with the config and the server list's state.
func (m *Model) exportAccount(invites bool) tview.Cmd {
	dir := filepath.Dir(m.cfg.Path)
	files := []string{m.cfg.Path, filepath.Join(dir, "tree.json")}
	state := m.state
	return cmd.Task(func() tview.Msg {
		folder, err := account.Write(state, dir, invites, files)
		if err != nil {
			return settings.NoteMsg("Could not export: " + err.Error())
		}
		return settings.NoteMsg("Exported to " + folder + ", with checklist.md of what to do by hand")
	})
}

// importProfile picks the folder of an export and sets the profile to it.
func (m *Model) importProfile() tview.Cmd {
	dir := filepath.Dir(m.cfg.Path) + string(filepath.Separator)
	state := m.state
	return cmd.Task(func() tview.Msg {
		folder, err := zenity.SelectFile(zenity.Directory(), zenity.Filename(dir), zenity.Title("Pick the folder of an export"))
		if errors.Is(err, zenity.ErrCanceled) {
			return nil
		}
		if err != nil {
			return settings.NoteMsg("Could not pick a folder: " + err.Error())
		}
		done, err := account.Import(state, folder)
		if err != nil {
			return settings.NoteMsg("Could not import: " + err.Error())
		}
		return settings.NoteMsg("Imported " + strings.Join(done, " and ") + ". checklist.md in the export lists the rest.")
	})
}
