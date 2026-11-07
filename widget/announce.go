package widget

import (
	"slices"
	"sync/atomic"
)

// Politeness is how urgently an announcement interrupts an assistive
// technology: AT-SPI's live politeness, ARIA's aria-live.
type Politeness uint8

const (
	// Polite waits for the screen reader to finish what it says.
	Polite Politeness = iota + 1
	// Assertive interrupts it.
	Assertive
)

// Announcement is text an assistive technology speaks without the focus
// moving: the widget it comes from, the text, and how urgently.
type Announcement struct {
	Widget     Widget
	Text       string
	Politeness Politeness
}

// announcing is whether an assistive technology listens: the AT-SPI
// bridge turns it on while it serves the application. Off, Announce
// does nothing at all.
var announcing atomic.Bool

// announcements are what waits for the bridge's next pass, one per
// widget: a live region changing again before it is spoken replaces its
// text. Loop goroutine only.
var announcements []Announcement

// SetAnnouncementsHeard tells the toolkit whether an assistive technology
// listens to announcements; the accessibility bridge sets it as it
// starts and stops. Turning it off drops what waits. Loop goroutine
// only.
func SetAnnouncementsHeard(on bool) {
	checkLoop("SetAnnouncementsHeard")
	announcing.Store(on)
	if !on {
		clear(announcements)
		announcements = announcements[:0]
	}
}

// Announce has assistive technology speak text for w without moving the
// focus: a notification appearing, a value the user asked for. It costs
// nothing while no assistive technology listens. Call it on the loop
// goroutine.
func Announce(w Widget, text string, p Politeness) {
	if !announcing.Load() || text == "" || w == nil {
		return
	}
	checkLoop("Announce")
	queueAnnouncement(Announcement{Widget: w, Text: text, Politeness: p})
}

// queueAnnouncement adds a, replacing what its widget still has
// waiting.
func queueAnnouncement(a Announcement) {
	if i := slices.IndexFunc(announcements, func(o Announcement) bool { return o.Widget == a.Widget }); i >= 0 {
		announcements[i] = a
	} else {
		announcements = append(announcements, a)
	}
	MarkA11yChanged()
}

// TakeAnnouncements appends what waits to buf, in the order it was
// announced, and empties the queue: the bridge's pass speaks each once.
// Loop goroutine only.
func TakeAnnouncements(buf []Announcement) []Announcement {
	buf = append(buf, announcements...)
	clear(announcements)
	announcements = announcements[:0]
	return buf
}

// SetLiveRegion makes w a live region: text changing inside it (a
// label's, the widget's own) is announced with politeness p, once per
// accessibility pass however often it changes, with the latest text.
// Zero makes it an ordinary widget again.
func SetLiveRegion(w Widget, p Politeness) {
	if n := nodeOf(w); n != nil {
		n.live = p
	}
}

// LiveRegion returns w's politeness as a live region, zero when it is
// none.
func LiveRegion(w Widget) Politeness {
	if n := nodeOf(w); n != nil {
		return n.live
	}
	return 0
}

// liveTextChanged announces text when w is in a live region: the region
// is w itself or its nearest live ancestor.
func liveTextChanged(w Widget, text string) {
	if !announcing.Load() || text == "" {
		return
	}
	for cur := w; cur != nil; cur = parentOf(cur) {
		if n := nodeOf(cur); n != nil && n.live != 0 {
			queueAnnouncement(Announcement{Widget: cur, Text: text, Politeness: n.live})
			return
		}
	}
}
