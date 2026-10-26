package widget

import (
	"slices"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/transfer"
)

// doubleClickWindow is the maximum gap between the clicks of a
// double-click.
const doubleClickWindow = 400 * time.Millisecond

// BTNLeft is the wayland button code of the primary pointer button.
const BTNLeft uint32 = 0x110

// BTNRight is the wayland button code of the secondary pointer button.
const BTNRight uint32 = 0x111

// BTNMiddle is the wayland button code of the middle pointer button,
// the X11 paste button.
const BTNMiddle uint32 = 0x112

// ContentPaster takes ctrl+v content in formats of its own instead of
// text: PasteMimes lists the mimes it reads, best first, and
// PasteContent receives the best one the clipboard offers. widget.Image
// reads images (decoding off the loop goroutine); a file list would
// read transfer.MimeURIList. Holding this interface is what makes a
// widget a non-text paste target.
type ContentPaster interface {
	PasteMimes() []string
	PasteContent(mime string, data []byte)
}

// TooltipMarkupper is a widget whose tooltip may be RichLabel markup;
// every node is one (SetTooltipMarkup).
type TooltipMarkupper interface {
	TooltipMarkup() (markup string, ok bool)
}

// HoverSetter receives hover tracking from the Router.
type HoverSetter interface {
	SetHovered(on bool)
}

// PressSetter receives press tracking from the Router.
type PressSetter interface {
	SetPressed(on bool)
}

// PressAter receives where a press landed, right after SetPressed(true):
// a slider warps its value there (gtk-primary-button-warps-slider).
type PressAter interface {
	PressAt(p Point)
}

// Clicker is invoked when a press and release land on the same widget.
// p is the release point in root coordinates.
type Clicker interface {
	ClickAt(p Point)
}

// DragMover receives pointer motion while the widget is pressed.
type DragMover interface {
	DragMove(p Point)
}

// PressEnder receives the end of a press gesture: the release (after
// any click), a cancelled press (the gesture became a data-device
// drag), or pointer loss (the device went away mid-gesture). Widgets
// that accumulate gesture state across DragMove calls — a list's
// rubber-band selection, say — settle and flush it here, so a gesture
// that ends off-widget still concludes exactly once.
type PressEnder interface {
	PressEnd()
}

// CursorNamer lets a widget request a pointer shape while hovered,
// using xcursor names ("xterm" for text, "left_ptr" arrow default).
type CursorNamer interface {
	// CursorName returns the xcursor shape for the hovered widget.
	CursorName() string
}

// TooltipTexter exposes a widget's hover tooltip text.
type TooltipTexter interface {
	// TooltipText returns the tooltip, empty when none is set.
	TooltipText() string
}

// TooltipAnchorer is a widget whose tooltip belongs to a part of it,
// a calendar's hovered day: the tooltip centers under that rect, in the
// tree's coordinates, rather than under the widget's bounds.
type TooltipAnchorer interface {
	TooltipAnchor() render.Rect
}

// IsInteractive reports whether w or any ancestor consumes presses:
// buttons, sliders, toggles, text inputs, and scroll areas. Hit tests
// return the deepest widget, often a plain label inside a control, so
// apps checking "was this chrome?" must walk up. Parents are recorded
// during Arrange, so call it on an arranged tree. Enabled state does
// not change the answer: a disabled button is still interactive — it
// consumes the press and swallows it — never chrome.
func IsInteractive(w Widget) bool { return interactiveWithin(w, nil) }

// interactiveWithin is IsInteractive's walk stopped at stop (exclusive):
// whether a press at w is consumed by a control at or below stop - a
// selectable container's item asks it to leave controls inside the
// item their clicks.
func interactiveWithin(w, stop Widget) bool {
	for ; w != nil && w != stop; w = parentOf(w) {
		if isControl(w) {
			return true
		}
	}
	return false
}

// isControl reports whether w itself consumes presses.
func isControl(w Widget) bool {
	switch w.(type) {
	case *Button, *Slider, *Switch, *CheckButton, *Entry, *TextArea, *Scroll, *Dropdown, *List:
		return true
	}
	return false
}

// IsEnabled reports whether w accepts input: its own flag AND every
// ancestor's, so disabling a container disables its subtree by query
// (see node.SetEnabled for why propagation is a walk, not a rewrite).
// Widgets that carry no enable state — third-party Widget
// implementations without an Enabled method — read as enabled, and so
// does nil (nothing there to block). Parents are recorded during
// Arrange, so call it on an arranged tree.
func IsEnabled(w Widget) bool {
	for w != nil {
		if e, ok := w.(interface{ Enabled() bool }); ok && !e.Enabled() {
			return false
		}
		p, ok := w.(interface{ Parent() Widget })
		if !ok {
			return true
		}
		w = p.Parent()
	}
	return true
}

// HoverMover receives pointer motion while the widget is hovered, even
// without a press: menus highlight rows with it.
type HoverMover interface {
	HoverMove(p Point)
}

// ScrollHandler receives scroll deltas in 40px steps: dx from the
// horizontal axis (tilt wheels, trackpads), dy from the vertical.
type ScrollHandler interface {
	ScrollBy(dx, dy int)
}

