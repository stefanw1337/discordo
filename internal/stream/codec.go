package stream

import (
	"encoding/binary"
	"errors"
	"io"
)

// Codecs the stream can be received in, by the names Discord gives them.
const (
	codecH264 = "H264"
	codecVP8  = "VP8"
	codecOpus = "opus"
)

// depacketizer turns the payloads of the packets of a frame into the frame, as the encoder made it.
type depacketizer interface {
	// frame returns the frame that payloads, in order, carry, and whether it is a keyframe, which a decoder can
	// start from.
	frame(payloads [][]byte) (data []byte, keyframe bool, err error)
}

var errBadPayload = errors.New("stream: malformed payload")

// annexBStartCode goes before each NAL unit of an H.264 frame in Annex B form, which is what Discord's senders
// encrypt end to end and what mpv reads.
var annexBStartCode = []byte{0, 0, 0, 1}

// h264Depacketizer reads H.264 as RTP carries it (RFC 6184): single NAL units, STAP-A aggregates and FU-A fragments.
type h264Depacketizer struct{}

const (
	h264NALIDR   = 5
	h264NALSPS   = 7
	h264NALSTAPA = 24
	h264NALFUA   = 28
)

func (h264Depacketizer) frame(payloads [][]byte) ([]byte, bool, error) {
	var out []byte
	keyframe := false
	// nal tells of a NAL unit that starts, for keyframes to be told by its type.
	nal := func(header byte) {
		if t := header & 0x1F; t == h264NALIDR || t == h264NALSPS {
			keyframe = true
		}
	}

	for _, payload := range payloads {
		if len(payload) < 1 {
			return nil, false, errBadPayload
		}

		switch t := payload[0] & 0x1F; {
		case t >= 1 && t <= 23:
			nal(payload[0])
			out = append(out, annexBStartCode...)
			out = append(out, payload...)
		case t == h264NALSTAPA:
			for rest := payload[1:]; len(rest) > 0; {
				if len(rest) < 2 {
					return nil, false, errBadPayload
				}
				size := int(binary.BigEndian.Uint16(rest))
				rest = rest[2:]
				if size == 0 || size > len(rest) {
					return nil, false, errBadPayload
				}
				nal(rest[0])
				out = append(out, annexBStartCode...)
				out = append(out, rest[:size]...)
				rest = rest[size:]
			}
		case t == h264NALFUA:
			if len(payload) < 2 {
				return nil, false, errBadPayload
			}
			indicator, header := payload[0], payload[1]
			// The first fragment starts the NAL unit, whose header is put back together from the two bytes.
			if header&0x80 != 0 {
				reconstructed := indicator&0xE0 | header&0x1F
				nal(reconstructed)
				out = append(out, annexBStartCode...)
				out = append(out, reconstructed)
			}
			out = append(out, payload[2:]...)
		default:
			return nil, false, errBadPayload
		}
	}
	return out, keyframe, nil
}

// vp8Depacketizer reads VP8 as RTP carries it (RFC 7741): a payload descriptor before each part of the frame.
type vp8Depacketizer struct{}

func (vp8Depacketizer) frame(payloads [][]byte) ([]byte, bool, error) {
	var out []byte
	keyframe := false
	for i, payload := range payloads {
		data, start, err := vp8Payload(payload)
		if err != nil {
			return nil, false, err
		}
		// The frame tag at the start of the frame has the inverse key frame bit.
		if i == 0 && start && len(data) > 0 && data[0]&0x01 == 0 {
			keyframe = true
		}
		out = append(out, data...)
	}
	return out, keyframe, nil
}

// vp8Payload returns the part of a frame after the payload descriptor, and whether it is the start of a partition.
func vp8Payload(payload []byte) (data []byte, start bool, err error) {
	if len(payload) < 1 {
		return nil, false, errBadPayload
	}
	descriptor := payload[0]
	start = descriptor&0x10 != 0
	i := 1
	if descriptor&0x80 != 0 {
		if len(payload) < 2 {
			return nil, false, errBadPayload
		}
		extension := payload[1]
		i = 2
		if extension&0x80 != 0 { // I: picture ID, of 15 bits if its first bit is set.
			if len(payload) <= i {
				return nil, false, errBadPayload
			}
			if payload[i]&0x80 != 0 {
				i += 2
			} else {
				i++
			}
		}
		if extension&0x40 != 0 { // L: TL0PICIDX.
			i++
		}
		if extension&0x30 != 0 { // T or K: TID, Y and KEYIDX.
			i++
		}
	}
	if len(payload) < i {
		return nil, false, errBadPayload
	}
	return payload[i:], start, nil
}

// ivfWriter writes VP8 frames in the IVF container, which mpv reads from a pipe.
type ivfWriter struct {
	w      io.Writer
	header bool
}

// writeFrame writes frame, decoded at timestamp on the 90 kHz clock of RTP video.
func (w *ivfWriter) writeFrame(frame []byte, timestamp uint32) error {
	if !w.header {
		var header [32]byte
		copy(header[0:4], "DKIF")
		binary.LittleEndian.PutUint16(header[6:8], 32)
		copy(header[8:12], "VP80")
		// The size is read from the frames themselves.
		binary.LittleEndian.PutUint32(header[16:20], 90000)
		binary.LittleEndian.PutUint32(header[20:24], 1)
		if _, err := w.w.Write(header[:]); err != nil {
			return err
		}
		w.header = true
	}

	var header [12]byte
	binary.LittleEndian.PutUint32(header[0:4], uint32(len(frame)))
	binary.LittleEndian.PutUint64(header[4:12], uint64(timestamp))
	if _, err := w.w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.w.Write(frame)
	return err
}
