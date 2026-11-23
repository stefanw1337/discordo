package guildstree

import (
	"image"
	"strings"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/discordo/internal/ui/sixel"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/row"
	"github.com/ayn2op/tview/text"
	"github.com/ayn2op/tview/tree"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func TestNameIndex(t *testing.T) {
	if got := nameIndex([]text.Span{{Text: "name"}, {Text: " 1/5"}}); got != 0 {
		t.Fatalf("nameIndex without icons = %d, want 0", got)
	}
	if got := nameIndex(nil); got != 0 {
		t.Fatalf("nameIndex(nil) = %d, want 0", got)
	}
}

func TestGuildIcons(t *testing.T) {
	sixel.SetEnabled(true)
	t.Cleanup(func() { sixel.SetEnabled(false) })

	s := ningen.FromState(state.NewWithStore("", defaultstore.New()))
	guild := discord.Guild{ID: 1, Name: "guild", Icon: "abc"}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg, s)
	ready := &gateway.ReadyEvent{Guilds: []gateway.GuildCreateEvent{{Guild: guild}}, UserSettings: &gateway.UserSettings{}}
	s.State.Call(ready)
	m, command := m.Update(ready)
	if command == nil {
		t.Fatal("no icons were asked for")
	}
	task, ok := command.(cmd.Task)
	if !ok {
		t.Fatalf("command = %T, want a task", command)
	}
	request, ok := task().(images.RequestedMsg)
	url := iconURL(guild)
	if !ok || request.Squares[url] != 1 {
		t.Fatalf("asked for %#v, want the icon %s a row tall", request, url)
	}

	icon := sixel.New(image.NewRGBA(image.Rect(0, 0, 20, 20)), 2, 10, 20)
	m, _ = m.Update(images.ChangedMsg{url: icon})
	spans := m.guildNodes[guild.ID].Spans()
	// The first span, whose style the tree draws the expand marker in, must not be part of the icon.
	if len(spans) != 4 || spans[0].Text != "" || sixel.IsMarker(spans[0].Style) || !sixel.IsMarker(spans[1].Style) || spans[3].Text != " guild" {
		t.Fatalf("spans = %#v, want an empty lead, the icon, then the name", spans)
	}
	if nameIndex(spans) != 3 {
		t.Fatalf("nameIndex = %d, want 3", nameIndex(spans))
	}
}

func TestInitials(t *testing.T) {
	for name, want := range map[string]string{
		"Danielblnc's Neural Rendering Mods": "DNRM",
		"  ":                                 "?",
		"[GD] Reign of Terror Mod":           "GROT",
		"æøå server":                         "ÆS",
	} {
		if got := initials(name, 4); got != want {
			t.Errorf("initials(%q) = %q, want %q", name, got, want)
		}
	}
	if got := initialsSpan("Danielblnc's Neural", 2).Text; got != "DN" {
		t.Errorf("initials two wide = %q", got)
	}
}

func TestServerBar(t *testing.T) {
	s := ningen.FromState(state.NewWithStore("", defaultstore.New()))
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.UI.GuildsTree.GuildIcons = "bar"
	m := NewModel(cfg, s)
	guilds := []gateway.GuildCreateEvent{{Guild: discord.Guild{ID: 1, Name: "one"}}, {Guild: discord.Guild{ID: 2, Name: "two"}}}
	ready := &gateway.ReadyEvent{Guilds: guilds, UserSettings: &gateway.UserSettings{}}
	s.State.Call(ready)
	m, _ = m.Update(ready)

	entries := m.barEntries()
	if len(entries) != 3 || entries[0] != m.dmRootNode {
		t.Fatalf("bar entries = %d, want direct messages and two servers", len(entries))
	}
	m, _ = m.Update(barOpenMsg(2))
	if m.barRoot() != entries[2] || m.title() != "two" {
		t.Fatalf("open = %v, title = %q; want the second server", m.barRoot(), m.title())
	}
	m, _ = m.Update(tcell.NewEventKey(tcell.KeyRune, "[", tcell.ModNone))
	if m.barRoot() != entries[1] {
		t.Fatal("[ did not open the server before")
	}
	// Dragging the second server onto the first puts it first, and a press and release on one opens it.
	m, _ = m.Update(barPressMsg(2))
	m, _ = m.Update(barReleaseMsg(1))
	if entries := m.barEntries(); entries[1] != guildNode(m, 2) || entries[2] != guildNode(m, 1) {
		t.Fatal("the dragged server was not moved")
	}
	m, _ = m.Update(barPressMsg(1))
	m, _ = m.Update(barReleaseMsg(1))
	if m.barRoot() != guildNode(m, 2) {
		t.Fatal("clicking a server did not open it")
	}

	if _, ok := m.View().(row.Widget); !ok {
		t.Fatalf("view = %T, want the bar beside the tree", m.View())
	}

	// Drawn, the bar shows the servers without icons by their initials, the open one with its pill.
	screen, err := tcell.NewTerminfoScreenFromTty(vt.NewMockTerm(vt.MockOptSize{X: 40, Y: 20}))
	if err != nil {
		t.Fatal(err)
	}
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	m.View().Draw(screen, tview.Rectangle{Width: 40, Height: 20})
	cell := func(x, y int) string { str, _, _ := screen.Get(x, y); return str }
	if got := cell(0, 4) + cell(1, 5) + cell(2, 5) + cell(3, 5) + cell(4, 5); !strings.Contains(got, "T") || cell(0, 4) != "▌" {
		t.Fatalf("the open server's entry reads %q, want its pill and initial", got)
	}
	if cell(3, 1) != "D" && cell(2, 1) != "D" && cell(4, 1) != "D" {
		t.Fatalf("direct messages are not shown by their initials: %q", cell(1, 1)+cell(2, 1)+cell(3, 1)+cell(4, 1))
	}
}

func guildNode(m Model, id discord.GuildID) *tree.Node { return m.guildNodes[id] }
