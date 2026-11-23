package sixel

import (
	"image"
	"image/draw"
	"image/gif"
	"io"
	"time"
)

// Animation is an image of frames shown one after another, such as an animated GIF, looping. As an image.Image it is
// its first frame, so that what does not animate shows that.
type Animation struct {
	image.Image
	Frames []image.Image
	// Delays are how long each frame is shown.
	Delays []time.Duration
	total  time.Duration
}

// maxFrames is the most frames an animation keeps, as each is kept whole and encoded once.
const maxFrames = 120

// minDelay is the shortest a frame is shown, as browsers do for GIFs that ask for less.
const minDelay = 20 * time.Millisecond

// DecodeGIF returns the GIF read from r: an Animation if it has more than one frame, or its only frame.
func DecodeGIF(r io.Reader) (image.Image, error) {
	g, err := gif.DecodeAll(r)
	if err != nil {
		return nil, err
	}
	if len(g.Image) == 0 {
		return &image.Alpha{}, nil
	}

	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if bounds.Empty() {
		bounds = g.Image[0].Bounds()
	}
	canvas := image.NewRGBA(bounds)
	frames := make([]image.Image, 0, min(len(g.Image), maxFrames))
	delays := make([]time.Duration, 0, cap(frames))
	for i, frame := range g.Image {
		if i >= maxFrames {
			break
		}
		var previous *image.RGBA
		if i < len(g.Disposal) && g.Disposal[i] == gif.DisposalPrevious {
			previous = image.NewRGBA(bounds)
			draw.Draw(previous, bounds, canvas, bounds.Min, draw.Src)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		shown := image.NewRGBA(bounds)
		draw.Draw(shown, bounds, canvas, bounds.Min, draw.Src)
		frames = append(frames, shown)
		delay := minDelay
		if i < len(g.Delay) {
			delay = max(time.Duration(g.Delay[i])*10*time.Millisecond, minDelay)
		}
		delays = append(delays, delay)

		if i < len(g.Disposal) {
			switch g.Disposal[i] {
			case gif.DisposalBackground:
				draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
			case gif.DisposalPrevious:
				canvas = previous
			}
		}
	}
	if len(frames) == 1 {
		return frames[0], nil
	}
	return NewAnimation(frames, delays), nil
}

// NewAnimation returns the animation of frames, each shown for its delay.
func NewAnimation(frames []image.Image, delays []time.Duration) *Animation {
	a := &Animation{Image: frames[0], Frames: frames, Delays: delays}
	for _, delay := range delays {
		a.total += delay
	}
	return a
}

// frameAt returns the index of the frame shown at t, counting from the start of time, so that every copy of an
// animation shows the same frame.
func (a *Animation) frameAt(t time.Time) int {
	if a.total <= 0 {
		return 0
	}
	into := time.Duration(t.UnixNano()) % a.total
	for i, delay := range a.Delays {
		if into < delay {
			return i
		}
		into -= delay
	}
	return len(a.Frames) - 1
}
