package widget

import (
	"slices"
	"weak"

	"github.com/stubbedev/lys-ui/anim"
)

type hosting struct{ clock *anim.Clock }

var (
	hosted   = map[any]*hosting{}
	awaiting []weak.Pointer[node]
)

// Host marks root as the top of a tree a host shows on screen, a
// window's, a popover's or a toast's, whose animations run on clock,
// and returns the call that ends it. Only widgets under a hosted root
// animate: a subtree removed from its tree, a tree whose window closed,
// or one a test arranged and dropped schedules no frames, and what it
// launches waits until its tree is hosted again. Hosting a root again
// moves it to the later host; ending the earlier one leaves it there.
func Host(root Widget, clock *anim.Clock) (unhost func()) {
	checkLoop("Host")
	if clock == nil {
		panic("widget: Host: nil clock")
	}
	key := hostKey(root)
	h := &hosting{clock: clock}
	hosted[key] = h
	return func() {
		if hosted[key] == h {
			delete(hosted, key)
		}
	}
}

func hostKey(w Widget) any {
	if n := nodeOf(w); n != nil {
		return n
	}
	return w
}

func topKey(n *node) any {
	for {
		p := n.parent
		if p == nil {
			return n
		}
		for nodeOf(p) == nil {
			up := parentOf(p)
			if up == nil {
				return p
			}
			p = up
		}
		n = nodeOf(p)
	}
}

func (n *node) hostClock() *anim.Clock {
	if h := hosted[topKey(n)]; h != nil {
		return h.clock
	}
	return nil
}

func onScreen(w Widget) bool {
	n := nodeOf(w)
	if n == nil || n.bounds.Empty() {
		return false
	}
	for a := w; a != nil; a = parentOf(a) {
		if !ownVisible(a) {
			return false
		}
	}
	return hosted[topKey(n)] != nil
}

// ScheduleAnimation starts l on the clock of the host whose tree holds
// the widget, now or, while no hosted tree holds it, once one does
// (AttachAnimations): anim.Owner.
func (n *node) ScheduleAnimation(l *anim.Launch) {
	if c := n.hostClock(); c != nil {
		l.Start(c)
		return
	}
	if len(n.awaiting) == 0 {
		awaiting = append(awaiting, weak.Make(n))
	}
	n.awaiting = append(slices.DeleteFunc(n.awaiting, (*anim.Launch).Cancelled), l)
}

// AttachAnimations starts what widgets launched while no hosted tree held
// them, for each widget a hosted tree holds now, on that host's clock.
// The application loop calls it once its windows have arranged; a test
// that hosts a tree and arranges it calls it before it ticks.
func AttachAnimations() {
	checkLoop("AttachAnimations")
	kept := awaiting[:0]
	for _, wp := range awaiting {
		n := wp.Value()
		if n == nil {
			continue
		}
		n.awaiting = slices.DeleteFunc(n.awaiting, (*anim.Launch).Cancelled)
		if len(n.awaiting) == 0 {
			continue
		}
		c := n.hostClock()
		if c == nil {
			kept = append(kept, wp)
			continue
		}
		for _, l := range n.awaiting {
			l.Start(c)
		}
		n.awaiting = nil
	}
	clear(awaiting[len(kept):])
	awaiting = kept
}
