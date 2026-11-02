// Escape arm: optional lys_escape_v1 support. An open dropdown that
// leaves the keyboard with another client's window claims the next
// Escape press through it, which no standard protocol can do. Feature-
// detected: without the global ArmEscape fails with
// ErrEscapeUnavailable.
package wlsession

import (
	"errors"
	"fmt"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// ErrEscapeUnavailable reports that the compositor advertised no
// lys_escape_manager_v1.
var ErrEscapeUnavailable = errors.New("wlsession: no lys-escape protocol")

// escapeManagerAPI is the request side of lys_escape_manager_v1, seen
// through a narrow interface so tests can record the requests.
type escapeManagerAPI interface {
	Arm() (escapeArmAPI, error)
}

// escapeArmAPI is a lys_escape_arm_v1. *wlr.LysEscapeArmV1 satisfies it.
type escapeArmAPI interface {
	Destroy() error
	AddPressedHandler(h wlr.LysEscapeArmV1PressedHandler)
}

// wireEscapeManager adapts the generated manager proxy to
// escapeManagerAPI.
type wireEscapeManager struct {
	mgr *wlr.LysEscapeManagerV1
}

// Arm implements escapeManagerAPI.
func (m wireEscapeManager) Arm() (escapeArmAPI, error) {
	a, err := m.mgr.Arm()
	if err != nil {
		return nil, err
	}
	return a, nil
}

// bindEscapeManager binds lys_escape_manager_v1.
func (s *Session) bindEscapeManager(ev wl.RegistryGlobalEvent) {
	mgr := wlr.NewLysEscapeManagerV1(s.registry.Context())
	if s.bindOptional(ev, 1, mgr) {
		s.escapeMgr = wireEscapeManager{mgr: mgr}
	}
}

// EscapeArm is one claim on the next Escape press: pressed runs when
// the compositor kept that press from the focused window, at most once.
type EscapeArm struct {
	req     escapeArmAPI
	pressed func()
}

// ArmEscape claims the next Escape press for pressed.
func (s *Session) ArmEscape(pressed func()) (*EscapeArm, error) {
	if s.escapeMgr == nil {
		return nil, ErrEscapeUnavailable
	}
	req, err := s.escapeMgr.Arm()
	if err != nil {
		return nil, fmt.Errorf("wlsession: arm escape: %w", err)
	}
	a := &EscapeArm{req: req, pressed: pressed}
	req.AddPressedHandler(a)
	return a, nil
}

// HandleLysEscapeArmV1Pressed implements
// wlr.LysEscapeArmV1PressedHandler.
func (a *EscapeArm) HandleLysEscapeArmV1Pressed(wlr.LysEscapeArmV1PressedEvent) {
	if a.req != nil {
		a.pressed()
	}
}

// Disarm gives the claim up; a second call does nothing.
func (a *EscapeArm) Disarm() {
	if a.req == nil {
		return
	}
	if err := a.req.Destroy(); err != nil {
		debug.Log("input", "escape disarm: %v", err)
	}
	a.req = nil
}
