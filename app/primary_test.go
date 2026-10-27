package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/clipboard"
	"github.com/stubbedev/lys-ui/widget"
)

// primaryClaim is one primary-selection claim a fakePrimarySource saw.
type primaryClaim struct {
	text   string
	serial uint32
}

// fakePrimarySource serves a preset primary payload and records
// claims, standing in for the wire-backed clipboard.
type fakePrimarySource struct {
	readText string
	readErr  error
	claims   []primaryClaim
}

func (f *fakePrimarySource) ReadPrimary() (string, error) { return f.readText, f.readErr }

func (f *fakePrimarySource) WritePrimary(text string, serial uint32) error {
	f.claims = append(f.claims, primaryClaim{text: text, serial: serial})
	return nil
}

// newPrimaryEntry builds a router with one arranged, focused entry.
// Focus comes from traversal, not a click, so tests stay clear of the
// double-click window's selection side effects.
func newPrimaryEntry(t *testing.T) (*widget.Router, *widget.Entry) {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	e := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
	r := &widget.Router{Root: e}
	r.FocusNext()
	return r, e
}

// newPrimaryInput builds a surface input routing into r, carrying the
// given primary-selection behavior.
func newPrimaryInput(r *widget.Router, p *primarySelection) *surfaceInput {
	return &surfaceInput{router: r, request: func() {}, primary: p}
}

func TestPastePrimaryReplacesSelectionAndFiresOnce(t *testing.T) {
	r, e := newPrimaryEntry(t)
	e.SetText("hello")
	e.MoveHome()
	e.MoveCursorExtending(3) // "hel" selected
	changes := 0
	e.OnChanged = func(string) { changes++ }

	p := &primarySelection{src: &fakePrimarySource{readText: "XY"}}
	if ok, _ := p.pasteAt(r); !ok {
		t.Fatal("pasteAt = false, want a paste")
	}
	if got := e.Text(); got != "XYlo" {
		t.Errorf("text = %q, want the selection replaced by the paste: XYlo", got)
	}
	if changes != 1 {
		t.Errorf("OnChanged fired %d times, want exactly once", changes)
	}
	if _, _, active := e.Selection(); active {
		t.Error("the selection survived the paste")
	}
}

func TestPastePrimaryTargetsHoveredOverFocused(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	left := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	right := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	root := widget.NewBox(widget.Row, 0, 0).Append(left, true).Append(right, true)
	widget.SizeWithin(root, widget.Size{W: 200, H: 30})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
	r := &widget.Router{Root: root}

	// Focus the right entry, then hover the left one: X11 pastes
	// where the mouse is, not where the keyboard is.
	r.Press(widget.BTNLeft, widget.Point{X: 150, Y: 15}, 0)
	r.Release(widget.BTNLeft, widget.Point{X: 150, Y: 15})
	if r.Focused() != right {
		t.Fatalf("focus = %v, want the right entry", r.Focused())
	}
	r.Move(widget.Point{X: 30, Y: 15})
	if r.Hovered() != left {
		t.Fatalf("hover = %v, want the left entry", r.Hovered())
	}

	p := &primarySelection{src: &fakePrimarySource{readText: "XY"}}
	if ok, _ := p.pasteAt(r); !ok {
		t.Fatal("pasteAt = false, want a paste")
	}
	if got := left.Text(); got != "XY" {
		t.Errorf("hovered entry text = %q, want the paste", got)
	}
	if got := right.Text(); got != "" {
		t.Errorf("focused entry text = %q, want untouched", got)
	}
}

func TestPastePrimaryFallsBackToFocused(t *testing.T) {
	r, e := newPrimaryEntry(t)
	// The pointer is nowhere (no hover): the paste lands on the
	// keyboard focus instead.
	r.Move(widget.Point{X: 500, Y: 500})
	if r.Hovered() != nil {
		t.Fatalf("hover = %v, want none outside the tree", r.Hovered())
	}

	p := &primarySelection{src: &fakePrimarySource{readText: "XY"}}
	if ok, _ := p.pasteAt(r); !ok {
		t.Fatal("pasteAt = false, want a paste into the focused entry")
	}
	if got := e.Text(); got != "XY" {
		t.Errorf("focused entry text = %q, want the paste", got)
	}
}

func TestPastePrimaryWithoutSourceOrText(t *testing.T) {
	r, _ := newPrimaryEntry(t)

	// No clipboard configured (or no primary protocol): no paste, no
	// panic.
	var disabled *primarySelection
	if ok, _ := disabled.pasteAt(r); ok {
		t.Error("a nil primary pasted")
	}
	if ok, _ := (&primarySelection{}).pasteAt(r); ok {
		t.Error("a primary without a source pasted")
	}
	// A source that reports nothing to read: no paste.
	unavailable := &primarySelection{src: &fakePrimarySource{readErr: clipboard.ErrUnavailable}}
	if ok, _ := unavailable.pasteAt(r); ok {
		t.Error("a failed read pasted")
	}
	// Neither the widget under the pointer nor the focus is an
	// editor: no paste target.
	noTarget := &primarySelection{src: &fakePrimarySource{readText: "XY"}}
	blank := &widget.Router{Root: widget.NewBox(widget.Row, 0, 0)}
	if ok, _ := noTarget.pasteAt(blank); ok {
		t.Error("a paste without a text target succeeded")
	}
}

