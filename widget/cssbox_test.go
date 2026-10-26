package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

// addCSS installs css at priority for one test.
func addCSS(t *testing.T, css string, priority int) *Stylesheet {
	t.Helper()
	s := AddStylesheet(css, priority)
	t.Cleanup(s.Remove)
	return s
}

// paintTree paints root onto a fresh w x h canvas and returns the
// pixels.
func paintTree(root Widget, w, h int) []byte {
	data := make([]byte, render.Stride(w)*h)
	root.Paint(render.New(data, render.Stride(w), w, h))
	return data
}

func TestBoxModelMarginBorderPadding(t *testing.T) {
	loadCSS(t, `.b { margin: 1px 2px 3px 4px; padding: 5px 6px 7px 8px; border: 1px solid #ffffff; }`)
	child := NewSpacer(10, 10)
	box := NewBox(Row, 0, 0)
	box.Append(child, false)
	box.AddClass("b")
	sz := box.Measure(Constraints{Max: Size{W: 200, H: 200}})
	if want := (Size{W: 10 + 2 + 4 + 2 + 6 + 8, H: 10 + 1 + 3 + 2 + 5 + 7}); sz != want {
		t.Fatalf("measure = %+v, want %+v (content + margin + border + padding)", sz, want)
	}
	box.Arrange(render.Rect{W: sz.W, H: sz.H})
	if got, want := box.Bounds(), (render.Rect{X: 4, Y: 1, W: sz.W - 6, H: sz.H - 4}); got != want {
		t.Errorf("border box = %+v, want %+v (the margin stays outside)", got, want)
	}
	if got, want := child.Bounds(), (render.Rect{X: 4 + 1 + 8, Y: 1 + 1 + 5, W: 10, H: 10}); got != want {
		t.Errorf("content box = %+v, want %+v", got, want)
	}
	// A hit on the margin misses the box: margins are not the widget.
	if box.HitTest(Point{X: 1, Y: 1}) != nil {
		t.Error("the margin hit the box")
	}

	// Without a stylesheet the programmatic per-side padding applies;
	// the stylesheet overrides only the sides it sets.
	LoadStylesheet("")
	plain := NewBox(Row, 0, 0)
	plain.Append(NewSpacer(10, 10), false)
	plain.SetPadding(render.Insets{Top: 1, Right: 2, Bottom: 3, Left: 4})
	if got := plain.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 16, H: 14}) {
		t.Errorf("programmatic per-side padding: %+v", got)
	}
	loadCSS(t, `box { padding-left: 20px; }`)
	if got := plain.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 32, H: 14}) {
		t.Errorf("stylesheet padding-left over programmatic: %+v", got)
	}
}

func TestMinSizeFloorsTheContentBox(t *testing.T) {
	loadCSS(t, `box { min-width: 40px; min-height: 20px; padding: 2px; }`)
	box := NewBox(Row, 0, 0)
	box.Append(NewSpacer(10, 10), false)
	if got := box.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 44, H: 24}) {
		t.Errorf("min size floors the content box, padding outside: %+v", got)
	}
	// The floor never beats the constraints.
	if got := box.Measure(Constraints{Max: Size{W: 30, H: 100}}); got.W != 30 {
		t.Errorf("min-width overflowed the constraint: %+v", got)
	}
}

func TestBorderSpacing(t *testing.T) {
	loadCSS(t, `box { border-spacing: 7px 3px; }`)
	row := NewBox(Row, 1, 0)
	row.Append(NewSpacer(10, 10), false)
	row.Append(NewSpacer(10, 10), false)
	// The CSS spacing adds to the constructor's, as in GTK 4.
	if got := row.Measure(Constraints{Max: Size{W: 100, H: 100}}).W; got != 28 {
		t.Errorf("row = %d total, want the gap 1 + 7", got)
	}
	col := NewBox(Column, 1, 0)
	col.Append(NewSpacer(10, 10), false)
	col.Append(NewSpacer(10, 10), false)
	if got := col.Measure(Constraints{Max: Size{W: 100, H: 100}}).H; got != 24 {
		t.Errorf("column = %d total, want the gap 1 + 3", got)
	}
	loadCSS(t, `* { border-spacing: 0; }`)
	if got := row.Measure(Constraints{Max: Size{W: 100, H: 100}}).W; got != 21 {
		t.Errorf("a zero border-spacing erased the constructor spacing: %d", got)
	}
	LoadStylesheet("")
	if got := row.Measure(Constraints{Max: Size{W: 100, H: 100}}).W; got != 21 {
		t.Errorf("without a sheet the constructor spacing applies: %d", got)
	}
}

