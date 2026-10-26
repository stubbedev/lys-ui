// Package layersurface maps the wlr-layer-shell role onto a wl_surface:
// anchor, layer, keyboard mode, the configure handshake, and the closed
// signal.
package layersurface

import (
	"errors"
	"fmt"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/wlnull"
)

// ErrNotConfigured reports a draw attempt before the first configure
// event. The compositor rejects buffers committed before the initial
// configure.
var ErrNotConfigured = errors.New("layersurface: not configured yet")

// ErrClosed reports that the compositor closed the layer surface; the
// surface object must not be used again.
var ErrClosed = errors.New("layersurface: closed by compositor")

// Anchor is a bitmask of the edges the surface is anchored to.
type Anchor uint8

// Anchor edges. The numeric values are the wire values of the
// protocol enum.
const (
	AnchorTop Anchor = 1 << iota
	AnchorBottom
	AnchorLeft
	AnchorRight
)

// Layer selects the stack layer the surface renders in. The numeric values
// are the wire values of the protocol enum.
type Layer uint8

// Stack layers, background through overlay.
const (
	LayerBackground Layer = iota
	LayerBottom
	LayerTop
	LayerOverlay
)

// KeyboardMode selects how the compositor gives the surface keyboard
// focus. The numeric values are the wire values of the protocol enum.
type KeyboardMode uint8

// Keyboard interactivity modes: none, exclusive grab, on-demand.
const (
	KeyboardNone KeyboardMode = iota
	KeyboardExclusive
	KeyboardOnDemand
)

// Config describes the layer surface before the first commit.
type Config struct {
	Layer  Layer
	Anchor Anchor

	// Width and Height are the surface size in surface (logical) pixels.
	// An axis set to 0 lets the compositor assign it, which the protocol
	// only allows when both edges of that axis are anchored; New
	// enforces this locally so the error surfaces before the wire.
	Width, Height uint32

	// Margin distances the surface from the edges it is anchored to,
	// in surface (logical) pixels.
	Margin Margins

	// ExclusiveZone reserves space along the anchored edge, in surface
	// pixels. Negative values distance the surface from the edge by the
	// given amount. 0 requests no reservation.
	ExclusiveZone int32

	Keyboard  KeyboardMode
	Namespace string

	// shellVersion is the bound layer shell's version (zero reads as
	// 1), taken from the Shell at New: it gates what the surface may
	// ask for - set_layer from v2, on-demand keyboard from v4.
	shellVersion uint32
}

// Shell is the bound layer shell and the version it was bound at; the
// session is one. Taking both together keeps a caller from asking for
// what the bound version cannot carry.
type Shell interface {
	LayerShell() *wlr.ZwlrLayerShellV1
	LayerShellVersion() uint32
}

// Protocol versions requests and values need.
const (
	versionSetLayer       = 2
	versionKeyboardDemand = 4
)

// keyboardSupported reports whether a shell at version can carry mode.
func keyboardSupported(mode KeyboardMode, version uint32) error {
	if mode == KeyboardOnDemand && max(version, 1) < versionKeyboardDemand {
		return fmt.Errorf("%w: on-demand keyboard needs v%d", ErrShellTooOld, versionKeyboardDemand)
	}
	return nil
}

// ErrShellTooOld reports a request the compositor's layer shell
// version cannot carry.
var ErrShellTooOld = errors.New("layersurface: the compositor's layer shell is too old for this request")

// Margins is the space between the surface and the anchored edges.
type Margins struct {
	Top, Right, Bottom, Left int32
}

// Set applies the margins to the layer surface.
func (m Margins) Set(ls *wlr.ZwlrLayerSurfaceV1) {
	_ = ls.SetMargin(m.Top, m.Right, m.Bottom, m.Left)
}

// Surface is one layer surface and its configure handshake.
type Surface struct {
	WLSurface *wl.Surface
	Layer     *wlr.ZwlrLayerSurfaceV1

	cfg        Config
	configured bool
	closed     bool
	// destroyed is the client-side destroy (Close); closed also covers
	// the compositor closing the surface.
	destroyed bool
	width     uint32
	height    uint32
}

