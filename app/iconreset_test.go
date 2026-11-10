package app

import (
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/stubbedev/lys-lib/icons"
)

func TestAClosedApplicationIsNoLongerHeldByTheIconCacheItFollowed(t *testing.T) {
	c := icons.New("hicolor")
	a := testApp(nil)
	a.repaintOnIconReset(c)
	held := weak.Make(a)
	runtime.GC()
	if held.Value() == nil {
		t.Fatal("the cache does not hold the application that follows its resets")
	}
	a.Close()
	a = nil
	for range 3 {
		runtime.GC()
	}
	if held.Value() != nil {
		t.Error("the icon cache still holds a closed application")
	}
	c.InvalidateTheme()
}

func TestAnIconResetRunningWhileTheApplicationClosesIsDropped(t *testing.T) {
	c := icons.New("hicolor")
	a := testApp(nil)
	window := &hostWindow{}
	a.windows = []*hostWindow{window}
	c.OnReset(a.Close)
	a.repaintOnIconReset(c)

	c.InvalidateTheme()
	if n := a.queues.pending(); n != 0 {
		t.Fatalf("a reset that reached the application after it closed queued %d invokes", n)
	}
	a.pump(time.Now())
	if window.dirty {
		t.Error("a closed application repainted for an icon reset")
	}
}
