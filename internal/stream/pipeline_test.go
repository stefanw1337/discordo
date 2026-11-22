package stream

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// maxRTPPayload is the size the test packetizers split payloads at, as a sender keeps under the MTU.
const maxRTPPayload = 1100

// splitAnnexB returns the NAL units of an Annex B stream, without their start codes.
func splitAnnexB(stream []byte) [][]byte {
	var nals [][]byte
	start := -1
	for i := 0; i+3 <= len(stream); i++ {
		if stream[i] == 0 && stream[i+1] == 0 && stream[i+2] == 1 {
			if start >= 0 {
				end := i
				if end > start && stream[end-1] == 0 {
					end--
				}
				nals = append(nals, stream[start:end])
			}
			start = i + 3
			i += 2
		}
	}
	if start >= 0 && start < len(stream) {
		nals = append(nals, stream[start:])
	}
	return nals
}

// packetizeH264 splits the NAL units of each access unit into RTP payloads as RFC 6184 does: small ones whole, and
// large ones in FU-A fragments.
func packetizeH264(nals [][]byte) [][][]byte {
	var frames [][][]byte
	var current [][]byte
	for _, nal := range nals {
		t := nal[0] & 0x1F
		// An access unit ends before the next one's first slice, or its parameter sets.
		if (t == 7 || t == 9) && len(current) > 0 {
			frames = append(frames, current)
			current = nil
		}
		if len(nal) <= maxRTPPayload {
			current = append(current, nal)
		} else {
			header := nal[0]
			for i := 1; i < len(nal); i += maxRTPPayload {
				end := min(i+maxRTPPayload, len(nal))
				fu := []byte{header&0xE0 | 28, header & 0x1F}
				if i == 1 {
					fu[1] |= 0x80
				}
				if end == len(nal) {
					fu[1] |= 0x40
				}
				current = append(current, append(fu, nal[i:end]...))
			}
		}
		// Each slice of these test streams is a whole frame.
		if t == 1 || t == 5 {
			frames = append(frames, current)
			current = nil
		}
	}
	if len(current) > 0 {
		frames = append(frames, current)
	}
	return frames
}

// toPackets numbers the payloads of frames as RTP packets, a frame per timestamp with the marker on its last.
func toPackets(frames [][][]byte) []rtpPacket {
	var packets []rtpPacket
	seq := uint16(65000) // wraps around during the test
	for i, payloads := range frames {
		for j, payload := range payloads {
			packets = append(packets, rtpPacket{sequence: seq, timestamp: uint32(i) * 3000, marker: j == len(payloads)-1, payload: payload})
			seq++
		}
	}
	return packets
}

// shuffleNearby reorders packets within windows of 4, as a network does, so that none moves more than 3 places.
func shuffleNearby(packets []rtpPacket, r *rand.Rand) {
	for start := 0; start < len(packets); start += 4 {
		window := packets[start:min(start+4, len(packets))]
		r.Shuffle(len(window), func(i, j int) { window[i], window[j] = window[j], window[i] })
	}
}

// decodedFrames returns how many frames ffmpeg decodes from file, read as format.
func decodedFrames(t *testing.T, ffprobe, format, file string) int {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-f", format, "-count_frames", "-select_streams", "v:0", "-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.Split(string(out), "\n")[0]))
	if err != nil {
		t.Fatalf("ffprobe said %q", out)
	}
	return n
}

// playAll runs packets through a jitter buffer and depacketizer as the receiver does, and returns the frames it
// would play: from a keyframe on, and after a loss only from the next keyframe.
func playAll(t *testing.T, packets []rtpPacket, d depacketizer) [][]byte {
	t.Helper()
	jitter := newJitterBuffer(jitterDepth)
	// Packets come 2 ms apart, a frame of a few packets every 33 ms or so.
	clock := time.Unix(0, 0)
	jitter.now = func() time.Time { return clock }
	needKeyframe := true
	var played [][]byte
	for _, packet := range packets {
		clock = clock.Add(2 * time.Millisecond)
		for _, f := range jitter.push(packet) {
			if f.afterLoss {
				needKeyframe = true
			}
			data, keyframe, err := d.frame(f.payloads)
			if err != nil {
				needKeyframe = true
				continue
			}
			if needKeyframe && !keyframe {
				continue
			}
			needKeyframe = false
			played = append(played, data)
		}
	}
	return played
}

