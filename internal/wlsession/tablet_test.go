package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// tabletRecorder records tablet frames.
type tabletRecorder struct {
	recordingHandler
	frames []TabletSample
}

func (h *tabletRecorder) HandleTabletFrame(s TabletSample) { h.frames = append(h.frames, s) }

// A tool's events accumulate into one sample per frame, routed to the
// surface it is near; per-frame changes clear, the axes persist, and
// leaving proximity drops the surface.
func TestTabletFrames(t *testing.T) {
	s := newRoutingSession()
	surf, h := &wl.Surface{}, &tabletRecorder{}
	s.SetSurfaceInput(surf, h)
	tool := &toolState{s: s}
	tool.HandleZwpTabletToolV2Type(wlr.ZwpTabletToolV2TypeEvent{ToolType: 0x141})
	tool.HandleZwpTabletToolV2ProximityIn(wlr.ZwpTabletToolV2ProximityInEvent{Serial: 4, Surface: surf})
	tool.HandleZwpTabletToolV2Motion(wlr.ZwpTabletToolV2MotionEvent{X: 10, Y: 20})
	tool.HandleZwpTabletToolV2Frame(wlr.ZwpTabletToolV2FrameEvent{})
	tool.HandleZwpTabletToolV2Down(wlr.ZwpTabletToolV2DownEvent{Serial: 9})
	tool.HandleZwpTabletToolV2Pressure(wlr.ZwpTabletToolV2PressureEvent{Pressure: 65535})
	tool.HandleZwpTabletToolV2Tilt(wlr.ZwpTabletToolV2TiltEvent{TiltX: 30, TiltY: -10})
	tool.HandleZwpTabletToolV2Frame(wlr.ZwpTabletToolV2FrameEvent{})
	tool.HandleZwpTabletToolV2Motion(wlr.ZwpTabletToolV2MotionEvent{X: 12, Y: 22})
	tool.HandleZwpTabletToolV2Frame(wlr.ZwpTabletToolV2FrameEvent{})
	tool.HandleZwpTabletToolV2ProximityOut(wlr.ZwpTabletToolV2ProximityOutEvent{})
	tool.HandleZwpTabletToolV2Frame(wlr.ZwpTabletToolV2FrameEvent{})
	tool.HandleZwpTabletToolV2Motion(wlr.ZwpTabletToolV2MotionEvent{X: 1, Y: 1})
	tool.HandleZwpTabletToolV2Frame(wlr.ZwpTabletToolV2FrameEvent{})
	f := h.frames
	if len(f) != 4 {
		t.Fatalf("frames = %d, want 4 (none after proximity out)", len(f))
	}
	if !f[0].ProximityIn || f[0].Tool != ToolEraser || f[0].X != 10 || f[0].Serial != 4 {
		t.Errorf("frame 0: %+v", f[0])
	}
	if !f[1].Down || f[1].Pressure != 1 || f[1].TiltX != 30 || f[1].Serial != 9 || f[1].ProximityIn {
		t.Errorf("frame 1: %+v", f[1])
	}
	if f[2].Down || f[2].Pressure != 1 || f[2].X != 12 {
		t.Errorf("frame 2 (axes persist, changes clear): %+v", f[2])
	}
	if !f[3].ProximityOut {
		t.Errorf("frame 3: %+v", f[3])
	}
}