// DragContent is the payload a drag carries - mime types best first
// with a writer, the actions offered (copy unless set), and the
// source's feedback and completion hooks; see transfer.Drag.
type DragContent = transfer.Drag

// DragSource lets a widget start a drag-and-drop: a press on it plus
// motion past the app's drag threshold offers DragContent through the
// wl_data_device. Return nil to decline this particular press.
type DragSource interface {
	// DragContent returns the offered payload, or nil to not drag.
	DragContent() *DragContent
}

// DragIconer is a DragSource that names the widget its drag icon
// renders, instead of itself: a list dragging several rows shows a
// stack of them.
type DragIconer interface {
	DragIcon() Widget
}

// DragEnterer decides whether a widget accepts a drag hovering it,
// by mime. mimes lists the offered types best first; return the mime
// to receive on drop, or "" to reject the drag.
type DragEnterer interface {
	DragEnter(mimes []string, p Point) string
}

// DragHoverer receives drag movement while a drag hovers the widget,
// for position-aware feedback (drop indicators, insertion gaps).
type DragHoverer interface {
	DragHover(p Point)
}

// DragLeaver receives a drag that left the widget without dropping.
type DragLeaver interface {
	DragLeave()
}

// DropActionChooser is a drop target that picks the action from those
// the source offers - a file view moving within one disk, copying
// across. Without it a drop prefers copy.
type DropActionChooser interface {
	DropAction(offered transfer.Action) transfer.Action
}

// Dropper receives a drop: the accepted mime, its bytes, and the
// drop point in root coordinates.
type Dropper interface {
	Drop(mime string, data []byte, p Point)
}

// DragOverSetter receives drag-hover tracking from the Router, the
// highlight bit while a drag sits over the widget — the drop-target
// counterpart of HoverSetter.
type DragOverSetter interface {
	SetDragOver(on bool)
}

// Axis routes vertical and horizontal scrolling to the hovered widget
// or the nearest ancestor that handles scrolling, mods held. A scroll
// hook (SetOnScroll) on the way sees the step first and consumes it
// only when it says so; a disabled ScrollHandler stops the walk: the
// wheel never scrolls through an inert viewport to an outer one.
func (r *Router) Axis(dx, dy float64, mods Mods) {
	for target := r.hover; target != nil; target = parentOf(target) {
		if r.claimScroll(target, int(dx), int(dy), mods) {
			return
		}
		if sc, ok := target.(ScrollHandler); ok {
			if IsEnabled(target) {
				sc.ScrollBy(int(dx), int(dy))
			}
			return
		}
	}
}

// PixelScroller is a ScrollHandler that takes exact pixel deltas
// (touchpads, continuous devices) as well as wheel steps.
type PixelScroller interface {
	ScrollPixels(dx, dy float64)
}

// ScrollEnder is a PixelScroller told when a finger scroll ends (the
// fingers lifted): Scroll glides on from the gesture's velocity.
type ScrollEnder interface {
	ScrollEnd()
}

// scrollStepPx is one wheel step in pixels, the factor every built-in
// ScrollHandler applies to ScrollBy's steps.
const scrollStepPx = 40

// AxisPixels routes exact scroll deltas like Axis routes steps: to the
// nearest scrolling widget under the pointer, which takes the pixels
// when it is a PixelScroller; any other, and a scroll hook on the way,
// gets whole steps as the pixels add up to them.
func (r *Router) AxisPixels(dx, dy float64, mods Mods) {
	gateTouchAxis(r.hover, &dx, &dy)
	if dx == 0 && dy == 0 && r.pixelTarget == nil {
		return
	}
	var steps [2]int
	stepped := false
	step := func() [2]int {
		if !stepped {
			steps, stepped = r.pixelSteps(dx, dy), true
		}
		return steps
	}
	for target := r.hover; target != nil; target = parentOf(target) {
		if c, ok := target.(scrollClaimer); ok && c.listensScroll() {
			if s := step(); s != [2]int{} && r.claimScroll(target, s[0], s[1], mods) {
				return
			}
		}
		if sc, ok := target.(ScrollHandler); ok {
			if !IsEnabled(target) {
				return
			}
			if p, ok := target.(PixelScroller); ok {
				p.ScrollPixels(dx, dy)
				r.pixelTarget = target
				return
			}
			if s := step(); s != [2]int{} {
				sc.ScrollBy(s[0], s[1])
			}
			return
		}
	}
}

// claimScroll offers a step to target's scroll hook, an enabled one
// that listens, and reports whether it consumed the step.
func (r *Router) claimScroll(target Widget, dx, dy int, mods Mods) bool {
	c, ok := target.(scrollClaimer)
	if !ok || !c.listensScroll() || !IsEnabled(target) {
		return false
	}
	return c.claimScroll(ScrollEvent{DX: dx, DY: dy, Mods: mods, At: r.pointer})
}

