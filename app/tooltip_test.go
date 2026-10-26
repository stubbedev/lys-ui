package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

func TestTooltipShouldOpen(t *testing.T) {
	base := time.Now()

	t.Run("dwell past the delay opens", func(t *testing.T) {
		if !tooltipShouldOpen(false, &widget.Label{}, "text", base, base.Add(tooltipDelay), tooltipDelay) {
			t.Error("resting hover with text did not open at the delay")
		}
	})

	t.Run("before the delay nothing opens", func(t *testing.T) {
		if tooltipShouldOpen(false, &widget.Label{}, "text", base, base.Add(tooltipDelay-time.Millisecond), tooltipDelay) {
			t.Error("opened before the dwell elapsed")
		}
	})

	t.Run("no text never opens", func(t *testing.T) {
		if tooltipShouldOpen(false, &widget.Label{}, "", base, base.Add(time.Hour), tooltipDelay) {
			t.Error("opened without tooltip text")
		}
	})

	t.Run("no hover never opens", func(t *testing.T) {
		if tooltipShouldOpen(false, nil, "text", base, base.Add(time.Hour), tooltipDelay) {
			t.Error("opened without a hovered widget")
		}
	})

	t.Run("an open tooltip blocks a second", func(t *testing.T) {
		if tooltipShouldOpen(true, &widget.Label{}, "text", base, base.Add(time.Hour), tooltipDelay) {
			t.Error("opened a second tooltip while one was up")
		}
	})
}

func TestTooltipShouldClose(t *testing.T) {
	t.Run("hover change closes", func(t *testing.T) {
		if !tooltipShouldClose(true, true, true) {
			t.Error("hover change kept the tooltip")
		}
	})

	t.Run("lost text closes", func(t *testing.T) {
		if !tooltipShouldClose(true, false, false) {
			t.Error("tooltip stayed after its text vanished")
		}
	})

	t.Run("nothing open closes nothing", func(t *testing.T) {
		if tooltipShouldClose(false, true, true) {
			t.Error("closed a tooltip that was not open")
		}
	})
}

// tipTarget is a hoverable widget carrying a tooltip, for router tests.
type tipTarget struct {
	bounds  render.Rect
	tooltip string
}

func newTipTarget(text string) *tipTarget { return &tipTarget{tooltip: text} }

func (t *tipTarget) Measure(con widget.Constraints) widget.Size {
	if con.Max.W < 8 {
		return con.Max
	}
	if con.Max.H < 8 {
		return con.Max
	}
	return widget.Size{W: 8, H: 8}
}

func (t *tipTarget) Arrange(r render.Rect)   { t.bounds = r }
func (t *tipTarget) Paint(cv *render.Canvas) {}

func (t *tipTarget) HitTest(p widget.Point) widget.Widget {
	if t.bounds.Contains(p.X, p.Y) {
		return t
	}
	return nil
}

func (t *tipTarget) TooltipText() string { return t.tooltip }

// stub is a popup double recording dismissals.
type stub struct{ closed int }

func (s *stub) Dismissed() bool { return s.closed > 0 }
func (s *stub) Dismiss()        { s.closed++ }

func TestTooltipCtlUpdate(t *testing.T) {
	target := newTipTarget("hover text")
	target.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
	router := &widget.Router{Root: target}

	var openStub *stub
	var openedText string
	opener := func(w widget.Widget, text string) (tooltipWindow, tooltipPainter) {
		openedText = text
		openStub = &stub{}
		return openStub, nil
	}

	ctl := &tooltipCtl{}
	base := time.Now()

	t.Run("arrival does not open early", func(t *testing.T) {
		router.Move(widget.Point{X: 10, Y: 10})
		ctl.update(router, base, opener)
		if openStub != nil {
			t.Error("opened without dwell")
		}
	})

	t.Run("dwell opens exactly once with the widget's text", func(t *testing.T) {
		ctl.update(router, base.Add(tooltipDelay), opener)
		if openStub == nil {
			t.Fatal("dwell did not open the tooltip")
		}
		if openedText != "hover text" {
			t.Errorf("opener text = %q, want the widget's tooltip", openedText)
		}
		first := openStub
		ctl.update(router, base.Add(2*tooltipDelay), opener)
		if openStub != first {
			t.Error("a second tooltip replaced the first while the hover held")
		}
	})

	t.Run("hover change closes and re-arms the dwell", func(t *testing.T) {
		router.Move(widget.Point{X: 10, Y: 200})
		ctl.update(router, base.Add(2*tooltipDelay+time.Second), opener)
		if openStub.closed != 1 {
			t.Errorf("closes = %d, want 1 after hover change", openStub.closed)
		}
		router.Move(widget.Point{X: 10, Y: 10})
		ctl.update(router, base.Add(3*tooltipDelay), opener)
		if openStub == nil || openStub.closed != 1 {
			t.Error("state lost across the hover change")
		}
	})
}

