package app

import (
	"slices"

	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// Toplevel is one window the compositor lists through
// zwlr_foreign_toplevel_management_v1 on the application's own
// connection, so its entry can name a rectangle on one of the
// application's surfaces. Two Toplevels are the same window exactly when
// they are equal. Read it and send its requests on the loop.
type Toplevel struct{ t *wlsession.Toplevel }

// ToplevelsAvailable reports whether the compositor offers the
// foreign-toplevel manager; without it Toplevels is always empty.
func (a *Application) ToplevelsAvailable() bool { return a.sess.ForeignToplevelAvailable() }

// Toplevels is every window the compositor lists, the application's own
// included, in the order they appeared.
func (a *Application) Toplevels() []Toplevel {
	listed := a.sess.Toplevels()
	out := make([]Toplevel, len(listed))
	for i, t := range listed {
		out[i] = Toplevel{t}
	}
	return out
}

// OnToplevelsChanged calls fn on the loop after a window appears, closes
// or applies a batch of changes. The returned stop ends the calls.
func (a *Application) OnToplevelsChanged(fn func()) (stop func()) {
	w := &fn
	a.toplevelWatchers = append(a.toplevelWatchers, w)
	return func() {
		a.toplevelWatchers = slices.DeleteFunc(a.toplevelWatchers, func(o *func()) bool { return o == w })
	}
}

func (a *Application) toplevelsChanged(*wlsession.Toplevel) {
	gen := widget.DamageGeneration()
	for _, w := range slices.Clone(a.toplevelWatchers) {
		(*w)()
	}
	a.markDirtyIfDamaged(gen)
}

// Title is the window's title.
func (t Toplevel) Title() string { return t.t.Title }

// AppID is the window's application id.
func (t Toplevel) AppID() string { return t.t.AppID }

// Minimized reports whether the window is minimized.
func (t Toplevel) Minimized() bool { return t.t.Minimized }

// Activated reports whether the window has the focus.
func (t Toplevel) Activated() bool { return t.t.Activated }

// Fullscreen reports whether the window is fullscreen: a video, a game
// or a presentation a shell keeps its notifications from covering.
func (t Toplevel) Fullscreen() bool { return t.t.Fullscreen }

// OnOutput reports whether the window is on the output with this
// xdg-output name.
func (t Toplevel) OnOutput(name string) bool {
	return slices.ContainsFunc(t.t.Outputs, func(o *Output) bool { return o.Name == name })
}

// Activate focuses the window, bringing it back when minimized.
func (t Toplevel) Activate() { t.t.Activate() }

// Minimize minimizes the window.
func (t Toplevel) Minimize() { t.t.SetMinimized() }

// Restore brings a minimized window back.
func (t Toplevel) Restore() { t.t.UnsetMinimized() }

// Close asks the window to close.
func (t Toplevel) Close() { t.t.Close() }

// SetRectangle names r, in host's surface-local logical pixels, as where
// the window's entry sits, so the compositor's minimize animation heads
// for it; an empty r withdraws it. A host without a surface sends
// nothing.
func (t Toplevel) SetRectangle(host Host, r render.Rect) {
	if r.Empty() {
		r = render.Rect{}
	}
	t.t.SetRectangle(host.HostSurface(), intconv.Clamp[int32](r.X), intconv.Clamp[int32](r.Y), intconv.Clamp[int32](r.W), intconv.Clamp[int32](r.H))
}
