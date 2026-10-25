package widget

import "time"

// PointerEvent is a pointer button press as a hook sees it.
type PointerEvent struct {
	// Button is the wayland button code: BTNLeft, BTNRight, BTNMiddle,
	// or a side button beyond them.
	Button uint32
	// Clicks counts the presses of this button on this widget, each
	// within the double-click window of the last: 1, 2 for a double
	// press, 3 for a triple.
	Clicks int
	// Mods are the keyboard modifiers held.
	Mods Mods
	// At is where the press landed, in root coordinates.
	At Point
}

// ScrollEvent is a wheel step as a hook sees it, in ScrollBy's steps:
// DY positive downward, DX positive rightward.
type ScrollEvent struct {
	DX, DY int
	// Mods are the keyboard modifiers held.
	Mods Mods
	// At is where the pointer rests, in root coordinates.
	At Point
}

// pointerClaimer takes a press before the router's own handling;
// true consumes it.
type pointerClaimer interface {
	claimPointer(e PointerEvent) bool
}

// scrollClaimer takes a wheel step before the scroll walk goes on;
// true consumes it. listensScroll reports whether a hook is set, so a
// widget without one costs the walk nothing.
type scrollClaimer interface {
	listensScroll() bool
	claimScroll(e ScrollEvent) bool
}

// SetOnPointer registers fn to see every button press on the widget or
// a descendant that is not a control of its own (a button inside keeps
// its presses), any button with its click count and modifiers. A true
// return consumes the press: no click, focus move, context menu or
// drag follows. The nearest registered widget sees it first. nil
// unregisters.
func (n *node) SetOnPointer(fn func(e PointerEvent) bool) { n.onPointer = fn }

// SetOnScroll registers fn to see the wheel steps over the widget or a
// descendant before any scroller between them; a true return consumes
// the step, false lets it scroll on. nil unregisters.
func (n *node) SetOnScroll(fn func(e ScrollEvent) bool) { n.onScroll = fn }

func (n *node) claimPointer(e PointerEvent) bool { return n.onPointer != nil && n.onPointer(e) }

func (n *node) listensScroll() bool { return n.onScroll != nil }

func (n *node) claimScroll(e ScrollEvent) bool { return n.onScroll != nil && n.onScroll(e) }

// claimPointer offers e to the pointer hooks from hit up to the first
// control, the press's own widget, and reports whether one consumed it.
func claimPointer(hit Widget, e PointerEvent) bool {
	for w := hit; w != nil; w = parentOf(w) {
		if c, ok := w.(pointerClaimer); ok && c.claimPointer(e) {
			return true
		}
		if isControl(w) {
			return false
		}
	}
	return false
}

// pressCount counts a press of button on hit: one more than the last
// when that was the same button on the same widget within the
// double-click window, else 1.
func (r *Router) pressCount(hit Widget, button uint32) int {
	now := time.Now()
	if hit == r.lastPress.w && button == r.lastPress.button && now.Sub(r.lastPress.at) < doubleClickWindow {
		r.lastPress.count++
	} else {
		r.lastPress.count = 1
	}
	r.lastPress.w, r.lastPress.button, r.lastPress.at = hit, button, now
	return r.lastPress.count
}
