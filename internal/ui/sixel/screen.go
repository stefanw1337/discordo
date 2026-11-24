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
	// The last row number is anyRow's.
	if cols <= 0 || rows <= 0 || cols > MaxCols || rows >= MaxRows {
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

// anyRow is the row a marker on the terminal's screen tells: none, so that the cells of an image that moves up or
// down, as in a scroll, stay the same there, and the terminal does not draw them anew, which would clear the image
// before it is written again, making it blink. The screen keeps the rows itself.
const anyRow = MaxRows - 1

// storedStyle returns the style a marker of the cell at col of the image with id is given on the terminal's screen.
func storedStyle(id, col int) tcell.Style {
	v := int32(id)<<(colBits+rowBits) | int32(col)<<rowBits | anyRow
	return tcell.StyleDefault.Foreground(color.NewHexColor(v))
}

// IsMarker reports whether style is that of a marker, which nothing should change, so that it still tells its image.
func IsMarker(style tview.Style) bool {
	if _, _, _, ok := decodeMarker(style); ok {
		return true
	}
	_, _, ok := decodeStored(style)
	return ok
}

// decodeStored returns the image and column that style, of a marker as the terminal's screen holds it, tells.
func decodeStored(style tview.Style) (id, col int, ok bool) {
	fg := style.GetForeground()
	if !fg.IsRGB() || style.GetBackground() != color.Default {
		return 0, 0, false
	}
	v := int(fg.Hex())
	id, col = v>>(colBits+rowBits), v>>rowBits&(MaxCols-1)
	if id == 0 || v&(MaxRows-1) != anyRow {
		return 0, 0, false
	}
	e, found := lookup(id)
	if !found || col >= e.cols {
		return 0, 0, false
	}
	return id, col, true
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
	// quantized keeps images, and frames of animations, scaled and given their palettes, by ID and frame.
	quantized map[[2]int]*Indexed
	// now returns the time, which tests set.
	now func() time.Time
	// size is the size of the screen when it was last shown.
	size [2]int
	// marks are the markers put in the frame being drawn, by cell, with the rows the terminal's screen leaves out.
	marks map[[2]int]mark
	// unsynchronized leaves out synchronized updates, for tests to see only the images written.
	unsynchronized bool
	// moved is when images last moved, as they do while scrolling.
	moved time.Time
}

// mark is the cell of an image that a marker stands for.
type mark struct{ id, col, row int }

// Put puts str in a cell, keeping the row of a marker to itself and giving the terminal's screen the marker without it.
func (s *Screen) Put(x, y int, str string, style tcell.Style) (string, int) {
	return s.Screen.Put(x, y, str, s.store(x, y, style))
}

// SetContent sets a cell, as Put does.
func (s *Screen) SetContent(x, y int, primary rune, combining []rune, style tcell.Style) {
	s.Screen.SetContent(x, y, primary, combining, s.store(x, y, style))
}

// Clear clears the screen, and the markers of the last frame.
func (s *Screen) Clear() {
	clear(s.marks)
	s.Screen.Clear()
}

func (s *Screen) store(x, y int, style tcell.Style) tcell.Style {
	id, col, row, ok := decodeMarker(style)
	if !ok {
		return style
	}
	if s.marks == nil {
		s.marks = make(map[[2]int]mark)
	}
	s.marks[[2]int{x, y}] = mark{id, col, row}
	return storedStyle(id, col)
}

// settle is how long animations wait once images stop moving, for their frames not to slow a scroll.
const settle = 120 * time.Millisecond

// frameInterval is how often animations are moved to their next frame.
const frameInterval = 40 * time.Millisecond

// NewScreen returns screen, writing the images marked on it with sixels once SetEnabled turns them on.
func NewScreen(screen tcell.Screen) *Screen {
	s := &Screen{Screen: screen, written: make(map[[2]int]shown), cache: make(map[shown][]byte), quantized: make(map[[2]int]*Indexed), now: time.Now}
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
	// Animations wait while the screen scrolls, for scrolling not to be slowed by their frames.
	if s.now().Sub(s.moved) < settle {
		return
	}
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
	// A resized screen is drawn anew, which clears the images over it: they are all written again, even where
	// the cells under them did not change.
	if width, height := s.Size(); [2]int{width, height} != s.size {
		s.size = [2]int{width, height}
		clear(s.written)
	}
	enabled := Enabled()
	if enabled {
		s.synchronize(true)
	}
	s.Screen.Show()
	s.mutex.Unlock()
	if enabled {
		s.writeImages()
		s.mutex.Lock()
		s.synchronize(false)
		s.mutex.Unlock()
	}
}

// synchronize begins or ends a synchronized update, which a terminal that knows it, such as Windows Terminal, shows
// at once when it ends: the text drawn over images that moved, which clears them, and the images written again,
// which would otherwise blink between the two. Other terminals ignore it.
func (s *Screen) synchronize(begin bool) {
	if s.unsynchronized {
		return
	}
	out := s.out
	if out == nil {
		tty, ok := s.Tty()
		if !ok {
			return
		}
		out = tty
	}
	sequence := "\x1b[?2026l"
	if begin {
		sequence = "\x1b[?2026h"
	}
	_, _ = io.WriteString(out, sequence)
}

// Sync redraws the whole screen, and so its images.
func (s *Screen) Sync() {
	s.mutex.Lock()
	clear(s.written)
	enabled := Enabled()
	if enabled {
		s.synchronize(true)
	}
	s.Screen.Sync()
	s.mutex.Unlock()
	if enabled {
		s.writeImages()
		s.mutex.Lock()
		s.synchronize(false)
		s.mutex.Unlock()
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
	for cell, m := range s.marks {
		x, y := cell[0], cell[1]
		if x >= width || y >= height {
			continue
		}
		// A marker drawn over since is no longer one.
		if _, style, _ := s.Get(x, y); style != storedStyle(m.id, m.col) {
			continue
		}
		key := place{m.id, x - m.col, y - m.row}
		f := marks[key]
		if f == nil {
			f = &found{x: key.x, y: key.y, full: map[int]int{}}
			marks[key] = f
		}
		f.full[m.row]++
	}

	now := s.now()
	moved := false

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
		if before, ok := s.written[where]; !ok || before.id != image.id {
			moved = true
		}
		s.write(image, e)
	}
	if moved {
		s.moved = now
	}
	s.written = written
}

// hashCells returns a hash of what the cells of a rectangle hold, which tells whether the screen wrote over them.
func (s *Screen) hashCells(x, y, width, height int) uint64 {
	h := fnv.New64a()
	for row := y; row < y+height; row++ {
		for col := x; col < x+width; col++ {
			str, style, _ := s.Get(col, row)
			_, _ = fmt.Fprint(h, str, style)
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
		// The image is quantized once, whichever of its rows are shown, as they change while it scrolls in and
		// out of view.
		whole := [2]int{image.id, image.frame}
		indexed, ok := s.quantized[whole]
		if !ok {
			if len(s.quantized) > 512 {
				clear(s.quantized)
			}
			img := e.img
			if animation, ok := img.(*Animation); ok {
				img = animation.Frames[image.frame]
			}
			indexed = Quantize(img, e.cols*e.cellWidth, e.rows*e.cellHeight)
			s.quantized[whole] = indexed
		}
		data = indexed.Sixel(image.top*e.cellHeight, image.bottom*e.cellHeight)
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

// SetOutForTest sets where screen writes its sixels, for tests of other packages.
func SetOutForTest(screen *Screen, out io.Writer) { screen.out = out }
