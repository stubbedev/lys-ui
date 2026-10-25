package widget

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/render"
)

func entryFace(tb testing.TB) *render.Typeface {
	tb.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		tb.Fatal(err)
	}
	return face
}

func TestEntryEditing(t *testing.T) {
	newEntry := func() *Entry {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("abc")
		return e
	}

	t.Run("insert puts text at the cursor and advances it", func(t *testing.T) {
		e := newEntry()
		e.MoveHome()
		e.Insert("X")
		if e.Text() != "Xabc" {
			t.Errorf("text = %q, want Xabc", e.Text())
		}
		if e.Cursor() != 1 {
			t.Errorf("cursor = %d, want 1", e.Cursor())
		}
		e.MoveCursor(3)
		e.Insert("Z")
		if e.Text() != "XabcZ" {
			t.Errorf("text = %q, want XabcZ", e.Text())
		}
	})

	t.Run("backspace removes the rune before the cursor", func(t *testing.T) {
		e := newEntry()
		e.MoveCursor(-1)
		e.Backspace()
		if e.Text() != "ac" {
			t.Errorf("text = %q, want ac", e.Text())
		}
		if e.Cursor() != 1 {
			t.Errorf("cursor = %d, want 1", e.Cursor())
		}
	})

	t.Run("backspace at the start is a no-op", func(t *testing.T) {
		e := newEntry()
		e.MoveHome()
		e.Backspace()
		if e.Text() != "abc" || e.Cursor() != 0 {
			t.Errorf("text = %q cursor = %d, want abc/0", e.Text(), e.Cursor())
		}
	})

	t.Run("delete removes the rune at the cursor", func(t *testing.T) {
		e := newEntry()
		e.MoveHome()
		e.Delete()
		if e.Text() != "bc" || e.Cursor() != 0 {
			t.Errorf("text = %q cursor = %d, want bc/0", e.Text(), e.Cursor())
		}
	})

	t.Run("delete at the end is a no-op", func(t *testing.T) {
		e := newEntry()
		e.MoveEnd()
		e.Delete()
		if e.Text() != "abc" || e.Cursor() != 3 {
			t.Errorf("text = %q cursor = %d, want abc/3", e.Text(), e.Cursor())
		}
	})

	t.Run("cursor motion clamps at both ends", func(t *testing.T) {
		e := newEntry()
		e.MoveCursor(-5)
		if e.Cursor() != 0 {
			t.Errorf("cursor = %d, want 0", e.Cursor())
		}
		e.MoveCursor(99)
		if e.Cursor() != 3 {
			t.Errorf("cursor = %d, want 3", e.Cursor())
		}
	})

	t.Run("home and end", func(t *testing.T) {
		e := newEntry()
		e.MoveEnd()
		if e.Cursor() != 3 {
			t.Errorf("end cursor = %d", e.Cursor())
		}
		e.MoveHome()
		if e.Cursor() != 0 {
			t.Errorf("home cursor = %d", e.Cursor())
		}
	})

	t.Run("insert works on an empty entry and with multibyte runes", func(t *testing.T) {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.Insert("héllo")
		if e.Text() != "héllo" {
			t.Errorf("text = %q", e.Text())
		}
		if e.Cursor() != 5 {
			t.Errorf("cursor = %d, want 5 (rune count)", e.Cursor())
		}
	})
}

func TestEntryMeasurePaint(t *testing.T) {
	face := entryFace(t)

	t.Run("placeholder gives an empty entry width", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.SetPlaceholder("type here")
		got := e.Measure(Constraints{Max: Size{W: 500, H: 100}})
		if got.W <= 0 {
			t.Errorf("placeholder entry width = %d, want > 0", got.W)
		}
	})

	t.Run("empty entry without placeholder wants only padding", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		got := e.Measure(Constraints{Max: Size{W: 500, H: 100}})
		if want := entryPad.Left + entryPad.Right; got.W != want {
			t.Errorf("empty entry width = %d, want the padding %d", got.W, want)
		}
	})

	t.Run("painting an empty entry shows no text ink beyond the cursor", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.Measure(Constraints{Max: Size{W: 500, H: 100}})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 28})
		cv := render.New(make([]byte, render.Stride(200)*28), render.Stride(200), 200, 28)
		e.Paint(cv)
	})
}

