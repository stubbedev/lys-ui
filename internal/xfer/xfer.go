// Package xfer moves selection and drag-and-drop payloads through the
// pipes Wayland hands the two ends of a transfer. Both ends are
// foreign clients — the offer we read from, the consumer we write to —
// and either may be hostile or broken: an app on the machine can offer
// a text payload and stream gigabytes into a paste, or request our
// selection and never read it. Every transfer here is therefore
// bounded twice: in size, so a peer cannot grow our memory without
// limit, and in time, so a stalled peer cannot hang a paste or the
// event loop.
package xfer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
)

// MaxPayload caps one transfer at 16 MiB: orders of magnitude above
// any honest text selection or drop payload, yet small enough that a
// peer streaming at the cap cannot crowd the rest of the process out
// of memory. Reading past the cap truncates the transfer — the bytes
// that fit come back alongside an error, and the fd is drained (under
// the same deadline) so the peer's pipe never blocks forever.
const MaxPayload = 16 << 20

// DefaultTimeout bounds one transfer against a peer that stalls: long
// enough for an honest but slow peer, short enough that a hung paste
// surfaces as an error instead of a frozen window. Callers keep it in
// a var so tests can shorten it.
const DefaultTimeout = 5 * time.Second

var (
	// ErrTooLarge reports that the peer streamed past MaxPayload; the
	// bytes that fit were returned with it. Callers decide what a
	// truncated payload is worth — lys-ui's app layer treats the error
	// as a rejected paste.
	ErrTooLarge = errors.New("xfer: peer streamed past the payload budget")

	// ErrTimeout reports that the peer stalled — never wrote, stopped
	// mid-stream, or stopped reading — and the deadline cut the
	// transfer off.
	ErrTimeout = errors.New("xfer: peer stalled and the deadline cut the transfer off")
)

// Read drains r up to limit bytes, under a deadline, into a fresh
// slice: Copy into memory Read owns.
//
// A payload past limit is truncated: the bytes that fit are returned
// together with an error wrapping ErrTooLarge. A failed read returns no
// bytes.
func Read(r *os.File, limit int64, timeout time.Duration) ([]byte, error) {
	var b bytes.Buffer
	b.Grow(int(min(limit, 64<<10)))
	_, err := Copy(&b, r, limit, timeout)
	if err != nil && !errors.Is(err, ErrTooLarge) {
		return nil, err
	}
	return b.Bytes(), err
}

// Copy drains r into w up to limit bytes, under a deadline, and reports
// the bytes written: the streaming form of Read, for a caller that
// owns the memory a transfer lands in (reused, or off the Go heap). The
// deadline is a timer that closes r: a read blocked on a silent peer
// wakes with our own close error, which reports as ErrTimeout.
// time.AfterFunc creates no goroutine until it fires and the defer
// stops it when the transfer finishes early, so nothing is created
// for, or leaks after, a normal transfer.
//
// A payload past limit is truncated: the bytes that fit are written,
// and the error wraps ErrTooLarge, after the rest of the stream is
// drained into the void (still under the deadline) so the peer's
// writes to the shared pipe never block on a full buffer. A write w
// refuses ends the transfer with its error.
func Copy(w io.Writer, r *os.File, limit int64, timeout time.Duration) (int64, error) {
	timer := time.AfterFunc(timeout, func() { _ = r.Close() })
	defer timer.Stop()

	chunk := make([]byte, 32<<10)
	var written int64
	overrun := false
	readErr := error(nil)
	for readErr == nil && !overrun {
		var n int
		n, readErr = r.Read(chunk)
		if room := limit - written; n > 0 && int64(n) > room {
			n = int(max(room, 0))
			overrun = true
		}
		if n > 0 {
			if _, err := w.Write(chunk[:n]); err != nil {
				return written, fmt.Errorf("xfer: write: %w", err)
			}
			written += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				// EOF is the honest end of the transfer.
				readErr = nil
			}
			break
		}
	}
	if overrun {
		if readErr == nil {
			// The peer streamed past the cap. Keep discarding — the
			// deadline still applies, the timer closes r when the
			// peer will not stop — so the peer is never left
			// blocked on the pipe.
			if _, derr := io.Copy(io.Discard, r); derr != nil && !errors.Is(derr, io.EOF) {
				return written, errors.Join(oversize(limit), readFailure(derr))
			}
		} else if !errors.Is(readErr, io.EOF) {
			return written, errors.Join(oversize(limit), readFailure(readErr))
		}
		return written, oversize(limit)
	}
	if readErr != nil {
		return written, readFailure(readErr)
	}
	return written, nil
}

// DeadlineWriter prepares the pipe write end a consumer handed over —
// the compositor dup'ed it out of the requesting client's pipe — for a
// bounded source write: a consumer that stops reading cannot stall the
// writer forever, the deadline breaks the write, and EPIPE from a
// consumer that closed early ends it at once. Close the writer when
// the transfer ends, whatever the outcome, so the consumer sees EOF
// rather than a hang; a write cut short by the deadline or EPIPE
// leaves the consumer a short payload plus EOF — evidence the transfer
// broke, not a puzzle about where it stopped.
//
// The descriptor arrives in blocking mode, and a blocking os.File is
// invisible to the runtime poller — no deadline could touch it and a
// stuck write would be unstoppable. Flipping the descriptor to
// non-blocking first hands NewFile a pollable file whose write takes
// the deadline. The flag lives on the open file description, but the
// requesting client closed its hold on that description when it handed
// the end over, so nothing else observes the switch.
func DeadlineWriter(fd uintptr, timeout time.Duration) (*os.File, error) {
	if fd == 0 {
		return nil, errors.New("xfer: nil send descriptor")
	}
	if err := syscall.SetNonblock(int(fd), true); err != nil {
		return nil, fmt.Errorf("xfer: unblock send descriptor: %w", err)
	}
	f := os.NewFile(fd, "wayland-payload-send")
	if f == nil {
		return nil, errors.New("xfer: bad send descriptor")
	}
	if err := f.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		// The poller refused the descriptor. A descriptor flipped
		// non-blocking but left outside the poller would misreport
		// EAGAIN on every write, so restore the original blocking
		// mode and hand back an unguarded writer: in a running lys-ui
		// the Wayland socket itself needs the poller, so this is
		// nearly unreachable, and an unbounded write still beats
		// failing every send.
		_ = syscall.SetNonblock(int(fd), false)
		return f, nil
	}
	return f, nil
}

// oversize reports the truncation of a transfer capped at limit.
func oversize(limit int64) error {
	return fmt.Errorf("%w: capped at %d bytes", ErrTooLarge, limit)
}

// readFailure maps a pipe read that died into the transfer's error
// vocabulary: our deadline timer closing the fd is the stall signal,
// anything else is the failure itself.
func readFailure(err error) error {
	if errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("xfer: read: %w", ErrTimeout)
	}
	return fmt.Errorf("xfer: read: %w", err)
}