// AxisEnd ends a finger scroll: the PixelScroller that took the
// gesture's pixels hears it (ScrollEnder) even when the pointer has
// since moved off it.
func (r *Router) AxisEnd() {
	t := r.pixelTarget
	r.pixelTarget = nil
	if e, ok := t.(ScrollEnder); ok && IsEnabled(t) {
		e.ScrollEnd()
	}
}

// pixelSteps adds pixel deltas to the router's remainder and takes the
// whole wheel steps out of it.
func (r *Router) pixelSteps(dx, dy float64) [2]int {
	r.pxRemainder[0] += dx
	r.pxRemainder[1] += dy
	var out [2]int
	for i := range out {
		out[i] = int(r.pxRemainder[i] / scrollStepPx)
		r.pxRemainder[i] -= float64(out[i] * scrollStepPx)
	}
	return out
}

// Mods is a bitmask of held keyboard modifiers.
type Mods uint8

// Modifier bits, xkb's core modifier indices.
const (
	ModShift Mods = 1 << iota
	ModCapsLock
	ModCtrl
	ModAlt
	// ModSuper is the logo key (Mod4).
	ModSuper Mods = 1 << 6
)

// TabTrapper lets a focused widget absorb a plain Tab press as
// indentation. When the focused widget implements it and TrapTab
// returns true, focus stays; ctrl+Tab and shift+Tab always move focus
// regardless of this interface.
type TabTrapper interface {
	// TrapTab inserts the widget's tab or indent and reports true.
	TrapTab(ctrl bool) bool
}

// KeyAction is an editing or activation action derived from a keyboard
// event.
type KeyAction uint8

// Editing and activation actions, derived from keysyms by
// KeyActionForSym. KeyBackspace is zero, so the zero KeyAction is the
// common delete-backward edit.
const (
	KeyBackspace KeyAction = iota
	KeyDelete
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyEnter
	KeyPriorPage
	KeyNextPage
	KeyDismiss
	KeySpace
	// KeyContextMenu opens the focused widget's context menu: the Menu
	// key, or Shift+F10.
	KeyContextMenu
)

// KeyActionForSym maps a keysym to an editing or activation action.
// Popup hosts use it to route seat keyboard events into popup-local
// widget trees without importing the app package.
func KeyActionForSym(sym xkb.Keysym) (KeyAction, bool) {
	switch sym {
	case xkb.KeyBackSpace:
		return KeyBackspace, true
	case xkb.KeyDelete:
		return KeyDelete, true
	case xkb.KeyLeft:
		return KeyLeft, true
	case xkb.KeyRight:
		return KeyRight, true
	case xkb.KeyUp:
		return KeyUp, true
	case xkb.KeyDown:
		return KeyDown, true
	case xkb.KeyHome:
		return KeyHome, true
	case xkb.KeyEnd:
		return KeyEnd, true
	case xkb.KeyReturn, xkb.KeyKPEnter:
		return KeyEnter, true
	case xkb.KeyPrior:
		return KeyPriorPage, true
	case xkb.KeyNext:
		return KeyNextPage, true
	case xkb.KeyEscape:
		return KeyDismiss, true
	case xkb.KeySpace, xkb.KeyKPSpace:
		return KeySpace, true
	case xkb.KeyMenu:
		return KeyContextMenu, true
	}
	return 0, false
}

// KeyActionHandler receives editing and activation actions with the
// modifiers held at the time.
type KeyActionHandler interface {
	KeyAction(a KeyAction, mods Mods)
}

// RawKeyHandler sees the key presses the action translation does not
// cover — letters and other keysyms that carry no KeyAction — while
// its tree holds the keyboard (a popup menu under a grab, say).
// Returning true consumes the press; a false return hands nothing on
// (the host decides what, if anything, happens next). Mnemonics and
// accelerators ride this path.
type RawKeyHandler interface {
	// RawKey reports whether the press was consumed. code is the
	// evdev keycode, sym its keysym, mods the held modifiers.
	RawKey(code uint32, mods Mods, sym xkb.Keysym) bool
}

// MnemonicActivator is implemented by widgets whose rows carry
// Alt-letter mnemonics (Menu): ActivateMnemonic fires the row bound
// to sym, case-insensitively, and reports whether one fired.
type MnemonicActivator interface {
	ActivateMnemonic(sym xkb.Keysym) bool
}

// ActivateMnemonic fires the first mnemonic bound to sym at or below
// w, in paint order, and reports whether one fired. Hosts that hold
// raw keys for a tree (a popover's key root) route Alt-letter presses
// through here.
func ActivateMnemonic(w Widget, sym xkb.Keysym) bool {
	if w == nil {
		return false
	}
	if ma, ok := w.(MnemonicActivator); ok && ma.ActivateMnemonic(sym) {
		return true
	}
	if c, ok := w.(childser); ok {
		for _, k := range c.Children() {
			if ActivateMnemonic(k, sym) {
				return true
			}
		}
	}
	return false
}

// RuneHandler receives typed characters.
type RuneHandler interface {
	InsertRune(r rune)
}

