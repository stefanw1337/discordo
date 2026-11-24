package profilecard

import (
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// plateLine is a line of text drawn over a nameplate, in its color.
type plateLine struct {
	text  string
	color color.Color
	bold  bool
}

var (
	fontsOnce        sync.Once
	regular, bold    *opentype.Font
	fontsUnavailable bool
)

func loadFonts() {
	var err error
	if regular, err = opentype.Parse(gomono.TTF); err != nil {
		fontsUnavailable = true
		return
	}
	if bold, err = opentype.Parse(gomonobold.TTF); err != nil {
		fontsUnavailable = true
	}
}

// composePlate draws a nameplate as Discord shows it behind a name: filling width by height pixels from the right,
// where Discord shows it, fading out to the left, with lines drawn over it, one to a row of rows, from the second.
func composePlate(plate image.Image, width, height, rows int, lines []plateLine) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	if bounds := plate.Bounds(); bounds.Dx() > 0 && bounds.Dy() > 0 {
		opaque := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if _, _, _, a := plate.At(x, y).RGBA(); a > 0x8000 {
					opaque++
				}
			}
		}
		slog.Info("drawing names over nameplate", "type", fmt.Sprintf("%T", plate), "size", bounds.Size(), "opaque", opaque, "into", image.Pt(width, height))
		// Scaled to the height, turned around as Discord shows it, and kept to the right.
		w := bounds.Dx() * height / bounds.Dy()
		scaled := image.NewNRGBA(image.Rect(0, 0, w, height))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), plate, bounds, xdraw.Src, nil)
		for y := range height {
			for x := range w {
				c := scaled.NRGBAAt(w-1-x, y)
				at := width - w + x
				if at < 0 {
					continue
				}
				// Fading in from the left third of the width to full at the middle.
				fade := min(max(float64(at)/float64(width)*2-0.3, 0), 1)
				c.A = uint8(float64(c.A) * fade)
				dst.SetNRGBA(at, y, c)
			}
		}
	}

	fontsOnce.Do(loadFonts)
	if fontsUnavailable || rows <= 0 {
		return dst
	}
	cell := height / rows
	for i, line := range lines {
		row := i + max((rows-len(lines))/2, 0)
		if row >= rows {
			break
		}
		f := regular
		if line.bold {
			f = bold
		}
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(cell) * 0.78, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			continue
		}
		drawer := font.Drawer{Dst: dst, Src: image.NewUniform(line.color), Face: face}
		text := fitText(drawer, drawable(face, line.text), width-cell/2)
		drawer.Dot = fixed.P(cell/4, row*cell+cell*4/5)
		// A shadow keeps the text readable over a bright nameplate.
		shadow := drawer
		shadow.Src = image.NewUniform(color.NRGBA{0, 0, 0, 200})
		shadow.Dot = drawer.Dot.Add(fixed.P(1, 1))
		shadow.DrawString(text)
		drawer.DrawString(text)
		_ = face.Close()
	}
	return dst
}

// drawable leaves out of text what face has no glyph for, such as the ornaments some names have, which would be
// drawn as empty boxes.
func drawable(face font.Face, text string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if _, ok := face.GlyphAdvance(r); !ok {
			return -1
		}
		return r
	}, text))
}

// fitText cuts text, ending it with …, to fit within width pixels.
func fitText(drawer font.Drawer, text string, width int) string {
	if drawer.MeasureString(text).Ceil() <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && drawer.MeasureString(string(runes)+"…").Ceil() > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
