// The allocation budgets (#77): hard ceilings derived from the measured
// baseline in bench/baseline.md, gated here so a regression fails `just
// check`. Each budget carries 1.2x headroom over the baseline number,
// not invented room: a fix that lowers the baseline should tighten it
// here in the same commit.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/testbuild"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

var budgetNow = time.Unix(1_700_000_000, 0).UTC()

func quietAllocBudget(tb testing.TB) {
	tb.Helper()
	testbuild.SkipAllocBudget(tb)
	sysfont.Warm(tb.Context())
}

// idleBudgetApp builds an application over a zero-value session whose
// single window is a wired fake host: enough loop machinery to run one
// real tick, no compositor.
func idleBudgetApp(tb testing.TB) *Application {
	tb.Helper()
	animclock.Reset()
	a := NewApplication(&wlsession.Session{})
	a.step = func() error { return nil }
	a.wake = func(time.Duration) {}
	a.windows = append(a.windows, budgetWindow(a, &fakeHost{w: 640, h: 470}))
	return a
}

// TestAllocBudgetIdleLoopTick pins the parked loop's steady tick: an
// idle pass over one wired window — pump, repeat check, tooltip state,
// window bookkeeping, wake computation, one dispatch — must allocate
// nothing. The strace baseline (bench/baseline.md) shows the real loop
// parks at zero syscalls when idle; this is its in-process
// counterpart. The baseline measured 0 allocs/tick, so the budget
// stays at 0.
func TestAllocBudgetIdleLoopTick(t *testing.T) {
	quietAllocBudget(t)
	a := idleBudgetApp(t)
	tick := func() {
		if err := a.tick(a.step, budgetNow); err != nil {
			panic(err)
		}
	}
	tick() // warm: tooltip bookkeeping, icon maps, theme stamps
	const budget = 0
	allocs := testing.AllocsPerRun(100, tick)
	if allocs > budget {
		t.Errorf("idle loop tick allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetKeyDelivery pins one key press through the focused
// window with both the window- and app-level OnKey hooks installed:
// the pair is precomputed at install, so the delivery itself must
// allocate nothing.
func TestAllocBudgetKeyDelivery(t *testing.T) {
	quietAllocBudget(t)
	a := idleBudgetApp(t)
	a.windows[0].cfg.onKey = func(*widget.Router, uint32, wlsession.Mods) {}
	a.OnKey(func(*widget.Router, uint32, wlsession.Mods) {})
	deliver := func() { a.deliverKey(a.sess, 30, 0) }
	deliver()
	const budget = 0
	allocs := testing.AllocsPerRun(100, deliver)
	if allocs > budget {
		t.Errorf("key delivery allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetIconReap pins the idle reap over a many-window
// application with no icons configured: with nothing to reap the pass
// must early out, maps included.
func TestAllocBudgetIconReap(t *testing.T) {
	quietAllocBudget(t)
	a := idleBudgetApp(t)
	for range 29 {
		a.windows = append(a.windows, &hostWindow{
			router: &widget.Router{Root: widget.NewBox(widget.Column, 8, 0)},
		})
	}
	a.reapWindowIcons()
	const budget = 0
	allocs := testing.AllocsPerRun(100, a.reapWindowIcons)
	if allocs > budget {
		t.Errorf("icon reap allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetFileWatchDrain pins one readiness pass of a file
// watch: the inotify buffer lives on the watch, so draining one
// event's worth of writes allocates nothing.
func TestAllocBudgetFileWatchDrain(t *testing.T) {
	quietAllocBudget(t)
	path := filepath.Join(t.TempDir(), "watched")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	w := &fileWatch{fd: fd, dirs: map[int]string{}, buf: make([]byte, 64*(unix.SizeofInotifyEvent+unix.NAME_MAX+1))}
	w.add(path)
	out, err := unix.Open(path, unix.O_WRONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(out) })
	one := []byte("y")
	tick := func() {
		if _, err := unix.Write(out, one); err != nil {
			t.Fatal(err)
		}
		if !w.drain() {
			t.Fatal("watched write was not a change")
		}
	}
	tick()
	const budget = 0
	allocs := testing.AllocsPerRun(100, tick)
	if allocs > budget {
		t.Errorf("file watch drain allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetToastDamageWalk pins the damage walk over a window
// tree wrapped by a visible toast layer: the layer's Children view is
// cached, so a walk with a toast on screen allocates nothing.
func TestAllocBudgetToastDamageWalk(t *testing.T) {
	quietAllocBudget(t)
	layer := &toastLayer{root: widget.NewBox(widget.Row, 0, 0)}
	layer.add(widget.NewToast(testFace(t), "Saved", 0))
	widget.CollectDamage(layer)
	const budget = 0
	allocs := testing.AllocsPerRun(100, func() { widget.CollectDamage(layer) })
	if allocs > budget {
		t.Errorf("toast damage walk allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetWindowFrame pins one damage-restricted frame over
// the paint harness (a nudge and the draw it owes): the device-rect
// scratch is per window, so the frame allocates nothing.
func TestAllocBudgetWindowFrame(t *testing.T) {
	quietAllocBudget(t)
	root := widget.NewBox(widget.Column, 8, 8)
	progress := widget.NewProgressBar(0)
	root.Append(widget.NewBox(widget.Row, 0, 0).Append(progress, false), false)
	h := newPaintHarness(root, 320, 200)
	h.warm(func() { progress.SetValue(progress.Value() + 0.05) }, 3)
	base := widget.DamageWalks()
	frame := func() {
		progress.SetValue(progress.Value() + 0.05)
		h.frame()
	}
	frame()
	if got := widget.DamageWalks(); got == base {
		t.Error("an invalidating frame ran no damage walk")
	}
	const budget = 0
	allocs := testing.AllocsPerRun(50, frame)
	if allocs > budget {
		t.Errorf("window frame allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetPumpScopedDamageWalk pins the pump-work rule: a
// drained Invoke that touched no tree must not cost a CollectDamage
// walk, while one that invalidates still does.
func TestAllocBudgetPumpScopedDamageWalk(t *testing.T) {
	a := idleBudgetApp(t)
	widget.CollectDamage(a.windows[0].router.Root)
	tick := func() {
		if err := a.tick(a.step, budgetNow); err != nil {
			panic(err)
		}
	}
	a.Invoke(func() {})
	tick()
	base := widget.DamageWalks()
	a.Invoke(func() {})
	tick()
	if got := widget.DamageWalks(); got != base {
		t.Errorf("a no-op Invoke ran %d damage walk(s), want none", got-base)
	}
}

// fakeTipPainter is a tooltip painter double counting passes.
type fakeTipPainter struct {
	owes   bool
	passes int
}

func (f *fakeTipPainter) Owes() bool                      { return f.owes }
func (f *fakeTipPainter) Pass() (bool, error)             { f.passes++; return true, nil }
func (f *fakeTipPainter) TakeFrame() (uint32, bool, bool) { return 0, false, false }

// TestAllocBudgetTooltipPainterPass pins the open-tooltip painter
// gate: a settled tooltip runs no Pass, an owed frame runs exactly
// one.
func TestAllocBudgetTooltipPainterPass(t *testing.T) {
	target := newTipTarget("hover text")
	target.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
	router := &widget.Router{Root: target}
	router.Move(widget.Point{X: 10, Y: 10})
	p := &fakeTipPainter{owes: true}
	tip := &tooltipCtl{}
	base := time.Now()
	opener := func(widget.Widget, string) (tooltipWindow, tooltipPainter) {
		return &stub{}, p
	}
	tip.update(router, base, opener)
	if p.passes != 0 {
		t.Fatalf("arrival ran %d passes before any dwell", p.passes)
	}
	tip.update(router, base.Add(time.Second), opener)
	if p.passes == 0 {
		t.Fatal("the opening pass never painted")
	}
	p.owes = false
	tip.update(router, base, opener)
	if p.passes != 1 {
		t.Errorf("a settled tooltip ran %d passes, want 1 (the opener's)", p.passes)
	}
	p.owes = true
	tip.update(router, base.Add(time.Millisecond), opener)
	if p.passes != 2 {
		t.Error("an owed tooltip frame was skipped")
	}
}

func TestAllocBudgetFocusedTextAreaTick(t *testing.T) {
	quietAllocBudget(t)
	a := idleBudgetApp(t)
	wire := &fakeIMEWire{available: true}
	a.ime = newIMEController(wire)
	area := widget.NewTextArea(imeBudgetFace(t), 14, render.RGB(255, 255, 255))
	area.SetText(strings.Repeat("the quick brown fox jumps over the lazy dog\n", 5000))
	widget.SizeWithin(area, widget.Size{W: 600, H: 400})
	area.Arrange(render.Rect{W: 600, H: 400})
	w := a.windows[0]
	w.router.Root = area
	if err := w.router.SetFocus(area); err != nil {
		t.Fatal(err)
	}
	tick := func() {
		if err := a.tick(a.step, budgetNow); err != nil {
			panic(err)
		}
	}
	tick()
	tick()
	pushed := len(wire.updates)
	if pushed != 1 || !wire.updates[0].Enabled {
		t.Fatalf("focus pushed %d updates, want one enabling", pushed)
	}
	if n := len(wire.updates[0].Surrounding); n > wlsession.MaxIMESurrounding {
		t.Errorf("pushed %d bytes of surrounding text, the protocol allows %d", n, wlsession.MaxIMESurrounding)
	}
	const budget = 0
	if allocs := testing.AllocsPerRun(100, tick); allocs > budget {
		t.Errorf("a tick with a focused text area allocates %v allocs/op, budget %d", allocs, budget)
	}
	if len(wire.updates) != pushed {
		t.Errorf("nothing changed, yet %d more updates went out", len(wire.updates)-pushed)
	}
}

func imeBudgetFace(tb testing.TB) *render.Typeface {
	tb.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		tb.Fatal(err)
	}
	return face
}
