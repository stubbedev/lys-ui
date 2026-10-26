package app

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/icons"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/widget"
)

// testApp builds an Application whose Invoke can be driven without a
// wayland connection; the wake stub stands in for the parked loop's
// session kick, optionally signaling a channel so a fake loop can park
// on it.
func testApp(kick chan struct{}) *Application {
	return &Application{wake: func(time.Duration) {
		if kick != nil {
			kick <- struct{}{}
		}
	}}
}

// TestInvokeWakeCoalesces pins the invoke wake budget: a storm of
// enqueues arms exactly one wake, the drain consumes it, and the next
// enqueue arms a fresh one. An Invoke that re-armed per call would
// turn a busy producer into a pass storm - the parked loop would never
// park at all.
func TestInvokeWakeCoalesces(t *testing.T) {
	a := testApp(nil)

	arms := 0
	for range 100 {
		if a.queues.enqueue(func() {}) {
			arms++
		}
	}
	if arms != 1 {
		t.Errorf("100 enqueues armed %d wakes, want 1", arms)
	}
	if got := a.queues.pending(); got != 100 {
		t.Errorf("pending invokes = %d, want 100", got)
	}

	a.pump(time.Now())
	if got := a.queues.pending(); got != 0 {
		t.Errorf("pump left %d pending invokes", got)
	}
	if !a.queues.enqueue(func() {}) {
		t.Error("enqueue after drain did not re-arm the wake")
	}
}

// TestInvokeRunsInOrderOnPump checks the loop-side half of the
// contract: fns run synchronously inside the pump, in enqueue order,
// and the drain leaves the queue empty so the loop can park.
func TestInvokeRunsInOrderOnPump(t *testing.T) {
	a := testApp(nil)
	var order []int
	for i := range 5 {
		a.Invoke(func() { order = append(order, i) })
	}
	a.pump(time.Now())
	if len(order) != 5 || order[0] != 0 || order[4] != 4 {
		t.Errorf("invoke order = %v, want 0..4 in enqueue order", order)
	}
}

// TestInvokeRunsOnLoopGoroutine is the goroutine-identity half of the
// acceptance: an Invoke-routed widget touch from a foreign goroutine
// never trips the off-loop hook, because fn runs on the goroutine that
// pumps - the loop goroutine. A direct touch from the same foreign
// goroutine trips it, which is what makes the contract testable.
func TestInvokeRunsOnLoopGoroutine(t *testing.T) {
	widget.UnmarkLoop()
	defer func() {
		widget.UnmarkLoop()
		widget.SetOffLoopHook(nil)
	}()
	var hits atomic.Int32
	widget.SetOffLoopHook(func(string) { hits.Add(1) })

	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	label := widget.NewLabel(face, 14, "start", render.RGB(255, 255, 255))

	a := testApp(nil)
	const passes = 3
	kicks := make(chan struct{})
	started := make(chan struct{})
	done := make(chan struct{})
	go func() { // the fake loop goroutine
		defer close(done)
		widget.MarkLoop()
		close(started)
		for range passes {
			<-kicks // the park
			a.pump(time.Now())
		}
	}()
	<-started
	for i := range passes {
		// From the producer goroutine: the sanctioned path.
		a.Invoke(func() { label.SetText(fmt.Sprintf("tick %d", i)) })
		kicks <- struct{}{}
	}
	<-done
	if got := hits.Load(); got != 0 {
		t.Errorf("invoke-routed SetText tripped the off-loop hook %d times", got)
	}

	// The violation: the same producer touches the widget directly.
	label.SetText("off loop")
	if got := hits.Load(); got == 0 {
		t.Error("off-loop widget touch was not detected by the hook")
	}
}

