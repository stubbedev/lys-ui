// xdg-foreign-unstable-v2: exporting a toplevel yields a string handle
// another client can name it by. xdg-desktop-portal takes it as a
// dialog's parent_window ("wayland:<handle>"), so portal dialogs stack
// over and stay modal to the window that opened them; importing such a
// handle is the other side, a portal backend parenting its dialog to
// the app's window. Both globals are feature-detected: without them the
// session hands out nil, portal calls go unparented, and a window asked
// for a foreign parent fails to open.
package wlsession

import (
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

const (
	maxExporterVersion = 1
	maxImporterVersion = 1
)

func (s *Session) bindExporter(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	exp := wlr.NewZxdgExporterV2(ctx)
	if !s.bindOptional(ev, maxExporterVersion, exp) {
		return
	}
	s.exporter = exp
	debug.Log("shell", "xdg-foreign exporter bound")
}

func (s *Session) bindImporter(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	imp := wlr.NewZxdgImporterV2(ctx)
	if !s.bindOptional(ev, maxImporterVersion, imp) {
		return
	}
	s.importer = imp
	debug.Log("shell", "xdg-foreign importer bound")
}

// Exporter returns the bound zxdg_exporter_v2, or nil when the
// compositor does not advertise xdg-foreign v2.
func (s *Session) Exporter() *wlr.ZxdgExporterV2 { return s.exporter }

// Importer returns the bound zxdg_importer_v2, or nil when the
// compositor does not advertise xdg-foreign v2.
func (s *Session) Importer() *wlr.ZxdgImporterV2 { return s.importer }
