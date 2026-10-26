package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// axisRecordingHandler captures scroll deltas.
type axisRecordingHandler struct {
	recordingHandler
	dx, dy []float64
}

func (h *axisRecordingHandler) HandlePointerAxis(dx, dy float64) {
	h.dx = append(h.dx, dx)
	h.dy = append(h.dy, dy)
}

// TestWheelValue120Conversion pins the discrete-wheel pipeline:
// axis_value120 accumulates per axis and the pointer frame flushes
// it as whole-notch deltas on the right axis, signed as the axis is
// (positive down or right, the protocol's rule), and the app's
// 10-per-step scale sees one step per notch.
func TestWheelValue120Conversion(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	h := &axisRecordingHandler{}
	s.SetSurfaceInput(surf, h)
	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})

	// Two notches down.
	s.HandlePointerAxisValue120(wl.PointerAxisValue120Event{Axis: 0, Value120: 240})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dy) != 1 || h.dy[0] != 20 || h.dx[0] != 0 {
		t.Errorf("flushed dx=%v dy=%v, want one event with dy=20", h.dx, h.dy)
	}

	// A horizontal wheel event lands on dx.
	s.HandlePointerAxisValue120(wl.PointerAxisValue120Event{Axis: 1, Value120: 120})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dx) != 2 || h.dx[1] != 10 {
		t.Errorf("flushed dx = %v, want [0 10] (one notch on the second flush)", h.dx)
	}

	// An empty frame does not emit a spurious scroll.
	before := len(h.dx) + len(h.dy)
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dx)+len(h.dy) != before {
		t.Errorf("empty frame emitted a scroll")
	}
}

// TestSmoothAxisPassthrough pins that smooth (trackpad) axis values
// bypass the value120 pipeline and deliver their raw value directly.
func TestSmoothAxisPassthrough(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	h := &axisRecordingHandler{}
	s.SetSurfaceInput(surf, h)
	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})

	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 0, Value: 12.5})
	if len(h.dy) != 1 || h.dy[0] != 12.5 {
		t.Errorf("smooth dy = %v, want [12.5]", h.dy)
	}
}

// preciseHandler takes pixel scrolling too.
type preciseHandler struct {
	axisRecordingHandler
	px [][2]float64
}

func (h *preciseHandler) HandlePointerScrollPixels(dx, dy float64) {
	h.px = append(h.px, [2]float64{dx, dy})
}

// A framed seat routes a frame's scroll once: wheel notches (with the
// smooth value the compositor sends beside them) as steps, finger
// scrolling as pixels to a precise handler, and as axis motion to any
// other.
func TestAxisFrames(t *testing.T) {
	s := newRoutingSession()
	s.seatVersion = 7
	surf := &wl.Surface{}
	h := &preciseHandler{}
	s.SetSurfaceInput(surf, h)
	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})

	// A wheel notch: source, discrete and the smooth 15 in one frame.
	s.HandlePointerAxisSource(wl.PointerAxisSourceEvent{AxisSource: wl.PointerAxisSourceWheel})
	s.HandlePointerAxisDiscrete(wl.PointerAxisDiscreteEvent{Axis: 0, Discrete: 1})
	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 0, Value: 15})
	if len(h.dy) != 0 {
		t.Fatal("the axis routed before its frame")
	}
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dy) != 1 || h.dy[0] != 10 || len(h.px) != 0 {
		t.Fatalf("a notch routed dy=%v px=%v, want one 10 (one step), once", h.dy, h.px)
	}

	// Finger scrolling: exact pixels, on both axes.
	s.HandlePointerAxisSource(wl.PointerAxisSourceEvent{AxisSource: wl.PointerAxisSourceFinger})
	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 0, Value: 2.5})
	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 1, Value: -1})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.px) != 1 || h.px[0] != [2]float64{-1, 2.5} || len(h.dy) != 1 {
		t.Errorf("finger scroll routed px=%v dy=%v, want the exact pixels", h.px, h.dy)
	}

	// A source-less smooth value (no discrete) is axis motion.
	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 0, Value: 15})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dy) != 2 || h.dy[1] != 15 {
		t.Errorf("smooth wheel routed dy=%v, want 15", h.dy)
	}

	// A handler without pixels takes finger scrolling as motion.
	plain := &axisRecordingHandler{}
	s.SetSurfaceInput(surf, plain)
	s.HandlePointerAxisSource(wl.PointerAxisSourceEvent{AxisSource: wl.PointerAxisSourceFinger})
	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 0, Value: 3})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(plain.dy) != 1 || plain.dy[0] != 3 {
		t.Errorf("plain handler got %v", plain.dy)
	}
}

// endingHandler takes pixel scrolling and hears the gesture end.
type endingHandler struct {
	preciseHandler
	ends int
}

func (h *endingHandler) HandlePointerScrollEnd() { h.ends++ }

// The finger lifting (axis_stop, in its own frame with the finger
// source) ends the scroll once; a wheel frame never does.
func TestAxisStopEndsFingerScroll(t *testing.T) {
	s := newRoutingSession()
	s.seatVersion = 7
	surf := &wl.Surface{}
	h := &endingHandler{}
	s.SetSurfaceInput(surf, h)
	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})

	s.HandlePointerAxisSource(wl.PointerAxisSourceEvent{AxisSource: wl.PointerAxisSourceFinger})
	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 0, Value: 4})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if h.ends != 0 {
		t.Fatal("a moving frame ended the scroll")
	}
	s.HandlePointerAxisSource(wl.PointerAxisSourceEvent{AxisSource: wl.PointerAxisSourceFinger})
	s.HandlePointerAxisStop(wl.PointerAxisStopEvent{Axis: 0})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if h.ends != 1 || len(h.px) != 1 {
		t.Errorf("stop frame: ends=%d px=%v, want one end and no motion", h.ends, h.px)
	}
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	s.HandlePointerAxisSource(wl.PointerAxisSourceEvent{AxisSource: wl.PointerAxisSourceWheel})
	s.HandlePointerAxisStop(wl.PointerAxisStopEvent{Axis: 0})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if h.ends != 1 {
		t.Errorf("ends = %d after an empty frame and a wheel stop, want still 1", h.ends)
	}
}
