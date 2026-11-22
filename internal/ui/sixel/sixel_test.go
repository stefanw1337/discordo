package sixel

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"
)

func solid(width, height int, c color.Color) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return img
}

// decode reads a sixel image as Encode writes it, and returns its pixels' palette colors in percent, or -1 where
// none is set.
func decode(t *testing.T, data []byte) (width, height int, pixels [][]int, palette map[int][3]int) {
	t.Helper()
	s := string(data)
	if !strings.HasPrefix(s, "\x1bP0;1;0q") || !strings.HasSuffix(s, "\x1b\\") {
		t.Fatalf("not a sixel image: %q", s)
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "\x1bP0;1;0q"), "\x1b\\")
	palette = map[int][3]int{}

	number := func() int {
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		n, _ := strconv.Atoi(s[:i])
		s = s[i:]
		return n
	}
	x, band, current := 0, 0, 0
	for len(s) > 0 {
		c := s[0]
		s = s[1:]
		switch {
		case c == '"':
			number()
			s = s[1:]
			number()
			s = s[1:]
			width = number()
			s = s[1:]
			height = number()
			pixels = make([][]int, height)
			for i := range pixels {
				pixels[i] = make([]int, width)
				for j := range pixels[i] {
					pixels[i][j] = -1
				}
			}
		case c == '#':
			current = number()
			if len(s) > 0 && s[0] == ';' {
				s = s[1:]
				number() // 2: RGB
				var rgb [3]int
				for i := range rgb {
					s = s[1:]
					rgb[i] = number()
				}
				palette[current] = rgb
			}
		case c == '$':
			x = 0
		case c == '-':
			x, band = 0, band+1
		case c == '!':
			n := number()
			bits := s[0] - '?'
			s = s[1:]
			for range n {
				set(pixels, x, band, bits, current)
				x++
			}
		case c >= '?' && c <= '~':
			set(pixels, x, band, c-'?', current)
			x++
		}
	}
	return width, height, pixels, palette
}

func set(pixels [][]int, x, band int, bits byte, color int) {
	for dy := range 6 {
		if bits&(1<<dy) != 0 && band*6+dy < len(pixels) && x < len(pixels[0]) {
			pixels[band*6+dy][x] = color
		}
	}
}

func TestEncode(t *testing.T) {
	t.Run("solid", func(t *testing.T) {
		width, height, pixels, palette := decode(t, Encode(solid(4, 4, color.NRGBA{255, 0, 0, 255}), 10, 8, 0, 8))
		if width != 10 || height != 8 {
			t.Fatalf("size = %dx%d, want 10x8", width, height)
		}
		for y, row := range pixels {
			for x, c := range row {
				if palette[c] != [3]int{100, 0, 0} {
					t.Fatalf("pixel %d,%d = %v, want red", x, y, palette[c])
				}
			}
		}
	})

	t.Run("cropped rows", func(t *testing.T) {
		// Red on top, blue below: the bottom half alone is blue.
		img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
		img.Set(1, 0, color.NRGBA{255, 0, 0, 255})
		img.Set(0, 1, color.NRGBA{0, 0, 255, 255})
		img.Set(1, 1, color.NRGBA{0, 0, 255, 255})
		_, height, pixels, palette := decode(t, Encode(img, 4, 12, 6, 12))
		if height != 6 {
			t.Fatalf("height = %d, want 6", height)
		}
		if palette[pixels[0][0]] != [3]int{0, 0, 100} {
			t.Fatalf("the top of the bottom half is %v, want blue", palette[pixels[0][0]])
		}
	})

	t.Run("transparent", func(t *testing.T) {
		_, _, pixels, _ := decode(t, Encode(solid(2, 2, color.NRGBA{0, 0, 0, 0}), 4, 6, 0, 6))
		for _, row := range pixels {
			for _, c := range row {
				if c != -1 {
					t.Fatal("a transparent pixel was set")
				}
			}
		}
	})
}

