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
	"slices"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

// maxColors is how many colors an image is given, the most sixel terminals keep at once.
const maxColors = 256

// minAlpha is how opaque a pixel must be to be drawn. Those less opaque are left transparent.
const minAlpha = 128

// Encode returns img as a sixel image of width by height pixels, its rows from top to bottom of that height.
//
// The image is given a palette of its own colors, chosen by median cut, and dithered to it by error diffusion, which
// keeps gradients smooth without the repeating pattern of an ordered dither.
func Encode(img image.Image, width, height, top, bottom int) []byte {
	if width <= 0 || height <= 0 || bottom <= top {
		return nil
	}
	return Quantize(img, width, height).Sixel(top, bottom)
}

// Indexed is an image scaled, given a palette and dithered to it: the costly part of drawing it, done once, from
// which any of its rows are written quickly, as they are while it scrolls in and out of view.
type Indexed struct {
	width, height int
	palette       []color.NRGBA
	// indices are the palette's color of each pixel, or -1 for a transparent one.
	indices []int16
}

// Quantize scales img to width by height pixels and gives it a palette of its own colors.
func Quantize(img image.Image, width, height int) *Indexed {
	pixels := scale(img, width, height, 0, height)
	// Transparent pixels are left out of the palette and the image.
	opaque := make([]bool, width*height)
	var colors []color.NRGBA
	for i := range opaque {
		c := pixels.NRGBAAt(i%width, i/width)
		if c.A >= minAlpha {
			opaque[i] = true
			colors = append(colors, c)
		}
	}
	palette := medianCut(colors, maxColors)
	indices := make([]int16, width*height)
	dither(pixels, opaque, palette, indices)
	return &Indexed{width: width, height: height, palette: palette, indices: indices}
}

// Sixel returns the rows from top to bottom of the image as a sixel image.
func (x *Indexed) Sixel(top, bottom int) []byte {
	top, bottom = max(top, 0), min(bottom, x.height)
	if bottom <= top || x.width <= 0 {
		return nil
	}
	width, rows := x.width, bottom-top
	var out bytes.Buffer
	// P2=1: pixels that are not set keep what is behind them.
	out.WriteString("\x1bP0;1;0q")
	out.WriteString(`"1;1;` + strconv.Itoa(width) + ";" + strconv.Itoa(rows))
	for i, c := range x.palette {
		out.WriteString("#" + strconv.Itoa(i) + ";2;" + strconv.Itoa(percent(c.R)) + ";" + strconv.Itoa(percent(c.G)) + ";" +
			strconv.Itoa(percent(c.B)))
	}

	// Each band is six rows of pixels, drawn a color at a time: a sixel's six bits are the rows it sets. The bits
	// of every color of a band are gathered in one pass over its pixels.
	bands := make([][]byte, len(x.palette))
	inBand := make([]bool, len(x.palette))
	var used []int
	for y0 := 0; y0 < rows; y0 += 6 {
		used = used[:0]
		for dy := 0; dy < 6 && y0+dy < rows; dy++ {
			row := x.indices[(top+y0+dy)*width : (top+y0+dy+1)*width]
			for px, index := range row {
				if index < 0 {
					continue
				}
				band := bands[index]
				if band == nil {
					band = make([]byte, width)
					bands[index] = band
				}
				if !inBand[index] {
					inBand[index] = true
					used = append(used, int(index))
				}
				band[px] |= 1 << dy
			}
		}
		slices.Sort(used)
		for n, i := range used {
			if n > 0 {
				// Back to the start of the band, for the next color.
				out.WriteByte('$')
			}
			out.WriteString("#" + strconv.Itoa(i))
			writeRuns(&out, bands[i])
			clear(bands[i])
			inBand[i] = false
		}
		out.WriteByte('-')
	}
	out.WriteString("\x1b\\")
	return out.Bytes()
}

func scale(img image.Image, width, height, top, bottom int) *image.NRGBA {
	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, width, bottom-top))
	if bounds.Empty() {
		return dst
	}
	full := image.Rect(0, -top, width, height-top)
	var scaler xdraw.Scaler = xdraw.NearestNeighbor
	if bounds.Dx() > width || bounds.Dy() > height {
		scaler = xdraw.CatmullRom
	}
	scaler.Scale(dst, full, img, bounds, xdraw.Src, nil)
	return dst
}

// percent returns value, of 255, in percent, as sixels give colors.
func percent(value uint8) int {
	return (int(value)*100 + 127) / 255
}

// box is a set of colors that median cut splits.
type box []color.NRGBA

