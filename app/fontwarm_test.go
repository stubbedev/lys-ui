package app

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func holdFontWarm(t *testing.T) (entered <-chan struct{}, returned <-chan struct{}) {
	t.Helper()
	enteredCh, returnedCh := make(chan struct{}), make(chan struct{})
	prev := warmFonts
	warmFonts = func(ctx context.Context) {
		defer close(returnedCh)
		close(enteredCh)
		<-ctx.Done()
	}
	t.Cleanup(func() { warmFonts = prev })
	return enteredCh, returnedCh
}

func TestRunStopsAFontWarmUpStillScanning(t *testing.T) {
	entered, returned := holdFontWarm(t)
	sess, _ := deadPeerSession(t)
	baseline := runtime.NumGoroutine()
	a, _, _ := disconnectApp(t, sess)
	<-entered
	if err := a.Run(); err == nil {
		t.Fatal("Run over a dead socket returned no error")
	}
	select {
	case <-returned:
	default:
		t.Fatal("Run returned with the font warm-up still scanning")
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > baseline {
		t.Errorf("goroutines after Run = %d, want the baseline %d from before the application", n, baseline)
	}
}

func TestAFontWarmUpThatFinishesIsNotWaitedOnAgain(t *testing.T) {
	ran := make(chan struct{})
	stop := startFontWarm(func(context.Context) { close(ran) })
	<-ran
	start := time.Now()
	stop()
	stop()
	if waited := time.Since(start); waited >= fontWarmStopTimeout {
		t.Errorf("stopping a finished warm-up waited %v", waited)
	}
}
