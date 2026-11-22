package sixel

import (
	"image"

	"github.com/ayn2op/tview"
	tviewimage "github.com/ayn2op/tview/image"
	"github.com/ayn2op/tview/layout"
	"github.com/ayn2op/tview/text"
)

// Widget draws an image with sixels while they are enabled, and with half blocks otherwise.
type Widget struct {
	// halfBlocks lays the image out and draws it without sixels.
	halfBlocks            tviewimage.Widget
	src                   image.Image
	cellWidth, cellHeight int
}

// New returns a widget that draws src cols wide, in cells of cellWidth by cellHeight pixels.
func New(src image.Image, cols, cellWidth, cellHeight int) Widget {
	return Widget{
		halfBlocks: tviewimage.New(src).Width(cols).CellSize(cellWidth, cellHeight),
		src:        src,
		cellWidth:  cellWidth,
		cellHeight: cellHeight,
	}
}

func (w Widget) Size() (width, height layout.Length) { return w.halfBlocks.Size() }

func (w Widget) Layout(limits layout.Limits) layout.Size { return w.halfBlocks.Layout(limits) }

func (w Widget) Handle(msg tview.Msg, area tview.Rectangle) tview.Msg { return msg }

// cells returns the columns and rows the image takes.
func (w Widget) cells() (cols, rows int) {
	width, height := w.halfBlocks.Size()
	return width.Cells(), height.Cells()
}

// Draw marks the cells the image takes within area, which the screen writes it over once the frame is shown.
func (w Widget) Draw(screen tview.Screen, area tview.Rectangle) {
	cols, rows := w.cells()
	id, ok := w.register(cols, rows)
	if !ok || cols > area.Width || rows > area.Height {
		w.halfBlocks.Draw(screen, area)
		return
	}
	for row := range rows {
		for col := range cols {
			screen.Put(area.X+col, area.Y+row, " ", markerStyle(id, col, row))
		}
	}
}

// Lines returns the image as text, a line of markers per row, to show it within other text.
func (w Widget) Lines() [][]text.Span {
	cols, rows := w.cells()
	id, ok := w.register(cols, rows)
	if !ok {
		return w.halfBlocks.Lines()
	}
	lines := make([][]text.Span, rows)
	for row := range rows {
		for col := range cols {
			lines[row] = append(lines[row], text.Span{Text: " ", Style: markerStyle(id, col, row)})
		}
	}
	return lines
}

func (w Widget) register(cols, rows int) (int, bool) {
	if !Enabled() || w.src == nil || w.src.Bounds().Empty() {
		return 0, false
	}
	return register(w.src, cols, rows, w.cellWidth, w.cellHeight)
}