func TestInlineStyleCustomPropertiesAndPriority(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `label { color: var(--tint, #010101); } box { color: #020202; }`)
	box := NewBox(Column, 0, 0)
	lbl := NewLabel(face, 14, "x", 0)
	box.Append(lbl, false)
	arrangeTree(t, box, 100, 40)
	if got := lbl.style(lbl).Color; got != render.RGB(1, 1, 1) {
		t.Fatalf("fallback before any inline var: %08x", uint32(got))
	}
	box.SetInlineStyle("--tint: #0a0b0c")
	CollectDamage(box)
	if got := lbl.style(lbl).Color; got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("inline custom property did not reach the child: %08x", uint32(got))
	}
	// Changing the variable restyles the subtree.
	box.SetInlineStyle("--tint: #0d0e0f")
	rects, _ := CollectDamage(box)
	if got := lbl.style(lbl).Color; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("var change did not restyle the child: %08x", uint32(got))
	}
	if !slices.Contains(rects, lbl.Bounds()) {
		t.Errorf("the child did not repaint: %v", rects)
	}
	if box.InlineStyle() != "--tint: #0d0e0f" {
		t.Errorf("InlineStyle = %q", box.InlineStyle())
	}

	// Inline (user priority) beats the application sheet, loses to a
	// higher-priority sheet — GTK's provider order.
	lbl.SetInlineStyle("color: #111111")
	CollectDamage(box)
	if got := lbl.style(lbl).Color; got != render.RGB(0x11, 0x11, 0x11) {
		t.Errorf("inline lost to the application sheet: %08x", uint32(got))
	}
	top := addCSS(t, `label { color: #222222; }`, StylePriorityUser+100)
	CollectDamage(box)
	if got := lbl.style(lbl).Color; got != render.RGB(0x22, 0x22, 0x22) {
		t.Errorf("higher-priority sheet lost to inline: %08x", uint32(got))
	}
	top.Remove()
	top.Remove() // idempotent
	CollectDamage(box)
	if got := lbl.style(lbl).Color; got != render.RGB(0x11, 0x11, 0x11) {
		t.Errorf("removal did not restore: %08x", uint32(got))
	}
	top.Load(`label { color: #333333; }`)
	CollectDamage(box)
	if got := lbl.style(lbl).Color; got != render.RGB(0x33, 0x33, 0x33) || top.Priority() != StylePriorityUser+100 {
		t.Errorf("Load after Remove did not reinstall: %08x", uint32(got))
	}
	lbl.SetInlineStyle("")
	top.Remove()
	CollectDamage(box)
	if got := lbl.style(lbl).Color; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("clearing the inline style: %08x", uint32(got))
	}
}

func TestHoverChainMatchesAncestors(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `.outer:hover label { color: #00ff00; } label { color: #ff0000; }`)
	lbl := NewLabel(face, 14, "hi", 0)
	btn := NewButton(lbl, 4, 0)
	outer := NewBox(Row, 0, 0)
	outer.AddClass("outer")
	outer.Append(btn, false)
	other := NewSpacer(40, 20)
	root := NewBox(Row, 0, 0)
	root.Append(outer, false)
	root.Append(other, false)
	arrangeTree(t, root, 200, 40)
	r := &Router{Root: root}
	if got := lbl.style(lbl).Color; got != render.RGB(0xff, 0, 0) {
		t.Fatalf("resting color: %08x", uint32(got))
	}
	r.Move(Point{X: btn.Bounds().X + 2, Y: 5})
	CollectDamage(root)
	if !outer.hoverChain || !root.hoverChain {
		t.Fatal("the hovered button's ancestors are not in the hover chain")
	}
	if got := lbl.style(lbl).Color; got != render.RGB(0, 0xff, 0) {
		t.Errorf("ancestor :hover did not match: %08x", uint32(got))
	}
	r.Leave()
	CollectDamage(root)
	if outer.hoverChain || root.hoverChain {
		t.Error("leave kept the hover chain")
	}
	if got := lbl.style(lbl).Color; got != render.RGB(0xff, 0, 0) {
		t.Errorf("hover chain cleared but the rule still matched: %08x", uint32(got))
	}
}

