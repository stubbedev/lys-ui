// Reconnect-with-rebuild (#117): a compositor restart kills the
// connection and every object on it, but none of the application's own
// state - widget trees, Invoke/Every queues, accelerators, the windows'
// declarative configs. With SetReconnect, a lost connection tears down
// what was wire-owned, dials the compositor again, and rebuilds every
// window the application opened itself on the new session, keeping
// every *Window and *LayerWindow handle valid. A panel keeps its
// scroll positions, expanded sections, and typed text across a sway
// reload.
package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/dragdrop"
	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// ReconnectOptions turn a lost compositor connection into a rebuild
// instead of the clean exit.
type ReconnectOptions struct {
	// Timeout is how long to keep dialing while the compositor comes
	// back; zero is ten seconds. Past it, Run returns the disconnect
	// error as without reconnect.
	Timeout time.Duration
	// Connect dials the new session; nil is wlsession.Connect (the
	// WAYLAND_DISPLAY the process started with).
	Connect func() (*wlsession.Session, error)
	// OnReconnected, optional, runs on the loop goroutine once every
	// window is rebuilt on the new session.
	OnReconnected func()
}

// defaultReconnectTimeout is how long a reconnect keeps dialing.
const defaultReconnectTimeout = 10 * time.Second

// SetReconnect enables reconnect-with-rebuild (nil disables it, the
// default: the clean exit with DisconnectExitCode). It applies to a
// connection lost without a protocol verdict (a compositor that killed
// the client for a protocol error would do so again) and to
// applications whose every window came from NewWindow, NewLayer, or a
// dialog - a host built outside the application cannot be rebuilt, and
// its loop exits as before.
//
// What survives: widget trees and everything in them (text, scroll,
// selection, focus within each window), window handles, titles, sizes,
// limits, maximized and fullscreen requests, transient parents, dialog
// modality, layer anchors/margins/zones/layers, the output a layer was
// pinned to (re-resolved by name), content types, opacity, queues,
// timers, accelerators, and the clipboard handle. What does not: the
// compositor's own state - clipboard contents, drags, open popovers
// and tooltips (closed, OnClosed firing), session locks, idle
// inhibitors and idle watches, pointer constraints, and pending
// configure serials. Code that holds the *wlsession.Session itself must
// read the current one from Application.Session after a reconnect.
func (a *Application) SetReconnect(opts *ReconnectOptions) { a.reconnect = opts }

// Session is the application's current Wayland session - a new one
// after each reconnect.
func (a *Application) Session() *wlsession.Session { return a.sess }

// KeySym translates a key code through the current session's keymap -
// what OnKey handlers use, so a handler never holds a session a
// reconnect replaced.
func (a *Application) KeySym(code uint32) xkb.Keysym { return a.sess.KeySym(code) }

// rebuildable reports whether every window can be rebuilt on a new
// session: each one an application-made window or layer.
func (a *Application) rebuildable() bool {
	for _, w := range a.windows {
		if w.win == nil && w.layer == nil {
			return false
		}
	}
	return true
}

// reconnectAfter runs the reconnect a disconnect planned: dial the
// compositor again, adopt the new session, rebuild the planned
// windows. True when the loop may go on; false leaves err as Run's
// result.
func (a *Application) reconnectAfter(err error) bool {
	plan := a.rebuildPlan
	a.rebuildPlan = nil
	if plan == nil || a.reconnect == nil || !errors.Is(err, ErrDisconnected) {
		return false
	}
	sess, derr := a.redial()
	if derr != nil {
		debug.Log("wire", "app: reconnect failed: %v", derr)
		a.dropCarriedToasts()
		return false
	}
	a.adoptSession(sess)
	if gerr := a.reagreeRenderer(); gerr != nil {
		debug.Log("wire", "app: reconnect failed: %v", gerr)
		a.dropCarriedToasts()
		sess.Close()
		return false
	}
	if rerr := a.rebuildWindows(plan); rerr != nil {
		debug.Log("wire", "app: rebuild failed: %v", rerr)
		a.dropCarriedToasts()
		for _, w := range a.windows {
			a.toasts.closeHost(w.host)
			w.release()
		}
		a.windows = nil
		sess.Close()
		return false
	}
	a.disconnectOnce.Store(false)
	debug.Log("wire", "app: reconnected, %d windows rebuilt", len(plan))
	if a.reconnect.OnReconnected != nil {
		a.reconnect.OnReconnected()
	}
	return true
}

