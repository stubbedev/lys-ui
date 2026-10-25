package headlesstest

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// RuntimeDir creates a private XDG_RUNTIME_DIR at the temp root and
// points the process env at it. It sits outside t.TempDir because the
// kernel caps a socket path at 107 bytes and CI's nested temp dirs push
// a t.TempDir path past that, so bind fails with EINVAL. The dir goes
// away at cleanup.
func RuntimeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "lys-ui-rt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	return dir
}

// LoopbackDisplay binds a real unix socket as a wayland display the
// test process owns: the far end for wire-level and dead-peer tests,
// no compositor involved. The socket lives in a fresh RuntimeDir and
// the process env is pointed at the display, so wl.Connect("")
// reaches the listener. The caller closes the listener.
func LoopbackDisplay(t *testing.T, display string) (*net.UnixListener, string) {
	t.Helper()
	sock := filepath.Join(RuntimeDir(t), display)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WAYLAND_DISPLAY", display)
	return ln, sock
}

// WireFrame is one request as it hit a WireServer: object id, opcode,
// and the payload words.
type WireFrame struct {
	Obj    uint32
	Opcode uint32
	Body   []byte
}

// Word reads payload word i as int32.
func (f WireFrame) Word(i int) int32 {
	return int32(binary.NativeEndian.Uint32(f.Body[4*i : 4*i+4]))
}

// WireServer listens on a LoopbackDisplay and drains every request the
// first client sends into the returned channel, so a test pins the
// bytes a binding actually writes. The wayland connection is
// endianness-native, so the decoder is too. The listener closes at
// cleanup.
func WireServer(t *testing.T, display string) <-chan WireFrame {
	t.Helper()
	ln, _ := LoopbackDisplay(t, display)
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
