package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// fakeIMEWire stands in for the session's text-input requests and
// records everything the controller pushes.
type fakeIMEWire struct {
	available bool
	updates   []wlsession.IMEState
}

func (f *fakeIMEWire) TextInputAvailable() bool { return f.available }

func (f *fakeIMEWire) UpdateIME(st wlsession.IMEState) { f.updates = append(f.updates, st) }

// imeRouter returns a router focused on an entry holding "ab".
func imeRouter(t *testing.T) (*widget.Router, *widget.Entry) {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	e := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	e.SetText("ab")
	e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
	r := &widget.Router{Root: e}
	// Click past the end of the text but inside the field: focuses
	// the entry with the caret at the end.
	r.Press(widget.BTNLeft, widget.Point{X: 190, Y: 5}, 0)
	r.Release(widget.BTNLeft, widget.Point{X: 190, Y: 5})
	return r, e
}

func TestIMESync(t *testing.T) {
	t.Run("an editable focus enables text input with surrounding text", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, _ := imeRouter(t)
		newIMEController(wire).sync(r, true)
		if len(wire.updates) != 1 {
			t.Fatalf("updates = %d, want 1", len(wire.updates))
		}
		st := wire.updates[0]
		if !st.Enabled {
			t.Error("the input method must be enabled over a focused entry")
		}
		if st.Surrounding != "ab" || st.Cursor != 2 || st.Anchor != 2 {
			t.Errorf("surrounding = %q %d..%d, want ab 2..2", st.Surrounding, st.Cursor, st.Anchor)
		}
		if !st.CauseOther {
			t.Error("syncs outside the input method must carry cause other")
		}
		if st.CursorRect.H <= 0 {
			t.Errorf("cursor rect = %+v, want the caret band", st.CursorRect)
		}
	})

	t.Run("unchanged state is not pushed again", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, _ := imeRouter(t)
		c := newIMEController(wire)
		c.sync(r, true)
		c.sync(r, true)
		if len(wire.updates) != 1 {
			t.Errorf("updates = %d, want 1 (unchanged state must dedup)", len(wire.updates))
		}
	})

	t.Run("caret motion re-pushes the surrounding text", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, e := imeRouter(t)
		c := newIMEController(wire)
		c.sync(r, true)
		e.MoveCursor(-1)
		c.sync(r, true)
		if len(wire.updates) != 2 {
			t.Fatalf("updates = %d, want 2", len(wire.updates))
		}
		if wire.updates[1].Cursor != 1 {
			t.Errorf("cursor = %d, want 1", wire.updates[1].Cursor)
		}
	})

	t.Run("focus leaving text disables the input method", func(t *testing.T) {
		face, err := render.LoadFont(goregular.TTF)
		if err != nil {
			t.Fatal(err)
		}
		e := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
		e.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 30})
		b := widget.NewButton(widget.NewLabel(face, 13, "go", render.RGB(255, 255, 255)), 8, 4)
		box := widget.NewBox(widget.Row, 4, 0).Append(e, true).Append(b, false)
		box.Measure(widget.Constraints{Max: widget.Size{W: 300, H: 60}})
		box.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 60})
		r := &widget.Router{Root: box}
		ep := e.Bounds()
		p := widget.Point{X: ep.X + 5, Y: ep.Y + 5}
		r.Press(widget.BTNLeft, p, 0)
		r.Release(widget.BTNLeft, p)

		wire := &fakeIMEWire{available: true}
		c := newIMEController(wire)
		c.sync(r, true)
		if len(wire.updates) != 1 || !wire.updates[0].Enabled {
			t.Fatalf("first sync must enable, got %+v", wire.updates)
		}
		bp := box.Bounds()
		r.Press(widget.BTNLeft, widget.Point{X: bp.X + bp.W - 5, Y: 15}, 0)
		r.Release(widget.BTNLeft, widget.Point{X: bp.X + bp.W - 5, Y: 15})
		c.sync(r, true)
		if len(wire.updates) != 2 {
			t.Fatalf("updates = %d, want 2", len(wire.updates))
		}
		if wire.updates[1].Enabled {
			t.Error("a button focus must disable the input method")
		}
	})

	t.Run("the caret rect is surface-local at any device scale", func(t *testing.T) {
		// The tree is laid out in logical surface pixels, so the caret
		// rect passes to set_cursor_rectangle unmodified - there is no
		// scale division left to round wrong at fractional factors.
		wire := &fakeIMEWire{available: true}
		r, _ := imeRouter(t)
		newIMEController(wire).sync(r, true)
		st := wire.updates[0]
		if st.CursorRect.X > 200 || st.CursorRect.H > 30 {
			t.Errorf("caret rect = %+v, want surface (not buffer) coordinates", st.CursorRect)
		}
	})
}

