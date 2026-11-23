// Package aec cancels the echo of what is played that a microphone picks up from speakers, with a partitioned
// block frequency-domain adaptive filter that learns how the room carries the sound from the speakers to the
// microphone, and subtracts what it predicts.
package aec

import (
	"math/cmplx"
)

const (
	// block is how many samples are processed at a time; the transforms are twice as long.
	block = 256
	size  = 2 * block
	bins  = size/2 + 1
	// step is how fast the filter learns. Faster makes it unstable, as most partitions are kept causal only now and
	// then.
	step = 0.025
	// smoothing is how slowly the power of what is played, which normalizes the step, follows it.
	smoothing = 0.9
	// minRate is the least share of step the filter learns at, for it to start learning before it predicts any echo.
	minRate = 0.1
	// averaging is how slowly the energies that compare the two filters follow the errors.
	averaging = 0.7
)

// Canceller cancels the echo of the far end, what is played, from the near end, what the microphone captures,
// sampled at the same rate. It is not safe for concurrent use.
type Canceller struct {
	fft *fft
	// weights are the filter that learns, one partition of block samples of echo each. It learns from everything
	// the microphone picks up, the user's own voice too, which makes it worse while the user talks: fore, the
	// filter whose prediction is subtracted, is replaced by it only while it predicts the echo better, and it is
	// put back to fore when it went astray.
	weights, fore [][]complex128
	// learning and kept are the energies of what is left after subtracting the predictions of weights and fore.
	learning, kept float64
	// astray counts the blocks in a row that weights did much worse than fore.
	astray int
	// spectra are the transforms of the last far end blocks, newest first, one per partition.
	spectra [][]complex128
	power   []float64
	// far is the last two blocks of the far end.
	far []float64
	// constrain is the partition whose weights are kept causal next, one per block, as constraining all is costly.
	constrain int

	// in and out buffer the samples between blocks.
	nearIn, farIn, out []float64
	buf, err           []complex128
}

// New returns a canceller of echoes up to tail samples long.
func New(tail int) *Canceller {
	partitions := max((tail+block-1)/block, 1)
	c := &Canceller{
		fft:     newFFT(size),
		weights: make([][]complex128, partitions),
		fore:    make([][]complex128, partitions),
		spectra: make([][]complex128, partitions),
		power:   make([]float64, bins),
		far:     make([]float64, size),
		buf:     make([]complex128, size),
		err:     make([]complex128, size),
		// The first block comes out once it is processed: until then, silence does.
		out: make([]float64, block),
	}
	for i := range c.weights {
		c.weights[i] = make([]complex128, bins)
		c.fore[i] = make([]complex128, bins)
		c.spectra[i] = make([]complex128, bins)
	}
	return c
}

// Latency is how many samples later than captured the near end comes out.
func (c *Canceller) Latency() int { return block }

// Process takes near and far, as many samples of each, and returns as many samples of near without the echo of far,
// Latency samples late.
func (c *Canceller) Process(near, far []float64) []float64 {
	c.nearIn = append(c.nearIn, near...)
	c.farIn = append(c.farIn, far...)
	for len(c.nearIn) >= block && len(c.farIn) >= block {
		c.out = append(c.out, c.processBlock(c.nearIn[:block], c.farIn[:block])...)
		c.nearIn, c.farIn = c.nearIn[block:], c.farIn[block:]
	}
	c.nearIn, c.farIn = compact(c.nearIn), compact(c.farIn)
	result := make([]float64, len(near))
	copy(result, c.out)
	c.out = compact(c.out[len(near):])
	return result
}

func compact(s []float64) []float64 { return append([]float64(nil), s...) }

