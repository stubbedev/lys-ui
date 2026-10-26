package app

import (
	"errors"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// fakePopSurface records the loop's calls on a popover's popup.
type fakePopSurface struct {
	destroyed, dismissed bool
	frames               int
	// w, h is the requested size; resizes records each Resize.
	w, h    int
	resizes [][2]int
}

func (f *fakePopSurface) Destroyed() bool            { return f.destroyed }
func (f *fakePopSurface) Dismissed() bool            { return f.dismissed }
func (f *fakePopSurface) MarkFrame()                 { f.frames++ }
func (f *fakePopSurface) RequestedSize() (int, int)  { return f.w, f.h }
func (f *fakePopSurface) Gutter() int                { return 2 }
func (f *fakePopSurface) Position() (int, int, bool) { return 0, 0, true }
func (f *fakePopSurface) Resize(w, h int) bool {
	f.resizes = append(f.resizes, [2]int{w, h})
	f.w, f.h = w, h
	return true
}

// fakePopPainter records passes.
type fakePopPainter struct {
	passes, closes int
	err            error
}

func (f *fakePopPainter) Pass() (bool, error) { f.passes++; return true, f.err }
func (f *fakePopPainter) Close()              { f.closes++ }

func newLoopPopover(content widget.Widget) (*openPopover, *fakePopSurface, *fakePopPainter, *int) {
	surf, painter := &fakePopSurface{}, &fakePopPainter{}
	root := &popoverKeyRoot{onDismiss: func() {}, content: content}
	closed := 0
	op := &openPopover{
		host: &fakeHost{w: 100, h: 100}, pop: surf, painter: painter,
		router: &widget.Router{Root: root}, keyRoot: root,
		popover: &Popover{}, detach: func() {}, fireClosed: func() { closed++ }, unanchor: func() {},
	}
	return op, surf, painter, &closed
}

func TestDrivePopoversPaintsOnDamageAndReapsDestroyed(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	label := widget.NewLabel(face, 14, "a", render.RGB(255, 255, 255))
	op, surf, painter, closed := newLoopPopover(label)
	op.keyRoot.Arrange(render.Rect{W: 100, H: 30})
	widget.CollectDamage(op.keyRoot)
	a := &Application{openPopovers: []*openPopover{op}}
	a.popovers.openOrReplace(op.host, op.popover)

	// Nothing changed: a pass, no repaint request.
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if painter.passes != 1 || surf.frames != 0 {
		t.Fatalf("idle: passes %d frames %d", painter.passes, surf.frames)
	}
	// Loop work ran (an Invoke): the content changed, so it repaints.
	label.SetText("b")
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if surf.frames != 1 {
		t.Errorf("damaged content: frames %d, want a repaint", surf.frames)
	}
	// A pass with nothing damaged (loop work that left the content
	// alone, a timer elsewhere) repaints nothing (#327).
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if surf.frames != 1 {
		t.Errorf("undamaged pass: frames %d, want no repaint", surf.frames)
	}
	// The exit tween landed: reaped, closed once, registry cleared.
	surf.destroyed = true
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if len(a.openPopovers) != 0 || painter.closes != 1 || *closed != 1 || a.popovers.get(op.host) != nil {
		t.Errorf("reap: %d open, %d closes, %d closed, registry %v", len(a.openPopovers), painter.closes, *closed, a.popovers.get(op.host))
	}
	if err := a.drivePopovers(); err != nil || painter.passes != 3 {
		t.Errorf("a reaped popover was driven again: %d passes", painter.passes)
	}
}

func TestDrivePopoversSurfacesPaintErrors(t *testing.T) {
	op, _, painter, _ := newLoopPopover(widget.NewSpacer(1, 1))
	painter.err = errors.New("wire")
	a := &Application{openPopovers: []*openPopover{op}}
	if err := a.drivePopovers(); err == nil {
		t.Error("a paint failure was swallowed")
	}
}

// A focused Entry inside a popover takes typed text and editing keys;
// Esc dismisses anyway; without focus, actions reach the content.
func TestPopoverKeysReachAFocusedEntry(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	entry := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	op, surf, _, _ := newLoopPopover(entry)
	dismissed := 0
	op.keyRoot.onDismiss = func() { dismissed++ }
	op.keyRoot.Arrange(render.Rect{W: 200, H: 30})
	tr := &fakeTranslator{
		text: map[uint32]string{30: "a", 48: "b"},
		syms: map[uint32]xkb.Keysym{30: xkb.Keysym('a'), 48: xkb.Keysym('b'), 14: xkb.KeyBackSpace, 1: xkb.KeyEscape},
	}
	a := &Application{accels: newAccelTable()}

	// Unfocused: a printable key does not type.
	a.deliverPopoverKey(tr, op, 30, 0)
	if entry.Text() != "" {
		t.Fatalf("unfocused entry got %q", entry.Text())
	}
	// Click to focus, then type and edit.
	op.router.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5}, 0)
	op.router.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	a.deliverPopoverKey(tr, op, 30, 0)
	a.deliverPopoverKey(tr, op, 48, 0)
	a.deliverPopoverKey(tr, op, 14, 0)
	if entry.Text() != "a" {
		t.Errorf("typed text = %q, want a", entry.Text())
	}
	if surf.frames < 3 {
		t.Errorf("keys did not request repaints: %d", surf.frames)
	}
	a.deliverPopoverKey(tr, op, 1, 0)
	if dismissed != 1 {
		t.Errorf("Esc with a focused entry: dismissed %d", dismissed)
	}
}