func TestFocusVisibleAndWithin(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `entry:focus-visible { min-width: 300px; } .form:focus-within { background-color: #123456; }`)
	entry := NewEntry(face, 14, 0)
	form := NewBox(Column, 0, 0)
	form.AddClass("form")
	form.Append(entry, false)
	root := NewBox(Column, 0, 0)
	root.Append(form, false)
	arrangeTree(t, root, 400, 100)
	r := &Router{Root: root}

	r.Press(BTNLeft, Point{X: 5, Y: 5}, 0)
	CollectDamage(root)
	if !entry.focused || entry.focusVisible {
		t.Fatal("a pointer focus must not be focus-visible")
	}
	if entry.style(entry).Has(style.PropMinWidth) {
		t.Error(":focus-visible matched a pointer focus")
	}
	if got := form.style(form).Background; got != render.RGB(0x12, 0x34, 0x56) {
		t.Errorf(":focus-within on the container: %08x", uint32(got))
	}
	if root.focusWithin != 1 {
		t.Errorf("focus-within count on the root = %d", root.focusWithin)
	}
	r.FocusNext()
	CollectDamage(root)
	if !entry.focusVisible || entry.style(entry).Layout.MinWidth.Px() != 300 {
		t.Error("keyboard traversal did not make the focus visible")
	}
	// Focus leaves the form: nothing within it is focused any more.
	r.Forget(entry)
	CollectDamage(root)
	if form.style(form).Has(style.PropBackgroundColor) || form.focusWithin != 0 {
		t.Error(":focus-within outlived the focus")
	}
}

func TestSetStateChecked(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `button:checked { background-color: #0000ff; } switch:checked { min-width: 99px; }`)
	b := NewButton(NewLabel(face, 14, "x", 0), 2, 0)
	arrangeTree(t, b, 100, 30)
	if b.style(b).Has(style.PropBackgroundColor) || b.HasState(StateChecked) {
		t.Fatal("checked before SetState")
	}
	b.SetState(StateChecked, true)
	CollectDamage(b)
	if !b.HasState(StateChecked) || b.style(b).Background != render.RGB(0, 0, 0xff) {
		t.Error("SetState(checked) did not match :checked")
	}
	b.SetState(StateChecked, false)
	CollectDamage(b)
	if b.style(b).Has(style.PropBackgroundColor) {
		t.Error(":checked outlived the flag")
	}
	sw := NewSwitch(true)
	arrangeTree(t, sw, 100, 30)
	if sw.style(sw).Layout.MinWidth.Px() != 99 {
		t.Error("an on switch is :checked intrinsically")
	}
}

func TestSiblingSelectorsFollowStructure(t *testing.T) {
	loadCSS(t, `spacer:first-child { min-height: 5px; } spacer + spacer { min-width: 7px; }`)
	a, b := NewSpacer(1, 1), NewSpacer(1, 1)
	a.SetElement("spacer")
	b.SetElement("spacer")
	row := NewBox(Row, 0, 0)
	row.Append(a, false)
	row.Append(b, false)
	arrangeTree(t, row, 100, 30)
	if a.style(a).Layout.MinHeight.Px() != 5 || b.style(b).Has(style.PropMinHeight) {
		t.Fatal(":first-child")
	}
	if b.style(b).Layout.MinWidth.Px() != 7 || a.style(a).Has(style.PropMinWidth) {
		t.Fatal("adjacent sibling")
	}
	// A new first child takes :first-child from the old one.
	c := NewSpacer(1, 1)
	c.SetElement("spacer")
	row.InsertAt(0, c, false)
	arrangeTree(t, row, 100, 30)
	if a.style(a).Has(style.PropMinHeight) || c.style(c).Layout.MinHeight.Px() != 5 {
		t.Error("InsertAt did not move :first-child")
	}
	// Hiding the first child makes the next one first.
	c.SetVisible(false)
	arrangeTree(t, row, 100, 30)
	if a.style(a).Layout.MinHeight.Px() != 5 {
		t.Error("a hidden first child still counted")
	}
}

func TestOpacityAndBrightnessPaint(t *testing.T) {
	loadCSS(t, `.o { background-color: #ff0000; opacity: 0.5; } .f { background-color: #402010; filter: brightness(2); }`)
	o := NewBox(Row, 0, 0)
	o.Append(NewSpacer(4, 4), false)
	o.AddClass("o")
	arrangeTree(t, o, 4, 4)
	if got := pxColorAt(paintTree(o, 4, 4), render.Stride(4), 1, 1); got.A() < 125 || got.A() > 130 {
		t.Errorf("opacity 0.5 painted alpha %d", got.A())
	}
	f := NewBox(Row, 0, 0)
	f.Append(NewSpacer(4, 4), false)
	f.AddClass("f")
	arrangeTree(t, f, 4, 4)
	if got := pxColorAt(paintTree(f, 4, 4), render.Stride(4), 1, 1); got != render.RGB(0x80, 0x40, 0x20) {
		t.Errorf("brightness(2) = %08x, want doubled channels", uint32(got))
	}
}

