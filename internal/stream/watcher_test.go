package stream

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/voice/udp"
	"github.com/disgoorg/godave"
)

func TestVideoEventPrimary(t *testing.T) {
	for _, test := range []struct {
		name      string
		event     videoEvent
		ssrc, rtx uint32
	}{
		{"best active stream", videoEvent{VideoSSRC: 1, Streams: []streamInfo{{SSRC: 10, RTXSSRC: 11, Quality: 50, Active: true}, {SSRC: 20, RTXSSRC: 21, Quality: 100, Active: true}, {SSRC: 30, Quality: 100}}}, 20, 21},
		{"top level", videoEvent{VideoSSRC: 5, RTXSSRC: 9}, 5, 9},
		{"rtx after primary", videoEvent{Streams: []streamInfo{{SSRC: 40, Active: true, Quality: 100}}}, 40, 41},
		{"none", videoEvent{}, 0, 0},
	} {
		if ssrc, rtx := test.event.primary(); ssrc != test.ssrc || rtx != test.rtx {
			t.Errorf("%s: primary = %d %d, want %d %d", test.name, ssrc, rtx, test.ssrc, test.rtx)
		}
	}
}

func TestChooseMode(t *testing.T) {
	if mode, err := chooseMode([]string{"xsalsa20_poly1305", udp.ModeXChaCha20Poly1305, udp.ModeAES256GCM}); err != nil || mode != udp.ModeAES256GCM {
		t.Fatalf("chooseMode = %q %v, want AES-GCM", mode, err)
	}
	if _, err := chooseMode([]string{"xsalsa20_poly1305"}); err == nil {
		t.Fatal("an unsupported mode was chosen")
	}
}

// udpPair returns a transport with key, and the connection it talks to as the voice server would.
func udpPair(t *testing.T, key [32]byte) (*transport, net.PacketConn) {
	t.Helper()
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	conn, err := net.Dial("udp", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	tr := &transport{conn: conn, ssrc: 77, closed: make(chan struct{})}
	t.Cleanup(func() { tr.close() })
	if err := tr.useKey(udp.ModeAES256GCM, key); err != nil {
		t.Fatal(err)
	}
	return tr, server
}

func TestTransport(t *testing.T) {
	var key [32]byte
	key[0] = 7
	block, _ := aes.NewCipher(key[:])
	aead, _ := cipher.NewGCM(block)
	tr, server := udpPair(t, key)

	t.Run("reads RTP as Discord encrypts it", func(t *testing.T) {
		// Prime the server with the client's address.
		if _, err := tr.conn.Write([]byte{0}); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 64)
		_, client, err := server.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}

		header := rtpHeader(payloadH264, true, 3, 900, 20, true)
		// The extension's elements are encrypted with the payload.
		body := append([]byte{0x10, 0, 0, 0}, 0x41, 0xAB)
		var nonce [12]byte
		binary.BigEndian.PutUint32(nonce[:4], 5)
		packet := aead.Seal(append([]byte(nil), header...), nonce[:], body, header)
		packet = append(packet, nonce[:4]...)
		// RTCP and pings before it are skipped.
		server.WriteTo([]byte{0x13, 0x37, 0xF0, 0x0D, 0, 0, 0, 0}, client)
		server.WriteTo(append([]byte{0x80, 200, 0, 1}, make([]byte, 12)...), client)
		server.WriteTo(packet, client)

		got, err := tr.readRTP(make([]byte, maxPacketSize))
		if err != nil || got.ssrc != 20 || got.sequence != 3 || !got.marker || string(got.payload) != string([]byte{0x41, 0xAB}) {
			t.Fatalf("readRTP = %+v %v", got, err)
		}
	})

	t.Run("asks for a keyframe", func(t *testing.T) {
		if err := tr.sendPLI(20); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 64)
		server.SetReadDeadline(time.Now().Add(time.Second))
		n, _, err := server.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		packet := buf[:n]
		if !isRTCP(packet) || packet[1] != 206 || packet[0]&0x1F != 1 || binary.BigEndian.Uint32(packet[4:8]) != 77 {
			t.Fatalf("PLI header = % x", packet[:8])
		}
		var nonce [12]byte
		copy(nonce[:4], packet[n-4:])
		body, err := aead.Open(nil, nonce[:], packet[8:n-4], packet[:8])
		if err != nil || binary.BigEndian.Uint32(body) != 20 {
			t.Fatalf("PLI body = % x %v, want media SSRC 20", body, err)
		}
	})
}

