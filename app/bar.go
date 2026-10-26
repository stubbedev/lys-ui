package app

import (
	"errors"

	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/logutil"
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
}

// BarWindow is a bar's layer surface. Its thickness is the bar's
// natural size across its edge, and its exclusive zone follows that
// thickness as the bar's content changes.
type BarWindow struct {
	*LayerWindow
	cfg BarConfig
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
		OnResize: func(width, height int) { w.reserve(width, height) },
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

// reserve sets the exclusive zone to the thickness across the edge.
func (w *BarWindow) reserve(width, height int) {
	if !w.cfg.Exclusive || w.LayerWindow == nil {
		return
	}
	thick := height
	if w.cfg.Bar.Edge().Vertical() {
		thick = width
	}
	if err := w.SetExclusiveZone(intconv.Int32(thick)); err != nil {
		logutil.L().Warn("app: bar: reserve its exclusive zone", "thickness", thick, "err", err)
	}
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
