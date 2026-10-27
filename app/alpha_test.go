// The alpha-path pin, end to end at the render level: an app background
// with alpha < 255 goes through buffer -> canvas (ClearDevice overwrite)
// -> widget blend -> wl_shm ARGB bytes, and the pixels are compared
// against premultiplied source-over done by hand in this file. Also
// pinned here: the wl_surface.set_opaque_region traffic the frame
// pipeline owes an opaque surface (set iff the background is fully
// opaque, once per size change, never per frame), and the byte order
// the compositor reads through its own mapping. No wayland connection;
// the surfaceHandle fake records the region calls.
package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// solid paints one opaque color over its bounds: the known source the
// compositing checks blend over the background. It is wire-free test
// scaffolding - no invalidation state - so the harness pre-seeds the
// pending damage region to get a frame painted.
type solid struct {
	col     render.Color
	natural widget.Size
	bounds  render.Rect
}

func (s *solid) Measure(axis widget.Axis, forSize int) widget.Sizes {
	return widget.Sizes{Nat: s.natural.Along(axis)}
}

func (s *solid) Arrange(r render.Rect) { s.bounds = r }

func (s *solid) Paint(cv *render.Canvas) { cv.FillRect(s.bounds, s.col) }

func (s *solid) HitTest(p widget.Point) widget.Widget {
	if p.X >= s.bounds.X && p.X < s.bounds.X+s.bounds.W &&
		p.Y >= s.bounds.Y && p.Y < s.bounds.Y+s.bounds.H {
		return s
	}
	return nil
}

// refPremul premultiplies straight components. Independent of
// render.RGBA: the expectations must not be computed by the code under
// test.
func refPremul(r, g, b, a uint8) render.Color {
	pre := func(c uint8) uint32 { return uint32(c) * uint32(a) / 255 }
	return render.Color(uint32(a)<<24 | pre(r)<<16 | pre(g)<<8 | pre(b))
}

// refOver is source-over in premultiplied space, straight from the
// Porter-Duff definition out = src + dst*(1-srcA) with round-half-up
// uint8 stores.
func refOver(src, dst render.Color) render.Color {
	keep := 255 - uint32(src.A())
	term := func(d uint8) uint32 { return (uint32(d)*keep + 127) / 255 }
	return render.Color((uint32(src.A())+term(dst.A()))<<24 |
		(uint32(src.R())+term(dst.R()))<<16 |
		(uint32(src.G())+term(dst.G()))<<8 |
		(uint32(src.B()) + term(dst.B())))
}

// bufferPixel decodes the pixel at (x, y) from raw wl_shm bytes by
// hand: little-endian ARGB8888 means the mapping holds B, G, R, A.
func bufferPixel(data []byte, stride, x, y int) render.Color {
	o := y*stride + x*4
	return render.Color(uint32(data[o+3])<<24 | uint32(data[o+2])<<16 |
		uint32(data[o+1])<<8 | uint32(data[o]))
}

