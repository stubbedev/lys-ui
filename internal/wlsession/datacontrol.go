// Data control: optional ext_data_control_v1 and
// zwlr_data_control_unstable_v1 support, the clipboard-manager
// protocols. Unlike wl_data_device they need no surface and no
// keyboard focus, so a bar or a background clipboard history can
// watch, read, and claim the seat's selections. Both manager globals
// are feature-detected; the session only owns the manager objects,
// and internal/datacontrol creates the device and drives it.
package wlsession

import (
	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxExtDataControlVersion is the ext_data_control_manager_v1 version
// we bind: the staging protocol is at version 1, which already carries
// the primary selection.
const maxExtDataControlVersion = 1

// maxWlrDataControlVersion is the zwlr_data_control_manager_v1
// version we bind: v2 added the primary selection.
const maxWlrDataControlVersion = 2

// bindExtDataControlManager binds the optional ext data-control
// manager global.
func (s *Session) bindExtDataControlManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewDataControlManagerV1(ctx)
	if !s.bindOptional(ev, maxExtDataControlVersion, mgr) {
		return
	}
	s.extDataControlMgr = mgr
	debug.Log("input", "ext-data-control-v1 bound")
}

// bindWlrDataControlManager binds the optional wlr data-control
// manager global, recording the bound version (primary selection
// needs 2).
func (s *Session) bindWlrDataControlManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwlrDataControlManagerV1(ctx)
	if !s.bindOptional(ev, maxWlrDataControlVersion, mgr) {
		return
	}
	s.wlrDataControlMgr = mgr
	s.wlrDataControlVersion = bindVersion(ev.Version, maxWlrDataControlVersion)
	debug.Log("input", "wlr-data-control-unstable-v1 bound")
}

// ExtDataControlManager returns the bound ext_data_control_manager_v1,
// nil when the compositor lacks it.
func (s *Session) ExtDataControlManager() *wlr.DataControlManagerV1 {
	return s.extDataControlMgr
}

// WlrDataControlManager returns the bound zwlr_data_control_manager_v1
// and its bound version, nil and 0 when the compositor lacks it.
func (s *Session) WlrDataControlManager() (*wlr.ZwlrDataControlManagerV1, uint32) {
	return s.wlrDataControlMgr, s.wlrDataControlVersion
}
