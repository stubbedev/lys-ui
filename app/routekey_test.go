package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// fakeTranslator stands in for the session's keymap translation.
type fakeTranslator struct {
	text map[uint32]string
	syms map[uint32]xkb.Keysym
}

// KeyUTF8 implements keyTranslator.
func (f *fakeTranslator) KeyUTF8(code uint32) string { return f.text[code] }

// KeySym implements keyTranslator.
func (f *fakeTranslator) KeySym(code uint32) xkb.Keysym { return f.syms[code] }

func TestRouteKey(t *testing.T) {
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

	t.Run("printable keys reach the focused widget as text", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{text: map[uint32]string{30: "x"}}
		routeKey(tr, r, 30, 0, nil, nil, nil)
		if got := e.Text(); got != "x" {
			t.Errorf("text = %q, want x", got)
		}
	})

	t.Run("editing keysyms become actions", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{14: xkb.KeyBackSpace}}
		e.SetText("abc")
		routeKey(tr, r, 14, 0, nil, nil, nil)
		if got := e.Text(); got != "ab" {
			t.Errorf("text = %q, want ab", got)
		}
	})

	t.Run("ctrl+a selects all instead of typing", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{30: xkb.Keysym('a')}}
		e.SetText("abc")
		routeKey(tr, r, 30, 0, nil, nil, nil)
		routeKey(tr, r, 30, wlsessionCtrl(), nil, nil, nil)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 3 {
			t.Errorf("selection = %d..%d active=%v, want 0..3 true", start, end, active)
		}
	})

	t.Run("alt never produces text", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{text: map[uint32]string{30: "x"}}
		routeKey(tr, r, 30, wlsessionAlt(), nil, nil, nil)
		if got := e.Text(); got != "" {
			t.Errorf("alt+key typed %q", got)
		}
	})

	t.Run("the app hook sees every key", func(t *testing.T) {
		r, _ := newRouter()
		tr := &fakeTranslator{}
		seen := 0
		routeKey(tr, r, 1, 0, nil, nil, func(*widget.Router, uint32, wlsession.Mods) { seen++ })
		if seen != 1 {
			t.Errorf("hook calls = %d, want 1", seen)
		}
	})

	t.Run("shift+editing keys extend through to the action", func(t *testing.T) {
		r, e := newRouter()
		tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{106: xkb.KeyRight}}
		e.SetText("hello")
		e.MoveHome()
		routeKey(tr, r, 106, wlsessionShift(), nil, nil, nil)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 1 {
			t.Errorf("selection = %d..%d active=%v, want 0..1 true", start, end, active)
		}
	})
}

// wlsessionShift, wlsessionCtrl, and wlsessionAlt build modifier masks
// without importing the session package's unexported state.
func wlsessionShift() wlsession.Mods { return wlsession.ModShift }

func wlsessionCtrl() wlsession.Mods { return wlsession.ModCtrl }

func wlsessionAlt() wlsession.Mods { return wlsession.ModAlt }

// Super+Tab is a chord, never traversal: the plain Tab trap (indent in
// a tab-absorbing widget, focus movement everywhere else) takes plain,
// ctrl, and shift Tab only, so a bound <Super>Tab accelerator fires
// and an unbound one moves no focus.
func TestSuperTabSkipsTheTabTrap(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	first := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	first.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
	second := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	second.Arrange(render.Rect{X: 0, Y: 30, W: 200, H: 30})
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(first, false)
	root.Append(second, false)
	r := &widget.Router{Root: root}
	r.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5}, 0)
	r.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})

	tr := &fakeTranslator{syms: map[uint32]xkb.Keysym{24: xkb.KeyTab}}
	routeKey(tr, r, 24, 0, nil, nil, nil)
	if r.Focused() != second {
		t.Fatal("plain Tab did not move focus")
	}
	routeKey(tr, r, 24, wlsession.ModSuper, nil, nil, nil)
	if r.Focused() != second {
		t.Error("Super+Tab moved focus; it must reach accelerators, not traversal")
	}

	app := accelApp()
	fired := false
	if err := app.accels.bind(second, "<Super>Tab", testAction(func() { fired = true })); err != nil {
		t.Fatal(err)
	}
	routeKey(tr, r, 24, wlsession.ModSuper, nil, app.accels, nil)
	if !fired {
		t.Error("the bound <Super>Tab accelerator did not fire")
	}
}
