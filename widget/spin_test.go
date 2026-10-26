package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func testSpin(t *testing.T, min, max, step float64, digits int) (*SpinButton, *[]float64) {
	t.Helper()
	s := NewSpinButton(testFace(t), 14, render.Color(0xff000000), min, max, step, digits)
	var fired []float64
	s.OnValueChanged = func(v float64) { fired = append(fired, v) }
	return s, &fired
}

func typeInto(s *SpinButton, text string) {
	s.SelectAll()
	s.Insert(text)
}

func TestSpinButtonStepsAndClamps(t *testing.T) {
	s, fired := testSpin(t, 0, 10, 2, 0)
	if s.Value() != 0 || s.Text() != "0" {
		t.Fatalf("initial %v %q, want the minimum", s.Value(), s.Text())
	}
	s.KeyAction(KeyUp, 0)
	s.KeyAction(KeyUp, 0)
	s.KeyAction(KeyDown, 0)
	s.KeyAction(KeyPriorPage, 0)
	if s.Value() != 10 || s.Text() != "10" {
		t.Errorf("after steps %v %q, want clamped at 10", s.Value(), s.Text())
	}
	s.KeyAction(KeyUp, 0) // at the top: no change, no event
	s.KeyAction(KeyNextPage, 0)
	if want := []float64{2, 4, 2, 10, 0}; !equalFloats(*fired, want) {
		t.Errorf("fired %v, want %v", *fired, want)
	}
}

func TestSpinButtonCommitsTypedText(t *testing.T) {
	s, fired := testSpin(t, -5, 5, 0.5, 1)
	typeInto(s, " 2.26 ")
	if s.Value() != -5 || len(*fired) != 0 {
		t.Fatal("typing alone committed")
	}
	s.KeyAction(KeyEnter, 0)
	if s.Value() != 2.3 || s.Text() != "2.3" {
		t.Errorf("Enter committed %v %q, want 2.3 rounded to one digit", s.Value(), s.Text())
	}
	typeInto(s, "99")
	s.KeyAction(KeyEnter, 0)
	if s.Value() != 5 || s.Text() != "5.0" {
		t.Errorf("out of range committed %v %q, want clamped to 5.0", s.Value(), s.Text())
	}
	// Garbage reverts, without an event.
	typeInto(s, "abc")
	s.KeyAction(KeyEnter, 0)
	if s.Value() != 5 || s.Text() != "5.0" {
		t.Errorf("garbage left %v %q, want the value back", s.Value(), s.Text())
	}
	// A step starts from the typed text.
	typeInto(s, "1")
	s.KeyAction(KeyUp, 0)
	if s.Value() != 1.5 {
		t.Errorf("step from typed 1 = %v, want 1.5", s.Value())
	}
	if want := []float64{2.3, 5, 1, 1.5}; !equalFloats(*fired, want) {
		t.Errorf("fired %v, want %v", *fired, want)
	}
}

// Leaving the entry commits; SetValue never fires.
func TestSpinButtonCommitsOnBlurAndSetValueIsSilent(t *testing.T) {
	s, fired := testSpin(t, 0, 100, 1, 0)
	other := newFocusTarget()
	root := NewBox(Column, 0, 0)
	root.Append(s, false)
	root.Append(other, false)
	r := &Router{Root: root}
	r.SetFocus(s)
	typeInto(s, "42")
	r.SetFocus(other)
	if s.Value() != 42 {
		t.Errorf("blur left %v, want 42 committed", s.Value())
	}
	s.SetValue(7)
	s.SetRange(10, 20)
	if s.Value() != 10 || s.Text() != "10" {
		t.Errorf("SetRange left %v %q, want re-clamped to 10", s.Value(), s.Text())
	}
	if want := []float64{42}; !equalFloats(*fired, want) {
		t.Errorf("fired %v, want only the blur commit", *fired)
	}
	if s.Element() != "spinbutton" {
		t.Errorf("element %q, want spinbutton", s.Element())
	}
}

func TestSpinButtonDisabledDoesNotStep(t *testing.T) {
	s, fired := testSpin(t, 0, 10, 1, 0)
	s.SetEnabled(false)
	s.KeyAction(KeyUp, 0)
	if s.Value() != 0 || len(*fired) != 0 {
		t.Error("a disabled spin button stepped")
	}
}

func equalFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSpinButtonStepAndDigitsChange(t *testing.T) {
	s, fired := testSpin(t, 0, 100, 0.05, 2)
	s.SetValue(1.25)
	s.SetDigits(0)
	if s.Value() != 1 || s.Text() != "1" {
		t.Errorf("digits 0: %v %q, want 1", s.Value(), s.Text())
	}
	s.SetStep(10)
	s.KeyAction(KeyUp, 0)
	if s.Value() != 11 {
		t.Errorf("step 10 from 1 = %v, want 11", s.Value())
	}
	s.SetStep(0)
	s.KeyAction(KeyUp, 0)
	if s.Value() != 12 {
		t.Errorf("a zero step = %v, want the step of 1", s.Value())
	}
	if len(*fired) != 2 {
		t.Errorf("fired %v, want only the two steps", *fired)
	}
}

// A spin button is wide enough for its widest value beside its
// steppers, and follows its range and digits.
func TestSpinButtonFitsItsRange(t *testing.T) {
	face := goldenFace(t)
	s := NewSpinButton(face, 14, 0, 0, 23, 1, 0)
	if min, max := s.WidthChars(); min != 2 || max != 2 {
		t.Errorf("0-23 sizes for %d-%d chars, want 2", min, max)
	}
	s.SetRange(-1000, 5)
	s.SetDigits(2)
	if min, _ := s.WidthChars(); min != len("-1000.00") {
		t.Errorf("-1000 at two decimals sizes for %d chars, want %d", min, len("-1000.00"))
	}
}