// New assigns the layer role and sends the initial state. The caller must
// still commit the wl_surface; the compositor answers with the first
// configure event.
func New(shell Shell, surf *wl.Surface, output *wl.Output, cfg Config) (*Surface, error) {
	if shell.LayerShell() == nil {
		return nil, errors.New("layersurface: compositor has no zwlr_layer_shell_v1")
	}
	cfg.shellVersion = shell.LayerShellVersion()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	ls, err := getLayerSurface(shell.LayerShell(), surf, output, uint32(cfg.Layer), cfg.Namespace)
	if err != nil {
		return nil, fmt.Errorf("layersurface: get_layer_surface: %w", err)
	}
	s := &Surface{WLSurface: surf, Layer: ls, cfg: cfg}
	ls.AddConfigureHandler(s)
	ls.AddClosedHandler(s)

	if err := ls.SetSize(cfg.Width, cfg.Height); err != nil {
		return nil, fmt.Errorf("layersurface: set_size: %w", err)
	}
	if err := ls.SetAnchor(uint32(cfg.Anchor)); err != nil {
		return nil, fmt.Errorf("layersurface: set_anchor: %w", err)
	}
	if err := ls.SetExclusiveZone(cfg.ExclusiveZone); err != nil {
		return nil, fmt.Errorf("layersurface: set_exclusive_zone: %w", err)
	}
	cfg.Margin.Set(ls)
	if err := ls.SetKeyboardInteractivity(uint32(cfg.Keyboard)); err != nil {
		return nil, fmt.Errorf("layersurface: set_keyboard_interactivity: %w", err)
	}
	return s, nil
}

// validate mirrors the protocol's invalid_size rule: an automatic (zero)
// axis is only allowed when both edges of that axis are anchored.
func (c Config) validate() error {
	if err := keyboardSupported(c.Keyboard, c.shellVersion); err != nil {
		return err
	}
	if c.Width == 0 && c.Anchor&(AnchorLeft|AnchorRight) != AnchorLeft|AnchorRight {
		return fmt.Errorf("layersurface: auto width needs both left and right anchors (anchor=%d)", c.Anchor)
	}
	if c.Height == 0 && c.Anchor&(AnchorTop|AnchorBottom) != AnchorTop|AnchorBottom {
		return fmt.Errorf("layersurface: auto height needs both top and bottom anchors (anchor=%d)", c.Anchor)
	}
	return nil
}

// SetSize requests a new size after creation. The same invalid_size
// rule as the initial size applies and is enforced locally; the size
// is double-buffered on the wire, so it lands with the next commit.
func (s *Surface) SetSize(width, height uint32) error {
	return s.change(func(c *Config) { c.Width, c.Height = width, height }, "set_size",
		func() error { return s.Layer.SetSize(width, height) })
}

// SetAnchor re-anchors the surface after creation (gtk4-layer-shell
// allows every declarative property to change): a bar moving from the
// top edge to the bottom. The auto-axis rule is checked against the
// current size; the change lands with the next commit.
func (s *Surface) SetAnchor(a Anchor) error {
	return s.change(func(c *Config) { c.Anchor = a }, "set_anchor",
		func() error { return s.Layer.SetAnchor(uint32(a)) })
}

// Reshape changes the anchor and the size together, validated as one
// state: a zero axis may become concrete in the same change that drops
// one of its anchors, which SetAnchor and SetSize alone would each
// refuse. Both requests land with the next commit.
func (s *Surface) Reshape(a Anchor, width, height uint32) error {
	return s.change(func(c *Config) { c.Anchor, c.Width, c.Height = a, width, height }, "reshape",
		func() error {
			if err := s.Layer.SetSize(width, height); err != nil {
				return err
			}
			return s.Layer.SetAnchor(uint32(a))
		})
}

// SetMargin changes the distances from the anchored edges.
func (s *Surface) SetMargin(m Margins) error {
	return s.change(func(c *Config) { c.Margin = m }, "set_margin",
		func() error { return s.Layer.SetMargin(m.Top, m.Right, m.Bottom, m.Left) })
}

// SetExclusiveZone changes the reserved space along the anchored edge
// (a bar auto-hiding gives its zone back).
func (s *Surface) SetExclusiveZone(zone int32) error {
	return s.change(func(c *Config) { c.ExclusiveZone = zone }, "set_exclusive_zone",
		func() error { return s.Layer.SetExclusiveZone(zone) })
}

// SetLayer moves the surface to another stack layer (layer shell v2).
func (s *Surface) SetLayer(l Layer) error {
	if max(s.cfg.shellVersion, 1) < versionSetLayer {
		return fmt.Errorf("%w: set_layer needs v%d", ErrShellTooOld, versionSetLayer)
	}
	return s.change(func(c *Config) { c.Layer = l }, "set_layer",
		func() error { return s.Layer.SetLayer(uint32(l)) })
}

