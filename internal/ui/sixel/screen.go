package sixel

import (
	"fmt"
	"hash/fnv"
	"image"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

// Markers are spaces whose foreground color is 24 bits: the image's ID, its column and its row.
const (
	idBits  = 12
	colBits = 6
	rowBits = 6

	maxImages = 1<<idBits - 1
	// MaxCols and MaxRows are the most cells an image can take.
	MaxCols = 1 << colBits
	MaxRows = 1 << rowBits
)

// entry is an image that markers stand for, and the cells it takes.
type entry struct {
	img        image.Image
	cols, rows int
	// cellWidth and cellHeight are the size of a cell in pixels that the image is drawn for.
	cellWidth, cellHeight int
}

// registry gives the images drawn with sixels IDs, which last while the image is kept, so that text built once,
// as custom emoji in messages are, keeps standing for its image.
type registry struct {
	mutex   sync.Mutex
	ids     map[entryKey]int
	entries map[int]entry
	next    int
}

type entryKey struct {
	img        image.Image
	cols, rows int
}

var images = registry{ids: make(map[entryKey]int), entries: make(map[int]entry)}

// enabled reports whether images are drawn with sixels.
var enabled atomic.Bool

// Enabled reports whether images are drawn with sixels.
func Enabled() bool { return enabled.Load() }

// SetEnabled sets whether images are drawn with sixels, when the terminal is known to show them.
func SetEnabled(on bool) { enabled.Store(on) }

// register returns the ID of img drawn over cols by rows cells of cellWidth by cellHeight pixels.
func register(img image.Image, cols, rows, cellWidth, cellHeight int) (int, bool) {
	if cols <= 0 || rows <= 0 || cols > MaxCols || rows > MaxRows {
		return 0, false
	}
	images.mutex.Lock()
	defer images.mutex.Unlock()

	key := entryKey{img, cols, rows}
	if id, ok := images.ids[key]; ok {
		return id, true
	}
	// IDs are used again once they run out, the oldest first, which a long session of many images reaches.
	images.next = images.next%maxImages + 1
	id := images.next
	if old, ok := images.entries[id]; ok {
		delete(images.ids, entryKey{old.img, old.cols, old.rows})
	}
	images.ids[key] = id
	images.entries[id] = entry{img: img, cols: cols, rows: rows, cellWidth: cellWidth, cellHeight: cellHeight}
	return id, true
}

func lookup(id int) (entry, bool) {
	images.mutex.Lock()
	defer images.mutex.Unlock()
	e, ok := images.entries[id]
	return e, ok
}

// markerStyle returns the style of the marker of the cell at col, row of the image with id.
func markerStyle(id, col, row int) tcell.Style {
	v := int32(id)<<(colBits+rowBits) | int32(col)<<rowBits | int32(row)
	return tcell.StyleDefault.Foreground(color.NewHexColor(v))
}

// IsMarker reports whether style is that of a marker, which nothing should change, so that it still tells its image.
func IsMarker(style tview.Style) bool {
	_, _, _, ok := decodeMarker(style)
	return ok
}

// decodeMarker returns the image and cell that style marks.
func decodeMarker(style tview.Style) (id, col, row int, ok bool) {
	fg := style.GetForeground()
	if !fg.IsRGB() || style.GetBackground() != color.Default {
		return 0, 0, 0, false
	}
	v := int(fg.Hex())
	id, col, row = v>>(colBits+rowBits), v>>rowBits&(MaxCols-1), v&(MaxRows-1)
	if id == 0 {
		return 0, 0, 0, false
	}
	e, found := lookup(id)
	if !found || col >= e.cols || row >= e.rows {
		return 0, 0, 0, false
	}
	return id, col, row, true
}

// shown is an image as it was written over the screen: where, which part of it, and the cells under it.
type shown struct {
	id, x, y, top, bottom int
	// frame is the frame of an animation shown.
	frame int
	// cells is a hash of the cells under it, which the screen writes again, over it, whenever they change.
	cells uint64
}

// Screen is a screen that writes the images marked on it with sixels once a frame is shown.
type Screen struct {
	tcell.Screen
	// out is where sixels are written, the terminal unless a test sets it.
	out io.Writer

	mutex sync.Mutex
	// written are the images over the screen, by where they are.
	written map[[2]int]shown
	// cache keeps images encoded, as they are written again whenever the screen is redrawn under them.
	cache map[shown][]byte
	// now returns the time, which tests set.
	now func() time.Time
}

// frameInterval is how often animations are moved to their next frame.
const frameInterval = 40 * time.Millisecond

// NewScreen returns screen, writing the images marked on it with sixels once SetEnabled turns them on.
func NewScreen(screen tcell.Screen) *Screen {
	s := &Screen{Screen: screen, written: make(map[[2]int]shown), cache: make(map[shown][]byte), now: time.Now}
	go s.animate()
	return s
}

// animate writes the next frames of the animations on the screen as they are due, where they were last written.
func (s *Screen) animate() {
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	for range ticker.C {
		if Enabled() {
			s.nextFrames()
		}
	}
}

// nextFrames writes the frames now due of the animations written, over the same cells.
func (s *Screen) nextFrames() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for where, image := range s.written {
		e, ok := lookup(image.id)
		if !ok {
			continue
		}
		animation, ok := e.img.(*Animation)
		if !ok {
			continue
		}
		if frame := animation.frameAt(s.now()); frame != image.frame {
			image.frame = frame
			s.write(image, e)
			s.written[where] = image
		}
	}
}

