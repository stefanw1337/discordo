package aec

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestCancelsARoomsEcho plays noise, whose echo, delayed and filtered as by a room, the microphone picks up, and
// checks that most of it is gone once the filter learned.
func TestCancelsARoomsEcho(t *testing.T) {
	const rate = 48000
	rng := rand.New(rand.NewPCG(1, 2))
	far := make([]float64, 6*rate)
	for i := range far {
		far[i] = rng.NormFloat64() * 3000
	}
	// The room: 30 ms of delay, then a decaying response.
	room := make([]float64, 2000)
	for i := range room {
		room[i] = math.Exp(-float64(i)/300) * rng.NormFloat64() * 0.02
	}
	delay := rate * 30 / 1000
	near := make([]float64, len(far))
	for n := range near {
		for k, h := range room {
			if i := n - delay - k; i >= 0 {
				near[n] += h * far[i]
			}
		}
	}

	c := New(rate / 5)
	var out []float64
	for i := 0; i < len(near); i += 480 {
		out = append(out, c.Process(near[i:i+480], far[i:i+480])...)
	}
	// The last second, after learning, against the echo it had.
	before := energy(near[len(near)-rate-c.Latency() : len(near)-c.Latency()])
	after := energy(out[len(out)-rate:])
	reduction := 10 * math.Log10(before/after)
	t.Logf("echo reduced by %.1f dB", reduction)
	if reduction < 20 {
		t.Fatalf("echo reduced by only %.1f dB", reduction)
	}
}

func TestKeepsTheNearEndWithoutEcho(t *testing.T) {
	c := New(4800)
	near := make([]float64, 4800)
	for i := range near {
		near[i] = 1000 * math.Sin(float64(i)/10)
	}
	out := c.Process(near, make([]float64, len(near)))
	for i := c.Latency(); i < len(out); i++ {
		if math.Abs(out[i]-near[i-c.Latency()]) > 1e-6 {
			t.Fatalf("sample %d changed: %v, want %v", i, out[i], near[i-c.Latency()])
		}
	}
}

// TestKeepsTheUserTalkingOverTheEcho checks that what the user says while the far end plays is kept, colored noise
// standing in for speech.
func TestKeepsTheUserTalkingOverTheEcho(t *testing.T) {
	const rate = 48000
	rng := rand.New(rand.NewPCG(3, 4))
	far := make([]float64, 6*rate)
	prev := 0.0
	for i := range far {
		prev = 0.95*prev + rng.NormFloat64()*1000
		far[i] = prev
	}
	near := make([]float64, len(far))
	delay := rate / 50
	for n := range near {
		if i := n - delay; i >= 0 {
			near[n] = 0.2*far[i] - 0.1*far[max(i-40, 0)]
		}
	}
	voice := make([]float64, len(far))
	for n := 4 * rate; n < len(voice); n++ {
		voice[n] = 4000 * math.Sin(2*math.Pi*220*float64(n)/rate)
		near[n] += voice[n]
	}
	c := New(rate / 5)
	var out []float64
	for i := 0; i < len(near); i += 960 {
		out = append(out, c.Process(near[i:i+960], far[i:i+960])...)
	}
	lat := c.Latency()
	residual := make([]float64, rate)
	for i := range residual {
		n := len(out) - rate + i
		residual[i] = out[n] - voice[n-lat]
	}
	echo := make([]float64, rate)
	for i := range echo {
		n := len(near) - rate - lat + i
		echo[i] = near[n] - voice[n]
	}
	t.Logf("while talking: echo %.1f dB under what it was", 10*math.Log10(energy(echo)/energy(residual)))
	t.Logf("residual %g echo %g", energy(residual), energy(echo))
	if energy(residual) > energy(echo) {
		t.Fatal("the user's voice was taken for echo")
	}
}
