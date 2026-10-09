// Package discord publishes a presence to the local Discord client over its
// IPC socket. It needs only an application id. There is no bot token, Gateway
// connection, or OAuth.
package discord

import (
	"encoding/binary"
	"fmt"
	"io"
)

// IPC opcodes. Every frame is a little endian opcode and length, then JSON.
const (
	opHandshake uint32 = 0
	opFrame     uint32 = 1
	opClose     uint32 = 2
	opPing      uint32 = 3
	opPong      uint32 = 4
)

const (
	headerSize = 8
	// maxFrame bounds what a misbehaving peer can make us allocate. Real
	// frames, READY included, are a few kilobytes.
	maxFrame = 1 << 20
)

// writeFrame sends header and payload in one write, so a concurrent pong
// cannot land between them.
func writeFrame(w io.Writer, op uint32, payload []byte) error {
	buf := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint32(buf[0:4], op)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(payload)))
	copy(buf[headerSize:], payload)
	_, err := w.Write(buf)
	return err
}

func readFrame(r io.Reader) (op uint32, payload []byte, err error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	op = binary.LittleEndian.Uint32(header[0:4])
	size := binary.LittleEndian.Uint32(header[4:8])
	if size > maxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes exceeds the %d byte limit", size, maxFrame)
	}
	payload = make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return op, payload, nil
}