// redial connects until the compositor answers or the timeout passes,
// backing off from 50ms to a second between attempts.
func (a *Application) redial() (*wlsession.Session, error) {
	connect := a.reconnect.Connect
	if connect == nil {
		connect = wlsession.Connect
	}
	deadline := time.Now().Add(orDefault(a.reconnect.Timeout, defaultReconnectTimeout))
	backoff := 50 * time.Millisecond
	for {
		sess, err := connect()
		if err == nil {
			return sess, nil
		}
		if time.Now().Add(backoff).After(deadline) {
			return nil, fmt.Errorf("app: no compositor within the reconnect timeout: %w", err)
		}
		time.Sleep(backoff)
		backoff = min(2*backoff, time.Second)
	}
}

// adoptSession re-seats everything session-bound on sess: the
// dispatch and wake seams, drag and drop, the input method, key
// repeat, the clipboard handle, and the session hooks.
func (a *Application) adoptSession(sess *wlsession.Session) {
	a.sess = sess
	a.step, a.wake = sess.Step, sess.WakeAfter
	a.dnd = dragdrop.New(sess)
	a.ime = newIMEController(sess)
	a.rep = newKeyRepeater(sess.RepeatInfo())
	a.kicker = &loopKicker{}
	if a.clip != nil {
		a.clip.Reset(sess)
	}
	a.dataControl = nil
	a.sessionLock = nil
	a.eyedropChecked = false
	a.wireSession()
}

// planRebuild records the windows to rebuild, sets their toasts
// aside, and drops the wire-only surfaces riding on them - popovers
// close (OnClosed fires) - before the teardown releases the windows
// themselves.
func (a *Application) planRebuild() []*hostWindow {
	plan := append([]*hostWindow(nil), a.windows...)
	// Toasts are the application's widgets: they ride over to the
	// rebuilt windows (rebuildWindows) instead of stopping.
	a.carriedToasts = map[*hostWindow]movedToasts{}
	for _, w := range plan {
		if m, ok := a.toasts.takeHost(w.host); ok {
			a.carriedToasts[w] = m
		}
	}
	for _, op := range a.openPopovers {
		op.release()
		op.fireClosed()
	}
	a.openPopovers = nil
	a.popovers = popoverRegistry{}
	return plan
}

// rebuildWindows opens every planned window again on the current
// session, in the original order, then re-links transient parents and
// dialog modality across the rebuilt set.
func (a *Application) rebuildWindows(plan []*hostWindow) error {
	// The parent links name the old toplevels: map them to their
	// handles before the rebuild replaces them.
	handleOf := map[*window.Window]*Window{}
	parents := map[*Window]*window.Window{}
	for _, old := range plan {
		if w := old.win; w != nil {
			handleOf[w.win] = w
			if p := w.win.Parent(); p != nil {
				parents[w] = p
			}
		}
	}
	for _, old := range plan {
		var err error
		switch {
		case old.win != nil:
			err = a.rebuildToplevel(old)
		case old.layer != nil:
			err = a.rebuildLayer(old)
		}
		if err != nil {
			return err
		}
	}
	for w, p := range parents {
		if ph := handleOf[p]; ph != nil {
			w.SetTransientFor(ph)
		}
	}
	for _, d := range a.dialogs {
		if d.cfg.Modal && d.win.win.Parent() != nil {
			if err := d.win.win.SetModal(a.sess.DialogManager(), true); err != nil {
				debug.Log("shell", "dialog modality hint: %v", err)
			}
		}
	}
	a.repostIcons()
	for _, old := range plan {
		if m, ok := a.carriedToasts[old]; ok {
			if hw := a.rebuiltOf(old); hw != nil {
				for _, t := range m.toasts {
					a.hostToast(hw, t, m.placement)
				}
			}
		}
	}
	a.carriedToasts = nil
	return nil
}

