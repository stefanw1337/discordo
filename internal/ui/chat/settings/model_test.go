package settings

import (
	"path/filepath"
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview/cmd"
)

func TestChangeSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg)
	m.Open()

	// The server list goes from icons and names to icons only, at once and in the file.
	m, command := m.Update(selectedMsg(0))
	if cfg.UI.GuildsTree.GuildIcons != "icon" {
		t.Fatalf("guild_icons = %q, want icon", cfg.UI.GuildsTree.GuildIcons)
	}
	changed, ok := command.(cmd.Task)().(ChangedMsg)
	if !ok || !changed.Live {
		t.Fatalf("changed = %#v, want a live change", changed)
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.UI.GuildsTree.GuildIcons != "icon" {
		t.Fatalf("saved guild_icons = %q, want icon", saved.UI.GuildsTree.GuildIcons)
	}

	// A switch is turned off, and marked for the next start.
	mouse := len(Settings()) - 2
	m, _ = m.Update(selectedMsg(mouse))
	if cfg.Mouse || !m.restart {
		t.Fatalf("mouse = %v, restart = %v; want off, to restart", cfg.Mouse, m.restart)
	}
}

func TestSettingsCoverChoices(t *testing.T) {
	cfg := config.Default()
	for _, setting := range Settings() {
		if setting.Get == nil || setting.Set == nil || len(setting.Choices) < 2 {
			t.Fatalf("setting %q is incomplete", setting.Label)
		}
		if current(setting, &cfg) < 0 {
			t.Errorf("the default of %q is none of its choices", setting.Label)
		}
	}
}