// Router turns pointer and keyboard input into widget state changes: hover
// tracking, press/release with click detection, drags, scroll bubbling,
// and focus for keyboard text.
//
// The Router is not safe for concurrent use; call it from the same
// goroutine that dispatches the wayland connection.
type Router struct {
	// Root is the widget tree inputs route through.
	Root Widget

	hover, pressed, focus Widget
	// chained is the widget whose ancestry holds :hover: the hovered
	// widget, or over the surface's empty space the root.
	chained  Widget
	dragging bool
	// pointer is where the pointer rests while pointerIn, for Rehover.
	pointer   Point
	pointerIn bool
	// pxRemainder holds pixel scrolling not yet a whole step for a
	// step-only scroller (AxisPixels).
	pxRemainder [2]float64
	// pixelTarget is the PixelScroller the current finger scroll feeds,
	// told when it ends (AxisEnd).
	pixelTarget Widget
	// gestureTarget claimed the gesture in progress (Gesture).
	gestureTarget   Widget
	lastClick       time.Time
	lastClickWidget Widget
	// lastPress is the last press, for PointerEvent.Clicks.
	lastPress struct {
		w      Widget
		button uint32
		at     time.Time
		count  int
	}

	// drop-target state during a wl_data_device drag: the widget the
	// drag is over, the offered mimes, and the mime it accepted.
	dragTarget Widget
	dragMimes  []string
	dragMime   string
}

// Move updates hover state and feeds drags and in-widget hover
// tracking. p is in root coordinates. A hit inside a disabled subtree
// hovers nothing: an inert control shows no hover shade.
func (r *Router) Move(p Point) {
	r.pointer, r.pointerIn = p, true
	hit := r.hitAt(p)
	if r.hover != hit {
		if h, ok := r.hover.(HoverSetter); ok {
			h.SetHovered(false)
		}
		r.hover = hit
		if h, ok := hit.(HoverSetter); ok {
			h.SetHovered(true)
		}
	}
	r.rechain(hit)
	if h, ok := r.hover.(HoverMover); ok {
		h.HoverMove(p)
	}
	if r.dragging {
		if d, ok := r.pressed.(DragMover); ok {
			d.DragMove(p)
		}
	}
}

// rechain moves the :hover chain to w's ancestry; with w nil while the
// pointer is over the surface, to the root's, which the pointer still
// hovers: a gap between widgets leaves the root's chain as it was
// instead of flipping the whole tree's :hover off and back.
func (r *Router) rechain(w Widget) {
	if w == nil && r.pointerIn {
		w = r.Root
	}
	setHoverChain(r.chained, w)
	r.chained = w
}

// hitAt is the enabled widget at p, or nil.
func (r *Router) hitAt(p Point) Widget {
	if hit := r.Root.HitTest(p); IsEnabled(hit) {
		return hit
	}
	return nil
}

// Rehover re-picks the hovered widget under the resting pointer after
// a layout moved the tree beneath it (a scroll, a reflow, a removal):
// hover follows what is under the pointer now, not what was there when
// it last moved. A press in progress keeps its target. Reports whether
// the hover changed.
func (r *Router) Rehover() bool {
	if !r.pointerIn || r.pressed != nil || r.hitAt(r.pointer) == r.hover {
		return false
	}
	r.Move(r.pointer)
	return true
}

// Press records a pointer button press, mods held. p is in root
// coordinates. A pointer hook (SetOnPointer) sees it first and may
// consume it. A press on a disabled widget is swallowed whole: no
// pressed shade, no drag, and no focus move — focus stays where it
// was, exactly like a press on empty space outside any control leaves
// a text field alone.
func (r *Router) Press(button uint32, p Point, mods Mods) {
	hit := r.Root.HitTest(p)
	if hit != nil && !IsEnabled(hit) {
		return
	}
	if claimPointer(hit, PointerEvent{Button: button, Clicks: r.pressCount(hit, button), Mods: mods, At: p}) {
		return
	}
	if button != BTNLeft {
		if o := contextMenuOpener(hit); button == BTNRight && o != nil {
			o.OpenContextMenu(p)
		}
		return
	}
	if n := nodeOf(hit); n == nil || !n.noFocusOnClick {
		setFocusStyle(r.focus, hit, false)
		r.focus = hit
	}
	r.pressed = hit
	r.dragging = hit != nil
	if pr, ok := hit.(PressSetter); ok {
		pr.SetPressed(true)
	}
	if pa, ok := hit.(PressAter); ok {
		pa.PressAt(p)
	}
}

// setFocusStyle flips the :focus style bits between the old and new
// focus and restyles both — the router is the one writer of focus, so
// the bits stay in step with r.focus by construction. visible marks a
// keyboard-driven focus (:focus-visible); the :focus-within count moves
// along both ancestor chains.
func setFocusStyle(old, new Widget, visible bool) {
	if old == new {
		if n := nodeOf(new); n != nil && n.focused && n.focusVisible != visible {
			n.focusVisible = visible
			n.invalidateState(style.FocusVisible)
		}
		return
	}
	if n := nodeOf(old); n != nil && n.focused {
		n.focused, n.focusVisible = false, false
		n.invalidateState(style.Focus | style.FocusVisible)
		shiftFocusWithin(old, -1)
		if n.onFocus != nil {
			n.onFocus(false)
		}
	}
	if n := nodeOf(new); n != nil && !n.focused {
		n.focused, n.focusVisible = true, visible
		n.invalidateState(style.Focus | style.FocusVisible)
		shiftFocusWithin(new, 1)
		if n.onFocus != nil {
			n.onFocus(true)
		}
	}
	notifyFocusObservers(old, new)
}

