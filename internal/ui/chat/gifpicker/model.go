// Package gifpicker searches GIFs through Discord, as its GIF picker does, and picks one to send.
package gifpicker

import (
	"encoding/json"
	"fmt"
	"image"
	"log/slog"
	"net/url"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/discordo/internal/ui/images"
	uipicker "github.com/ayn2op/discordo/internal/ui/picker"
	"github.com/ayn2op/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	"github.com/ayn2op/tview/layout"
	"github.com/ayn2op/tview/picker"
	"github.com/ayn2op/tview/row"
	"github.com/ayn2op/tview/text"
	"github.com/gdamore/tcell/v3"
)

const (
	// previewCols and previewRows are the most cells the GIF shown beside the list takes.
	previewCols = 40
	previewRows = 18
)

// GIF is a GIF Discord found: the page sent as the message, which Discord shows as the GIF, and the files to show it.
type GIF struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// URL is the GIF's page, such as https://tenor.com/view/..., which is what is sent.
	URL     string `json:"url"`
	Src     string `json:"src"`
	GIFSrc  string `json:"gif_src"`
	Preview string `json:"preview"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

// image returns the picture shown of the GIF: the GIF itself, which moves, or its still preview.
func (g GIF) image() discord.URL {
	if g.GIFSrc != "" {
		return discord.URL(g.GIFSrc)
	}
	return discord.URL(g.Preview)
}

// Model is the GIF picker.
type Model struct {
	list   uipicker.Model
	cfg    *config.Config
	state  *ningen.State
	query  string
	gifs   []GIF
	err    error
	images map[discord.URL]images.Widget
	// requested are the pictures asked for, each once.
	requested map[discord.URL]bool
	// search counts the searches, so that only the answer to the last is shown.
	search int
}

func NewModel(cfg *config.Config, state *ningen.State) Model {
	return Model{list: uipicker.NewModel(cfg, "GIFs"), cfg: cfg, state: state}
}

// SelectedMsg is the GIF picked, to send.
type SelectedMsg struct{ GIF GIF }

// CancelMsg closes the picker.
type CancelMsg struct{}

type foundMsg struct {
	search int
	gifs   []GIF
	err    error
}

// Open searches query, or shows the GIFs trending if it is empty.
func (m *Model) Open(query string) tview.Cmd {
	m.search++
	m.query, m.gifs, m.err = query, nil, nil
	m.requested = map[discord.URL]bool{}
	m.list.SetItems(m.items())
	search, client := m.search, m.state.Client
	return cmd.Task(func() tview.Msg {
		gifs, err := find(client, query)
		return foundMsg{search: search, gifs: gifs, err: err}
	})
}

// find asks Discord for the GIFs of query, as its client does, which also keeps who searched from the GIF provider.
func find(client *api.Client, query string) ([]GIF, error) {
	params := url.Values{"media_format": {"gif"}, "provider": {"tenor"}, "locale": {"en-US"}}
	endpoint := api.Endpoint + "gifs/" + "trending-gifs"
	if query != "" {
		params.Set("q", query)
		endpoint = api.Endpoint + "gifs/" + "search"
	}
	raw, err := client.RequestJSON[json.RawMessage]("GET", endpoint+"?"+params.Encode())
	if err != nil {
		return nil, err
	}
	var gifs []GIF
	if err := json.Unmarshal(raw, &gifs); err != nil {
		slog.Error("unexpected answer to a GIF search", "body", string(raw), "err", err)
		return nil, fmt.Errorf("unexpected answer: %w", err)
	}
	return gifs, nil
}

func (m Model) items() picker.Items {
	dim := tcell.StyleDefault.Dim(true)
	var items picker.Items
	for i, gif := range m.gifs {
		title := gif.Title
		if title == "" {
			title = fmt.Sprintf("GIF %d", i+1)
		}
		items = append(items, picker.Item{Content: text.New(title).Wrapping(text.WrappingNone), FilterText: title, Reference: i})
	}
	if len(items) == 0 {
		note := "Searching…"
		switch {
		case m.err != nil:
			note = "Could not search: " + m.err.Error()
		case m.gifs != nil:
			note = "No GIFs found"
		}
		items = append(items, picker.Item{Content: text.Rich(text.Span{Text: note, Style: dim}).Wrapping(text.WrappingNone)})
	}
	return items
}

// selected returns the GIF selected.
func (m Model) selected() (GIF, bool) {
	item, ok := m.list.Selected()
	if !ok {
		return GIF{}, false
	}
	i, ok := item.Reference.(int)
	if !ok || i < 0 || i >= len(m.gifs) {
		return GIF{}, false
	}
	return m.gifs[i], true
}

// loadPreview asks for the picture of the GIF selected, if it was not asked for yet.
func (m *Model) loadPreview() tview.Cmd {
	gif, ok := m.selected()
	if !ok || gif.image() == "" || m.requested[gif.image()] {
		return nil
	}
	m.requested[gif.image()] = true
	size := image.Pt(gif.Width, gif.Height)
	if size.X <= 0 || size.Y <= 0 {
		size = image.Pt(498, 280)
	}
	request := images.RequestedMsg{Fitted: map[discord.URL]images.Fit{gif.image(): {Size: size, MaxCols: previewCols, MaxRows: previewRows}}}
	return cmd.Task(func() tview.Msg { return request })
}

type selectedMsg struct{ index int }

var _ tview.Model[Model] = Model{}

func (Model) Init() tview.Cmd { return nil }

func (m Model) Update(msg tview.Msg) (Model, tview.Cmd) {
	switch msg := msg.(type) {
	case foundMsg:
		if msg.search != m.search {
			return m, nil
		}
		m.gifs, m.err = msg.gifs, msg.err
		if m.gifs == nil && m.err == nil {
			m.gifs = []GIF{}
		}
		m.list.SetItems(m.items())
		return m, m.loadPreview()
	case images.ChangedMsg:
		m.images = msg
		return m, nil
	case selectedMsg:
		if msg.index < 0 || msg.index >= len(m.gifs) {
			return m, nil
		}
		gif := m.gifs[msg.index]
		return m, cmd.Task(func() tview.Msg { return SelectedMsg{GIF: gif} })
	}
	var command tview.Cmd
	m.list, command = m.list.Update(msg)
	// Moving the selection shows the GIF selected.
	return m, cmd.Batch{command, m.loadPreview()}
}

// View shows the GIFs found beside the one selected.
func (m Model) View() tview.Widget {
	list := m.list.View(func(item picker.Item) tview.Msg {
		i, ok := item.Reference.(int)
		if !ok {
			return nil
		}
		return selectedMsg{i}
	}, CancelMsg{})
	gif, ok := m.selected()
	if !ok {
		return list
	}
	var preview tview.Widget = text.Rich(text.Span{Text: "Loading…", Style: tcell.StyleDefault.Dim(true)})
	if widget, ok := m.images[gif.image()]; ok {
		preview = widget
	}
	title := "GIFs: " + m.query
	if m.query == "" {
		title = "Trending GIFs"
	}
	box := ui.Box(preview, m.cfg, false).Title(ui.Label(title, m.cfg.Theme.Title, false))
	return row.New(list, sized{box}).Height(layout.Fill)
}

// sized keeps the preview as wide as a GIF is shown.
type sized struct{ tview.Widget }

func (sized) Size() (width, height layout.Length) { return layout.Fixed(previewCols + 2), layout.Fill }