// change is every post-creation property change: validate the next
// config, refuse on a closed surface, send, record. The properties are
// double-buffered, so they land with the next commit.
func (s *Surface) change(edit func(*Config), request string, send func() error) error {
	next := s.cfg
	edit(&next)
	if err := next.validate(); err != nil {
		return err
	}
	if s.closed {
		return ErrClosed
	}
	if err := send(); err != nil {
		return fmt.Errorf("layersurface: %s: %w", request, err)
	}
	s.cfg = next
	return nil
}

// Config is the surface's current declarative state: creation's,
// updated by every accepted runtime change - what a rebuild on a new
// connection reads back.
func (s *Surface) Config() Config { return s.cfg }

// KeyboardMode is the keyboard interactivity in effect.
func (s *Surface) KeyboardMode() KeyboardMode { return s.cfg.Keyboard }

// SetKeyboardMode changes the keyboard interactivity after creation (a
// bar that takes keys while one of its popovers is open). The state is
// double-buffered: it is committed here so it applies before the next
// request that depends on it, a popup grab.
func (s *Surface) SetKeyboardMode(mode KeyboardMode) error {
	if s.closed {
		return ErrClosed
	}
	if mode == s.cfg.Keyboard {
		return nil
	}
	if err := keyboardSupported(mode, s.cfg.shellVersion); err != nil {
		return err
	}
	if err := s.Layer.SetKeyboardInteractivity(uint32(mode)); err != nil {
		return fmt.Errorf("layersurface: set_keyboard_interactivity: %w", err)
	}
	s.cfg.Keyboard = mode
	return s.WLSurface.Commit()
}

// HandleZwlrLayerSurfaceV1Configure implements the configure handler: it
// records the new size and acknowledges the serial. A zero width or height
// keeps the current choice for that axis, per the protocol.
func (s *Surface) HandleZwlrLayerSurfaceV1Configure(ev wlr.ZwlrLayerSurfaceV1ConfigureEvent) {
	s.configure(ev.Serial, ev.Width, ev.Height, s.Layer.AckConfigure)
}

// configure applies one configure and acks it through ack. A surface
// the client destroyed ignores the configures still in flight: the
// proxy stays registered until the compositor's delete_id, and acking
// the destroyed object is a fatal protocol error ("invalid object").
func (s *Surface) configure(serial, width, height uint32, ack func(uint32) error) {
	if s.destroyed {
		return
	}
	s.applyConfigure(width, height)
	_ = ack(serial)
}

// applyConfigure records the configure without touching the wire; tests
// exercise the state machine through it.
func (s *Surface) applyConfigure(width, height uint32) {
	s.configured = true
	if width != 0 {
		s.width = width
	}
	if height != 0 {
		s.height = height
	}
}

// HandleZwlrLayerSurfaceV1Closed marks the surface closed.
func (s *Surface) HandleZwlrLayerSurfaceV1Closed(wlr.ZwlrLayerSurfaceV1ClosedEvent) {
	s.closed = true
}

// EnsureUsable gates drawing: the surface must have completed the first
// configure and must not be closed. A closed surface wins over the
// configured state.
func (s *Surface) EnsureUsable() error {
	if s.closed {
		return ErrClosed
	}
	if !s.configured {
		return ErrNotConfigured
	}
	return nil
}

// Closed reports whether the compositor closed the surface.
func (s *Surface) Closed() bool {
	return s.closed
}

// Close destroys the layer surface from the client side. The closed
// flag turns on immediately so the owning loop drops the window.
func (s *Surface) Close() {
	_ = s.Layer.Destroy()
	s.destroyed = true
	s.closed = true
}

// Size returns the last configured size in surface (logical) pixels. An
// unconfigured surface reports zeros.
func (s *Surface) Size() (int, int) {
	return int(s.width), int(s.height)
}

// HostSurface returns the underlying wl_surface.
func (s *Surface) HostSurface() *wl.Surface { return s.WLSurface }

// LayerPopupSurface returns the layer surface for popup parenting;
// layer surfaces have no xdg_surface of their own.
func (s *Surface) LayerPopupSurface() *wlr.ZwlrLayerSurfaceV1 { return s.Layer }

// getLayerSurface is zwlr_layer_shell.get_layer_surface; a nil output
// lets the compositor choose one (usually the focused output), sent as
// wlnull.Null because the binding cannot send a typed-nil output.
func getLayerSurface(shell *wlr.ZwlrLayerShellV1, surf *wl.Surface, output *wl.Output, layer uint32, namespace string) (*wlr.ZwlrLayerSurfaceV1, error) {
	if output != nil {
		return shell.GetLayerSurface(surf, output, layer, namespace)
	}
	ls := wlr.NewZwlrLayerSurfaceV1(shell.Context())
	return ls, shell.Context().SendRequest(shell, 0, ls, surf, wlnull.Null, layer, namespace)
}