func TestIconSizeAndTintFromStylesheet(t *testing.T) {
	loadCSS(t, `box { -gtk-icon-size: 24px; color: #00ff00; }`)
	ic := NewSVGIcon([]byte(testSymbolicSVG), 16)
	box := NewBox(Row, 0, 0)
	box.Append(ic, false)
	arrangeTree(t, box, 50, 50)
	if got := ic.Measure(Constraints{Max: Size{W: 50, H: 50}}); got != (Size{W: 24, H: 24}) {
		t.Errorf("inherited -gtk-icon-size: %+v", got)
	}
	b := ic.Bounds()
	cx, cy := b.X+b.W/2, b.Y+b.H/2
	if got := pxColorAt(paintTree(box, 50, 50), render.Stride(50), cx, cy); got != render.RGB(0, 0xff, 0) {
		t.Errorf("symbolic tint from the stylesheet color: %08x", uint32(got))
	}
	// A pinned tint outranks the stylesheet.
	ic.SetTint(render.RGB(0xff, 0, 0))
	if got := pxColorAt(paintTree(box, 50, 50), render.Stride(50), cx, cy); got != render.RGB(0xff, 0, 0) {
		t.Errorf("programmatic tint lost to the stylesheet: %08x", uint32(got))
	}
}

func TestLabelTextTransform(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `.up { text-transform: uppercase; }`)
	lbl := NewLabel(face, 14, "hello world", render.RGB(1, 1, 1))
	upper := NewLabel(face, 14, "HELLO WORLD", render.RGB(1, 1, 1))
	arrangeTree(t, lbl, 300, 30)
	arrangeTree(t, upper, 300, 30)
	plainW := lbl.Measure(Constraints{Max: Size{W: 300, H: 30}}).W
	lbl.AddClass("up")
	CollectDamage(lbl)
	if got, want := lbl.Measure(Constraints{Max: Size{W: 300, H: 30}}).W, upper.Measure(Constraints{Max: Size{W: 300, H: 30}}).W; got != want || got == plainW {
		t.Errorf("uppercase width %d, want %d (plain %d)", got, want, plainW)
	}
	if lbl.Text() != "hello world" {
		t.Error("text-transform rewrote the label's text")
	}
	for in, want := range map[string]string{"hello big world": "Hello Big World", "": ""} {
		if got := transformText(in, style.TransformCapitalize); got != want {
			t.Errorf("capitalize(%q) = %q", in, got)
		}
	}
}

func TestOutlineAndShadowGrowDamage(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `button { outline: 2px solid #ff0000; outline-offset: 1px; box-shadow: 0 4px 0 #000000; }`)
	b := NewButton(NewLabel(face, 14, "x", 0), 2, 0)
	root := NewBox(Row, 20, 20)
	root.Append(b, false)
	arrangeTree(t, root, 200, 100)
	b.Invalidate()
	rects, _ := CollectDamage(root)
	bb := b.Bounds()
	want := render.Rect{X: bb.X - 3, Y: bb.Y - 3, W: bb.W + 6, H: bb.H + 3 + 4}
	if !slices.Contains(rects, want) {
		t.Errorf("damage %v, want the ink rect %+v (outline 3 around, shadow 4 below)", rects, want)
	}
}

// A tooltip surface reserved the theme's whole shadow gutter around a
// card that painted no shadow, a transparent margin that reached over
// the widget the card described. InkOutset is the room a card's own
// style paints outside it, nothing when it paints nothing there.
func TestInkOutsetIsWhatACardPaintsOutsideItself(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `tooltip { padding: 6px; }`)
	card := NewTooltipCard(face, "tip", false)
	arrangeTree(t, card, 100, 40)
	if ink := InkOutset(card); !ink.Zero() {
		t.Errorf("a shadowless card's ink outset %+v, want none", ink)
	}
	loadCSS(t, `tooltip { box-shadow: 0 4px 6px #000000; }`)
	card = NewTooltipCard(face, "tip", false)
	arrangeTree(t, card, 100, 40)
	want := render.Insets{Top: 2, Right: 6, Bottom: 10, Left: 6}
	if ink := InkOutset(card); ink != want {
		t.Errorf("a shadowed card's ink outset %+v, want its shadow's %+v", ink, want)
	}
}

