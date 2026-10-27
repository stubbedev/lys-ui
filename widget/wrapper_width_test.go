package widget

import (
	"math"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// longEllipsizing is a single-line label far wider than any test box,
// cut at the end, and the width of its ellipsis.
func longEllipsizing(t *testing.T) (*Label, int) {
	t.Helper()
	face := goldenFace(t)
	l := NewLabel(face, 14, strings.Repeat("a long description ", 30), 0)
	l.SetEllipsize(EllipsizeEnd)
	return l, int(math.Ceil(face.Shape(render.Ellipsis, 14).Advance()))
}

// A sidebar of buttons in a scroll that only scrolls vertically asks
// for its buttons' minimum, not their labels' full text, and lays out
// inside a narrow scroll with the labels ellipsized (#378).
func TestButtonGivesWayToItsEllipsizingLabel(t *testing.T) {
	label, ell := longEllipsizing(t)
	b := NewButton(label, 6, 4)
	if got := MinWidth(b); got != ell+12 {
		t.Errorf("button minimum %d, want the ellipsis %d and its padding", got, ell+12)
	}
	side := NewBox(Column, 0, 0)
	side.Append(b, false)
	s := NewScroll(side)
	s.VerticalOnly = true
	if got := s.Measure(Constraints{Max: Size{W: 2000, H: 400}}).W; got >= 200 {
		t.Errorf("the scroll asks for %d, want the sidebar's minimum", got)
	}
	row := NewBox(Row, 0, 0)
	row.Append(b, false)
	row.Append(newStub(100, 20), false)
	row.Measure(Constraints{Max: Size{W: 300, H: 40}})
	row.Arrange(render.Rect{W: 300, H: 40})
	if got := b.Bounds().W; got > 200 || got <= ell+12 {
		t.Errorf("button in a 300px row beside 100px is %d wide, want the 200 left, ellipsized", got)
	}
}

// A revealer around a long ellipsizing row counts at the row's minimum,
// revealed or collapsed, so a page holding one fits a narrow scroll
// (#378).
func TestRevealerGivesWayToItsChild(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	for _, revealed := range []bool{true, false} {
		label, ell := longEllipsizing(t)
		r := NewRevealer(label)
		r.SetTransition(RevealSlideDown)
		r.SetRevealed(revealed)
		page := NewBox(Column, 0, 0)
		page.Append(newStub(40, 20), false)
		page.Append(r, false)
		if got, want := MinWidth(page), max(40, ell); got != want {
			t.Errorf("revealed %v: page minimum %d, want %d", revealed, got, want)
		}
		s := NewScroll(page)
		s.VerticalOnly = true
		s.Measure(Constraints{Max: Size{W: 300, H: 400}})
		s.Arrange(render.Rect{W: 300, H: 400})
		if got := label.Bounds().W; revealed && got > 300 {
			t.Errorf("the revealed row is %d wide in a 300px scroll", got)
		}
	}
	// A sideways slide keeps its child's natural width.
	label, _ := longEllipsizing(t)
	r := NewRevealer(label)
	r.SetTransition(RevealSlideRight)
	r.SetCollapse(true)
	r.SetRevealed(true)
	if got, nat := MinWidth(r), r.Measure(unbounded).W; got != nat {
		t.Errorf("sideways slide minimum %d, want its natural %d", got, nat)
	}
}

// Every pass-through wrapper reports its child's minimum, plus what it
// adds around it (#378).
func TestWrappersPassTheirChildsMinimumThrough(t *testing.T) {
	for name, wrap := range map[string]func(Widget) Widget{
		"Fader":        func(w Widget) Widget { return NewFader(w) },
		"Elevation":    func(w Widget) Widget { return NewElevation(w) },
		"SwipeDismiss": func(w Widget) Widget { return NewSwipeDismiss(w) },
		"Morph":        func(w Widget) Widget { return NewMorph(w) },
		"FlowBoxChild": func(w Widget) Widget { return &FlowBoxChild{child: w} },
	} {
		label, ell := longEllipsizing(t)
		w := wrap(label)
		extra := w.Measure(unbounded).W - label.Measure(unbounded).W
		if got := MinWidth(w); got != ell+extra {
			t.Errorf("%s: minimum %d, want the ellipsis %d plus its own %d", name, got, ell, extra)
		}
		if ws, ok := w.(WidthShrinker); !ok || ws.ShrinkableWidth() == 0 {
			t.Errorf("%s gives up no width around an ellipsizing label", name)
		}
	}
}

// A wrapping label beside a button in a short row wraps narrower and
// taller so the button stays inside the row, as GTK lays them out.
func TestWrappingLabelGivesWayToASibling(t *testing.T) {
	face := goldenFace(t)
	l := NewLabel(face, 14, strings.Repeat("a notice that wraps ", 12), 0)
	l.SetWrap(true)
	btn := NewButton(newStub(60, 20), 6, 4)
	row := NewBox(Row, 8, 0)
	row.Append(l, true)
	row.Append(btn, false)
	sz := row.Measure(Constraints{Max: Size{W: 300, H: 400}})
	row.Arrange(render.Rect{W: 300, H: sz.H})
	if b := btn.Bounds(); b.X+b.W > 300 {
		t.Errorf("button ends at %d, want inside the 300px row", b.X+b.W)
	}
	lb := l.Bounds()
	if need := l.Measure(Constraints{Max: Size{W: lb.W, H: 400}}).H; lb.H < need {
		t.Errorf("label %dx%d, want the %d it needs wrapped at its width", lb.W, lb.H, need)
	}
}

// A badge in a row short of room ellipsizes, giving width down to the
// ellipsis inside its box, and keeps its whole text with room (#403).
func TestBadgeGivesWay(t *testing.T) {
	face := goldenFace(t)
	b := NewBadge(face, "configured by the user's own file")
	nat := b.Measure(Constraints{Max: Size{W: 1000, H: 100}}).W
	if min := MinWidth(b); min >= nat/2 {
		t.Errorf("badge minimum %d of its natural %d, want down to the ellipsis", min, nat)
	}
	row := NewBox(Row, 0, 0)
	title := newStub(200, 20)
	row.Append(title, true)
	row.Append(b, false)
	row.Measure(Constraints{Max: Size{W: 260, H: 40}})
	row.Arrange(render.Rect{W: 260, H: 40})
	if r := b.Bounds(); r.X+r.W > 260 || r.W >= nat {
		t.Errorf("badge %v in a 260px row beside a 200px title, want it narrowed inside the row", r)
	}
	row.Measure(Constraints{Max: Size{W: 1000, H: 40}})
	row.Arrange(render.Rect{W: 1000, H: 40})
	if got := b.Bounds().W; got != nat {
		t.Errorf("with room the badge is %d wide, want its natural %d", got, nat)
	}
}
