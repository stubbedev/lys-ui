package popup

import (
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// placement is one xdg_positioner setup: the anchor rect, which point
// of it the popup hangs from, the direction it grows, how the
// compositor may adjust it against the output, and the offset that
// keeps the shadow gutter outside the content's alignment.
type placement struct {
	rect             render.Rect
	anchor, gravity  uint32
	adjust           uint32
	offsetX, offsetY int32
}

// placementFor maps a Config onto the positioner. A point (menus) is
// a 1x1 rect the popup grows down and right of, sliding on both axes
// and flipping up. A rect (popovers, tooltips) opens on the Gravity
// side centered on the anchor along the other axis, as GtkPopover
// places itself (a bottom popover's rect anchor south, surface anchor
// north), and may flip across the anchor on that axis and slide along
// the other, so a bar's dropdown that has no room below the screen
// edge opens above the bar instead of over it; with room on neither
// side it shrinks on that axis (GtkPopover's resize), its content
// giving way, rather than leave the output. The shadow gutter rides
// on every side, so only the opening axis needs an offset to put the
// content, not the gutter, the Gap away from the anchor.
func placementFor(cfg Config) placement {
	const (
		slideX  = xdg.PositionerConstraintAdjustmentSlideX
		slideY  = xdg.PositionerConstraintAdjustmentSlideY
		flipX   = xdg.PositionerConstraintAdjustmentFlipX
		flipY   = xdg.PositionerConstraintAdjustmentFlipY
		resizeX = xdg.PositionerConstraintAdjustmentResizeX
		resizeY = xdg.PositionerConstraintAdjustmentResizeY
	)
	if cfg.AnchorRect.Empty() {
		return placement{
			rect:    render.Rect{X: cfg.X, Y: cfg.Y, W: 1, H: 1},
			anchor:  xdg.PositionerAnchorTopLeft,
			gravity: xdg.PositionerGravityBottomRight,
			adjust:  slideX | slideY | flipY,
		}
	}
	g := intconv.Int32(cfg.Gutter - cfg.Gap)
	p := placement{rect: cfg.AnchorRect}
	switch cfg.Gravity {
	case GravityTop:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorTop, xdg.PositionerGravityTop, slideX|flipY|resizeY
		p.offsetY = g
	case GravityRight:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorRight, xdg.PositionerGravityRight, slideY|flipX|resizeX
		p.offsetX = -g
	case GravityLeft:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorLeft, xdg.PositionerGravityLeft, slideY|flipX|resizeX
		p.offsetX = g
	default:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorBottom, xdg.PositionerGravityBottom, slideX|flipY|resizeY
		p.offsetY = -g
	}
	return p
}

// place applies the placement to a positioner.
func place(pos *xdg.Positioner, cfg Config) error {
	p := placementFor(cfg)
	r := p.rect
	if err := pos.SetAnchorRect(intconv.Int32(r.X), intconv.Int32(r.Y), intconv.Int32(r.W), intconv.Int32(r.H)); err != nil {
		return err
	}
	if err := pos.SetAnchor(p.anchor); err != nil {
		return err
	}
	if err := pos.SetGravity(p.gravity); err != nil {
		return err
	}
	if err := pos.SetConstraintAdjustment(p.adjust); err != nil {
		return err
	}
	if p.offsetX != 0 || p.offsetY != 0 {
		return pos.SetOffset(p.offsetX, p.offsetY)
	}
	return nil
}
