package chat

import (
	"bufio"
	"compress/zlib"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/discordo/internal/ui/images"
	"github.com/ayn2op/discordo/internal/ui/sixel"
	"github.com/ayn2op/discordo/internal/winterm"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/cmd"
	tviewimage "github.com/ayn2op/tview/image"
	_ "golang.org/x/image/webp"
)

// kittyPixels is the assumed cell width when the terminal does not report it.
const kittyPixels = 10

// maxKittyImages is how many images kitty's placeholders tell apart.
const maxKittyImages = 63488

// compressorPool reuses compressors, as each holds about a megabyte of state.
var compressorPool = sync.Pool{New: func() any {
	compressor, _ := zlib.NewWriterLevel(nil, zlib.BestSpeed)
	return compressor
}}

// pooledCompressor returns its writer to the pool on Close.
type pooledCompressor struct {
	*zlib.Writer
}

func newPooledCompressor(w io.Writer) (io.WriteCloser, error) {
	compressor := compressorPool.Get().(*zlib.Writer)
	compressor.Reset(w)
	return pooledCompressor{compressor}, nil
}

func (compressor pooledCompressor) Close() error {
	err := compressor.Writer.Close()
	compressorPool.Put(compressor.Writer)
	return err
}

// WezTerm and Konsole support kitty graphics but not Unicode placeholders.
var placeholderTerminals = []string{"kitty", "ghostty", "Rio"}

// imageCell returns the size of a cell in image pixels.
func (m Model) imageCell() image.Point {
	switch {
	case !m.kitty && !m.sixel:
		return image.Pt(1, 2)
	case (m.cellWidth == 0 || m.cellHeight == 0) && m.sixel:
		// Sixels are drawn at their size in pixels, so one too large for its cells would spill out of them: until
		// the terminal tells, cells are taken to be as small as they usually are.
		return image.Pt(sixelPixels, sixelPixels*2)
	case m.cellWidth == 0 || m.cellHeight == 0:
		return image.Pt(kittyPixels, kittyPixels*2)
	}
	return image.Pt(int(m.cellWidth), int(m.cellHeight))
}

// minCellWidth and minCellHeight are the smallest cell, in pixels, taken as the terminal's.
const minCellWidth, minCellHeight = 4, 8

// sixelPixels is the cell width assumed for sixels when the terminal does not report it.
const sixelPixels = 8

// requestPixelSize asks the terminal for the size of its text area in pixels (XTWINOPS 14), which tcell reads as a
// resize, as Windows' console does not tell it.
var requestPixelSize tview.Cmd = cmd.WriteRaw("\x1b[14t")

// reloadImages loads the shown images again if the cell size is no longer cell, as they were sized for it.
func (m *Model) reloadImages(cell image.Point) tview.Cmd {
	if m.imageCell() == cell || len(m.images) == 0 {
		return nil
	}

	return cmd.Batch{m.clearImages(), m.messagesList.LoadImages(), m.guildsTree.LoadImages()}
}

// clearImages drops the images and returns a command that frees them in the terminal.
func (m *Model) clearImages() tview.Cmd {
	return m.clearImagesExcept(nil)
}

// clearChannelImages drops the images of the channel left, keeping the guild icons the guilds tree shows.
func (m *Model) clearChannelImages() tview.Cmd {
	return m.clearImagesExcept(m.guildsTree.IconURLs())
}

// clearImagesExcept drops the images but those of keep, and returns a command that frees them in the terminal.
func (m *Model) clearImagesExcept(keep map[discord.URL]bool) tview.Cmd {
	var cmds []tview.Cmd
	for url, image := range m.images {
		if keep[url] {
			continue
		}
		if kitty, ok := image.(tviewimage.Widget); ok {
			cmds = append(cmds, kitty.Delete())
		}
		delete(m.images, url)
	}
	return cmd.Batch(append(cmds, m.sendImages()))
}

// sendImages passes a copy of the images to the models that draw them.
func (m *Model) sendImages() tview.Cmd {
	images := images.ChangedMsg(maps.Clone(m.images))
	var command tview.Cmd
	m.reactionsPicker, command = m.reactionsPicker.Update(images)
	m.profileCard, _ = m.profileCard.Update(images)
	return cmd.Batch{command, m.updatePane(paneMessagesList, images), m.updatePane(paneComposer, images), m.updatePane(paneGuildsTree, images)}
}

func (m *Model) loadImages(msg images.RequestedMsg) tview.Cmd {
	cell := m.imageCell()
	cellWidth, cellHeight := uint(cell.X), uint(cell.Y)
	var cmds []tview.Cmd
	for url, rows := range msg.Squares {
		cols := squareCols(rows, cell)
		bounds := image.Rect(0, 0, int(cols*cellWidth), int(cols*cellWidth))
		cmds = append(cmds, m.loadImage(url, bounds, func(img image.Image) images.Widget {
			return m.newImage(square(img), int(cols), int(cellWidth), int(cellHeight))
		}))
	}

	attachments := m.cfg.UI.MessagesList.Attachments
	fitted := make(map[discord.URL]images.Fit, len(msg.Previews)+len(msg.Fitted))
	for url, size := range msg.Previews {
		fitted[url] = images.Fit{Size: size, MaxCols: attachments.PreviewWidth, MaxRows: attachments.PreviewHeight}
	}
	maps.Copy(fitted, msg.Fitted)

	for url, fit := range fitted {
		size := fit.Size
		cols := previewCols(uint(size.X), uint(size.Y), cellWidth, cellHeight, fit.MaxCols, fit.MaxRows)
		width := min(uint(size.X), uint(cols)*cellWidth)
		bounds := image.Rect(0, 0, int(width), int(uint(size.Y)*width/uint(size.X)))
		cmds = append(cmds, m.loadImage(url, bounds, func(img image.Image) images.Widget {
			return m.newImage(img, cols, int(cellWidth), int(cellHeight))
		}))
	}

	return cmd.Batch(cmds)
}