// Regression for the tooltip ghost class (tree mutation, issue #58):
// removing the hovered widget must dismiss the open tooltip anchored
// to it and never reopen one for the dead widget. The app installs the
// removal hook so a mutation drops the widget from the router's hover;
// tooltipCtl closes on the hover change like any other.
func TestTooltipGhostOnRemove(t *testing.T) {
	box := widget.NewBox(widget.Row, 0, 0)
	target := newTipTarget("hover text")
	box.Append(target, false)
	box.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 20}})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
	router := &widget.Router{Root: box}

	widget.SetRemovedHook(router.Forget)
	defer widget.SetRemovedHook(nil)

	var openStub *stub
	opener := func(w widget.Widget, text string) (tooltipWindow, tooltipPainter) {
		openStub = &stub{}
		return openStub, nil
	}
	ctl := &tooltipCtl{}
	base := time.Now()

	router.Move(widget.Point{X: 5, Y: 5})
	ctl.update(router, base, opener)
	ctl.update(router, base.Add(tooltipDelay), opener)
	if openStub == nil {
		t.Fatal("dwell did not open the tooltip")
	}

	box.Remove(target)
	ctl.update(router, base.Add(2*tooltipDelay), opener)
	if openStub.closed != 1 {
		t.Errorf("dismissals = %d, want 1: the tooltip outlived its widget", openStub.closed)
	}
	// No ghost dwell: the removed widget must never earn a new tooltip.
	ctl.update(router, base.Add(3*tooltipDelay), opener)
	if openStub.closed != 1 {
		t.Error("a tooltip reopened for a removed widget")
	}
}

// Regression: hover cursor management. The hovered widget decides the
// shape; everything else falls back to the arrow.
func TestCursorFor(t *testing.T) {
	entry := widget.NewEntry(testFace(t), 14, render.RGB(255, 255, 255))
	if got := cursorFor(entry); got != "xterm" {
		t.Errorf("entry cursor = %q, want xterm", got)
	}
	target := newTipTarget("t")
	if got := cursorFor(target); got != "" {
		t.Errorf("plain widget cursor = %q, want empty for the arrow", got)
	}
	if got := cursorFor(nil); got != "" {
		t.Errorf("nil hover cursor = %q, want empty", got)
	}
}

// A tooltip that follows the pointer within one widget (a calendar's
// per-day detail) changes its text without a hover change: the open
// tooltip closes and the new text gets its own dwell.
func TestTooltipCtlTextChange(t *testing.T) {
	target := newTipTarget("monday")
	target.Arrange(render.Rect{W: 100, H: 20})
	router := &widget.Router{Root: target}
	var opened []string
	var last *stub
	opener := func(_ widget.Widget, text string) (tooltipWindow, tooltipPainter) {
		opened = append(opened, text)
		last = &stub{}
		return last, nil
	}
	ctl := &tooltipCtl{}
	base := time.Now()
	router.Move(widget.Point{X: 10, Y: 10})
	ctl.update(router, base, opener)
	ctl.update(router, base.Add(tooltipDelay), opener)
	first := last
	target.tooltip = "tuesday"
	ctl.update(router, base.Add(tooltipDelay+time.Millisecond), opener)
	if first.closed != 1 || len(opened) != 1 {
		t.Fatalf("a text change: closes=%d opened=%v", first.closed, opened)
	}
	ctl.update(router, base.Add(2*tooltipDelay+time.Millisecond), opener)
	if len(opened) != 2 || opened[1] != "tuesday" {
		t.Errorf("opened %v, want the new text after its dwell", opened)
	}
}

// Zero options keep the defaults; set ones win, and the dwell follows.
func TestTooltipOptions(t *testing.T) {
	var o TooltipOptions
	if o.delay() != tooltipDelay {
		t.Error("default delay")
	}
	if o.gap() != tooltipGap {
		t.Error("default gap")
	}
	o = TooltipOptions{Delay: time.Second, Gap: 8, MaxWidth: 600}
	if w, h := o.maxSize(); w != 600 || h != tooltipMaxH {
		t.Errorf("max size %dx%d", w, h)
	}
	if o.gap() != 8 {
		t.Errorf("gap %d", o.gap())
	}
	tc := &tooltipCtl{delay: o.Delay}
	if tc.dwell() != time.Second || (&tooltipCtl{}).dwell() != tooltipDelay {
		t.Error("dwell")
	}
}

// Point and rect anchors, and the popover ceiling's defaults: the
// largest output, not the host window, bounds an unset width (#279)
// and an unset height (#322).
func TestAnchorsAndPopoverCeiling(t *testing.T) {
	if b := AnchorAt(30, 40).Bounds(); b != (render.Rect{X: 30, Y: 40, W: 1, H: 1}) {
		t.Errorf("AnchorAt = %+v", b)
	}
	r := render.Rect{X: 1, Y: 2, W: 3, H: 4}
	if AnchorRect(r).Bounds() != r {
		t.Error("AnchorRect")
	}
	outputs := []*wlsession.Output{{LogicalW: 1280, LogicalH: 800}, {LogicalW: 2560, LogicalH: 1440}}
	if got := popoverCeiling(outputs, 0, 0); got != (widget.Size{W: 2560, H: 1440}) {
		t.Errorf("default ceiling %+v, want the largest output's size", got)
	}
	if got := popoverCeiling(outputs, 300, 200); got != (widget.Size{W: 300, H: 200}) {
		t.Errorf("set ceiling %+v", got)
	}
}

