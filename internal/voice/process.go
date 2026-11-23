//go:build !no_voice

package voice

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"sync"

	rnnoise "github.com/VoiceBlender/rnnoise-go"
	"github.com/VoiceBlender/rnnoise-go/model"
	"github.com/ayn2op/discordo/internal/voice/aec"
)

const (
	// echoTail is the longest echo cancelled, from the speakers through the room to the microphone, with the
	// latency of both devices.
	echoTail = sampleRate / 4
	// maxPlayed is how much of what was played is kept to cancel; more is dropped down to keptPlayed, for what was
	// played not to lag behind what the microphone captures by more than the echo's reach.
	maxPlayed  = sampleRate / 16
	keptPlayed = sampleRate / 50
)

var errNoModel = errors.New("noise suppression failed to start")

// processor cleans what the microphone captures before it is sent: it removes the echo of what was played, then the
// noise.
type processor struct {
	// mu guards played, what the speakers played, in mono, not yet cancelled from what the microphone captured.
	mu     sync.Mutex
	played []float64

	// The rest is used by the goroutine that sends alone.
	denoiser  *rnnoise.Denoiser
	failed    bool
	canceller *aec.Canceller
	in, out   []float32
}

// play keeps what the speakers play, interleaved 16-bit PCM, to cancel its echo.
func (p *processor) play(output []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i+channels*sampleBytes <= len(output); i += channels * sampleBytes {
		left := int16(binary.LittleEndian.Uint16(output[i:]))
		right := int16(binary.LittleEndian.Uint16(output[i+sampleBytes:]))
		p.played = append(p.played, (float64(left)+float64(right))/2)
	}
	if len(p.played) > maxPlayed {
		p.played = append(p.played[:0], p.played[len(p.played)-keptPlayed:]...)
	}
}

// takePlayed returns the next n samples played, silence where none were.
func (p *processor) takePlayed(n int) []float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	far := make([]float64, n)
	taken := copy(far, p.played)
	p.played = append(p.played[:0], p.played[taken:]...)
	return far
}

// process cleans frame, 20 ms of interleaved 16-bit PCM, in place, as options say.
func (p *processor) process(frame []byte, options Options) {
	if !options.NoiseSuppression && !options.EchoCancellation {
		p.canceller = nil
		return
	}
	n := len(frame) / (channels * sampleBytes)
	mono := make([]float64, n)
	for i := range mono {
		left := int16(binary.LittleEndian.Uint16(frame[i*channels*sampleBytes:]))
		right := int16(binary.LittleEndian.Uint16(frame[i*channels*sampleBytes+sampleBytes:]))
		mono[i] = (float64(left) + float64(right)) / 2
	}

	if options.EchoCancellation {
		if p.canceller == nil {
			p.canceller = aec.New(echoTail)
		}
		mono = p.canceller.Process(mono, p.takePlayed(n))
	} else {
		p.canceller = nil
	}

	if options.NoiseSuppression {
		// Should it fail, what is captured is sent as it is.
		_ = p.denoise(mono)
	}

	for i, v := range mono {
		sample := uint16(int16(max(min(math.Round(v), math.MaxInt16), math.MinInt16)))
		binary.LittleEndian.PutUint16(frame[i*channels*sampleBytes:], sample)
		binary.LittleEndian.PutUint16(frame[i*channels*sampleBytes+sampleBytes:], sample)
	}
}

// denoise removes the noise of mono, in place, in RNNoise's frames of 10 ms.
func (p *processor) denoise(mono []float64) error {
	if p.denoiser == nil {
		if p.failed {
			return errNoModel
		}
		m, err := model.Load()
		if err == nil {
			p.denoiser, err = rnnoise.New(rnnoise.Options{SampleRate: sampleRate, Model: m})
		}
		if err != nil {
			slog.Error("failed to start noise suppression", "err", err)
			p.failed = true
			return err
		}
	}
	size := p.denoiser.FrameSize()
	if len(p.in) != size {
		p.in, p.out = make([]float32, size), make([]float32, size)
	}
	for start := 0; start+size <= len(mono); start += size {
		for i := range size {
			p.in[i] = float32(mono[start+i])
		}
		if _, err := p.denoiser.Process(p.out, p.in); err != nil {
			return err
		}
		for i := range size {
			mono[start+i] = float64(p.out[i])
		}
	}
	return nil
}
