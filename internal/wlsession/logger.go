// Logging convention for the session. lys-ui is silent by default: the
// logger starts as a discarding sink and nothing writes to
// stdout/stderr on its own. Applications install their logger once —
// SetLogger below, mirrored by app.SetLogger — and every library emit
// site (this package, widget theme warnings, the appearance monitor)
// routes through it. See internal/logutil for the levels contract.

package wlsession

import (
	"log/slog"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/logutil"
)

// SetLogger installs the library-wide logger: protocol-level warnings
// (an advertised optional global that failed to bind), event-loop
// diagnostics (missing optional protocols, capability changes) and
// terminal conditions (the compositor raising a fatal protocol error)
// are emitted through it. Nothing logs per-frame or per-keypress, and
// nothing logs at Info. A nil logger selects the default: discard all
// output. Safe from any goroutine.
func SetLogger(l *slog.Logger) { logutil.Set(l) }

// optionalGlobals are the interfaces HandleRegistryGlobal binds when
// advertised but never requires: the session degrades gracefully
// without each one. Keep in sync with HandleRegistryGlobal's optional
// cases.
var optionalGlobals = []string{
	"wl_seat",
	"xdg_wm_base",
	"wl_data_device_manager",
	"wp_viewporter",
	"wp_presentation",
	"wp_fractional_scale_manager_v1",
	"zxdg_decoration_manager_v1",
	"zwp_text_input_manager_v3",
	"zwp_primary_selection_device_manager_v1",
	"zwlr_foreign_toplevel_management_v1",
	"xdg_activation_v1",
	"zwp_idle_inhibit_manager_v1",
	"zwp_keyboard_shortcuts_inhibit_manager_v1",
	"zxdg_output_manager_v1",
	"xdg_wm_dialog_v1",
	"zxdg_exporter_v2",
	"wl_subcompositor",
	"zwp_linux_dmabuf_v1",
	"xdg_toplevel_icon_manager_v1",
	"ext_data_control_manager_v1",
	"zwlr_data_control_manager_v1",
}

// logOptionalGlobals reports at Debug which optional protocols the
// compositor did not advertise. Their absence is the normal case on
// minimal compositors, so it is chatter, not a warning: every
// consuming feature already no-ops behind an Available accessor.
func (s *Session) logOptionalGlobals() {
	log := logutil.L()
	for _, g := range optionalGlobals {
		if !s.globals[g] {
			log.Debug("wlsession: optional protocol not advertised", slog.String("global", g))
		}
	}
}

// bindOptional binds an optional global at the capped version. False
// means the bind failed and the caller must skip every use of obj;
// the failure is reported through bindFailed.
func (s *Session) bindOptional(ev wl.RegistryGlobalEvent, maxVersion uint32, obj wl.Proxy) bool {
	if err := s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, maxVersion), obj); err != nil {
		s.bindFailed(ev.Interface, err)
		return false
	}
	return true
}

// bindFailed reports an advertised optional global whose bind failed.
// The compositor advertised the protocol, so this is real degradation —
// the feature silently disappears — and is Warn, not Debug: degraded
// but running. The shared funnel of every optional bind site.
func (s *Session) bindFailed(global string, err error) {
	logutil.L().Warn("wlsession: optional global bind failed; feature disabled",
		slog.String("global", global), slog.Any("err", err))
}
