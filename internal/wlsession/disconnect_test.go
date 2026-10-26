package wlsession

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/wltest"
)

// deadSocketSession returns a Session wired to a real unix socket whose
// peer has closed: the honest "compositor went away" shape, no
// compositor needed. The caller must Close the session.
func deadSocketSession(t *testing.T) *Session {
	t.Helper()
	ln, _ := wltest.LoopbackDisplay(t, "wayland-dead")
	d, err := wl.Connect("")
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	// The peer disappears: reads now hit EOF, writes EPIPE — exactly
	// what a killed compositor leaves behind.
	ln.Close()
	t.Cleanup(func() { _ = d.Context().Close() })
	return &Session{Display: d}
}

// TestStepClassifiesDeadSocket is the headline: a dispatch on a socket
// whose peer died must come back as one typed DisconnectError matching
// ErrDisconnected — not a raw errno, not a spin.
func TestStepClassifiesDeadSocket(t *testing.T) {
	s := deadSocketSession(t)
	done := make(chan error, 1)
	go func() { done <- s.Step() }()
	select {
	case err := <-done:
		var de *DisconnectError
		if !errors.As(err, &de) {
			t.Fatalf("Step on a dead socket = %v, want a *DisconnectError", err)
		}
		if de.Reason != DisconnectConnectionLost {
			t.Errorf("reason = %s, want connection lost", de.Reason)
		}
		if !errors.Is(err, ErrDisconnected) {
			t.Errorf("errors.Is(err, ErrDisconnected) = false for %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Step spun on a dead socket instead of returning")
	}
}

// TestRoundtripReturnsOnDeadSocket pins the Roundtrip half of the same
// contract: the sync's read fails immediately, so the (now bounded)
// retry loop must return the typed error rather than loop forever.
func TestRoundtripReturnsOnDeadSocket(t *testing.T) {
	s := deadSocketSession(t)
	done := make(chan error, 1)
	go func() { done <- s.Roundtrip() }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("Roundtrip on a dead socket = %v, want ErrDisconnected", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Roundtrip spun on a dead socket instead of returning")
	}
	// Post-close dispatch is classified too: it is the same dead
	// connection, and a caller looping on it must see a disconnect.
	s.Close()
	err := s.Step()
	if _, ok := errors.AsType[*DisconnectError](err); !ok {
		t.Fatalf("Step after Close = %v, want a *DisconnectError", err)
	}
}

// TestProtocolVerdictYieldsProtocolReason: a compositor that raised a
// fatal wl_display.error before the drop must be reported as
// DisconnectProtocol — reconnecting would replay the fatal exchange, so
// the reason has to tell the two deaths apart.
func TestProtocolVerdictYieldsProtocolReason(t *testing.T) {
	s := deadSocketSession(t)
	s.HandleDisplayError(wl.DisplayErrorEvent{Message: "surface not from this client"})
	err := s.Step()
	var de *DisconnectError
	if !errors.As(err, &de) {
		t.Fatalf("Step = %v, want a *DisconnectError", err)
	}
	if de.Reason != DisconnectProtocol {
		t.Errorf("reason = %s, want protocol error", de.Reason)
	}
	if !strings.Contains(err.Error(), "surface not from this client") {
		t.Errorf("error %q lost the compositor's verdict", err)
	}
}

// fakeCombined mimics the wire's combined-error shape: an external
// marker plus an internal cause, where Unwrap exposes only the cause —
// the real type is unexported in the wire package, so the classifier's
// External walk is exercised through this.
type fakeCombined struct{ internal error }

func (f fakeCombined) Error() string { return "external: " + f.internal.Error() }
func (fakeCombined) External() error { return wl.ErrContextRunProtocolError }
func (f fakeCombined) Unwrap() error { return f.internal }

// TestAsDisconnectClassificationTable walks the error shapes the wire
// actually produces (eof, dead-socket errnos on read and write, the
// combined-error wrapper the Context folds them into, and unclassifiable
// noise) through the classifier.
func TestAsDisconnectClassificationTable(t *testing.T) {
	s := &Session{}
	cases := []struct {
		name string
		err  error
		want bool // classified as a disconnect?
	}{
		{"connection closed (peer EOF)", wl.ErrContextRunConnectionClosed, true},
		{"read errno ECONNRESET", syscall.ECONNRESET, true},
		{"write errno EPIPE", syscall.EPIPE, true},
		{"post-close fd", syscall.EBADF, true},
		{"wrapped errno", fmt.Errorf("read: %w", syscall.ECONNRESET), true},
		// The shape the live kill test surfaced: a peer-closed read wrapped
		// twice by the wire (event reading error → read header → EOF). The
		// wire's own EOF check is defeated by its combined wrapper, so the
		// classifier must find EOF through the unwrap chain.
		{"wrapped EOF from a peer-closed read", fmt.Errorf("%w: %w",
			wl.ErrContextRunEventReadingError, fmt.Errorf("%w: %w", wl.ErrReadHeader, io.EOF)), true},
		{"unclassifiable", errors.New("something else"), false},
		{"timeout", wl.ErrContextRunTimeout, false},
	}
	for _, tc := range cases {
		got := s.asDisconnect(tc.err)
		var de *DisconnectError
		classified := errors.As(got, &de)
		if classified != tc.want {
			t.Errorf("%s: asDisconnect(%v) classified = %v, want %v", tc.name, tc.err, classified, tc.want)
			continue
		}
		if classified && !errors.Is(got, ErrDisconnected) {
			t.Errorf("%s: %v does not match ErrDisconnected", tc.name, got)
		}
	}
	// A protocol-error marker inside the combined shape classifies even
	// without a recorded protoErr event — errors.Is cannot see the
	// marker (Unwrap exposes only the cause), the External walk must.
	marker := fakeCombined{internal: errors.New("bad opcode")}
	if !carriesExternal(marker, wl.ErrContextRunProtocolError) {
		t.Fatal("carriesExternal missed the External marker")
	}
	if errors.Is(marker, wl.ErrContextRunProtocolError) {
		t.Fatal("errors.Is sees through the combined shape; the External walk is redundant")
	}
	var de *DisconnectError
	if !errors.As(s.asDisconnect(marker), &de) || de.Reason != DisconnectProtocol {
		t.Errorf("protocol marker %v: want DisconnectError with protocol reason", marker)
	}
}

// TestRunThroughProxyNilBounded pins the retry budget: destroyed-proxy
// aborts retry (they are bookkeeping), but the loop returns after a
// bounded number of passes on any error shape — including one that
// never stops reporting ProxyNil.
func TestRunThroughProxyNilBounded(t *testing.T) {
	t.Run("proxy-nil aborts retry and finish", func(t *testing.T) {
		passes := 0
		err := runThroughProxyNil(func() error {
			passes++
			if passes <= 3 {
				return wl.ErrContextRunProxyNil
			}
			return nil
		})
		if err != nil {
			t.Fatalf("runThroughProxyNil = %v, want nil", err)
		}
		if passes != 4 {
			t.Errorf("passes = %d, want 4 (three aborts, then done)", passes)
		}
	})
	t.Run("a permanent proxy-nil returns instead of spinning", func(t *testing.T) {
		passes := 0
		err := runThroughProxyNil(func() error {
			passes++
			return wl.ErrContextRunProxyNil
		})
		if err == nil {
			t.Fatal("runThroughProxyNil returned nil after unbounded proxy-nil")
		}
		if passes != maxProxyNilRetries+1 {
			t.Errorf("passes = %d, want %d (bounded)", passes, maxProxyNilRetries+1)
		}
		if !errors.Is(err, wl.ErrContextRunProxyNil) {
			t.Errorf("bound error %v lost the underlying cause", err)
		}
	})
	t.Run("a dead socket error is not retried at all", func(t *testing.T) {
		passes := 0
		err := runThroughProxyNil(func() error {
			passes++
			return wl.ErrContextRunConnectionClosed
		})
		if passes != 1 || !errors.Is(err, wl.ErrContextRunConnectionClosed) {
			t.Errorf("passes = %d err = %v; dead-socket shapes must return immediately", passes, err)
		}
	})
}

// TestCloseArenasOnDeadSessionClose: the disconnect cleanup that
// Session.Close runs must be safe on an already-dead connection and
// idempotent — the app teardown calls it before the caller's own
// deferred Close.
func TestCloseIsIdempotent(t *testing.T) {
	s := deadSocketSession(t)
	s.Close()
	s.Close()
}

// closedAfterVerdict connects a Session to a loopback compositor that,
// when verdict is set, raises a wl_display.error on the display (code
// 1, "invalid method") and then closes the connection: the client's
// next action is a write, which hits the closed socket before the
// verdict was read. It returns the session and that write's error.
func closedAfterVerdict(t *testing.T, verdict bool) (*Session, error) {
	t.Helper()
	ln, _ := wltest.LoopbackDisplay(t, "wayland-verdict")
	d, err := wl.Connect("")
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Context().Close() })
	conn, err := ln.AcceptUnix()
	ln.Close()
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{Display: d}
	d.AddErrorHandler(s)
	if verdict {
		msg := "invalid method"
		pad := (len(msg) + 1 + 3) &^ 3
		frame := binary.NativeEndian.AppendUint32(nil, 1)
		frame = binary.NativeEndian.AppendUint32(frame, uint32(8+4+4+4+pad)<<16)
		frame = binary.NativeEndian.AppendUint32(frame, 1)
		frame = binary.NativeEndian.AppendUint32(frame, 1)
		frame = binary.NativeEndian.AppendUint32(frame, uint32(len(msg)+1))
		frame = append(frame, msg...)
		frame = append(frame, make([]byte, pad-len(msg))...)
		if _, err := conn.Write(frame); err != nil {
			t.Fatal(err)
		}
	}
	conn.Close()
	_, err = d.Sync()
	if err == nil {
		t.Fatal("a write to the closed compositor succeeded")
	}
	return s, err
}

// A verdict the compositor sent before closing is read back when the
// next write finds the socket closed (#248): the disconnect is the
// protocol error, naming the object, the code and the message.
func TestWriteAfterVerdictKeepsTheVerdict(t *testing.T) {
	s, werr := closedAfterVerdict(t, true)
	err := s.Classify(werr)
	de, ok := errors.AsType[*DisconnectError](err)
	if !ok || de.Reason != DisconnectProtocol {
		t.Fatalf("Classify(%v) = %v, want a protocol DisconnectError", werr, err)
	}
	if want := "*wl.Display@1: error 1: invalid method"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q, want it to carry %q", err, want)
	}
}

// A compositor that closes without a verdict stays a lost connection.
func TestWriteAfterSilentCloseIsConnectionLost(t *testing.T) {
	s, werr := closedAfterVerdict(t, false)
	de, ok := errors.AsType[*DisconnectError](s.Classify(werr))
	if !ok || de.Reason != DisconnectConnectionLost || s.protoErr != nil {
		t.Fatalf("Classify(%v) = %v, verdict %v; want a lost connection without one", werr, de, s.protoErr)
	}
}
