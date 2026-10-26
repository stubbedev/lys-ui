package headlesstest

import (
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// WireFrame is one request as it hit a WireServer: object id, opcode,
// and the payload words.
type WireFrame struct {
	Obj    uint32
	Opcode uint32
	Body   []byte
}

// Word reads payload word i as int32.
func (f WireFrame) Word(i int) int32 {
	return intconv.Int32Bits(binary.NativeEndian.Uint32(f.Body[4*i : 4*i+4]))
}

// WireServer listens on a LoopbackDisplay and drains every request the
// first client sends into the returned channel, so a test pins the
// bytes a binding actually writes. The wayland connection is
// endianness-native, so the decoder is too. The listener closes at
// cleanup.
func WireServer(t *testing.T, display string) <-chan WireFrame {
	t.Helper()
	ln, _ := wltest.LoopbackDisplay(t, display)
	t.Cleanup(func() { _ = ln.Close() })

	frames := make(chan WireFrame, 64)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			var head [8]byte
			if _, err := io.ReadFull(conn, head[:]); err != nil {
				return
			}
			size := int(binary.NativeEndian.Uint32(head[4:8]) >> 16)
			body := make([]byte, size-8)
			if _, err := io.ReadFull(conn, body); err != nil {
				return
			}
			frames <- WireFrame{
				Obj:    binary.NativeEndian.Uint32(head[0:4]),
				Opcode: binary.NativeEndian.Uint32(head[4:8]) & 0xffff,
				Body:   body,
			}
		}
	}()
	return frames
}

// WaitForFrame consumes frames until one matches, failing the test
// after two seconds; the buffered noise passes through.
func WaitForFrame(t *testing.T, frames <-chan WireFrame, name string, match func(WireFrame) bool) WireFrame {
	t.Helper()
	for {
		select {
		case f := <-frames:
			if match(f) {
				return f
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no %s request reached the wire", name)
		}
	}
}
