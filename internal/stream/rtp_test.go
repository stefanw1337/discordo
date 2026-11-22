package stream

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
)

func TestKey(t *testing.T) {
	for _, test := range []struct {
		key     Key
		guild   discord.GuildID
		channel discord.ChannelID
		user    discord.UserID
	}{
		{"guild:1:2:3", 1, 2, 3},
		{"call:2:3", 0, 2, 3},
	} {
		if got := NewKey(test.guild, test.channel, test.user); got != test.key {
			t.Errorf("NewKey = %q, want %q", got, test.key)
		}
		guild, channel, user, err := test.key.Parse()
		if err != nil || guild != test.guild || channel != test.channel || user != test.user {
			t.Errorf("Parse(%q) = %v %v %v %v", test.key, guild, channel, user, err)
		}
	}
	for _, bad := range []Key{"", "guild:1:2", "call:a:b", "lobby:1:2"} {
		if _, _, _, err := bad.Parse(); err == nil {
			t.Errorf("Parse(%q) took an invalid key", bad)
		}
	}
}

// rtpHeader returns a 12 byte RTP header, with the extension bit and a one-word extension preamble if ext.
func rtpHeader(pt uint8, marker bool, seq uint16, ts, ssrc uint32, ext bool) []byte {
	h := make([]byte, 12)
	h[0] = 0x80
	h[1] = pt
	if marker {
		h[1] |= 0x80
	}
	binary.BigEndian.PutUint16(h[2:], seq)
	binary.BigEndian.PutUint32(h[4:], ts)
	binary.BigEndian.PutUint32(h[8:], ssrc)
	if ext {
		h[0] |= 0x10
		h = append(h, 0xBE, 0xDE, 0, 1)
	}
	return h
}

func TestParseRTP(t *testing.T) {
	packet := rtpHeader(101, true, 7, 9000, 42, true)
	header, extension, err := rtpHeaderLength(append(packet, make([]byte, 10)...))
	if err != nil || header != 16 || extension != 4 {
		t.Fatalf("rtpHeaderLength = %d %d %v, want 16 4", header, extension, err)
	}

	body := append([]byte{1, 2, 3, 4}, 'h', 'i')
	p, err := parseRTP(packet, extension, body)
	if err != nil || p.payloadType != 101 || !p.marker || p.sequence != 7 || p.timestamp != 9000 || p.ssrc != 42 || string(p.payload) != "hi" {
		t.Fatalf("parseRTP = %+v %v", p, err)
	}

	t.Run("padding", func(t *testing.T) {
		padded := rtpHeader(101, false, 1, 1, 1, false)
		padded[0] |= 0x20
		p, err := parseRTP(padded, 0, []byte{'o', 'k', 0, 0, 3})
		if err != nil || string(p.payload) != "ok" {
			t.Fatalf("parseRTP = %q %v, want ok", p.payload, err)
		}
	})

	t.Run("rtx", func(t *testing.T) {
		rtx := rtpPacket{payloadType: 102, sequence: 500, ssrc: 43, payload: []byte{0, 9, 'x'}}
		p, err := unwrapRTX(rtx, 42, 101)
		if err != nil || p.sequence != 9 || p.ssrc != 42 || p.payloadType != 101 || string(p.payload) != "x" {
			t.Fatalf("unwrapRTX = %+v %v", p, err)
		}
	})
}

func packetOf(seq uint16, ts uint32, marker bool, payload string) rtpPacket {
	return rtpPacket{sequence: seq, timestamp: ts, marker: marker, payload: []byte(payload)}
}

