package profilecard

import (
	"image"
	"image/color"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	// plateOpacity is how much of the nameplate shows, over the color behind it, plateBackground how much of
	// that color shows at the right end, and plateShade how dark it is, of the nameplate's own color.
	plateOpacity    = 0.6
	plateBackground = 0.85
	plateShade      = 0.45
	// plateStart is how much of the color shows at the left end, behind the name, and plateBehind the gray of the
	// terminal's background it is laid over.
	plateStart  = 0.3
	plateBehind = 12.0
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

// composePlate draws a nameplate as Discord shows it behind a name: as tall as height pixels and kept to the right
// of width, with lines drawn over it, one to a row of rows.
func composePlate(plate image.Image, width, height, rows int, lines []plateLine) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	if bounds := plate.Bounds(); bounds.Dx() > 0 && bounds.Dy() > 0 {
		// Scaled to the height and kept to the right, where Discord shows it.
		w := bounds.Dx() * height / bounds.Dy()
		scaled := image.NewNRGBA(image.Rect(0, 0, w, height))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), plate, bounds, xdraw.Src, nil)

		// Discord fills the row behind the name with the nameplate's color, darkened, coming in from the left: it is
		// taken from the nameplate's own pixels.
		var r, g, b, n float64
		for i := 0; i+3 < len(scaled.Pix); i += 4 {
			if a := scaled.Pix[i+3]; a > 128 {
				r, g, b, n = r+float64(scaled.Pix[i]), g+float64(scaled.Pix[i+1]), b+float64(scaled.Pix[i+2]), n+1
			}
		}
		if n > 0 {
			r, g, b = r/n*plateShade, g/n*plateShade, b/n*plateShade
		}
		for y := range height {
			for x := range width {
				// The color, from a little at the left, behind the name, to most at the right.
				backA := plateStart + (plateBackground-plateStart)*float64(x)/float64(max(width-1, 1))
				var front color.NRGBA
				if at := x - (width - w); at >= 0 && at < w {
					front = scaled.NRGBAAt(at, y)
				}
				// The nameplate itself, see-through, over the color.
				frontA := float64(front.A) / 255 * plateOpacity
				outA := frontA + backA*(1-frontA)
				if outA <= 0 || n == 0 && frontA == 0 {
					continue
				}
				mix := func(f uint8, back float64) uint8 {
					return uint8(min((float64(f)*frontA+back*backA*(1-frontA))/outA, 255))
				}
				// Sixels have no see-through pixels but whole ones, so the plate is laid over the terminal's dark
				// background here, and drawn whole.
				over := func(c uint8, bg float64) uint8 { return uint8(min(float64(c)*outA+bg*(1-outA), 255)) }
				dst.SetNRGBA(x, y, color.NRGBA{
					over(mix(front.R, r), plateBehind), over(mix(front.G, g), plateBehind), over(mix(front.B, b), plateBehind), 255,
				})
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
