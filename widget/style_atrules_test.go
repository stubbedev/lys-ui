package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func resetMedia(t *testing.T) {
	t.Helper()
	SetMediaViewport(0, 0)
	SetMediaContrast(false)
	SetMediaResolution(96)
	t.Cleanup(func() {
		SetMediaViewport(0, 0)
		SetMediaContrast(false)
		SetMediaResolution(96)
	})
}

func TestMediaViewportRestyle(t *testing.T) {
	resetMedia(t)
	loadCSS(t, `@media (min-width: 400px) { button { background-color: #123456; } }`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	if bg := StyleBackground(btn); bg != 0 {
		t.Fatalf("applied with no viewport set: %08x", uint32(bg))
	}
	SetMediaViewport(800, 600)
	if bg := StyleBackground(btn); bg != render.RGB(0x12, 0x34, 0x56) {
		t.Fatalf("wide viewport: %08x", uint32(bg))
	}
	SetMediaViewport(200, 200)
	if bg := StyleBackground(btn); bg != 0 {
		t.Fatalf("narrow viewport: %08x", uint32(bg))
	}
	SetMediaViewport(800, 600)
	if bg := StyleBackground(btn); bg != render.RGB(0x12, 0x34, 0x56) {
		t.Fatalf("back to wide: %08x", uint32(bg))
	}
}

func TestMediaViewportRange(t *testing.T) {
	resetMedia(t)
	loadCSS(t, `@media (400px <= width <= 800px) { button { background-color: #010203; } }`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	SetMediaViewport(600, 400)
	if bg := StyleBackground(btn); bg != render.RGB(0x01, 0x02, 0x03) {
		t.Fatalf("in range: %08x", uint32(bg))
	}
	SetMediaViewport(900, 400)
	if bg := StyleBackground(btn); bg != 0 {
		t.Fatalf("out of range: %08x", uint32(bg))
	}
}

func TestMediaColorSchemeRestyle(t *testing.T) {
	resetMedia(t)
	SetTheme(DarkTheme())
	t.Cleanup(func() { SetTheme(DarkTheme()) })
	loadCSS(t, `@media (prefers-color-scheme: light) { button { background-color: #abcdef; } }`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	if bg := StyleBackground(btn); bg != 0 {
		t.Fatalf("dark theme must not apply the light rule: %08x", uint32(bg))
	}
	SetTheme(LightTheme())
	if bg := StyleBackground(btn); bg != render.RGB(0xab, 0xcd, 0xef) {
		t.Fatalf("light theme: %08x", uint32(bg))
	}
}

func TestMediaReducedMotionRestyle(t *testing.T) {
	resetMedia(t)
	th := DarkTheme()
	th.Animations = false
	SetTheme(th)
	t.Cleanup(func() { SetTheme(DarkTheme()) })
	loadCSS(t, `@media (prefers-reduced-motion: reduce) { button { background-color: #040506; } }`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	if bg := StyleBackground(btn); bg != render.RGB(0x04, 0x05, 0x06) {
		t.Fatalf("animations off reads as reduced motion: %08x", uint32(bg))
	}
	SetTheme(DarkTheme())
	if bg := StyleBackground(btn); bg != 0 {
		t.Fatalf("animations on: %08x", uint32(bg))
	}
}

func TestMediaContrastRestyle(t *testing.T) {
	resetMedia(t)
	loadCSS(t, `@media (prefers-contrast: more) { button { background-color: #070809; } }`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	if bg := StyleBackground(btn); bg != 0 {
		t.Fatalf("contrast off: %08x", uint32(bg))
	}
	SetMediaContrast(true)
	if bg := StyleBackground(btn); bg != render.RGB(0x07, 0x08, 0x09) {
		t.Fatalf("contrast on: %08x", uint32(bg))
	}
}

func TestMediaWithoutMediaRulesFree(t *testing.T) {
	resetMedia(t)
	loadCSS(t, `button { background-color: #102030; }`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	SetMediaViewport(800, 600)
	if bg := StyleBackground(btn); bg != render.RGB(0x10, 0x20, 0x30) {
		t.Fatalf("plain rule: %08x", uint32(bg))
	}
}

func TestImportantBeatsSpecificity(t *testing.T) {
	loadCSS(t, `
		button { background-color: #111111 !important; }
		#save button { background-color: #222222; }
	`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	if bg := StyleBackground(btn); bg != render.RGB(0x11, 0x11, 0x11) {
		t.Fatalf("important must win: %08x", uint32(bg))
	}
}

func TestNestingDesugars(t *testing.T) {
	loadCSS(t, `
		.card {
			background-color: #010101;
			&:hover { background-color: #020202; }
			.title { color: #030303; }
		}
	`)
	card := NewBox(Column, 4, 4)
	card.AddClass("card")
	title := NewLabel(testFace(t), 12, "t", 0)
	title.AddClass("title")
	card.Append(title, false)
	arrangeTree(t, card, 200, 200)
	if bg := StyleBackground(card); bg != render.RGB(0x01, 0x01, 0x01) {
		t.Fatalf("own declaration: %08x", uint32(bg))
	}
	setHoverChain(nil, card)
	if bg := StyleBackground(card); bg != render.RGB(0x02, 0x02, 0x02) {
		t.Fatalf("&:hover: %08x", uint32(bg))
	}
	setHoverChain(card, nil)
	if c := CascadeColor(title); c != render.RGB(0x03, 0x03, 0x03) {
		t.Fatalf("nested descendant inherits: %08x", uint32(c))
	}
}

func TestStylesheetFileImport(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := "@import \"sub/tokens.css\";\nbutton { background-color: #0a0b0c; }"
	if err := os.WriteFile(filepath.Join(dir, "sheet.css"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "tokens.css"), []byte("button { padding: 9; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := AddStylesheetFile(filepath.Join(dir, "sheet.css"), StylePriorityApplication)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	t.Cleanup(s.Remove)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	if bg := StyleBackground(btn); bg != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Fatalf("importer rule: %08x", uint32(bg))
	}
	if p := btn.style(btn).Padding; !p.Uniform(9) {
		t.Fatalf("imported padding: %+v", p)
	}
}

func TestStylesheetFileImportCycle(t *testing.T) {
	dir := t.TempDir()
	a := "@import \"b.css\";\nbutton { color: red; }"
	b := "@import \"a.css\";\nbutton { color: blue; }"
	if err := os.WriteFile(filepath.Join(dir, "a.css"), []byte(a), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.css"), []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := AddStylesheetFile(filepath.Join(dir, "a.css"), StylePriorityApplication)
	if err == nil {
		t.Fatal("a cyclic import must fail the load")
	}
	for _, want := range []string{"cycle", filepath.Join(dir, "a.css")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must mention %q", err.Error(), want)
		}
	}
}

func TestStringStylesheetImport(t *testing.T) {
	resetMedia(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "part.css"), []byte("button { background-color: #0d0e0f; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	loadCSS(t, `@import "part.css";`)
	btn := NewButton(NewSpacer(0, 0), 4, 4)
	if bg := StyleBackground(btn); bg != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Fatalf("string-sheet import resolves from the working directory: %08x", uint32(bg))
	}
}

func TestViewportUnitsRestyleOnResize(t *testing.T) {
	minWidth := func(b *Button) int { return b.style(b).Layout.MinWidth.Px() }
	for _, tc := range []struct{ name, css, inline string }{
		{name: "sheet", css: `button { min-width: 10vw; }`},
		{name: "calc", css: `button { min-width: calc(10vw + 0px); }`},
		{name: "custom property", css: `button { --w: 10vw; min-width: var(--w); }`},
		{name: "var fallback", css: `button { min-width: var(--unset, 10vw); }`},
		{name: "inline", inline: `min-width: 10vw;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetMedia(t)
			loadCSS(t, tc.css)
			btn := NewButton(NewSpacer(0, 0), 4, 4)
			btn.SetInlineStyle(tc.inline)
			SetMediaViewport(800, 600)
			if got := minWidth(btn); got != 80 {
				t.Fatalf("at 800 wide: min-width %d, want 80", got)
			}
			SetMediaViewport(400, 600)
			if got := minWidth(btn); got != 40 {
				t.Errorf("after resizing to 400: min-width %d, want 40", got)
			}

			root := NewBox(Column, 0, 0)
			inRoot := NewButton(NewSpacer(0, 0), 4, 4)
			inRoot.SetInlineStyle(tc.inline)
			root.Append(inRoot, false)
			SetStyleRoot(root)
			arrangeTree(t, root, 300, 300)
			t.Cleanup(func() { SetMediaViewportFor(root, 0, 0) })
			SetMediaViewportFor(root, 300, 300)
			if got := minWidth(inRoot); got != 30 {
				t.Fatalf("window at 300 wide: min-width %d, want 30", got)
			}
			SetMediaViewportFor(root, 500, 300)
			if got := minWidth(inRoot); got != 50 {
				t.Errorf("window resized to 500: min-width %d, want 50", got)
			}
		})
	}
}

func TestContainerUnitsResolveAgainstTheContainer(t *testing.T) {
	minWidth := func(b *Button) int { return b.style(b).Layout.MinWidth.Px() }
	for _, tc := range []struct{ name, css, inline string }{
		{name: "custom property", css: `.c { container-type: inline-size; } button { --w: 10cqw; min-width: var(--w); }`},
		{name: "inline", css: `.c { container-type: inline-size; }`, inline: `min-width: 10cqw;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetMedia(t)
			loadCSS(t, tc.css)
			c := NewBox(Column, 0, 0)
			c.AddClass("c")
			btn := NewButton(NewSpacer(0, 0), 4, 4)
			btn.SetInlineStyle(tc.inline)
			c.Append(btn, false)
			SetMediaViewport(800, 600)
			arrangeTree(t, c, 300, 300)
			if got := minWidth(btn); got != 30 {
				t.Fatalf("in a 300 wide container: min-width %d, want 30", got)
			}
			arrangeTree(t, c, 500, 300)
			if got := minWidth(btn); got != 50 {
				t.Errorf("container resized to 500: min-width %d, want 50", got)
			}
		})
	}
}