func TestCopyOnSelectDefaultsOff(t *testing.T) {
	r, e := newPrimaryEntry(t)
	e.SetText("hello")
	e.MoveHome()
	e.MoveCursorExtending(3)

	src := &fakePrimarySource{}
	p := &primarySelection{src: src}
	p.copyAfterRelease(r, 9)
	if len(src.claims) != 0 {
		t.Errorf("claims = %v, want none: copy-on-select must default off", src.claims)
	}
	// The selection itself is unaffected by the option being off.
	if _, _, active := e.Selection(); !active {
		t.Error("the entry lost its selection")
	}
}

func TestCopyOnSelectClaimsPrimaryWithReleaseSerial(t *testing.T) {
	r, e := newPrimaryEntry(t)
	e.SetText("hello")
	e.MoveHome()
	e.MoveCursorExtending(3)

	src := &fakePrimarySource{}
	p := &primarySelection{src: src, copyOnSelect: true}
	p.copyAfterRelease(r, 9)
	if len(src.claims) != 1 {
		t.Fatalf("claims = %v, want exactly one", src.claims)
	}
	if got := src.claims[0]; got != (primaryClaim{text: "hel", serial: 9}) {
		t.Errorf("claim = %+v, want the selection text with the release serial 9", got)
	}

	// A release without a selection claims nothing: plain clicks must
	// not clobber the primary.
	src.claims = nil
	r2, e2 := newPrimaryEntry(t)
	e2.SetText("hello")
	p.copyAfterRelease(r2, 11)
	if len(src.claims) != 0 {
		t.Errorf("claims = %v, want none without a selection", src.claims)
	}
}

func TestSurfaceInputButton2Pastes(t *testing.T) {
	r, e := newPrimaryEntry(t)
	e.SetText("abc")
	e.MoveHome() // caret at the start, deterministic
	changes := 0
	e.OnChanged = func(string) { changes++ }
	in := newPrimaryInput(r, &primarySelection{src: &fakePrimarySource{readText: "Z!"}})

	in.HandlePointerButton(widget.BTNMiddle, 1, 5)
	if got := e.Text(); got != "Z!abc" {
		t.Errorf("text = %q, want the primary pasted at the caret", got)
	}
	if changes != 1 {
		t.Errorf("OnChanged fired %d times, want exactly once", changes)
	}
	// A middle press is not a widget press: focus and pressed state
	// stay put.
	if r.Pressed() != nil {
		t.Error("the middle press opened a widget press")
	}
	if r.Focused() != e {
		t.Error("the middle press moved focus")
	}
}

func TestSurfaceInputButton2WithoutPrimaryIsInert(t *testing.T) {
	r, e := newPrimaryEntry(t)
	e.SetText("abc")
	changes := 0
	e.OnChanged = func(string) { changes++ }
	in := newPrimaryInput(r, nil)

	in.HandlePointerButton(widget.BTNMiddle, 1, 5)
	if got := e.Text(); got != "abc" {
		t.Errorf("text = %q, want unchanged without a primary source", got)
	}
	if changes != 0 {
		t.Errorf("OnChanged fired %d times, want none", changes)
	}
}

func TestSurfaceInputReleaseDrivesCopyOnSelect(t *testing.T) {
	src := &fakePrimarySource{}
	newInput := func() (*surfaceInput, *widget.Entry) {
		r, e := newPrimaryEntry(t)
		return newPrimaryInput(r, &primarySelection{src: src, copyOnSelect: true}), e
	}

	// A plain click (press + release, no selection) claims nothing.
	in, e := newInput()
	e.SetText("hello")
	in.HandlePointerButton(widget.BTNLeft, 1, 5)
	in.HandlePointerButton(widget.BTNLeft, 0, 7)
	if len(src.claims) != 0 {
		t.Errorf("claims after a plain click = %v, want none", src.claims)
	}

	// Selecting text and releasing claims the primary with the
	// release serial — the X11 flow.
	e.SelectAll()
	in.HandlePointerButton(widget.BTNLeft, 1, 8)
	in.HandlePointerButton(widget.BTNLeft, 0, 9)
	if len(src.claims) != 1 {
		t.Fatalf("claims = %v, want exactly one", src.claims)
	}
	if got := src.claims[0]; got != (primaryClaim{text: "hello", serial: 9}) {
		t.Errorf("claim = %+v, want the selection with the release serial 9", got)
	}
}
