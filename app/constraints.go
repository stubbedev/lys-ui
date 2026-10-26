package app

import (
	"image"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// PointerConstraint is a pointer lock or confinement on one host
// (Application.LockPointer, ConfinePointer). The compositor activates
// it while the pointer is over the host; OnActive hears that. The
// compositor never force-releases a constraint on a key of its own,
// so an application that locks the pointer must offer the way out -
// the convention is an accelerator (Escape, say) that calls Release.
type PointerConstraint struct{ c *wlsession.PointerConstraint }

// Active reports whether the constraint is in effect.
func (p *PointerConstraint) Active() bool { return p.c.Active() }

// OnActive registers fn to hear each activation and deactivation, on
// the loop goroutine.
func (p *PointerConstraint) OnActive(fn func(active bool)) { p.c.OnActive = fn }

// SetCursorHint asks for the cursor to reappear at (x, y) in the
// host's surface when a lock ends; it applies with the next frame.
func (p *PointerConstraint) SetCursorHint(x, y float64) { p.c.SetCursorHint(x, y) }

// Release ends the constraint. Closing the host releases it too.
func (p *PointerConstraint) Release() { p.c.Release() }

// PointerConstraintsAvailable reports whether the compositor supports
// pointer locking and confinement.
func (a *Application) PointerConstraintsAvailable() bool {
	return a.sess.PointerConstraintsAvailable()
}

// RelativePointerAvailable reports whether the compositor delivers
// relative pointer motion.
func (a *Application) RelativePointerAvailable() bool { return a.sess.RelativePointerAvailable() }

// LockPointer locks the pointer in place while it is over host - an
// FPS-style view, a whiteboard pan - after which motion arrives only
// through OnRelativeMotion. persistent keeps the lock for every return
// of the pointer; otherwise it ends with its first deactivation. One
// constraint per host at a time.
func (a *Application) LockPointer(host Host, persistent bool) (*PointerConstraint, error) {
	c, err := a.sess.LockPointer(host.HostSurface(), persistent)
	if err != nil {
		return nil, err
	}
	return &PointerConstraint{c: c}, nil
}

// ConfinePointer keeps the pointer inside region (logical rects of the
// host; nil confines to the whole host) while it is over host.
func (a *Application) ConfinePointer(host Host, region []render.Rect, persistent bool) (*PointerConstraint, error) {
	var rects []image.Rectangle
	for _, r := range region {
		rects = append(rects, image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H))
	}
	c, err := a.sess.ConfinePointer(host.HostSurface(), rects, persistent)
	if err != nil {
		return nil, err
	}
	return &PointerConstraint{c: c}, nil
}

// OnRelativeMotion registers fn to receive the pointer's relative
// motion while it is over host (locked or not): the accelerated deltas
// and the raw ones before acceleration - what a remote desktop client
// forwards. Nil unregisters.
func (a *Application) OnRelativeMotion(host Host, fn func(dx, dy, rawDX, rawDY float64)) {
	if hw := a.hostWindowOf(host); hw != nil && hw.input != nil {
		hw.input.onRelative = fn
	}
}