func TestButtonTransparentFillStrokesARoundedRing(t *testing.T) {
	loadCSS(t, `button { border: 2px solid #ffffff; border-radius: 8px; }`)
	b := NewButton(NewSpacer(20, 20), 0, 0)
	b.BgExplicit = true
	arrangeTree(t, b, 24, 24)
	data := paintTree(b, 24, 24)
	if got := pxColorAt(data, render.Stride(24), 0, 0); got != 0 {
		t.Errorf("corner pixel = %08x, want the rounded ring to leave it empty", uint32(got))
	}
	if got := pxColorAt(data, render.Stride(24), 12, 0); got != render.RGB(0xff, 0xff, 0xff) {
		t.Errorf("edge pixel = %08x, want the ring", uint32(got))
	}
	if got := pxColorAt(data, render.Stride(24), 12, 12); got != 0 {
		t.Errorf("center = %08x, want the transparent fill", uint32(got))
	}
}

func TestSetClassesAndClasses(t *testing.T) {
	loadCSS(t, `.a { min-width: 1px; } .b { min-height: 2px; }`)
	s := NewSpacer(0, 0)
	s.AddClass("a")
	s.SetClasses("b", "b", "")
	if s.HasClass("a") || !s.HasClass("b") || len(s.Classes()) != 1 {
		t.Fatalf("SetClasses: %v", s.Classes())
	}
	v := s.style(s)
	if v.Has(style.PropMinWidth) || v.Layout.MinHeight.Px() != 2 {
		t.Error("the class swap did not restyle")
	}
	cl := s.Classes()
	cl[0] = "mutated"
	if !s.HasClass("b") {
		t.Error("Classes returned the live slice")
	}
}

func TestSetRootFontSize(t *testing.T) {
	loadCSS(t, `box { padding: 1rem; }`)
	t.Cleanup(func() { SetRootFontSize(16) })
	box := NewBox(Row, 0, 0)
	if got := box.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 32, H: 32}) {
		t.Fatalf("1rem at 16px: %+v", got)
	}
	SetRootFontSize(10)
	if RootFontSize() != 10 {
		t.Fatal("RootFontSize")
	}
	if got := box.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 20, H: 20}) {
		t.Errorf("1rem at 10px: %+v", got)
	}
	SetRootFontSize(-1) // ignored
	if RootFontSize() != 10 {
		t.Error("a non-positive size was accepted")
	}
}

func TestAttachedStylesheetStylesItsSubtreeOnly(t *testing.T) {
	face := goldenFace(t)
	sheet := NewStylesheet(`* { padding: 0; } label { color: #00ff00; } .inner label { color: #0000ff; }`, StylePriorityUser)
	a := NewBox(Column, 0, 5)
	la := NewLabel(face, 14, "a", 0)
	a.Append(la, false)
	b := NewBox(Column, 0, 5)
	lb := NewLabel(face, 14, "b", 0)
	b.Append(lb, false)
	arrangeTree(t, a, 100, 40)
	arrangeTree(t, b, 100, 40)

	a.AttachStylesheet(sheet)
	a.AttachStylesheet(sheet) // idempotent
	t.Cleanup(func() { a.DetachStylesheet(sheet) })
	CollectDamage(a)
	CollectDamage(b)
	if got := la.style(la).Color; got != render.RGB(0, 0xff, 0) {
		t.Errorf("attached sheet missed its subtree: %08x", uint32(got))
	}
	if lb.style(lb).Has(style.PropColor) || b.style(b).Has(style.PropPaddingTop) {
		t.Error("an attached sheet leaked into another tree")
	}
	if got, want := a.Measure(Constraints{Max: Size{W: 100, H: 100}}).H, la.Measure(Constraints{Max: Size{W: 100, H: 100}}).H; got != want {
		t.Errorf("the sheet's padding: 0 did not override the programmatic padding: %d, want %d", got, want)
	}
	// Selectors see the full ancestry.
	a.AddClass("inner")
	CollectDamage(a)
	if got := la.style(la).Color; got != render.RGB(0, 0, 0xff) {
		t.Errorf("ancestor selector inside the scope: %08x", uint32(got))
	}
	// A reload restyles every attached subtree.
	sheet.Load(`label { color: #ff0000; }`)
	CollectDamage(a)
	if got := la.style(la).Color; got != render.RGB(0xff, 0, 0) {
		t.Errorf("reload: %08x", uint32(got))
	}
	a.DetachStylesheet(sheet)
	a.DetachStylesheet(sheet) // no-op
	CollectDamage(a)
	if la.style(la).Has(style.PropColor) {
		t.Error("detach kept styling the subtree")
	}
}

