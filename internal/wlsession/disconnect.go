// Compositor disconnect classification. When the connection dies — the
// compositor crashed, was reloaded, or was upgraded out from under the
// client — the wire surfaces it as a read or write failure on the unix
// socket, and the neurlang Context folds those into a handful of error
// shapes. This file maps every shape the dispatch paths can return onto
// one typed error, so an application's loop sees a disconnect it can
// act on instead of whatever a raw errno means.
package wlsession

import (
	"errors"
	"fmt"
	"io"
	"syscall"

	"github.com/neurlang/wayland/wl"
)

// DisconnectReason classifies why the compositor connection ended.
type DisconnectReason int

const (
	// DisconnectConnectionLost means the compositor went away without
	// a verdict — EOF on the socket read, or EPIPE/ECONNRESET on a
	// read or write. This is the crash/reload/upgrade case: the
	// compositor is gone, and every proxy on the connection is dead
	// with it.
	DisconnectConnectionLost DisconnectReason = iota
	// DisconnectProtocol means the compositor raised a fatal
	// wl_display.error against this client before the connection
	// ended. That is a bug (client or compositor) rather than a
	// restart: reconnecting would replay the fatal exchange.
	DisconnectProtocol
)

func (r DisconnectReason) String() string {
	switch r {
	case DisconnectProtocol:
		return "protocol error"
	default:
		return "connection lost"
	}
}

// ErrDisconnected reports that the compositor ended the connection.
// Every disconnect the dispatch paths return matches it under
// errors.Is; the classified reason lives on the *DisconnectError in the
// chain (errors.As), and app re-exports both for its callers.
var ErrDisconnected = errors.New("wlsession: compositor disconnected")

// DisconnectError is the typed shape of a dead connection: the
// classified reason plus the underlying wire failure.
type DisconnectError struct {
	Reason DisconnectReason
	// Err is the wire failure, decorated with the compositor's verdict
	// when one was recorded before the drop.
	Err error
}

func (e *DisconnectError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("wlsession: compositor disconnected (%s): %v", e.Reason, e.Err)
	}
	return "wlsession: compositor disconnected (" + e.Reason.String() + ")"
}

func (e *DisconnectError) Unwrap() error { return e.Err }

// Is makes every DisconnectError match the ErrDisconnected sentinel,
// whatever else the chain wraps.
func (e *DisconnectError) Is(target error) bool { return target == ErrDisconnected }

// maxProxyNilRetries bounds the dispatch retries for events whose proxy
// was destroyed mid-queue. The retry is legitimate bookkeeping (a done
// frame callback, a dismissed popup abort one dispatch pass), and each
// pass consumes at least one queued event, so a live connection cannot
// need anywhere near this many; the bound exists so no error shape — a
// dead fd chief among them — can ever turn it into a spin.
const maxProxyNilRetries = 1024

// runThroughProxyNil runs one dispatch pass and retries while the wire
// reports a destroyed-proxy abort. Bounded by maxProxyNilRetries: past
// the bound this returns instead of looping, because a retry loop that
// never ends is indistinguishable from a hung process.
func runThroughProxyNil(run func() error) error {
	err := run()
	for i := 0; errors.Is(err, wl.ErrContextRunProxyNil); i++ {
		if i >= maxProxyNilRetries {
			return fmt.Errorf("wlsession: dispatch stuck on destroyed proxies after %d retries: %w", i, err)
		}
		err = run()
	}
	return err
}

// asDisconnect classifies a dispatch or send failure. Connection-death
// shapes (EOF, dead-socket errnos, the post-close context state, a
// compositor verdict) become a *DisconnectError matching
// ErrDisconnected; anything else — a timeout, a decode failure with no
// verdict — comes back decorated but unclassified, because treating
// every wire hiccup as a disconnect would kill sessions for nothing.
func (s *Session) asDisconnect(err error) error {
	if err == nil {
		return nil
	}
	if s.protoErr == nil && peerClosed(err) {
		s.drainVerdict()
	}
	protocol := s.protoErr != nil || carriesExternal(err, wl.ErrContextRunProtocolError)
	if !protocol && !isConnectionDeath(err) {
		return s.withProtoErr(err)
	}
	reason := DisconnectConnectionLost
	if protocol {
		reason = DisconnectProtocol
	}
	cause := err
	if s.protoErr != nil {
		cause = fmt.Errorf("%w (compositor: %w)", err, s.protoErr)
	}
	return &DisconnectError{Reason: reason, Err: cause}
}

// drainLimit bounds how many queued events drainVerdict dispatches.
const drainLimit = 4096

// drainVerdict dispatches what a closed compositor left in the socket,
// up to its wl_display.error: a compositor raises the error and then
// closes, so a write that hits the closed socket first fails before
// the verdict was read. The peer is closed, so each read returns
// queued data or fails at once; it never blocks.
func (s *Session) drainVerdict() {
	if s.Display == nil {
		return
	}
	for range drainLimit {
		if s.Display.Context().Run() != nil || s.protoErr != nil {
			return
		}
	}
}

// peerClosed reports whether err is a write or read on a socket whose
// peer has closed (EPIPE, ECONNRESET), the shapes after which the
// queued events are still readable without blocking.
func peerClosed(err error) bool {
	errno, ok := errors.AsType[syscall.Errno](err)
	return ok && (errno == syscall.EPIPE || errno == syscall.ECONNRESET)
}

// Classify maps a raw wire failure onto the disconnect taxonomy — the
// same mapping Step and Roundtrip apply before returning. The app loop
// runs it on failures that surface outside the park (a frame's
// attach/commit on a dead socket), so every dispatch surface reports
// disconnects the same way.
func (s *Session) Classify(err error) error { return s.asDisconnect(err) }

// isConnectionDeath reports whether err is a shape the wire returns
// when the connection is dead or dying: the peer closed (EOF), a socket
// operation failed with a dead-connection errno, the local context was
// closed under the dispatch, or a dispatched event carried the
// compositor's fatal protocol verdict.
func isConnectionDeath(err error) bool {
	if errors.Is(err, wl.ErrContextRunConnectionClosed) ||
		errors.Is(err, wl.ErrContextConnNil) ||
		errors.Is(err, wl.ErrContextNil) {
		return true
	}
	// A peer-closed read surfaces as io.EOF wrapped in the read-error
	// chain (the wire only maps a bare io.EOF to its connection-closed
	// shape, which the combined wrapper defeats), so look for EOF
	// through the unwrap too. In this wire EOF can only come from the
	// socket: the compositor is gone.
	if errors.Is(err, io.EOF) {
		return true
	}
	if carriesExternal(err, wl.ErrContextRunProtocolError) {
		return true
	}
	if errno, ok := errors.AsType[syscall.Errno](err); ok {
		switch errno {
		case syscall.EPIPE, syscall.ECONNRESET, syscall.ECONNABORTED,
			syscall.ENOTCONN, syscall.ENETDOWN, syscall.ENETUNREACH,
			syscall.ENETRESET, syscall.ESHUTDOWN, syscall.EBADF:
			return true
		}
	}
	return false
}

// carriesExternal walks the error chain for the wire's combined-error
// shape: combinedError wraps an external marker and an internal cause,
// and its Unwrap only exposes the cause, so errors.Is cannot see the
// marker — the External accessor is the only way in.
func carriesExternal(err error, target error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if ext, ok := e.(interface{ External() error }); ok && errors.Is(ext.External(), target) {
			return true
		}
	}
	return false
}
