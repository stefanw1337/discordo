package stream

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ayn2op/arikawa/v3/voice/udp"
	"golang.org/x/crypto/chacha20poly1305"
)

// maxPacketSize is the largest UDP packet read. Discord's packets fit an MTU of 1500.
const maxPacketSize = 1500

// transport is the UDP connection that a stream's media comes over, encrypted with the session's key.
type transport struct {
	conn net.Conn
	ssrc uint32
	// address and port are what the voice server sees the connection come from.
	address string
	port    uint16

	mutex sync.Mutex // guards aead and sendNonce, which are set once the key is known
	aead  cipher.AEAD
	// sendNonce counts the packets sent, whose nonce it is.
	sendNonce uint32

	closed chan struct{}
	once   sync.Once
}

// dialTransport connects to the voice server at addr as ssrc and asks it which address it sees, which it is told to
// send to.
func dialTransport(ctx context.Context, addr string, ssrc uint32) (*transport, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial the stream's voice server: %w", err)
	}

	address, port, err := discoverIP(ctx, conn, ssrc)
	if err != nil {
		conn.Close()
		return nil, err
	}

	t := &transport{conn: conn, ssrc: ssrc, address: address, port: port, closed: make(chan struct{})}
	go t.keepAlive()
	return t, nil
}

// discoverIP returns the address and port that the voice server sees conn come from.
//
// https://docs.discord.food/topics/voice-connections#ip-discovery
func discoverIP(ctx context.Context, conn net.Conn, ssrc uint32) (string, uint16, error) {
	stop := context.AfterFunc(ctx, func() { conn.SetReadDeadline(time.Now()) })
	defer stop()

	var request [74]byte
	binary.BigEndian.PutUint16(request[0:2], 1)
	binary.BigEndian.PutUint16(request[2:4], 70)
	binary.BigEndian.PutUint32(request[4:8], ssrc)
	if _, err := conn.Write(request[:]); err != nil {
		return "", 0, fmt.Errorf("failed to send IP discovery: %w", err)
	}

	var response [74]byte
	if _, err := io.ReadFull(conn, response[:]); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return "", 0, fmt.Errorf("failed to read IP discovery: %w", err)
	}
	if !stop() {
		return "", 0, ctx.Err()
	}
	conn.SetReadDeadline(time.Time{})

	address := response[8:72]
	end := bytes.IndexByte(address, 0)
	if end < 0 {
		return "", 0, errors.New("IP discovery has no null terminator")
	}
	return string(address[:end]), binary.BigEndian.Uint16(response[72:74]), nil
}

// chooseMode returns the encryption mode to use of those that the voice server offers, the most preferred first.
func chooseMode(offered []string) (string, error) {
	for _, mode := range udp.Modes() {
		for _, o := range offered {
			if o == mode {
				return mode, nil
			}
		}
	}
	return "", fmt.Errorf("no supported encryption mode among %v", offered)
}

// useKey decrypts and encrypts with key in mode from now on.
func (t *transport) useKey(mode string, key [32]byte) error {
	var aead cipher.AEAD
	var err error
	switch mode {
	case udp.ModeAES256GCM:
		var block cipher.Block
		if block, err = aes.NewCipher(key[:]); err == nil {
			aead, err = cipher.NewGCM(block)
		}
	case udp.ModeXChaCha20Poly1305:
		aead, err = chacha20poly1305.NewX(key[:])
	default:
		err = fmt.Errorf("unknown encryption mode %q", mode)
	}
	if err != nil {
		return err
	}

	t.mutex.Lock()
	t.aead = aead
	t.mutex.Unlock()
	return nil
}

func (t *transport) cipher() cipher.AEAD {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.aead
}

// keepAlive pings the voice server every 5 seconds until the transport closes, so that what is between them keeps
// the connection open.
func (t *transport) keepAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	ping := []byte{0x13, 0x37, 0xCA, 0xFE, 0, 0, 0, 0}
	for seq := uint32(0); ; seq++ {
		binary.BigEndian.PutUint32(ping[4:], seq)
		if _, err := t.conn.Write(ping); err != nil {
			return
		}
		select {
		case <-ticker.C:
		case <-t.closed:
			return
		}
	}
}

func (t *transport) close() error {
	t.once.Do(func() { close(t.closed) })
	return t.conn.Close()
}

// isRTCP reports whether packet is RTCP rather than RTP: its second byte is a packet type from 192 to 223.
func isRTCP(packet []byte) bool {
	return len(packet) >= 8 && packet[0]>>6 == 2 && packet[1] >= 192 && packet[1] <= 223
}

// readRTP reads packets until one is RTP that decrypts, and returns it. RTCP, pings and packets that arrive before
// the key are skipped.
func (t *transport) readRTP(buf []byte) (rtpPacket, error) {
	var nonce [24]byte
	for {
		n, err := t.conn.Read(buf)
		if err != nil {
			return rtpPacket{}, err
		}
		packet := buf[:n]
		aead := t.cipher()
		if aead == nil || n < rtpHeaderSize+4 || packet[0]>>6 != 2 || isRTCP(packet) {
			continue
		}

		header, extension, err := rtpHeaderLength(packet[:n-4])
		if err != nil {
			continue
		}

		// The nonce's counter is sent after the packet.
		copy(nonce[:4], packet[n-4:])
		body, err := aead.Open(nil, nonce[:aead.NonceSize()], packet[header:n-4], packet[:header])
		if err != nil {
			continue
		}

		parsed, err := parseRTP(packet[:header], extension, body)
		if err != nil || len(parsed.payload) == 0 {
			continue
		}
		return parsed, nil
	}
}

// sendPLI asks the sender of mediaSSRC for a keyframe with an RTCP Picture Loss Indication (RFC 4585). Its header
// is sent in the clear and authenticated, and its body encrypted.
func (t *transport) sendPLI(mediaSSRC uint32) error {
	aead := t.cipher()
	if aead == nil {
		return errors.New("no key to encrypt with yet")
	}

	var header [8]byte
	header[0] = 0x80 | 1 // version 2, feedback message type 1: PLI
	header[1] = 206      // payload-specific feedback
	binary.BigEndian.PutUint16(header[2:4], 2)
	binary.BigEndian.PutUint32(header[4:8], t.ssrc)
	var body [4]byte
	binary.BigEndian.PutUint32(body[:], mediaSSRC)

	t.mutex.Lock()
	t.sendNonce++
	counter := t.sendNonce
	t.mutex.Unlock()

	var nonce [24]byte
	binary.BigEndian.PutUint32(nonce[:4], counter)
	packet := aead.Seal(header[:], nonce[:aead.NonceSize()], body[:], header[:])
	packet = append(packet, nonce[:4]...)
	_, err := t.conn.Write(packet)
	return err
}
