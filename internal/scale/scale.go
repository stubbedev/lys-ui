// Package scale owns one surface's buffer scaling: the integer
// wl_surface fallback, the optional wp_viewporter and
// wp_fractional_scale_v1 objects, and the output transform. Compositors
// without the fractional-scale protocols keep the exact integer
// behavior; feature detection happens at bind time in internal/wlsession.
//
// With the protocols present, a surface scales fractionally by giving
// the compositor a wp_viewport destination in logical pixels and
// attaching buffers of ceil(logical * preferred/120) device pixels; the
// wl_surface buffer scale stays 1, per the fractional-scale protocol.
// preferred_scale is 120-based: 150 is 1.25, 240 is 2.
package scale

import (
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// Denom is the fixed denominator of the 120-based fractional-scale wire
// units.
const Denom = 120

// surfaceWire is the wl_surface half of the scale state; an interface so
// tests can record the requests.
type surfaceWire interface {
	SetBufferScale(int32) error
	SetBufferTransform(int32) error
}

// viewportWire is the wp_viewport half; nil without wp_viewporter.
type viewportWire interface {
	SetDestination(width, height int32) error
}

// Controller is one surface's scale state. The zero value is unusable;
// build with New.
type Controller struct {
	surf     surfaceWire
	viewport viewportWire
	frac     *wlr.WpScaleV1
	// onPreferred receives every preferred_scale event, including the
	// initial one the compositor sends right after get_fractional_scale.
	onPreferred func(frac120 uint32)
}

// New wires surf for scaling on sess. With the fractional-scale protocol
// it attaches wp_viewport and wp_fractional_scale objects and reports
// every preferred_scale event through fn; without them the controller
// falls back to integer wl_surface.set_buffer_scale and fn never fires.
// A missing protocol object (bind or creation failure) degrades the same
// way, per-feature.
//
// Only one Controller may exist per surface: wp_viewporter allows a
// single wp_viewport per wl_surface, and a second get_viewport is a
// fatal protocol error that kills the connection (it once took down
// every tooltip, via a duplicate controller on the popup surface).
// Surfaces owned by popup.Popup get their controller from Popup.Scale.
func New(sess *wlsession.Session, surf *wl.Surface, fn func(frac120 uint32)) *Controller {
	c := &Controller{surf: surf, onPreferred: fn}
	if sess == nil || surf == nil {
		return c
	}
	if vp := sess.Viewporter(); vp != nil {
		if v, err := vp.GetViewport(surf); err == nil {
			c.viewport = v
		}
	}
	if mgr := sess.FractionalScaleManager(); mgr != nil {
		if f, err := mgr.GetScale(surf); err == nil {
			c.frac = f
			f.AddPreferredScaleHandler(c)
		}
	}
	return c
}

// HandleWpScaleV1PreferredScale implements the wp_fractional_scale_v1
// preferred_scale handler.
func (c *Controller) HandleWpScaleV1PreferredScale(ev wlr.WpScaleV1PreferredScaleEvent) {
	if c.onPreferred != nil && ev.Scale > 0 {
		c.onPreferred(ev.Scale)
	}
}

// Fractional reports whether the surface follows the fractional-scale
// protocol; false means the integer set_buffer_scale fallback.
func (c *Controller) Fractional() bool { return c.viewport != nil || c.frac != nil }

// SetTransform publishes the buffer transform (an output's rotation, as
// wl_output.geometry reports it) the surface's buffers are submitted
// for. It is double-buffered state, applied at the next commit.
func (c *Controller) SetTransform(transform int32) error {
	return c.surf.SetBufferTransform(transform)
}

// Apply publishes device scale frac120 (120-based) for a surface of
// logical size w x h. Fractional surfaces get their viewport destination
// set (the buffer scale stays 1); integer surfaces get the rounded-up
// buffer scale. It is a no-op while the surface has no size yet - the
// destination must be positive - so callers re-apply once the first
// configure arrived.
func (c *Controller) Apply(frac120 uint32, w, h int) error {
	if c.surf == nil || w <= 0 || h <= 0 {
		return nil
	}
	if c.viewport != nil {
		return c.viewport.SetDestination(intconv.Int32(w), intconv.Int32(h))
	}
	return c.surf.SetBufferScale(intconv.Int32(max(1, DeviceSize(1, frac120))))
}

// DeviceSize rounds the logical size v up to device pixels at the
// 120-based scale frac120, so the buffer always fully covers the
// destination.
func DeviceSize(v int, frac120 uint32) int {
	if frac120 == 0 {
		frac120 = Denom
	}
	return (v*int(frac120) + Denom - 1) / Denom
}

// IntegerScale rounds a 120-based scale up to the integer fallback:
// 150 (1.25) needs a 2x integer buffer scale without the viewporter.
func IntegerScale(frac120 uint32) int {
	return max(1, DeviceSize(1, frac120))
}
