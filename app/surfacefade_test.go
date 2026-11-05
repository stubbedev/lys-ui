package app

import (
	"testing"

	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

func TestSurfaceFadeTakesOnlyASurfaceRootTheCompositorCanFade(t *testing.T) {
	a := &Application{sess: &wlsession.Session{}}
	win := &Window{app: a, win: &window.Window{}}
	root, tree := widget.NewBox(widget.Column, 0, 0), widget.NewBox(widget.Column, 0, 0)
	hw := &hostWindow{host: win.win, win: win, input: &surfaceInput{}, router: &widget.Router{Root: root}, styleRoot: tree}
	a.windows = []*hostWindow{hw}

	if a.fadeSurface(root, 0.5) {
		t.Error("a widget wrapping the app's tree took the surface fade")
	}
	if win.alpha != 0 {
		t.Errorf("a refused fade set the window opacity to %v", win.alpha)
	}
	hw.styleRoot = root
	if a.fadeSurface(root, 0.5) {
		t.Error("the surface fade was taken without a compositor alpha modifier")
	}
}

func TestOwedSurfaceStateCommitsWithoutAPaint(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 64, 32)
	h.frame()
	h.frame()
	attaches, commits := h.surf.attaches, h.surf.commits

	h.frame()
	if h.surf.commits != commits {
		t.Fatalf("a frame with nothing damaged and no state owed committed")
	}

	h.wnd.stateOwed = true
	h.frame()
	if h.surf.commits != commits+1 {
		t.Errorf("owed surface state made %d commits, want 1", h.surf.commits-commits)
	}
	if h.surf.attaches != attaches {
		t.Errorf("owed surface state attached %d buffers, want none", h.surf.attaches-attaches)
	}
	if h.wnd.stateOwed {
		t.Error("the commit left the surface state owed")
	}
	if !h.wnd.framePending {
		t.Error("the state commit armed no frame callback")
	}
}
