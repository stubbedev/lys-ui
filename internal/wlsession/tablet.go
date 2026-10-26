package wlsession

import (
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// Tablets (zwp_tablet_v2): pens and their kin. Each tool's events
// accumulate over a frame - proximity, motion, pressure, tilt,
// distance, tip down/up, buttons - and the frame routes to the surface
// the tool is near as one TabletSample. Pads (express keys, rings,
// strips) are not bound.

// TabletTool is a tool's physical kind.
type TabletTool uint8

// Tablet tools, the protocol's types.
const (
	ToolPen TabletTool = iota
	ToolEraser
	ToolBrush
	ToolPencil
	ToolAirbrush
	ToolFinger
	ToolMouse
	ToolLens
)

// toolTypes maps the protocol's tool type codes.
var toolTypes = map[uint32]TabletTool{
	0x140: ToolPen, 0x141: ToolEraser, 0x142: ToolBrush, 0x143: ToolPencil,
	0x144: ToolAirbrush, 0x145: ToolFinger, 0x146: ToolMouse, 0x147: ToolLens,
}

// TabletSample is one tool frame: where the tool is (surface-local
// logical coordinates) and its axes, plus what changed this frame.
type TabletSample struct {
	Tool TabletTool
	X, Y float64
	// Pressure and Distance are normalized to 0..1; TiltX/TiltY and
	// Rotation are degrees.
	Pressure, Distance float64
	TiltX, TiltY       float64
	Rotation           float64
	// Down and Up mark the tip touching or leaving the surface this
	// frame; ProximityIn/Out the tool entering or leaving range.
	Down, Up                  bool
	ProximityIn, ProximityOut bool
	// Button is a stylus button that changed this frame (0: none),
	// Pressed its new state.
	Button  uint32
	Pressed bool
	// Serial is the frame's input serial (the down's, or proximity's).
	Serial uint32
}

// SurfaceTabletHandler is a SurfacePointerHandler that takes tablet
// tools.
type SurfaceTabletHandler interface {
	HandleTabletFrame(s TabletSample)
}

// toolState is one tool's accumulating frame.
type toolState struct {
	s       *Session
	surface *wl.Surface
	sample  TabletSample
}

// maxTabletVersion is the newest tablet manager the session speaks.
const maxTabletVersion = 1

// bindTabletManager binds the tablet manager global.
func (s *Session) bindTabletManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpTabletManagerV2(ctx)
	if !s.bindOptional(ev, maxTabletVersion, mgr) {
		return
	}
	s.tabletMgr = mgr
	debug.Log("shell", "tablet-v2 bound")
	s.ensureTabletSeat()
}

// ensureTabletSeat gets the seat's tablet object once both the manager
// and the seat exist.
func (s *Session) ensureTabletSeat() {
	if s.tabletMgr == nil || s.seat == nil || s.tabletSeat != nil {
		return
	}
	ts, err := s.tabletMgr.GetSeat(s.seat)
	if err != nil {
		return
	}
	s.tabletSeat = ts
	ts.AddToolAddedHandler(s)
}

// HandleZwpTabletSeatV2ToolAdded wires a new tool's events.
func (s *Session) HandleZwpTabletSeatV2ToolAdded(ev wlr.ZwpTabletSeatV2ToolAddedEvent) {
	t := &toolState{s: s}
	tool := ev.Id
	tool.AddTypeHandler(t)
	tool.AddProximityInHandler(t)
	tool.AddProximityOutHandler(t)
	tool.AddDownHandler(t)
	tool.AddUpHandler(t)
	tool.AddMotionHandler(t)
	tool.AddPressureHandler(t)
	tool.AddDistanceHandler(t)
	tool.AddTiltHandler(t)
	tool.AddRotationHandler(t)
	tool.AddButtonHandler(t)
	tool.AddFrameHandler(t)
}