func TestJitterBuffer(t *testing.T) {
	t.Run("reorders", func(t *testing.T) {
		j := newJitterBuffer(8)
		if frames := j.push(packetOf(1, 10, false, "a")); len(frames) != 0 {
			t.Fatal("a frame was let out before its last packet")
		}
		if frames := j.push(packetOf(3, 20, true, "c")); len(frames) != 0 {
			t.Fatal("a frame was let out ahead of a missing one")
		}
		frames := j.push(packetOf(2, 10, true, "b"))
		if len(frames) != 2 || string(bytes.Join(frames[0].payloads, nil)) != "ab" || string(frames[1].payloads[0]) != "c" {
			t.Fatalf("frames = %v, want ab then c", frames)
		}
		if frames[0].afterLoss || frames[1].afterLoss {
			t.Fatal("frames were marked as after a loss")
		}
	})

	t.Run("wraps around", func(t *testing.T) {
		j := newJitterBuffer(8)
		j.push(packetOf(65535, 10, false, "a"))
		frames := j.push(packetOf(0, 10, true, "b"))
		if len(frames) != 1 || string(bytes.Join(frames[0].payloads, nil)) != "ab" {
			t.Fatalf("frames = %v, want ab", frames)
		}
	})

	t.Run("gives up on a lost packet", func(t *testing.T) {
		j := newJitterBuffer(3)
		j.push(packetOf(1, 10, true, "a"))
		// Packet 2, a whole frame, is lost.
		var frames []frame
		for seq := uint16(3); seq <= 6; seq++ {
			frames = append(frames, j.push(packetOf(seq, uint32(seq)*10, true, "x"))...)
		}
		if len(frames) == 0 {
			t.Fatal("the buffer waited forever on a lost packet")
		}
		if !frames[0].afterLoss {
			t.Fatal("the first frame after the loss was not marked")
		}
	})

	t.Run("drops a frame whose start was lost", func(t *testing.T) {
		j := newJitterBuffer(2)
		j.push(packetOf(1, 10, true, "a"))
		// Packet 2, the start of frame 20, is lost; 3 is its end.
		var frames []frame
		for _, p := range []rtpPacket{packetOf(3, 20, true, "tail"), packetOf(4, 30, true, "b"), packetOf(5, 40, true, "c")} {
			frames = append(frames, j.push(p)...)
		}
		for _, f := range frames {
			if string(f.payloads[0]) == "tail" {
				t.Fatal("a frame without its start was let out")
			}
		}
		if len(frames) == 0 || string(frames[0].payloads[0]) != "b" {
			t.Fatalf("frames = %v, want b first", frames)
		}
	})

	t.Run("drops late and repeated packets", func(t *testing.T) {
		j := newJitterBuffer(8)
		j.push(packetOf(5, 10, true, "a"))
		if frames := j.push(packetOf(4, 5, true, "late")); len(frames) != 0 {
			t.Fatal("a late packet was let out")
		}
		if frames := j.push(packetOf(5, 10, true, "a")); len(frames) != 0 {
			t.Fatal("a repeated packet was let out")
		}
	})
}

func TestH264Depacketizer(t *testing.T) {
	sps := []byte{0x67, 1, 2}
	pps := []byte{0x68, 3}
	idrStart := []byte{0x7C, 0x85, 0xAA} // FU-A, start, type 5
	idrEnd := []byte{0x7C, 0x45, 0xBB}   // FU-A, end
	stap := []byte{0x18, 0, 3}
	stap = append(stap, sps...)
	stap = append(stap, 0, 2)
	stap = append(stap, pps...)

	data, keyframe, err := h264Depacketizer{}.frame([][]byte{stap, idrStart, idrEnd})
	if err != nil || !keyframe {
		t.Fatalf("frame: keyframe %v, err %v", keyframe, err)
	}
	want := []byte{0, 0, 0, 1, 0x67, 1, 2, 0, 0, 0, 1, 0x68, 3, 0, 0, 0, 1, 0x65, 0xAA, 0xBB}
	if !bytes.Equal(data, want) {
		t.Fatalf("frame = % x, want % x", data, want)
	}

	data, keyframe, err = h264Depacketizer{}.frame([][]byte{{0x41, 9}})
	if err != nil || keyframe || !bytes.Equal(data, []byte{0, 0, 0, 1, 0x41, 9}) {
		t.Fatalf("single NAL: % x %v %v", data, keyframe, err)
	}

	if _, _, err := (h264Depacketizer{}).frame([][]byte{{0x18, 0, 9, 1}}); err == nil {
		t.Fatal("a STAP-A with a NAL unit longer than the packet was taken")
	}
}

func TestVP8Depacketizer(t *testing.T) {
	// X with I set and a 15 bit picture ID, S set, then a key frame tag.
	first := []byte{0x90, 0x80, 0x81, 0x02, 0x10, 'a'}
	rest := []byte{0x80, 0x80, 0x81, 0x02, 'b'}
	data, keyframe, err := vp8Depacketizer{}.frame([][]byte{first, rest})
	if err != nil || !keyframe || !bytes.Equal(data, []byte{0x10, 'a', 'b'}) {
		t.Fatalf("frame = % x %v %v", data, keyframe, err)
	}

	data, keyframe, err = vp8Depacketizer{}.frame([][]byte{{0x10, 0x01, 'c'}})
	if err != nil || keyframe || !bytes.Equal(data, []byte{0x01, 'c'}) {
		t.Fatalf("inter frame = % x %v %v", data, keyframe, err)
	}
}

func TestIVFWriter(t *testing.T) {
	var buf bytes.Buffer
	w := ivfWriter{w: &buf}
	if err := w.writeFrame([]byte("ab"), 90000); err != nil {
		t.Fatal(err)
	}
	if err := w.writeFrame([]byte("c"), 93000); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if string(out[:4]) != "DKIF" || string(out[8:12]) != "VP80" || len(out) != 32+12+2+12+1 {
		t.Fatalf("ivf = % x", out)
	}
	if binary.LittleEndian.Uint32(out[32:]) != 2 || binary.LittleEndian.Uint64(out[36:]) != 90000 {
		t.Fatalf("frame header = % x", out[32:44])
	}
}
