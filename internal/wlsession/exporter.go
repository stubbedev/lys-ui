// xdg-foreign-unstable-v2: exporting a toplevel yields a string handle
// another client can name it by. xdg-desktop-portal takes it as a
// dialog's parent_window ("wayland:<handle>"), so portal dialogs stack
// over and stay modal to the window that opened them. The exporter
// global is feature-detected: without it the session hands out nil and
// portal calls go unparented.
package wlsession

import (
	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"
)

const maxExporterVersion = 1

func (s *Session) bindExporter(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	exp := wlr.NewZxdgExporterV2(ctx)
	if !s.bindOptional(ev, maxExporterVersion, exp) {
		return
	}
	s.exporter = exp
	debug.Log("shell", "xdg-foreign exporter bound")
}

// Exporter returns the bound zxdg_exporter_v2, or nil when the
// compositor does not advertise xdg-foreign v2.
func (s *Session) Exporter() *wlr.ZxdgExporterV2 { return s.exporter }