func TestKeyPopoverIsTheNewestLiveOne(t *testing.T) {
	older, oldSurf, _, _ := newLoopPopover(widget.NewSpacer(1, 1))
	newer, newSurf, _, _ := newLoopPopover(widget.NewSpacer(1, 1))
	a := &Application{}
	if a.keyPopover() != nil {
		t.Error("no popovers: a key target")
	}
	a.openPopovers = []*openPopover{older, newer}
	if a.keyPopover() != newer {
		t.Error("the newest popover does not hold the keyboard")
	}
	newSurf.dismissed = true
	if a.keyPopover() != older {
		t.Error("a dismissing popover still takes keys")
	}
	oldSurf.dismissed = true
	if a.keyPopover() != nil {
		t.Error("all dismissing: still a key target")
	}
}

func TestPumpReportsWork(t *testing.T) {
	a := testApp(nil)
	if a.pump(time.Now()) {
		t.Error("an empty pump reported work")
	}
	a.Invoke(func() {})
	if !a.pump(time.Now()) {
		t.Error("an Invoke did not report work")
	}
}

func TestPopoverSetFocusRoutesThroughItsRouter(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	a, b := widget.NewEntry(face, 13, 0), widget.NewEntry(face, 13, 0)
	content := widget.NewBox(widget.Column, 0, 0)
	content.Append(a, false)
	content.Append(b, false)
	op, surf, _, _ := newLoopPopover(content)
	p := op.popover
	p.focus = func(w widget.Widget) { op.router.SetFocus(w); op.pop.MarkFrame() }
	p.SetFocus(b)
	if op.router.Focused() != b || surf.frames != 1 {
		t.Errorf("focused %v frames %d, want b and a repaint", op.router.Focused(), surf.frames)
	}
	p.markClosed()
	p.SetFocus(a)
	if op.router.Focused() != b {
		t.Error("a closed popover moved focus")
	}
	var none *Popover
	none.SetFocus(a) // nil-safe
}

// TestPopoverClickReachesTheWidgetUnderThePointer pins the popover's
// pointer routing: a press and release over a button inside the
// content click that button. Regression: the root's hit test answered
// the whole content, so no widget inside any popover ever got a click.
func TestPopoverClickReachesTheWidgetUnderThePointer(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	clicks := 0
	button := widget.NewButton(widget.NewLabel(face, 14, "go", render.RGB(255, 255, 255)), 4, 4)
	button.OnClick = func() { clicks++ }
	content := widget.NewBox(widget.Column, 0, 10)
	content.Append(widget.NewLabel(face, 14, "title", render.RGB(255, 255, 255)), false)
	content.Append(button, false)
	op, _, _, _ := newLoopPopover(content)
	op.keyRoot.Measure(widget.Constraints{Max: widget.Size{W: 200, H: 200}})
	op.keyRoot.Arrange(render.Rect{W: 200, H: 200})

	b := button.Bounds()
	at := widget.Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
	if hit := op.keyRoot.HitTest(at); hit != widget.Widget(button) {
		t.Fatalf("hit %T, want the button", hit)
	}
	op.router.Move(at)
	op.router.Press(widget.BTNLeft, at, 0)
	op.router.Release(widget.BTNLeft, at)
	if clicks != 1 {
		t.Errorf("clicks = %d, want the button clicked once", clicks)
	}
	// Off the button: nothing clicks; off the content, the root answers.
	op.router.Press(widget.BTNLeft, widget.Point{X: 2, Y: 2}, 0)
	op.router.Release(widget.BTNLeft, widget.Point{X: 2, Y: 2})
	if clicks != 1 {
		t.Error("a press off the button clicked it")
	}
	if hit := op.keyRoot.HitTest(widget.Point{X: 500, Y: 500}); hit != widget.Widget(op.keyRoot) {
		t.Errorf("off-content hit = %T, want the root", hit)
	}
}

