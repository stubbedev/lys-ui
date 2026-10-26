package widget

import (
	"sync/atomic"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"
)

// TestOffLoopMutationTripsHook enforces the threading contract's
// detection half (docs/threading.md): with the guard armed on the
// goroutine playing the loop, mutations on that goroutine are silent,
// and the same mutations from any other goroutine fire the off-loop
// hook. This is the test that fails when a callback touches widgets
// off-loop.
func TestOffLoopMutationTripsHook(t *testing.T) {
	UnmarkLoop()
	defer func() {
		UnmarkLoop()
		SetOffLoopHook(nil)
	}()
	var hits atomic.Int32
	SetOffLoopHook(func(what string) { hits.Add(1) })

	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}

	MarkLoop() // this goroutine plays the loop
	label := NewLabel(face, 14, "one", render.RGB(255, 255, 255))
	label.SetText("two")
	box := NewBox(Column, 4, 4)
	box.Append(label, false)
	box.Invalidate()
	if got := hits.Load(); got != 0 {
		t.Fatalf("on-loop mutations tripped the hook %d times", got)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		label.SetText("off loop") // the violation under test
	}()
	<-done
	if got := hits.Load(); got == 0 {
		t.Error("off-loop SetText was not detected; the guard missed a real violation")
	}

	// A second off-loop widget (InvalidateRect path) is caught too.
	before := hits.Load()
	off := NewLabel(face, 14, "x", render.RGB(255, 255, 255))
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		off.InvalidateRect(render.Rect{X: 1, Y: 1, W: 3, H: 3})
	}()
	<-done2
	if hits.Load() == before {
		t.Error("off-loop InvalidateRect was not detected")
	}

	// Disarming silences the guard: post-Run teardown may touch
	// widgets from any goroutine.
	UnmarkLoop()
	done3 := make(chan struct{})
	go func() {
		defer close(done3)
		label.SetText("teardown")
	}()
	<-done3
	if got := hits.Load(); got != before+1 {
		t.Errorf("disarmed guard still tripped (%d hits, want %d)", hits.Load(), before+1)
	}
}

// TestMarkLoopReArms checks that marking a different goroutine moves
// the guard: lys-ui-hello style tests mark whichever goroutine drives
// the tree.
func TestMarkLoopReArms(t *testing.T) {
	UnmarkLoop()
	defer UnmarkLoop()
	var hits atomic.Int32
	SetOffLoopHook(func(string) { hits.Add(1) })

	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}

	MarkLoop()
	entered := make(chan struct{})
	done := make(chan struct{})
	go func() { // a new "loop" goroutine takes over
		defer close(done)
		MarkLoop()
		close(entered)
		label := NewLabel(face, 12, "new loop owns the tree", render.RGB(255, 255, 255))
		label.SetText("moved")
	}()
	<-entered
	if got := hits.Load(); got != 0 {
		t.Errorf("re-marked loop goroutine tripped the hook %d times", got)
	}
	<-done
}
