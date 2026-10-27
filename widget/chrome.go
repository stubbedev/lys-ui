package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// Client-side window chrome (#91): HeaderBar, ActionBar, and
// WindowControls - the pieces a CSD window draws itself, matching
// adw::HeaderBar's shape. The bar reports window-move grabs through
// the WindowMover interface and the app routes the press into
// xdg_toplevel.move; the control buttons are hooks the app wires to
// its window (the widget never reaches the compositor).

// WindowMover is implemented by widgets whose background area is a
// drag handle for moving the window: a press that resolves into the
// widget (not a deeper control) starts the compositor's move grab.
// HeaderBar carries it; a stylesheet's custom header can too.
type WindowMover interface {
	// WindowMoveGrab reports whether a press on the widget itself (not
	// its children) should move the window.
	WindowMoveGrab() bool
}

// WindowDoubleClickable is the optional second half of WindowMover:
// a double press inside the grab area maximizes, GTK header style.
type WindowDoubleClickable interface {
	// WindowDoubleClick fires on the second press inside the grab
	// interval; the widget routes it where it wants (HeaderBar's
	// OnDoubleClick hook).
	WindowDoubleClick()
}

// DescendsFrom reports whether w is ancestor or a descendant of it -
// the walk the input pipeline uses to find a drag handle under a press
// that resolved into a child control.
func DescendsFrom(w, ancestor Widget) bool {
	for cur := w; cur != nil; {
		if cur == ancestor {
			return true
		}
		cur = parentOf(cur)
	}
	return false
}

// HeaderBar is the CSD title bar: a start pack (left), the window
// title and subtitle (center), and an end pack with the window
// controls (right). Pressing the bar's own background moves the window
// (the app routes it to xdg_toplevel.move); a double press maximizes -
// both delivered through the WindowMover contract, so the bar works in
// any host that honors it. Styled as the `headerbar` element, its
// title as `headerbar title` (docs/css.md).
type HeaderBar struct {
	composite
	packs
	face   render.Font
	sizePx float64

	row         *Box
	center      *Box
	controlSlot *Box
	title       *Label
	sub         *Label
	controls    *WindowControls

	// OnDoubleClick fires on a second press inside the grab interval -
	// the app wires it to maximize/restore.
	OnDoubleClick func()
}

// NewHeaderBar returns a bar sized for face/sizePx content.
func NewHeaderBar(face render.Font, sizePx float64) *HeaderBar {
	face = requireFace("widget.NewHeaderBar", face)
	h := &HeaderBar{face: face, sizePx: sizePx}
	h.SetElement("headerbar")
	h.title = NewLabel(face, sizePx, "", 0)
	h.sub = NewLabel(face, sizePx-3, "", 0)
	h.sub.AddClass(css.DimLabel)
	h.title.SetElement("title")
	h.sub.SetElement("subtitle")
	h.center = NewBox(Column, 1, 0)
	h.center.Append(h.title, false)
	h.center.Append(h.sub, false)
	h.initPacks(h, 6)
	h.controlSlot = NewBox(Row, 0, 0)
	h.row = NewBox(Row, 8, 6)
	h.row.Append(h.start, false)
	h.row.Append(NewSpacer(0, 0), true)
	h.row.AppendAligned(h.center, false, AlignCenter)
	h.row.Append(NewSpacer(0, 0), true)
	h.row.Append(h.end, false)
	h.row.AppendAligned(h.controlSlot, false, AlignCenter)
	h.initComposite(h, h.row)
	h.SetHExpand(true)
	h.minHeight = int(sizePx) + 22
	h.surface = surfaceFill
	return h
}

// SetTitle sets the window title; SetSubtitle the line under it (an
// empty subtitle hides the row).
func (h *HeaderBar) SetTitle(t string) { h.title.SetText(t) }

// SetSubtitle sets the subtitle; empty hides it.
func (h *HeaderBar) SetSubtitle(s string) { h.sub.SetText(s) }

// Controls returns the window controls, wiring them is the app's job:
//
//	bar.Controls().ShowClose(true)
//	bar.Controls().OnClose = window.Close
func (h *HeaderBar) Controls() *WindowControls {
	if h.controls == nil {
		h.controls = NewWindowControls(h.face, h.sizePx)
		h.controlSlot.Append(h.controls, false)
	}
	return h.controls
}

// WindowMoveGrab implements WindowMover: presses on the bar's own
// chrome move the window.
func (h *HeaderBar) WindowMoveGrab() bool { return true }

