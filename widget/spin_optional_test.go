package widget

import "testing"

func TestAnOptionalSpinButtonHoldsNoValueUntilOneIsGiven(t *testing.T) {
	s := NewSpinButton(testFace(t), 12, 0, 0, 100, 5, 0)
	s.SetOptional("Inherited")
	s.SetValue(20)
	s.Clear()
	if !s.Empty() || s.Text() != "" {
		t.Fatalf("cleared: empty %v, text %q", s.Empty(), s.Text())
	}
	var got []float64
	cleared := 0
	s.OnValueChanged = func(v float64) { got = append(got, v) }
	s.OnCleared = func() { cleared++ }
	s.SetDigits(0)
	s.SetRange(0, 50)
	if !s.Empty() {
		t.Fatal("new digits or bounds filled an empty field")
	}
	s.KeyAction(KeyUp, 0)
	if s.Empty() || s.Value() != 25 || len(got) != 1 || got[0] != 25 || cleared != 0 {
		t.Fatalf("a step from empty: empty %v value %v fired %v cleared %d; want 25 from the kept 20", s.Empty(), s.Value(), got, cleared)
	}
	s.SelectAll()
	s.Backspace()
	s.KeyAction(KeyEnter, 0)
	if !s.Empty() || cleared != 1 || len(got) != 1 {
		t.Fatalf("emptying: empty %v cleared %d fired %v", s.Empty(), cleared, got)
	}
	s.KeyAction(KeyEnter, 0)
	if cleared != 1 {
		t.Error("committing an empty field again fired OnCleared again")
	}
	s.SetOptional("")
	if s.Empty() || s.Text() != "25" {
		t.Errorf("no longer optional: empty %v text %q", s.Empty(), s.Text())
	}
}

func TestASpinButtonThatIsNotOptionalRevertsAnEmptyCommit(t *testing.T) {
	s := NewSpinButton(testFace(t), 12, 0, 0, 100, 1, 0)
	s.SetValue(7)
	s.Clear()
	if s.Empty() || s.Text() != "7" {
		t.Fatalf("Clear on a plain spin: empty %v text %q", s.Empty(), s.Text())
	}
	s.SelectAll()
	s.Backspace()
	s.KeyAction(KeyEnter, 0)
	if s.Empty() || s.Value() != 7 || s.Text() != "7" {
		t.Errorf("an empty commit: empty %v value %v text %q", s.Empty(), s.Value(), s.Text())
	}
}

func TestAnOptionalSpinButtonFitsItsPlaceholder(t *testing.T) {
	s := NewSpinButton(testFace(t), 12, 0, 0, 9, 1, 0)
	before := s.widthChars
	s.SetOptional("Inherited")
	if s.widthChars != len("Inherited") || before >= s.widthChars {
		t.Errorf("width %d chars after %d, want the placeholder's %d", s.widthChars, before, len("Inherited"))
	}
	s.SetDigits(0)
	if s.widthChars != len("Inherited") {
		t.Errorf("new digits narrowed it to %d chars", s.widthChars)
	}
}
