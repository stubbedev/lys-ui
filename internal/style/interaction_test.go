package style

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestCursor pins the cursor property: every keyword, the url() form
// with its hotspot, the fallback list, auto, and the rejects.
func TestCursor(t *testing.T) {
	s := parseOne(t, `
		a { cursor: pointer; }
		b { cursor: url("custom.cur") 2 4, pointer; }
		c { cursor: url("plain.cur"); }
		d { cursor: auto; }
		e { cursor: none; }
		f { cursor: nesw-resize; }
	`)
	cur := func(n string) CursorSpec { return computeTree(s, el(n)).Cursor }
	if c := cur("a"); c.Name != "pointer" || c.URL != "" {
		t.Errorf("pointer: %+v", c)
	}
	if c := cur("b"); c.URL != "custom.cur" || c.HotX != 2 || c.HotY != 4 {
		t.Errorf("url with hotspot: %+v", c)
	}
	if c := cur("c"); c.URL != "plain.cur" || c.HotX != 0 || c.HotY != 0 {
		t.Errorf("url without hotspot: %+v", c)
	}
	if c := cur("d"); c != (CursorSpec{}) {
		t.Errorf("auto: %+v", c)
	}
	if c := cur("e"); c.Name != "none" {
		t.Errorf("none: %+v", c)
	}
	if c := cur("f"); c.Name != "nesw-resize" {
		t.Errorf("nesw-resize: %+v", c)
	}
	for _, w := range []string{
		"pointer", "crosshair", "text", "wait", "progress",
		"help", "move", "n-resize", "s-resize", "e-resize", "w-resize",
		"ne-resize", "nw-resize", "se-resize", "sw-resize", "ns-resize",
		"ew-resize", "nesw-resize", "nwse-resize", "zoom-in", "zoom-out",
		"grab", "grabbing", "context-menu", "cell", "vertical-text",
		"alias", "copy", "no-drop", "not-allowed", "col-resize",
		"row-resize", "all-scroll", "default",
	} {
		var v Values
		if !parseCursor(tokenize(w), &ctx{}, &v) || v.Cursor.Name != w {
			t.Errorf("cursor: %s did not parse", w)
		}
	}
	for _, bad := range []string{"sparkly", "url(3)", "url(\"a.cur\") 2", "url(\"a.cur\") 2 4 6"} {
		var v Values
		if parseCursor(tokenize(bad), &ctx{}, &v) {
			t.Errorf("%q parsed", bad)
		}
	}
}

// cursor inherits down the tree, like CSS.
func TestCursorInherits(t *testing.T) {
	s := parseOne(t, `a { cursor: crosshair; }`)
	root := el("a")
	kid := root.add("b")
	if c := computeTree(s, kid).Cursor; c.Name != "crosshair" {
		t.Errorf("child cursor = %+v, want the inherited crosshair", c)
	}
}

// TestPointerEvents pins pointer-events: auto and none, and the
// rejects.
func TestPointerEvents(t *testing.T) {
	s := parseOne(t, `
		a { pointer-events: none; }
		b { pointer-events: auto; }
	`)
	if pe := computeTree(s, el("a")).PointerEvents; pe != PointerNone {
		t.Errorf("none = %d", pe)
	}
	if pe := computeTree(s, el("b")).PointerEvents; pe != PointerAuto {
		t.Errorf("auto = %d", pe)
	}
	for _, bad := range []string{"fill", "stroke", "visible"} {
		var v Values
		if parsePointerEvents(tokenize(bad), &ctx{}, &v) {
			t.Errorf("%q parsed", bad)
		}
	}
}

// TestUserSelect pins user-select's computed values: none, text, all,
// contain, and auto, which stays auto until the widget layer resolves
// its used value.
func TestUserSelect(t *testing.T) {
	s := parseOne(t, `
		a { user-select: none; }
		b { user-select: text; }
		c { user-select: all; }
		d { user-select: auto; }
		e { user-select: contain; }
	`)
	want := map[string]UserSelect{"a": SelectNone, "b": SelectText, "c": SelectAll, "d": SelectAuto, "e": SelectContain}
	for n, w := range want {
		if us := computeTree(s, el(n)).UserSelect; us != w {
			t.Errorf("%s = %d, want %d", n, us, w)
		}
	}
	var v Values
	if parseUserSelect(tokenize("element"), &ctx{}, &v) {
		t.Error("element parsed")
	}
}

