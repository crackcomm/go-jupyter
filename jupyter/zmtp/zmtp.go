// Package zmtp implements the ZMTP 3.0/3.1 protocol for message framing and handshake.
package zmtp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

// ZMTP Frame flags per RFC 23/ZMTP specification.
const (
	FlagMore    byte = 0x01
	FlagLong    byte = 0x02
	FlagCommand byte = 0x04
)

// Limits to prevent OOM and Denial of Service (DoS) attacks.
const (
	MaxFrameSize       uint64 = 64 * 1024 * 1024 // 64 MB single frame payload limit
	MaxMessageSize     uint64 = 64 * 1024 * 1024 // 64 MB total message payload limit
	MaxMultipartFrames int    = 1024             // Max frames per multipart message
)

var (
	ErrFrameTooLarge    = errors.New("zmtp: frame size exceeds maximum allowed limit")
	ErrMessageTooLarge  = errors.New("zmtp: total message size exceeds maximum allowed limit")
	ErrTooManyFrames    = errors.New("zmtp: multipart message exceeds maximum frame count")
	ErrInvalidFrame     = errors.New("zmtp: invalid frame header or reserved flags set")
	ErrInvalidGreeting  = errors.New("zmtp: invalid peer greeting signature or version")
	ErrInvalidMechanism = errors.New("zmtp: peer mechanism mismatch, expected NULL")
	ErrUnexpectedFrame  = errors.New("zmtp: unexpected command or frame sequence")
)

// Frame represents a ZMTP 3.0/3.1 message or command frame.
type Frame struct {
	Data      []byte
	HasMore   bool
	IsCommand bool
}

// Pre-constructed 64-octet ZMTP 3.0 NULL-mechanism greeting template.
var nullGreetingTemplate = [64]byte{
	0: 0xff, 8: 0x01, 9: 0x7f, 10: 0x03, 11: 0x00,
	12: 'N', 13: 'U', 14: 'L', 15: 'L',
}

// PerformHandshake performs ZMTP 3.0 greeting and READY command exchange.
func PerformHandshake(conn net.Conn, socketType string) error {
	// 1. Send Local Greeting
	if _, err := conn.Write(nullGreetingTemplate[:]); err != nil {
		return fmt.Errorf("write greeting: %w", err)
	}

	// 2. Read & Validate Peer Greeting
	var peerGreeting [64]byte
	if _, err := io.ReadFull(conn, peerGreeting[:]); err != nil {
		return fmt.Errorf("read greeting: %w", err)
	}
	// Per RFC 23: Octet 0 must be 0xFF, Octet 9 must be 0x7F, Octet 10 must be major version >= 3.
	// Octets 1-8 are padding bytes used for legacy peer detection and should be ignored.
	if peerGreeting[0] != 0xff || peerGreeting[9] != 0x7f || peerGreeting[10] < 3 {
		return ErrInvalidGreeting
	}
	if !bytes.Equal(peerGreeting[12:32], nullGreetingTemplate[12:32]) {
		return ErrInvalidMechanism
	}

	// 3. Send Local READY Command
	readyCmd, err := makeReadyCommand(socketType)
	if err != nil {
		return fmt.Errorf("make ready command: %w", err)
	}
	if _, err := conn.Write(readyCmd); err != nil {
		return fmt.Errorf("write ready command: %w", err)
	}

	// 4. Read & Validate Peer READY Command
	frame, err := readFrame(conn)
	if err != nil {
		return fmt.Errorf("read peer ready command: %w", err)
	}
	if !frame.IsCommand || !bytes.HasPrefix(frame.Data, []byte("\x05READY")) {
		return ErrUnexpectedFrame
	}

	return nil
}

