package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/widget"
)

func TestClosedWindowStopsItsAnimations(t *testing.T) {
	t.Setenv(rendererEnv, "cpu")
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	t.Cleanup(animclock.Reset)
	animclock.Reset()
	spinner := widget.NewSpinner(24)
	spinner.SetSpinning(true)
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(spinner, false)
	h := newPaintHarness(root, 120, 80)
	h.frame()
	if !animclock.Active() {
		t.Fatal("a spinner in a drawn window schedules no frame")
	}
	h.wnd.release()
	for range 3 {
		wake, ok := animclock.Next()
		if !ok {
			break
		}
		now = wake
		animclock.Tick(wake)
	}
	if _, ok := animclock.Next(); ok || animclock.Active() {
		t.Error("a closed window's spinner still schedules frames")
	}
}
