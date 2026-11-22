package stream

import (
	"encoding/binary"
	"errors"
	"time"
)

// rtpHeaderSize is the size of an RTP header without CSRCs or an extension.
const rtpHeaderSize = 12

// errShortPacket is returned for a packet too short for what its header says it has.
var errShortPacket = errors.New("stream: packet too short")

// rtpPacket is an RTP packet whose payload was decrypted.
type rtpPacket struct {
	payloadType uint8
	marker      bool
	sequence    uint16
	timestamp   uint32
	ssrc        uint32
	payload     []byte
}

// rtpHeaderLength returns the length of the header of packet that is authenticated but not encrypted in Discord's
// rtpsize modes: the fixed header, the CSRCs and the 4 byte preamble of the extension if there is one, and the
// length of the extension's elements, which are encrypted with the payload.
func rtpHeaderLength(packet []byte) (header, extension int, err error) {
	if len(packet) < rtpHeaderSize {
		return 0, 0, errShortPacket
	}

	header = rtpHeaderSize + 4*int(packet[0]&0x0F)
	if packet[0]&0x10 != 0 {
		if len(packet) < header+4 {
			return 0, 0, errShortPacket
		}
		extension = 4 * int(binary.BigEndian.Uint16(packet[header+2:header+4]))
		header += 4
	}
	if len(packet) < header {
		return 0, 0, errShortPacket
	}
	return header, extension, nil
}

// parseRTP returns the packet whose clear header is header and whose decrypted body, extension elements first, is body.
func parseRTP(header []byte, extension int, body []byte) (rtpPacket, error) {
	if len(body) < extension {
		return rtpPacket{}, errShortPacket
	}
	payload := body[extension:]

	// The padding bit means the last byte is the number of bytes of padding.
	if header[0]&0x20 != 0 && len(payload) > 0 {
		padding := int(payload[len(payload)-1])
		if padding > len(payload) {
			return rtpPacket{}, errShortPacket
		}
		payload = payload[:len(payload)-padding]
	}

	return rtpPacket{
		payloadType: header[1] & 0x7F,
		marker:      header[1]&0x80 != 0,
		sequence:    binary.BigEndian.Uint16(header[2:4]),
		timestamp:   binary.BigEndian.Uint32(header[4:8]),
		ssrc:        binary.BigEndian.Uint32(header[8:12]),
		payload:     payload,
	}, nil
}

// unwrapRTX returns the packet that an RTX packet repairs: its payload starts with the sequence number of the
// packet it is a copy of, which is sent with ssrc and payloadType.
func unwrapRTX(packet rtpPacket, ssrc uint32, payloadType uint8) (rtpPacket, error) {
	if len(packet.payload) < 2 {
		return rtpPacket{}, errShortPacket
	}
	packet.sequence = binary.BigEndian.Uint16(packet.payload)
	packet.payload = packet.payload[2:]
	packet.ssrc, packet.payloadType = ssrc, payloadType
	return packet, nil
}

// seqLess reports whether sequence number a comes before b, wrapping around.
func seqLess(a, b uint16) bool {
	return a != b && b-a < 0x8000
}

// jitterBuffer puts packets back in order and gathers them into frames: the packets with the same timestamp, the
// last of which has the marker bit. A frame is let out once all of its packets are in. A missing packet is given up
// on once it is maxGapWait late, or once maxFramesBehind whole frames or more packets than the buffer's depth wait
// behind it, so that a lost packet holds the video up for no longer than a reordered one could come in.
type jitterBuffer struct {
	// now returns the time, which tests set.
	now func() time.Time
	// gapSince is when the frame at next was first found incomplete with packets after it, or zero.
	gapSince time.Time

	// depth is how many packets after a gap are awaited before the gap is given up on.
	depth int
	// emitted reports whether a frame was let out, before which a packet older than the first one in may still
	// start the stream.
	emitted bool

	packets map[uint16]rtpPacket
	// next is the sequence number of the packet expected next, valid once started.
	next    uint16
	started bool
	// lost reports whether a packet was given up on since the last frame let out, so that the frame that comes next
	// may be missing a part and a keyframe should be asked for.
	lost bool
}

