package app

import (
	"errors"
	"fmt"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/internal/sessionlock"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// Session locking (ext-session-lock-v1): the secure lock-screen
// protocol. While a lock holds, the compositor shows only the lock
// surfaces — one per output — and routes input only to them; if the
// client dies, the session stays locked. The protocol's client-side
// rules are enforced in internal/sessionlock; this file maps them onto
// the application loop: one hosted widget tree per output, output
// hotplug followed for the lock's lifetime, and the loop kept alive
// while a lock exists even with no surface mapped (an unplugged last
// output must not end the process that is responsible for unlocking).

// ErrSessionLockUnavailable reports a compositor without
// ext_session_lock_manager_v1: the session cannot be locked securely.
var ErrSessionLockUnavailable = errors.New("app: compositor has no ext_session_lock_manager_v1")

// ErrSessionLockActive reports a LockSession while this application
// already holds a lock (pending or locked).
var ErrSessionLockActive = errors.New("app: a session lock is already active")

// Errors from SessionLock.Unlock and SessionLock.Cancel.
var (
	// ErrNotLocked: Unlock before the compositor confirmed the lock.
	ErrNotLocked = sessionlock.ErrNotLocked
	// ErrLocked: Cancel after the compositor confirmed the lock; a
	// locked session only ends through Unlock.
	ErrLocked = sessionlock.ErrLocked
	// ErrLockEnded: the lock was already unlocked, cancelled, or
	// finished by the compositor.
	ErrLockEnded = sessionlock.ErrEnded
)

// LockState is a session lock's lifecycle state.
type LockState = sessionlock.State

// Session lock lifecycle states.
const (
	LockPending   = sessionlock.Pending
	LockLocked    = sessionlock.Locked
	LockUnlocked  = sessionlock.Unlocked
	LockCancelled = sessionlock.Cancelled
	LockFinished  = sessionlock.Finished
)

// LockSurface is the content of one output's lock surface.
type LockSurface struct {
	// Root is the surface's widget tree; build a fresh one per call.
	Root widget.Widget
	// Background fills the frame before the tree paints. Keep it
	// opaque (alpha 255): the lock surface is what hides the session.
	Background render.Color
	// OnPress, OnPointerMove, and OnKey mirror LayerConfig.
	OnPress       func(button uint32, over widget.Widget)
	OnPointerMove func(x, y float64)
	OnKey         func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	// OnClosed runs when this surface went away: the lock ended, or
	// its output was unplugged.
	OnClosed func()
	// Focus, when set, takes keyboard focus as the surface is created
	// (the password entry), so typing lands without a click.
	Focus widget.Widget
}

// SessionLockConfig declares a session lock.
type SessionLockConfig struct {
	// Surface builds the lock surface for one output. It runs for
	// every output present at LockSession and again for each output
	// plugged in while the lock lives — a reconnected monitor without
	// a lock surface would show the compositor's fallback color. Must
	// not be nil.
	Surface func(out *Output) LockSurface
	// OnLocked fires once the compositor confirmed the lock: every
	// output shows lock content and nothing unlocked is visible. Safe
	// to suspend from here.
	OnLocked func()
	// OnFinished fires when the compositor ended the lock instead:
	// it denied it (another client holds the session lock) or ended a
	// locked session by its own policy. Every surface is gone and the
	// lock handle is spent.
	OnFinished func()
}

// SessionLock is the application's handle on one session lock.
type SessionLock struct {
	app      *Application
	cfg      SessionLockConfig
	lock     *sessionlock.Lock
	stop     func()
	surfaces map[*Output]*hostWindow
}

// SessionLockAvailable reports whether the compositor offers
// ext-session-lock-v1.
func (a *Application) SessionLockAvailable() bool { return a.sess.SessionLockManager() != nil }

// SessionLock returns the lock this application holds, nil when none
// is pending or locked.
func (a *Application) SessionLock() *SessionLock { return a.sessionLock }

// LockSession asks the compositor to lock the session and creates a
// lock surface on every output (and on every output plugged in later,
// until the lock ends). The compositor answers asynchronously:
// OnLocked once the lock holds, OnFinished if it refused. One lock at
// a time: a second LockSession while one is active fails with
// ErrSessionLockActive.
func (a *Application) LockSession(cfg SessionLockConfig) (*SessionLock, error) {
	if cfg.Surface == nil {
		return nil, errors.New("app: SessionLockConfig.Surface is required")
	}
	if a.sessionLock != nil {
		return nil, ErrSessionLockActive
	}
	mgr := a.sess.SessionLockManager()
	if mgr == nil {
		return nil, ErrSessionLockUnavailable
	}
	l := &SessionLock{app: a, cfg: cfg, surfaces: make(map[*Output]*hostWindow)}
	lock, err := sessionlock.New(mgr, sessionlock.Hooks{
		Locked:   l.onLocked,
		Finished: l.onFinished,
	})
	if err != nil {
		return nil, err
	}
	l.lock = lock
	a.sessionLock = l
	// Every output right away, so the compositor can present the lock
	// content in its first locked frame instead of blanking first.
	for _, out := range a.sess.Outputs() {
		l.cover(out)
	}
	l.stop = a.sess.WatchOutputs(l.cover, l.uncover)
	a.sess.WakeAfter(0)
	return l, nil
}

// State returns the lock's lifecycle state.
func (l *SessionLock) State() LockState { return l.lock.State() }

// Locked reports whether the compositor confirmed the lock and it has
// not ended.
func (l *SessionLock) Locked() bool { return l.lock.State() == sessionlock.Locked }

// Unlock ends a locked session: the compositor unlocks and every lock
// surface is destroyed. It fails with ErrNotLocked while the lock is
// still pending — only a confirmed lock can be unlocked — and with
// ErrLockEnded once the lock is over. A process that exits right
// after unlocking must call Session.Roundtrip first (outside the
// event loop), or the compositor may never see the request.
func (l *SessionLock) Unlock() error {
	if err := l.lock.Unlock(); err != nil {
		return err
	}
	l.end()
	return nil
}

// Cancel withdraws a lock the compositor has not confirmed yet. It
// fails with ErrLocked once the lock holds: a locked session only
// ends through Unlock.
func (l *SessionLock) Cancel() error {
	if err := l.lock.Cancel(); err != nil {
		return err
	}
	l.end()
	return nil
}

// SetFocus moves keyboard focus to w in the lock surface whose tree
// holds it (a password entry, after a failed attempt re-enabled it);
// see widget.Router.SetFocus for what is ignored. The compositor still
// decides which lock surface holds the keyboard.
func (l *SessionLock) SetFocus(w widget.Widget) {
	for _, hw := range l.surfaces {
		hw.router.SetFocus(w)
		hw.dirty = true
	}
}

// Outputs returns the outputs currently covered by a lock surface.
func (l *SessionLock) Outputs() []*Output {
	outs := make([]*Output, 0, len(l.surfaces))
	for _, out := range l.app.sess.Outputs() {
		if _, ok := l.surfaces[out]; ok {
			outs = append(outs, out)
		}
	}
	return outs
}

func (l *SessionLock) onLocked() {
	if l.cfg.OnLocked != nil {
		l.cfg.OnLocked()
	}
}

// onFinished runs after internal/sessionlock released the lock and
// closed its surfaces.
func (l *SessionLock) onFinished() {
	l.end()
	if l.cfg.OnFinished != nil {
		l.cfg.OnFinished()
	}
}

// end stops following hotplug and hands the application back its
// no-lock state; the surfaces are already closed, and the next loop
// pass reaps their windows.
func (l *SessionLock) end() {
	if l.stop != nil {
		l.stop()
		l.stop = nil
	}
	l.surfaces = make(map[*Output]*hostWindow)
	if l.app.sessionLock == l {
		l.app.sessionLock = nil
	}
	l.app.sess.WakeAfter(0)
}

// cover creates the lock surface for out. A failure is logged and the
// output left uncovered: the compositor shows its fallback color there,
// and the output stays locked.
func (l *SessionLock) cover(out *Output) {
	if out == nil || out.WL == nil || l.lock.State().Ended() {
		return
	}
	if _, ok := l.surfaces[out]; ok {
		return
	}
	hw, err := l.app.newLockWindow(l.lock, out, l.cfg.Surface(out))
	if err != nil {
		logutil.L().Warn("app: session lock: lock surface", "output", out.Name, "err", err)
		return
	}
	l.surfaces[out] = hw
}

// uncover destroys the lock surface of an unplugged output, as the
// protocol recommends.
func (l *SessionLock) uncover(out *Output) {
	hw, ok := l.surfaces[out]
	if !ok {
		return
	}
	delete(l.surfaces, out)
	if lh, ok := hw.host.(*lockHost); ok {
		lh.s.Close()
	}
	l.app.sess.WakeAfter(0)
}

// newLockWindow creates one lock surface and its loop state. Unlike a
// layer surface there is no initial commit: the compositor sends the
// first configure unprompted, and committing before acking it is the
// commit_before_first_ack protocol error — the host stays unusable
// (nothing draws, nothing commits) until the ack.
func (a *Application) newLockWindow(lock *sessionlock.Lock, out *Output, content LockSurface) (*hostWindow, error) {
	surf, err := a.sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("app: create surface: %w", err)
	}
	ls, err := lock.NewSurface(surf, out.WL)
	if err != nil {
		return nil, err
	}
	scale := out.Scale
	if scale == 0 {
		scale = 1
	}
	// KindMenu: no enter/exit tween. The first frame must be the
	// finished lock content, and teardown on unlock is immediate.
	hw := a.newWindow(&lockHost{s: ls}, scale, content.Root, windowHooks{
		background: content.Background,
		opaque:     opaqueFor(content.Background, false),
		onPress:    content.OnPress,
		onMove:     content.OnPointerMove,
		onKey:      content.OnKey,
	}, content.OnClosed, surfx.KindMenu)
	if out.Transform != 0 && hw.sc != nil {
		_ = hw.sc.SetTransform(out.Transform)
	}
	if content.Focus != nil {
		hw.router.SetFocus(content.Focus)
	}
	return hw, nil
}

// lockHost adapts a lock surface to Host.
type lockHost struct{ s *sessionlock.Surface }

func (h *lockHost) EnsureUsable() error      { return h.s.EnsureUsable() }
func (h *lockHost) Closed() bool             { return h.s.Closed() }
func (h *lockHost) Size() (int, int)         { return h.s.Size() }
func (h *lockHost) HostSurface() *wl.Surface { return h.s.HostSurface() }
