package app

import (
	"testing"

	"github.com/stubbedev/lys-ui/internal/layersurface"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// Only an automatic axis missing one of its anchors follows the
// content; a concrete axis, or one anchored on both edges, does not.
func TestContentAxes(t *testing.T) {
	for name, tc := range map[string]struct {
		w, h   uint32
		anchor layersurface.Anchor
		want   fitAxes
	}{
		"one edge, both auto":       {0, 0, layersurface.AnchorBottom, fitAxes{w: true, h: true}},
		"corner, both auto":         {0, 0, layersurface.AnchorTop | layersurface.AnchorRight, fitAxes{w: true, h: true}},
		"bar: stretched width":      {0, 30, layersurface.AnchorTop | layersurface.AnchorLeft | layersurface.AnchorRight, fitAxes{}},
		"fixed width, auto height":  {400, 0, layersurface.AnchorTop, fitAxes{h: true}},
		"full screen":               {0, 0, layersurface.AnchorTop | layersurface.AnchorBottom | layersurface.AnchorLeft | layersurface.AnchorRight, fitAxes{}},
		"side panel: stretched h":   {0, 0, layersurface.AnchorLeft | layersurface.AnchorTop | layersurface.AnchorBottom, fitAxes{w: true}},
		"concrete on a single edge": {120, 40, layersurface.AnchorBottom, fitAxes{}},
	} {
		if got := contentAxes(tc.w, tc.h, tc.anchor); got != tc.want {
			t.Errorf("%s: %+v, want %+v", name, got, tc.want)
		}
	}
}

// A toplevel without a requested size takes its content's natural
// extent on each zero axis; a requested axis stays as asked.
func TestDefaultSize(t *testing.T) {
	root := widget.NewDrawingArea(320, 180)
	if w, h := defaultSize(root, 0, 0, nil); w != 320 || h != 180 {
		t.Errorf("unsized window %dx%d, want the content's 320x180", w, h)
	}
	if w, h := defaultSize(root, 500, 0, nil); w != 500 || h != 180 {
		t.Errorf("width-only request %dx%d, want 500x180", w, h)
	}
	if w, h := defaultSize(root, 640, 480, nil); w != 640 || h != 480 {
		t.Errorf("sized window %dx%d, want 640x480 untouched", w, h)
	}
	if w, h := defaultSize(widget.NewDrawingArea(0, 0), 0, 0, nil); w != 1 || h != 1 {
		t.Errorf("empty content %dx%d, want the 1x1 floor", w, h)
	}
}

// A content-sized axis is bounded by its surface's output, by the
// largest output when the compositor places the surface, and by
// unboundedFit only while no output reports a size.
func TestFitBounds(t *testing.T) {
	small := &wlsession.Output{LogicalW: 1920, LogicalH: 1080}
	big := &wlsession.Output{LogicalW: 2560, LogicalH: 1440}
	if got := fitBounds(small, []*wlsession.Output{small, big}); got != (widget.Size{W: 1920, H: 1080}) {
		t.Errorf("pinned output bound %v", got)
	}
	if got := fitBounds(nil, []*wlsession.Output{small, big}); got != (widget.Size{W: 2560, H: 1440}) {
		t.Errorf("compositor-placed bound %v, want the largest output", got)
	}
	if got := fitBounds(nil, nil); got != (widget.Size{W: unboundedFit, H: unboundedFit}) {
		t.Errorf("no outputs bound %v", got)
	}
}

func TestLayerMotionIsTheToolkitsUnlessTheSurfaceOwnsIt(t *testing.T) {
	if got := layerMotion(LayerConfig{}); got != surfx.KindOverlay {
		t.Errorf("default = %v, want the overlay profile", got)
	}
	if got := layerMotion(LayerConfig{OwnMotion: true}); got != surfx.KindMenu {
		t.Errorf("own motion = %v, want no window-level animation", got)
	}
}
