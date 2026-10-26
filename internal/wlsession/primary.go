// Primary selection: optional zwp_primary_selection_unstable_v1
// support, the X11-style PRIMARY that middle-click pastes. The manager
// global is feature-detected — compositors without it keep the session
// running with every primary-selection call a no-op.
package wlsession

import (
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxPrimarySelectionVersion is the zwp_primary_selection_unstable_v1
// version we bind. The protocol stopped at version 1: every request,
// event, and object is v1, so there is nothing newer to cap.
const maxPrimarySelectionVersion = 1

// bindPrimarySelectionManager binds the optional primary selection
// manager global.
func (s *Session) bindPrimarySelectionManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpPrimarySelectionDeviceManagerV1(ctx)
	if !s.bindOptional(ev, maxPrimarySelectionVersion, mgr) {
		return
	}
	s.primarySelectionMgr = mgr
	s.ensurePrimarySelectionDevice()
}

// ensurePrimarySelectionDevice creates the seat's primary selection
// device once both the manager and the seat are bound, whichever
// arrives first.
func (s *Session) ensurePrimarySelectionDevice() {
	if s.primarySelectionDev != nil || s.primarySelectionMgr == nil || s.seat == nil {
		return
	}
	dev, err := s.primarySelectionMgr.GetDevice(s.seat)
	if err != nil {
		return
	}
	s.primarySelectionDev = dev
	// The data_offer and selection events go to
	// internal/clipboard, which registers its own handlers — the
	// session only owns the objects.
	debug.Log("input", "primary-selection-v1 bound")
}

// PrimarySelectionAvailable reports whether the compositor advertised
// zwp_primary_selection_device_manager_v1 and the seat's device is
// bound. When false, every primary-selection call on the session is a
// no-op and the clipboard runs exactly as without the protocol.
func (s *Session) PrimarySelectionAvailable() bool { return s.primarySelectionDev != nil }

// PrimarySelectionDevice returns the seat's primary selection device,
// nil without the protocol.
func (s *Session) PrimarySelectionDevice() *wlr.ZwpPrimarySelectionDeviceV1 {
	return s.primarySelectionDev
}