// TestNestedPopoverChain pins the chain bookkeeping: a parent's close
// tears its children down deepest first through their own teardown, a
// child that closes on its own leaves the parent's list, and Root and
// Child walk the links.
func TestNestedPopoverChain(t *testing.T) {
	var order []string
	mk := func(name string, parent *Popover) *Popover {
		p := &Popover{parent: parent}
		p.teardown = func() {
			order = append(order, name)
			p.closeChildren()
		}
		if parent != nil {
			parent.children = append(parent.children, p)
		}
		return p
	}
	root := mk("root", nil)
	mid := mk("mid", root)
	leaf := mk("leaf", mid)
	if leaf.Root() != root || root.Child() != mid || mid.Child() != leaf || leaf.Child() != nil || mid.Parent() != root {
		t.Fatal("chain links")
	}
	root.closeChildren()
	if len(order) != 2 || order[0] != "mid" || order[1] != "leaf" || root.Child() != nil {
		t.Errorf("teardown order = %v", order)
	}
	// A child closing on its own leaves its parent alone.
	other := mk("other", root)
	root.dropChild(other)
	if root.Child() != nil {
		t.Error("a closed child stayed listed")
	}
}

// TestDrivePopoversReapsChildrenFirst pins the wire order: a nested
// popover destroyed in the same pass as its parent goes first, so the
// parent never leaves while a child of it is still mapped.
func TestDrivePopoversReapsChildrenFirst(t *testing.T) {
	var order []string
	mk := func(name string) *openPopover {
		op, surf, _, _ := newLoopPopover(widget.NewBox(widget.Column, 0, 0))
		surf.destroyed = true
		op.fireClosed = func() { order = append(order, name) }
		return op
	}
	parent, child := mk("parent"), mk("child")
	a := &Application{openPopovers: []*openPopover{parent, child}}
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "child" || order[1] != "parent" || len(a.openPopovers) != 0 {
		t.Errorf("reaped %v", order)
	}
	// Survivors keep their order.
	keep1, _, _, _ := newLoopPopover(widget.NewBox(widget.Column, 0, 0))
	keep2, _, _, _ := newLoopPopover(widget.NewBox(widget.Column, 0, 0))
	a.openPopovers = []*openPopover{keep1, keep2}
	_ = a.drivePopovers()
	if a.openPopovers[0] != keep1 || a.openPopovers[1] != keep2 {
		t.Error("live popovers were reordered")
	}
}

// TestHeldModsReadsTheSession pins HeldMods to the session's keyboard
// state: nothing held is no modifiers.
func TestHeldModsReadsTheSession(t *testing.T) {
	a := &Application{sess: &wlsession.Session{}}
	if a.HeldMods() != 0 {
		t.Errorf("held = %v with no keyboard state", a.HeldMods())
	}
}

