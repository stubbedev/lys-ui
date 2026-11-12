package waittest

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

type fatalRecorder struct {
	testing.TB
	failure string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.failure = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func failureOf(t *testing.T, wait func(testing.TB)) string {
	t.Helper()
	orig, origSlow := limit, slowLimit
	limit, slowLimit = 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { limit, slowLimit = orig, origSlow })
	r := &fatalRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		wait(r)
	}()
	<-done
	return r.failure
}

func TestForReturnsWhatArrives(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 7
	if got := For(t, ch, "the value"); got != 7 {
		t.Errorf("For = %d, want what was sent", got)
	}
}

func TestForAnswersAClosedSignal(t *testing.T) {
	done := make(chan struct{})
	close(done)
	For(t, done, "the closed signal")
}

func TestForFailsNamingWhatNeverArrived(t *testing.T) {
	got := failureOf(t, func(tb testing.TB) { For(tb, make(chan int), "the reply") })
	if want := "waited 10ms for the reply"; got != want {
		t.Errorf("failure %q, want %q", got, want)
	}
}

func TestUntilReturnsOnceTheConditionHolds(t *testing.T) {
	polls := 0
	Until(t, func() bool { polls++; return polls == 3 }, "the third poll")
	if polls != 3 {
		t.Errorf("Until polled %d times, want to stop at the third", polls)
	}
}

func TestUntilFailsNamingWhatNeverHeld(t *testing.T) {
	got := failureOf(t, func(tb testing.TB) { Until(tb, func() bool { return false }, "the condition") })
	if want := "waited 10ms for the condition"; got != want {
		t.Errorf("failure %q, want %q", got, want)
	}
}

func TestForSlowWaitsForTheSlowLimit(t *testing.T) {
	got := failureOf(t, func(tb testing.TB) { ForSlow(tb, make(chan int), "the GPU worker") })
	if want := "waited 20ms for the GPU worker"; got != want {
		t.Errorf("failure %q, want %q", got, want)
	}
}