func TestSliderValueModel(t *testing.T) {
	t.Run("constructor clamps the initial value", func(t *testing.T) {
		if got := NewSlider(0, 10, 0, 99).Value(); got != 10 {
			t.Errorf("value = %v, want 10", got)
		}
		if got := NewSlider(0, 10, 0, -3).Value(); got != 0 {
			t.Errorf("value = %v, want 0", got)
		}
	})

	t.Run("SetValue clamps", func(t *testing.T) {
		s := NewSlider(0, 10, 0, 5)
		s.SetValue(42)
		if s.Value() != 10 {
			t.Errorf("value = %v, want 10", s.Value())
		}
	})

	t.Run("step snaps", func(t *testing.T) {
		s := NewSlider(0, 10, 2.5, 0)
		s.SetValue(3.2)
		if s.Value() != 2.5 {
			t.Errorf("value = %v, want 2.5", s.Value())
		}
	})

	t.Run("OnChanged fires only when the value moved", func(t *testing.T) {
		fired := 0
		s := NewSlider(0, 10, 0, 5)
		s.OnChanged = func(float64) { fired++ }
		s.SetValue(6)
		s.SetValue(6)
		if fired != 1 {
			t.Errorf("fired %d times, want 1", fired)
		}
	})

	t.Run("ValueAt maps the trough to values with edge padding", func(t *testing.T) {
		s := NewSlider(0, 100, 0, 0)
		s.Measure(Constraints{Max: Size{W: 500, H: 100}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 204, H: 18})
		if got := s.ValueAt(Point{X: 4}); got != 0 {
			t.Errorf("left edge = %v, want 0", got)
		}
		if got := s.ValueAt(Point{X: 200}); got != 100 {
			t.Errorf("right edge = %v, want 100", got)
		}
		if got := s.ValueAt(Point{X: 102}); got < 49 || got > 51 {
			t.Errorf("middle = %v, want ~50", got)
		}
		if got := s.ValueAt(Point{X: -100}); got != 0 {
			t.Errorf("far left = %v, want clamped 0", got)
		}
	})

	t.Run("degenerate min==max never divides by zero", func(t *testing.T) {
		s := NewSlider(5, 5, 0, 5)
		if got := s.ValueAt(Point{X: 100}); got != 5 {
			t.Errorf("value = %v, want 5", got)
		}
	})
}

func TestSwitchAndCheck(t *testing.T) {
	t.Run("SetOn fires only on flips", func(t *testing.T) {
		fired := 0
		s := NewSwitch(false)
		s.OnChanged = func(bool) { fired++ }
		s.SetOn(true)
		s.SetOn(true)
		if fired != 1 || !s.On() {
			t.Errorf("fired = %d, on = %v", fired, s.On())
		}
	})

	t.Run("Toggle flips", func(t *testing.T) {
		s := NewSwitch(false)
		s.Toggle()
		if !s.On() {
			t.Error("toggle did not turn on")
		}
		s.Toggle()
		if s.On() {
			t.Error("toggle did not turn off")
		}
	})

	t.Run("check button toggles and fires on flips only", func(t *testing.T) {
		fired := 0
		c := NewCheckButton(false)
		c.OnChanged = func(bool) { fired++ }
		c.Toggle()
		c.SetChecked(true)
		if !c.Checked() || fired != 1 {
			t.Errorf("checked = %v, fired = %d", c.Checked(), fired)
		}
	})

	t.Run("paint distinguishes on and off", func(t *testing.T) {
		paint := func(on bool) render.Color {
			data := make([]byte, render.Stride(40)*22)
			cv := render.New(data, render.Stride(40), 40, 22)
			s := NewSwitch(on)
			s.Measure(Constraints{Max: Size{W: 100, H: 100}})
			s.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 22})
			s.Paint(cv)
			return render.ColorFromBytes(data[11*render.Stride(40)+3*4:])
		}
		off := paint(false)
		on := paint(true)
		if on == off {
			t.Error("on and off tracks paint identically")
		}
	})
}

func TestProgressBar(t *testing.T) {
	t.Run("value clamps to [0, 1]", func(t *testing.T) {
		p := NewProgressBar(5)
		if p.Value() != 1 {
			t.Errorf("value = %v, want 1", p.Value())
		}
		p.SetValue(-1)
		if p.Value() != 0 {
			t.Errorf("value = %v, want 0", p.Value())
		}
		p.SetValue(0.5)
		if p.Value() != 0.5 {
			t.Errorf("value = %v, want 0.5", p.Value())
		}
	})
}