// span returns the channel, 0 to 2 for red, green and blue, whose values in b are most spread, and how far.
func (b box) span() (channel int, spread int) {
	var lo, hi [3]uint8
	lo = [3]uint8{255, 255, 255}
	for _, c := range b {
		for i, v := range [3]uint8{c.R, c.G, c.B} {
			lo[i], hi[i] = min(lo[i], v), max(hi[i], v)
		}
	}
	for i := range 3 {
		if d := int(hi[i]) - int(lo[i]); d > spread {
			channel, spread = i, d
		}
	}
	return channel, spread
}

// average returns the mean color of b.
func (b box) average() color.NRGBA {
	var r, g, bl int
	for _, c := range b {
		r, g, bl = r+int(c.R), g+int(c.G), bl+int(c.B)
	}
	n := len(b)
	return color.NRGBA{uint8((r + n/2) / n), uint8((g + n/2) / n), uint8((bl + n/2) / n), 255}
}

// medianCut returns a palette of at most n colors for colors: the box of them most spread is split at its median
// until there are n, and each box gives its mean.
func medianCut(colors []color.NRGBA, n int) []color.NRGBA {
	if len(colors) == 0 {
		return nil
	}
	boxes := []box{box(colors)}
	for len(boxes) < n {
		best, bestSpread, bestChannel := -1, 0, 0
		for i, b := range boxes {
			if len(b) < 2 {
				continue
			}
			// Boxes of many pixels are split first among those as spread, as they cover more of the image.
			channel, spread := b.span()
			if weighted := spread * intSqrt(len(b)); weighted > bestSpread {
				best, bestSpread, bestChannel = i, weighted, channel
			}
		}
		if best < 0 {
			break
		}
		b := boxes[best]
		slices.SortFunc(b, func(x, y color.NRGBA) int {
			return int(channelOf(x, bestChannel)) - int(channelOf(y, bestChannel))
		})
		mid := len(b) / 2
		boxes[best] = b[:mid]
		boxes = append(boxes, b[mid:])
	}

	palette := make([]color.NRGBA, 0, len(boxes))
	seen := make(map[color.NRGBA]bool, len(boxes))
	for _, b := range boxes {
		c := b.average()
		if !seen[c] {
			seen[c] = true
			palette = append(palette, c)
		}
	}
	return palette
}

func channelOf(c color.NRGBA, channel int) uint8 {
	switch channel {
	case 0:
		return c.R
	case 1:
		return c.G
	}
	return c.B
}

// intSqrt returns the square root of n, rounded down.
func intSqrt(n int) int {
	r := 0
	for (r+1)*(r+1) <= n {
		r++
	}
	return r
}

// dither sets indices to the palette color of each opaque pixel, carrying each pixel's error to its neighbors
// (Floyd–Steinberg), and to -1 for the others.
func dither(pixels *image.NRGBA, opaque []bool, palette []color.NRGBA, indices []int16) {
	width, rows := pixels.Rect.Dx(), pixels.Rect.Dy()
	// errs holds the error carried to this row and the next, in a sixteenth of a level.
	cur, next := make([][3]int, width+2), make([][3]int, width+2)
	cache := make(map[[3]uint8]int16)
	for y := range rows {
		for x := range width {
			i := y*width + x
			if !opaque[i] || len(palette) == 0 {
				indices[i] = -1
				continue
			}
			c := pixels.NRGBAAt(x, y)
			var want [3]int
			for ch, v := range [3]uint8{c.R, c.G, c.B} {
				want[ch] = max(0, min(255, int(v)+cur[x+1][ch]/16))
			}
			key := [3]uint8{uint8(want[0]), uint8(want[1]), uint8(want[2])}
			index, ok := cache[key]
			if !ok {
				index = nearest(palette, key)
				cache[key] = index
			}
			indices[i] = index
			got := palette[index]
			for ch, v := range [3]uint8{got.R, got.G, got.B} {
				e := want[ch] - int(v)
				cur[x+2][ch] += e * 7
				next[x][ch] += e * 3
				next[x+1][ch] += e * 5
				next[x+2][ch] += e
			}
		}
		cur, next = next, cur
		clear(next)
	}
}

// nearest returns the index of the color of palette nearest to c.
func nearest(palette []color.NRGBA, c [3]uint8) int16 {
	best, bestDistance := 0, int(^uint(0)>>1)
	for i, p := range palette {
		dr, dg, db := int(p.R)-int(c[0]), int(p.G)-int(c[1]), int(p.B)-int(c[2])
		// Weighted as the eye sees differences in each.
		if d := 2*dr*dr + 4*dg*dg + 3*db*db; d < bestDistance {
			best, bestDistance = i, d
		}
	}
	return int16(best)
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
