// Package sessionlock maps ext-session-lock-v1 onto lys-ui: the lock
// object's locked/finished lifecycle and the per-output lock surface
// role with its configure handshake.
//
// The protocol is a security boundary, so the client-side rules it
// states are enforced here instead of being left to callers:
//
//   - a lock that received locked is only ever released with
//     unlock_and_destroy (destroy would be invalid_destroy), and
//     unlock_and_destroy is refused until locked arrived
//     (invalid_unlock);
//   - an output gets at most one lock surface (duplicate_output);
//   - a lock surface is not usable (nothing may commit on it) before
//     its first configure was acked (commit_before_first_ack), and
//     its size is exactly the acked configure size
//     (dimensions_mismatch);
//   - finished releases the lock with the request matching its state,
//     and no surface can be created on an ended lock.
package sessionlock

import (
	"errors"
	"fmt"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// ErrNotLocked reports an unlock attempted before the compositor sent
// locked: unlock_and_destroy would be the invalid_unlock protocol
// error. Cancel the pending lock instead.
var ErrNotLocked = errors.New("sessionlock: session is not locked yet")

// ErrLocked reports a cancel attempted after the compositor sent
// locked: destroy would be the invalid_destroy protocol error, and a
// locked session only ends through Unlock.
var ErrLocked = errors.New("sessionlock: session is locked; only Unlock ends it")

// ErrEnded reports a request on a lock that was already unlocked,
// cancelled, or finished by the compositor.
var ErrEnded = errors.New("sessionlock: lock has ended")

// ErrDuplicateOutput reports a second lock surface for one output,
// the duplicate_output protocol error.
var ErrDuplicateOutput = errors.New("sessionlock: output already has a lock surface")

// ErrNotConfigured gates drawing before the first configure: the
// compositor kills the client for committing earlier.
var ErrNotConfigured = errors.New("sessionlock: lock surface not configured yet")

// ErrClosed reports a lock surface that was destroyed.
var ErrClosed = errors.New("sessionlock: lock surface closed")

// State is where a lock is in its lifecycle.
type State uint8

// Lock lifecycle states.
const (
	// Pending: lock was requested; neither locked nor finished arrived.
	Pending State = iota
	// Locked: the compositor confirmed the session is locked.
	Locked
	// Unlocked: the client released a locked session.
	Unlocked
	// Cancelled: the client withdrew a lock that never locked.
	Cancelled
	// Finished: the compositor ended the lock (denied it, or ended a
	// locked session through its own policy).
	Finished
)

// String names the state for logs.
func (s State) String() string {
	switch s {
	case Pending:
		return "pending"
	case Locked:
		return "locked"
	case Unlocked:
		return "unlocked"
	case Cancelled:
		return "cancelled"
	case Finished:
		return "finished"
	}
	return fmt.Sprintf("State(%d)", uint8(s))
}

// Ended reports whether the lock is over, whichever way it ended.
func (s State) Ended() bool { return s >= Unlocked }

// lockAPI is the request side of ext_session_lock_v1, narrowed so
// tests record the requests. wireLock adapts the generated proxy.
type lockAPI interface {
	GetLockSurface(surf *wl.Surface, out *wl.Output) (surfaceAPI, error)
	UnlockAndDestroy() error
	Destroy() error
}

// surfaceAPI is the request side of ext_session_lock_surface_v1.
type surfaceAPI interface {
	AckConfigure(serial uint32) error
	Destroy() error
}

// wireLock adapts *wlr.SessionLockV1 to lockAPI.
type wireLock struct{ l *wlr.SessionLockV1 }

func (w wireLock) GetLockSurface(surf *wl.Surface, out *wl.Output) (surfaceAPI, error) {
	s, err := w.l.GetLockSurface(surf, out)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (w wireLock) UnlockAndDestroy() error { return w.l.UnlockAndDestroy() }
func (w wireLock) Destroy() error          { return w.l.Destroy() }

// Hooks observe the compositor's verdicts. Both run on the event-loop
// goroutine, inside dispatch.
type Hooks struct {
	// Locked fires once the compositor confirmed the lock: every
	// output shows lock content, nothing unlocked is visible.
	Locked func()
	// Finished fires when the compositor ended the lock: it denied it
	// (another client holds the lock) or ended a locked session by its
	// own policy. The lock is already released on the wire.
	Finished func()
}

// Lock is one ext_session_lock_v1 object.
type Lock struct {
	req     lockAPI
	hooks   Hooks
	state   State
	outputs map[*wl.Output]*Surface
}

// New requests a session lock from the manager. The compositor answers
// with locked or finished; lock surfaces should be created for every
// output right away so it can show them in the first locked frame.
func New(mgr *wlr.SessionLockManagerV1, hooks Hooks) (*Lock, error) {
	if mgr == nil {
		return nil, errors.New("sessionlock: no lock manager")
	}
	l, err := mgr.Lock()
	if err != nil {
		return nil, fmt.Errorf("sessionlock: lock: %w", err)
	}
	lock := newLock(wireLock{l: l}, hooks)
	l.AddLockedHandler(lock)
	l.AddFinishedHandler(lock)
	return lock, nil
}

// newLock builds the state machine over a request sink; tests pass a
// recorder.
func newLock(req lockAPI, hooks Hooks) *Lock {
	return &Lock{req: req, hooks: hooks, outputs: make(map[*wl.Output]*Surface)}
}

// State returns the lifecycle state.
func (l *Lock) State() State { return l.state }

// HandleSessionLockV1Locked implements wlr.SessionLockV1LockedHandler.
func (l *Lock) HandleSessionLockV1Locked(wlr.SessionLockV1LockedEvent) { l.onLocked() }

// HandleSessionLockV1Finished implements
// wlr.SessionLockV1FinishedHandler.
func (l *Lock) HandleSessionLockV1Finished(wlr.SessionLockV1FinishedEvent) { l.onFinished() }

func (l *Lock) onLocked() {
	if l.state != Pending {
		// A locked after the client already cancelled crossed on the
		// wire; the object is gone on our side.
		return
	}
	l.state = Locked
	if l.hooks.Locked != nil {
		l.hooks.Locked()
	}
}

// onFinished releases the lock with the request its state allows
// (destroy before locked, unlock_and_destroy after) and closes every
// surface: the compositor no longer uses them.
func (l *Lock) onFinished() {
	if l.state.Ended() {
		return
	}
	if l.state == Locked {
		_ = l.req.UnlockAndDestroy()
	} else {
		_ = l.req.Destroy()
	}
	l.state = Finished
	l.closeSurfaces()
	if l.hooks.Finished != nil {
		l.hooks.Finished()
	}
}

// NewSurface assigns the lock-surface role for out to surf. The
// surface must be fresh: no role, nothing attached or committed. The
// compositor answers with the first configure; until it is acked the
// surface reports ErrNotConfigured and must not commit.
func (l *Lock) NewSurface(surf *wl.Surface, out *wl.Output) (*Surface, error) {
	if l.state.Ended() {
		return nil, ErrEnded
	}
	if surf == nil || out == nil {
		return nil, errors.New("sessionlock: a lock surface needs a surface and an output")
	}
	if _, ok := l.outputs[out]; ok {
		return nil, ErrDuplicateOutput
	}
	req, err := l.req.GetLockSurface(surf, out)
	if err != nil {
		return nil, fmt.Errorf("sessionlock: get_lock_surface: %w", err)
	}
	s := &Surface{WLSurface: surf, req: req, lock: l, out: out}
	if wire, ok := req.(*wlr.SessionLockSurfaceV1); ok {
		wire.AddConfigureHandler(s)
	}
	l.outputs[out] = s
	return s, nil
}

// SurfaceFor returns the live lock surface on out, if any.
func (l *Lock) SurfaceFor(out *wl.Output) (*Surface, bool) {
	s, ok := l.outputs[out]
	return s, ok
}

// Unlock ends a locked session: unlock_and_destroy goes out and every
// lock surface is destroyed. Refused with ErrNotLocked while the lock
// is pending — the compositor never confirmed it, so it cannot be
// unlocked; Cancel withdraws it instead.
//
// A client that exits right after unlocking must round-trip the
// display first, or the compositor may never process the request.
func (l *Lock) Unlock() error {
	switch l.state {
	case Pending:
		return ErrNotLocked
	case Locked:
	default:
		return ErrEnded
	}
	if err := l.req.UnlockAndDestroy(); err != nil {
		return fmt.Errorf("sessionlock: unlock_and_destroy: %w", err)
	}
	debug.Log("shell", "session lock: unlock_and_destroy sent")
	l.state = Unlocked
	l.closeSurfaces()
	return nil
}

// Cancel withdraws a lock the compositor has not confirmed yet.
// Refused with ErrLocked once locked arrived: a locked session only
// ends through Unlock.
func (l *Lock) Cancel() error {
	switch l.state {
	case Pending:
	case Locked:
		return ErrLocked
	default:
		return ErrEnded
	}
	if err := l.req.Destroy(); err != nil {
		return fmt.Errorf("sessionlock: destroy: %w", err)
	}
	l.state = Cancelled
	l.closeSurfaces()
	return nil
}

// closeSurfaces destroys every live lock surface.
func (l *Lock) closeSurfaces() {
	for _, s := range l.outputs {
		s.Close()
	}
}

// Surface is one ext_session_lock_surface_v1 and its configure
// handshake. It implements the host contract the app layer draws
// through: usable only after the first acked configure, sized exactly
// as configured.
type Surface struct {
	WLSurface *wl.Surface

	req    surfaceAPI
	lock   *Lock
	out    *wl.Output
	acked  bool
	closed bool
	width  uint32
	height uint32
}

// HandleSessionLockSurfaceV1Configure implements the configure
// handler: record the exact size and ack the serial.
func (s *Surface) HandleSessionLockSurfaceV1Configure(ev wlr.SessionLockSurfaceV1ConfigureEvent) {
	s.configure(ev.Serial, ev.Width, ev.Height)
}

// configure applies one configure: the size is an exact requirement,
// so both axes are taken as sent. A closed surface ignores late
// configures (acking a destroyed object is a protocol error).
func (s *Surface) configure(serial, width, height uint32) {
	if s.closed {
		return
	}
	s.width, s.height = width, height
	if err := s.req.AckConfigure(serial); err == nil {
		s.acked = true
	}
}

// EnsureUsable gates drawing: nothing may commit before the first
// configure was acked, and a closed surface never draws again.
func (s *Surface) EnsureUsable() error {
	if s.closed {
		return ErrClosed
	}
	if !s.acked {
		return ErrNotConfigured
	}
	return nil
}

// Closed reports whether the surface was destroyed.
func (s *Surface) Closed() bool { return s.closed }

// Size returns the last configured size in surface (logical) pixels;
// buffers must match it exactly. Zero before the first configure.
func (s *Surface) Size() (int, int) { return int(s.width), int(s.height) }

// HostSurface returns the underlying wl_surface.
func (s *Surface) HostSurface() *wl.Surface { return s.WLSurface }

// Close destroys the lock surface. On a still-locked session the
// compositor falls back to a solid color on that output — the
// behavior the protocol prescribes for a removed output's surface.
// Idempotent.
func (s *Surface) Close() {
	if s.closed {
		return
	}
	s.closed = true
	_ = s.req.Destroy()
	if s.lock != nil && s.lock.outputs[s.out] == s {
		delete(s.lock.outputs, s.out)
	}
}
