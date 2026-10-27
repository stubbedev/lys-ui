// Box-shadow behavior at the widget layer: the theme toggle and
// restyle knobs, the hit-test rule (the gutter is never a hit), and
// the damage ring (owed exactly when the shadow changes, never on a
// hover twitch). Direct pixel assertions — golden snapshots for the
// on/off/blur variants should move into #45's golden harness once it
// lands.
package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// paintCard paints an Elevation card — plate plus shadow — on a
// transparent canvas under th and returns the raw buffer.
func paintCard(t *testing.T, th *Theme) []byte {
	t.Helper()
	SetTheme(th)
	child := NewBox(Column, 4, 4)
	e := NewElevation(child).WithPlate(render.RGB(200, 60, 60))
	SizeWithin(e, Size{W: 300, H: 300})
	e.Arrange(render.Rect{X: 20, Y: 20, W: 40, H: 30})
	data := make([]byte, render.Stride(80)*80)
	e.Paint(render.New(data, render.Stride(80), 80, 80))
	return data
}

func pixelAt(data []byte, x, y int) render.Color {
	return render.ColorFromBytes(data[y*render.Stride(80)+x*4:])
}

// gutterPt sits 4 logical px left of the paintCard card: inside the
// shadow ring, outside the logical bounds.
var gutterPt = [2]int{16, 35}

func TestThemeShadowToggle(t *testing.T) {
	defer SetTheme(DarkTheme())

	on := paintCard(t, DarkTheme())
	if on[gutterPt[1]*render.Stride(80)+gutterPt[0]*4+3] == 0 {
		t.Error("default theme painted no shadow in the gutter")
	}
	if got := pixelAt(on, 40, 35); got != render.RGB(200, 60, 60) {
		t.Errorf("plate = %#08x, want the opaque plate color", uint32(got))
	}

	for name, th := range map[string]*Theme{
		"blur zero":            DarkTheme().WithShadowBlur(0),
		"from-scratch palette": {},
	} {
		off := paintCard(t, th)
		if got := pixelAt(off, 16, 35); got != render.Color(0) {
			t.Errorf("%s: gutter pixel = %#08x, want untouched transparent — shadows must be disableable", name, uint32(got))
		}
	}

	// The zero color means unset, not transparent: it derives the
	// default, same as every theme color — identical to spelling the
	// default out.
	derived := paintCard(t, DarkTheme().WithShadowColor(render.Color(0)))
	explicit := paintCard(t, DarkTheme().WithShadowColor(defaultShadowColor))
	if pixelAt(derived, 16, 35) != pixelAt(explicit, 16, 35) {
		t.Error("unset shadow color did not derive the default")
	}
}

func TestThemeShadowRestyle(t *testing.T) {
	defer SetTheme(DarkTheme())

	t.Run("blur widens the ring", func(t *testing.T) {
		narrow := paintCard(t, DarkTheme().WithShadowBlur(8))
		wide := paintCard(t, DarkTheme().WithShadowBlur(24))
		// 12 px out: inside the wide falloff, past the narrow one.
		if got := pixelAt(narrow, 8, 35); got != render.Color(0) {
			t.Errorf("blur 8 at 12px out = %#08x, want untouched", uint32(got))
		}
		if got := pixelAt(wide, 8, 35); got.A() == 0 {
			t.Error("blur 24 painted nothing 12px out; the falloff did not follow ShadowBlur")
		}
	})

	t.Run("color restyles the ring", func(t *testing.T) {
		red := paintCard(t, DarkTheme().WithShadowColor(render.RGBA(255, 0, 0, 220)))
		blue := paintCard(t, DarkTheme().WithShadowColor(render.RGBA(0, 0, 255, 220)))
		r, b := pixelAt(red, 16, 35), pixelAt(blue, 16, 35)
		if r == b || r.R() == 0 || b.B() == 0 {
			t.Errorf("shadow color did not reach the ring: red %#08x blue %#08x", uint32(r), uint32(b))
		}
	})
}

func TestShadowGutterMatchesBlur(t *testing.T) {
	defer SetTheme(DarkTheme())

	if got := DarkTheme().ShadowGutter(); got != DarkTheme().ShadowBlur {
		t.Errorf("ShadowGutter = %d, want the blur %d", got, DarkTheme().ShadowBlur)
	}
	if got := DarkTheme().WithShadowBlur(0).ShadowGutter(); got != 0 {
		t.Errorf("disabled blur gutter = %d, want 0", got)
	}
}

func TestShadowRingMatchesPaint(t *testing.T) {
	defer SetTheme(DarkTheme())

	SetTheme(DarkTheme())
	th := Current()
	bounds := render.Rect{X: 20, Y: 20, W: 40, H: 30}
	ring := th.ShadowRing(bounds)
	if ring.Empty() {
		t.Fatal("ShadowRing empty while shadows are on")
	}
	if want := expandRect(bounds, th.ShadowBlur+1); ring != want {
		t.Errorf("ring = %v, want the bounds grown by blur+1 (%v)", ring, want)
	}

	// The ring must cover every pixel the shadow paints: draw onto a
	// fresh canvas and probe the ring's edge and corners.
	data := make([]byte, render.Stride(100)*100)
	cv := render.New(data, render.Stride(100), 100, 100)
	th.DrawShadow(cv, bounds, 6)
	for _, p := range [][2]int{
		{ring.X, ring.Y + 10},
		{ring.X + ring.W - 1, ring.Y + 10},
		{ring.X + 10, ring.Y},
		{ring.X + 10, ring.Y + ring.H - 1},
	} {
		if got := render.ColorFromBytes(data[p[1]*render.Stride(100)+p[0]*4:]); got != render.Color(0) {
			t.Errorf("shadow inked outside its ring at (%d,%d) = %#08x", p[0], p[1], uint32(got))
		}
	}
}