func TestInlineStyleInheritsWithoutAnyStylesheet(t *testing.T) {
	face := goldenFace(t)
	LoadStylesheet("")
	box := NewBox(Column, 0, 0)
	lbl := NewLabel(face, 14, "x", 0)
	box.Append(lbl, false)
	box.SetInlineStyle("color: #0a0a0a")
	arrangeTree(t, box, 100, 40)
	if got := lbl.style(lbl).Color; got != render.RGB(10, 10, 10) {
		t.Errorf("inline color did not inherit with no sheet loaded: %08x", uint32(got))
	}
}

func TestOnHoverWithinFollowsTheChain(t *testing.T) {
	face := goldenFace(t)
	btn := NewButton(NewLabel(face, 14, "hi", 0), 4, 0)
	inner := NewLabel(face, 14, "x", 0)
	row := NewBox(Row, 0, 0)
	row.Append(btn, false)
	row.Append(inner, false)
	other := NewSpacer(40, 20)
	root := NewBox(Row, 0, 0)
	root.Append(row, false)
	root.Append(other, false)
	arrangeTree(t, root, 300, 40)
	var events []bool
	row.SetOnHoverWithin(func(on bool) { events = append(events, on) })
	r := &Router{Root: root}

	r.Move(Point{X: btn.Bounds().X + 2, Y: 5})
	// Moving between two children of the row is not a leave.
	r.Move(Point{X: inner.Bounds().X + 1, Y: 5})
	if len(events) != 1 || !events[0] || !row.HoverWithin() {
		t.Fatalf("enter = %v (within %v)", events, row.HoverWithin())
	}
	r.Move(Point{X: other.Bounds().X + 2, Y: 5})
	if len(events) != 2 || events[1] || row.HoverWithin() {
		t.Fatalf("leave = %v", events)
	}
	r.Move(Point{X: btn.Bounds().X + 2, Y: 5})
	r.Leave()
	if len(events) != 4 || !events[2] || events[3] {
		t.Errorf("re-enter and pointer leave = %v", events)
	}
	row.SetOnHoverWithin(nil)
	r.Move(Point{X: btn.Bounds().X + 2, Y: 5})
	if len(events) != 4 {
		t.Error("an unregistered callback still fired")
	}
}

// TestFirstLayoutResolvesScopedStyles pins the first-layout contract: a
// tree measured before it was ever arranged had no parent links, so its
// widgets saw no scoped stylesheet; arranging parents them and leaves
// the layout pending, and the next pass measures the styled sizes.
func TestFirstLayoutResolvesScopedStyles(t *testing.T) {
	face := goldenFace(t)
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(`label { font-size: 40px; }`, StylePriorityUser))
	label := NewLabel(face, 10, "Big", render.RGBA(0, 0, 0, 255))
	root.Append(label, false)
	con := Constraints{Max: Size{W: 400, H: 400}}
	// The label is parented as it measures: the first measure is
	// already styled, and one arrange settles the tree.
	first := root.Measure(con)
	root.Arrange(render.Rect{W: 400, H: 400})
	unstyled := NewLabel(face, 10, "Big", render.RGBA(0, 0, 0, 255)).Measure(con)
	if first.H <= unstyled.H || label.Bounds().H != first.H {
		t.Errorf("first %v (unstyled %v), label %v: the scoped font-size did not take on the first pass", first, unstyled, label.Bounds())
	}
	if LayoutPending(root) {
		t.Error("the first layout left layout pending")
	}
}

// passthrough is a cache-less wrapper, as the inspector overlay is.
type passthrough struct{ child Widget }

func (p passthrough) Measure(c Constraints) Size { return p.child.Measure(c) }
func (p passthrough) Arrange(r render.Rect)      { p.child.Arrange(r) }
func (p passthrough) Paint(cv *render.Canvas)    { p.child.Paint(cv) }
func (p passthrough) HitTest(pt Point) Widget    { return p.child.HitTest(pt) }
func (p passthrough) Children() []Widget         { return []Widget{p.child} }

func TestLayoutPendingSeesThroughWrappers(t *testing.T) {
	root := NewBox(Column, 0, 0)
	root.Append(NewSpacer(10, 10), false)
	wrapped := passthrough{root}
	wrapped.Measure(Constraints{Max: Size{W: 100, H: 100}})
	wrapped.Arrange(render.Rect{W: 100, H: 100})
	// A style change inside makes layout pending.
	root.AttachStylesheet(NewStylesheet(`box { padding: 3px; }`, StylePriorityUser))
	if !LayoutPending(wrapped) {
		t.Fatal("a wrapper hid its root's pending layout")
	}
	for LayoutPending(wrapped) {
		wrapped.Measure(Constraints{Max: Size{W: 100, H: 100}})
		wrapped.Arrange(render.Rect{W: 100, H: 100})
	}
	if LayoutPending(wrapped) {
		t.Error("still pending once settled")
	}
}

