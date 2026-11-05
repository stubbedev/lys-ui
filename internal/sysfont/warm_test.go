package sysfont

import (
	"context"
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/typesetting/fontscan"
)

func swapStore(t *testing.T, build func(context.Context) (*fontscan.FontMap, error)) {
	t.Helper()
	storeInitMu.Lock()
	prevFM, prevErr, prevBuild := storeFM, storeErr, buildStore
	storeFM, storeErr, buildStore = nil, nil, build
	storeInitMu.Unlock()
	t.Cleanup(func() {
		storeInitMu.Lock()
		storeFM, storeErr, buildStore = prevFM, prevErr, prevBuild
		storeInitMu.Unlock()
	})
}

func TestWarmReturnsOnceItsContextIsDone(t *testing.T) {
	entered := make(chan struct{})
	swapStore(t, func(ctx context.Context) (*fontscan.FontMap, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	warmed := make(chan struct{})
	go func() {
		defer close(warmed)
		Warm(ctx)
	}()
	<-entered
	cancel()
	<-warmed
}

func TestACancelledWarmLeavesTheStoreToTheNextCaller(t *testing.T) {
	builds := 0
	want := fontscan.NewFontMap(quietLogger{})
	swapStore(t, func(ctx context.Context) (*fontscan.FontMap, error) {
		builds++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return want, nil
	})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	Warm(cancelled)
	fm, err := fontmap(context.Background())
	if err != nil || fm != want {
		t.Fatalf("the store after a cancelled warm-up = %p, %v, want the built store", fm, err)
	}
	if builds != 2 {
		t.Errorf("%d builds, want the cancelled one and the next caller's", builds)
	}
}

func TestAFailedStoreBuildSticks(t *testing.T) {
	builds := 0
	failure := errors.New("no fonts")
	swapStore(t, func(context.Context) (*fontscan.FontMap, error) {
		builds++
		return nil, failure
	})
	for range 2 {
		if _, err := fontmap(context.Background()); !errors.Is(err, failure) {
			t.Fatalf("fontmap = %v, want the build's failure", err)
		}
	}
	if builds != 1 {
		t.Errorf("a failed build ran %d times, want once", builds)
	}
}