func TestStack(t *testing.T) {
	newStack := func() *Stack {
		return NewStack().
			Add("a", newStub(10, 10)).
			Add("b", newStub(30, 5))
	}

	t.Run("first added child is visible", func(t *testing.T) {
		if got := newStack().Visible(); got != "a" {
			t.Errorf("visible = %q, want a", got)
		}
	})

	t.Run("measure takes the largest child", func(t *testing.T) {
		got := newStack().Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got != (Size{W: 30, H: 10}) {
			t.Errorf("stack measure = %v, want 30x10", got)
		}
	})

	t.Run("show switches the painted child", func(t *testing.T) {
		s := newStack()
		s.Show("b")
		if s.Visible() != "b" {
			t.Fatalf("visible = %q, want b", s.Visible())
		}
	})

	t.Run("show ignores unknown names", func(t *testing.T) {
		s := newStack()
		s.Show("nope")
		if s.Visible() != "a" {
			t.Errorf("visible = %q, want still a", s.Visible())
		}
	})

	t.Run("hit test respects visibility", func(t *testing.T) {
		s := NewStack().Add("a", newStub(10, 10)).Add("b", newStub(10, 10))
		s.Measure(Constraints{Max: Size{W: 100, H: 100}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})
		if got := s.HitTest(Point{X: 5, Y: 5}); got != s.kids["a"] {
			t.Errorf("hit = %v, want child a", got)
		}
	})
}

func TestOverlay(t *testing.T) {
	o := NewOverlay().Append(newStub(10, 10)).Append(newStub(5, 5))
	o.Measure(Constraints{Max: Size{W: 100, H: 100}})
	o.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

	t.Run("measure takes the largest child", func(t *testing.T) {
		if got := o.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 10, H: 10}) {
			t.Errorf("overlay measure = %v", got)
		}
	})

	t.Run("topmost child wins the hit test", func(t *testing.T) {
		bottom, top := newStub(20, 20), newStub(20, 20)
		ov := NewOverlay().Append(bottom).Append(top)
		ov.Measure(Constraints{Max: Size{W: 100, H: 100}})
		ov.Arrange(render.Rect{X: 0, Y: 0, W: 30, H: 30})
		if got := ov.HitTest(Point{X: 5, Y: 5}); got != top {
			t.Errorf("hit = %v, want top", got)
		}
	})

	t.Run("an aligned child sits at its natural size and hits only there", func(t *testing.T) {
		content, corner := newStub(20, 20), newStub(6, 4)
		ov := NewOverlay().Append(content).AppendAligned(corner, AlignEnd, AlignStart)
		ov.Measure(Constraints{Max: Size{W: 100, H: 100}})
		ov.Arrange(render.Rect{X: 10, Y: 10, W: 50, H: 30})
		if got := corner.Bounds(); got != (render.Rect{X: 54, Y: 10, W: 6, H: 4}) {
			t.Errorf("corner arranged at %v, want the top-right 6x4", got)
		}
		if got := content.Bounds(); got != (render.Rect{X: 10, Y: 10, W: 50, H: 30}) {
			t.Errorf("content arranged at %v, want the whole overlay", got)
		}
		if got := ov.HitTest(Point{X: 56, Y: 12}); got != corner {
			t.Errorf("hit in the corner = %v, want the corner child", got)
		}
		if got := ov.HitTest(Point{X: 20, Y: 20}); got != content {
			t.Errorf("hit beside the corner = %v, want the content beneath", got)
		}
		centered := newStub(10, 10)
		ov2 := NewOverlay().AppendAligned(centered, AlignCenter, AlignFill)
		ov2.Arrange(render.Rect{W: 30, H: 20})
		if got := centered.Bounds(); got != (render.Rect{X: 10, Y: 0, W: 10, H: 20}) {
			t.Errorf("centered child at %v, want centered across, filling down", got)
		}
	})
}