// TestCaretShape pins caret-shape: auto, bar, block, underscore.
func TestCaretShape(t *testing.T) {
	s := parseOne(t, `
		a { caret-shape: bar; }
		b { caret-shape: block; }
		c { caret-shape: underscore; }
		d { caret-shape: auto; }
	`)
	want := map[string]CaretShape{"a": CaretBar, "b": CaretBlock, "c": CaretUnderscore, "d": CaretAuto}
	for n, w := range want {
		if cs := computeTree(s, el(n)).CaretShape; cs != w {
			t.Errorf("%s = %d, want %d", n, cs, w)
		}
	}
	var v Values
	if parseCaretShape(tokenize("blink"), &ctx{}, &v) {
		t.Error("blink parsed")
	}
}

// TestAccentColor pins accent-color: a color and the auto fallback.
func TestAccentColor(t *testing.T) {
	s := parseOne(t, `
		a { accent-color: #ff00ff; }
		b { accent-color: auto; }
	`)
	v := computeTree(s, el("a"))
	if v.AccentColor != render.RGB(0xff, 0, 0xff) || v.AccentAuto {
		t.Errorf("color: %+v", v)
	}
	v = computeTree(s, el("b"))
	if v.AccentColor != 0 || !v.AccentAuto {
		t.Errorf("auto: %+v", v)
	}
	var w Values
	if parseAccentColor(tokenize("left"), &ctx{}, &w) {
		t.Error("left parsed")
	}
}

// TestAppearance pins appearance: none and auto parse; none's chrome
// stripping is deferred (docs/css.md) and paints nothing here.
func TestAppearance(t *testing.T) {
	s := parseOne(t, `
		a { appearance: none; }
		b { appearance: auto; }
	`)
	if ap := computeTree(s, el("a")).Appearance; ap != AppearanceNone {
		t.Errorf("none = %d", ap)
	}
	if ap := computeTree(s, el("b")).Appearance; ap != AppearanceAuto {
		t.Errorf("auto = %d", ap)
	}
	var v Values
	if parseAppearance(tokenize("button"), &ctx{}, &v) {
		t.Error("button parsed")
	}
}

// TestTouchAction pins touch-action: the keywords, the pan
// combinations, and the rejects.
func TestTouchAction(t *testing.T) {
	s := parseOne(t, `
		a { touch-action: none; }
		b { touch-action: pan-x; }
		c { touch-action: pan-y; }
		d { touch-action: auto; }
		e { touch-action: manipulation; }
		f { touch-action: pan-x pan-y; }
		g { touch-action: pan-left; }
		h { touch-action: pan-down; }
	`)
	want := map[string]TouchAction{
		"a": TouchNone, "b": TouchPanX, "c": TouchPanY, "d": TouchAuto,
		"e": TouchAuto, "f": TouchAuto, "g": TouchPanX, "h": TouchPanY,
	}
	for n, w := range want {
		if ta := computeTree(s, el(n)).TouchAction; ta != w {
			t.Errorf("%s = %d, want %d", n, ta, w)
		}
	}
	for _, bad := range []string{"pinch-zoom", "pan-x pinch-zoom", "auto none"} {
		var v Values
		if parseTouchAction(tokenize(bad), &ctx{}, &v) {
			t.Errorf("%q parsed", bad)
		}
	}
}

// The deferred list: appearance paints nothing (parse-only), and the
// pinch half of touch-action (manipulation, pinch-zoom) has no
// gesture-layer mapping yet — the values still parse so sheets load.
func TestInteractionDeferredList(t *testing.T) {
	s := parseOne(t, `
		a { appearance: none; touch-action: manipulation; }
	`)
	v := computeTree(s, el("a"))
	if !v.Has(PropAppearance) || v.Appearance != AppearanceNone {
		t.Errorf("appearance none did not compute: %+v", v)
	}
	if !v.Has(PropTouchAction) || v.TouchAction != TouchAuto {
		t.Errorf("manipulation did not compute to the pan axes: %+v", v)
	}
}
