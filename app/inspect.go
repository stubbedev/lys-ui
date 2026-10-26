// The inspector hookup: key chords, application state, and per-window
// overlay application. The overlay itself lives in internal/inspect;
// this file decides when it paints and what the chords do. The chords
// fire only while the inspector is armed - LYS_UI_INSPECT=1 at startup
// or an explicit SetInspect/ToggleInspect - so an app that never asks
// for the inspector keeps ctrl+shift+i and ctrl+shift+d to itself.
package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/inspect"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// inspectAction is what an inspector chord asks for.
type inspectAction uint8

const (
	inspectToggle inspectAction = iota + 1
	inspectDump
)

// doctorRequested reports whether the environment asked for the
// diagnostics block at startup: LYS_UI_DOCTOR=1 (or true/yes/on).
func doctorRequested() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LYS_UI_DOCTOR"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// inspectKey maps an inspector chord to its action: ctrl+shift+i
// toggles the widget-tree overlay, ctrl+shift+d dumps the tree to
// stdout - the GTK inspector's muscle memory. Both require ctrl and
// shift, and neither fires with alt held. ok is false for everything
// else, including every chord while the inspector is unarmed (callers
// check arming first).
func inspectKey(sym xkb.Keysym, mods wlsession.Mods) (inspectAction, bool) {
	if mods&wlsession.ModCtrl == 0 || mods&wlsession.ModShift == 0 || mods&wlsession.ModAlt != 0 {
		return 0, false
	}
	switch normalizeSym(sym) {
	case xkb.Keysym('i'):
		return inspectToggle, true
	case xkb.Keysym('d'):
		return inspectDump, true
	}
	return 0, false
}

// SetInspect arms the inspector and shows or hides the widget-tree
// overlay on every window. Arming enables the chords (ctrl+shift+i
// toggles, ctrl+shift+d dumps the tree to stdout); apps binding those
// chords themselves while armed lose out - the inspector consumes
// them first, exactly like the GTK inspector's own binding.
func (a *Application) SetInspect(on bool) { a.setInspect(on) }

// ToggleInspect flips the overlay, arming the inspector on the way.
func (a *Application) ToggleInspect() { a.setInspect(!a.inspectOn) }

// Inspect reports whether the overlay currently paints.
func (a *Application) Inspect() bool { return a.inspectOn }

// setInspect applies overlay visibility everywhere. Every window gets
// a full-repaint rect: annotations change pixels no widget owns, so
// the damage collector alone would leave stale chrome behind.
func (a *Application) setInspect(on bool) {
	a.inspectArmed = true
	a.inspectOn = on
	for _, w := range a.windows {
		w.setInspect(on)
	}
}

// setInspect flips one window's overlay and schedules the full
// repaint that makes the change visible.
func (w *hostWindow) setInspect(on bool) {
	if w.inspector != nil {
		w.inspector.SetOn(on)
	}
	bw, bh := w.layoutSize()
	if bw > 0 && bh > 0 {
		w.pendingRects = append(w.pendingRects, render.Rect{W: bw, H: bh})
	}
	w.dirty = true
}

// dumpTree prints one window's widget tree to stdout: the on-demand
// text form of the overlay, with live focus/hover/press state. The
// wrapped (real) tree dumps, never the overlay node itself.
func (a *Application) dumpTree(w *hostWindow) {
	root := w.router.Root
	if w.inspector != nil {
		root = w.inspector.Root()
	}
	fmt.Fprint(os.Stdout, inspect.Dump(root, w.router))
}
