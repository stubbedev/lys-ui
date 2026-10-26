package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// nilContractCase is one constructor's nil-face call: name is the test
// label, ctor the prefix the panic message must carry.
type nilContractCase struct {
	name string
	ctor string
	call func()
}

// nilContractCases lists every constructor that takes a font, with a
// call that passes a nil face - untyped and, the realistic trap, typed
// nils (*render.Typeface)(nil) and (*render.Chain)(nil) inside the
// interface. Each must panic at construction with a message naming the
// constructor and the face argument, never defer the failure to the
// first Shape deep in shaping.
func nilContractCases() []nilContractCase {
	return []nilContractCase{
		{"NewMenu", "widget.NewMenu", func() { NewMenu(nil, 13, MenuItem{Label: "x"}) }},
		{"NewEntry", "widget.NewEntry", func() { NewEntry(nil, 14, render.RGB(0, 0, 0)) }},
		{"NewTextArea", "widget.NewTextArea", func() { NewTextArea(nil, 14, render.RGB(0, 0, 0)) }},
		{"NewToast", "widget.NewToast", func() { NewToast(nil, "hi", 0) }},
		{"NewExpander", "widget.NewExpander", func() { NewExpander(nil, "hi", NewSpacer(0, 0)) }},
		{"NewNotebook", "widget.NewNotebook", func() { NewNotebook(nil) }},
		{"NewLabel", "widget.NewLabel", func() { NewLabel(nil, 14, "hi", render.RGB(0, 0, 0)) }},
		{"NewRichLabel", "widget.NewRichLabel", func() { NewRichLabel(nil, 14, "hi", render.RGB(0, 0, 0)) }},
		{"NewDropdown", "widget.NewDropdown", func() { NewDropdown(nil, 13, []string{"a"}, 0) }},
		{"NewDropdownOf", "widget.NewDropdownOf", func() {
			NewDropdownOf(nil, 13, []DropdownItem[string]{{Label: "a"}}, 0)
		}},
		{"typed-nil *render.Typeface into NewMenu", "widget.NewMenu", func() {
			var f *render.Typeface
			NewMenu(f, 13)
		}},
		{"typed-nil *render.Chain into NewEntry", "widget.NewEntry", func() {
			var f *render.Chain
			NewEntry(f, 14, render.RGB(0, 0, 0))
		}},
		{"typed-nil *render.Typeface into NewTextArea", "widget.NewTextArea", func() {
			var f *render.Typeface
			NewTextArea(f, 14, render.RGB(0, 0, 0))
		}},
		{"typed-nil *render.Typeface into NewLabel", "widget.NewLabel", func() {
			var f *render.Typeface
			NewLabel(f, 14, "hi", render.RGB(0, 0, 0))
		}},
	}
}

// TestConstructorsPanicOnNilFace pins the toolkit-wide nil-face
// contract: pass nil (typed or not) and the constructor panics right
// there, naming the face argument.
func TestConstructorsPanicOnNilFace(t *testing.T) {
	for _, tc := range nilContractCases() {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%s accepted a nil face", tc.name)
				}
				msg, ok := r.(string)
				if !ok {
					t.Fatalf("%s panicked with %v, want a string message", tc.name, r)
				}
				if !strings.Contains(msg, tc.ctor) {
					t.Fatalf("%s panic message %q does not name the constructor", tc.name, msg)
				}
				if !strings.Contains(msg, "face") {
					t.Fatalf("%s panic message %q does not name the face argument", tc.name, msg)
				}
			}()
			tc.call()
		})
	}
}

// TestNewChainPanicsOnNilPrimary pins the render half of the same
// contract: a chain over a nil primary panics at construction.
func TestNewChainPanicsOnNilPrimary(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("render.NewChain accepted a nil primary")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "primary") {
			t.Fatalf("panic %v does not name the primary argument", r)
		}
	}()
	render.NewChain(nil)
}

// TestConstructorsAcceptRealFaces walks the same constructors with a
// usable face: the nil contract must not reject valid input.
func TestConstructorsAcceptRealFaces(t *testing.T) {
	face := testFace(t)
	chain := render.NewChain(face)

	if m := NewMenu(face, 13, MenuItem{Label: "x"}); m == nil {
		t.Fatal("NewMenu rejected a real face")
	}
	if e := NewEntry(chain, 14, render.RGB(0, 0, 0)); e == nil {
		t.Fatal("NewEntry rejected a real chain")
	}
	if a := NewTextArea(face, 14, render.RGB(0, 0, 0)); a == nil {
		t.Fatal("NewTextArea rejected a real face")
	}
	if o := NewToast(face, "hi", 0); o == nil {
		t.Fatal("NewToast rejected a real face")
	}
	if x := NewExpander(chain, "hi", NewSpacer(0, 0)); x == nil {
		t.Fatal("NewExpander rejected a real face")
	}
	if n := NewNotebook(face); n == nil {
		t.Fatal("NewNotebook rejected a real face")
	}
	if l := NewLabel(face, 14, "hi", render.RGB(0, 0, 0)); l == nil {
		t.Fatal("NewLabel rejected a real face")
	}
	if l := NewRichLabel(face, 14, "hi", render.RGB(0, 0, 0)); l == nil {
		t.Fatal("NewRichLabel rejected a real face")
	}
	if d := NewDropdown(face, 13, []string{"a", "b"}, 0); d == nil {
		t.Fatal("NewDropdown rejected a real face")
	}
	if d := NewDropdownOf(chain, 13, []DropdownItem[string]{{Label: "a"}}, 0); d == nil {
		t.Fatal("NewDropdownOf rejected a real chain")
	}
}

