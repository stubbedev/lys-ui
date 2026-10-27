package widget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

func TestPerWindowMediaViewport(t *testing.T) {
	resetMedia(t)
	loadCSS(t, `@media (max-width: 500px) { button { background-color: #123456; } }`)
	small := NewBox(Column, 0, 0)
	smallBtn := NewButton(NewSpacer(0, 0), 4, 4)
	small.Append(smallBtn, false)
	large := NewBox(Column, 0, 0)
	largeBtn := NewButton(NewSpacer(0, 0), 4, 4)
	large.Append(largeBtn, false)
	SetStyleRoot(small)
	SetStyleRoot(large)
	arrangeTree(t, small, 300, 300)
	arrangeTree(t, large, 800, 600)
	t.Cleanup(func() {
		SetMediaViewportFor(small, 0, 0)
		SetMediaViewportFor(large, 0, 0)
	})
	SetMediaViewportFor(small, 300, 300)
	SetMediaViewportFor(large, 800, 600)
	if bg := StyleBackground(smallBtn); bg != render.RGB(0x12, 0x34, 0x56) {
		t.Errorf("the small window's tree matches its own viewport: %08x", uint32(bg))
	}
	if bg := StyleBackground(largeBtn); bg != 0 {
		t.Errorf("the large window's tree is unaffected: %08x", uint32(bg))
	}
	// Resizing one window restyles only trees that read the change.
	CollectDamage(small)
	SetMediaViewportFor(large, 400, 400)
	if rects, _ := CollectDamage(small); len(rects) != 0 {
		t.Errorf("the small window owes %v after the large one resized", rects)
	}
	if bg := StyleBackground(largeBtn); bg != render.RGB(0x12, 0x34, 0x56) {
		t.Errorf("after the resize the large window matches: %08x", uint32(bg))
	}
	if bg := StyleBackground(smallBtn); bg != render.RGB(0x12, 0x34, 0x56) {
		t.Errorf("the small window keeps its match: %08x", uint32(bg))
	}
}

func TestMediaViewportOfFollowsTheStyleRoot(t *testing.T) {
	resetMedia(t)
	root := NewBox(Column, 0, 0)
	leaf := NewSpacer(0, 0)
	root.Append(leaf, false)
	SetStyleRoot(root)
	arrangeTree(t, root, 100, 100)
	SetMediaViewport(640, 480)
	SetMediaViewportFor(root, 320, 200)
	if w, h := MediaViewportOf(leaf); w != 320 || h != 200 {
		t.Errorf("a widget reads its window's viewport, got %dx%d", w, h)
	}
	SetMediaViewportFor(root, 0, 0)
	if w, h := MediaViewportOf(leaf); w != 640 || h != 480 {
		t.Errorf("a cleared window falls back to the global viewport, got %dx%d", w, h)
	}
}

func TestContainerQueryResize(t *testing.T) {
	resetMedia(t)
	face := goldenFace(t)
	loadCSS(t, `
		.card { container-type: inline-size; }
		@container (min-width: 200px) { label { color: #010203; } }
	`)
	root := NewBox(Column, 0, 0)
	root.AddClass("card")
	lbl := NewLabel(face, 14, "in the card", 0)
	root.Append(lbl, false)
	SetStyleRoot(root)
	t.Cleanup(func() { SetMediaViewportFor(root, 0, 0) })

	setSize := func(w, h int) {
		SetMediaViewportFor(root, w, h)
		SizeWithin(root, Size{W: w, H: h})
		root.Arrange(render.Rect{W: w, H: h})
	}
	setSize(400, 300)
	if c := CascadeColor(lbl); c != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("wide container applies: %08x", uint32(c))
	}
	setSize(100, 300)
	if c := CascadeColor(lbl); c == render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("narrow container drops the rule: %08x", uint32(c))
	}
	setSize(400, 300)
	if c := CascadeColor(lbl); c != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("re-widening re-applies: %08x", uint32(c))
	}
}

func TestContainerUnitResolutionWidget(t *testing.T) {
	resetMedia(t)
	face := goldenFace(t)
	loadCSS(t, `
		.card { container-type: size; }
		@container (min-width: 100px) { label { padding: 5cqw 5cqh; } }
	`)
	root := NewBox(Column, 0, 0)
	root.AddClass("card")
	lbl := NewLabel(face, 14, "unit", 0)
	root.Append(lbl, false)
	SetStyleRoot(root)
	t.Cleanup(func() { SetMediaViewportFor(root, 0, 0) })
	SetMediaViewportFor(root, 200, 100)
	SizeWithin(root, Size{W: 200, H: 100})
	root.Arrange(render.Rect{W: 200, H: 100})
	v := nodeOf(lbl).style(lbl)
	if v.Padding != (style.Sides{Top: 10, Right: 5, Bottom: 10, Left: 5}) {
		t.Errorf("container units resolve against the arranged container: %v", v.Padding)
	}
}

func TestFirstLetterPseudo(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `label::first-letter { color: #040506; font-size: 20; }`)
	lbl := NewLabel(face, 14, "Lead", 0)
	fl := PseudoStyle(lbl, "first-letter")
	if fl == nil || !fl.Has(style.PropColor) || fl.Color != render.RGB(0x04, 0x05, 0x06) || fl.FontSize != 20 {
		t.Fatalf("::first-letter cascades: %+v", fl)
	}
	if v := PseudoStyle(lbl, "before"); v == nil || v.Has(style.PropColor) {
		t.Errorf("the other pseudo slots stay apart: %+v", v)
	}
}

func TestContentURLPseudo(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `.tag::before { content: url("img.png"); }`)
	lbl := NewLabel(face, 14, "tagged", 0)
	lbl.AddClass("tag")
	pv := PseudoStyle(lbl, "before")
	if pv == nil || pv.ContentURL != "img.png" {
		t.Fatalf("content url() lands on the pseudo style: %+v", pv)
	}
}

func TestFontFaceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile("testdata/Cantarell-Regular.ttf")
	if err != nil {
		t.Fatalf("fixture font: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.ttf"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	css := "@font-face { font-family: App Font; src: url(app.ttf) format(\"truetype\"); }\n"
	path := filepath.Join(dir, "sheet.css")
	if err := os.WriteFile(path, []byte(css), 0o644); err != nil {
		t.Fatalf("write sheet: %v", err)
	}
	s, err := AddStylesheetFile(path, StylePriorityApplication)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	t.Cleanup(s.Remove)
	if face, ok := resolveFace("App Font", 400, nil); !ok || face == nil {
		t.Fatal("the @font-face family resolves after load")
	}
	if _, ok := resolveFace("No Such Family", 400, nil); ok {
		t.Error("an unknown family still declines")
	}

	bad := filepath.Join(dir, "bad.css")
	if err := os.WriteFile(bad, []byte("@font-face { font-family: Gone; src: url(missing.ttf); }"), 0o644); err != nil {
		t.Fatalf("write bad sheet: %v", err)
	}
	_, err = AddStylesheetFile(bad, StylePriorityApplication)
	if err == nil {
		t.Fatal("an unreadable src fails the file load with its path")
	}
}