// uncached measures without the cache, as a Base embedder in an app
// does: its own dirty flag never clears.
type uncached struct {
	Base
	child Widget
}

func (u *uncached) Measure(c Constraints) Size { return u.child.Measure(c) }
func (u *uncached) Arrange(r render.Rect) {
	u.ArrangeSelf(r)
	u.child.Arrange(r)
	SetParents(u, u.child)
}
func (u *uncached) Paint(cv *render.Canvas) { u.child.Paint(cv) }
func (u *uncached) HitTest(p Point) Widget  { return u.child.HitTest(p) }
func (u *uncached) Children() []Widget      { return []Widget{u.child} }

func TestInvalidationClimbsPastAnUncachedWidget(t *testing.T) {
	face := goldenFace(t)
	label := NewLabel(face, 10, "a", render.RGBA(0, 0, 0, 255))
	inner := NewBox(Column, 0, 0)
	inner.Append(label, false)
	root := NewBox(Column, 0, 0)
	root.Append(&uncached{child: inner}, false)
	con := Constraints{Max: Size{W: 400, H: 400}}
	for range 3 {
		root.Measure(con)
		root.Arrange(render.Rect{W: 400, H: 400})
	}
	before := root.Measure(con)
	label.SetText("a much longer label than before")
	if !LayoutPending(root) {
		t.Fatal("a label change below an uncached widget left the root clean")
	}
	if after := root.Measure(con); after.W <= before.W {
		t.Errorf("root still measures %v after the label grew (was %v)", after, before)
	}
}

// TestEntryHonorsItsMargin pins the field inside the stylesheet's
// margin: it measures the margin in and arranges its field (and hit
// area) inside it, as every CSS box does. A save-name field styled
// `margin: 0 12px 8px` used to run edge to edge.
func TestEntryHonorsItsMargin(t *testing.T) {
	face := testFace(t)
	plain := NewEntry(face, 13, render.RGB(255, 255, 255))
	base := plain.Measure(Constraints{Max: Size{W: 500, H: 100}})
	loadCSS(t, `.m { margin: 1px 12px 8px 4px; }`)
	e := NewEntry(face, 13, render.RGB(255, 255, 255))
	e.AddClass("m")
	if got, want := e.Measure(Constraints{Max: Size{W: 500, H: 100}}), (Size{W: base.W + 16, H: base.H + 9}); got != want {
		t.Errorf("measure = %+v, want %+v (the field plus its margin)", got, want)
	}
	e.Arrange(render.Rect{W: 300, H: 40})
	if got, want := e.Bounds(), (render.Rect{X: 4, Y: 1, W: 284, H: 31}); got != want {
		t.Errorf("field = %+v, want %+v inside the margin", got, want)
	}
	if e.HitTest(Point{X: 2, Y: 20}) != nil {
		t.Error("the margin hit the field")
	}
}

// A stylesheet styles a switch the GTK way: `switch` the track,
// `:checked` while on, `switch slider` the knob.
func TestSwitchTakesTheStylesheet(t *testing.T) {
	loadCSS(t, `switch { background-color: #ff0000; min-width: 50px; min-height: 30px; border-radius: 0; }
switch:checked { background-color: #00ff00; }
switch slider { background-color: #0000ff; min-width: 10px; min-height: 10px; margin: 5px; border-radius: 0; }
switch:checked slider { background-color: #ffffff; }
switch.hot { --knob: #ffff00; }
switch slider { background-color: var(--knob, #0000ff); }`)
	t.Cleanup(anim.SetInstant(true))
	s := NewSwitch(false)
	if sz := s.Measure(Constraints{Max: Size{W: 200, H: 200}}); sz != (Size{W: 50, H: 30}) {
		t.Fatalf("measure = %v, want the 50x30 min size", sz)
	}
	s.Arrange(render.Rect{W: 50, H: 30})
	at := func(data []byte, x, y int) render.Color {
		return render.ColorFromBytes(data[y*render.Stride(50)+x*4:])
	}
	off := paintTree(s, 50, 30)
	if got := at(off, 45, 2); got != render.RGB(0xff, 0, 0) {
		t.Errorf("off track = %#08x, want red", uint32(got))
	}
	if got := at(off, 10, 15); got != render.RGB(0, 0, 0xff) {
		t.Errorf("off knob = %#08x, want blue at the left", uint32(got))
	}
	if got := at(off, 4, 15); got != render.RGB(0xff, 0, 0) {
		t.Errorf("left of the knob = %#08x, want the track inside the 5px margin", uint32(got))
	}
	s.SetOn(true)
	if !s.HasState(StateChecked) {
		t.Fatal("an on switch is not :checked")
	}
	on := paintTree(s, 50, 30)
	if got := at(on, 10, 15); got != render.RGB(0, 0xff, 0) {
		t.Errorf("on track = %#08x, want green where the knob was", uint32(got))
	}
	if got := at(on, 40, 15); got != render.RGB(0xff, 0xff, 0xff) {
		t.Errorf("on knob = %#08x, want the checked white at the right", uint32(got))
	}
	s.SetOn(false)
	if s.HasState(StateChecked) {
		t.Error("an off switch stayed :checked")
	}
	// A value the switch passes down reaches the knob.
	s.AddClass("hot")
	if got := at(paintTree(s, 50, 30), 10, 15); got != render.RGB(0xff, 0xff, 0) {
		t.Errorf("knob = %#08x, want the inherited yellow", uint32(got))
	}
}

