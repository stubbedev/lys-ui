package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// newAccelFixtureFace returns a real face for menu construction in the
// app tests (the fixture's Entry keeps its face private).
func newAccelFixtureFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestPopoverRawKeyAgreesWithRouteKey pins the accelerator ownership
// rule (#63): while a popover menu is open, its key root fires the
// application's accelerators from the very table the main routeKey
// path uses — each combo exactly once, whichever path sees it — and
// Alt-letters reach the menu's mnemonics first.
func TestPopoverRawKeyAgreesWithRouteKey(t *testing.T) {
	f := newAccelFixture(t)
	fired := 0
	actCount := widget.NewAction("count", func() { fired++ })
	if err := f.app.AddAccel("ctrl+q", actCount); err != nil {
		t.Fatal(err)
	}

	menuFired := 0
	menu := widget.NewMenu(newAccelFixtureFace(t), 14,
		widget.MenuItem{Label: "Quit all", Mnemonic: 'q', OnClick: func() { menuFired++ }},
	)
	menu.OnDismiss = func() {}
	root := &popoverKeyRoot{
		onDismiss: func() {},
		content:   menu,
		fireAccel: func(sym xkb.Keysym, mods wlsession.Mods) bool {
			return f.app.accels.fire(nil, sym, mods)
		},
	}

	// The main path: routeKey fires the accelerator once (the entry
	// holds focus and claims nothing).
	f.tr.syms[16] = xkb.Keysym('q')
	f.press(16, wlsession.ModCtrl)
	if fired != 1 {
		t.Fatalf("routeKey fired the accelerator %d times, want 1", fired)
	}

	// The popover path: the same combo fires exactly the same action,
	// still once.
	codeQ := uint32(24)
	if !root.RawKey(codeQ, widget.ModCtrl, xkb.Keysym('q')) {
		t.Error("ctrl+q was not consumed by the popover root")
	}
	if fired != 2 {
		t.Errorf("fired = %d after both paths, want 2 (one per path)", fired)
	}

	// Alt+q is the menu's first: the mnemonic fires and the
	// accelerator never sees it, even though 'q' names both.
	if !root.RawKey(codeQ, widget.ModAlt, xkb.Keysym('q')) {
		t.Error("Alt+q was not consumed")
	}
	if menuFired != 1 {
		t.Errorf("mnemonic fired %d times, want 1", menuFired)
	}
	if fired != 2 {
		t.Errorf("the mnemonic leaked into the accelerator path: %d", fired)
	}

	// An unbound combo consumes nothing.
	if root.RawKey(30, widget.ModCtrl, xkb.Keysym('a')) {
		t.Error("an unbound ctrl combo reported firing")
	}
}

// TestPopoverRawKeyMnemonicBeforeAccel pins the deterministic order on
// a conflict: an accelerator bound to an Alt+letter that is also a
// live mnemonic belongs to the open menu.
func TestPopoverRawKeyMnemonicBeforeAccel(t *testing.T) {
	f := newAccelFixture(t)
	accels := 0
	actAltthing := widget.NewAction("altthing", func() { accels++ })
	if err := f.app.AddAccel("alt+o", actAltthing); err != nil {
		t.Fatal(err)
	}
	menuFired := 0
	menu := widget.NewMenu(newAccelFixtureFace(t), 14,
		widget.MenuItem{Label: "Open", Mnemonic: 'o', OnClick: func() { menuFired++ }},
	)
	menu.OnDismiss = func() {}
	root := &popoverKeyRoot{
		onDismiss: func() {},
		content:   menu,
		fireAccel: func(sym xkb.Keysym, mods wlsession.Mods) bool {
			return f.app.accels.fire(nil, sym, mods)
		},
	}
	if !root.RawKey(32, widget.ModAlt, xkb.Keysym('o')) {
		t.Error("Alt+o was not consumed")
	}
	if menuFired != 1 || accels != 0 {
		t.Errorf("mnemonic = %d, accelerator = %d; the open menu owns its Alt-letters", menuFired, accels)
	}
}
