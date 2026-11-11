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

	"github.com/stubbedev/lys-ui/internal/fdtest"
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

func testClipboard(t *testing.T) *Clipboard {
	t.Helper()
	c := &Clipboard{invoke: func(fn func()) { fn() }}
	t.Cleanup(c.Stop)
	return c
}

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

func TestReadWithoutASchedulerFailsLoudly(t *testing.T) {
	var got error
	(&Clipboard{}).ReadText(func(_ string, err error) { got = err })
	if !errors.Is(got, errNoScheduler) {
		t.Errorf("read without a scheduler = %v, want errNoScheduler", got)
	}
}

func TestSendToAStuckConsumerIsBounded(t *testing.T) {
	c := testClipboard(t)
	c.content = transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))
	r, w := fdtest.RawPipe(t)
	defer r.Close()
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: transfer.MimeText, Fd: w})
	c.Stop()
}

func TestPrimarySendToAStuckConsumerIsBounded(t *testing.T) {
	c := testClipboard(t)
	c.primaryContent = transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))
	r, w := fdtest.RawPipe(t)
	defer r.Close()
	c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{MimeType: transfer.MimeText, Fd: w})
	c.Stop()
}

func TestSendDeadlineEndsAStuckConsumer(t *testing.T) {
	shortTransferTimeout(t)
	c := testClipboard(t)
	c.content = transfer.Text(string(bytes.Repeat([]byte("p"), 1<<20)))
	r, w := fdtest.RawPipe(t)
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
	r, w := fdtest.RawPipe(t)
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

// Every send request's fd is closed, regular and primary: one served,
// one for a mime the content does not offer, and one past Stop.
func TestSendHandlersCloseTheirFds(t *testing.T) {
	c := testClipboard(t)
	c.content, c.primaryContent = transfer.Text("clipboard payload"), transfer.Text("primary payload")
	sends := []func(mime string, fd uintptr){
		func(mime string, fd uintptr) { c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: mime, Fd: fd}) },
		func(mime string, fd uintptr) {
			c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{MimeType: mime, Fd: fd})
		},
	}
	request := func(send func(string, uintptr), mime string) {
		t.Helper()
		r, w := fdtest.RawPipe(t)
		defer r.Close()
		send(mime, w)
		if _, err := io.ReadAll(r); err != nil {
			t.Fatal(err)
		}
	}
	request(sends[0], transfer.MimeText)
	before := fdtest.Own(t)
	const events = 16
	for range events {
		for _, send := range sends {
			request(send, transfer.MimeText)
			request(send, "image/png")
		}
	}
	c.Stop()
	for range events {
		for _, send := range sends {
			request(send, transfer.MimeText)
		}
	}
	fdtest.RequireNoneBeyond(t, before, "serving, refusing and stopping send requests")
}