func TestScroll(t *testing.T) {
	tall := NewBox(Column, 0, 0)
	for range 10 {
		tall.Append(newStub(50, 20), false)
	}

	newScroll := func() *Scroll {
		s := NewScroll(tall)
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		return s
	}

	t.Run("natural size is the child clamped to the constraints", func(t *testing.T) {
		// Regression: Measure used to return the constraints verbatim,
		// an unbounded appetite that blew up natural measurement in
		// parent boxes and laid the viewport past the window edge.
		s := newScroll()
		if got := s.Measure(Constraints{Max: Size{W: 60, H: 60}}); got != (Size{W: 50, H: 60}) {
			t.Errorf("viewport measure = %v, want the child natural 50x60 clamped to 60 height", got)
		}
		if s.nat.H != 200 {
			t.Errorf("child natural height = %d, want 200", s.nat.H)
		}
	})

	t.Run("a box row with an expanding scroll stays inside the parent", func(t *testing.T) {
		// Regression for the showcase: the scroll's unbounded natural
		// width pushed the row's second column off the window.
		face := entryFace(t)
		lbl := NewLabel(face, 12, "side", render.RGB(255, 255, 255))
		list := NewBox(Column, 0, 0)
		for range 24 {
			list.Append(NewLabel(face, 12, "server-01.example", render.RGB(255, 255, 255)), false)
		}
		row := NewBox(Row, 20, 0).
			Append(NewBox(Column, 8, 0).Append(lbl, false), true).
			Append(NewScroll(list), true)
		const (
			w, h = 640, 470
		)
		row.Measure(Constraints{Max: Size{W: w, H: h}})
		row.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
		// Walk to the scroll: its viewport must sit inside the window.
		var found *Scroll
		var walk func(w Widget)
		walk = func(w Widget) {
			if s, ok := w.(*Scroll); ok {
				found = s
			}
			if c, ok := w.(interface{ Children() []Widget }); ok {
				for _, k := range c.Children() {
					walk(k)
				}
			}
		}
		walk(row)
		if found == nil {
			t.Fatal("no scroll in the row")
		}
		b := found.Bounds()
		if b.X < 0 || b.Y < 0 || b.X+b.W > w || b.Y+b.H > h {
			t.Errorf("scroll viewport %v leaves the %dx%d window", b, w, h)
		}
	})

	t.Run("offset clamps to the overflow", func(t *testing.T) {
		s := newScroll()
		s.SetOffset(0, 500)
		if s.offY != 140 {
			t.Errorf("offset = %d, want clamped 140", s.offY)
		}
		s.SetOffset(0, -10)
		if s.offY != 0 {
			t.Errorf("offset = %d, want 0", s.offY)
		}
	})

	t.Run("child smaller than the viewport stays pinned", func(t *testing.T) {
		s := NewScroll(newStub(10, 10))
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		s.SetOffset(5, 5)
		if s.offX != 0 || s.offY != 0 {
			t.Errorf("offset = %d,%d, want 0,0", s.offX, s.offY)
		}
	})

	t.Run("hits inside land on the child, empty area on the scroll", func(t *testing.T) {
		s := NewScroll(newStub(20, 20))
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		// The 20x20 child centers in the 60x60 viewport: it spans
		// (20,20)-(40,40); everything else is the scroll's empty area.
		if got := s.HitTest(Point{X: 30, Y: 30}); got != s.child {
			t.Errorf("hit = %v, want the child", got)
		}
		if got := s.HitTest(Point{X: 10, Y: 10}); got != Widget(s) {
			t.Errorf("hit over empty viewport = %v, want the scroll", got)
		}
		if got := s.HitTest(Point{X: 500, Y: 500}); got != nil {
			t.Errorf("hit outside = %v, want nil", got)
		}
	})

	t.Run("fill stretches a smaller child across the viewport", func(t *testing.T) {
		s := NewScroll(newStub(20, 20))
		s.FillX, s.FillY = true, true
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		cb := s.child.(*stub).Bounds()
		if cb.W != 60 || cb.H != 60 {
			t.Errorf("child = %dx%d, want 60x60 under fill", cb.W, cb.H)
		}
		if got := s.HitTest(Point{X: 5, Y: 5}); got != s.child {
			t.Errorf("hit = %v, want the stretched child", got)
		}
	})

	t.Run("hit-testing resolves through the scroll offset", func(t *testing.T) {
		list := NewBox(Column, 0, 0)
		labels := make([]Widget, 0, 6)
		for range 6 {
			l := newStub(60, 20)
			labels = append(labels, l)
			list.Append(l, false)
		}
		s := NewScroll(list)
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		s.SetOffset(0, 30)                               // scrolled 1.5 rows down
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60}) // the loop re-arranges each frame
		// The third row's content now sits at viewport y=10..30: a press
		// there must hit row 3, not whatever row was there before.
		if got := s.HitTest(Point{X: 30, Y: 15}); got != labels[2] {
			t.Errorf("scrolled hit = %v, want the third row", got)
		}
	})
}

