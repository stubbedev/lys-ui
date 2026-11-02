package app

import (
	"errors"
	"fmt"
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/layersurface"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// clickAwayNamespace tags the catcher surfaces for compositor rules.
const clickAwayNamespace = "lys-ui-click-away"

// clickAway is what closes a PopoverClickAway popover without a seat
// grab, which would take the keyboard: an invisible layer surface on
// every output that hears the press landing outside the popover. Popups
// stack above layer surfaces, and the surfaces take no keyboard, so the
// focused window keeps its keys. The press goes no further, as under a
// grab. The surface that heard it lives until the button comes back up,
// so the window beneath never sees a release without its press.
type clickAway struct {
	wire     catcherWire
	onPress  func()
	catchers map[*wlsession.Output]catcher
	held     catcher
	closing  bool
}

// catcher is one output's surface.
type catcher interface{ destroy() }

// catcherWire builds a catcher on an output; press and release are the
// surface's input.
type catcherWire func(out *wlsession.Output, press, release func()) (catcher, error)

// newClickAway arms a catcher on every output.
func newClickAway(wire catcherWire, outputs []*wlsession.Output, onPress func()) (*clickAway, error) {
	c := &clickAway{wire: wire, onPress: onPress, catchers: make(map[*wlsession.Output]catcher, len(outputs))}
	if err := c.sync(outputs); err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

// sync follows outputs coming and going while the popover is open.
func (c *clickAway) sync(outputs []*wlsession.Output) error {
	if c.closing {
		return nil
	}
	var errs []error
	for _, out := range outputs {
		if _, ok := c.catchers[out]; !ok {
			errs = append(errs, c.arm(out))
		}
	}
	for out, ct := range c.catchers {
		if !slices.Contains(outputs, out) {
			delete(c.catchers, out)
			if c.held == ct {
				c.held = nil
			}
			ct.destroy()
		}
	}
	return errors.Join(errs...)
}

func (c *clickAway) arm(out *wlsession.Output) error {
	var self catcher
	ct, err := c.wire(out, func() { c.press(self) }, func() { c.release(self) })
	if err != nil {
		return fmt.Errorf("app: click-away on output %q: %w", out.Name, err)
	}
	self = ct
	c.catchers[out] = ct
	return nil
}

func (c *clickAway) press(ct catcher) {
	if c.closing {
		return
	}
	c.held = ct
	c.onPress()
}

func (c *clickAway) release(ct catcher) {
	if c.held != ct {
		return
	}
	c.held = nil
	if c.closing {
		ct.destroy()
	}
}

// close drops every catcher, the one under a held press once it ends.
// A second close does nothing.
func (c *clickAway) close() {
	if c.closing {
		return
	}
	c.closing = true
	for _, ct := range c.catchers {
		if ct != c.held {
			ct.destroy()
		}
	}
	c.catchers = nil
}

// wireCatcher is the session's catcherWire.
func wireCatcher(sess *wlsession.Session) catcherWire {
	return func(out *wlsession.Output, press, release func()) (catcher, error) {
		return newLayerCatcher(sess, out, press, release)
	}
}

// layerCatcher is a transparent one-pixel buffer stretched across the
// output's usable area: one wl_shm pixel instead of a frame per output.
// It keeps off exclusive zones, so bars stay clickable.
type layerCatcher struct {
	sess     *wlsession.Session
	surf     *wl.Surface
	ls       *layersurface.Surface
	viewport *wlr.WpViewport
	buf      *buffer.Buffer
	attached bool
	gone     bool
	press    func()
	release  func()
}

func newLayerCatcher(sess *wlsession.Session, out *wlsession.Output, press, release func()) (c *layerCatcher, err error) {
	vp := sess.Viewporter()
	if vp == nil {
		return nil, errors.New("the compositor has no wp_viewporter")
	}
	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("create surface: %w", err)
	}
	c = &layerCatcher{sess: sess, surf: surf, press: press, release: release}
	defer func() {
		if err != nil {
			c.destroy()
		}
	}()
	if c.viewport, err = vp.GetViewport(surf); err != nil {
		return c, fmt.Errorf("get viewport: %w", err)
	}
	if c.buf, err = buffer.NewFile(sess.Shm(), 1, 1, 1); err != nil {
		return c, fmt.Errorf("allocate pixel: %w", err)
	}
	clear(c.buf.Data)
	c.ls, err = layersurface.New(sess, surf, out.WL, layersurface.Config{
		Layer:     layersurface.LayerOverlay,
		Anchor:    layersurface.AnchorTop | layersurface.AnchorBottom | layersurface.AnchorLeft | layersurface.AnchorRight,
		Keyboard:  layersurface.KeyboardNone,
		Namespace: clickAwayNamespace,
	})
	if err != nil {
		return c, fmt.Errorf("layer surface: %w", err)
	}
	c.ls.OnConfigured(c.configured)
	sess.SetSurfaceInput(surf, c)
	if err = surf.Commit(); err != nil {
		return c, fmt.Errorf("commit: %w", err)
	}
	return c, nil
}

// configured gives the pixel the configured size.
func (c *layerCatcher) configured() {
	if c.gone {
		return
	}
	w, h := c.ls.Size()
	if w <= 0 || h <= 0 {
		return
	}
	err := c.viewport.SetDestination(intconv.Clamp[int32](w), intconv.Clamp[int32](h))
	if err == nil && !c.attached {
		err = c.surf.Attach(c.buf.WL, 0, 0)
		c.attached = err == nil
	}
	if err == nil {
		err = c.surf.Commit()
	}
	if err != nil {
		debug.Log("input", "click-away catcher configure: %v", err)
	}
}

func (c *layerCatcher) destroy() {
	if c.gone {
		return
	}
	c.gone = true
	c.sess.SetSurfaceInput(c.surf, nil)
	if !c.attached {
		buffer.Cancel(c.buf)
	}
	var errs []error
	if c.viewport != nil {
		errs = append(errs, c.viewport.Destroy())
	}
	if c.ls != nil {
		c.ls.Close()
	}
	errs = append(errs, c.surf.Destroy())
	if err := errors.Join(errs...); err != nil {
		debug.Log("input", "click-away catcher destroy: %v", err)
	}
}

// HandlePointerEnter implements wlsession.SurfacePointerHandler.
func (c *layerCatcher) HandlePointerEnter(float64, float64) {
	if err := c.sess.SetCursor("default"); err != nil {
		debug.Log("input", "click-away catcher cursor: %v", err)
	}
}

// HandlePointerMotion implements wlsession.SurfacePointerHandler.
func (*layerCatcher) HandlePointerMotion(float64, float64) {}

// HandlePointerButton implements wlsession.SurfacePointerHandler.
func (c *layerCatcher) HandlePointerButton(_, state, _ uint32) {
	if state == 1 {
		c.press()
		return
	}
	c.release()
}

// HandlePointerAxis implements wlsession.SurfacePointerHandler.
func (*layerCatcher) HandlePointerAxis(float64, float64) {}

// HandlePointerLeave implements wlsession.SurfacePointerHandler.
func (c *layerCatcher) HandlePointerLeave() { c.release() }

// HandleTouchDown implements wlsession.SurfaceTouchHandler.
func (c *layerCatcher) HandleTouchDown(int32, float64, float64, uint32) { c.press() }

// HandleTouchMotion implements wlsession.SurfaceTouchHandler.
func (*layerCatcher) HandleTouchMotion(int32, float64, float64) {}

// HandleTouchUp implements wlsession.SurfaceTouchHandler.
func (c *layerCatcher) HandleTouchUp(int32) { c.release() }

// HandleTouchCancel implements wlsession.SurfaceTouchHandler.
func (c *layerCatcher) HandleTouchCancel() { c.release() }
