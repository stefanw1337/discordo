package aec

import (
	"math"
	"math/bits"
)

// fft is a radix-2 FFT of one size.
type fft struct {
	n       int
	twiddle []complex128
	rev     []int
}

func newFFT(n int) *fft {
	f := &fft{n: n, twiddle: make([]complex128, n/2), rev: make([]int, n)}
	for i := range f.twiddle {
		angle := -2 * math.Pi * float64(i) / float64(n)
		f.twiddle[i] = complex(math.Cos(angle), math.Sin(angle))
	}
	shift := bits.UintSize - bits.Len(uint(n-1))
	for i := range f.rev {
		f.rev[i] = int(bits.Reverse(uint(i)) >> shift)
	}
	return f
}

// transform transforms x in place, or inverts the transform, scaled by 1/n, if inverse is set.
func (f *fft) transform(x []complex128, inverse bool) {
	for i, j := range f.rev {
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= f.n; size <<= 1 {
		half, step := size/2, f.n/size
		for start := 0; start < f.n; start += size {
			for k := range half {
				w := f.twiddle[k*step]
				if inverse {
					w = complex(real(w), -imag(w))
				}
				a, b := x[start+k], x[start+k+half]*w
				x[start+k], x[start+k+half] = a+b, a-b
			}
		}
	}
	if inverse {
		scale := complex(1/float64(f.n), 0)
		for i := range x {
			x[i] *= scale
		}
	}
}
