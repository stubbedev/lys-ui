package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// fixedWidget is a leaf of constant natural size, the test child for
// containers.
type fixedWidget struct {
	node
	sz    Size
	paint int
}

func (f *fixedWidget) Measure(axis Axis, forSize int) Sizes { return FixedSizes(f.sz.Along(axis)) }

func (f *fixedWidget) Arrange(r render.Rect) { f.node.Arrange(r) }

func (f *fixedWidget) Paint(cv *render.Canvas) { f.paint++ }

func (f *fixedWidget) HitTest(p Point) Widget { return f.HitLeaf(f, p) }

func TestToastAutoDismiss(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	t0 := c.now()
	toast := NewToast(face, "Saved", 500*time.Millisecond)
	fired := 0
	toast.OnDismissed = func() { fired++ }

	t.Run("a toast out of the tree holds no timer", func(t *testing.T) {
		if animActiveNow(t) {
			t.Error("unarranged toast scheduled animation work")
		}
	})

	SizeWithin(toast, Size{W: 400, H: 100})
	toast.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 32})

	t.Run("arranging starts the entrance and the dismiss clock", func(t *testing.T) {
		if !animActiveNow(t) {
			t.Fatal("visible toast scheduled nothing")
		}
		wake, ok := animclock.Next()
		if !ok {
			t.Fatal("no wake scheduled")
		}
		// The entrance is mid-flight: the next wake is exactly one
		// frame period past the last tick.
		if want := t0.Add(animclock.FrameInterval); wake != want {
			t.Errorf("first wake = %v, want exactly %v", wake, want)
		}
	})

	t.Run("dismissal lands exactly at the timeout", func(t *testing.T) {
		last := t0
		for c.step() {
			last = c.now()
		}
		if want := t0.Add(500 * time.Millisecond); last != want {
			t.Errorf("dismissal ticked at %v, want exactly %v", last, want)
		}
		if fired != 1 {
			t.Errorf("OnDismissed fired %d times, want 1", fired)
		}
		if animActiveNow(t) {
			t.Error("finished toast still scheduled wakes")
		}
		c.set(t0.Add(10 * time.Second))
		animclock.Tick(c.now())
		if fired != 1 {
			t.Errorf("fired = %d after the world moved on, want 1", fired)
		}
	})
}

// animActiveNow reports whether the animation clock has anything
// scheduled; the zero-wake contract is the "no idle burn" pin.
func animActiveNow(tb testing.TB) bool {
	tb.Helper()
	if animclock.Active() {
		return true
	}
	_, ok := animclock.Next()
	return ok
}

func TestToastHoverCancelsDismissal(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	t0 := c.now()
	toast := NewToast(face, "Syncing", 500*time.Millisecond)
	fired := 0
	toast.OnDismissed = func() { fired++ }
	SizeWithin(toast, Size{W: 400, H: 100})
	toast.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 32})

	// One frame in, the pointer lands on the toast: the pending
	// dismissal cancels.
	if !c.step() {
		t.Fatal("entrance scheduled no frame")
	}
	toast.SetHovered(true)
	c.drive() // drains the entrance; the dismissal is gone

	if fired != 0 {
		t.Fatalf("toast dismissed despite hover, fired = %d", fired)
	}
	if animActiveNow(t) {
		t.Error("hovered toast still holds a timer")
	}
	// The timeout elapses with the pointer resting: nothing fires.
	c.set(t0.Add(5 * time.Second))
	animclock.Tick(c.now())
	if fired != 0 {
		t.Errorf("dismissed while hovered, fired = %d", fired)
	}

	// Leaving arms a fresh full timeout from now.
	toast.SetHovered(false)
	wake, ok := animclock.Next()
	if !ok {
		t.Fatal("unhovered toast armed no dismissal")
	}
	if want := t0.Add(5*time.Second + 500*time.Millisecond - toastPlan().Exit.Duration); wake != want {
		t.Errorf("rearmed fade start = %v, want exactly %v", wake, want)
	}
	for c.step() {
	}
	if fired != 1 {
		t.Errorf("fired = %d after the rearmed timeout, want 1", fired)
	}
}