// FocusObserver is an ancestor that hears the keyboard focus move
// into, out of or within its subtree: a form validating the field the
// focus just left.
type FocusObserver interface {
	FocusMoved(from, to Widget)
}

// notifyFocusObservers tells every FocusObserver above from or to,
// once each, that the focus moved from from to to.
func notifyFocusObservers(from, to Widget) {
	var told []FocusObserver
	for _, w := range [...]Widget{from, to} {
		for p := w; p != nil; p = parentOf(p) {
			if ob, ok := p.(FocusObserver); ok && !slices.Contains(told, ob) {
				told = append(told, ob)
				ob.FocusMoved(from, to)
			}
		}
	}
}

// shiftFocusWithin moves the :focus-within count of w and every
// ancestor by delta, restyling the ones whose bit flipped.
func shiftFocusWithin(w Widget, delta int) {
	for ; w != nil; w = parentOf(w) {
		n := nodeOf(w)
		if n == nil {
			continue
		}
		was := n.focusWithin > 0
		n.focusWithin = max(0, n.focusWithin+delta)
		if was != (n.focusWithin > 0) {
			n.restyleState(w, style.FocusWithin)
		}
	}
}

// Release finishes a press: a release over the pressed widget clicks
// it — unless the widget was disabled between press and release, in
// which case the click dies with the gesture.
func (r *Router) Release(button uint32, p Point) {
	if button != BTNLeft || r.pressed == nil {
		return
	}
	if pr, ok := r.pressed.(PressSetter); ok {
		pr.SetPressed(false)
	}
	hit := r.Root.HitTest(p)
	if hit == r.pressed && IsEnabled(hit) {
		handled := false
		if hit == r.lastClickWidget && time.Since(r.lastClick) < doubleClickWindow {
			// Only widgets with a double-click behavior consume the
			// second click; everything else treats each release as
			// its own click (two rapid clicks toggle twice).
			if dc, ok := hit.(DoubleClicker); ok {
				dc.DoubleClickAt(p)
				handled = true
			}
		}
		if !handled {
			if c, ok := hit.(Clicker); ok {
				c.ClickAt(p)
			} else if fn := clickWithinOf(hit); fn != nil {
				fn()
			}
		}
		r.lastClick = time.Now()
		r.lastClickWidget = hit
	}
	if pe, ok := r.pressed.(PressEnder); ok {
		pe.PressEnd()
	}
	r.pressed = nil
	r.dragging = false
}

// Leave clears hover state when the pointer leaves the surface.
func (r *Router) Leave() {
	r.pointerIn = false
	if h, ok := r.hover.(HoverSetter); ok {
		h.SetHovered(false)
	}
	r.hover = nil
	r.rechain(nil)
}

// Pressed returns the widget an implicit press is active on, or nil.
func (r *Router) Pressed() Widget { return r.pressed }

// CancelPress drops the active press without a click: a gesture that
// became a data-device drag must not fire the widget on release.
func (r *Router) CancelPress() {
	if pr, ok := r.pressed.(PressSetter); ok {
		pr.SetPressed(false)
	}
	if pe, ok := r.pressed.(PressEnder); ok {
		pe.PressEnd()
	}
	r.pressed = nil
	r.dragging = false
}

// PointerLost ends every pointer-derived state because the pointer
// stopped delivering events: the device went away (wl_seat capability
// loss) or the compositor ended the grab that kept it here. The active
// press is cancelled without a click — its release will never arrive —
// and hover clears, so nothing stays highlighted or half-pressed for a
// device that is gone.
func (r *Router) PointerLost() {
	r.CancelPress()
	r.Leave()
}

// Forget drops every piece of router state that points at w or into
// w's subtree — the widget-removal counterpart of PointerLost. Hover
// clears, so no hover shade or tooltip dwell outlives a removed
// widget; the press is cancelled without a click (its release must
// never click a ghost); the drop target clears, so a drop cannot land
// on detached content; and focus — including a focus on a descendant —
// moves to the next focusable widget in paint order, exactly like
// focusAfter, or nowhere when none follows. Containers fire this
// through the removal hook (SetRemovedHook); descent is decided from
// the parent links the last Arrange recorded, the same bookkeeping
// markSub walks.
func (r *Router) Forget(w Widget) {
	if w == nil {
		return
	}
	if inSubtree(r.hover, w) {
		if h, ok := r.hover.(HoverSetter); ok {
			h.SetHovered(false)
		}
		r.hover = nil
		r.rechain(nil)
	}
	if inSubtree(r.pressed, w) {
		r.CancelPress()
	}
	if inSubtree(r.pixelTarget, w) {
		r.pixelTarget = nil
	}
	if inSubtree(r.gestureTarget, w) {
		r.gestureTarget = nil
	}
	if inSubtree(r.focus, w) {
		setFocusStyle(r.focus, nil, false)
		r.focus = r.focusAfter(w)
		setFocusStyle(nil, r.focus, true)
	}
	if inSubtree(r.dragTarget, w) {
		r.applyDragTarget(nil, false)
		r.dragMime = ""
	}
}

