// Package sixel draws images with sixels, in terminals that show them, such as Windows Terminal, WezTerm, foot,
// mintty and Konsole, which draw images at their own resolution, unlike half blocks.
//
// Images are drawn in two steps. While a frame is drawn, an image fills its cells with markers: spaces whose
// foreground color tells the image and the cell of it. Once the frame is shown, the screen finds the markers and
// writes each image over its cells, as far as they are uncovered, unless it is already there.
package sixel

import (
	"bytes"
	"image"
	"image/color"
	"strconv"
)

// The palette is a 6×6×6 color cube.
const (
	levels      = 6
	paletteSize = levels * levels * levels
)

// bayer is a 4×4 ordered dither, which hides the steps between the colors of the palette.
var bayer = [4][4]int{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

// minAlpha is how opaque a pixel must be to be drawn. Those less opaque are left transparent.
const minAlpha = 128

// Encode returns img as a sixel image of width by height pixels, its rows from top to bottom of that height.
func Encode(img image.Image, width, height, top, bottom int) []byte {
	if width <= 0 || height <= 0 || bottom <= top {
		return nil
	}
	rows := bottom - top
	bounds := img.Bounds()

	// Each pixel is scaled to the nearest of the source and given the index of its color in the palette, or -1.
	indices := make([]int16, width*rows)
	used := make([]bool, paletteSize)
	for y := range rows {
		sy := bounds.Min.Y + (top+y)*bounds.Dy()/height
		for x := range width {
			sx := bounds.Min.X + x*bounds.Dx()/width
			c := color.NRGBAModel.Convert(img.At(sx, sy)).(color.NRGBA)
			if c.A < minAlpha {
				indices[y*width+x] = -1
				continue
			}
			threshold := bayer[(top+y)%4][x%4]
			i := quantize(c.R, threshold)*levels*levels + quantize(c.G, threshold)*levels + quantize(c.B, threshold)
			indices[y*width+x] = int16(i)
			used[i] = true
		}
	}

	var out bytes.Buffer
	// P2=1: pixels that are not set keep what is behind them.
	out.WriteString("\x1bP0;1;0q")
	out.WriteString(`"1;1;` + strconv.Itoa(width) + ";" + strconv.Itoa(rows))
	for i, ok := range used {
		if !ok {
			continue
		}
		r, g, b := paletteColor(i)
		out.WriteString("#" + strconv.Itoa(i) + ";2;" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b))
	}

	// Each band is six rows of pixels, drawn a color at a time: a sixel's six bits are the rows it sets.
	band := make([]byte, width)
	for y0 := 0; y0 < rows; y0 += 6 {
		first := true
		for i, ok := range used {
			if !ok {
				continue
			}
			any := false
			for x := range width {
				var bits byte
				for dy := 0; dy < 6 && y0+dy < rows; dy++ {
					if indices[(y0+dy)*width+x] == int16(i) {
						bits |= 1 << dy
					}
				}
				band[x] = bits
				any = any || bits != 0
			}
			if !any {
				continue
			}
			if !first {
				// Back to the start of the band, for the next color.
				out.WriteByte('$')
			}
			first = false
			out.WriteString("#" + strconv.Itoa(i))
			writeRuns(&out, band)
		}
		out.WriteByte('-')
	}
	out.WriteString("\x1b\\")
	return out.Bytes()
}

// quantize returns the level of the palette nearest to value, nudged by the dither's threshold, from 0 to 15.
func quantize(value uint8, threshold int) int {
	step := 255.0 / float64(levels-1)
	level := (float64(value) + (float64(threshold)/16-0.5)*step) / step
	return max(0, min(levels-1, int(level+0.5)))
}

// paletteColor returns the color of the palette at i, in percent as sixels give colors.
func paletteColor(i int) (r, g, b int) {
	return (i / (levels * levels)) * 100 / (levels - 1), (i / levels % levels) * 100 / (levels - 1), (i % levels) * 100 / (levels - 1)
}

// writeRuns writes a band of sixels, each the bits it sets, with runs of the same one shortened as "!count".
func writeRuns(out *bytes.Buffer, band []byte) {
	// Sixels that set nothing at the end of the band need not be written.
	end := len(band)
	for end > 0 && band[end-1] == 0 {
		end--
	}
	for i := 0; i < end; {
		j := i
		for j < end && band[j] == band[i] {
			j++
		}
		char := band[i] + '?'
		if n := j - i; n > 3 {
			out.WriteString("!" + strconv.Itoa(n))
			out.WriteByte(char)
		} else {
			for range n {
				out.WriteByte(char)
			}
		}
		i = j
	}
}
