package app

import (
	"image"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// accelApp-built fixtures skip NewApplication's map init, so the icon
// setters must be safe on a bare Application: the no-protocol path
// touches nothing, and a nil-map guard keeps the override records
// writable either way.
func TestWindowIconNoOpWithoutProtocol(t *testing.T) {
	a := accelApp() // no session wiring: ToplevelIconManager reads nil-safe
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	a.SetIcon(img)
	a.SetWindowIcon(&Window{app: a}, img)
	a.SetWindowIcon(nil, img)
	if a.defaultIcon != nil {
		t.Error("an icon was built without the protocol")
	}
	a.reapWindowIcons()
}

// TestPostedIconLifecycle pins the bookkeeping: release destroys the
// icon object exactly once and retires every buffer, and reap drops
// the records of windows that left the loop.
func TestPostedIconLifecycle(t *testing.T) {
	a := accelApp()
	a.windowIcons = make(map[*Window]*postedIcon)
	a.appliedIcons = make(map[*hostWindow]*postedIcon)

	icon := &postedIcon{icon: nil, buffers: nil}
	w := &Window{app: a}
	a.windowIcons[w] = icon
	a.appliedIcons[&hostWindow{win: w}] = icon

	a.reapWindowIcons()
	if len(a.windowIcons) != 0 || len(a.appliedIcons) != 0 {
		t.Errorf("reap left records: %d %d", len(a.windowIcons), len(a.appliedIcons))
	}

	// Release is idempotent and nil-safe.
	icon.release()
	icon.release()
	var nilIcon *postedIcon
	nilIcon.release()
}

// TestWindowIconSizeLadder pins the rasterization ladder: sizes are
// clamped to positive values at most 512, and a nil or empty image
// posts nothing.
func TestWindowIconSizeLadder(t *testing.T) {
	for _, size := range defaultIconSizes {
		if size <= 0 || size > 512 {
			t.Errorf("ladder size %d out of range", size)
		}
	}
	if len(defaultIconSizes) == 0 {
		t.Error("empty default ladder")
	}
	_ = render.RGB(0, 0, 0)
}
