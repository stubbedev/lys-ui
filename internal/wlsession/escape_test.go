package wlsession

import (
	"errors"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/wlr"
)

type fakeEscapeArm struct {
	handler   wlr.LysEscapeArmV1PressedHandler
	destroyed int
}

func (f *fakeEscapeArm) Destroy() error {
	f.destroyed++
	return nil
}

func (f *fakeEscapeArm) AddPressedHandler(h wlr.LysEscapeArmV1PressedHandler) { f.handler = h }

type fakeEscapeManager struct {
	arm *fakeEscapeArm
	err error
}

func (f *fakeEscapeManager) Arm() (escapeArmAPI, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.arm = &fakeEscapeArm{}
	return f.arm, nil
}

func TestEscapeArmIsOptional(t *testing.T) {
	if slices.Contains(requiredGlobals, "lys_escape_manager_v1") {
		t.Error("lys_escape_manager_v1 is required; it must stay feature-detected")
	}
	if !slices.Contains(optionalGlobals, "lys_escape_manager_v1") {
		t.Error("lys_escape_manager_v1 is not among the optional globals the session reports")
	}
	s := &Session{}
	arm, err := s.ArmEscape(func() { t.Error("pressed without a protocol") })
	if !errors.Is(err, ErrEscapeUnavailable) || arm != nil {
		t.Errorf("ArmEscape without the protocol = %v, %v, want nil, ErrEscapeUnavailable", arm, err)
	}
}

func TestEscapeArmReportsThePressUntilDisarmed(t *testing.T) {
	mgr := &fakeEscapeManager{}
	s := &Session{escapeMgr: mgr}
	pressed := 0
	arm, err := s.ArmEscape(func() { pressed++ })
	if err != nil {
		t.Fatal(err)
	}
	mgr.arm.handler.HandleLysEscapeArmV1Pressed(wlr.LysEscapeArmV1PressedEvent{})
	if pressed != 1 {
		t.Fatalf("pressed %d times, want 1", pressed)
	}
	arm.Disarm()
	arm.Disarm()
	if mgr.arm.destroyed != 1 {
		t.Errorf("destroyed %d times, want 1", mgr.arm.destroyed)
	}
	mgr.arm.handler.HandleLysEscapeArmV1Pressed(wlr.LysEscapeArmV1PressedEvent{})
	if pressed != 1 {
		t.Errorf("a press in flight past the disarm fired: %d", pressed)
	}
}

func TestEscapeArmFailureSaysSo(t *testing.T) {
	boom := errors.New("boom")
	s := &Session{escapeMgr: &fakeEscapeManager{err: boom}}
	if _, err := s.ArmEscape(func() {}); !errors.Is(err, boom) {
		t.Errorf("ArmEscape = %v, want it to wrap the failure", err)
	}
}