func TestSpacer(t *testing.T) {
	s := NewSpacer(12, 4)
	if got := s.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 12, H: 4}) {
		t.Errorf("spacer = %v, want 12x4", got)
	}
	if got := s.HitTest(Point{X: 1, Y: 1}); got != nil {
		t.Errorf("spacer hit = %v, want nil", got)
	}
}

// paintSpy is a stub that counts its paints.
type paintSpy struct {
	stub
	paints int
}

func (p *paintSpy) Paint(*render.Canvas) { p.paints++ }

func (p *paintSpy) HitTest(pt Point) Widget { return p.HitLeaf(p, pt) }

// TestOverlayHiddenChildTakesNoPart pins Box semantics for a hidden
// Overlay child: it neither sizes the overlay, paints over the others,
// nor takes their hits — a hidden full-size layer used to paint its
// background over the visible one beneath.
func TestOverlayHiddenChildTakesNoPart(t *testing.T) {
	bottom := &paintSpy{nat: Size{W: 20, H: 20}}
	top := &paintSpy{nat: Size{W: 80, H: 80}}
	ov := NewOverlay().Append(bottom).Append(top)
	top.SetVisible(false)
	if got := ov.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 20, H: 20}) {
		t.Errorf("measure = %v, want the visible child's 20x20", got)
	}
	ov.Arrange(render.Rect{W: 30, H: 30})
	ov.Paint(render.New(make([]uint8, 30*30*4), 30*4, 30, 30))
	if bottom.paints != 1 || top.paints != 0 {
		t.Errorf("paints bottom %d top %d, want 1 and 0", bottom.paints, top.paints)
	}
	if got := ov.HitTest(Point{X: 5, Y: 5}); got != bottom {
		t.Errorf("hit = %v, want the visible bottom child", got)
	}
	top.SetVisible(true)
	ov.Paint(render.New(make([]uint8, 30*30*4), 30*4, 30, 30))
	if top.paints != 1 || ov.HitTest(Point{X: 5, Y: 5}) != top {
		t.Error("a shown child did not paint and hit on top again")
	}
}

// An overlay measures and arranges through its CSS box like a Box: min
// sizes floor it with only a zero-size filling child, padding insets
// its children, and without CSS sizes it is its largest child (#254).
func TestOverlayCSSBox(t *testing.T) {
	loadCSS(t, `.ring { min-width: 64px; min-height: 64px; padding: 4px; }`)
	fill := NewSpacer(0, 0)
	ring := NewOverlay().Append(fill)
	ring.AddClass("ring")
	if got := ring.Measure(Constraints{Max: Size{W: 200, H: 200}}); got != (Size{W: 72, H: 72}) {
		t.Errorf("styled overlay %v, want the 64px floor plus 4px padding each side", got)
	}
	ring.Arrange(render.Rect{W: 72, H: 72})
	if got := fill.Bounds(); got != (render.Rect{X: 4, Y: 4, W: 64, H: 64}) {
		t.Errorf("filling child %v, want the content box", got)
	}
	plain := NewOverlay().Append(NewSpacer(30, 20))
	if got := plain.Measure(Constraints{Max: Size{W: 200, H: 200}}); got != (Size{W: 30, H: 20}) {
		t.Errorf("unstyled overlay %v, want its largest child", got)
	}
}

// A switch in a taller row keeps its pill and knob at their natural
// size, centered across, and paints nothing on the next frame: the
// row's cross stretch grew the track and its knob with it, and the
// knob was placed in Paint, so every first paint damaged it again
// (#281).
func TestSwitchKeepsItsPillWhenStretched(t *testing.T) {
	row := NewBox(Row, 8, 0)
	sw := NewSwitch(false)
	row.Append(NewSpacer(10, 40), false)
	row.Append(sw, false)
	cv := render.New(make([]byte, render.Stride(200)*60), render.Stride(200), 200, 60)
	paintFrame(cv, row, 200, 60, Current().Bg, render.Rect{W: 200, H: 60})
	if b, k := sw.Bounds(), sw.knob.Bounds(); b.H != 18 || b.W != 32 || b.Y != (60-18)/2 || k.H != 16 {
		t.Errorf("switch %v, knob %v: want the 32x18 pill centered in 60px and a 16px knob", b, k)
	}
	if _, rects := paintFrame(cv, row, 200, 60, Current().Bg, render.Rect{}); len(rects) != 0 {
		t.Errorf("an unchanged frame damaged %v", rects)
	}
}
