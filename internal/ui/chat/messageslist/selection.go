package messageslist

import (
	"github.com/ayn2op/discordo/internal/ui/sixel"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

// selected draws its child highlighted with style, as the selected message is, except for the images in it, whose
// cells carry their pixels in their colors.
type selected struct {
	tview.Widget
	style tview.Style
	// inset is how many columns on the left, the avatar's, are left unhighlighted.
	inset int
}

func (w selected) Draw(screen tview.Screen, area tview.Rectangle) {
	styled := &selectionScreen{Screen: screen, style: w.style, minX: area.X + w.inset}
	styled.FillArea(area.X, area.Y, area.Width, area.Height, ' ', tcell.StyleDefault)
	w.Widget.Draw(styled, area)
}

// selectionScreen merges the selection's style into every cell drawn through it from column minX on.
type selectionScreen struct {
	tview.Screen
	style tview.Style
	minX  int
}

// merge returns style with the selection's merged into it, for a cell at column x, unless it marks where a sixel
// image goes, which it would no longer tell.
func (s *selectionScreen) merge(x int, style tview.Style) tview.Style {
	if x < s.minX || sixel.IsMarker(style) {
		return style
	}
	return tview.MergeStyle(s.style, style)
}

func (s *selectionScreen) SetContent(x, y int, primary rune, combining []rune, style tview.Style) {
	s.Screen.SetContent(x, y, primary, combining, s.merge(x, style))
}

func (s *selectionScreen) Put(x, y int, str string, style tview.Style) (string, int) {
	return s.Screen.Put(x, y, str, s.merge(x, style))
}

func (s *selectionScreen) FillArea(x, y, width, height int, r rune, style tview.Style) {
	if x < s.minX {
		width -= s.minX - x
		x = s.minX
	}
	if width <= 0 {
		return
	}
	s.Screen.FillArea(x, y, width, height, r, tview.MergeStyle(s.style, style))
}

func (s *selectionScreen) PutStr(x, y int, str string) {
	s.PutStrStyled(x, y, str, tcell.StyleDefault)
}

func (s *selectionScreen) PutStrStyled(x, y int, str string, style tview.Style) {
	s.Screen.PutStrStyled(x, y, str, s.merge(x, style))
}

// unhighlighted is an image, drawn in its own colors even in the selected message: half blocks are two pixels a
// cell, in its foreground and background, and kitty's placeholders tell the image by their foreground.
type unhighlighted struct {
	tview.Widget
}

func (w unhighlighted) Draw(screen tview.Screen, area tview.Rectangle) {
	if styled, ok := screen.(*selectionScreen); ok {
		screen = styled.Screen
	}
	w.Widget.Draw(screen, area)
}