func (c *Canceller) processBlock(near, far []float64) []float64 {
	// The far end's newest two blocks are transformed, the oldest partition is dropped.
	copy(c.far, c.far[block:])
	copy(c.far[block:], far)
	last := c.spectra[len(c.spectra)-1]
	copy(c.spectra[1:], c.spectra[:len(c.spectra)-1])
	c.spectra[0] = last
	for i, v := range c.far {
		c.buf[i] = complex(v, 0)
	}
	c.fft.transform(c.buf, false)
	copy(c.spectra[0], c.buf[:bins])

	for i, x := range c.spectra[0] {
		p := real(x)*real(x) + imag(x)*imag(x)
		c.power[i] = smoothing*c.power[i] + (1-smoothing)*p
	}

	out, _ := c.subtract(c.fore, near)
	errs, predicted := c.subtract(c.weights, near)
	c.learning = averaging*c.learning + (1-averaging)*energy(errs)
	c.kept = averaging*c.kept + (1-averaging)*energy(out)
	switch {
	case c.learning < 0.9*c.kept:
		// The filter learning predicts the echo better: it is the one subtracted from now on.
		copyWeights(c.fore, c.weights)
		c.kept, c.astray = c.learning, 0
		out = errs
	case c.learning > 4*c.kept && c.kept > 0:
		// It learned the user's voice, or noise, for long enough to be lost: it starts over from fore.
		if c.astray++; c.astray > 10 {
			copyWeights(c.weights, c.fore)
			c.learning, c.astray = c.kept, 0
			return out
		}
	default:
		c.astray = 0
	}

	// What is left is learned from in proportion to how much of it is likely echo: the more the predicted echo
	// takes of it, the more of it is the echo not yet learned, while what the user says, which no echo explains,
	// makes it learn slowly, for it not to learn the voice.
	rate := step * min(1, max(minRate, predicted/max(energy(errs), 1e-9)))
	clear(c.err)
	for i, v := range errs {
		c.err[block+i] = complex(v, 0)
	}
	c.fft.transform(c.err, false)
	floor := 1e-6
	for _, p := range c.power {
		floor = max(floor, p*1e-3)
	}
	for p, w := range c.weights {
		x := c.spectra[p]
		for k := range bins {
			w[k] += complex(rate/(c.power[k]+floor), 0) * cmplx.Conj(x[k]) * c.err[k]
		}
	}
	c.keepCausal(c.weights[c.constrain])
	c.constrain = (c.constrain + 1) % len(c.weights)
	return out
}

// subtract returns near less the echo that weights predict from the far end, and the energy of that echo.
func (c *Canceller) subtract(weights [][]complex128, near []float64) ([]float64, float64) {
	clear(c.buf)
	for p, w := range weights {
		x := c.spectra[p]
		for k := range bins {
			c.buf[k] += w[k] * x[k]
		}
	}
	mirror(c.buf)
	c.fft.transform(c.buf, true)
	out := make([]float64, block)
	predicted := 0.0
	for i := range block {
		echo := real(c.buf[block+i])
		out[i] = near[i] - echo
		predicted += echo * echo
	}
	return out, predicted
}

func copyWeights(dst, src [][]complex128) {
	for i := range dst {
		copy(dst[i], src[i])
	}
}

func energy(s []float64) float64 {
	e := 0.0
	for _, v := range s {
		e += v * v
	}
	return e
}

// keepCausal zeroes the second half of the impulse response of w, which the overlap-save needs to be empty.
func (c *Canceller) keepCausal(w []complex128) {
	copy(c.buf, w)
	mirror(c.buf)
	c.fft.transform(c.buf, true)
	for i := block; i < size; i++ {
		c.buf[i] = 0
	}
	for i := range block {
		c.buf[i] = complex(real(c.buf[i]), 0)
	}
	c.fft.transform(c.buf, false)
	copy(w, c.buf[:bins])
}

// mirror fills the upper half of the spectrum x of a real signal from the lower.
func mirror(x []complex128) {
	n := len(x)
	for k := 1; k < n/2; k++ {
		x[n-k] = cmplx.Conj(x[k])
	}
}
