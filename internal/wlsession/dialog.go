// xdg-dialog-v1: optional window-level dialog modality — the
// compositor-side companion of xdg_toplevel.set_parent. Registering a
// parented toplevel as a dialog and hinting it modal lets the
// compositor block input to the parent itself (keep the dialog on top,
// dismiss-on-outside-click, dimming), where the toolkit's
// application-level block only reaches as far as this client's own
// windows. The manager global is feature-detected: without it the
// session hands out nil and every caller degrades to
// application-level modality.
package wlsession

import (
	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxDialogManagerVersion is the xdg_wm_dialog_v1 version we bind: the
// protocol stopped at version 1.
const maxDialogManagerVersion = 1

// bindDialogManager binds the optional xdg-dialog manager global.
func (s *Session) bindDialogManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewWmDialogV1(ctx)
	if !s.bindOptional(ev, maxDialogManagerVersion, mgr) {
		return
	}
	s.dialogMgr = mgr
	debug.Log("shell", "xdg-dialog-v1 bound")
}

// DialogManager returns the bound xdg_wm_dialog_v1 manager, or nil
// when the compositor does not advertise the protocol; a nil manager
// makes every dialog-modality hint a no-op, and modality falls back to
// the application-level block.
func (s *Session) DialogManager() *wlr.WmDialogV1 { return s.dialogMgr }
