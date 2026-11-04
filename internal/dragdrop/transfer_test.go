package dragdrop

import (
	"bytes"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/xfer"
	"github.com/stubbedev/lys-ui/transfer"
)

// capturingOffer plays a foreign drag offer and hands a test the write
// end Receive was given, so it can attach a peer while the end is open.
type capturingOffer struct {
	fakeOffer
	onReceive func(fd uintptr)
}

func (c *capturingOffer) Receive(mimeType string, fd uintptr) error {
	c.onReceive(fd)
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
func rawWriteEnd(t *testing.T) (*os.File, uintptr) {
	t.Helper()
	var fds [2]int
	if err := syscall.Pipe2(fds[:], 0); err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(fds[0]), "dragdrop-test-raw-r"), uintptr(fds[1])
}

// receiveResult is one delivered drop payload.
type receiveResult struct {
	data []byte
	err  error
}

// awaitReceive reads through the controller's receive path and bounds
// the wait for its delivery.
func awaitReceive(t *testing.T, c *Controller, offer offerAPI, mime string) receiveResult {
	t.Helper()
	ch := make(chan receiveResult, 1)
	c.receive(offer, mime, func(data []byte, err error) { ch <- receiveResult{data, err} })
	select {
	case r := <-ch:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("the drop payload never delivered")
		return receiveResult{}
	}
}

// A cross-process drop keeps working for an honest source, and a
// successful transfer still finishes the drag transaction.
func TestReceivePayloadRoundtripsASmallDrop(t *testing.T) {
	var wait func() error
	offer := &capturingOffer{onReceive: func(fd uintptr) { wait = peerStream(t, fd, []byte("dropped text")) }}
	r := awaitReceive(t, newTestController(), offer, "text/plain")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if string(r.data) != "dropped text" {
		t.Errorf("payload = %q, want the source's payload", r.data)
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
	payload := bytes.Repeat([]byte{0xD5}, xfer.MaxPayload+64)
	var wait func() error
	offer := &capturingOffer{onReceive: func(fd uintptr) { wait = peerStream(t, fd, payload) }}
	r := awaitReceive(t, newTestController(), offer, "text/plain")
	if !errors.Is(r.err, xfer.ErrTooLarge) {
		t.Fatalf("read = %v, want xfer.ErrTooLarge", r.err)
	}
	if r.data != nil {
		t.Errorf("payload = %d bytes, want none: a capped drop is refused whole", len(r.data))
	}
	if offer.finished != 0 {
		t.Error("a truncated transfer was finished")
	}
	if err := wait(); err != nil {
		t.Errorf("source blocked or errored past the cap: %v", err)
	}
}

// A source that never writes leaves the drop pending, never the caller:
// the deadline ends it, unfinished.
func TestReceivePayloadCutsOffASilentSource(t *testing.T) {
	shortTransferTimeout(t)
	offer := &capturingOffer{onReceive: func(fd uintptr) { holdSilentPeer(t, fd) }}
	r := awaitReceive(t, newTestController(), offer, "text/plain")
	if !errors.Is(r.err, xfer.ErrTimeout) {
		t.Fatalf("read = %v, want xfer.ErrTimeout", r.err)
	}
	if offer.finished != 0 {
		t.Error("a timed-out transfer was finished")
	}
}

// A drop's read returns with the transfer running, whatever the source
// does, and the offer finishes only once the bytes are in. Stop ends it
// without its deadline.
func TestDropReturnsBeforeASilentSource(t *testing.T) {
	c := newTestController()
	offer := &capturingOffer{onReceive: func(fd uintptr) { holdSilentPeer(t, fd) }}
	c.dragOffer, c.accepted = offer, "text/plain"
	delivered := make(chan error, 1)
	c.ReadPayload("text/plain", func(_ []byte, err error) { delivered <- err })
	select {
	case err := <-delivered:
		t.Fatalf("a silent source's drop delivered %v, want it pending", err)
	default:
	}
	c.Stop()
	if err := <-delivered; err == nil {
		t.Error("a stopped transfer delivered no error")
	}
	if offer.finished != 0 {
		t.Error("a stopped transfer was finished")
	}
}

// A consumer that never reads holds a transfer goroutine, never the
// handler; Stop ends the write.
func TestSourceSendToAStuckConsumerIsBounded(t *testing.T) {
	c := newTestController()
	c.srcData = transfer.Drag{Mimes: []string{"text/plain"}, Write: fakeSourceWriter(bytes.Repeat([]byte("d"), 1<<20))}
	r, w := rawWriteEnd(t)
	defer r.Close()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: "text/plain", Fd: w})
	c.Stop()
}
