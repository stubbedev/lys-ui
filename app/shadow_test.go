// Box shadows through the real frame pipeline: a toast in the window's
// damage-tracked tree opens with exactly card-plus-shadow-ring damage,
// settles to nothing, and a hover twitch re-owes only the card — the
// cache makes the ring a no-op between geometry and theme changes.
package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/widget"
)

// drainAnim runs every scheduled animation callback to completion at
// clock speed, leaving the toast at rest.
func drainAnim(t *testing.T, now *time.Time) {
	t.Helper()
	for i := 0; ; i++ {
		wake, ok := animclock.Next()
		if !ok {
			return
		}
		if wake.After(*now) {
			*now = wake
		}
		animclock.Tick(*now)
		if i > 1000 {
			t.Fatal("animation schedule did not drain")
		}
	}
}

// frameDamage draws one frame and returns only the damage rects the
// wire saw (frame() also returns the painted-pixel count, which not
// every assertion needs).
func (h *paintHarness) frameDamage() []render.Rect {
	_, damage := h.frame()
	return damage
}

func TestToastShadowDamageInWindow(t *testing.T) {
	defer widget.SetTheme(widget.DarkTheme())
	widget.SetTheme(widget.DarkTheme())

	base := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return base }))
	t.Cleanup(animclock.Reset)
	animclock.Reset()

	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 320, 200)
	layer := &toastLayer{hw: h.wnd, root: root, margin: 16, max: 3}
	h.wnd.router.Root = layer
	h.warm(nil, 3) // present the empty frame; the toast frame lands in a clean, presented buffer

	toast := widget.NewToast(testFace(t), "Saved", 0)
	toast.OnDismissed = func() { layer.remove(toast) }
	layer.add(toast)

	// Opening the card invalidates the card plus its shadow ring —
	// and nothing beyond the ring (both clip to the window: the ring
	// hangs off the bottom edge here).
	damage := h.frameDamage()
	if toast.Bounds().Empty() {
		t.Fatal("toast never arranged")
	}
	ring := widget.Current().ShadowRing(toast.Bounds()).Intersect(render.Rect{W: 320, H: 200})
	if got := render.UnionAll(damage); got != ring {
		t.Errorf("open damage %v, want card plus shadow ring %v", got, ring)
	}
	for i, r := range damage {
		if ring != ring.Union(r) {
			t.Errorf("open damage rect %d (%v) escapes the ring — the gutter must be the only out-of-bounds ink", i, r)
		}
	}

	// The entrance tween drives the card to rest; its animated frames
	// put ink in the ring.
	drainAnim(t, &base)
	if painted, _ := h.frame(); painted == 0 {
		t.Fatal("the toast at rest painted nothing")
	}

	if painted, damage := h.frame(); painted != 0 || len(damage) != 0 {
		t.Errorf("settled frame painted %d px with damage %v, want a skip", painted, damage)
	}

	// The hover-twitch contract: repaint the card, never re-send the
	// ring. The first twitch lands in the other pooled buffer, which
	// still owes the rest frame's ring; from the second on, the
	// rotation is in sync and only the card repaints.
	toast.HoverMove(widget.Point{X: 60, Y: 50})
	if painted, _ := h.frame(); painted == 0 {
		t.Fatal("hover twitch painted nothing")
	}
	toast.HoverMove(widget.Point{X: 61, Y: 50})
	if painted, damage := h.frame(); painted == 0 {
		t.Fatal("hover twitch painted nothing")
	} else if got := render.UnionAll(damage); got != toast.Bounds() {
		t.Errorf("hover twitch damaged %v, want the logical bounds only", got)
	}

	// Dismissal runs the exit fade with the card still in the tree
	// (each tween frame repaints card plus ring), then detaches it —
	// a structural change the window flag repaints. Either way the
	// frame after the dust settles owes nothing.
	toast.Dismiss()
	drainAnim(t, &base)
	h.frame()
	if painted, damage := h.frame(); painted != 0 || len(damage) != 0 {
		t.Errorf("frame after dismissal painted %d px with damage %v, want a skip", painted, damage)
	}
}
