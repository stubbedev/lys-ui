package widget

// The CSS golden set (docs/css.md exit criterion 4): every property of
// the table shown applied on a consumer widget. New properties add a
// shot here; the pre-CSS goldens must stay untouched — the zero-style
// case is today's pixels.

import (
	"testing"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// cssShot pins the palette, loads css for the shot, and hands the
// rest to NewGolden.
func cssShot(t *testing.T, css string, w Widget, name string, opts ...goldenOption) {
	t.Helper()
	loadCSS(t, css)
	NewGolden(t, w, name, opts...)
}

func TestGoldenCSSButtonProperties(t *testing.T) {
	face := goldenFace(t)
	// background-color, border-radius, padding, border-width,
	// border-color in one rule on one consumer.
	btn := NewButton(NewLabel(face, 14, "Save", render.RGB(0xff, 0xff, 0xff)), 10, 6)
	btn.AddClass("primary")
	cssShot(t, `
		.primary {
			background-color: #7b3fa0;
			border-radius: 14;
			padding: 16;
			border-width: 3;
			border-style: solid;
			border-color: #30d5c8;
		}
	`, btn, "css-button-properties")
}

func TestGoldenCSSColorAndInheritance(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	// color on a label directly, and color inherited from a styled box
	// into a zero-color label — the inheritance half of the set.
	direct := NewLabel(face, 14, "direct color", 0)
	inherited := NewBox(Column, 8, 8)
	inherited.Append(NewLabel(face, 14, "inherited", 0), false)
	cssShot(t, `label { color: #e05561; }`, direct, "css-label-color", goldenTheme(th))
	cssShot(t, `box { color: #3fb68b; }`, inherited, "css-label-color-inherited", goldenTheme(th))
}

func TestGoldenCSSFontSize(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	lbl := NewLabel(face, 12, "sized by the sheet", th.Text)
	cssShot(t, `label { font-size: 20; }`, lbl, "css-label-font-size", goldenTheme(th))
}

// TestGoldenCSSPseudoElements pins ::before/::after content painting
// on a label: generated text in the pseudo-element's own color, at
// the content start and end.
func TestGoldenCSSPseudoElements(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	lbl := NewLabel(face, 14, "tagged", th.Text)
	lbl.AddClass("tag")
	cssShot(t, `
		.tag::before { content: "» "; color: #e05561; }
		.tag::after { content: " «"; color: #3fb68b; }
	`, lbl, "css-pseudo-elements", goldenTheme(th))
}

// installFixtureResolver wires the family/weight lookup to the two
// fixture faces: the Hebrew face stands in for both the "hebrew"
// family and the bold variant, since the fixture set carries two
// regular faces. It also records the lookups, so tests can pin that
// the declaration reached the resolver verbatim.
func installFixtureResolver(t *testing.T) *[]string {
	t.Helper()
	hebrew, err := render.NewFixtureHebrewTypeface()
	if err != nil {
		t.Fatalf("load hebrew fixture: %v", err)
	}
	var calls []string
	SetFaceResolver(func(family string, weight int) (render.Font, bool) {
		calls = append(calls, family+"/"+itoa(weight))
		if family == "hebrew" || weight >= 600 {
			return hebrew, true
		}
		return nil, false
	})
	t.Cleanup(func() { SetFaceResolver(nil) })
	return &calls
}

func TestGoldenCSSFontFamily(t *testing.T) {
	face := goldenFace(t)
	resolverCalls := installFixtureResolver(t)
	// Cantarell covers no Hebrew: without the sheet the label is
	// .notdef boxes; with font-family the resolver's face shapes it.
	lbl := NewLabel(face, 20, "אבג home", render.RGB(0xff, 0xff, 0xff))
	loadCSS(t, `label { font-family: "hebrew"; }`)
	NewGolden(t, lbl, "css-label-font-family")
	if len(*resolverCalls) == 0 || (*resolverCalls)[0] != "hebrew/0" {
		t.Errorf("resolver calls = %v, want hebrew/0", *resolverCalls)
	}
}

func TestGoldenCSSFontWeight(t *testing.T) {
	face := goldenFace(t)
	resolverCalls := installFixtureResolver(t)
	lbl := NewLabel(face, 20, "אבג home", render.RGB(0xff, 0xff, 0xff))
	loadCSS(t, `label { font-weight: bold; }`)
	NewGolden(t, lbl, "css-label-font-weight")
	// No font-family in the cascade: the weight applies to the label's
	// own family, never to an empty one the resolver would answer with
	// its default face.
	if want := face.Family() + "/700"; len(*resolverCalls) == 0 || (*resolverCalls)[0] != want {
		t.Errorf("resolver calls = %v, want %s", *resolverCalls, want)
	}
}

func TestGoldenCSSEntryMinAndBorder(t *testing.T) {
	face := goldenFace(t)
	// min-width floors the field well past its natural size, and the
	// border-width/border-color ring strokes it; background-color
	// fills between.
	entry := NewEntry(face, 14, render.RGB(0xff, 0xff, 0xff))
	entry.SetText("typed")
	cssShot(t, `
		entry {
			min-width: 260;
			border-width: 2;
			border-style: solid;
			border-color: #f5a623;
			background-color: #1b2a34;
		}
	`, entry, "css-entry-min-border")
}

func TestGoldenCSSTextAreaBackground(t *testing.T) {
	face := goldenFace(t)
	area := NewTextArea(face, 13, render.RGB(0xff, 0xff, 0xff))
	area.SetText("styled area\nsecond line")
	cssShot(t, `textview { background-color: #123456; }`, area, "css-textarea-background")
}

func TestGoldenCSSMediaQuery(t *testing.T) {
	face := goldenFace(t)
	SetMediaViewport(640, 480)
	t.Cleanup(func() { SetMediaViewport(0, 0) })
	btn := NewButton(NewLabel(face, 14, "responsive", render.RGB(0xff, 0xff, 0xff)), 10, 6)
	cssShot(t, `
		@media (min-width: 500px) {
			button { background-color: #4488cc; }
		}
	`, btn, "css-media-query")
	if bg := StyleBackground(btn); bg != render.RGB(0x44, 0x88, 0xcc) {
		t.Errorf("media rule cascade = %08x", uint32(bg))
	}
}

// TestGoldenToast pins Sonner's look: the popover card with its
// border, and the action as a small primary button.
func TestGoldenToast(t *testing.T) {
	face := goldenFace(t)
	for _, shot := range bothPalettes("toast") {
		toast := NewToast(face, "Event has been created", 0)
		toast.SetAction("Undo", func() {})
		NewGolden(t, toast, shot.name, goldenTheme(shot.th), goldenAfterArrange(func() { toast.progress = 1 }))
	}
}

// TestGoldenToastDescription pins Sonner's two-line toast: the muted
// description under the title, the block centered beside the action.
func TestGoldenToastDescription(t *testing.T) {
	face := goldenFace(t)
	for _, shot := range bothPalettes("toast-description") {
		toast := NewToast(face, "Event has been created", 0)
		toast.SetDescription("Sunday, December 03, 2023 at 9:00 AM")
		toast.SetAction("Undo", func() {})
		NewGolden(t, toast, shot.name, goldenTheme(shot.th), goldenAfterArrange(func() { toast.progress = 1 }))
	}
}

func TestGoldenCSSToastShadow(t *testing.T) {
	face := goldenFace(t)
	toast := NewToast(face, "saved", 0)
	cssShot(t, `
		toast {
			box-shadow: #a04000 24;
			border-radius: 10;
		}
	`, toast, "css-toast-shadow", goldenAfterArrange(func() {
		toast.progress = 1
	}))
	// The glow is subtle on the card; pin the cascade that painted it.
	v := toast.style(toast)
	if !v.Has(style.PropBoxShadow) || v.Shadow.N != 1 || v.Shadow.Layers[0].Color != render.RGBA(0xa0, 0x40, 0x00, 0xff) || v.Shadow.Layers[0].Blur != 24 {
		t.Errorf("toast shadow cascade = %+v", v.Shadow)
	}
}

// TestGoldenCSSLayout pins the CSS layout engines on one page: a flex
// bar spacing its chips apart and centering them across, a wrapping
// flex grid of growing tiles with gaps, a positioned stage whose
// absolutely placed cards stack by z-index and whose overflow clips the
// one spilling out, and a block flow with a float and inline blocks.
func TestGoldenCSSLayout(t *testing.T) {
	box := func(classes ...string) *Box {
		b := NewBox(Row, 0, 0)
		b.AddClass(classes...)
		return b
	}
	with := func(parent *Box, kids ...*Box) *Box {
		for _, k := range kids {
			parent.Append(k, false)
		}
		return parent
	}
	bar := with(box("bar"), box("chip", "c1"), box("chip", "c2"), box("chip", "c3"))
	tiles := box("tiles")
	for range 7 {
		tiles.Append(box("tile"), false)
	}
	stage := with(box("stage"), box("card", "back"), box("card", "front"), box("spill"))
	flow := with(box("flow"), box("float"), box("ib"), box("ib"), box("ib"), box("line"))
	page := with(NewBox(Column, 0, 0), bar, tiles, stage, flow)
	page.AddClass("page")
	const w, h = 320, 300
	cssShot(t, `
		.page { display: flex; flex-direction: column; gap: 8px; padding: 8px; background-color: #1e1e2e; }
		.bar { display: flex; justify-content: space-between; align-items: center; padding: 6px;
			background-color: #313244; border-radius: 6px; }
		.chip { min-width: 40px; border-radius: 4px; }
		.c1 { min-height: 16px; background-color: #89b4fa; }
		.c2 { min-height: 28px; min-width: 90px; background-color: #cba6f7; }
		.c3 { min-height: 20px; background-color: #94e2d5; }
		.tiles { display: flex; flex-wrap: wrap; gap: 6px; }
		.tile { flex: 1 1 60px; min-height: 24px; background-color: #fab387; border-radius: 3px; }
		.stage { position: relative; height: 70px; overflow: hidden; background-color: #45475a; border-radius: 6px; }
		.card { position: absolute; width: 90px; height: 40px; border-radius: 4px; }
		.back { left: 12px; top: 8px; z-index: 1; background-color: #a6e3a1; }
		.front { left: 60px; top: 24px; z-index: 2; background-color: #f38ba8; }
		.spill { position: absolute; right: -20px; bottom: -12px; width: 70px; height: 36px; background-color: #f9e2af; }
		.flow { display: block; padding: 4px; background-color: #313244; }
		.float { float: right; width: 60px; height: 40px; background-color: #74c7ec; }
		.ib { display: inline-block; width: 50px; height: 18px; margin: 2px; background-color: #b4befe; }
		.line { height: 10px; background-color: #f5c2e7; clear: both; }
	`, page, "css-layout", goldenFrame(w, h), goldenAfterArrange(func() { settle(t, page, w, h) }))
}

// TestGoldenCSSGrid pins the grid layout on one page: a named-area
// template (header across, a fixed sidebar, an fr main split by a
// nested auto-fill grid of tiles, one spanning two tracks, a footer),
// gaps, self alignment (a centered badge, an end-aligned chip), dense
// auto-placement, and an absolutely positioned marker on grid lines.
func TestGoldenCSSGrid(t *testing.T) {
	box := func(classes ...string) *Box {
		b := NewBox(Row, 0, 0)
		b.AddClass(classes...)
		return b
	}
	with := func(parent *Box, kids ...*Box) *Box {
		for _, k := range kids {
			parent.Append(k, false)
		}
		return parent
	}
	tiles := box("tiles")
	for i := range 7 {
		if i == 1 {
			tiles.Append(box("tile", "wide"), false)
			continue
		}
		tiles.Append(box("tile"), false)
	}
	page := with(box("page"),
		with(box("head"), box("badge")),
		with(box("side"), box("chip")),
		tiles,
		box("foot"),
		box("marker"))
	const w, h = 320, 240
	cssShot(t, `
		.page { display: grid; position: relative; padding: 8px; gap: 6px; background-color: #1e1e2e;
			grid-template: "head head" 36px "side main" 1fr "foot foot" 24px / 80px 1fr; }
		.head { grid-area: head; display: grid; background-color: #313244; border-radius: 6px; }
		.badge { width: 60px; height: 16px; place-self: center; background-color: #f9e2af; border-radius: 8px; }
		.side { grid-area: side; display: grid; background-color: #45475a; border-radius: 6px; }
		.chip { width: 40px; height: 20px; margin: 6px; justify-self: end; align-self: end;
			background-color: #89b4fa; border-radius: 4px; }
		.tiles { grid-area: main; display: grid; gap: 4px; grid-auto-flow: dense;
			grid-template-columns: repeat(auto-fill, minmax(50px, 1fr)); grid-auto-rows: 30px; }
		.tile { background-color: #fab387; border-radius: 3px; }
		.wide { grid-column: span 3; background-color: #cba6f7; }
		.foot { grid-area: foot; background-color: #313244; border-radius: 6px; }
		.marker { position: absolute; grid-area: side; right: 4px; top: 4px; width: 10px; height: 10px;
			background-color: #f38ba8; border-radius: 5px; }
	`, page, "css-grid", goldenFrame(w, h), goldenAfterArrange(func() { settle(t, page, w, h) }))
}

func TestGoldenCSSDialogCard(t *testing.T) {
	face := goldenFace(t)
	// The app-level surfaces name their card; dialog styles the
	// elevation's shadow, plate, and radius.
	card := NewElevation(NewLabel(face, 14, "dialog body", render.RGB(0xff, 0xff, 0xff))).
		WithRadius(6).
		WithPlate(DarkTheme().Surface)
	card.SetElement("dialog")
	cssShot(t, `
		dialog {
			box-shadow: #30d5c8 20;
			border-radius: 16;
			background-color: #202233;
		}
	`, card, "css-dialog-card")
}
