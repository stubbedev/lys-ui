package widget

import (
	"math"
	"strconv"
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// SpinButton is a numeric entry, GtkSpinButton: the text shows the
// value at Digits decimals; the drawn up/down steppers at its end and
// the Up and Down keys step it by Step, PageUp and PageDown by ten
// steps; Enter, or focus leaving the entry, commits the typed text. A commit clamps into
// [Min, Max] and rounds to Digits; text that does not parse as a
// number reverts to the value.
//
// The value changes only through those paths and SetValue; the text
// between commits is the entry's own.
type SpinButton struct {
	*Entry
	min, max, step float64
	digits         int
	value          float64

	// OnValueChanged fires when the user changed the value: a step
	// or a commit that moved it. SetValue does not fire it, so a
	// caller mirroring an external value never echoes it back.
	OnValueChanged func(v float64)
}

// NewSpinButton returns a spin button over [min, max] stepping by step
// (a step ≤ 0 is 1), showing digits decimals, starting at min.
func NewSpinButton(face render.Font, sizePx float64, color render.Color, min, max, step float64, digits int) *SpinButton {
	if step <= 0 {
		step = 1
	}
	s := &SpinButton{Entry: NewEntry(face, sizePx, color), min: min, max: max, step: step, digits: max0(digits)}
	s.SetElement("spinbutton")
	s.value = s.clamp(min)
	s.SetText(s.format(s.value))
	s.fitRange()
	s.OnActivate = func(string) { s.commit() }
	s.setSlots(nil, nil, newSpinSteppers(sizePx), s.stepperClick)
	s.SetOnFocusChanged(func(focused bool) {
		if !focused {
			s.commit()
		}
	})
	return s
}

func max0(n int) int { return max(n, 0) }

// fitRange sizes the field to its widest value, the formatted minimum
// or maximum, as GtkSpinButton does: the value always shows beside the
// steppers.
func (s *SpinButton) fitRange() {
	n := max(len([]rune(s.format(s.min))), len([]rune(s.format(s.max))))
	s.SetWidthChars(n, n)
}

// Value reports the committed value.
func (s *SpinButton) Value() float64 { return s.value }

// Range reports the bounds.
func (s *SpinButton) Range() (min, max float64) { return s.min, s.max }

// SetValue sets the value (clamped, rounded) and shows it, discarding
// uncommitted text; it does not fire OnValueChanged.
func (s *SpinButton) SetValue(v float64) {
	s.value = s.clamp(v)
	s.SetText(s.format(s.value))
}

// SetRange changes the bounds, re-clamping the value (silently, like
// SetValue).
func (s *SpinButton) SetRange(min, max float64) {
	s.min, s.max = min, max
	s.fitRange()
	s.SetValue(s.value)
}

// KeyAction steps on Up/Down and PageUp/PageDown; every other key
// edits the text.
func (s *SpinButton) KeyAction(a KeyAction, mods Mods) {
	if !s.Enabled() || s.ReadOnly() {
		s.Entry.KeyAction(a, mods)
		return
	}
	switch a {
	case KeyUp:
		s.stepBy(1)
	case KeyDown:
		s.stepBy(-1)
	case KeyPriorPage:
		s.stepBy(10)
	case KeyNextPage:
		s.stepBy(-10)
	default:
		s.Entry.KeyAction(a, mods)
	}
}

// stepperClick steps up from a press on the steppers' upper half,
// down from the lower.
func (s *SpinButton) stepperClick(p Point) {
	if !s.Enabled() || s.ReadOnly() {
		return
	}
	r := s.trailingRect()
	if p.Y < r.Y+r.H/2 {
		s.stepBy(1)
		return
	}
	s.stepBy(-1)
}

// newSpinSteppers is the stacked up/down chevron pair, sized to the
// text's line.
func newSpinSteppers(sizePx float64) Widget {
	px := max(int(sizePx*0.7), 8)
	col := NewBox(Column, 0, 0)
	col.Append(NewSymbol(SymbolChevronUp, px), false)
	col.Append(NewSymbol(SymbolChevronDown, px), false)
	return col
}

// stepBy commits the typed text, then moves n steps from it.
func (s *SpinButton) stepBy(n int) {
	s.commit()
	s.set(s.value + float64(n)*s.step)
}

// commit takes the typed text as the value, or reverts it.
func (s *SpinButton) commit() {
	v, err := strconv.ParseFloat(strings.TrimSpace(s.Text()), 64)
	if err != nil || math.IsNaN(v) {
		if text := strings.TrimSpace(s.Text()); text != s.format(s.value) {
			ErrorBell(s)
		}
		s.SetText(s.format(s.value))
		return
	}
	s.set(v)
}

// set applies a user-driven value: shown always, fired on a change.
func (s *SpinButton) set(v float64) {
	v = s.clamp(v)
	changed := v != s.value
	s.value = v
	if text := s.format(v); text != s.Text() {
		s.SetText(text)
	}
	if changed && s.OnValueChanged != nil {
		s.OnValueChanged(v)
	}
}

// clamp bounds v and rounds it to the shown digits, so the value is
// exactly what the text says.
func (s *SpinButton) clamp(v float64) float64 {
	p := math.Pow(10, float64(s.digits))
	if r := math.Round(v*p) / p; !math.IsInf(r, 0) {
		v = r
	}
	return min(max(v, s.min), s.max)
}

func (s *SpinButton) format(v float64) string {
	return strconv.FormatFloat(v, 'f', s.digits, 64)
}

// SetStep changes how far a key steps (a step ≤ 0 is 1).
func (s *SpinButton) SetStep(step float64) {
	if step <= 0 {
		step = 1
	}
	s.step = step
}

// SetDigits changes the shown decimals, re-rounding the value
// (silently, like SetValue).
func (s *SpinButton) SetDigits(digits int) {
	s.digits = max0(digits)
	s.fitRange()
	s.SetValue(s.value)
}
