package sixel

import (
	"image"
	"image/color"
	"testing"
)

func BenchmarkSixelRows(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 800, 450))
	for y := range 450 {
		for x := range 800 {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}
	indexed := Quantize(img, 800, 450)
	b.ResetTimer()
	for b.Loop() {
		_ = indexed.Sixel(20, 400)
	}
}