func TestH264Pipeline(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	ffprobe, err2 := exec.LookPath("ffprobe")
	if err != nil || err2 != nil {
		t.Skip("ffmpeg is needed to make and check the video")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.h264")
	// 60 frames with a keyframe every 20, in one slice each, as screen sharing is encoded.
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=30", "-frames:v", "60",
		"-pix_fmt", "yuv420p", "-c:v", "libx264", "-profile:v", "baseline", "-g", "20", "-bf", "0", "-x264-params", "slices=1:aud=0", "-f", "h264", source).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	frames := packetizeH264(splitAnnexB(data))
	packets := toPackets(frames)
	r := rand.New(rand.NewPCG(1, 2))

	t.Run("reordered", func(t *testing.T) {
		shuffled := append([]rtpPacket(nil), packets...)
		shuffleNearby(shuffled, r)
		played := playAll(t, shuffled, h264Depacketizer{})
		out := filepath.Join(dir, "reordered.h264")
		os.WriteFile(out, bytes.Join(played, nil), 0o644)
		if got := decodedFrames(t, ffprobe, "h264", out); got != 60 {
			t.Fatalf("decoded %d frames, want all 60", got)
		}
	})

	t.Run("a lost packet skips to the next keyframe", func(t *testing.T) {
		lost := append([]rtpPacket(nil), packets...)
		// Lose a packet of frame 25, after the keyframe at 20, so 25 to 39 cannot be shown until the keyframe at 40.
		for i, p := range lost {
			if p.timestamp == 25*3000 {
				lost = append(lost[:i], lost[i+1:]...)
				break
			}
		}
		played := playAll(t, lost, h264Depacketizer{})
		out := filepath.Join(dir, "lost.h264")
		os.WriteFile(out, bytes.Join(played, nil), 0o644)
		got := decodedFrames(t, ffprobe, "h264", out)
		if got != 45 {
			t.Fatalf("decoded %d frames, want 45: frames 0 to 24 and 40 to 59", got)
		}
	})
}

func TestVP8Pipeline(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	ffprobe, err2 := exec.LookPath("ffprobe")
	if err != nil || err2 != nil {
		t.Skip("ffmpeg is needed to make and check the video")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.ivf")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=30", "-frames:v", "30",
		"-c:v", "libvpx", "-g", "10", "-auto-alt-ref", "0", "-f", "ivf", source).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	// Split each frame of the IVF file into RTP payloads with a descriptor that starts a partition on the first.
	var frames [][][]byte
	for rest := data[32:]; len(rest) >= 12; {
		size := int(binary.LittleEndian.Uint32(rest))
		f := rest[12 : 12+size]
		rest = rest[12+size:]
		var payloads [][]byte
		for i := 0; i < len(f); i += maxRTPPayload {
			descriptor := byte(0x80) // X, with a 7 bit picture ID
			if i == 0 {
				descriptor |= 0x10
			}
			payloads = append(payloads, append([]byte{descriptor, 0x80, byte(len(frames) & 0x7F)}, f[i:min(i+maxRTPPayload, len(f))]...))
		}
		frames = append(frames, payloads)
	}

	packets := toPackets(frames)
	shuffleNearby(packets, rand.New(rand.NewPCG(3, 4)))
	played := playAll(t, packets, vp8Depacketizer{})

	var out bytes.Buffer
	w := ivfWriter{w: &out}
	for i, f := range played {
		w.writeFrame(f, uint32(i)*3000)
	}
	file := filepath.Join(dir, "out.ivf")
	os.WriteFile(file, out.Bytes(), 0o644)
	if got := decodedFrames(t, ffprobe, "ivf", file); got != 30 {
		t.Fatalf("decoded %d frames, want all 30", got)
	}
}