// inSubtree reports whether w sits at or below root along the parent
// links the last Arrange recorded.
func inSubtree(w, root Widget) bool {
	if w == nil || root == nil {
		return false
	}
	for c := w; c != nil; c = parentOf(c) {
		if c == root {
			return true
		}
	}
	return false
}

// dragHandlerAt walks up from the widget at p to the nearest ancestor
// that decides drag acceptance. Hit tests return the deepest widget —
// a label inside a row — so drop targets are found on the parent
// chain, the same way Axis finds the scroll handler.
func (r *Router) dragHandlerAt(p Point) Widget {
	for w := r.Root.HitTest(p); w != nil; w = parentOf(w) {
		if _, ok := w.(DragEnterer); ok {
			return w
		}
	}
	return nil
}

// DragEnter routes a drag entering the tree: find the drop target at
// p, ask it for a decision by mime, and highlight it only when it
// accepted. Returns the accepted mime ("" rejects).
func (r *Router) DragEnter(mimes []string, p Point) string {
	r.dragMimes = mimes
	w := r.dragHandlerAt(p)
	mime := ""
	if d, ok := w.(DragEnterer); ok {
		mime = d.DragEnter(mimes, p)
	}
	r.applyDragTarget(w, mime != "")
	r.dragMime = mime
	return mime
}

// DragHover routes drag movement within the tree. The compositor only
// sends enter on surface changes, so crossing between targets inside
// one surface retargets here, with the mimes from the enter.
func (r *Router) DragHover(p Point) string {
	w := r.dragHandlerAt(p)
	if w != r.dragTarget {
		return r.DragEnter(r.dragMimes, p)
	}
	if h, ok := w.(DragHoverer); ok {
		h.DragHover(p)
	}
	return r.dragMime
}

// DragLeave clears the drop target after the drag left the surface or
// ended without a drop.
func (r *Router) DragLeave() {
	r.applyDragTarget(nil, false)
	r.dragMime = ""
}

// Drop delivers the drop to the current drop target and clears the
// hover state. data carries the payload bytes the app fetched from
// the offer — or through the same-process shortcut.
func (r *Router) Drop(mime string, data []byte, p Point) {
	w := r.dragTarget
	r.applyDragTarget(nil, false)
	r.dragMime = ""
	if d, ok := w.(Dropper); ok {
		d.Drop(mime, data, p)
	}
}

// DragMime returns the mime the current drop target accepted, empty
// while nothing is accepted; the payload is fetched for it on drop.
func (r *Router) DragMime() string { return r.dragMime }

// DragAction is the current drop target's pick among offered
// (DropActionChooser), ActionNone to leave the choice to the default.
func (r *Router) DragAction(offered transfer.Action) transfer.Action {
	if c, ok := r.dragTarget.(DropActionChooser); ok && r.dragMime != "" {
		return c.DropAction(offered)
	}
	return transfer.ActionNone
}

// applyDragTarget swaps the drop target, clearing the old one's
// highlight and notifying it of the departure; the new one is
// highlighted only when it accepted the drag.
func (r *Router) applyDragTarget(w Widget, accepted bool) {
	if old := r.dragTarget; old != nil {
		if s, ok := old.(DragOverSetter); ok {
			s.SetDragOver(false)
		}
		if l, ok := old.(DragLeaver); ok {
			l.DragLeave()
		}
	}
	r.dragTarget = w
	if w == nil || !accepted {
		return
	}
	if s, ok := w.(DragOverSetter); ok {
		s.SetDragOver(true)
	}
}

// Axis routes vertical scrolling to the hovered widget or the nearest
// ancestor that handles it.

// KeyAction delivers an editing or activation action to the focused
// widget — never to one that went disabled since it took focus; see
// keyboardTarget. An ancestor KeyInterceptor sees the action first
// (GTK's key controller on a parent), and an ancestor FocusNavigator
// may move focus instead.
func (r *Router) KeyAction(a KeyAction, mods Mods) {
	target := r.keyboardTarget()
	if o := contextMenuOpener(target); a == KeyContextMenu && o != nil {
		at := Point{}
		if b, ok := target.(Boundser); ok {
			rect := b.Bounds()
			at = Point{X: rect.X + rect.W/2, Y: rect.Y + rect.H/2}
		}
		o.OpenContextMenu(at)
		return
	}
	for w := target; w != nil; w = parentOf(w) {
		if ic, ok := w.(KeyInterceptor); ok && ic.InterceptKey(target, a, mods) {
			return
		}
		if nav, ok := w.(FocusNavigator); ok {
			if next := nav.NavigateFocus(target, a); next != nil {
				r.SetFocus(next)
				return
			}
		}
	}
	if h, ok := target.(KeyActionHandler); ok {
		h.KeyAction(a, mods)
	}
}