// TestInvokeDeliversAcrossGoroutines is the -race acceptance in
// miniature: producers invoke from foreign goroutines while one loop
// goroutine pumps, and the fns mutate loop-owned state with no atomics
// - the race detector would flag any path that let a producer touch it.
func TestInvokeDeliversAcrossGoroutines(t *testing.T) {
	const producers = 4
	const perProducer = 200
	kick := make(chan struct{}, producers*perProducer)
	a := testApp(kick)

	delivered := 0
	done := make(chan struct{})
	go func() { // the loop: park, wake on the kick, pump - like Run
		defer close(done)
		for delivered < producers*perProducer {
			<-kick
			a.pump(time.Now())
		}
	}()

	var wg sync.WaitGroup
	for range producers {
		wg.Go(func() {
			for range perProducer {
				a.Invoke(func() { delivered++ })
			}
		})
	}
	wg.Wait()
	<-done
	if delivered != producers*perProducer {
		t.Errorf("delivered = %d, want %d", delivered, producers*perProducer)
	}
}

// TestInvokeDropsAfterShutdown pins the post-Run behavior: queued work
// is discarded at shutdown and later Invokes are dropped, so a worker
// that keeps polling after the windows close cannot leak a queue into
// a dead application.
func TestInvokeDropsAfterShutdown(t *testing.T) {
	a := testApp(nil)
	ran := false
	a.Invoke(func() { ran = true })
	a.pump(time.Now())
	if !ran {
		t.Error("invoke queued before the pump did not run")
	}

	a.queues.shutdown()
	a.Invoke(func() { t.Error("invoke ran after shutdown") })
	if got := a.queues.pending(); got != 0 {
		t.Errorf("shutdown left %d queued invokes", got)
	}
}

// TestEveryDeadlineJoinsWakeBudget ties the pollers into the loop's
// wake computation: one pending Every deadline produces exactly one
// wake per tick over a simulated four seconds, and after cancel the
// loop goes back to no wakeups at all - the idle-park acceptance, on a
// fake clock so it stays deterministic.
func TestEveryDeadlineJoinsWakeBudget(t *testing.T) {
	a := testApp(nil)
	ticks := 0
	cancel := a.Every(500*time.Millisecond, func() { ticks++ })

	now := time.Now()
	wakes := 0
	for range 8 { // 4 simulated seconds at 2Hz
		_, ok := nextWake(time.Time{}, time.Time{}, time.Time{}, a.queues.nextDeadline(), now)
		if !ok {
			t.Fatal("pending Every scheduled no wake; the tick would never fire")
		}
		wakes++
		now = now.Add(500 * time.Millisecond)
		a.pump(now) // the loop's pass: drains invokes, fires due timers
	}
	if ticks != 8 {
		t.Errorf("ticks over 4s = %d, want 8 at the 500ms cadence", ticks)
	}
	if wakes != 8 {
		t.Errorf("loop wakes over 4s = %d, want one per 500ms tick", wakes)
	}

	cancel()
	a.pump(now.Add(time.Second))
	if ticks != 8 {
		t.Errorf("canceled Every ticked %d more times", ticks-8)
	}
	if _, ok := nextWake(time.Time{}, time.Time{}, time.Time{}, a.queues.nextDeadline(), now); ok {
		t.Error("canceled Every still drives the wake computation; the loop would never park")
	}
	if got := a.queues.timerCount(); got != 0 {
		t.Errorf("canceled timer was not reaped, %d live timers", got)
	}
}

