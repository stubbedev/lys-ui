package app_test

import (
	"errors"
	"runtime"
	"testing"
	"weak"

	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

func TestHeadlessClosedWindowIsCollected(t *testing.T) {
	wltest.Require(t)
	var application *app.Application
	var closeWin func()
	var sizeOf func() (int, int)
	var shown, closedTree weak.Pointer[widget.DrawingArea]
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		application = a
		kept := widget.NewDrawingArea(40, 40)
		shown = weak.Make(kept)
		if _, err := a.NewLayer(app.LayerConfig{
			Output: out, Layer: app.LayerBottom, Anchor: app.AnchorTop,
			Namespace: "lys-ui-gc-kept", Root: kept,
		}); err != nil {
			return err
		}
		content := widget.NewDrawingArea(200, 60)
		closedTree = weak.Make(content)
		w, err := a.NewLayer(app.LayerConfig{
			Output: out, Layer: app.LayerOverlay, Anchor: app.AnchorBottom,
			Namespace: "lys-ui-gc-closed", Root: content,
		})
		if err != nil {
			return err
		}
		closeWin, sizeOf = w.Close, w.Size
		return nil
	})
	wltest.Eventually(t, "the window to close configured", func() error {
		sized := make(chan bool)
		application.Invoke(func() { w, _ := sizeOf(); sized <- w > 0 })
		if !<-sized {
			return errors.New("not configured yet")
		}
		return nil
	})
	closedNow := make(chan struct{})
	application.Invoke(func() {
		closeWin()
		closeWin, sizeOf = nil, nil
		close(closedNow)
	})
	<-closedNow
	wltest.Eventually(t, "the closed window's widget tree collected", func() error {
		runtime.GC()
		if closedTree.Value() != nil {
			return errors.New("the closed window's root is still reachable")
		}
		return nil
	})
	runtime.GC()
	if shown.Value() == nil {
		t.Fatal("the open window's root was collected")
	}
}