// TestBoundsZeroRectBeforeArrange pins the pre-arrange behavior of
// Bounds: constructing a widget and asking for its bounds is defined
// - the zero rect - never a panic and never stale garbage.
func TestBoundsZeroRectBeforeArrange(t *testing.T) {
	face := testFace(t)
	color := render.RGB(0, 0, 0)

	widgets := map[string]Widget{
		"label":     NewLabel(face, 14, "hi", color),
		"rich":      NewRichLabel(face, 14, "hi", color),
		"entry":     NewEntry(face, 14, color),
		"textarea":  NewTextArea(face, 14, color),
		"button":    NewButton(NewLabel(face, 14, "hi", color), 10, 8),
		"box":       NewBox(Row, 4, 2),
		"grid":      NewGrid(4, 4),
		"stack":     NewStack(),
		"overlay":   NewOverlay(),
		"scroll":    NewScroll(NewLabel(face, 14, "hi", color)),
		"list":      NewList(newCountingModel(3), 20),
		"menu":      NewMenu(face, 13),
		"dropdown":  NewDropdown(face, 13, []string{"a"}, 0),
		"notebook":  NewNotebook(face),
		"expander":  NewExpander(face, "hi", NewSpacer(0, 0)),
		"toast":     NewToast(face, "hi", 0),
		"switch":    NewSwitch(true),
		"check":     NewCheckButton(true),
		"slider":    NewSlider(0, 1, 0, 0),
		"progress":  NewProgressBar(0.5),
		"icon":      NewThemeIcon("dialog-information", 16),
		"image":     NewBytesImage(nil, "x"),
		"spinner":   NewSpinner(24),
		"separator": NewSeparator(Horizontal),
		"fader":     NewFader(NewSpacer(1, 1)),
		"elevation": NewElevation(NewSpacer(1, 1)),
		"spacer":    NewSpacer(3, 3),
	}
	for name, w := range widgets {
		if got := w.(interface{ Bounds() render.Rect }).Bounds(); !got.Empty() {
			t.Errorf("%s.Bounds() before Arrange = %v, want the zero rect", name, got)
		}
	}
}

// TestGetterSetterPairs pins the pairing convention: every Set* whose
// state is app-meaningful has a bare-name getter, and setting round-
// trips through it.
func TestGetterSetterPairs(t *testing.T) {
	face := testFace(t)
	color := render.RGB(9, 9, 9)

	l := NewLabel(face, 14, "x", color)
	l.SetAlignment(render.AlignCenter)
	if l.Alignment() != render.AlignCenter {
		t.Errorf("label alignment = %v, want center", l.Alignment())
	}

	r := NewRichLabel(face, 14, "x", color)
	r.SetAlignment(render.AlignEnd)
	if r.Alignment() != render.AlignEnd {
		t.Errorf("rich label alignment = %v, want end", r.Alignment())
	}

	e := NewEntry(face, 14, color)
	e.SetPlaceholder("who?")
	if e.Placeholder() != "who?" {
		t.Errorf("entry placeholder = %q, want who?", e.Placeholder())
	}

	a := NewTextArea(face, 14, color)
	a.SetPlaceholder("note")
	if a.Placeholder() != "note" {
		t.Errorf("area placeholder = %q, want note", a.Placeholder())
	}
	a.SetIndent(4)
	if a.Indent() != 4 {
		t.Errorf("area indent = %d, want 4", a.Indent())
	}
	a.SetWrap(false)
	if a.Wrap() {
		t.Error("area wrap = true, want false")
	}

	ic := NewThemeIcon("dialog-information", 16)
	ic.SetTint(render.RGB(1, 2, 3))
	if ic.Tint() != render.RGB(1, 2, 3) {
		t.Errorf("icon tint = %v, want RGB(1,2,3)", ic.Tint())
	}

	im := NewImage(nil)
	im.SetPlaceholderColor(color)
	if im.PlaceholderColor() != color {
		t.Errorf("image placeholder color = %v, want %v", im.PlaceholderColor(), color)
	}

	g := NewGrid(3, 5)
	if g.ColumnSpacing() != 3 || g.RowSpacing() != 5 {
		t.Errorf("grid spacing = %d/%d, want 3/5", g.ColumnSpacing(), g.RowSpacing())
	}
	g.SetColumnSpacing(7).SetRowSpacing(11).SetColumnHomogeneous(true).SetRowHomogeneous(true)
	if g.ColumnSpacing() != 7 || g.RowSpacing() != 11 {
		t.Errorf("grid spacing after set = %d/%d, want 7/11", g.ColumnSpacing(), g.RowSpacing())
	}
	if !g.ColumnHomogeneous() || !g.RowHomogeneous() {
		t.Error("grid homogeneity = false, want true")
	}

	s := NewSwitch(false)
	s.SetOn(true)
	if !s.On() {
		t.Error("switch on = false after SetOn(true)")
	}

	c := NewCheckButton(false)
	c.SetChecked(true)
	if !c.Checked() {
		t.Error("check checked = false after SetChecked(true)")
	}

	sl := NewSlider(0, 10, 1, 0)
	sl.SetValue(4)
	if sl.Value() != 4 {
		t.Errorf("slider value = %v, want 4", sl.Value())
	}

	to := NewToast(face, "x", 0)
	to.SetText("y")
	if to.Text() != "y" {
		t.Errorf("toast text = %q, want y", to.Text())
	}

	sp := NewSpinner(16)
	sp.SetSpinning(true)
	sp.SetSpinning(false) // stop cancels the tween; nothing may linger
	if sp.Spinning() {
		t.Error("spinner spinning = true after SetSpinning(false)")
	}
}
