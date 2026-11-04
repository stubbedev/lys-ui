package sysfont

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestAFallbackReadsNoFontWhereItShapes(t *testing.T) {
	primary, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	mono, err := render.LoadFont(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var opened atomic.Int32
	release := make(chan struct{})
	prev := storeFallback
	storeFallback = func([]string, rune) (*render.Typeface, error) {
		<-release
		opened.Add(1)
		return mono, nil
	}
	t.Cleanup(func() { storeFallback = prev })
	resolved := make(chan struct{}, 1)
	ch := Fallback(primary, func() { resolved <- struct{}{} })
	first := ch.Shape("a你", 14)
	if n := opened.Load(); n != 0 {
		t.Errorf("shaping an uncovered rune resolved %d faces on its goroutine, want 0", n)
	}
	if first.Runs() != 1 {
		t.Errorf("before the store answered: %d runs, want the primary alone", first.Runs())
	}
	close(release)
	select {
	case <-resolved:
	case <-time.After(time.Minute):
		t.Fatal("the store answered, and the chain's owner never heard")
	}
	if s := ch.Shape("a你", 14); s.Runs() != 2 {
		t.Errorf("after the store answered: %d runs, want 2", s.Runs())
	}
}
