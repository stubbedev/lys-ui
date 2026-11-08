package app

import (
	"errors"
	"fmt"

	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/logutil"
	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/widget"
)

// BarConfig shows a widget.Bar as a layer surface along a screen edge.
type BarConfig struct {
	// Output pins the bar to one output; nil lets the compositor
	// choose.
	Output *Output
	// Bar is the bar shown; its edge picks the anchors.
	Bar *widget.Bar
	// Layer is the stack layer, LayerTop for a usual bar.
	Layer Layer
	// Margin is the gap between the bar and its screen edge, and the
	// screen ends it runs to.
	Margin int32
	// Exclusive reserves the bar's thickness, and the margin, so
	// maximized windows stop at it.
	Exclusive bool
	// Namespace tags the surface for compositor rules.
	Namespace string
	// OnPresented mirrors LayerConfig.OnPresented.
	OnPresented func()
}

// BarWindow is a bar's layer surface. Its thickness is the bar's
// natural size across its edge, and its exclusive zone follows that
// thickness as the bar's content changes.
type BarWindow struct {
	*LayerWindow
	cfg BarConfig
	// walkDone ends the keyboard walk WalkKeyboard began, nil while
	// none runs.
	walkDone func()
}

// NewBar shows cfg.Bar along its edge: anchored to that edge and
// stretched along it, as thick as the bar's content.
func (a *Application) NewBar(cfg BarConfig) (*BarWindow, error) {
	if cfg.Bar == nil {
		return nil, errors.New("app: NewBar without a bar")
	}
	w := &BarWindow{cfg: cfg}
	edge := cfg.Bar.Edge()
	lw, err := a.NewLayer(LayerConfig{
		Output: cfg.Output, Layer: cfg.Layer, Anchor: barAnchor(edge), Margin: barMargins(edge, cfg.Margin),
		Namespace: cfg.Namespace, Root: cfg.Bar,
		OnResize:    func(width, height int) { w.reserve(width, height) },
		KeyCapture:  w.walkKey,
		OnPresented: cfg.OnPresented,
	})
	if err != nil {
		return nil, err
	}
	w.LayerWindow = lw
	return w, nil
}

// SetEdge moves the bar to edge: re-anchored, its margins and zone
// following, and its content turned to the new orientation.
func (w *BarWindow) SetEdge(edge widget.BarEdge) error {
	w.cfg.Bar.SetEdge(edge)
	if err := w.SetMargin(barMargins(edge, w.cfg.Margin)); err != nil {
		return err
	}
	return w.SetAnchor(barAnchor(edge))
}

// SetExclusive starts or stops reserving the bar's edge: on, the
// exclusive zone takes the bar's current thickness and follows it as
// the bar resizes; off, it drops to 0 and maximized windows reach the
// edge again. The change applies with the next commit.
func (w *BarWindow) SetExclusive(on bool) error {
	if on == w.cfg.Exclusive {
		return nil
	}
	w.cfg.Exclusive = on
	if !on {
		return w.SetExclusiveZone(0)
	}
	return w.SetExclusiveZone(w.thickness(w.Size()))
}

// reserve sets the exclusive zone to the thickness across the edge.
func (w *BarWindow) reserve(width, height int) {
	if !w.cfg.Exclusive || w.LayerWindow == nil {
		return
	}
	thick := w.thickness(width, height)
	if err := w.SetExclusiveZone(thick); err != nil {
		logutil.L().Warn("app: bar: reserve its exclusive zone", "thickness", thick, "err", err)
	}
}

// thickness is the bar's extent across its edge at width x height.
func (w *BarWindow) thickness(width, height int) int32 {
	if w.cfg.Bar.Edge().Vertical() {
		return intconv.Clamp[int32](width)
	}
	return intconv.Clamp[int32](height)
}

// barAnchor anchors a bar to its edge and both ends of it.
func barAnchor(edge widget.BarEdge) Anchor {
	switch edge {
	case widget.BarBottom:
		return AnchorBottom | AnchorLeft | AnchorRight
	case widget.BarLeft:
		return AnchorLeft | AnchorTop | AnchorBottom
	case widget.BarRight:
		return AnchorRight | AnchorTop | AnchorBottom
	}
	return AnchorTop | AnchorLeft | AnchorRight
}

// barMargins keeps m from the bar's edge and from both ends of it.
func barMargins(edge widget.BarEdge, m int32) Margins {
	out := Margins{Top: m, Right: m, Bottom: m, Left: m}
	switch edge {
	case widget.BarTop:
		out.Bottom = 0
	case widget.BarBottom:
		out.Top = 0
	case widget.BarLeft:
		out.Right = 0
	case widget.BarRight:
		out.Left = 0
	}
	return out
}

// WalkKeyboard hands the bar the keyboard so its controls are reached
// without a pointer: the bar takes the keyboard exclusively (a
// compositor need not focus a surface turning on demand), and its
// first control takes the focus with its ring. The arrows along the
// edge (Left and Right, Up and Down on a vertical bar) and Tab move
// between the controls, and Enter and Space press the focused one.
// Esc ends the walk: the focus clears, the bar gives the keyboard
// back, and done runs. Walking a bar that walks restarts at its first
// control once the earlier walk's done ran.
func (w *BarWindow) WalkKeyboard(done func()) error {
	hw := w.app.hostWindowOf(w.LayerWindow)
	if hw == nil {
		return fmt.Errorf("app: walk the bar's keyboard: %w", ErrClosed)
	}
	if err := w.holdKeyboard(KeyboardExclusive); err != nil {
		return fmt.Errorf("app: walk the bar's keyboard: %w", err)
	}
	if w.walkDone != nil {
		w.walkDone()
	}
	w.walkDone = done
	_ = hw.router.SetFocus(nil)
	hw.router.FocusNext()
	w.requestFrame()
	return nil
}

// Walking reports whether a WalkKeyboard walk runs.
func (w *BarWindow) Walking() bool { return w.walkDone != nil }

// walkKey is the bar's key capture: during a walk the arrows along the
// edge move the focus and Esc ends it; every other press, and every
// press outside a walk, routes as usual.
func (w *BarWindow) walkKey(a Accel) bool {
	if w.walkDone == nil || a.Mods != 0 {
		return false
	}
	hw := w.app.hostWindowOf(w.LayerWindow)
	if hw == nil {
		return false
	}
	prev, next := xkb.KeyLeft, xkb.KeyRight
	if w.cfg.Bar.Edge().Vertical() {
		prev, next = xkb.KeyUp, xkb.KeyDown
	}
	switch a.Sym {
	case prev:
		hw.router.FocusPrev()
	case next:
		hw.router.FocusNext()
	case xkb.KeyEscape:
		w.endWalk(hw)
	default:
		return false
	}
	hw.dirty = true
	return true
}

// endWalk clears the walk's focus, returns the keyboard to the bar's
// declared mode, and runs the walk's done.
func (w *BarWindow) endWalk(hw *hostWindow) {
	done := w.walkDone
	w.walkDone = nil
	_ = hw.router.SetFocus(nil)
	if err := w.holdKeyboard(w.LayerWindow.cfg.Keyboard); err != nil {
		logutil.L().Warn("app: bar: give the keyboard back", "err", err)
	}
	w.requestFrame()
	done()
}
