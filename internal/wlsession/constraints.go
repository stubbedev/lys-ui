package wlsession

import (
	"errors"
	"image"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/wlnull"
)

// Pointer constraints (zwp_pointer_constraints_v1) and relative motion
// (zwp_relative_pointer_v1): locking the pointer in place or confining
// it to a region of a surface, and reading the device's raw motion -
// what pointer-locked views (games, a whiteboard pan) and remote
// desktop clients need. The compositor never force-releases a
// constraint on its own key: an application that locks must offer the
// way out (an accelerator that releases it).

// ErrPointerConstraintsUnavailable reports a compositor without
// pointer constraints, or no pointer to constrain.
var ErrPointerConstraintsUnavailable = errors.New("wlsession: pointer constraints unavailable")

// ErrAlreadyConstrained reports a second constraint on one surface,
// which the protocol forbids.
var ErrAlreadyConstrained = errors.New("wlsession: surface already constrained")

// SurfaceRelativeHandler is a SurfacePointerHandler that takes the
// device's relative motion: accelerated deltas, and the raw ones
// before pointer acceleration.
type SurfaceRelativeHandler interface {
	HandleRelativeMotion(dx, dy, rawDX, rawDY float64)
}

// PointerConstraint is one lock or confinement on a surface. The
// compositor activates it while the pointer is in the surface (and the
// region, if any); OnActive hears each activation and deactivation. A
// persistent constraint re-activates when the pointer returns; a
// oneshot one ends with its first deactivation. Release removes it.
type PointerConstraint struct {
	s        *Session
	surface  *wl.Surface
	locked   *wlr.ZwpLockedPointerV1
	confined *wlr.ZwpConfinedPointerV1
	active   bool

	// OnActive fires with the compositor's activation state.
	OnActive func(active bool)
}

// Active reports whether the compositor has the constraint in effect.
func (c *PointerConstraint) Active() bool { return c.active }

// SetCursorHint tells the compositor where the cursor should appear
// when a lock ends (surface coordinates); it applies on the surface's
// next commit. Confinements ignore it.
func (c *PointerConstraint) SetCursorHint(x, y float64) {
	if c.locked != nil {
		_ = c.locked.SetCursorPositionHint(float32(x), float32(y))
	}
}

// Release removes the constraint.
func (c *PointerConstraint) Release() {
	if c.locked != nil {
		_ = c.locked.Destroy()
	}
	if c.confined != nil {
		_ = c.confined.Destroy()
	}
	c.locked, c.confined = nil, nil
	if c.s.constraints[c.surface] == c {
		delete(c.s.constraints, c.surface)
	}
	if c.active {
		c.setActive(false)
	}
}

// setActive records and announces the activation state.
func (c *PointerConstraint) setActive(on bool) {
	c.active = on
	debug.Log("input", "pointer constraint active=%v", on)
	if c.OnActive != nil {
		c.OnActive(on)
	}
}

// HandleZwpLockedPointerV1Locked implements the locked handler.
func (c *PointerConstraint) HandleZwpLockedPointerV1Locked(wlr.ZwpLockedPointerV1LockedEvent) {
	c.setActive(true)
}

// HandleZwpLockedPointerV1Unlocked implements the unlocked handler.
func (c *PointerConstraint) HandleZwpLockedPointerV1Unlocked(wlr.ZwpLockedPointerV1UnlockedEvent) {
	c.setActive(false)
}

// HandleZwpConfinedPointerV1Confined implements the confined handler.
func (c *PointerConstraint) HandleZwpConfinedPointerV1Confined(wlr.ZwpConfinedPointerV1ConfinedEvent) {
	c.setActive(true)
}

// HandleZwpConfinedPointerV1Unconfined implements the unconfined handler.
func (c *PointerConstraint) HandleZwpConfinedPointerV1Unconfined(wlr.ZwpConfinedPointerV1UnconfinedEvent) {
	c.setActive(false)
}

// bindPointerConstraints binds the constraints manager.
func (s *Session) bindPointerConstraints(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpPointerConstraintsV1(ctx)
	if s.bindOptional(ev, 1, mgr) {
		s.constraintsMgr = mgr
		debug.Log("shell", "pointer-constraints-v1 bound")
	}
}

// bindRelativePointer binds the relative pointer manager.
func (s *Session) bindRelativePointer(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpRelativePointerManagerV1(ctx)
	if s.bindOptional(ev, 1, mgr) {
		s.relativeMgr = mgr
		debug.Log("shell", "relative-pointer-v1 bound")
		s.ensureRelativePointer()
	}
}

// rawPointer is the bound wl_pointer proxy, nil without one.
func (s *Session) rawPointer() *wl.Pointer {
	if raw, ok := s.pointer.(interface{ wlPointer() *wl.Pointer }); ok {
		return raw.wlPointer()
	}
	return nil
}