// WindowDoubleClick implements WindowDoubleClickable: the second
// press fires the app-wired hook (maximize/restore).
func (h *HeaderBar) WindowDoubleClick() {
	if h.OnDoubleClick != nil {
		h.OnDoubleClick()
	}
}

// HitTest resolves into the composed row with the CSD rule applied:
// a press that lands on a button stays a button press; anything else
// on the bar - background, title, spacers - is the move surface and
// resolves to the bar itself.
func (h *HeaderBar) HitTest(p Point) Widget {
	if !h.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := HitChild(h.row, p); hit != nil {
		for cur := hit; cur != nil; cur = parentOf(cur) {
			if b, ok := cur.(*Button); ok {
				return b
			}
			if cur == Widget(h.row) {
				break
			}
		}
	}
	return h
}

// WindowControls are the CSD frame buttons: close, minimize, maximize.
// Each is shown only when asked - the compositor's capabilities decide
// which an app offers - and each fires its hook on click.
type WindowControls struct {
	composite
	face   render.Font
	sizePx float64
	row    *Box

	// OnClose, OnMinimize, and OnMaximize fire for the corresponding
	// button; the app wires them to its window.
	OnClose    func()
	OnMinimize func()
	OnMaximize func()

	closeBtn *Button
	minBtn   *Button
	maxBtn   *Button
}

// NewWindowControls returns a controls cluster with every button
// hidden; Show* turns them on.
func NewWindowControls(face render.Font, sizePx float64) *WindowControls {
	face = requireFace("widget.NewWindowControls", face)
	c := &WindowControls{face: face, sizePx: sizePx}
	c.row = NewBox(Row, 4, 0)
	c.initComposite(c, c.row)
	return c
}

// ShowClose shows the close button (a cross), wired to OnClose.
func (c *WindowControls) ShowClose(on bool) { c.toggle(&c.closeBtn, SymbolClose, &c.OnClose, on) }

// ShowMinimize shows the minimize button (an underscore), wired to
// OnMinimize.
func (c *WindowControls) ShowMinimize(on bool) {
	c.toggle(&c.minBtn, SymbolMinimize, &c.OnMinimize, on)
}

// ShowMaximize shows the maximize button (a square), wired to
// OnMaximize.
func (c *WindowControls) ShowMaximize(on bool) {
	c.toggle(&c.maxBtn, SymbolMaximize, &c.OnMaximize, on)
}

// Shown reports which buttons are shown.
func (c *WindowControls) Shown() (closeBtn, minimize, maximize bool) {
	return c.closeBtn != nil, c.minBtn != nil, c.maxBtn != nil
}

// toggle adds or removes one button.
func (c *WindowControls) toggle(slot **Button, glyph SymbolKind, hook *func(), on bool) {
	if on == (*slot != nil) {
		return
	}
	if !on {
		if i := c.rootChildIndex(*slot); i >= 0 {
			c.row.RemoveAt(i)
		}
		*slot = nil
		c.InvalidateLayout()
		return
	}
	btn := NewButton(NewSymbol(glyph, int(c.sizePx)), 6, 3)
	btn.OnClick = func() {
		if *hook != nil {
			(*hook)()
		}
	}
	// A button shown again (the compositor's capabilities changed)
	// returns to its own place: the row keeps close, minimize,
	// maximize order whatever order they were shown in.
	at := 0
	for _, s := range [...]**Button{&c.closeBtn, &c.minBtn, &c.maxBtn} {
		if s == slot {
			break
		}
		if *s != nil {
			at++
		}
	}
	*slot = btn
	c.row.InsertAt(at, btn, false)
	c.InvalidateLayout()
}

// childIndex finds a child's slot; buttons keep no other state.
func (c *WindowControls) rootChildIndex(w Widget) int {
	for i, child := range c.row.Children() {
		if child == w {
			return i
		}
	}
	return -1
}

// ActionBar is the bottom bar: one pack, centered content optional -
// the gtk ActionBar shape. PackStart/PackEnd as on HeaderBar.
type ActionBar struct {
	composite
	packs
}

// NewActionBar returns an empty bottom bar.
func NewActionBar() *ActionBar {
	a := &ActionBar{}
	a.SetElement("actionbar")
	a.initPacks(a, 6)
	row := NewBox(Row, 8, 6)
	row.Append(a.start, false)
	row.Append(NewSpacer(0, 0), true)
	row.Append(a.end, false)
	a.initComposite(a, row)
	a.SetHExpand(true)
	a.surface = surfaceFill
	return a
}
