package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

type firstFrameRig struct {
	t    *testing.T
	a    *Application
	h    *paintHarness
	path string
	held []func()
	now  time.Time
}

func newFirstFrameRig(t *testing.T) *firstFrameRig {
	t.Helper()
	r := &firstFrameRig{t: t, now: time.Unix(1000, 0)}
	r.path = filepath.Join(t.TempDir(), "eye.svg")
	if err := os.WriteFile(r.path, []byte(scaleTestSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(widget.NewThemeIcon(r.path, 16), false)
	r.h = newPaintHarness(root, 64, 48)
	r.a = NewTestApplication(t, &wlsession.Session{})
	r.a.step = func() error { return nil }
	r.a.wake = func(time.Duration) {}
	r.a.arm = func(time.Time) {}
	r.a.windows = []*hostWindow{r.h.wnd}
	t.Cleanup(widget.SetInvoker(func(fn func()) { r.held = append(r.held, fn) }))
	r.h.wnd.rescale(120)
	widget.WaitAssetLoads()
	if len(r.held) == 0 {
		t.Fatal("the tree asked for no icon before its first frame")
	}
	return r
}

func (r *firstFrameRig) tick(after time.Duration) {
	r.t.Helper()
	r.now = r.now.Add(after)
	if err := r.a.tick(r.a.step, r.now); err != nil {
		r.t.Fatalf("tick: %v", err)
	}
}

func (r *firstFrameRig) answer() {
	held := r.held
	r.held = nil
	for _, fn := range held {
		fn()
	}
}

func TestAWindowsFirstFrameWaitsForTheIconsItsTreeAskedFor(t *testing.T) {
	r := newFirstFrameRig(t)
	r.tick(0)
	if got := r.h.surf.commits; got != 0 {
		t.Fatalf("the first frame went out with %d commit(s) while its icon was still owed", got)
	}
	r.answer()
	r.tick(10 * time.Millisecond)
	if got := r.h.surf.commits; got != 1 {
		t.Fatalf("%d commits once the icon landed, want the first frame", got)
	}
	if !widget.ThemeIconPreloaded(r.path, 16, 120, 120) {
		t.Error("the first frame went out without the icon decoded")
	}
}

func TestAWindowsFirstFrameWaitsForAnIconOnlyAsLongAsItsBound(t *testing.T) {
	r := newFirstFrameRig(t)
	r.tick(0)
	r.tick(firstFrameAssetWait - time.Millisecond)
	if got := r.h.surf.commits; got != 0 {
		t.Fatalf("%d commits inside the bound with the icon still owed", got)
	}
	r.tick(2 * time.Millisecond)
	if got := r.h.surf.commits; got != 1 {
		t.Errorf("%d commits past the bound, want the first frame without the icon", got)
	}
}

func TestALaterFrameDoesNotWaitForAnIcon(t *testing.T) {
	r := newFirstFrameRig(t)
	r.answer()
	r.tick(0)
	if r.h.surf.commits != 1 {
		t.Fatalf("%d commits for the first frame", r.h.surf.commits)
	}
	root := r.h.wnd.router.Root.(*widget.Box)
	second := filepath.Join(t.TempDir(), "power.svg")
	if err := os.WriteFile(second, []byte(scaleTestSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	root.Append(widget.NewThemeIcon(second, 16), false)
	widget.WaitAssetLoads()
	for _, b := range r.h.bufs {
		if b.Busy() {
			b.Release()
		}
	}
	r.h.wnd.framePending = false
	r.h.wnd.dirty = true
	r.tick(10 * time.Millisecond)
	if got := r.h.surf.commits; got != 2 {
		t.Errorf("%d commits: a shown window held a later frame for an icon", got)
	}
}

func TestAnotherWindowsIconsDoNotHoldAWindowsFirstFrame(t *testing.T) {
	r := newFirstFrameRig(t)
	plain := newPaintHarness(widget.NewBox(widget.Column, 0, 0), 64, 48)
	r.a.windows = append(r.a.windows, plain.wnd)
	r.tick(0)
	if got := plain.surf.commits; got != 1 {
		t.Errorf("a window with no icons made %d commits while another window's icon was owed, want its first frame", got)
	}
	if got := r.h.surf.commits; got != 0 {
		t.Errorf("the window whose icon is owed made %d commits", got)
	}
}
