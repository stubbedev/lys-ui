package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

type serialProbe struct {
	recordingHandler
	s    *Session
	seen []uint32
}

func (h *serialProbe) HandlePointerButton(button, state, serial uint32) {
	h.seen = append(h.seen, h.s.InputSerial())
}

func TestInputSerialIsTheLatestUserPress(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	probe := &serialProbe{s: s}
	s.SetSurfaceInput(surf, probe)
	tool := &toolState{s: s}

	steps := []struct {
		name string
		do   func()
		want uint32
	}{
		{"nothing yet", func() {}, 0},
		{"pointer enter", func() { s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf, Serial: 3}) }, 0},
		{"keyboard enter", func() { s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: surf, Serial: 4}) }, 0},
		{"modifiers", func() { s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{Serial: 5}) }, 0},
		{"button press", func() { s.HandlePointerButton(wl.PointerButtonEvent{Button: 272, State: 1, Serial: 10}) }, 10},
		{"button release", func() { s.HandlePointerButton(wl.PointerButtonEvent{Button: 272, State: 0, Serial: 11}) }, 10},
		{"key press", func() { s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: 30, State: 1, Serial: 12}) }, 12},
		{"key release", func() { s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: 30, State: 0, Serial: 13}) }, 12},
		{"touch down", func() { s.HandleTouchDown(wl.TouchDownEvent{Surface: surf, Id: 1, Serial: 14}) }, 14},
		{"touch up", func() { s.HandleTouchUp(wl.TouchUpEvent{Id: 1, Serial: 15}) }, 14},
		{"tablet proximity", func() {
			tool.HandleZwpTabletToolV2ProximityIn(wlr.ZwpTabletToolV2ProximityInEvent{Serial: 16, Surface: surf})
		}, 14},
		{"tablet down", func() { tool.HandleZwpTabletToolV2Down(wlr.ZwpTabletToolV2DownEvent{Serial: 17}) }, 17},
		{"tablet button press", func() {
			tool.HandleZwpTabletToolV2Button(wlr.ZwpTabletToolV2ButtonEvent{Button: 331, State: 1, Serial: 18})
		}, 18},
		{"tablet button release", func() {
			tool.HandleZwpTabletToolV2Button(wlr.ZwpTabletToolV2ButtonEvent{Button: 331, State: 0, Serial: 19})
		}, 18},
	}
	for _, st := range steps {
		st.do()
		if got := s.InputSerial(); got != st.want {
			t.Errorf("after %s: InputSerial = %d, want %d", st.name, got, st.want)
		}
	}
	if len(probe.seen) != 2 || probe.seen[0] != 10 || probe.seen[1] != 10 {
		t.Errorf("serials the surface saw while its buttons dispatched = %v, want [10 10]", probe.seen)
	}
}
