package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func fallbackFaces(t *testing.T) (primary, other *render.Typeface) {
	t.Helper()
	primary, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	other, err = render.LoadFont(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return primary, other
}

func TestALabelWithAnUncoveredRuneRelayersOnceItsFaceLands(t *testing.T) {
	f := newFakeInvoker(t)
	primary, other := fallbackFaces(t)
	release := make(chan struct{})
	chain := render.NewChain(primary).WithResolver(func(rune) *render.Typeface {
		<-release
		return other
	}, FontFaceResolved)
	var l *Label
	onTime(t, "a label showing a rune no face of its own covers", func() {
		l = NewLabel(chain, 14, "a你", 0)
		l.Measure(Row, Unknown)
		l.Arrange(render.Rect{W: 200, H: 30})
		paintOnce(l, 200, 30)
	})
	if l.shaped.Runs() != 1 {
		t.Fatalf("before the answer the label shaped %d runs, want the primary alone", l.shaped.Runs())
	}
	misses := l.measureMissCount()
	close(release)
	f.drain(t, 1)
	l.Measure(Row, Unknown)
	if l.measureMissCount() == misses {
		t.Error("the answer left the label's measure standing")
	}
	if l.shaped.Runs() != 2 {
		t.Errorf("after the answer the label shaped %d runs, want 2", l.shaped.Runs())
	}
}

func TestAFallbackSettlesInlineWithoutALoop(t *testing.T) {
	SetInvoker(nil)
	primary, other := fallbackFaces(t)
	chain := render.NewChain(primary).WithResolver(func(rune) *render.Typeface { return other }, FontFaceResolved)
	if n := NewLabel(chain, 14, "a你", 0).shaped.Runs(); n != 2 {
		t.Errorf("a label without a loop shaped %d runs, want its fallback's 2", n)
	}
	if n := NewRichLabel(chain, 14, "b好", 0).shaped[0].sh.Runs(); n != 2 {
		t.Errorf("a rich label without a loop shaped %d runs, want its fallback's 2", n)
	}
}

func TestFallbackAnswersShareOneRelayout(t *testing.T) {
	f := newFakeInvoker(t)
	gen := styleGen
	FontFaceResolved()
	FontFaceResolved()
	FontFaceResolved()
	f.drain(t, 1)
	if styleGen != gen+1 {
		t.Errorf("three answers before the loop's pass bumped the style %d times, want 1", styleGen-gen)
	}
	FontFaceResolved()
	f.drain(t, 1)
	if styleGen != gen+2 {
		t.Errorf("an answer after the pass bumped the style %d times in all, want 2", styleGen-gen)
	}
}