// ReadMultipart reads all message frames belonging to a single ZMTP multipart message.
// Automatically handles and replies to ZMTP 3.1 PING heartbeat frames sent by libzmq/ipykernel.
func ReadMultipart(rw io.ReadWriter) ([][]byte, error) {
	frames := make([][]byte, 0, 4)
	var (
		totalSize   uint64
		totalFrames int
	)

	for {
		totalFrames++
		if totalFrames > MaxMultipartFrames {
			return nil, ErrTooManyFrames
		}

		frame, err := readFrame(rw)
		if err != nil {
			return nil, err
		}

		if frame.IsCommand {
			if len(frames) > 0 {
				return nil, ErrUnexpectedFrame // Command mid-message is invalid
			}
			// Handle ZMTP 3.1 PING control command from libzmq
			if bytes.HasPrefix(frame.Data, []byte("\x04PING")) {
				var pingBody []byte
				if len(frame.Data) > 5 {
					pingBody = frame.Data[5:]
				}
				if err := sendPong(rw, pingBody); err != nil {
					return nil, fmt.Errorf("reply pong: %w", err)
				}
			}
			continue // Skip standalone control commands received before payload
		}

		totalSize += uint64(len(frame.Data))
		if totalSize > MaxMessageSize {
			return nil, ErrMessageTooLarge
		}

		frames = append(frames, frame.Data)
		if !frame.HasMore {
			return frames, nil
		}
	}
}

// SendMultipart sends frames as a single message using vector I/O (writev).
func SendMultipart(conn net.Conn, frames [][]byte) error {
	if len(frames) == 0 {
		return nil
	}
	if len(frames) > MaxMultipartFrames {
		return ErrTooManyFrames
	}

	buffers := make(net.Buffers, 0, len(frames)*2)
	for i, f := range frames {
		if uint64(len(f)) > MaxFrameSize {
			return ErrFrameTooLarge
		}
		isLast := i == len(frames)-1
		hdr := encodeHeader(len(f), !isLast, false)
		buffers = append(buffers, hdr, f)
	}

	_, err := buffers.WriteTo(conn)
	return err
}

// SendSubscription sends a ZMTP SUB subscription frame required by PUB/SUB sockets (e.g. iopub).
// Pass prefix="" to subscribe to all published messages.
func SendSubscription(conn net.Conn, prefix string) error {
	subFrame := append([]byte{0x01}, []byte(prefix)...)
	return SendMultipart(conn, [][]byte{subFrame})
}

// readFrame reads a single ZMTP frame safely with frame size bounds checking.
func readFrame(r io.Reader) (Frame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}

	flag := hdr[0]
	// RFC 23 Section 2.1: Bits 3-7 are reserved and MUST be ignored when read.
	isCommand := (flag & FlagCommand) != 0
	hasMore := (flag & FlagMore) != 0
	isLong := (flag & FlagLong) != 0

	if isCommand && hasMore {
		return Frame{}, ErrInvalidFrame // MORE bit must be 0 for commands
	}

	var size uint64
	if isLong {
		var szBuf [8]byte
		szBuf[0] = hdr[1]
		if _, err := io.ReadFull(r, szBuf[1:]); err != nil {
			return Frame{}, err
		}
		size = binary.BigEndian.Uint64(szBuf[:])
	} else {
		size = uint64(hdr[1])
	}

	if size > MaxFrameSize {
		return Frame{}, ErrFrameTooLarge
	}

	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return Frame{}, err
	}

	return Frame{Data: data, HasMore: hasMore, IsCommand: isCommand}, nil
}

// makeReadyCommand constructs the ZMTP READY command frame for socket setup.
func makeReadyCommand(socketType string) ([]byte, error) {
	if len(socketType) > 255 {
		return nil, errors.New("zmtp: socket type length exceeds 255 bytes")
	}

	// Payload format: 5"READY" + 11"Socket-Type" + uint32(len) + socketType
	body := []byte("\x05READY\x0bSocket-Type")
	body = binary.BigEndian.AppendUint32(body, uint32(len(socketType)))
	body = append(body, socketType...)

	hdr := encodeHeader(len(body), false, true)
	return append(hdr, body...), nil
}

// sendPong constructs and sends a ZMTP 3.1 PONG command reply to keep connections alive.
func sendPong(w io.Writer, pingPayload []byte) error {
	body := append([]byte("\x04PONG"), pingPayload...)
	hdr := encodeHeader(len(body), false, true)
	_, err := w.Write(append(hdr, body...))
	return err
}

// encodeHeader generates a 2-byte or 9-byte ZMTP frame header.
func encodeHeader(length int, hasMore, isCommand bool) []byte {
	var flag byte
	if hasMore {
		flag |= FlagMore
	}
	if isCommand {
		flag |= FlagCommand
	}

	if length > 255 {
		b := make([]byte, 9)
		b[0] = flag | FlagLong
		binary.BigEndian.PutUint64(b[1:], uint64(length))
		return b
	}
	return []byte{flag, byte(length)}
}