// testScreen returns a screen of width by height cells whose sixels are written to out.
func testScreen(t *testing.T, width, height int) (*Screen, *bytes.Buffer) {
	t.Helper()
	inner, err := tcell.NewTerminfoScreenFromTty(vt.NewMockTerm(vt.MockOptSize{X: vt.Col(width), Y: vt.Row(height)}))
	if err != nil {
		t.Fatal(err)
	}
	if err := inner.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(inner.Fini)
	screen := NewScreen(inner)
	out := new(bytes.Buffer)
	screen.out = out
	return screen, out
}

func TestScreen(t *testing.T) {
	SetEnabled(true)
	t.Cleanup(func() { SetEnabled(false) })
	screen, out := testScreen(t, 20, 10)

	// A 4×2 cell image of 8×16 pixel cells, drawn at 3,2.
	widget := New(solid(32, 32, color.NRGBA{0, 255, 0, 255}), 4, 8, 16)
	frame := func(cover bool) {
		screen.Clear()
		widget.Draw(screen, tview.Rectangle{X: 3, Y: 2, Width: 10, Height: 5})
		if cover {
			screen.PutStr(3, 3, "text")
		}
		screen.Show()
	}

	frame(false)
	written := out.String()
	if !strings.HasPrefix(written, "\x1b[3;4H\x1bP") {
		t.Fatalf("the image was not written at its cells: %q", written[:min(20, len(written))])
	}
	if _, height, _, _ := decode(t, []byte(strings.TrimPrefix(written, "\x1b[3;4H"))); height != 32 {
		t.Fatalf("height = %d pixels, want 2 rows of 16", height)
	}

	out.Reset()
	frame(false)
	if out.Len() != 0 {
		t.Fatal("an image that is already there was written again")
	}

	out.Reset()
	frame(true)
	if !strings.HasPrefix(out.String(), "\x1b[3;4H\x1bP") {
		t.Fatalf("the image was not written again over its uncovered row: %q", out.String())
	}
	if _, height, _, _ := decode(t, []byte(strings.TrimPrefix(out.String(), "\x1b[3;4H"))); height != 16 {
		t.Fatalf("height = %d pixels, want the one row left uncovered", height)
	}

	t.Run("disabled", func(t *testing.T) {
		SetEnabled(false)
		defer SetEnabled(true)
		out.Reset()
		frame(false)
		if out.Len() != 0 {
			t.Fatal("a sixel was written while they are off")
		}
		if lines := widget.Lines(); IsMarker(lines[0][0].Style) {
			t.Fatal("markers were given while sixels are off")
		}
	})

	t.Run("in text", func(t *testing.T) {
		lines := widget.Lines()
		if len(lines) != 2 || len(lines[0]) != 4 || !IsMarker(lines[1][3].Style) {
			t.Fatalf("lines = %v, want 2 rows of 4 markers", lines)
		}
	})
}

func TestEncodeGradient(t *testing.T) {
	// A photo's gradients keep their colors, close enough that no pattern shows, which a fixed palette did not.
	img := image.NewNRGBA(image.Rect(0, 0, 480, 270))
	for y := range 270 {
		for x := range 480 {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / 479), uint8(y * 255 / 269), 90, 255})
		}
	}
	start := time.Now()
	_, _, pixels, palette := decode(t, Encode(img, 480, 270, 0, 270))
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %v", elapsed)
	}
	var total int
	for y, row := range pixels {
		for x, c := range row {
			want := img.NRGBAAt(x, y)
			got := palette[c]
			total += abs(got[0]-percent(want.R)) + abs(got[1]-percent(want.G)) + abs(got[2]-percent(want.B))
		}
	}
	if mean := float64(total) / (480 * 270 * 3); mean > 2 {
		t.Fatalf("mean error = %.2f%%, want at most 2%%", mean)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