// TestTranslucentBackgroundCompositesThroughTheBufferPath is the
// issue's verification test: a translucent background over a known
// solid, composited through the real frame pipeline (buffer, scaled
// canvas, ClearDevice, widget paint) and compared to the manual
// premultiplied expectation. Clear is an overwrite, so the background
// pixel must equal the premultiplied background exactly - alpha applied
// exactly once (a second application would halve every channel here) -
// and the widget pixel must be the manual source-over result.
func TestTranslucentBackgroundCompositesThroughTheBufferPath(t *testing.T) {
	const (
		br, bg_, bb, ba = 90, 150, 210, uint8(128) // translucent background
		fr, fg, fb      = 250, 120, 40             // the solid, opaque
	)
	root := widget.NewBox(widget.Row, 0, 0)
	sw := &solid{col: render.RGB(fr, fg, fb), natural: widget.Size{W: 24, H: 12}}
	root.Append(sw, false)

	h := newPaintHarness(root, 64, 32)
	h.wnd.cfg.background = render.RGBA(br, bg_, bb, ba)
	h.wnd.damage.Full(render.Rect{W: 64, H: 32})
	painted, damage := h.frame()
	if painted == 0 {
		t.Fatal("frame painted nothing; harness broken")
	}
	if got := render.UnionAll(damage); got != (render.Rect{W: 64, H: 32}) {
		t.Fatalf("damage = %+v, want the full 64x32 buffer", got)
	}
	b := h.bufs[len(h.bufs)-1]

	// Background-only pixel: the ClearDevice overwrite, byte-exact.
	wantBg := refPremul(br, bg_, bb, ba)
	if got := bufferPixel(b.Data, b.Stride, 48, 20); got != wantBg {
		t.Fatalf("background pixel = %+v, want %+v (premultiplied exactly once)", got.Straight(), wantBg.Straight())
	}
	// The alpha channel survived to the buffer: 128, not 64 or 255.
	if got := bufferPixel(b.Data, b.Stride, 48, 20).A(); got != ba {
		t.Fatalf("background alpha = %d, want %d (no double-alpha through Clear)", got, ba)
	}
	// Byte order: the same pixel read through the mapping's native
	// order - B, G, R, A for 0xAARRGGBB.
	o := 20*b.Stride + 48*4
	wantBytes := [4]byte{wantBg.B(), wantBg.G(), wantBg.R(), wantBg.A()}
	var gotBytes [4]byte
	copy(gotBytes[:], b.Data[o:o+4])
	if gotBytes != wantBytes {
		t.Fatalf("background bytes = %v, want %v (little-endian B,G,R,A)", gotBytes, wantBytes)
	}

	// Solid pixel: the widget's opaque fill blended over the
	// translucent background, exactly the manual source-over result.
	sb := sw.bounds
	wantSolid := refOver(refPremul(fr, fg, fb, 255), refPremul(br, bg_, bb, ba))
	if got := bufferPixel(b.Data, b.Stride, sb.X+4, sb.Y+2); got != wantSolid {
		t.Fatalf("solid pixel = %+v, want %+v (manual source-over)", got.Straight(), wantSolid.Straight())
	}
	if got := bufferPixel(b.Data, b.Stride, sb.X+4, sb.Y+2).A(); got != 255 {
		t.Fatalf("solid pixel alpha = %d, want 255 (opaque source stays opaque)", got)
	}
}

// TestOpaqueRegionSetOncePerSizeChange pins the wire traffic an opaque
// window owes: the full surface rect on the first frame and after a
// resize, and nothing in between - an idle frame re-sends nothing.
func TestOpaqueRegionSetOncePerSizeChange(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 64, 32)
	h.wnd.cfg.background = render.RGB(30, 30, 46) // alpha 255: automatic
	h.wnd.cfg.opaque = opaqueFor(h.wnd.cfg.background, false)

	painted, _ := h.frame()
	if painted == 0 {
		t.Fatal("frame painted nothing; harness broken")
	}
	if want := []render.Rect{{W: 64, H: 32}}; !rectsEqual(h.surf.opaque, want) {
		t.Fatalf("first frame opaque regions = %+v, want %+v", h.surf.opaque, want)
	}

	h.frame() // idle: the region tracks the size, so no re-send
	if want := []render.Rect{{W: 64, H: 32}}; !rectsEqual(h.surf.opaque, want) {
		t.Fatalf("idle frame opaque regions = %+v, want unchanged %+v", h.surf.opaque, want)
	}

	h.wnd.host.(*fakeHost).resizeTo(100, 50)
	painted, _ = h.frame()
	if painted == 0 {
		t.Fatal("resize frame painted nothing; harness broken")
	}
	if want := []render.Rect{{W: 64, H: 32}, {W: 100, H: 50}}; !rectsEqual(h.surf.opaque, want) {
		t.Fatalf("post-resize opaque regions = %+v, want %+v", h.surf.opaque, want)
	}
}