// rebuiltOf is the rebuilt loop record of a planned window.
func (a *Application) rebuiltOf(old *hostWindow) *hostWindow {
	for _, hw := range a.windows {
		if old.win != nil && hw.win == old.win || old.layer != nil && hw.layer == old.layer {
			return hw
		}
	}
	return nil
}

// rebuildToplevel reopens a toplevel from its config updated with
// what the old one carried at the disconnect.
func (a *Application) rebuildToplevel(old *hostWindow) error {
	w := old.win
	prev := w.win
	cfg := w.cfg
	cfg.Title = prev.Title()
	if pw, ph := prev.Size(); pw > 0 && ph > 0 {
		cfg.Width, cfg.Height = uint32(pw), uint32(ph)
	}
	minW, minH, maxW, maxH := prev.SizeLimits()
	cfg.MinWidth, cfg.MinHeight, cfg.MaxWidth, cfg.MaxHeight = uint32(minW), uint32(minH), uint32(maxW), uint32(maxH)
	cfg.Parent = nil // re-linked once every window is back
	state := prev.State()
	hw, err := a.openWindow(w, cfg)
	if err != nil {
		return fmt.Errorf("app: rebuild %q: %w", cfg.Title, err)
	}
	a.carryOver(old, hw)
	if state.Maximized {
		w.Maximize()
	}
	if state.Fullscreen {
		w.Fullscreen()
	}
	if w.alpha > 0 {
		_ = w.SetOpacity(w.alpha)
	}
	return nil
}

// rebuildLayer reopens a layer surface from its config updated with
// its current declarative state, on the same output by name.
func (a *Application) rebuildLayer(old *hostWindow) error {
	l := old.layer
	sc := l.ls.Config()
	cfg := l.cfg
	cfg.Layer, cfg.Anchor, cfg.Margin, cfg.ExclusiveZone = sc.Layer, sc.Anchor, sc.Margin, sc.ExclusiveZone
	cfg.Width, cfg.Height, cfg.Namespace = l.req[0], l.req[1], sc.Namespace
	cfg.Output = a.sameOutput(cfg.Output)
	l.cfg.Output = cfg.Output
	hw, err := a.openLayer(l, cfg)
	if err != nil {
		return fmt.Errorf("app: rebuild layer %q: %w", cfg.Namespace, err)
	}
	a.carryOver(old, hw)
	if l.alpha > 0 {
		_ = l.SetOpacity(l.alpha)
	}
	return nil
}

// carryOver moves loop-side state from a window's old loop record to
// its rebuilt one: dialog exemption, the modal block, and keyboard
// focus inside the tree.
func (a *Application) carryOver(old, hw *hostWindow) {
	hw.isDialog, hw.blocked = old.isDialog, old.blocked
	if f := old.router.Focused(); f != nil {
		hw.router.SetFocus(f)
	}
}

// sameOutput finds the output on the current session that old was -
// matched by its xdg-output name (DP-1) - or nil, the compositor's
// pick, when it has no name or is gone.
func (a *Application) sameOutput(old *wlsession.Output) *wlsession.Output {
	if old == nil {
		return nil
	}
	for _, o := range a.sess.Outputs() {
		if old.Name != "" && o.Name == old.Name {
			return o
		}
	}
	debug.Log("wire", "app: output %q gone after reconnect; the compositor picks", old.Name)
	return nil
}

// dropCarriedToasts stops the toasts a failed reconnect cannot place.
func (a *Application) dropCarriedToasts() {
	for _, m := range a.carriedToasts {
		for _, t := range m.toasts {
			t.Close()
		}
	}
	a.carriedToasts = nil
}