// KeyInterceptor is an ancestor that pre-empts a focused widget's key
// actions (GTK's KeyControllerKeyPressed on a parent). target is the
// focused widget the action was headed for; InterceptKey returns true
// when it consumed the action.
type KeyInterceptor interface {
	InterceptKey(target Widget, a KeyAction, mods Mods) bool
}

// SetFocusOnClick decides whether a press on the widget moves the
// keyboard focus to it (GTK's focus-on-click, on by default): a
// command palette's rows leave it in the search entry.
func (n *node) SetFocusOnClick(on bool) { n.noFocusOnClick = !on }

// ContextMenuOpener is a widget that opens a context menu for itself
// and its descendants: a right click on one of them, a long press, or
// the Menu key (Shift+F10) while one has the focus. at is where the
// menu opens, the pointer or the focused widget's center.
type ContextMenuOpener interface {
	OpenContextMenu(at Point)
}

// contextMenuOpener is w's or its nearest ancestor's context menu, nil
// for none.
func contextMenuOpener(w Widget) ContextMenuOpener {
	for ; w != nil; w = parentOf(w) {
		if o, ok := w.(ContextMenuOpener); ok && IsEnabled(w) {
			return o
		}
	}
	return nil
}

// FocusNavigator is an ancestor that moves focus among its own
// focusable descendants on navigation keys: the roving focus of an
// accordion's triggers. NavigateFocus returns the widget to focus for
// action a headed for target, or nil to let the action through.
type FocusNavigator interface {
	NavigateFocus(target Widget, a KeyAction) Widget
}

// SelectedTexter exposes the widget's active selection.
type SelectedTexter interface {
	// SelectedText returns the selected text and whether a non-empty
	// selection exists.
	SelectedText() (string, bool)
}

// TextInserter is an editable text widget that accepts pasted text:
// Insert places s at the caret, replacing any active selection, and
// reports the change through OnChanged. Entry and TextArea implement
// it; primary-selection pastes (middle click) target it.
type TextInserter interface {
	// Insert inserts s at the caret, replacing the selection.
	Insert(s string)
}

// Type delivers a typed character to the focused widget, honoring a
// disable-during-focus just like KeyAction.
func (r *Router) Type(ch rune) {
	if h, ok := r.keyboardTarget().(RuneHandler); ok {
		h.InsertRune(ch)
	}
}

// DoubleClicker is invoked on the second click of a double-click; p is
// the release point in root coordinates.
type DoubleClicker interface {
	DoubleClickAt(p Point)
}

// SelectAller clears or sets a full selection on the focused widget.
type SelectAller interface {
	SelectAll()
}

// SelectAll asks the focused widget to select its entire content,
// honoring a disable-during-focus just like KeyAction.
func (r *Router) SelectAll() {
	if s, ok := r.keyboardTarget().(SelectAller); ok {
		s.SelectAll()
	}
}

// Hovered returns the widget currently under the pointer.
func (r *Router) Hovered() Widget { return r.hover }

// Focused returns the widget receiving keyboard input.
func (r *Router) Focused() Widget { return r.focus }

// focusWalker visits the tree in paint order for focus traversal.
func focusWalker(w Widget, fn func(Widget)) {
	walkTree(w, 0, func(w Widget, _ int) { fn(w) })
}

// walkTreeStack is walkTree's per-depth snapshot buffers, reused
// across walks like the damage walk's walkStack: the walk is
// loop-goroutine work, a level's snapshot is a copy taken at visit
// time (so a callback mutating the tree changes the next walk, not
// this one), and a walk must not nest inside another walkTree.
var walkTreeStack [][]Widget

// walkTree visits w and every descendant in paint order (pre-order),
// passing each widget's nesting depth; the root is depth 0. The same
// Children walk focus traversal, the damage collector, and the a11y
// snapshot use. The walk iterates a Children snapshot per container,
// so mutating the tree from the callback sees the pre-mutation set.
func walkTree(w Widget, depth int, fn func(Widget, int)) {
	if w == nil {
		return
	}
	fn(w, depth)
	c, ok := w.(childser)
	if !ok {
		return
	}
	for len(walkTreeStack) <= depth {
		walkTreeStack = append(walkTreeStack, nil)
	}
	walkTreeStack[depth] = walkTreeStack[depth][:0]
	if cb, ok := c.(childBuf); ok {
		walkTreeStack[depth] = cb.appendChildren(walkTreeStack[depth])
	} else {
		walkTreeStack[depth] = append(walkTreeStack[depth], c.Children()...)
	}
	for _, k := range walkTreeStack[depth] {
		walkTree(k, depth+1, fn)
	}
}

