package activity

import (
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
)

func TestFindPrefersTheFirstListed(t *testing.T) {
	programs := []config.Program{{Exe: "StarCitizen.exe", Name: "Star Citizen"}, {Exe: "Spotify.exe", Name: "Spotify", ShowTitle: true}}
	windows := []Window{{Exe: "spotify.exe", Title: "Artist - Song"}, {Exe: "starcitizen.EXE", Title: "Star Citizen"}}
	program, title, ok := Find(programs, windows)
	if !ok || program.Name != "Star Citizen" || title != "Star Citizen" {
		t.Fatalf("Find = %+v %q %v", program, title, ok)
	}
	if _, _, ok := Find(programs, []Window{{Exe: "firefox.exe"}}); ok {
		t.Fatal("found a program that is not listed")
	}
}

func TestNew(t *testing.T) {
	start := time.UnixMilli(1000)
	a := New(config.Program{Exe: "Spotify.exe", Type: "listening", ShowTitle: true}, "Artist - Song", start)
	if a.Name != "Spotify" || a.Type != discord.ListeningActivity || a.Details != "Artist - Song" || a.Timestamps.Start != 1000 {
		t.Fatalf("New = %+v", a)
	}
	a = New(config.Program{Exe: "x.exe", Name: "X"}, "secret title", start)
	if a.Details != "" || a.Type != discord.GameActivity {
		t.Fatalf("title shown without show_title: %+v", a)
	}
	if got := Exes([]Window{{Exe: "a.exe"}, {Exe: "A.EXE"}, {Exe: "b.exe"}}); len(got) != 2 {
		t.Fatalf("Exes = %v", got)
	}
}