func TestTooltipStaysHiddenAfterAPressUntilTheHoverChanges(t *testing.T) {
	target := newTipTarget("hover text")
	target.Arrange(render.Rect{W: 100, H: 20})
	router := &widget.Router{Root: target}
	var opened []*stub
	opener := func(widget.Widget, string) (tooltipWindow, tooltipPainter) {
		s := &stub{}
		opened = append(opened, s)
		return s, nil
	}
	ctl := &tooltipCtl{}
	at := time.Now()
	router.Move(widget.Point{X: 10, Y: 10})
	ctl.update(router, at, opener)
	at = at.Add(tooltipDelay)
	ctl.update(router, at, opener)
	if len(opened) != 1 {
		t.Fatalf("dwell opened %d tooltips, want 1", len(opened))
	}

	ctl.press()
	if opened[0].closed != 1 {
		t.Errorf("the press closed the open tooltip %d times, want 1", opened[0].closed)
	}
	at = at.Add(10 * tooltipDelay)
	ctl.update(router, at, opener)
	if len(opened) != 1 {
		t.Errorf("the tooltip reopened over the pressed widget: %d opens", len(opened))
	}
	if _, pending := ctl.next(); pending {
		t.Error("a suppressed tooltip still schedules a wake")
	}

	router.Move(widget.Point{X: 10, Y: 200})
	ctl.update(router, at, opener)
	router.Move(widget.Point{X: 10, Y: 10})
	ctl.update(router, at, opener)
	ctl.update(router, at.Add(tooltipDelay), opener)
	if len(opened) != 2 {
		t.Errorf("a new dwell after the pointer left and came back opened %d tooltips in all, want 2", len(opened))
	}
}

func TestTooltipOpenedAcrossAPressIsDismissed(t *testing.T) {
	target := newTipTarget("hover text")
	target.Arrange(render.Rect{W: 100, H: 20})
	router := &widget.Router{Root: target}
	ctl := &tooltipCtl{}
	var opened *stub
	opener := func(widget.Widget, string) (tooltipWindow, tooltipPainter) {
		opened = &stub{}
		ctl.press()
		return opened, nil
	}
	at := time.Now()
	router.Move(widget.Point{X: 10, Y: 10})
	ctl.update(router, at, opener)
	ctl.update(router, at.Add(tooltipDelay), opener)
	if opened == nil || opened.closed != 1 || ctl.open != nil {
		t.Errorf("a tooltip whose open saw a press stayed up: opened %v, open %v", opened, ctl.open)
	}
}

// A tooltip anchors on the widget it describes, or on the part a
// TooltipAnchorer names, not at the pointer: it opened 12px right of
// and 22px under wherever the pointer rested, off to one side of its
// trigger.
func TestTooltipAnchorsOnItsWidget(t *testing.T) {
	window := render.Rect{W: 640, H: 480}
	btn := widget.NewButton(widget.NewSpacer(40, 20), 0, 0)
	btn.Arrange(render.Rect{X: 100, Y: 50, W: 80, H: 36})
	if r := tooltipAnchor(btn, 170, 80, window); r != btn.Bounds() {
		t.Errorf("tooltip anchor %v, want the button's %v", r, btn.Bounds())
	}
	day := render.Rect{X: 10, Y: 10, W: 20, H: 20}
	if r := tooltipAnchor(anchorAt{widget.NewBox(widget.Row, 0, 0), day}, 0, 0, window); r != day {
		t.Errorf("anchored tooltip on %v, want the named part %v", r, day)
	}
}

// A widget taller than the output anchored its tooltip below its own
// bottom edge, off the output, and the compositor slid the card back
// up over the pointer, where it swallowed the pointer's wheel on
// Hyprland. A tall widget's anchor is the band of it around the
// pointer, clipped to the window as the positioner requires.
func TestTooltipOfATallWidgetAnchorsByThePointer(t *testing.T) {
	window := render.Rect{W: 640, H: 480}
	list := widget.NewBox(widget.Column, 0, 0)
	list.Arrange(render.Rect{X: 20, Y: 100, W: 300, H: 2000})
	want := render.Rect{X: 20, Y: 200 - tooltipReach, W: 300, H: 2 * tooltipReach}
	if r := tooltipAnchor(list, 150, 200, window); r != want {
		t.Errorf("tall widget's anchor %v, want the band %v around the pointer", r, want)
	}
	list.Arrange(render.Rect{X: -50, Y: 400, W: 800, H: 2000})
	want = render.Rect{X: 0, Y: 400, W: 640, H: 410 + tooltipReach - 400}
	if r := tooltipAnchor(list, 150, 410, window); r != want {
		t.Errorf("anchor %v, want %v clipped to the widget and the window", r, want)
	}
}

// anchorAt is a widget whose tooltip belongs to one rect of it.
type anchorAt struct {
	*widget.Box
	r render.Rect
}

func (a anchorAt) TooltipAnchor() render.Rect { return a.r }