// TestEveryAnchorsAtFireTime checks that a late pass fires one tick
// and rolls the deadline from the fire time, instead of bursting to
// catch up the missed intervals. The timer's first fire is seeded on
// the test's fake timeline (Every seeds from the wall clock).
func TestEveryAnchorsAtFireTime(t *testing.T) {
	a := testApp(nil)
	ticks := 0
	base := time.Unix(0, 0)
	t2 := &loopTimer{
		every: 100 * time.Millisecond,
		fn:    func() { ticks++ },
		next:  base.Add(100 * time.Millisecond),
	}
	a.queues.addTimer(t2)
	defer a.queues.killTimer(t2)

	a.pump(base.Add(350 * time.Millisecond)) // a pass landing 250ms late
	if ticks != 1 {
		t.Errorf("late pass fired %d ticks, want 1 (no catch-up burst)", ticks)
	}
	next := a.queues.nextDeadline()
	if d := next.Sub(base); d != 450*time.Millisecond {
		t.Errorf("deadline after late pass = %v past base, want +450ms (fire-anchored)", d)
	}

	// A later pass fires once more and rolls again - one tick maximum
	// per pass, whatever the gap.
	a.pump(base.Add(750 * time.Millisecond))
	if ticks != 2 {
		t.Errorf("second pass fired %d total ticks, want 2", ticks)
	}
	if d := a.queues.nextDeadline().Sub(base); d != 850*time.Millisecond {
		t.Errorf("deadline after second pass = %v past base, want +850ms", d)
	}
}

// TestEveryStopsOnShutdown covers the Run-exit path: shutdown (Run's
// deferred cleanup) stops every live timer, and Every after shutdown
// registers nothing.
func TestEveryStopsOnShutdown(t *testing.T) {
	a := testApp(nil)
	ticks := 0
	a.Every(50*time.Millisecond, func() { ticks++ })
	if got := a.queues.timerCount(); got != 1 {
		t.Fatalf("live timers = %d, want 1", got)
	}

	a.queues.shutdown()
	a.pump(time.Now())
	if ticks != 0 {
		t.Errorf("timer fired after shutdown")
	}
	if got := a.queues.timerCount(); got != 0 {
		t.Errorf("shutdown left %d live timers", got)
	}

	cancel := a.Every(50*time.Millisecond, func() { ticks++ })
	cancel() // must be a safe no-op on a dead application
	if got := a.queues.timerCount(); got != 0 {
		t.Errorf("Every after shutdown registered %d timers", got)
	}
}

// TestEveryRejectsZeroInterval: a zero interval would spin the loop,
// so Every panics rather than scheduling it.
func TestEveryRejectsZeroInterval(t *testing.T) {
	a := testApp(nil)
	defer func() {
		if recover() == nil {
			t.Error("Every(0) did not panic")
		}
	}()
	a.Every(0, func() {})
}

// TestIconResetRepaints pins the icon-cache bridge: a reset from any
// goroutine (a refresh after icons were installed) marks every window
// dirty on the next pump, and nothing is marked before the pump runs.
func TestIconResetRepaints(t *testing.T) {
	a := testApp(nil)
	w1, w2 := &hostWindow{}, &hostWindow{}
	a.windows = []*hostWindow{w1, w2}
	c := icons.New("hicolor")
	a.repaintOnIconReset(c)

	done := make(chan struct{})
	go func() { c.InvalidateTheme(); close(done) }()
	<-done
	if w1.dirty || w2.dirty {
		t.Fatal("windows marked dirty off the loop, before the pump")
	}
	a.pump(time.Now())
	if !w1.dirty || !w2.dirty {
		t.Errorf("dirty after the pump = %v, %v; want both", w1.dirty, w2.dirty)
	}

	w1.dirty, w2.dirty = false, false
	c.SetTheme("hicolor")
	a.pump(time.Now())
	if w1.dirty || w2.dirty {
		t.Error("a no-op SetTheme repainted")
	}
}

func TestSetSurfaceMotion(t *testing.T) {
	a := testApp(nil)
	t.Cleanup(func() { surfx.SetEnabled(true) })
	a.SetSurfaceMotion(false)
	if st := surfx.Plan(surfx.KindOverlay, true); surfx.Enabled() || st.Enter.Duration != 0 || st.Enter.Spring != nil {
		t.Error("surface motion off left the overlay motion running")
	}
	a.SetSurfaceMotion(true)
	if st := surfx.Plan(surfx.KindOverlay, true); !surfx.Enabled() || st.Enter.Spring == nil {
		t.Error("surface motion on did not restore the overlay spring")
	}
}
