package widget

import (
	"strings"
	"testing"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/render"
)

// captureMenuDebug swaps the mnemonic-conflict sink for the test's run
// and returns the recorder.
func captureMenuDebug(t *testing.T) *strings.Builder {
	t.Helper()
	var b strings.Builder
	old := menuDebug
	menuDebug = func(msg string) { b.WriteString(msg + "\n") }
	t.Cleanup(func() { menuDebug = old })
	return &b
}

// TestMenuMnemonicResolution pins how rows earn their Alt-letter:
// explicit Mnemonic letters win in item order, conflicts drop with a
// Debug log, and unclaimed rows auto-resolve the first label letter no
// earlier row took.
func TestMenuMnemonicResolution(t *testing.T) {
	m := NewMenu(entryFace(t), 14,
		MenuItem{Label: "Save", Mnemonic: 'v'},             // explicit 'v'
		MenuItem{Label: "Save as", OnClick: func() {}},     // auto: 's' taken? no - first 's'
		MenuItem{Label: "Save a copy", OnClick: func() {}}, // auto: s,a(?) conflicts...
		MenuItem{Label: "Rename", Mnemonic: 'r', OnClick: func() {}},
		MenuItem{Label: "Reverse", Mnemonic: 'r', OnClick: func() {}}, // duplicate: dropped
		MenuSeparator(),
		MenuItem{Label: "Quit", OnClick: func() {}},
	)
	wantRows := map[rune]int{
		'v': 0,
		's': 1,
		'a': 2, // "Save a copy": s taken, a free
		'r': 3,
		'q': 6,
	}
	for letter, row := range wantRows {
		if got, ok := m.mnemonics[letter]; !ok || got != row {
			t.Errorf("mnemonic %q -> row %d (ok=%v), want row %d", letter, got, ok, row)
		}
	}
	if got := m.mnemonics['e']; got != 0 || len(m.mnemonics) != len(wantRows) {
		t.Errorf("duplicate 'r' leaked a second binding: %v", m.mnemonics)
	}

	// The dropped duplicate logs Debug, naming the letter and the row.
	logs := captureMenuDebug(t)
	NewMenu(entryFace(t), 14,
		MenuItem{Label: "Rename", Mnemonic: 'r'},
		MenuItem{Label: "Reverse", Mnemonic: 'r'},
	)
	if !strings.Contains(logs.String(), "'r'") || !strings.Contains(logs.String(), "Reverse") {
		t.Errorf("conflict log = %q, want the dropped letter and row", logs.String())
	}
}

// TestMenuMnemonicActivation pins the Alt+letter route (#63): the
// letter fires its row exactly once, case-insensitively, through the
// same activation path a click takes — submenus included — and arrow
// navigation plus Esc keep working around it.
func TestMenuMnemonicActivation(t *testing.T) {
	fired := 0
	submenu := 0
	dismissed := 0
	m := NewMenu(entryFace(t), 14,
		MenuItem{Label: "Alpha", OnClick: func() { fired++ }},
		MenuItem{Label: "Beta", Mnemonic: 'b', OnClick: func() { fired++ }},
		MenuItem{Label: "Gamma", Items: []MenuItem{{Label: "Nested", Mnemonic: 'n', OnClick: func() { fired++ }}}},
	)
	m.OnDismiss = func() { dismissed++ }
	m.OnSubmenu = func(index int, items []MenuItem) { submenu++ }

	m.Measure(Constraints{Max: Size{W: 200, H: 200}})
	m.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: m.Measure(Constraints{Max: Size{W: 400, H: 400}}).H})

	// Alt+b fires Beta (explicit), Alt+a fires Alpha (auto-resolved),
	// both case-insensitively. Each fires once and dismisses.
	if !m.RawKey(48, ModAlt, xkb.Keysym('b')) {
		t.Error("Alt+b was not consumed")
	}
	if fired != 1 {
		t.Errorf("fired = %d after Alt+b, want 1", fired)
	}
	m.RawKey(30, ModAlt, xkb.Keysym('A'))
	if fired != 2 {
		t.Errorf("fired = %d after Alt+A, want 2", fired)
	}

	// A mnemonic on a submenu row opens the submenu, exactly like
	// Right or a click would.
	if !m.RawKey(0, ModAlt, xkb.Keysym('g')) {
		t.Error("Alt+g was not consumed")
	}
	if submenu != 1 {
		t.Errorf("submenu opens = %d after Alt+g, want 1", submenu)
	}
	if fired != 2 {
		t.Errorf("the submenu mnemonic fired the row's action: %d", fired)
	}

	// Arrows and Esc are untouched by the mnemonic machinery, and an
	// unmatched letter consumes nothing.
	if m.RawKey(0, 0, xkb.Keysym('z')) {
		t.Error("a plain unmatched letter was consumed")
	}
	m.KeyAction(KeyUp, 0)
	if m.hovered != 1 {
		t.Errorf("arrow navigation broke: hovered = %d", m.hovered)
	}
	m.KeyAction(KeyDismiss, 0)
	if dismissed != 3 {
		t.Errorf("dismissals = %d, want 3 (one per activation)", dismissed)
	}

	// The nested submenu's own mnemonics activate through the same
	// walk: a child popup's menu consumes its letters.
	nested := NewMenu(entryFace(t), 14, m.items[2].Items...)
	nested.OnDismiss = func() {}
	if !nested.RawKey(0, ModAlt, xkb.Keysym('n')) {
		t.Error("the nested mnemonic was not consumed")
	}
	if fired != 3 {
		t.Errorf("fired = %d after the nested Alt+n, want 3", fired)
	}
}

// TestActivateMnemonicWalksTheTree pins the popover-root route: the
// walk finds a Menu nested in containers and fires its mnemonic, not
// the content's other widgets.
func TestActivateMnemonicWalksTheTree(t *testing.T) {
	fired := 0
	menu := NewMenu(entryFace(t), 14,
		MenuItem{Label: "Open", Mnemonic: 'o', OnClick: func() { fired++ }},
	)
	menu.OnDismiss = func() {}
	box := NewBox(Column, 4, 0)
	box.Append(NewBox(Column, 4, 0).Append(menu, false), false)

	if !ActivateMnemonic(box, xkb.Keysym('O')) {
		t.Error("the walk missed the nested menu's mnemonic")
	}
	if fired != 1 {
		t.Errorf("fired = %d, want 1", fired)
	}
	if ActivateMnemonic(box, xkb.Keysym('z')) {
		t.Error("an unmatched mnemonic reported firing")
	}
}

// A menu hides its mnemonic underlines until Alt is pressed in it, and
// the Alt-letters work either way.
func TestMenuMnemonicsShowOnAlt(t *testing.T) {
	fired := 0
	m := NewMenu(goldenFace(t), 14, MenuItem{Label: "Open", OnClick: func() { fired++ }})
	if m.showMnemonics {
		t.Fatal("a fresh menu shows its mnemonics")
	}
	m.RawKey(0, ModAlt, xkb.Keysym('o'))
	if fired != 1 {
		t.Error("a hidden mnemonic did not fire")
	}
	m.RawKey(0, 0, xkb.KeyAltL)
	if !m.showMnemonics {
		t.Error("Alt did not reveal the mnemonics")
	}
}