// Show shows the frame drawn, then writes its images over it. Animations, written between, are kept out meanwhile.
func (s *Screen) Show() {
	s.mutex.Lock()
	s.Screen.Show()
	s.mutex.Unlock()
	if Enabled() {
		s.writeImages()
	}
}

// Sync redraws the whole screen, and so its images.
func (s *Screen) Sync() {
	s.mutex.Lock()
	clear(s.written)
	s.Screen.Sync()
	s.mutex.Unlock()
	if Enabled() {
		s.writeImages()
	}
}

// writeImages finds the markers on the screen and writes the images they stand for that are not already there.
func (s *Screen) writeImages() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	width, height := s.Size()
	type found struct {
		x, y int
		// full counts the marked cells of each row, which is shown whole if all of its are.
		full map[int]int
	}
	// One image can be in several places, as a guild's icon is in its folder and by its name: each is found by where
	// its corner is.
	type place struct{ id, x, y int }
	marks := map[place]*found{}
	for y := range height {
		for x := range width {
			_, style, _ := s.Get(x, y)
			id, col, row, ok := decodeMarker(style)
			if !ok {
				continue
			}
			key := place{id, x - col, y - row}
			f := marks[key]
			if f == nil {
				f = &found{x: key.x, y: key.y, full: map[int]int{}}
				marks[key] = f
			}
			f.full[row]++
		}
	}

	written := make(map[[2]int]shown, len(marks))
	for key, f := range marks {
		id := key.id
		e, ok := lookup(id)
		if !ok {
			continue
		}
		// The longest run of whole rows is drawn. The bottom row of the screen is left out, as a sixel that
		// reaches it scrolls the screen in some terminals.
		top, bottom, bestTop, bestBottom := -1, -1, 0, 0
		for row := 0; row <= e.rows; row++ {
			whole := row < e.rows && f.full[row] == e.cols && f.y+row < height-1
			switch {
			case whole && top < 0:
				top = row
				bottom = row + 1
			case whole:
				bottom = row + 1
			case top >= 0:
				if bottom-top > bestBottom-bestTop {
					bestTop, bestBottom = top, bottom
				}
				top = -1
			}
		}
		if bestBottom == bestTop {
			continue
		}

		image := shown{id: id, x: f.x, y: f.y + bestTop, top: bestTop, bottom: bestBottom}
		if animation, ok := e.img.(*Animation); ok {
			image.frame = animation.frameAt(s.now())
		}
		image.cells = s.hashCells(f.x, f.y+bestTop, e.cols, bestBottom-bestTop)
		where := [2]int{image.x, image.y}
		written[where] = image
		if s.written[where] == image {
			continue
		}
		s.write(image, e)
	}
	s.written = written
}

// hashCells returns a hash of what the cells of a rectangle hold, which tells whether the screen wrote over them.
func (s *Screen) hashCells(x, y, width, height int) uint64 {
	h := fnv.New64a()
	for row := y; row < y+height; row++ {
		for col := x; col < x+width; col++ {
			str, style, _ := s.Get(col, row)
			fmt.Fprint(h, str, style)
		}
	}
	return h.Sum64()
}

// write writes the rows of the image from top to bottom over the cells where image says.
func (s *Screen) write(image shown, e entry) {
	key := shown{id: image.id, top: image.top, bottom: image.bottom, frame: image.frame}
	data, ok := s.cache[key]
	if !ok {
		if len(s.cache) > 1024 {
			clear(s.cache)
		}
		height := e.rows * e.cellHeight
		top, bottom := image.top*e.cellHeight, image.bottom*e.cellHeight
		img := e.img
		if animation, ok := img.(*Animation); ok {
			img = animation.Frames[image.frame]
		}
		data = Encode(img, e.cols*e.cellWidth, height, top, bottom)
		s.cache[key] = data
	}

	out := s.out
	if out == nil {
		tty, ok := s.Tty()
		if !ok {
			return
		}
		out = tty
	}
	if _, err := fmt.Fprintf(out, "\x1b[%d;%dH%s", image.y+1, image.x+1, data); err != nil {
		slog.Debug("failed to write a sixel image", "err", err)
	}
}
