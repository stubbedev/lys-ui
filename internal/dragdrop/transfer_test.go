package dragdrop

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/xfer"
	"github.com/stubbedev/lys-ui/transfer"
)

// capturingOffer plays a foreign drag offer and remembers the write
// end Receive was handed, so a test can attach a peer to it.
type capturingOffer struct {
	fakeOffer
	fd uintptr
}

func (c *capturingOffer) Receive(mimeType string, fd uintptr) error {
	c.fd = fd
	return c.receiveErr
}

// fakeSourceWriter adapts a payload into the Content provider shape.
func fakeSourceWriter(payload []byte) func(string, io.Writer) error {
	return func(_ string, w io.Writer) error {
		_, err := w.Write(payload)
		return err
	}
}

// peerStream plays the foreign source at the far end of the transfer
// pipe: it dups the descriptor (the way the compositor hands one out),
// streams payload from its own goroutine, and returns a wait func that
// reports once every byte landed without error — a bounded wait, so a
// regression that leaves the peer blocked on the pipe fails instead of
// hanging.
func peerStream(t *testing.T, fd uintptr, payload []byte) func() error {
	t.Helper()
	d, err := syscall.Dup(int(fd))
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(d), "dragdrop-test-peer")
	done := make(chan error, 1)
	go func() {
		_, werr := f.Write(payload)
		_ = f.Close()
		done <- werr
	}()
	return func() error {
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			return errors.New("peer still blocked on the pipe after 5s")
		}
	}
}

// holdSilentPeer dups fd and leaves the peer's end open but silent, so
// only the deadline can end the read; the dup closes with the test.
func holdSilentPeer(t *testing.T, fd uintptr) {
	t.Helper()
	d, err := syscall.Dup(int(fd))
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(d), "dragdrop-test-silent-peer")
	t.Cleanup(func() { _ = f.Close() })
}

// shortTransferTimeout shrinks the transfer deadline for one test.
func shortTransferTimeout(t *testing.T) {
	t.Helper()
	orig := transferTimeout
	transferTimeout = 100 * time.Millisecond
	t.Cleanup(func() { transferTimeout = orig })
}

// rawWriteEnd creates a pipe the Go runtime has never seen, so the
// send handler's DeadlineWriter gets a descriptor like the compositor
// hands out: never registered with the poller before. (An os.Pipe end
// is already registered, which the deadline arming would refuse.)
func rawWriteEnd(t *testing.T) (r, w *os.File) {
	t.Helper()
	var fds [2]int
	if err := syscall.Pipe2(fds[:], 0); err != nil {
		t.Fatal(err)
	}
	r = os.NewFile(uintptr(fds[0]), "dragdrop-test-raw-r")
	w = os.NewFile(uintptr(fds[1]), "dragdrop-test-raw-w")
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return r, w
}

// A cross-process drop keeps working for an honest source, and a
// successful transfer still finishes the drag transaction.
func TestReceivePayloadRoundtripsASmallDrop(t *testing.T) {
	offer := &capturingOffer{}
	var wait func() error
	flush := func() error {
		wait = peerStream(t, offer.fd, []byte("dropped text"))
		return nil
	}
	data, err := receivePayload(offer, "text/plain", flush, minActionVersion, transfer.ActionNone)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "dropped text" {
		t.Errorf("payload = %q, want the source's payload", data)
	}
	if offer.finished != 1 {
		t.Errorf("finish calls = %d, want 1 after a full transfer", offer.finished)
	}
	if err := wait(); err != nil {
		t.Fatal(err)
	}
}

// A source streaming past xfer.MaxPayload gets its transfer refused
// with an error, the fd is drained past the cap so the source never
// blocks, and the truncated transfer is not finished: the drag
// transaction unwinds instead of completing.
func TestReceivePayloadRefusesAnOversizeSource(t *testing.T) {
	offer := &capturingOffer{}
	payload := bytes.Repeat([]byte{0xD5}, xfer.MaxPayload+64)
	var wait func() error
	flush := func() error {
		wait = peerStream(t, offer.fd, payload)
		return nil
	}
	data, err := receivePayload(offer, "text/plain", flush, minActionVersion, transfer.ActionNone)
	if !errors.Is(err, xfer.ErrTooLarge) {
		t.Fatalf("read = %v, want xfer.ErrTooLarge", err)
	}
	if data != nil {
		t.Errorf("payload = %d bytes, want none: a capped drop is refused whole", len(data))
	}
	if offer.finished != 0 {
		t.Error("a truncated transfer was finished")
	}
	if err := wait(); err != nil {
		t.Errorf("source blocked or errored past the cap: %v", err)
	}
}

// A source that never writes must not hang the drop: the deadline cuts
// the read off, with no goroutine left behind.
func TestReceivePayloadCutsOffASilentSource(t *testing.T) {
	shortTransferTimeout(t)
	offer := &capturingOffer{}
	flush := func() error {
		holdSilentPeer(t, offer.fd)
		return nil
	}

	before := runtime.NumGoroutine()
	start := time.Now()
	if _, err := receivePayload(offer, "text/plain", flush, minActionVersion, transfer.ActionNone); !errors.Is(err, xfer.ErrTimeout) {
		t.Fatalf("read = %v, want xfer.ErrTimeout", err)
	} else if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("silent source hung the read for %s, want the short deadline", elapsed)
	}
	time.Sleep(250 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines %d -> %d: the deadline leaked one", before, after)
	}
	if offer.finished != 0 {
		t.Error("a timed-out transfer was finished")
	}
}

// A consumer that never reads must not stall the dispatch loop: the
// bounded send returns once the deadline fires, leaking nothing.
func TestSourceSendToAStuckConsumerIsBounded(t *testing.T) {
	shortTransferTimeout(t)
	c := newTestController()
	c.srcData = transfer.Drag{Mimes: []string{"text/plain"}, Write: fakeSourceWriter(bytes.Repeat([]byte("d"), 1<<20))}
	_, w := rawWriteEnd(t)

	before := runtime.NumGoroutine()
	start := time.Now()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: "text/plain", Fd: w.Fd()})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("send to a silent consumer blocked for %s, want the deadline", elapsed)
	}
	time.Sleep(250 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines %d -> %d: the bounded send leaked one", before, after)
	}
}