func TestReceiverPlays(t *testing.T) {
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("no cat to play into")
	}
	w := &Watcher{player: PlayerConfig{Path: cat}, updates: make(chan struct{}, 1), status: Status{Key: "call:1:2", Watching: true}}
	identify := identifyCommand{UserID: 1, ChannelID: 1}
	r := &receiver{
		watcher: w, key: "call:1:2", owner: 2, needKeyframe: true,
		gateway: newVoiceGateway("example.invalid", identify, godave.NewNoopSession),
		ended:   make(chan struct{}), received: make(chan struct{}),
	}
	t.Cleanup(r.stop)
	r.videoSSRC.Store(20)
	r.rtxSSRC.Store(21)

	inter := rtpPacket{payloadType: payloadH264, sequence: 1, timestamp: 10, marker: true, ssrc: 20, payload: []byte{0x41, 1}}
	if err := r.handlePacket(inter); err != nil {
		t.Fatal(err)
	}
	if r.player != nil {
		t.Fatal("playing started without a keyframe")
	}

	idr := rtpPacket{payloadType: payloadH264, sequence: 2, timestamp: 20, marker: true, ssrc: 20, payload: []byte{0x65, 1}}
	if err := r.handlePacket(idr); err != nil {
		t.Fatal(err)
	}
	if r.player == nil || !w.Status().Playing || w.Status().Codec != codecH264 {
		t.Fatalf("playing did not start on a keyframe: %+v", w.Status())
	}

	t.Run("other SSRCs are not played", func(t *testing.T) {
		before := r.jitter.next
		r.handlePacket(rtpPacket{payloadType: payloadH264, sequence: 3, timestamp: 30, marker: true, ssrc: 99, payload: []byte{0x41}})
		if r.jitter.next != before {
			t.Fatal("a packet of another SSRC was taken")
		}
	})

	t.Run("RTX repairs the video", func(t *testing.T) {
		rtx := rtpPacket{payloadType: payloadH264RTX, sequence: 900, timestamp: 30, marker: true, ssrc: 21, payload: []byte{0, 3, 0x41, 2}}
		if err := r.handlePacket(rtx); err != nil {
			t.Fatal(err)
		}
		if r.jitter.next != 4 {
			t.Fatalf("next = %d, want 4 after the repaired packet", r.jitter.next)
		}
	})
}

func TestDeleteError(t *testing.T) {
	if err := deleteError(&DeleteEvent{Reason: "stream_ended"}); err != ErrStreamEnded {
		t.Fatalf("err = %v, want ErrStreamEnded", err)
	}
	if err := deleteError(&DeleteEvent{Reason: "stream_full"}); err == nil || err == ErrStreamEnded {
		t.Fatalf("err = %v, want one that says it is full", err)
	}
}

// fakeSink records the audio it is given.
type fakeSink struct {
	packets [][]byte
	stopped bool
}

func (s *fakeSink) PlayStream(_ uint32, _ discord.UserID, _ uint16, packet []byte) error {
	s.packets = append(s.packets, packet)
	return nil
}

func (s *fakeSink) StopStream() { s.stopped = true }

func TestReceiverPlaysAudio(t *testing.T) {
	sink := &fakeSink{}
	w := &Watcher{audio: sink, updates: make(chan struct{}, 1)}
	r := &receiver{
		watcher: w, key: "call:1:2", owner: 2,
		gateway: newVoiceGateway("example.invalid", identifyCommand{UserID: 1, ChannelID: 1}, godave.NewNoopSession),
		ended:   make(chan struct{}), received: make(chan struct{}),
	}
	r.audioSSRC.Store(30)

	r.handlePacket(rtpPacket{payloadType: payloadOpus, ssrc: 30, sequence: 1, payload: []byte{0xF8, 0xFF, 0xFE}})
	// Audio of anyone else is not played.
	r.handlePacket(rtpPacket{payloadType: payloadOpus, ssrc: 31, sequence: 1, payload: []byte{1}})
	if len(sink.packets) != 1 || string(sink.packets[0]) != string([]byte{0xF8, 0xFF, 0xFE}) {
		t.Fatalf("played %v, want the owner's packet", sink.packets)
	}

	r.stop()
	if !sink.stopped {
		t.Fatal("the audio did not stop with the stream")
	}
}

func TestFindPlayer(t *testing.T) {
	if got := findPlayer(`F:\Discordo\mpv.exe`); got != `F:\Discordo\mpv.exe` {
		t.Fatalf("a path was changed to %q", got)
	}
	if got := findPlayer("no-such-player-here"); got != "no-such-player-here" {
		t.Fatalf("a player found nowhere was changed to %q", got)
	}
}
