package ui

import (
	"github.com/ayn2op/discordo/internal/ui/sixel"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/layout"
)

// Backdrop dims what is behind a dialog or picker, as tview's backdrop does, but leaves the cells where images go as
// they are: restyled, they would no longer tell their images, which would then not be drawn at all.
type Backdrop struct{ style tview.Style }

var _ tview.Widget = Backdrop{}

// NewBackdrop returns a backdrop that merges style into what is behind it.
func NewBackdrop(style tview.Style) Backdrop { return Backdrop{style: style} }

func (Backdrop) Size() (width, height layout.Length) { return layout.Fill, layout.Fill }

func (Backdrop) Layout(limits layout.Limits) layout.Size {
	return layout.Atomic(limits, layout.Fill, layout.Fill)
}

func (w Backdrop) Draw(screen tview.Screen, area tview.Rectangle) {
	screen.HideCursor()
	for y := area.Y; y < area.Y+area.Height; y++ {
		for x := area.X; x < area.X+area.Width; x++ {
			str, style, width := screen.Get(x, y)
			if !sixel.IsMarker(style) {
				screen.Put(x, y, str, tview.MergeStyle(style, w.style))
			}
			// Wide characters take the cells after them too.
			x += max(width, 1) - 1
		}
	}
}

func (Backdrop) Handle(msg tview.Msg, _ tview.Rectangle) tview.Msg { return msg }
