package clipboard

import (
	"bytes"
	"errors"
	"io"
	"os"
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
// hands out: never registered with the poller before, and the handler's
// to close. (An os.Pipe end is already registered, which the deadline
// arming would refuse.)
func rawWriteEnd(t *testing.T) (*os.File, uintptr) {
	t.Helper()
	var fds [2]int
	if err := syscall.Pipe2(fds[:], 0); err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(fds[0]), "clipboard-test-raw-r"), uintptr(fds[1])
}

// testClipboard is a clipboard whose invoke stands in for the event
// loop: it runs what comes back on the transfer goroutine, where the
// test's channels pick it up.
func testClipboard(t *testing.T) *Clipboard {
	t.Helper()
	c := &Clipboard{invoke: func(fn func()) { fn() }}
	t.Cleanup(c.Stop)
	return c
}

// awaitRead bounds the wait for one read's delivery.
func awaitRead[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(10 * time.Second):
		t.Fatal("the read never delivered")
		panic("unreachable")
	}
}

type readResult struct {
	data []byte
	mime string
	err  error
}

func collectRead(t *testing.T, start func(done func([]byte, string, error))) readResult {
	t.Helper()
	ch := make(chan readResult, 1)
	start(func(data []byte, mime string, err error) { ch <- readResult{data, mime, err} })
	return awaitRead(t, ch)
}

func collectText(t *testing.T, start func(done func(string, error))) (string, error) {
	t.Helper()
	r := collectRead(t, func(done func([]byte, string, error)) {
		start(func(s string, err error) { done([]byte(s), "", err) })
	})
	return string(r.data), r.err
}

// The offer pipeline keeps working for an honest peer.
func TestReadOfferTextRoundtripsASmallPayload(t *testing.T) {
	var wait func() error
	receive := func(mime string, fd uintptr) error {
		wait = fakePeer(t, fd, []byte("pasted text"))
		return nil
	}
	text, err := readOfferText(t, receive, map[string]bool{"text/plain;charset=utf-8": true})
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
	text, err := readOfferText(t, receive, map[string]bool{"text/plain": true})
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

// A peer that never writes leaves the paste pending, not the caller, and
// the deadline ends it with an error.
func TestReadOfferTextCutsOffASilentPeer(t *testing.T) {
	shortTransferTimeout(t)
	c := testClipboard(t)
	receive := func(mime string, fd uintptr) error {
		holdSilentPeer(t, fd)
		return nil
	}
	ch := make(chan error, 1)
	c.fetchText(selectionView{receive: receive, mimes: map[string]bool{"text/plain": true}}, transfer.TextMimes, bytesToString,
		func(_ string, err error) { ch <- err })
	if err := awaitRead(t, ch); !errors.Is(err, xfer.ErrTimeout) {
		t.Fatalf("read = %v, want xfer.ErrTimeout", err)
	}
}

// The caller returns before a silent peer has answered, and stopping the
// clipboard ends the transfer long before its deadline.
func TestReadReturnsBeforeASilentPeerAndStopEndsIt(t *testing.T) {
	c := testClipboard(t)
	receives := 0
	receive := func(mime string, fd uintptr) error {
		receives++
		holdSilentPeer(t, fd)
		return nil
	}
	delivered := make(chan error, 1)
	c.fetchText(selectionView{receive: receive, mimes: map[string]bool{"text/plain": true}}, transfer.TextMimes, bytesToString,
		func(_ string, err error) { delivered <- err })
	if receives != 1 {
		t.Fatalf("receive ran %d times before the read returned, want the request out", receives)
	}
	select {
	case err := <-delivered:
		t.Fatalf("a silent peer's read delivered %v, want it pending", err)
	default:
	}
	c.Stop()
	if err := awaitRead(t, delivered); err == nil {
		t.Error("a stopped transfer delivered no error")
	}
}

// Reads past xfer.MaxTransfers are refused rather than queued without
// bound, and refused on the loop like any other result.
func TestReadsPastTheBoundAreRefused(t *testing.T) {
	c := testClipboard(t)
	receive := func(mime string, fd uintptr) error {
		holdSilentPeer(t, fd)
		return nil
	}
	view := selectionView{receive: receive, mimes: map[string]bool{"text/plain": true}}
	for range xfer.MaxTransfers {
		c.fetchText(view, transfer.TextMimes, bytesToString, func(string, error) {})
	}
	ch := make(chan error, 1)
	c.fetchText(view, transfer.TextMimes, bytesToString, func(_ string, err error) { ch <- err })
	if err := awaitRead(t, ch); !errors.Is(err, xfer.ErrBusy) {
		t.Errorf("read past the bound = %v, want xfer.ErrBusy", err)
	}
}

// A clipboard no loop owns refuses a read at the call instead of leaving
// the reader waiting for a delivery nothing will run.
func TestReadWithoutASchedulerFailsLoudly(t *testing.T) {
	var got error
	(&Clipboard{}).ReadText(func(_ string, err error) { got = err })
	if !errors.Is(got, errNoScheduler) {
		t.Errorf("read without a scheduler = %v, want errNoScheduler", got)
	}
}

// A consumer that never reads holds a transfer goroutine, never the
// handler: it returns with the send running, and Stop ends the write.
func TestSendToAStuckConsumerIsBounded(t *testing.T) {
	c := testClipboard(t)
	c.content = transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))
	r, w := rawWriteEnd(t)
	defer r.Close()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: transfer.MimeText, Fd: w})
	c.Stop()
}

