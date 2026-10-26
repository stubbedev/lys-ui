package clipboard

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/xfer"
	"github.com/stubbedev/lys-ui/transfer"
)

// fakePeer plays the foreign client at the far end of a transfer pipe:
// it dups the descriptor (the way the compositor hands one out),
// streams payload from its own goroutine, and returns a wait func that
// reports once every byte landed without error — a bounded wait, so a
// regression that leaves the peer blocked on the pipe fails instead of
// hanging.
func fakePeer(t *testing.T, fd uintptr, payload []byte) func() error {
	t.Helper()
	d, err := syscall.Dup(int(fd))
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(d), "clipboard-test-peer")
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
	f := os.NewFile(uintptr(d), "clipboard-test-silent-peer")
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
	r = os.NewFile(uintptr(fds[0]), "clipboard-test-raw-r")
	w = os.NewFile(uintptr(fds[1]), "clipboard-test-raw-w")
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return r, w
}

// The offer pipeline keeps working for an honest peer.
func TestReadOfferTextRoundtripsASmallPayload(t *testing.T) {
	var wait func() error
	receive := func(mime string, fd uintptr) error {
		wait = fakePeer(t, fd, []byte("pasted text"))
		return nil
	}
	text, err := readOfferText(receive, func() error { return nil }, map[string]bool{"text/plain;charset=utf-8": true})
	if err != nil {
		t.Fatal(err)
	}
	if text != "pasted text" {
		t.Errorf("text = %q, want the peer's payload", text)
	}
	if err := wait(); err != nil {
		t.Fatal(err)
	}
}

// A peer streaming past xfer.MaxPayload gets its transfer refused with
// an error, and the fd is drained past the cap: the wait func only
// returns once the peer's whole payload landed without error, which
// takes the reader draining what it refused to keep.
func TestReadOfferTextRefusesAnOversizePeer(t *testing.T) {
	payload := bytes.Repeat([]byte{0xE5}, xfer.MaxPayload+64)
	var wait func() error
	receive := func(mime string, fd uintptr) error {
		wait = fakePeer(t, fd, payload)
		return nil
	}
	text, err := readOfferText(receive, func() error { return nil }, map[string]bool{"text/plain": true})
	if !errors.Is(err, xfer.ErrTooLarge) {
		t.Fatalf("read = %v, want xfer.ErrTooLarge", err)
	}
	if text != "" {
		t.Errorf("text = %d bytes, want none: a capped payload is refused whole", len(text))
	}
	if err := wait(); err != nil {
		t.Errorf("peer blocked or errored past the cap: %v", err)
	}
}

// A peer that never writes must not hang the paste: the deadline cuts
// the read off, with no goroutine left behind.
func TestReadOfferTextCutsOffASilentPeer(t *testing.T) {
	shortTransferTimeout(t)
	receive := func(mime string, fd uintptr) error {
		holdSilentPeer(t, fd)
		return nil
	}

	before := runtime.NumGoroutine()
	start := time.Now()
	_, err := readOfferText(receive, func() error { return nil }, map[string]bool{"text/plain": true})
	elapsed := time.Since(start)
	if !errors.Is(err, xfer.ErrTimeout) {
		t.Fatalf("read = %v, want xfer.ErrTimeout", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("silent peer hung the read for %s, want the short deadline", elapsed)
	}
	time.Sleep(250 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines %d -> %d: the deadline leaked one", before, after)
	}
}

// A consumer that never reads must not stall the dispatch loop: the
// bounded send returns once the deadline fires, leaking nothing. The
// primary selection shares sendPayload and is exercised the same way
// below.
func TestSendToAStuckConsumerIsBounded(t *testing.T) {
	shortTransferTimeout(t)
	c := &Clipboard{content: transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))} // far past the pipe buffer
	_, w := rawWriteEnd(t)

	before := runtime.NumGoroutine()
	start := time.Now()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: transfer.MimeText, Fd: w.Fd()})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("send to a silent consumer blocked for %s, want the deadline", elapsed)
	}
	time.Sleep(250 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines %d -> %d: the bounded send leaked one", before, after)
	}
}

func TestPrimarySendToAStuckConsumerIsBounded(t *testing.T) {
	shortTransferTimeout(t)
	c := &Clipboard{primaryContent: transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))}
	_, w := rawWriteEnd(t)

	before := runtime.NumGoroutine()
	start := time.Now()
	c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{MimeType: transfer.MimeText, Fd: w.Fd()})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("send to a silent consumer blocked for %s, want the deadline", elapsed)
	}
	time.Sleep(250 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines %d -> %d: the bounded send leaked one", before, after)
	}
}

// A consumer that closed its end early ends the send on EPIPE, right
// away.
func TestSendToAClosedConsumerEndsOnEPIPE(t *testing.T) {
	shortTransferTimeout(t)
	c := &Clipboard{content: transfer.Text("nobody reads this")}
	r, w := rawWriteEnd(t)
	r.Close()

	start := time.Now()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: transfer.MimeText, Fd: w.Fd()})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("send to a closed consumer took %s, want an immediate EPIPE", elapsed)
	}
}

// readOfferText and readOfferImage read an offer the way ReadText and
// ReadImageBytes do, against an injected receive/flush pair.
func readOfferText(receive func(string, uintptr) error, flush func() error, mimes map[string]bool) (string, error) {
	data, _, err := readOffer(receive, flush, mimes, transfer.TextMimes)
	return string(data), err
}

func readOfferImage(receive func(string, uintptr) error, flush func() error, mimes map[string]bool) ([]byte, string, error) {
	return readOffer(receive, flush, mimes, transfer.ImageMimes)
}

// pickTextMime and pickImageMime are the read preferences.
func pickTextMime(present func(string) bool) string {
	return transfer.Pick(transfer.TextMimes, present)
}

func pickImageMime(present func(string) bool) string {
	return transfer.Pick(transfer.ImageMimes, present)
}