func newJitterBuffer(depth int) *jitterBuffer {
	return &jitterBuffer{depth: depth, packets: make(map[uint16]rtpPacket), now: time.Now}
}

// frame is the payloads of the packets of a frame, in order, and whether one of the packets before it was lost.
type frame struct {
	timestamp uint32
	payloads  [][]byte
	// afterLoss reports whether packets were lost before the frame, which a decoder cannot make sense of until
	// the next keyframe.
	afterLoss bool
}

// push adds packet and returns the frames it completes, oldest first.
func (j *jitterBuffer) push(packet rtpPacket) []frame {
	if !j.started {
		j.next, j.started = packet.sequence, true
	}
	// Packets can come out of order from the start, so until a frame is let out the stream starts at the oldest.
	if !j.emitted && seqLess(packet.sequence, j.next) {
		j.next = packet.sequence
	}
	// A packet that comes after its turn passed, or again, is dropped.
	if seqLess(packet.sequence, j.next) {
		return nil
	}
	if _, ok := j.packets[packet.sequence]; ok {
		return nil
	}
	packet.payload = append([]byte(nil), packet.payload...)
	j.packets[packet.sequence] = packet

	var frames []frame
	for {
		if f, ok := j.take(); ok {
			frames = append(frames, f)
			continue
		}
		if len(j.packets) == 0 {
			j.gapSince = time.Time{}
			return frames
		}
		// Too much waits behind a gap, or for too long: give up on what is missing, and start again at the oldest
		// one held.
		now := j.now()
		if j.gapSince.IsZero() {
			j.gapSince = now
		}
		if now.Sub(j.gapSince) < maxGapWait && len(j.packets) <= j.depth && j.framesWaiting() < maxFramesBehind {
			return frames
		}
		j.gapSince = time.Time{}
		j.skipGap()
	}
}

const (
	// maxGapWait is how long a missing packet is waited for, about two frames of a 30 FPS stream.
	maxGapWait = 70 * time.Millisecond
	// maxFramesBehind is how many whole frames may wait behind a missing packet before it is given up on.
	maxFramesBehind = 8
)

// framesWaiting returns how many frames held are whole, which their last packet, with the marker, tells.
func (j *jitterBuffer) framesWaiting() int {
	n := 0
	for _, packet := range j.packets {
		if packet.marker {
			n++
		}
	}
	return n
}

// take lets out the frame that starts at next if all of it is in.
func (j *jitterBuffer) take() (frame, bool) {
	first, ok := j.packets[j.next]
	if !ok {
		return frame{}, false
	}

	var payloads [][]byte
	for seq := j.next; ; seq++ {
		packet, ok := j.packets[seq]
		if !ok || packet.timestamp != first.timestamp {
			return frame{}, false
		}
		payloads = append(payloads, packet.payload)
		if packet.marker {
			for s := j.next; s != seq+1; s++ {
				delete(j.packets, s)
			}
			j.next = seq + 1
			f := frame{timestamp: first.timestamp, payloads: payloads, afterLoss: j.lost}
			j.lost, j.emitted, j.gapSince = false, true, time.Time{}
			return f, true
		}
	}
}

// skipGap drops the packets of the frame that cannot be completed and moves next to the first packet of the frame
// after it.
func (j *jitterBuffer) skipGap() {
	oldest, found := uint16(0), false
	for seq := range j.packets {
		if !found || seqLess(seq, oldest) {
			oldest, found = seq, true
		}
	}
	if !found {
		return
	}
	j.lost = true

	// The oldest packet held may be the middle of a frame whose start was lost: skip to the frame after it.
	start := oldest
	first := j.packets[oldest]
	for seq := oldest; ; seq++ {
		packet, ok := j.packets[seq]
		if !ok || packet.timestamp != first.timestamp {
			break
		}
		start = seq + 1
		if packet.marker {
			break
		}
	}
	if start == oldest+1 && !j.packets[oldest].marker {
		start = oldest
	}
	// Drop whatever came before the new start.
	for seq := range j.packets {
		if seqLess(seq, start) {
			delete(j.packets, seq)
		}
	}
	if start == oldest && j.next == oldest {
		// Nothing would change: drop the oldest packet so that the buffer moves on.
		delete(j.packets, oldest)
		start = oldest + 1
	}
	j.next = start
}
