package app

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// TestDialogButtonRoles pins the role-aware key mapping: an explicit
// ButtonRoleDefault binds Enter wherever the button sits, an explicit
// ButtonRoleCancel binds Escape, and without roles the positional
// fallbacks (first default, last cancel) hold.
func TestDialogButtonRoles(t *testing.T) {
	plain := []DialogButton{
		{Label: "Yes", Response: "yes"},
		{Label: "No", Response: "no"},
		{Label: "Cancel", Response: "cancel"},
	}
	if got := defaultResponse(plain); got != "yes" {
		t.Errorf("positional default = %q, want the first button", got)
	}
	if got := cancelResponse(plain); got != "cancel" {
		t.Errorf("positional cancel = %q, want the last button", got)
	}

	question := []DialogButton{
		{Label: "Cancel", Response: "cancel", Role: ButtonRoleCancel},
		{Label: "Keep", Response: "keep"},
		{Label: "Discard", Response: "discard", Role: ButtonRoleDefault},
	}
	if got := defaultResponse(question); got != "discard" {
		t.Errorf("role default = %q, want discard", got)
	}
	if got := cancelResponse(question); got != "cancel" {
		t.Errorf("role cancel = %q, want cancel", got)
	}
}

// TestPaletteScopedPerApplication pins the #84 scoping: picks land in
// the confirming application's palette only, deduped and most recent
// first, capped.
func TestPaletteScopedPerApplication(t *testing.T) {
	one, two := &Application{}, &Application{}
	red, green := render.RGB(255, 0, 0), render.RGB(0, 255, 0)

	one.palette.remember(red)
	one.palette.remember(green)
	one.palette.remember(red)
	if got := one.palette.colors; len(got) != 2 || got[0] != red || got[1] != green {
		t.Errorf("palette = %v, want red then green, deduped", got)
	}
	if len(two.palette.colors) != 0 {
		t.Errorf("a pick leaked into another application: %v", two.palette.colors)
	}

	blue := render.RGB(0, 0, 255)
	for i := range paletteCap + 2 {
		one.palette.remember(render.RGB(uint8(i), uint8(i), uint8(i&0x7f)))
	}
	_ = blue
	if got := len(one.palette.colors); got != paletteCap {
		t.Errorf("palette grew to %d, want the cap %d", got, paletteCap)
	}
}