func TestToastHoverRescuesMidExit(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	t0 := c.now()
	toast := NewToast(face, "Undo?", time.Second)
	fired := 0
	toast.OnDismissed = func() { fired++ }
	SizeWithin(toast, Size{W: 400, H: 100})
	toast.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 32})

	toast.Dismiss()
	// Halfway through the exit fade, hover snaps the card back.
	for c.now().Sub(t0) < toastPlan().Exit.Duration/2 {
		if !c.step() {
			t.Fatal("exit fade scheduled no frame")
		}
	}
	if toast.progress <= 0 || toast.progress >= 1 {
		t.Fatalf("progress = %v, want mid-fade", toast.progress)
	}
	toast.SetHovered(true)
	if toast.progress != 1 {
		t.Errorf("progress = %v after hover rescue, want 1", toast.progress)
	}
	for c.step() {
	}
	if fired != 0 {
		t.Errorf("rescued toast dismissed anyway, fired = %d", fired)
	}
}

func TestToastCloseStopsEverything(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	toast := NewToast(face, "bye", time.Second)
	fired := 0
	toast.OnDismissed = func() { fired++ }
	SizeWithin(toast, Size{W: 400, H: 100})
	toast.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 32})
	if !animActiveNow(t) {
		t.Fatal("armed toast scheduled nothing")
	}

	toast.Close()
	if !toast.Closed() {
		t.Error("closed toast not marked closed")
	}
	if animActiveNow(t) {
		t.Error("closed toast kept its timer")
	}
	c.set(c.now().Add(5 * time.Second))
	animclock.Tick(c.now())
	if fired != 0 {
		t.Errorf("Close fired OnDismissed %d times, want 0", fired)
	}
}

func TestToastActionClick(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	toast := NewToast(face, "Update ready", time.Second)
	acted, fired := 0, 0
	toast.SetAction("Restart", func() { acted++ })
	toast.OnDismissed = func() { fired++ }
	SizeWithin(toast, Size{W: 400, H: 100})
	toast.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 36})
	if toast.actionRect.Empty() {
		t.Fatal("action hit rect never laid out")
	}

	t.Run("clicks off the button are inert", func(t *testing.T) {
		toast.ClickAt(Point{X: toast.bounds.X + 2, Y: toast.bounds.Y + 2})
		if acted != 0 || fired != 0 {
			t.Errorf("acted = %d fired = %d, want 0/0", acted, fired)
		}
	})

	t.Run("the button fires its action and dismisses", func(t *testing.T) {
		toast.ClickAt(Point{X: toast.actionRect.X + 2, Y: toast.actionRect.Y + 2})
		if acted != 1 {
			t.Fatalf("acted = %d, want 1", acted)
		}
		for c.step() {
		}
		if fired != 1 {
			t.Errorf("fired = %d after the action's dismissal, want 1", fired)
		}
	})
}

// A description is a second line under the text: the card grows by it
// and is as wide as the longer line; clearing it restores the card.
func TestToastDescriptionLine(t *testing.T) {
	face := testFace(t)
	toast := NewToast(face, "Saved", 0)
	one := SizeWithin(toast, Size{W: 1000, H: 1000})
	toast.SetDescription("Sunday, December 03, 2023 at 9:00 AM")
	two := SizeWithin(toast, Size{W: 1000, H: 1000})
	line := face.Shape(toast.Description(), toast.sizePx)
	if two.H != one.H+toastLineGap+line.LineHeight() || two.W != 2*toastPadX+int(line.Advance()+0.5) {
		t.Errorf("with a description %v, want %v grown by a line and as wide as it", two, one)
	}
	toast.SetDescription("")
	if got := SizeWithin(toast, Size{W: 1000, H: 1000}); got != one {
		t.Errorf("cleared description %v, want %v", got, one)
	}
}
