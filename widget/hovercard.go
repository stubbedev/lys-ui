package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/anim"
)

// The hover card's delays.
const (
	hoverCardOpenDelay  = 700 * time.Millisecond
	hoverCardCloseDelay = 300 * time.Millisecond
)

// HoverCard previews content beside a trigger: resting the pointer on
// the trigger opens the card after a delay, and it stays open while the
// pointer is on the trigger or the card, closing a moment after it
// leaves both. Keyboard focus on the trigger opens it the same way. The
// card takes no input grab: what the user points at keeps the keyboard.
// It styles as the `hovercard` element; the card's content as a
// .hovercard-content box.
type HoverCard struct {
	composite
	content Widget
	body    *hoverCardBody
	// trigger and card report the pointer or focus on the trigger and
	// the pointer on the card.
	trigger, card bool
	timer         anim.Cancel
	close         func()
}

// hoverCardBody is the card's content box: it tells the card while the
// pointer is on it.
type hoverCardBody struct {
	*Box
	hc *HoverCard
}

// hoverWithin keeps the card open while the pointer is on it.
func (b *hoverCardBody) hoverWithin(on bool) {
	b.hc.card = on
	b.hc.settle()
}

// NewHoverCard wraps trigger with a card showing content.
func NewHoverCard(trigger, content Widget) *HoverCard {
	h := &HoverCard{content: content}
	h.body = &hoverCardBody{Box: NewBox(Column, 0, 0), hc: h}
	h.body.AddClass("hovercard-content")
	h.body.Append(content, true)
	h.SetElement("hovercard")
	h.initComposite(h, trigger)
	h.boxed = true
	return h
}

// Open reports whether the card is showing.
func (h *HoverCard) Open() bool { return h.close != nil }

// hoverWithin tracks the pointer on the trigger.
func (h *HoverCard) hoverWithin(on bool) {
	h.trigger = on
	h.settle()
}

// FocusMoved implements FocusObserver: visible keyboard focus inside
// the trigger opens the card as the pointer does.
func (h *HoverCard) FocusMoved(_, to Widget) {
	h.trigger = inSubtree(to, h) && FocusVisible(to)
	h.settle()
}

// settle schedules the card toward what the pointer and focus ask: open
// after the open delay, closed after the close delay, nothing pending
// when it already is what they ask.
func (h *HoverCard) settle() {
	want := h.trigger || h.card
	if h.timer != nil {
		h.timer()
		h.timer = nil
	}
	if want == h.Open() {
		return
	}
	delay := hoverCardCloseDelay
	if want {
		delay = hoverCardOpenDelay
	}
	h.timer = anim.Start(delay, func(t float64) {
		if t < 1 {
			return
		}
		h.timer = nil
		if want {
			h.show()
		} else {
			h.hide()
		}
	})
}

// show opens the card under the trigger.
func (h *HoverCard) show() {
	if h.Open() {
		return
	}
	h.close = OpenPopover(h, h, h.body, PopoverOptions{NoGrab: true, Gap: popoverGap, OnClosed: func() { h.close = nil }})
}

// hide closes the card.
func (h *HoverCard) hide() {
	if h.close != nil {
		h.close()
		h.close = nil
	}
}