func TestPrimarySendToAStuckConsumerIsBounded(t *testing.T) {
	c := testClipboard(t)
	c.primaryContent = transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))
	r, w := rawWriteEnd(t)
	defer r.Close()
	c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{MimeType: transfer.MimeText, Fd: w})
	c.Stop()
}

// The deadline also ends a stuck send with nobody stopping it: the
// consumer sees EOF after the bytes the pipe took.
func TestSendDeadlineEndsAStuckConsumer(t *testing.T) {
	shortTransferTimeout(t)
	c := testClipboard(t)
	c.content = transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))
	r, w := rawWriteEnd(t)
	defer r.Close()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: transfer.MimeText, Fd: w})
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("the consumer read nothing before EOF")
	}
}

// A consumer that closed its end early ends the send on EPIPE, right
// away.
func TestSendToAClosedConsumerEndsOnEPIPE(t *testing.T) {
	c := testClipboard(t)
	c.content = transfer.Text("nobody reads this")
	r, w := rawWriteEnd(t)
	r.Close()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: transfer.MimeText, Fd: w})
	c.Stop()
}

// readOfferText and readOfferImage read an offer the way ReadText and
// ReadImageBytes do, against an injected receive.
func readOfferText(t *testing.T, receive func(string, uintptr) error, mimes map[string]bool) (string, error) {
	t.Helper()
	c := testClipboard(t)
	return collectText(t, func(done func(string, error)) {
		c.fetchText(selectionView{receive: receive, mimes: mimes}, transfer.TextMimes, bytesToString, done)
	})
}

func readOfferImage(t *testing.T, receive func(string, uintptr) error, mimes map[string]bool) ([]byte, string, error) {
	t.Helper()
	c := testClipboard(t)
	r := collectRead(t, func(done func([]byte, string, error)) {
		c.fetch(selectionView{receive: receive, mimes: mimes}, transfer.ImageMimes, func(data []byte, mime string, err error) func() {
			return func() { done(data, mime, err) }
		})
	})
	return r.data, r.mime, r.err
}

// pickTextMime and pickImageMime are the read preferences.
func pickTextMime(present func(string) bool) string {
	return transfer.Pick(transfer.TextMimes, present)
}

func pickImageMime(present func(string) bool) string {
	return transfer.Pick(transfer.ImageMimes, present)
}
