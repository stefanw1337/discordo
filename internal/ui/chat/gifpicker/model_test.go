package gifpicker

import (
	"encoding/json"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
)

func run(t *testing.T, command tview.Cmd) tview.Msg {
	t.Helper()
	task, ok := command.(cmd.Task)
	if !ok {
		t.Fatalf("command = %#v, want a task", command)
	}
	return task()
}

func TestFoundShowsAndPreviewsTheFirst(t *testing.T) {
	var gifs []GIF
	body := `[{"id":"1","title":"cat","url":"https://tenor.com/view/cat-1","src":"https://media.tenor.com/a/cat.mp4","gif_src":"https://media.tenor.com/a/cat.gif","preview":"https://media.tenor.com/a/cat.png","width":498,"height":280}]`
	if err := json.Unmarshal([]byte(body), &gifs); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	m := NewModel(&cfg, nil)
	m.requested = map[discord.URL]bool{}
	m.search = 1
	m, command := m.Update(foundMsg{search: 1, gifs: gifs})
	if command == nil {
		t.Fatal("the selected GIF was not asked for")
	}
	request, ok := run(t, command).(images.RequestedMsg)
	if !ok || request.Fitted["https://media.tenor.com/a/cat.gif"].MaxCols != previewCols {
		t.Fatalf("request = %#v", request)
	}
	if gif, ok := m.selected(); !ok || gif.URL != "https://tenor.com/view/cat-1" {
		t.Fatalf("selected %+v %v", gif, ok)
	}
	m, command = m.Update(selectedMsg{0})
	if sent, ok := run(t, command).(SelectedMsg); !ok || sent.GIF.URL != "https://tenor.com/view/cat-1" {
		t.Fatalf("sent %#v", sent)
	}
	var _ tview.Widget = m.View()
}
