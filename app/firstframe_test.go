package app

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
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
	held heldAnswers
	now  time.Time
}

type heldAnswers struct {
	mu  sync.Mutex
	fns []func()
}

func (h *heldAnswers) add(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fns = append(h.fns, fn)
}

func (h *heldAnswers) len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.fns)
}

func (h *heldAnswers) takeOne() (func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.fns) == 0 {
		return nil, false
	}
	fn := h.fns[0]
	h.fns = h.fns[1:]
	return fn, true
}

func (h *heldAnswers) takeAll() []func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	fns := h.fns
	h.fns = nil
	return fns
}

func newFirstFrameRig(t *testing.T) *firstFrameRig { return newFirstFrameRigOf(t, 1) }

// newFirstFrameRigOf is a window whose tree asks for icons icons before
// its first frame, path the first of them, their answers held.
func newFirstFrameRigOf(t *testing.T, icons int) *firstFrameRig {
	t.Helper()
	r := &firstFrameRig{t: t, now: time.Unix(1000, 0)}
	dir := t.TempDir()
	root := widget.NewBox(widget.Column, 0, 0)
	for i := range icons {
		path := filepath.Join(dir, "eye"+strconv.Itoa(i)+".svg")
		if err := os.WriteFile(path, []byte(scaleTestSVG), 0o600); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			r.path = path
		}
		root.Append(widget.NewThemeIcon(path, 16), false)
	}
	r.h = newPaintHarness(root, 64, 48)
	r.a = NewTestApplication(t, &wlsession.Session{})
	r.a.step = func() error { return nil }
	r.a.wake = func(time.Duration) {}
	r.a.arm = func(time.Time) {}
	r.a.windows = []*hostWindow{r.h.wnd}
	t.Cleanup(widget.SetInvoker(r.held.add))
	r.h.wnd.rescale(120)
	widget.WaitAssetLoads()
	if r.held.len() == 0 {
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

func (r *firstFrameRig) answerOne() {
	r.t.Helper()
	fn, ok := r.held.takeOne()
	if !ok {
		r.t.Fatal("no icon answer is held")
	}
	fn()
}

func (r *firstFrameRig) answer() {
	for _, fn := range r.held.takeAll() {
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
	r.tick(firstFrameStall - time.Millisecond)
	if got := r.h.surf.commits; got != 0 {
		t.Fatalf("%d commits inside the bound with the icon still owed", got)
	}
	r.tick(2 * time.Millisecond)
	if got := r.h.surf.commits; got != 1 {
		t.Errorf("%d commits past the bound, want the first frame without the icon", got)
	}
}

func TestEachIconThatLandsGivesTheFirstFrameAnotherStall(t *testing.T) {
	r := newFirstFrameRigOf(t, 3)
	r.tick(0)
	for range 2 {
		r.tick(firstFrameStall - time.Millisecond)
		r.answerOne()
		r.tick(0)
		if got := r.h.surf.commits; got != 0 {
			t.Fatalf("%d commits with an icon landing inside the stall and another still owed", got)
		}
	}
	r.tick(firstFrameStall - time.Millisecond)
	if got := r.h.surf.commits; got != 0 {
		t.Fatalf("%d commits inside the stall after the last landing", got)
	}
	r.answerOne()
	r.tick(0)
	if got := r.h.surf.commits; got != 1 {
		t.Errorf("%d commits once every icon landed, %s after the first usable pass, want the first frame", got, 3*(firstFrameStall-time.Millisecond))
	}
}

func TestIconsThatKeepLandingHoldTheFirstFrameOnlyToTheLimit(t *testing.T) {
	step := firstFrameStall / 2
	icons := int(firstFrameAssetLimit/step) + 2
	r := newFirstFrameRigOf(t, icons)
	r.tick(0)
	waited := time.Duration(0)
	for waited+step < firstFrameAssetLimit {
		r.tick(step)
		waited += step
		r.answerOne()
		r.tick(0)
		if got := r.h.surf.commits; got != 0 {
			t.Fatalf("%d commits %s into the wait, inside the limit with icons landing", got, waited)
		}
	}
	r.tick(firstFrameAssetLimit - waited)
	if got := r.h.surf.commits; got != 1 {
		t.Errorf("%d commits at the limit with %d icons still owed, want the first frame without them", got, r.held.len())
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
