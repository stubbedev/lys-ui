// Package blursync keeps a surface's compositor blur in step with what
// its widget tree asks for: the blur region on the wire when the
// compositor offers blur, css.BlurUnavailable on the root when it does
// not. Windows, layer surfaces and popups all go through it.
package blursync

import (
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

// Surface is the wire half of a blurred surface.
type Surface interface {
	// BlurAvailable reports whether the compositor blurs behind
	// surfaces at all right now.
	BlurAvailable() bool
	// SetBlurRegion asks the compositor to blur behind rects
	// (surface-local, logical pixels; none removes the effect), applying
	// with the next commit.
	SetBlurRegion(rects []render.Rect) error
}

// Sync is one surface's blur state. Like the opaque region it is
// surface state, re-sent only when the shape changes. The zero value
// has sent nothing.
type Sync struct {
	// sent is the region last sent, set whether one went out at all.
	sent []render.Rect
	set  bool
	// fallback is whether the root carries css.BlurUnavailable, so the
	// class flips only on a change.
	fallback bool
}

// Update publishes shape's result while want holds and the compositor
// offers blur, nothing otherwise. While blur is wanted but not in effect
// root carries css.BlurUnavailable, so its style can turn opaque rather
// than show the unblurred desktop. Call it before the frame's damage
// drain, so a class flip repaints that very frame, and before the commit
// the region should ride on.
func (s *Sync) Update(surf Surface, want bool, root widget.Widget, shape func() []render.Rect) {
	if !want && !s.set && !s.fallback {
		return
	}
	var rects []render.Rect
	active := want && surf.BlurAvailable()
	if active {
		rects = shape()
	}
	if fallback := want && !active; fallback != s.fallback {
		if c, ok := root.(interface {
			AddClass(...string)
			RemoveClass(...string)
		}); ok {
			if fallback {
				c.AddClass(css.BlurUnavailable)
			} else {
				c.RemoveClass(css.BlurUnavailable)
			}
		}
		s.fallback = fallback
	}
	if slices.Equal(rects, s.sent) && (s.set || rects == nil) {
		return
	}
	if err := surf.SetBlurRegion(rects); err != nil {
		debug.Log("frame", "blur region: %v", err)
		return
	}
	s.sent, s.set = slices.Clone(rects), true
	debug.Log("frame", "blur region: %d rects", len(rects))
}

// Active reports whether the compositor blurs behind the surface: a
// region is out and non-empty.
func (s *Sync) Active() bool { return s.set && len(s.sent) > 0 }
