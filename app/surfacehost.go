package app

import (
	"fmt"

	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// surfaceHost serves a surface's widgets (widget.SurfaceHost): menu
// popovers over host, nested under parent when the surface is a
// popover, and focus moves through the surface's router.
type surfaceHost struct {
	a      *Application
	host   Host
	parent *Popover
	router *widget.Router
}

// Focus implements widget.SurfaceHost.
func (p surfaceHost) Focus(w widget.Widget) { p.router.SetFocus(w) }

// OpenMenu implements widget.SurfaceHost.
func (p surfaceHost) OpenMenu(anchor widget.Boundser, face render.Font, sizePx float64, items []widget.MenuItem, onClosed func()) {
	cfg := MenuPopoverConfig{Anchor: anchor, Face: face, SizePx: sizePx, Items: items, Parent: p.parent, OnClosed: onClosed}
	if _, err := p.a.OpenMenuPopover(p.host, cfg); err != nil {
		p.a.reportPopup(fmt.Errorf("app: open a widget's menu: %w", err))
	}
}

// OpenPopover implements widget.SurfaceHost; a popover that cannot
// open is reported and closes as nothing.
func (p surfaceHost) OpenPopover(anchor widget.Boundser, content widget.Widget, opts widget.PopoverOptions) func() {
	pop, err := p.a.OpenPopover(p.host, PopoverConfig{
		Anchor: anchor, Content: content, Focus: opts.Focus, Parent: p.parent,
		NoAutohide: opts.NoGrab, OnClosed: opts.OnClosed, Gap: opts.Gap,
	})
	if err != nil {
		p.a.reportPopup(fmt.Errorf("app: open a widget's popover: %w", err))
		return func() {}
	}
	return pop.Dismiss
}

// SetPopupErrorHandler installs fn to hear popups a widget asked for
// (widget.OpenMenu, widget.OpenPopover) that could not open, so the app can show a toast.
// fn runs on the loop goroutine.
func (a *Application) SetPopupErrorHandler(fn func(err error)) { a.onPopupError = fn }

// reportPopup hands a failed popup to the installed handler.
func (a *Application) reportPopup(err error) {
	debug.Log("popup", "%v", err)
	if a.onPopupError != nil {
		a.onPopupError(err)
	}
}