// HandleZwpTabletToolV2Type records the tool's kind.
func (t *toolState) HandleZwpTabletToolV2Type(ev wlr.ZwpTabletToolV2TypeEvent) {
	t.sample.Tool = toolTypes[ev.ToolType]
}

// HandleZwpTabletToolV2ProximityIn records the surface the tool nears.
func (t *toolState) HandleZwpTabletToolV2ProximityIn(ev wlr.ZwpTabletToolV2ProximityInEvent) {
	t.surface = ev.Surface
	t.sample.ProximityIn, t.sample.Serial = true, ev.Serial
}

// HandleZwpTabletToolV2ProximityOut marks the tool leaving range.
func (t *toolState) HandleZwpTabletToolV2ProximityOut(wlr.ZwpTabletToolV2ProximityOutEvent) {
	t.sample.ProximityOut = true
}

// HandleZwpTabletToolV2Down marks the tip touching.
func (t *toolState) HandleZwpTabletToolV2Down(ev wlr.ZwpTabletToolV2DownEvent) {
	t.sample.Down, t.sample.Serial = true, ev.Serial
	t.s.inputSerial = ev.Serial
}

// HandleZwpTabletToolV2Up marks the tip lifting.
func (t *toolState) HandleZwpTabletToolV2Up(wlr.ZwpTabletToolV2UpEvent) { t.sample.Up = true }

// HandleZwpTabletToolV2Motion records the position.
func (t *toolState) HandleZwpTabletToolV2Motion(ev wlr.ZwpTabletToolV2MotionEvent) {
	t.sample.X, t.sample.Y = float64(ev.X), float64(ev.Y)
}

// HandleZwpTabletToolV2Pressure records the normalized pressure.
func (t *toolState) HandleZwpTabletToolV2Pressure(ev wlr.ZwpTabletToolV2PressureEvent) {
	t.sample.Pressure = float64(ev.Pressure) / 65535
}

// HandleZwpTabletToolV2Distance records the normalized distance.
func (t *toolState) HandleZwpTabletToolV2Distance(ev wlr.ZwpTabletToolV2DistanceEvent) {
	t.sample.Distance = float64(ev.Distance) / 65535
}

// HandleZwpTabletToolV2Tilt records the tilt.
func (t *toolState) HandleZwpTabletToolV2Tilt(ev wlr.ZwpTabletToolV2TiltEvent) {
	t.sample.TiltX, t.sample.TiltY = float64(ev.TiltX), float64(ev.TiltY)
}

// HandleZwpTabletToolV2Rotation records the rotation.
func (t *toolState) HandleZwpTabletToolV2Rotation(ev wlr.ZwpTabletToolV2RotationEvent) {
	t.sample.Rotation = float64(ev.Degrees)
}

// HandleZwpTabletToolV2Button records a stylus button change.
func (t *toolState) HandleZwpTabletToolV2Button(ev wlr.ZwpTabletToolV2ButtonEvent) {
	t.sample.Button, t.sample.Pressed, t.sample.Serial = ev.Button, ev.State == 1, ev.Serial
	if t.sample.Pressed {
		t.s.inputSerial = ev.Serial
	}
}

// HandleZwpTabletToolV2Frame delivers the frame to the tool's surface
// and clears what was per-frame; leaving proximity drops the surface.
func (t *toolState) HandleZwpTabletToolV2Frame(wlr.ZwpTabletToolV2FrameEvent) { t.frame() }

// frame routes the accumulated sample.
func (t *toolState) frame() {
	if h, ok := t.s.surfaceHandlers[t.surface].(SurfaceTabletHandler); ok {
		h.HandleTabletFrame(t.sample)
	}
	if t.sample.ProximityOut {
		t.surface = nil
	}
	t.sample.Down, t.sample.Up = false, false
	t.sample.ProximityIn, t.sample.ProximityOut = false, false
	t.sample.Button, t.sample.Pressed = 0, false
}
