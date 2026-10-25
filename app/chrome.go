package app

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/stubbedev/lys-ui/widget"
)

// Client-side window chrome wiring (#91): the widgets live in the
// widget package (HeaderBar, WindowControls, ActionBar, MenuBar);
// this file is the app half - the runtime window operations the
// controls and the header's grab route into, and the one-call
// attach helpers that wire a bar to its window.
//
// Decoration policy: xdg-decoration stays as it is - when the
// compositor offers server-side decoration the window asks for it and
// keeps its passive edge handles; a HeaderBar in the tree is an app's
// explicit opt into client-side chrome, the GTK4 default inverted:
// lys-ui windows are undecorated unless the app draws chrome or the
// compositor decorates them. Both can coexist (a compositor frame
// around an app-drawn header); the app is always right about what it
// draws.

// SetTitle updates the compositor-visible window title at runtime -
// taskbars, alt-tab, wherever the environment shows it. The app's own
// HeaderBar text is separate and stays the app's to update.
func (w *Window) SetTitle(title string) {
	if w.win != nil {
		_ = w.win.SetTitle(title)
	}
}

// Minimize asks the compositor to minimize the window; a hint
// compositors without a minimized concept ignore. See Maximize for
// the state-request contract.
func (w *Window) Minimize() {
	if w.win != nil {
		_ = w.win.Minimize()
	}
}

// ToggleMaximize flips between maximized and restored by the last
// configure-confirmed state - the header double-press contract.
func (w *Window) ToggleMaximize() {
	if w.stateMaximized() {
		w.Unmaximize()
		return
	}
	w.Maximize()
}

// AttachHeader wires a HeaderBar to its window: close, minimize, and
// maximize controls hooked, the double-press mapped to
// maximize/restore. The press-to-move grab needs no wiring - the input
// pipeline routes any WindowMover press into xdg_toplevel.move. The
// minimize and maximize buttons follow the compositor's
// wm_capabilities: shown until it says otherwise (a compositor that
// never says - toplevel before v5 - offers everything), hidden, and
// the double-press inert, for what it declares it cannot do.
func (a *Application) AttachHeader(win *Window, bar *widget.HeaderBar) {
	controls := bar.Controls()
	controls.ShowClose(true)
	controls.OnClose = win.Close
	controls.OnMinimize = win.Minimize
	controls.OnMaximize = win.ToggleMaximize
	apply := func(c WMCapabilities) {
		controls.ShowMinimize(c.Minimize)
		controls.ShowMaximize(c.Maximize)
		bar.OnDoubleClick = nil
		if c.Maximize {
			bar.OnDoubleClick = win.ToggleMaximize
		}
	}
	apply(win.Capabilities())
	win.OnCapabilities(apply)
}

// AttachSplitButton wires a SplitButton's arrow half to items: the
// arrow opens the menu popover below itself.
func (a *Application) AttachSplitButton(host Host, b *widget.SplitButton, items []widget.MenuItem) {
	b.OnMenu = func(anchor widget.Boundser) {
		if _, err := a.OpenMenuPopover(host, MenuPopoverConfig{Anchor: anchor, Face: a.resolveFace(nil), Items: items}); err != nil {
			a.reportPopup(fmt.Errorf("app: open a split button's menu: %w", err))
		}
	}
}

// stateMaximized reports the confirmed maximized state for the
// double-click toggle (tests).
func (w *Window) stateMaximized() bool {
	hw := w.app.hostWindowOf(w)
	if hw == nil || hw.state == nil {
		return false
	}
	return hw.state().Maximized
}

// AttachFontButton makes b open the font chooser dialog over parent on
// click and apply the pick.
func (a *Application) AttachFontButton(parent *Window, b *widget.FontButton) {
	b.OnOpen = func(current widget.FontChoice) {
		_, _ = a.FontChooserDialog(parent, FontChooserConfig{Family: current.Family, Size: current.Size, OnFont: func(family string, size float64) {
			b.Choose(widget.FontChoice{Family: family, Size: size})
		}})
	}
}

// AttachTabView binds libadwaita's tab shortcuts within scope (the
// subtree holding the view and its bar): ctrl+Tab and ctrl+PageDown
// select the next page, ctrl+shift+Tab and ctrl+PageUp the previous,
// and alt+1 through alt+9 the page at that position (alt+9 the last).
func (a *Application) AttachTabView(scope widget.Widget, v *widget.TabView) error {
	bindings := map[string]func(){
		"ctrl+Tab":       v.SelectNext,
		"ctrl+Next":      v.SelectNext,
		"ctrl+shift+Tab": v.SelectPrevious,
		"ctrl+Prior":     v.SelectPrevious,
		"alt+9":          func() { v.SelectIndex(v.NPages() - 1) },
	}
	for i := 1; i <= 8; i++ {
		bindings["alt+"+strconv.Itoa(i)] = func() { v.SelectIndex(i - 1) }
	}
	for _, keys := range slices.Sorted(maps.Keys(bindings)) {
		if err := a.AddScopedAccel(scope, keys, widget.NewAction("tab "+keys, bindings[keys])); err != nil {
			return fmt.Errorf("app: tab shortcuts: %w", err)
		}
	}
	return nil
}