// ensureRelativePointer gets the pointer's relative object once both
// the manager and the pointer exist.
func (s *Session) ensureRelativePointer() {
	if s.relativeMgr == nil || s.relative != nil {
		return
	}
	p := s.rawPointer()
	if p == nil {
		return
	}
	if rp, err := s.relativeMgr.GetPointer(p); err == nil {
		s.relative = rp
		rp.AddMotionHandler(s)
	}
}

// dropRelativePointer destroys the relative object with its pointer.
func (s *Session) dropRelativePointer() {
	if s.relative != nil {
		_ = s.relative.Destroy()
		s.relative = nil
	}
}

// HandleZwpRelativePointerV1Motion routes relative motion to
// the pointer's surface.
func (s *Session) HandleZwpRelativePointerV1Motion(ev wlr.ZwpRelativePointerV1MotionEvent) {
	s.routeRelative(float64(ev.Dx), float64(ev.Dy), float64(ev.DxUnaccel), float64(ev.DyUnaccel))
}

// routeRelative delivers relative motion to the pointer target.
func (s *Session) routeRelative(dx, dy, rawDX, rawDY float64) {
	if h, ok := s.pointerTarget().(SurfaceRelativeHandler); ok {
		h.HandleRelativeMotion(dx, dy, rawDX, rawDY)
	}
}

// LockPointer locks the pointer in place while it is in surf; motion
// then arrives only as relative motion. persistent keeps the lock for
// every return of the pointer; otherwise it ends with its first
// deactivation.
func (s *Session) LockPointer(surf *wl.Surface, persistent bool) (*PointerConstraint, error) {
	return s.constrain(surf, nil, persistent, true)
}

// ConfinePointer keeps the pointer inside region (surface rects; nil
// is the whole surface) while it is in surf.
func (s *Session) ConfinePointer(surf *wl.Surface, region []image.Rectangle, persistent bool) (*PointerConstraint, error) {
	return s.constrain(surf, region, persistent, false)
}

// constrain creates a lock or a confinement.
func (s *Session) constrain(surf *wl.Surface, region []image.Rectangle, persistent, lock bool) (*PointerConstraint, error) {
	if surf != nil && s.constraints[surf] != nil {
		return nil, ErrAlreadyConstrained
	}
	p := s.rawPointer()
	if s.constraintsMgr == nil || p == nil || surf == nil {
		return nil, ErrPointerConstraintsUnavailable
	}
	var reg wl.Proxy = wlnull.Null
	if region != nil {
		r, err := s.Compositor().CreateRegion()
		if err != nil {
			return nil, err
		}
		for _, rc := range region {
			_ = r.Add(intconv.Int32(rc.Min.X), intconv.Int32(rc.Min.Y), intconv.Int32(rc.Dx()), intconv.Int32(rc.Dy()))
		}
		defer func() { _ = r.Destroy() }()
		reg = r
	}
	lifetime := uint32(wlr.ZwpPointerConstraintsV1LifetimeOneshot)
	if persistent {
		lifetime = wlr.ZwpPointerConstraintsV1LifetimePersistent
	}
	c := &PointerConstraint{s: s, surface: surf}
	ctx := s.constraintsMgr.Context()
	if lock {
		c.locked = wlr.NewZwpLockedPointerV1(ctx)
		c.locked.AddLockedHandler(c)
		c.locked.AddUnlockedHandler(c)
		if err := ctx.SendRequest(s.constraintsMgr, 1, c.locked, surf, p, reg, lifetime); err != nil {
			return nil, err
		}
	} else {
		c.confined = wlr.NewZwpConfinedPointerV1(ctx)
		c.confined.AddConfinedHandler(c)
		c.confined.AddUnconfinedHandler(c)
		if err := ctx.SendRequest(s.constraintsMgr, 2, c.confined, surf, p, reg, lifetime); err != nil {
			return nil, err
		}
	}
	if s.constraints == nil {
		s.constraints = map[*wl.Surface]*PointerConstraint{}
	}
	s.constraints[surf] = c
	return c, nil
}

// releaseConstraint drops surf's constraint when its input goes away:
// the constraint's life is the surface's.
func (s *Session) releaseConstraint(surf *wl.Surface) {
	if c := s.constraints[surf]; c != nil {
		c.Release()
	}
}

// PointerConstraintsAvailable reports whether the compositor offers
// pointer constraints.
func (s *Session) PointerConstraintsAvailable() bool { return s.constraintsMgr != nil }

// RelativePointerAvailable reports whether the compositor offers
// relative pointer motion.
func (s *Session) RelativePointerAvailable() bool { return s.relativeMgr != nil }
