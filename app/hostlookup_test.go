package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/layersurface"
	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

func TestHandlesResolveToTheirWindows(t *testing.T) {
	face := testFace(t)
	a := &Application{sess: &wlsession.Session{}}
	win := &Window{app: a, win: &window.Window{}}
	layer := &LayerWindow{app: a, ls: &layersurface.Surface{}}
	winRoot, layerRoot := &spyRoot{}, &spyRoot{}
	whw := &hostWindow{host: win.win, win: win, anim: &a.anim, frac120: 240, input: &surfaceInput{}, router: &widget.Router{Root: winRoot}}
	lhw := &hostWindow{host: &layerHost{ls: layer.ls}, layer: layer, anim: &a.anim, frac120: 150, input: &surfaceInput{}, router: &widget.Router{Root: layerRoot}}
	a.windows = []*hostWindow{lhw, whw}

	if got := a.fracFor(win); got != 240 {
		t.Errorf("popover scale over the window = %d, want its 240", got)
	}
	if got := a.fracFor(layer); got != 150 {
		t.Errorf("popover scale over the layer = %d, want its 150", got)
	}

	a.OnRelativeMotion(win, func(_, _, _, _ float64) {})
	if whw.input.onRelative == nil || lhw.input.onRelative != nil {
		t.Errorf("relative motion hook on window %v, on layer %v; want only the window's",
			whw.input.onRelative != nil, lhw.input.onRelative != nil)
	}

	if _, err := a.ShowToast("saved", time.Minute, &ToastConfig{Face: face, Host: win}); err != nil {
		t.Fatalf("toast on the window handle: %v", err)
	}
	if _, ok := whw.router.Root.(*toastLayer); !ok {
		t.Errorf("the toast named for the window landed elsewhere: window root %T", whw.router.Root)
	}

	a.SetToastHost(layer)
	if _, err := a.ShowToast("default", time.Minute, &ToastConfig{Face: face}); err != nil {
		t.Fatalf("toast on the toast host: %v", err)
	}
	if _, ok := lhw.router.Root.(*toastLayer); !ok {
		t.Errorf("SetToastHost(layer) did not route the toast to the layer: layer root %T", lhw.router.Root)
	}
}
