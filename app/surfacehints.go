package app

import (
	"errors"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// ContentType hints what a surface shows (wp_content_type_v1), so the
// compositor may tune for it: a photo viewer, a video, a game.
type ContentType = wlsession.ContentType

// Content types.
const (
	ContentNone  = wlsession.ContentNone
	ContentPhoto = wlsession.ContentPhoto
	ContentVideo = wlsession.ContentVideo
	ContentGame  = wlsession.ContentGame
)

// WMCapabilities is what the compositor's window management offers a
// window; Known is false until it said, and until then everything
// counts as available.
type WMCapabilities = window.Capabilities

// WindowState is a toplevel's compositor-confirmed state: maximized,
// fullscreen, activated, tiled edges and so on (Window.State).
type WindowState = window.State

// SetContentType hints the window's content kind; it applies with the
// next frame. Fails when the compositor lacks the protocol.
func (w *Window) SetContentType(ct ContentType) error {
	w.cfg.ContentType = ct
	err := w.app.sess.SetContentType(w.win.WLSurface, ct)
	w.requestFrame()
	return err
}

// SetOpacity sets the opacity the compositor applies to the whole
// window (0..1) - a fade with no repaint (wp_alpha_modifier_v1). It
// applies with the next frame; fails when the compositor lacks the
// protocol.
func (w *Window) SetOpacity(alpha float64) error {
	w.opacity.setApp(alpha)
	return w.sendOpacity(w.opacity)
}

// sendOpacity sends o, the app's opacity times the fade.
func (w *Window) sendOpacity(o surfaceOpacity) error {
	v, _ := o.value()
	err := w.app.sess.SetSurfaceAlpha(w.win.WLSurface, v)
	w.requestFrame()
	return err
}

// SetBackgroundBlur turns the compositor blur behind the window on or
// off (WindowConfig.BackgroundBlur); it applies with the next frame.
func (w *Window) SetBackgroundBlur(on bool) {
	w.cfg.BackgroundBlur = on
	if hw := w.app.hostWindowOf(w); hw != nil {
		hw.cfg.blur = on
		hw.dirty = true
	}
}

// BackgroundBlurActive reports whether the compositor blurs behind the
// window now: asked for, offered, and its region sent.
func (w *Window) BackgroundBlurActive() bool {
	hw := w.app.hostWindowOf(w)
	return hw != nil && hw.blurActive()
}

// Capabilities reports the compositor's window-management offer.
func (w *Window) Capabilities() WMCapabilities { return w.win.Capabilities() }

// WhenPresented runs fn on the loop goroutine once the window's first commit after this call is presented.
func (w *Window) WhenPresented(fn func()) error {
	hw := w.app.hostWindowOf(w)
	if hw == nil {
		return errors.New("app: WhenPresented on a window that has no surface")
	}
	hw.whenNextPresented(fn)
	return nil
}

// requestFrame marks the window for a frame (the commit that applies
// surface state).
func (w *Window) requestFrame() {
	if hw := w.app.hostWindowOf(w); hw != nil {
		hw.dirty = true
		hw.stateOwed = true
	}
}

// SetContentType hints the layer surface's content kind.
func (l *LayerWindow) SetContentType(ct ContentType) error {
	l.cfg.ContentType = ct
	err := l.app.sess.SetContentType(l.ls.WLSurface, ct)
	l.requestFrame()
	return err
}

// SetOpacity sets the opacity the compositor applies to the layer
// surface (a panel fading out, no repaint).
func (l *LayerWindow) SetOpacity(alpha float64) error {
	l.opacity.setApp(alpha)
	return l.sendOpacity(l.opacity)
}

// sendOpacity sends o, the app's opacity times the fade.
func (l *LayerWindow) sendOpacity(o surfaceOpacity) error {
	v, _ := o.value()
	err := l.app.sess.SetSurfaceAlpha(l.ls.WLSurface, v)
	l.requestFrame()
	return err
}

// SetBackgroundBlur turns the compositor blur behind the layer surface
// on or off (LayerConfig.BackgroundBlur); it applies with the next
// frame.
func (l *LayerWindow) SetBackgroundBlur(on bool) {
	l.cfg.BackgroundBlur = on
	if hw := l.app.hostWindowOf(l); hw != nil {
		hw.cfg.blur = on
		hw.dirty = true
	}
}

// BackgroundBlurActive reports whether the compositor blurs behind the
// layer surface now.
func (l *LayerWindow) BackgroundBlurActive() bool {
	hw := l.app.hostWindowOf(l)
	return hw != nil && hw.blurActive()
}

// BackgroundBlurAvailable reports whether the compositor offers
// background blur (ext_background_effect_v1 with its blur capability):
// what an app checks to pick a translucent or an opaque style.
// Hyprland has no client protocol for it; its users' window and layer
// rules decide instead.
func (a *Application) BackgroundBlurAvailable() bool { return a.sess.BlurAvailable() }

// requestFrame marks the layer surface for a frame.
func (l *LayerWindow) requestFrame() {
	if hw := l.app.hostWindowOf(l); hw != nil {
		hw.dirty = true
		hw.stateOwed = true
	}
}

// Bell rings the system bell for host (nil: the application's).
func (a *Application) Bell(host Host) error {
	if host == nil {
		return a.sess.Bell(nil)
	}
	return a.sess.Bell(host.HostSurface())
}

// ringFor is the widget error bell: it rings the window holding w.
func (a *Application) ringFor(w widget.Widget) {
	if hw := a.windowOf(w); hw != nil {
		_ = a.sess.Bell(hw.host.HostSurface())
		return
	}
	_ = a.sess.Bell(nil)
}

func (a *Application) fadeSurface(root widget.Widget, alpha float64) bool {
	hw := a.windowOf(root)
	if hw == nil || hw.styleRoot != root {
		return false
	}
	switch {
	case hw.win != nil:
		return fadeOpacity(&hw.win.opacity, alpha, hw.win.sendOpacity)
	case hw.layer != nil:
		return fadeOpacity(&hw.layer.opacity, alpha, hw.layer.sendOpacity)
	}
	return false
}

// initialHints applies a config's surface hints before the initial
// commit; a compositor without the protocol just doesn't get them.
func (a *Application) initialHints(surf *wl.Surface, ct ContentType) {
	if ct != ContentNone {
		_ = a.sess.SetContentType(surf, ct)
	}
}

// OnCapabilities adds fn to what hears the compositor's
// window-management offer change (each wm_capabilities event); the
// window repaints after.
func (w *Window) OnCapabilities(fn func(WMCapabilities)) {
	prev := w.win.OnCapabilities
	w.win.OnCapabilities = func(c WMCapabilities) {
		if prev != nil {
			prev(c)
		}
		fn(c)
		w.requestFrame()
	}
}
