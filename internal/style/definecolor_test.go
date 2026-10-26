package style

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestDefineColor pins GTK named colors on the custom-property
// machinery: @define-color declares, @name references resolve (inside
// functions and other defines too), a later define wins by source
// order, a subtree's --name overrides it, and an undefined reference
// is invalid at computed-value time rather than an invented color.
func TestDefineColor(t *testing.T) {
	s := parseOne(t, `
		@define-color brand #112233;
		@define-color brand #445566;
		@define-color ring mix(@brand, #ffffff, 0.5);
		button { color: @brand; background-color: @ring; }
		.scoped { --brand: #ff0000; }
		label { background-color: @missing; }
	`)
	btn := computeTree(s, el("window").add("button"))
	if btn.Color != render.RGB(0x44, 0x55, 0x66) {
		t.Errorf("button color = %08x, want the later define", uint32(btn.Color))
	}
	if btn.Background == 0 || btn.Background == btn.Color {
		t.Errorf("a define referencing another define did not resolve: %08x", uint32(btn.Background))
	}
	scoped := computeTree(s, el("window").add("box", "scoped").add("button"))
	if scoped.Color != render.RGB(0xff, 0, 0) {
		t.Errorf("scoped button color = %08x, want the subtree override", uint32(scoped.Color))
	}
	if lbl := computeTree(s, el("window").add("label")); lbl.Background != 0 {
		t.Errorf("an undefined @name produced %08x", uint32(lbl.Background))
	}
}

// TestDefineColorMalformed pins the skip policy: a define without a
// value, or with a block, warns and parses on.
func TestDefineColorMalformed(t *testing.T) {
	warns := capture(t)
	s := Parse(`@define-color lonely; @define-color blocky { color: red; } label { color: #010203; }`)
	if len(*warns) != 2 {
		t.Errorf("warnings = %v, want two", *warns)
	}
	if v := computeTree(s, el("window").add("label")); v.Color != render.RGB(1, 2, 3) {
		t.Errorf("parsing did not continue past the malformed defines: %08x", uint32(v.Color))
	}
}

// TestDefineColorLayers pins the global, layered semantics: a higher
// priority layer's define wins over a lower one, and a define cycle is
// invalid like a var() cycle instead of recursing.
func TestDefineColorLayers(t *testing.T) {
	theme := parseOne(t, `@define-color fg #010101; label { color: @fg; } .loop { color: @a; }
		@define-color a @b; @define-color b @a;`)
	app := parseOne(t, `@define-color fg #020202;`)
	layers := []Layer{{Sheet: app, Priority: PriorityApplication}, {Sheet: theme, Priority: PriorityTheme}}
	if v := computeLayers(layers, el("window").add("label")); v.Color != render.RGB(2, 2, 2) {
		t.Errorf("color = %08x, want the application layer's define", uint32(v.Color))
	}
	if v := computeLayers(layers[1:], el("window").add("label")); v.Color != render.RGB(1, 1, 1) {
		t.Errorf("color = %08x, want the theme define alone", uint32(v.Color))
	}
	if v := computeLayers(layers, el("window").add("label", "loop")); v.Color == render.RGB(1, 1, 1) {
		t.Error("a define cycle resolved instead of invalidating")
	}
}