// SetFocus moves keyboard focus to w, the programmatic counterpart of
// clicking it: a search entry that should take typing the moment its
// surface maps is focused this way. A w that cannot take focus (no
// KeyActionHandler, disabled, or hidden) or is not in this router's
// tree is ignored and focus stays where it was; nil clears focus.
func (r *Router) SetFocus(w Widget) {
	if w == nil {
		setFocusStyle(r.focus, nil, false)
		r.focus = nil
		return
	}
	if !isFocusable(w) || !inTree(r.Root, w) {
		return
	}
	setFocusStyle(r.focus, w, false)
	r.focus = w
}

// inTree reports whether w is root or sits below it in paint order.
func inTree(root, w Widget) bool {
	found := false
	focusWalker(root, func(x Widget) {
		if x == w {
			found = true
		}
	})
	return found
}

// FocusNext moves focus to the next focusable widget in paint order,
// wrapping around; with no focus it takes the first.
func (r *Router) FocusNext() { r.focusStep(1) }

// FocusPrev moves focus to the previous focusable widget.
func (r *Router) FocusPrev() { r.focusStep(-1) }

// focusStep walks the tree collecting focusable widgets and lands on
// the neighbor of the current focus. Disabled and hidden widgets never
// join the order, so Tab cannot land on one; a focus that went
// unfocusable since it was set moves to the next focusable first.
func (r *Router) focusStep(dir int) {
	r.dropUnfocusable()
	var order []Widget
	focusWalker(r.focusScope(), func(w Widget) {
		if isFocusable(w) {
			order = append(order, w)
		}
	})
	if len(order) == 0 {
		return
	}
	idx := 0
	if r.focus != nil {
		for i, w := range order {
			if w == r.focus {
				idx = (i + dir + len(order)) % len(order)
				break
			}
		}
	} else if dir < 0 {
		idx = len(order) - 1
	}
	setFocusStyle(r.focus, order[idx], true)
	r.focus = order[idx]
	// Keyboard focus scrolls into view (GtkViewport scroll-to-focus).
	RevealRect(r.focus, boundsOf(r.focus))
}

// FocusTrapper is a widget that can hold Tab traversal inside a part
// of itself: a modal sheet over the content. FocusTrap returns that
// part while it traps, nil otherwise.
type FocusTrapper interface {
	FocusTrap() Widget
}

// focusScope is the subtree Tab traverses: the last active trap in
// paint order - the topmost - or the whole tree.
func (r *Router) focusScope() Widget {
	scope := r.Root
	focusWalker(r.Root, func(w Widget) {
		if ft, ok := w.(FocusTrapper); ok {
			if trap := ft.FocusTrap(); trap != nil {
				scope = trap
			}
		}
	})
	return scope
}

// dropUnfocusable moves focus off a widget that stopped accepting it
// since it took it (disabled or hidden, directly or through a
// container): focus falls to the next focusable widget in paint order,
// or nowhere when there is none. Delivery paths (KeyAction, Type,
// SelectAll) and traversal (focusStep) call it before touching
// r.focus, so keys are never fed into a dead control.
func (r *Router) dropUnfocusable() {
	if r.focus == nil || isFocusable(r.focus) {
		return
	}
	next := r.focusAfter(r.focus)
	setFocusStyle(r.focus, next, true)
	r.focus = next
}

// focusAfter returns the first focusable widget after skip
// in paint order (wrapping), or nil when traversal has nowhere to go.
// Nothing at or below skip qualifies — a container detached from the
// tree takes its whole subtree out of the traversal order.
func (r *Router) focusAfter(skip Widget) Widget {
	var order []Widget
	focusWalker(r.Root, func(w Widget) { order = append(order, w) })
	start := 0
	for i, w := range order {
		if w == skip {
			start = i + 1
			break
		}
	}
	for i := range order {
		w := order[(start+i)%len(order)]
		if isFocusable(w) && !inSubtree(w, skip) {
			return w
		}
	}
	return nil
}

// keyboardTarget returns the widget keyboard input should reach,
// dropping a focus that went disabled or hidden since it was set.
func (r *Router) keyboardTarget() Widget {
	r.dropUnfocusable()
	return r.focus
}

// Boundser exposes a widget's arranged rect; the app draws the focus
// ring around whatever widget.Boundser the router focuses.
type Boundser interface {
	Bounds() render.Rect
}

// clickWithinOf is the nearest SetOnClickWithin hook at or above w, nil
// when no enabled ancestor registered one. A clicking widget on the way
// up stops the walk: its own click is the one that counts, so a label
// inside a button never bubbles past the button.
func clickWithinOf(w Widget) func() {
	for at := w; at != nil; at = parentOf(at) {
		if at != w {
			if _, ok := at.(Clicker); ok {
				return nil
			}
		}
		if cw, ok := at.(interface{ clickWithin() func() }); ok {
			if fn := cw.clickWithin(); fn != nil {
				if !IsEnabled(at) {
					return nil
				}
				return fn
			}
		}
	}
	return nil
}

// FocusVisible reports whether w holds a focus that should show
// (:focus-visible): one that arrived by keyboard traversal, not by a
// pointer press or SetFocus.
func FocusVisible(w Widget) bool {
	n := nodeOf(w)
	return n != nil && n.focused && n.focusVisible
}
