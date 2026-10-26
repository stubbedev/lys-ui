package popup

import (
	"testing"

	"github.com/neurlang/wayland/xdg"
	"github.com/stubbedev/lys-lib/render"
)

// A point anchors as a 1x1 rect the popup grows down-right of, free to
// slide on both axes and flip up (menus at the pointer, tooltips).
func TestPlacementForAPoint(t *testing.T) {
	p := placementFor(Config{X: 40, Y: 60})
	want := placement{
		rect:   render.Rect{X: 40, Y: 60, W: 1, H: 1},
		anchor: xdg.PositionerAnchorTopLeft, gravity: xdg.PositionerGravityBottomRight,
		adjust: xdg.PositionerConstraintAdjustmentSlideX | xdg.PositionerConstraintAdjustmentSlideY | xdg.PositionerConstraintAdjustmentFlipY,
	}
	if p != want {
		t.Errorf("point = %+v, want %+v", p, want)
	}
}

// A rect anchors edge-aligned on the gravity side and may flip across
// the anchor on that axis (a top bar's dropdown with no room below
// opens above, never over the bar) and slide along the other; the
// gutter offset keeps the content edge on the anchor edge.
func TestPlacementForARect(t *testing.T) {
	rect := render.Rect{X: 400, Y: 6, W: 180, H: 42}
	const g = 10
	flipY := uint32(xdg.PositionerConstraintAdjustmentSlideX | xdg.PositionerConstraintAdjustmentFlipY | xdg.PositionerConstraintAdjustmentResizeY)
	flipX := uint32(xdg.PositionerConstraintAdjustmentSlideY | xdg.PositionerConstraintAdjustmentFlipX | xdg.PositionerConstraintAdjustmentResizeX)
	for _, tc := range []struct {
		gravity         Gravity
		anchor, grow    uint32
		adjust          uint32
		offsetX, offset int32
	}{
		// Centered on the anchor across the opening axis, as GtkPopover
		// places itself; the gutter offset only on the opening axis.
		{GravityBottom, xdg.PositionerAnchorBottom, xdg.PositionerGravityBottom, flipY, 0, -g},
		{GravityTop, xdg.PositionerAnchorTop, xdg.PositionerGravityTop, flipY, 0, g},
		{GravityRight, xdg.PositionerAnchorRight, xdg.PositionerGravityRight, flipX, -g, 0},
		{GravityLeft, xdg.PositionerAnchorLeft, xdg.PositionerGravityLeft, flipX, g, 0},
	} {
		p := placementFor(Config{AnchorRect: rect, Gravity: tc.gravity, Gutter: g, X: 999, Y: 999})
		want := placement{rect: rect, anchor: tc.anchor, gravity: tc.grow, adjust: tc.adjust, offsetX: tc.offsetX, offsetY: tc.offset}
		if p != want {
			t.Errorf("gravity %d = %+v, want %+v", tc.gravity, p, want)
		}
	}
	// A gap moves the content off the anchor on the opening axis.
	if p := placementFor(Config{AnchorRect: rect, Gutter: g, Gap: 4}); p.offsetY != -g+4 {
		t.Errorf("gap offset = %d, want %d", p.offsetY, -g+4)
	}
	// No gutter, no offset.
	if p := placementFor(Config{AnchorRect: rect}); p.offsetX != 0 || p.offsetY != 0 {
		t.Errorf("gutterless offset = %d,%d", p.offsetX, p.offsetY)
	}
}

// A vertical popup the compositor shrank tries the anchor's other side
// once and goes back when that side gives less; one placed whole, a
// sideways or point popup, or a finished check never moves (#325).
func TestSideStep(t *testing.T) {
	below := Config{AnchorRect: render.Rect{X: 10, Y: 800, W: 100, H: 30}, Width: 200, Height: 1600, Gravity: GravityBottom}
	for _, tc := range []struct {
		name     string
		side     sideCheck
		cfg      Config
		h, sideH int32
		next     sideCheck
		flip     bool
	}{
		{"shrunk below tries above", sideUnchecked, below, 190, 0, sideTrying, true},
		{"above gives more and stays", sideTrying, below, 830, 190, sideChecked, false},
		{"above gives less and goes back", sideTrying, below, 120, 190, sideChecked, true},
		{"placed whole stays", sideUnchecked, below, 1600, 0, sideChecked, false},
		{"sideways stays", sideUnchecked, Config{AnchorRect: below.AnchorRect, Height: 1600, Gravity: GravityRight}, 190, 0, sideChecked, false},
		{"a point popup stays", sideUnchecked, Config{Height: 1600}, 190, 0, sideChecked, false},
		{"checked stays", sideChecked, below, 190, 0, sideChecked, false},
	} {
		if next, flip := sideStep(tc.side, tc.cfg, tc.h, tc.sideH); next != tc.next || flip != tc.flip {
			t.Errorf("%s: step (%v, %v), want (%v, %v)", tc.name, next, flip, tc.next, tc.flip)
		}
	}
}