// A CSS transform lands at the same logical place on every device
// scale: the layer and the composite are device pixels, the transform
// logical (#224).
func TestTransformOnScaledOutput(t *testing.T) {
	loadCSS(t, `.t { min-width: 10px; min-height: 10px; background-color: #ff0000; transform: translateX(10px) rotate(0deg); }`)
	for _, num := range []int{1, 2, 3} {
		b := NewBox(Column, 0, 0)
		b.AddClass("t")
		root := NewBox(Column, 0, 0)
		root.Append(b, false)
		root.Measure(Constraints{Max: Size{W: 60, H: 30}})
		root.Arrange(render.Rect{W: 60, H: 30})
		w, h := 60*num, 30*num
		data := make([]byte, render.Stride(w)*h)
		root.Paint(render.NewScaled(data, render.Stride(w), w, h, num, 1))
		first := -1
		for x := range w {
			if render.ColorFromBytes(data[(5*num)*render.Stride(w)+x*4:]).A() > 128 {
				first = x
				break
			}
		}
		if want := 10 * num; first < want-1 || first > want+1 {
			t.Errorf("at %dx the translated box starts at device x %d, want %d", num, first, want)
		}
	}
}

// The pointer over the surface's empty space, between widgets, still
// hovers the root: the root's :hover stays on, instead of flipping off
// and back, which restyled and repainted every widget in the window on
// each crossing of a gap (#328).
func TestHoverChainKeepsTheRootOverAGap(t *testing.T) {
	loadCSS(t, `.outer { display: contents; } .outer:hover label { color: #00ff00; } label + label { color: #0000ff; }`)
	a, b := NewSpacer(40, 20), NewSpacer(40, 20)
	root := NewBox(Row, 40, 0)
	root.AddClass("outer")
	root.Append(a, false)
	root.Append(b, false)
	arrangeTree(t, root, 200, 20)
	r := &Router{Root: root}
	r.Move(Point{X: a.Bounds().X + 2, Y: 5})
	CollectDamage(root)
	gap := Point{X: a.Bounds().X + a.Bounds().W + 20, Y: 5}
	if root.HitTest(gap) != nil {
		t.Fatalf("the gap hits %T, want nothing", root.HitTest(gap))
	}
	gen := styleGen
	r.Move(gap)
	if !root.hoverChain {
		t.Error("the root left the hover chain over a gap")
	}
	r.Move(Point{X: b.Bounds().X + 2, Y: 5})
	if styleGen != gen {
		t.Errorf("crossing a gap restyled the whole tree (styleGen %d -> %d)", gen, styleGen)
	}
	if rects, _ := CollectDamage(root); len(rects) > 0 && rects[0] == root.Bounds() {
		t.Errorf("crossing a gap repainted the root's whole box: %v", rects)
	}
	r.Move(Point{X: 500, Y: 500})
	if root.hoverChain {
		t.Error("the pointer outside the root's bounds kept it hovered")
	}
	r.Move(gap)
	gen = styleGen
	r.Leave()
	if root.hoverChain {
		t.Error("leave kept the root hovered")
	}
	if styleGen != gen {
		t.Errorf("the pointer leaving restyled every tree, not the root's (styleGen %d -> %d)", gen, styleGen)
	}
}
