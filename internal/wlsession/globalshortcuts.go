// Global shortcuts: the optional hyprland_global_shortcuts_manager_v1,
// the de-facto wlroots way for a client to register actions the
// compositor binds to keys (Hyprland offers it; a portal bridges it to
// org.freedesktop.portal.GlobalShortcuts).
package wlsession

import (
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// bindGlobalShortcutsManager binds the optional manager global.
func (s *Session) bindGlobalShortcutsManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewGlobalShortcutsManagerV1(ctx)
	if !s.bindOptional(ev, 1, mgr) {
		return
	}
	s.globalShortcutsMgr = mgr
	debug.Log("input", "hyprland-global-shortcuts-v1 bound")
}

// GlobalShortcutsManager returns the bound manager, nil when the
// compositor lacks it.
func (s *Session) GlobalShortcutsManager() *wlr.GlobalShortcutsManagerV1 {
	return s.globalShortcutsMgr
}
