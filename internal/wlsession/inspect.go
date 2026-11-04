// The session-facing half of the debug inspector's doctor block
// (internal/inspect): a read-only snapshot of what the connection
// bound and resolved. Everything here is observation only - no wire
// traffic beyond the one-time cursor theme load the cursor path
// performs anyway on first SetCursor.
package wlsession

import (
	"errors"
	"maps"
	"slices"
)

// errNoShm reports a cursor theme probe without a wl_shm global to
// load the theme through.
var errNoShm = errors.New("wlsession: no wl_shm to load a cursor theme from")

// InspectInfo is a diagnostics snapshot of the session: the bound
// globals with their registry versions, the live outputs, the
// resolved cursor theme, and which scale protocols the compositor
// offered. internal/inspect formats it into the doctor block.
type InspectInfo struct {
	// Globals maps every interface the registry advertised to the
	// version it advertised. Nil means there is no session.
	Globals map[string]uint32
	// Outputs are the live outputs in registry order.
	Outputs []*Output
	// CursorTheme is the resolved xcursor theme name and its nominal
	// size; CursorErr reports why it is unavailable when empty.
	CursorTheme string
	CursorSize  uint32
	CursorErr   error
	// FractionalScale reports both wp_viewporter and
	// wp_fractional_scale_manager_v1 are bound, so surfaces scale
	// fractionally instead of through integer set_buffer_scale.
	FractionalScale bool
}

// Inspect snapshots the session for diagnostics.
func (s *Session) Inspect() InspectInfo {
	info := InspectInfo{
		Globals:         s.Globals(),
		Outputs:         slices.Clone(s.outputs),
		FractionalScale: s.viewporter != nil && s.fracScaleManager != nil,
	}
	info.CursorTheme, info.CursorSize, info.CursorErr = s.CursorTheme()
	return info
}

// Globals returns a copy of the bound wayland globals mapped to the
// registry version each was advertised with.
func (s *Session) Globals() map[string]uint32 {
	out := make(map[string]uint32, len(s.globalVersions))
	maps.Copy(out, s.globalVersions)
	return out
}

// CursorTheme reports the xcursor theme the session resolves pointer
// shapes from: the theme's name and nominal size, as the environment
// names them. Without a shm global no theme can draw, and the error says
// so.
func (s *Session) CursorTheme() (name string, size uint32, err error) {
	if s.shm == nil {
		return "", 0, errNoShm
	}
	name, size = cursorThemeFromEnv(cursorThemeSize)
	return name, size, nil
}
