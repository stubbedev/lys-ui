package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

func TestRouteKeyUndoRedo(t *testing.T) {
	newRouter := func() (*widget.Router, *widget.Entry) {
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

	t.Run("ctrl+z undoes, ctrl+shift+z and ctrl+y redo", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{
			text: map[uint32]string{30: "x"},
			syms: map[uint32]xkb.Keysym{52: xkb.Keysym('z'), 56: xkb.Keysym('y')},
		}
		routeKey(tr, r, 30, 0, nil, nil, nil) // types x
		routeKey(tr, r, 52, wlsessionCtrl(), nil, nil, nil)
		if got := e.Text(); got != "" {
			t.Fatalf("after ctrl+z text = %q, want empty", got)
		}
		routeKey(tr, r, 52, wlsessionCtrl()|wlsessionShift(), nil, nil, nil)
		if got := e.Text(); got != "x" {
			t.Errorf("after ctrl+shift+z text = %q, want x", got)
		}
		routeKey(tr, r, 52, wlsessionCtrl(), nil, nil, nil)
		routeKey(tr, r, 56, wlsessionCtrl(), nil, nil, nil)
		if got := e.Text(); got != "x" {
			t.Errorf("after ctrl+y text = %q, want x", got)
		}
	})

	t.Run("without undoable focus ctrl+z is left for accels and OnKey", func(t *testing.T) {
		r := &widget.Router{} // nothing focused
		tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{52: xkb.Keysym('z')}}
		hooked := 0
		routeKey(tr, r, 52, wlsessionCtrl(), nil, nil, func(*widget.Router, uint32, wlsession.Mods) { hooked++ })
		if hooked != 1 {
			t.Errorf("hook calls = %d, want 1", hooked)
		}
	})
}

func TestPasteLandsAsOneUndoEntry(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	e := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
	r := &widget.Router{Root: e}
	r.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5}, 0)
	r.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	// pasteSelection inserts the whole clipboard in one TextInserter
	// call; exercise that path's shape.
	ins, ok := r.Focused().(widget.TextInserter)
	if !ok {
		t.Fatal("entry must accept bulk inserts")
	}
	ins.Insert("hello world")
	if !e.Undo() || e.Text() != "" {
		t.Errorf("one undo gave %q, want empty (a paste is one entry)", e.Text())
	}
}
