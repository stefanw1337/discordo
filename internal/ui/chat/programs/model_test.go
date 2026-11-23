package programs

import (
	"path/filepath"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
)

func TestAddRenameAndSave(t *testing.T) {
	cfg := config.Default()
	cfg.Path = filepath.Join(t.TempDir(), "config.toml")
	cfg.Activity.Enabled = true
	m := NewModel(&cfg)
	m.Open()
	m, _ = m.Update(windowsMsg{"StarCitizen.exe", "explorer.exe"})

	m, _ = m.Update(selectedMsg{add("StarCitizen.exe")})
	if m.renaming != 0 || m.name != "StarCitizen" {
		t.Fatalf("not renaming the added program: %d %q", m.renaming, m.name)
	}
	m, _ = m.Update(nameMsg{"Star Citizen", 12})
	m, _ = m.Update(submitNameMsg{})
	m, _ = m.Update(actionMsg(actionTitle))

	got, err := config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Activity.Programs) != 1 || got.Activity.Programs[0] != (config.Program{Exe: "StarCitizen.exe", Name: "Star Citizen", ShowTitle: true}) {
		t.Fatalf("saved %+v", got.Activity.Programs)
	}
	if got.Activity.Enabled {
		t.Fatal("the page turned sharing on by itself")
	}
	var _ tview.Widget = m.View()
}