// TestDrivePopoversFitsGrowingContent pins the resize: content whose
// natural size changed (a menu re-published with more rows) asks the
// popup for the new size plus its gutter, once; unchanged content and
// a dismissed popover ask nothing.
func TestDrivePopoversFitsGrowingContent(t *testing.T) {
	box := widget.NewBox(widget.Column, 0, 0)
	box.Append(widget.NewSpacer(40, 20), false)
	op, surf, _, _ := newLoopPopover(box)
	surf.w, surf.h = 44, 24 // 40x20 plus the gutter
	op.keyRoot.Arrange(render.Rect{W: 40, H: 20})
	widget.CollectDamage(op.keyRoot)
	a := &Application{openPopovers: []*openPopover{op}}

	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if len(surf.resizes) != 0 {
		t.Fatalf("unchanged content resized: %v", surf.resizes)
	}
	box.Append(widget.NewSpacer(60, 30), false)
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if len(surf.resizes) != 1 || surf.resizes[0] != [2]int{64, 54} {
		t.Fatalf("grown content: resizes %v, want one to 64x54", surf.resizes)
	}
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if len(surf.resizes) != 1 {
		t.Errorf("the same size was asked for again: %v", surf.resizes)
	}
	surf.dismissed = true
	box.Append(widget.NewSpacer(10, 10), false)
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if len(surf.resizes) != 1 {
		t.Errorf("a dismissed popover resized: %v", surf.resizes)
	}
}

// The content's own size request floors the popover, as GTK honors a
// popover's width and height requests: content smaller than the
// request fits the request, and content that outgrows it grows past.
func TestPopoverFitHonorsTheContentsSizeRequest(t *testing.T) {
	box := widget.NewBox(widget.Column, 0, 0)
	box.Append(widget.NewSpacer(40, 20), false)
	op, surf, _, _ := newLoopPopover(box)
	surf.w, surf.h = 44, 24
	op.keyRoot.Arrange(render.Rect{W: 40, H: 20})
	widget.CollectDamage(op.keyRoot)
	a := &Application{openPopovers: []*openPopover{op}}
	box.SetSizeRequest(100, 80)
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	// The fake surface's gutter is 2px a side.
	if got := surf.resizes[len(surf.resizes)-1]; got != [2]int{104, 84} {
		t.Errorf("small content fits %v, want the 100x80 request plus the gutter", got)
	}
	op.keyRoot.Arrange(render.Rect{W: 100, H: 80})
	widget.CollectDamage(op.keyRoot)
	// The fake host's 100px caps the width; the height grows past the
	// request.
	box.Append(widget.NewSpacer(90, 100), false)
	if err := a.drivePopovers(); err != nil {
		t.Fatal(err)
	}
	if got := surf.resizes[len(surf.resizes)-1]; got != [2]int{104, 124} {
		t.Errorf("grown content fits %v, want 100x120 plus the gutter", got)
	}
}

// A shrinking popover keeps its surface while the content's layout
// motion carries it down, then resizes once to the final size; a
// growing one grows at once (#225).
func TestPopoverShrinkWaitsForTheContent(t *testing.T) {
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	content := widget.NewBox(widget.Column, 0, 0)
	content.SetLayoutMotion(widget.LayoutMotionSmooth)
	content.Append(widget.NewSpacer(50, 60), false)
	op, surf, _, _ := newLoopPopover(content)
	surf.w, surf.h = 54, 64
	a := &Application{openPopovers: []*openPopover{op}}
	a.popovers.openOrReplace(op.host, op.popover)
	buf := make([]byte, render.Stride(100)*100)
	cv := render.New(buf, render.Stride(100), 100, 100)
	frame := func() {
		t.Helper()
		if err := a.drivePopovers(); err != nil {
			t.Fatal(err)
		}
		op.keyRoot.Arrange(render.Rect{X: 2, Y: 2, W: surf.w - 4, H: surf.h - 4})
		widget.CollectDamage(op.keyRoot)
		op.keyRoot.Paint(cv)
	}
	frame()
	frame()
	content.Clear()
	content.Append(widget.NewSpacer(50, 20), false)
	frame()
	if len(surf.resizes) != 0 {
		t.Fatalf("the surface resized at once to %v", surf.resizes)
	}
	for range 60 {
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
		frame()
		if widget.ShownRect(content).H > 20 && len(surf.resizes) != 0 {
			t.Fatalf("the surface resized to %v while the content was still shown %v", surf.resizes, widget.ShownRect(content))
		}
	}
	if len(surf.resizes) != 1 || surf.resizes[0] != [2]int{54, 24} {
		t.Errorf("resizes %v, want one, to 54x24, after the content landed", surf.resizes)
	}
	content.Append(widget.NewSpacer(50, 40), false)
	frame()
	if last := surf.resizes[len(surf.resizes)-1]; last != [2]int{54, 64} {
		t.Errorf("growing resized to %v, want 54x64 at once", last)
	}
}