// newImage returns the widget that draws img cols wide in cells of cellWidth by cellHeight pixels: with sixels in a
// terminal that shows them, and otherwise with kitty's graphics or half blocks.
func (m *Model) newImage(img image.Image, cols, cellWidth, cellHeight int) images.Widget {
	if m.sixel {
		return sixel.New(img, cols, cellWidth, cellHeight)
	}
	return tviewimage.New(img).Width(cols).CellSize(cellWidth, cellHeight)
}

// A blank image holds the place until it loads, so nothing shifts.
func (m *Model) loadImage(url discord.URL, bounds image.Rectangle, widget func(image.Image) images.Widget) tview.Cmd {
	if _, ok := m.images[url]; ok {
		return nil
	}

	m.images[url] = widget(image.NewAlpha(bounds))
	cell := m.imageCell()
	return cmd.Task(func() tview.Msg {
		img, err := m.downloadImage(url, bounds)
		if err != nil {
			slog.Error("failed to load image", "url", url, "err", err)
			img = &image.Alpha{}
		}
		return images.LoadedMsg{URL: url, Image: widget(img), Cell: cell}
	})
}

// downloadImage expects proxy to be a Discord media proxy URL, which scales the image to bounds.
func (m Model) downloadImage(proxy discord.URL, bounds image.Rectangle) (image.Image, error) {
	previewURL, err := url.Parse(proxy)
	if err != nil {
		return nil, err
	}

	query := previewURL.Query()
	query.Set("width", strconv.Itoa(bounds.Dx()))
	query.Set("height", strconv.Itoa(bounds.Dy()))
	previewURL.RawQuery = query.Encode()

	resp, err := m.messagesList.Get(previewURL.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	return decodeImage(resp.Body)
}

// decodeImage reads an image, an animated GIF as an animation that sixels play and other images show the first frame
// of.
func decodeImage(r io.Reader) (image.Image, error) {
	reader := bufio.NewReader(r)
	if magic, _ := reader.Peek(4); string(magic) == "GIF8" {
		return sixel.DecodeGIF(reader)
	}
	img, _, err := image.Decode(reader)
	return img, err
}

// isKitty reports whether images are drawn with kitty's placeholders.
func (m Model) isKitty(name string) bool {
	switch m.cfg.UI.MessagesList.ImageProtocol {
	case "kitty":
		return true
	case "halfblocks", "sixel":
		return false
	}
	return slices.Contains(placeholderTerminals, name)
}

// sixelTerminals show sixels, as far as their name, which the terminal reports, tells.
var sixelTerminals = []string{"WezTerm", "foot", "mintty", "mlterm", "contour", "konsole", "Konsole", "iTerm2", "xterm"}

// isSixel reports whether images are drawn with sixels: when set, or for a terminal that shows them and has no kitty
// placeholders. Windows Terminal does not report its name, but sets WT_SESSION.
func (m Model) isSixel(name string) bool {
	switch m.cfg.UI.MessagesList.ImageProtocol {
	case "sixel":
		return true
	case "kitty", "halfblocks":
		return false
	}
	if m.isKitty(name) {
		return false
	}
	// Windows Terminal shows sixels, as do the other terminals that run programs in a pseudoconsole on Windows, but
	// does not always pass WT_SESSION on, as when it is Windows' default terminal.
	if os.Getenv("WT_SESSION") != "" || (runtime.GOOS == "windows" && winterm.Console() == "PseudoConsoleWindow") {
		return true
	}
	for _, terminal := range sixelTerminals {
		if strings.Contains(name, terminal) {
			return true
		}
	}
	return false
}

// squareCols returns the width in cells of a square image rows tall.
func squareCols(rows uint, cell image.Point) uint {
	return rows * uint(cell.Y) / uint(cell.X)
}

// previewCols returns the width in cells of a preview of an image of width by height pixels, at most maxWidth by
// maxHeight cells.
func previewCols(width, height, cellWidth, cellHeight, maxWidth, maxHeight uint) int {
	// Not scaled up.
	own := (width + cellWidth - 1) / cellWidth
	// A tall image is not cut off.
	tall := width * maxHeight * cellHeight / (height * cellWidth)
	return int(max(min(own, maxWidth, tall), 1))
}

// square centers img on a transparent square.
func square(img image.Image) image.Image {
	bounds := img.Bounds()
	if bounds.Dx() == bounds.Dy() {
		return img
	}

	side := max(bounds.Dx(), bounds.Dy())
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	at := image.Pt((side-bounds.Dx())/2, (side-bounds.Dy())/2)
	draw.Draw(dst, bounds.Sub(bounds.Min).Add(at), img, bounds.Min, draw.Src)
	return dst
}
