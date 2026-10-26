package main

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget"
)

// The keyboard-first accessibility guarantee over the real showcase
// tree (see docs/a11y.md): Tab reaches every interactive control in
// paint order, Enter or Space activates whatever is focused, and the
// text area's Tab trap still lets ctrl+Tab and shift+Tab escape - so
// the whole showcase is usable without a pointer.

// tabStep routes one Tab press the way app.routeKey does: shift+Tab
// walks backwards, ctrl+Tab always moves focus (escaping the text
// area's trap), and a plain Tab indents inside a trapper.
func tabStep(r *widget.Router, shift, ctrl bool) {
	switch {
	case shift:
		r.FocusPrev()
	case ctrl:
		r.FocusNext()
	default:
		if f := r.Focused(); f != nil {
			if tt, ok := f.(widget.TabTrapper); ok && tt.TrapTab(false) {
				return
			}
		}
		r.FocusNext()
	}
}

// focusOn tabs forward until w is focused, escaping a Tab trap the way
// ctrl+Tab does.
func focusOn(t *testing.T, r *widget.Router, w widget.Widget) {
	t.Helper()
	for range 16 {
		if r.Focused() == w {
			return
		}
		before := r.Focused()
		tabStep(r, false, false)
		if r.Focused() == before {
			tabStep(r, false, true)
		}
	}
	t.Fatalf("tab never landed on %T", w)
}

func TestShowcaseTabTraversal(t *testing.T) {
	show := arrangedShowcase(t)
	r := &widget.Router{Root: show.root}

	// Paint order: left column top-down, then the right column.
	expect := []widget.Widget{
		show.button, show.slider, show.sw, show.check,
		show.entry, show.area,
	}

	t.Run("tab visits every interactive control in paint order", func(t *testing.T) {
		for i, want := range expect {
			tabStep(r, false, false)
			if r.Focused() != want {
				t.Fatalf("tab %d landed on %T, want %T", i+1, r.Focused(), want)
			}
		}
	})

	t.Run("the cycle is complete: every Tab stop, one lap each", func(t *testing.T) {
		var focusable []widget.A11yState
		for _, st := range widget.DescribeTree(show.root) {
			if st.TabStop {
				focusable = append(focusable, st)
			}
		}
		// The sixth-pass section below the fold adds its own focusable
		// controls after the text area (the showcase's scroll keeps the
		// tree laid out to its natural height, so they are all in the
		// cycle); the lap must cover exactly what the tree reports -
		// the six above plus the section's - with no repeats.
		if len(focusable) <= len(expect) {
			t.Fatalf("%d focusable controls in the tree, want more than the %d core ones", len(focusable), len(expect))
		}
		seen := map[widget.Widget]bool{}
		start := r.Focused()
		for range len(focusable) + 2 {
			w := r.Focused()
			if w == nil {
				t.Fatal("focus lost mid-cycle")
			}
			if seen[w] {
				t.Fatalf("%T visited twice in one lap", w)
			}
			seen[w] = true
			tabStep(r, false, true) // ctrl+Tab rides the full cycle
			if r.Focused() == start {
				break
			}
		}
		if r.Focused() != start {
			t.Errorf("after a lap focus = %T, want wrapped back to %T", r.Focused(), start)
		}
		if len(seen) != len(focusable) {
			t.Errorf("one lap visited %d controls, want every one of the %d Tab stops", len(seen), len(focusable))
		}
		for _, want := range expect {
			if !seen[want] {
				t.Errorf("%T was never focused", want)
			}
		}
	})

	t.Run("shift+tab walks backwards", func(t *testing.T) {
		focusOn(t, r, show.button)
		tabStep(r, true, false)
		if r.Focused() == widget.Widget(show.button) || r.Focused() == nil {
			t.Fatalf("shift+tab from the first control = %T, want the last", r.Focused())
		}
		last := r.Focused()
		tabStep(r, false, true)
		if r.Focused() != widget.Widget(show.button) {
			t.Errorf("ctrl+tab from the last control (%T) = %T, want wrapped to the first", last, r.Focused())
		}
	})

	t.Run("a plain tab inside the text area indents instead of moving focus", func(t *testing.T) {
		focusOn(t, r, show.area)
		before := len(show.area.Text())
		tabStep(r, false, false)
		if r.Focused() != widget.Widget(show.area) {
			t.Fatalf("plain tab moved focus to %T", r.Focused())
		}
		if len(show.area.Text()) != before+1 {
			t.Errorf("trapped tab inserted %d runes, want 1", len(show.area.Text())-before)
		}
	})
}

func TestShowcaseKeyboardActivation(t *testing.T) {
	show := arrangedShowcase(t)
	r := &widget.Router{Root: show.root}

	t.Run("enter and space click the focused button", func(t *testing.T) {
		clicks := 0
		show.button.OnClick = func() { clicks++ }
		focusOn(t, r, show.button)
		r.KeyAction(widget.KeyEnter, 0)
		r.Type(' ')
		if clicks != 2 {
			t.Errorf("button clicks = %d, want 2 (Enter + Space)", clicks)
		}
	})

	t.Run("arrows move the focused slider", func(t *testing.T) {
		focusOn(t, r, show.slider)
		before := show.slider.Value()
		r.KeyAction(widget.KeyRight, 0)
		if show.slider.Value() <= before {
			t.Errorf("slider = %v after KeyRight, want > %v", show.slider.Value(), before)
		}
	})

	t.Run("enter and space toggle the focused switch", func(t *testing.T) {
		focusOn(t, r, show.sw)
		want := !show.sw.On()
		r.KeyAction(widget.KeyEnter, 0)
		if show.sw.On() != want {
			t.Errorf("switch = %v after Enter, want %v", show.sw.On(), want)
		}
		r.Type(' ')
		if show.sw.On() != !want {
			t.Errorf("switch = %v after Space, want %v", show.sw.On(), !want)
		}
	})

	t.Run("enter and space toggle the focused checkbox", func(t *testing.T) {
		focusOn(t, r, show.check)
		want := !show.check.Checked()
		r.KeyAction(widget.KeyEnter, 0)
		if show.check.Checked() != want {
			t.Errorf("checkbox = %v after Enter, want %v", show.check.Checked(), want)
		}
		r.Type(' ')
		if show.check.Checked() != !want {
			t.Errorf("checkbox = %v after Space, want %v", show.check.Checked(), !want)
		}
	})

	t.Run("typing lands in the focused entry", func(t *testing.T) {
		focusOn(t, r, show.entry)
		before := show.entry.Text()
		r.Type('k')
		if show.entry.Text() != before+"k" {
			t.Errorf("entry = %q, want %q", show.entry.Text(), before+"k")
		}
	})

	t.Run("space types into the focused text area instead of clicking", func(t *testing.T) {
		focusOn(t, r, show.area)
		before := len(show.area.Text())
		r.Type(' ')
		if len(show.area.Text()) != before+1 {
			t.Error("space did not insert into the text area")
		}
	})
}
