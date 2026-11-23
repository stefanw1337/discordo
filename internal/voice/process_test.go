//go:build !no_voice

package voice

import (
	"encoding/binary"
	"math/rand/v2"
	"testing"
)

func frameEnergy(frame []byte) float64 {
	e := 0.0
	for i := 0; i+1 < len(frame); i += sampleBytes {
		v := float64(int16(binary.LittleEndian.Uint16(frame[i:])))
		e += v * v
	}
	return e
}

func noiseFrame(rng *rand.Rand, scale float64) []byte {
	frame := make([]byte, frameBytes)
	for i := 0; i < frameSamples; i += channels {
		v := uint16(int16(rng.NormFloat64() * scale))
		binary.LittleEndian.PutUint16(frame[i*sampleBytes:], v)
		binary.LittleEndian.PutUint16(frame[(i+1)*sampleBytes:], v)
	}
	return frame
}

func TestProcessorSuppressesNoise(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	var p processor
	var before, after float64
	for i := range 100 {
		frame := noiseFrame(rng, 300)
		e := frameEnergy(frame)
		p.process(frame, Options{NoiseSuppression: true})
		if i >= 50 {
			before, after = before+e, after+frameEnergy(frame)
		}
	}
	if after > before/10 {
		t.Fatalf("noise kept %.0f%% of its energy", 100*after/before)
	}
}

func TestProcessorCancelsWhatWasPlayed(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	var p processor
	var before, after float64
	for i := range 300 {
		played := noiseFrame(rng, 3000)
		p.play(played)
		// The microphone picks up what was played, quieter.
		captured := make([]byte, len(played))
		for j := 0; j+1 < len(played); j += sampleBytes {
			binary.LittleEndian.PutUint16(captured[j:], uint16(int16(binary.LittleEndian.Uint16(played[j:]))/4))
		}
		e := frameEnergy(captured)
		p.process(captured, Options{EchoCancellation: true})
		if i >= 250 {
			before, after = before+e, after+frameEnergy(captured)
		}
	}
	if after > before/100 {
		t.Fatalf("echo kept %.1f%% of its energy", 100*after/before)
	}
}
