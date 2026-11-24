// Package images holds the messages that load images and pass them to the models that draw them.
package images

import (
	"image"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/tview"
	tviewimage "github.com/ayn2op/tview/image"
	"github.com/ayn2op/tview/text"
)

// RequestedMsg asks for images: Squares by rows tall, Previews by size in pixels.
type RequestedMsg struct {
	Squares  map[discord.URL]uint
	Previews map[discord.URL]image.Point
	// Fitted are images drawn as large as they fit, without being scaled up.
	Fitted map[discord.URL]Fit
}

// Fit is the size in pixels of an image to draw, and the most cells it may take.
type Fit struct {
	Size             image.Point
	MaxCols, MaxRows uint
	// Fill makes the image take MaxCols by MaxRows cells whole, cut to that shape around its middle, as a banner
	// fills the width of a profile.
	Fill bool
	// Mirror turns the image around left to right, as Discord shows a nameplate.
	Mirror bool
	// Beside, with Fill, leaves room on the left for a square image this many rows tall, and a cell between.
	Beside uint
	// Compose, with Fill, makes the picture shown from the image loaded and the size in pixels it fills, such as
	// a nameplate with the name drawn over it.
	Compose func(img image.Image, width, height int) image.Image
}

// Widget is an image to draw: a widget of its own, or lines of text within other text, as custom emoji are.
// It is drawn with half blocks, kitty's graphics or sixels, as the terminal can.
type Widget interface {
	tview.Widget
	Lines() [][]text.Span
}

var _ Widget = tviewimage.Widget{}

// ChangedMsg is every image by URL, blank while it loads, replacing the images before.
type ChangedMsg map[discord.URL]Widget

// LoadedMsg is an image that finished loading, or an empty one if it failed.
type LoadedMsg struct {
	URL   discord.URL
	Image Widget
	// Cell is the size of a cell in image pixels that the image was sized for.
	Cell image.Point
}