func TestIMESyncWithoutProtocol(t *testing.T) {
	// Regression: a compositor without zwp_text_input_manager_v3 must
	// see exactly today's behavior — no requests, typing intact.
	wire := &fakeIMEWire{}
	r, e := imeRouter(t)
	c := newIMEController(wire)
	c.sync(r, true)
	c.sync(r, true)
	if len(wire.updates) != 0 {
		t.Errorf("updates without the protocol = %d, want 0", len(wire.updates))
	}
	tr := &fakeTranslator{text: map[uint32]string{30: "x"}}
	routeKey(tr, r, 30, 0, nil, nil, nil)
	if got := e.Text(); got != "abx" {
		t.Errorf("text = %q, want abx (keyboard path must be untouched)", got)
	}
	c.deliver(r, wlsession.IMEEvent{Commit: "y", Current: true})
	if len(wire.updates) != 0 {
		t.Errorf("updates = %d, want 0 even after a batch", len(wire.updates))
	}
}

func TestIMEDeliver(t *testing.T) {
	t.Run("preedit shows in the entry without touching the contents", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, e := imeRouter(t)
		newIMEController(wire).deliver(r, wlsession.IMEEvent{
			Preedit: "かん", PreeditCursorBegin: 3, PreeditCursorEnd: 3, Current: true,
		})
		if !e.Composing() {
			t.Fatal("the entry must be composing after a preedit batch")
		}
		if got := e.Text(); got != "ab" {
			t.Errorf("text = %q, want ab", got)
		}
	})

	t.Run("commit lands in the entry and clears the preedit", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, e := imeRouter(t)
		c := newIMEController(wire)
		c.deliver(r, wlsession.IMEEvent{Preedit: "かん", PreeditCursorBegin: 3, PreeditCursorEnd: 3})
		c.deliver(r, wlsession.IMEEvent{Commit: "漢", Current: true})
		if got := e.Text(); got != "ab漢" {
			t.Errorf("text = %q, want ab漢", got)
		}
		if e.Composing() {
			t.Error("commit must clear the composing display")
		}
	})

	t.Run("delete surrounding applies before the commit", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, e := imeRouter(t)
		newIMEController(wire).deliver(r, wlsession.IMEEvent{
			DeleteBefore: 1, Commit: "z", Current: true,
		})
		if got := e.Text(); got != "az" {
			t.Errorf("text = %q, want az", got)
		}
	})

	t.Run("a current batch re-pushes state with the input-method cause", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, _ := imeRouter(t)
		c := newIMEController(wire)
		c.deliver(r, wlsession.IMEEvent{Commit: "x", Current: true})
		if len(wire.updates) != 1 {
			t.Fatalf("updates = %d, want 1 resync", len(wire.updates))
		}
		if wire.updates[0].CauseOther {
			t.Error("a resync after input-method changes must not claim cause other")
		}
		if !wire.updates[0].Enabled || wire.updates[0].Surrounding != "abx" {
			t.Errorf("resync = %+v, want enabled with surrounding abx", wire.updates[0])
		}
	})

	t.Run("a stale batch applies but skips the resync", func(t *testing.T) {
		wire := &fakeIMEWire{available: true}
		r, e := imeRouter(t)
		newIMEController(wire).deliver(r, wlsession.IMEEvent{Commit: "x"})
		if got := e.Text(); got != "abx" {
			t.Errorf("text = %q, want abx (stale batches still apply)", got)
		}
		if len(wire.updates) != 0 {
			t.Errorf("updates = %d, want 0 (stale batches skip the resync)", len(wire.updates))
		}
	})
}

func TestIMEReset(t *testing.T) {
	wire := &fakeIMEWire{available: true}
	r, e := imeRouter(t)
	c := newIMEController(wire)
	c.sync(r, true)
	c.reset()
	c.sync(r, true)
	if len(wire.updates) != 2 {
		t.Errorf("updates after reset = %d, want 2 (focus moves invalidate state)", len(wire.updates))
	}
	_ = e
}

// While a popover holds the keyboard the input method follows its
// router: its Entry enables text input, batches land in it (not in the
// window's), the popover repaints, and dismissal hands text input back
// to the window's focus - disabling it when that is not editable.
func TestIMEFollowsPopover(t *testing.T) {
	wire := &fakeIMEWire{available: true}
	popRouter, popEntry := imeRouter(t)
	op, surf, _, _ := newLoopPopover(popEntry)
	op.router = popRouter
	label := widget.NewLabel(testFace(t), 14, "w", render.RGB(255, 255, 255))
	win := &hostWindow{router: &widget.Router{Root: label}}
	a := &Application{sess: &wlsession.Session{}, ime: newIMEController(wire), windows: []*hostWindow{win}, openPopovers: []*openPopover{op}}

	a.ime.sync(a.keyboardRouter(), true)
	if len(wire.updates) != 1 || !wire.updates[0].Enabled || wire.updates[0].Surrounding != "ab" {
		t.Fatalf("popover entry did not enable text input: %+v", wire.updates)
	}
	a.imeEvent(wlsession.IMEEvent{Commit: "中"})
	if popEntry.Text() != "ab中" {
		t.Errorf("commit landed as %q, want it in the popover entry", popEntry.Text())
	}
	if surf.frames == 0 || win.dirty {
		t.Errorf("repaint: popover frames %d, window dirty %v", surf.frames, win.dirty)
	}

	surf.dismissed = true
	a.ime.sync(a.keyboardRouter(), true)
	if last := wire.updates[len(wire.updates)-1]; last.Enabled {
		t.Error("dismissing the popover left text input enabled over a label")
	}
}