func TestElevationHitTestExcludesGutter(t *testing.T) {
	defer SetTheme(DarkTheme())
	SetTheme(DarkTheme())

	child := NewBox(Column, 2, 2)
	e := NewElevation(child)
	SizeWithin(e, Size{W: 300, H: 300})
	e.Arrange(render.Rect{X: 20, Y: 20, W: 40, H: 30})

	if hit := e.HitTest(Point{X: 15, Y: 35}); hit != nil {
		t.Errorf("point 5px left of the card hit %T — the shadow gutter must never be a hit", hit)
	}
	if hit := e.HitTest(Point{X: 30, Y: 55}); hit != nil {
		t.Errorf("point 5px below the card hit %T — the shadow gutter must never be a hit", hit)
	}
	if hit := e.HitTest(Point{X: 30, Y: 35}); hit != child {
		t.Errorf("point on the card hit %T, want the child", hit)
	}
}

func TestElevationRingDamage(t *testing.T) {
	defer SetTheme(DarkTheme())
	SetTheme(DarkTheme()) // blur 16

	e := NewElevation(NewBox(Column, 0, 0)).WithPlate(render.RGB(10, 10, 10))
	SizeWithin(e, Size{W: 300, H: 300})
	e.Arrange(render.Rect{X: 20, Y: 20, W: 40, H: 30})

	rects, any := CollectDamage(e)
	ring := Current().ShadowRing(e.Bounds())
	if !any {
		t.Fatal("a freshly arranged card owed no damage")
	}
	if got := render.UnionAll(rects); got != ring {
		t.Errorf("open damage union %v, want bounds plus the shadow ring %v", got, ring)
	}
	for i, r := range rects {
		if ring != ring.Union(r) {
			t.Errorf("open damage rect %d (%v) escapes the shadow ring — the gutter must be the only out-of-bounds ink", i, r)
		}
	}

	if rects, any := CollectDamage(e); any {
		t.Errorf("steady state owed damage again: %v", rects)
	}

	// The hover-twitch contract: repainting the card never re-owes the
	// falloff.
	e.Invalidate()
	if rects, any := CollectDamage(e); !any || render.UnionAll(rects) != e.Bounds() {
		t.Errorf("hover repaint damaged %v, want the logical bounds only", rects)
	}

	// A move erases the old ring and paints the new one.
	e.Arrange(render.Rect{X: 60, Y: 60, W: 40, H: 30})
	rects, any = CollectDamage(e)
	want := ring.Union(Current().ShadowRing(e.Bounds()))
	if !any || render.UnionAll(rects) != want {
		t.Errorf("move damaged %v, want the old and new rings %v", rects, want)
	}

	// A theme restyle re-owes the ring even over unchanged bounds —
	// the bounds repaint a SetTheme triggers never covers it. The
	// frame after SetTheme re-arranges before collecting, which is
	// what lets the tracker notice the generation bump. The blur
	// shrank 16 → 4, so the old ring (around the current bounds)
	// contains the new one.
	SetTheme(DarkTheme().WithShadowBlur(4))
	e.Arrange(e.Bounds())
	if rects, any := CollectDamage(e); !any {
		t.Error("theme restyle owed no ring damage")
	} else if got := render.UnionAll(rects); got != expandRect(e.Bounds(), 17) {
		t.Errorf("restyle damaged %v, want the previous blur-16 ring %v", got, expandRect(e.Bounds(), 17))
	}
}

func TestToastShadowHitTestAndRing(t *testing.T) {
	defer SetTheme(DarkTheme())
	SetTheme(DarkTheme())
	pinAnimClock(t)

	toast := NewToast(testFace(t), "Saved", 0)
	SizeWithin(toast, Size{W: 400, H: 100})
	toast.Arrange(render.Rect{X: 40, Y: 40, W: 120, H: 32})

	// Clicks just outside the logical rect pass through, even though
	// the ring (blur 16) covers them.
	for _, p := range []Point{{X: 36, Y: 56}, {X: 165, Y: 56}, {X: 100, Y: 76}} {
		if hit := toast.HitTest(p); hit != nil {
			t.Errorf("point (%d,%d) outside the card hit %T — the gutter must not extend the hit area", p.X, p.Y, hit)
		}
	}
	if hit := toast.HitTest(Point{X: 100, Y: 50}); hit != toast {
		t.Errorf("point on the card hit %T, want the toast", hit)
	}

	// The ring is owed once, then steady; the hover path re-owes only
	// the bounds.
	if rects, any := CollectDamage(toast); !any || render.UnionAll(rects) != Current().ShadowRing(toast.Bounds()) {
		t.Errorf("fresh toast damage %v, want the shadow ring", rects)
	}
	if _, any := CollectDamage(toast); any {
		t.Error("steady toast owed damage again")
	}
	toast.HoverMove(Point{X: 60, Y: 50})
	if rects, any := CollectDamage(toast); !any || render.UnionAll(rects) != toast.Bounds() {
		t.Errorf("hover twitch damaged %v, want the logical bounds only", rects)
	}
}
