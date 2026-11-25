package winterm

import (
	"image"
	"image/color"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Badge is what the taskbar's button of the terminal shows over its icon: the number of unread mentions, or that
// there is something unread.
type Badge struct {
	Mentions int
	Unread   bool
}

// badgeSize is the size of a taskbar overlay icon.
const badgeSize = 16

// badgeImage draws badge as Discord does: a red circle with the number of mentions, 9+ past nine, or a small white
// dot for unread messages without mentions. It returns nil for nothing to show.
func badgeImage(badge Badge) *image.NRGBA {
	if badge.Mentions <= 0 && !badge.Unread {
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, badgeSize, badgeSize))
	fill := color.NRGBA{237, 66, 69, 255}
	radius := float64(badgeSize) / 2
	if badge.Mentions <= 0 {
		fill, radius = color.NRGBA{255, 255, 255, 255}, 4
	}
	center := float64(badgeSize) / 2
	for y := range badgeSize {
		for x := range badgeSize {
			dx, dy := float64(x)+0.5-center, float64(y)+0.5-center
			if d := dx*dx + dy*dy; d <= radius*radius {
				img.SetNRGBA(x, y, fill)
			}
		}
	}
	if badge.Mentions <= 0 {
		return img
	}
	label := strconv.Itoa(badge.Mentions)
	if badge.Mentions > 9 {
		label = "9+"
	}
	drawer := font.Drawer{Dst: img, Src: image.NewUniform(color.White), Face: basicfont.Face7x13}
	width := drawer.MeasureString(label).Ceil()
	drawer.Dot = fixed.P((badgeSize-width)/2, 12)
	drawer.DrawString(label)
	return img
}