// TestOpaqueRegionIsInSurfaceCoordinates pins the units (#165): the
// region is surface-local, the logical size, so a rescale that rebuilds
// the buffers at twice the pixels sends no new region.
func TestOpaqueRegionIsInSurfaceCoordinates(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	fh := newFracHarness(root, 64, 32, 120)
	fh.wnd.cfg.background = render.RGB(30, 30, 46)
	fh.wnd.cfg.opaque = opaqueFor(fh.wnd.cfg.background, false)

	fh.frame()
	fh.wnd.rescale(240)
	fh.frame()

	want := []render.Rect{{W: 64, H: 32}}
	if !rectsEqual(fh.surf.opaque, want) {
		t.Fatalf("opaque regions across rescale = %+v, want only the logical %+v", fh.surf.opaque, want)
	}
}

// TestTranslucentBackgroundNeverSetsOpaqueRegion is the panels leg: a
// background with alpha < 255 must leave the surface blendable, so no
// opaque region ever goes out - through frames and a resize.
func TestTranslucentBackgroundNeverSetsOpaqueRegion(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 64, 32)
	h.wnd.cfg.background = render.RGBA(30, 30, 46, 216)
	h.wnd.cfg.opaque = opaqueFor(h.wnd.cfg.background, false)
	if h.wnd.cfg.opaque {
		t.Fatal("translucent background resolved opaque; opaqueFor is broken")
	}

	h.frame()
	h.wnd.host.(*fakeHost).resizeTo(100, 50)
	h.frame()

	if len(h.surf.opaque) != 0 {
		t.Fatalf("translucent window set opaque regions %+v; the compositor would skip the blur blend", h.surf.opaque)
	}
}

// TestOpaqueOverrideSetsRegionDespiteTranslucence pins the explicit
// opt-in: Opaque forces the region even though the background is
// translucent - the caller's promise, not the pipeline's.
func TestOpaqueOverrideSetsRegionDespiteTranslucence(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 64, 32)
	h.wnd.cfg.background = render.RGBA(30, 30, 46, 128)
	h.wnd.cfg.opaque = opaqueFor(h.wnd.cfg.background, true)

	h.frame()
	if want := []render.Rect{{W: 64, H: 32}}; !rectsEqual(h.surf.opaque, want) {
		t.Fatalf("opaque override regions = %+v, want %+v", h.surf.opaque, want)
	}
}

// TestOpaqueFor pins the decision rule: automatic on a fully opaque
// background, never on a translucent one, overridable either way.
func TestOpaqueFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		bg   render.Color
		flag bool
		want bool
	}{
		{"opaque background", render.RGB(1, 2, 3), false, true},
		{"translucent background", render.RGBA(1, 2, 3, 254), false, false},
		{"zero background", 0, false, false},
		{"forced with translucent background", render.RGBA(1, 2, 3, 128), true, true},
	} {
		if got := opaqueFor(tc.bg, tc.flag); got != tc.want {
			t.Errorf("%s: opaqueFor = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// rectsEqual compares recorded vs wanted region lists, order included.
func rectsEqual(got, want []render.Rect) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestOpaqueRegionCutsOutGPUAreas(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 64, 32)
	h.wnd.cfg.background = render.RGB(30, 30, 46)
	h.wnd.cfg.opaque = opaqueFor(h.wnd.cfg.background, false)
	g := &gpuSurface{hw: h.wnd, bounds: render.Rect{X: 4, Y: 4, W: 20, H: 10}}
	h.wnd.gpu = append(h.wnd.gpu, g, &gpuSurface{hw: h.wnd})

	h.frame()
	if len(h.surf.holes) != 1 || !rectsEqual(h.surf.holes[0], []render.Rect{g.bounds}) {
		t.Fatalf("first region holes %v, want the placed area only", h.surf.holes)
	}
	h.frame()
	if len(h.surf.holes) != 1 {
		t.Errorf("an idle frame re-sent the region: %v", h.surf.holes)
	}
	g.bounds.X = 10
	h.wnd.dirty = true
	h.frame()
	if len(h.surf.holes) != 2 || !rectsEqual(h.surf.holes[1], []render.Rect{g.bounds}) {
		t.Errorf("a moved area: holes %v", h.surf.holes)
	}
}
