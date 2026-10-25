package app

import (
	"path/filepath"
	"testing"

	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/compose"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// startCompose parses the shared compose fixture for wiring tests.
func startCompose(t *testing.T) *compose.State {
	t.Helper()
	tb, err := compose.NewTableFile(filepath.Join("..", "internal", "compose", "testdata", "XCompose"))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return tb.Start()
}

// composeTranslator is a keyTranslator that carries a compose machine,
// the way *wlsession.Session does.
type composeTranslator struct {
	*fakeTranslator
	comp *compose.State
}

func (c *composeTranslator) ComposePending() bool { return c.comp.Composing() }

func (c *composeTranslator) FeedCompose(sym xkb.Keysym) (compose.Result, string) {
	return c.comp.Feed(sym)
}

func (c *composeTranslator) ComposeBackspace() bool { return c.comp.Backspace() }

// deadKeys maps keycodes to the us-intl keysyms the compose tests use.
func deadKeys() map[uint32]xkb.Keysym {
	return map[uint32]xkb.Keysym{
		30: xkb.KeyDeadAcute,
		31: 'e',
		14: xkb.KeyBackSpace,
		50: 's',
		51: 't',
		52: 'r',
	}
}

func TestRouteKeyCompose(t *testing.T) {
	newRouter := func(t *testing.T) (*widget.Router, *widget.Entry) {
		t.Helper()
		face, err := render.LoadFont(goregular.TTF)
		if err != nil {
			t.Fatal(err)
		}
		e := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
		e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
		r := &widget.Router{Root: e}
		r.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5}, 0)
		r.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
		return r, e
	}

	t.Run("dead keys commit only on completion", func(t *testing.T) {
		r, e := newRouter(t)
		tr := &composeTranslator{fakeTranslator: &fakeTranslator{syms: deadKeys()}, comp: startCompose(t)}
		routeKey(tr, r, 30, 0, nil, nil, nil)
		if got := e.Text(); got != "" {
			t.Fatalf("pending dead key typed %q", got)
		}
		if !tr.ComposePending() {
			t.Fatal("sequence should be in flight")
		}
		routeKey(tr, r, 31, 0, nil, nil, nil)
		if got := e.Text(); got != "é" {
			t.Fatalf("text = %q, want é", got)
		}
		if tr.ComposePending() {
			t.Fatal("sequence should be over")
		}
	})

	t.Run("backspace unwinds a pending level without deleting", func(t *testing.T) {
		r, e := newRouter(t)
		tr := &composeTranslator{fakeTranslator: &fakeTranslator{syms: deadKeys()}, comp: startCompose(t)}
		e.SetText("ab")
		routeKey(tr, r, 30, 0, nil, nil, nil)
		routeKey(tr, r, 14, 0, nil, nil, nil)
		if got := e.Text(); got != "ab" {
			t.Fatalf("pending backspace deleted text: %q", got)
		}
		if tr.ComposePending() {
			t.Fatal("the last level was unwound: nothing should be pending")
		}
		// A deeper sequence keeps its surviving prefix pending.
		routeKey(tr, r, 50, 0, nil, nil, nil)
		routeKey(tr, r, 51, 0, nil, nil, nil)
		routeKey(tr, r, 14, 0, nil, nil, nil)
		if !tr.ComposePending() {
			t.Fatal("one compose level should remain")
		}
		routeKey(tr, r, 51, 0, nil, nil, nil)
		routeKey(tr, r, 52, 0, nil, nil, nil)
		if got := e.Text(); got != "ab★" {
			t.Fatalf("text = %q, want ab★", got)
		}
	})

	t.Run("backspace deletes when nothing is pending", func(t *testing.T) {
		r, e := newRouter(t)
		tr := &composeTranslator{fakeTranslator: &fakeTranslator{syms: deadKeys()}, comp: startCompose(t)}
		e.SetText("abc")
		routeKey(tr, r, 14, 0, nil, nil, nil)
		if got := e.Text(); got != "ab" {
			t.Fatalf("text = %q, want ab", got)
		}
	})

	t.Run("a non-sequence key cancels and types", func(t *testing.T) {
		r, e := newRouter(t)
		tr := &composeTranslator{
			fakeTranslator: &fakeTranslator{
				text: map[uint32]string{45: "x"},
				syms: deadKeys(),
			},
			comp: startCompose(t),
		}
		routeKey(tr, r, 30, 0, nil, nil, nil)
		routeKey(tr, r, 45, 0, nil, nil, nil)
		if got := e.Text(); got != "x" {
			t.Fatalf("text = %q, want x", got)
		}
		if tr.ComposePending() {
			t.Fatal("sequence should be cancelled")
		}
	})

	t.Run("translators without compose are untouched", func(t *testing.T) {
		r, e := newRouter(t)
		tr := &fakeTranslator{text: map[uint32]string{30: "x"}}
		routeKey(tr, r, 30, 0, nil, nil, nil)
		if got := e.Text(); got != "x" {
			t.Fatalf("text = %q, want x", got)
		}
	})
}
