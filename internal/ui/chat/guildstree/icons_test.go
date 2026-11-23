package guildstree

import (
	"image"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/discordo/internal/ui/sixel"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/text"
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
