package widget

import (
	"math"
	"strconv"
	"strings"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/gomath"
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
	// optional lets the field hold no value; empty is that state, the
	// text cleared and the placeholder showing.
	optional, empty bool

	// OnCleared fires when the user empties an optional field.
	OnCleared func()

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
	s.empty = false
	s.value = s.clamp(v)
	s.SetText(s.format(s.value))
}

// SetRange changes the bounds, re-clamping the value silently; an empty
// field stays empty.
func (s *SpinButton) SetRange(min, max float64) {
	s.min, s.max = min, max
	s.fitRange()
	s.reclamp()
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
	s.set(s.value + float64(float64(n)*s.step))
}

// SetOptional lets the field hold no value: emptying it fires OnCleared
// and shows the placeholder, and a step from there starts at the last
// value. Turning it off fills an empty field with that value again.
func (s *SpinButton) SetOptional(on bool) {
	s.optional = on
	if !on && s.empty {
		s.SetValue(s.value)
	}
}

// Clear empties an optional field without firing OnCleared, the value it
// held kept as where a step starts; a field that is not optional ignores it.
func (s *SpinButton) Clear() {
	if !s.optional {
		return
	}
	s.empty = true
	s.SetText("")
}

// Empty reports whether an optional field holds no value.
func (s *SpinButton) Empty() bool { return s.empty }

// commit takes the typed text as the value, or reverts it.
func (s *SpinButton) commit() {
	if s.optional && strings.TrimSpace(s.Text()) == "" {
		if !s.empty {
			s.empty = true
			if s.OnCleared != nil {
				s.OnCleared()
			}
		}
		return
	}
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
	changed := v != s.value || s.empty
	s.empty = false
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
	p := gomath.Pow(10, float64(s.digits))
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
// silently; an empty field stays empty.
func (s *SpinButton) SetDigits(digits int) {
	s.digits = max0(digits)
	s.fitRange()
	s.reclamp()
}

// reclamp fits the value to new bounds or digits, silently, an empty
// field staying empty.
func (s *SpinButton) reclamp() {
	s.value = s.clamp(s.value)
	if !s.empty {
		s.SetText(s.format(s.value))
	}
}
