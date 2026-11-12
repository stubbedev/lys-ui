// Window lifecycle coverage for the buffer pool: closing a window must
// hand its pool's storage back to the session arena - the teardown that
// used to be missing entirely, leaking every mapped buffer on close.
// Drives the real draw pipeline over fake buffers, no compositor.
package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/widget"
)

func TestWindowCloseReturnsPool(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	root.Append(widget.NewProgressBar(0), false)
	h := newPaintHarness(root, 320, 200)

	// A couple of frames so the rotation holds real, partly busy state.
	h.frame()
	h.frame()

	// The compositor returned every buffer; then the window closes.
	for _, b := range h.bufs {
		b.Release()
	}
	if h.wnd.pool == nil {
		t.Fatal("harness window has no pool")
	}
	h.wnd.release()
	if got := h.wnd.pool.Pending(); got != 0 {
		t.Errorf("window closed with %d pending buffers: pool storage was not returned", got)
	}
}

func TestWindowCloseWithHeldBuffersDefersThem(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 320, 200)
	h.frame()
	h.wnd.dirty = true
	if !h.wnd.draw() { // one buffer is now handed out (busy)
		t.Fatal(h.wnd.drawErr)
	}

	h.wnd.release()
	if got := h.wnd.pool.Pending(); got != 1 {
		t.Fatalf("pending = %d, want the one buffer the compositor still holds", got)
	}
	for _, b := range h.bufs {
		b.Release() // the release lands after the window is gone
	}
	if got := h.wnd.pool.Pending(); got != 0 {
		t.Errorf("pending = %d after post-close release, want 0", got)
	}
}

// A held buffer whose release never comes back - sway does not release
// the buffer a destroyed surface showed - returns with the compositor's
// answer to the sync the close sends; until then it stays pending.
func TestWindowCloseReclaimsHeldBuffersAfterTheSync(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 320, 200)
	var answer func()
	h.wnd.afterSync = func(fn func()) error { answer = fn; return nil }
	h.frame()
	h.wnd.dirty = true
	if !h.wnd.draw() {
		t.Fatal(h.wnd.drawErr)
	}
	h.wnd.release()
	if got := h.wnd.pool.Pending(); got != 1 || answer == nil {
		t.Fatalf("pending = %d, sync sent %v; want the held buffer waiting on a sync", got, answer != nil)
	}
	answer()
	if got := h.wnd.pool.Pending(); got != 0 {
		t.Errorf("pending = %d after the sync's answer, want 0", got)
	}
}

// A window whose buffers all came back sends no sync at close.
func TestWindowCloseWithNothingHeldSendsNoSync(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 320, 200)
	h.wnd.afterSync = func(func()) error { t.Error("a sync was sent with nothing held"); return nil }
	h.frame()
	for _, b := range h.bufs {
		b.Release()
	}
	h.wnd.release()
}

func budgetWindow(a *Application, host *fakeHost) *hostWindow {
	router := &widget.Router{Root: widget.NewBox(widget.Column, 8, 0)}
	return &hostWindow{
		host: host, sess: a.sess, router: router, anim: &a.anim,
		input: &surfaceInput{router: router}, tip: &tooltipCtl{since: time.Now()},
		frac120: 120, scale: 1, lastW: host.w, lastH: host.h,
		pool: buffer.New(func() (*buffer.Buffer, error) { return fakeBuffer(host.w, host.h), nil }, 3),
	}
}

func TestLoopDropsAClosedWindowAndItsSurface(t *testing.T) {
	a := idleBudgetApp(t)
	open := a.windows[0].host.(*fakeHost)
	closing := &fakeHost{w: 100, h: 100, closed: true}
	reopened := &fakeHost{w: 100, h: 100}
	w := budgetWindow(a, closing)
	w.sc = &fakeScaleWire{}
	w.cfg.onClosed = func() { a.windows = append(a.windows, budgetWindow(a, reopened)) }
	a.windows = append(a.windows, w)
	backing := a.windows[:cap(a.windows)]

	if err := a.tick(a.step, time.Now()); err != nil {
		t.Fatal(err)
	}

	if closing.destroyed != 1 {
		t.Errorf("closed surface destroyed %d times, want once", closing.destroyed)
	}
	if got := w.sc.(*fakeScaleWire).destroyed; got != 1 {
		t.Errorf("scale objects destroyed %d times, want once", got)
	}
	if open.destroyed != 0 {
		t.Error("an open window's surface was destroyed")
	}
	if len(a.windows) != 2 || a.windows[0].host != open || a.windows[1].host != reopened {
		t.Errorf("windows after the tick = %d, want the open one and the one onClosed opened", len(a.windows))
	}
	for _, kept := range backing {
		if kept == w {
			t.Error("the window list's backing array still holds the closed window")
		}
	}
}
