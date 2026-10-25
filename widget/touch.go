package widget

import (
	"math"
	"time"

	"github.com/stubbedev/lys-ui/anim"
)

// GestureKind is a recognized gesture's kind.
type GestureKind uint8

// Gestures.
const (
	// GestureSwipe is a multi-finger swipe (a touchpad's three
	// fingers): DX/DY carry the motion.
	GestureSwipe GestureKind = iota
	// GesturePinch is a pinch-zoom and rotation (two fingers on a
	// touchscreen, or a touchpad pinch): Scale and Rotation are
	// relative to the gesture's start.
	GesturePinch
	// GestureHold is fingers resting on a touchpad.
	GestureHold
	// GestureLongPress is a contact held still past the long-press
	// delay; consuming it cancels the press's click.
	GestureLongPress
)

// GesturePhase is where a gesture is in its life.
type GesturePhase uint8

// Gesture phases. A long press arrives as one GestureEnd.
const (
	GestureBegin GesturePhase = iota
	GestureUpdate
	GestureEnd
	GestureCancel
)

// Gesture is one gesture event. At is where it happens (the centroid
// for a multi-contact gesture); DX/DY the motion since the last event;
// Scale and Rotation (degrees, clockwise) the pinch relative to its
// start.
type Gesture struct {
	Kind     GestureKind
	Phase    GesturePhase
	At       Point
	Fingers  int
	DX, DY   float64
	Scale    float64
	Rotation float64
}

// GestureHandler takes gestures: returning true from a Begin (or a
// long press) claims the gesture, and its Update/End/Cancel then go
// to the same widget.
type GestureHandler interface {
	Gesture(g Gesture) bool
}

// Gesture routes g: a Begin (or a long press) goes to the widget under
// g.At and up its ancestors until one claims it; the rest of the
// gesture goes to the claimer. It reports whether a widget took it.
func (r *Router) Gesture(g Gesture) bool {
	if g.Phase == GestureBegin || g.Kind == GestureLongPress {
		r.gestureTarget = nil
		for w := r.Root.HitTest(g.At); w != nil; w = parentOf(w) {
			if h, ok := w.(GestureHandler); ok && IsEnabled(w) && h.Gesture(g) {
				if g.Kind != GestureLongPress {
					r.gestureTarget = w
				}
				return true
			}
		}
		return false
	}
	h, ok := r.gestureTarget.(GestureHandler)
	if !ok {
		return false
	}
	if g.Phase == GestureEnd || g.Phase == GestureCancel {
		r.gestureTarget = nil
	}
	h.Gesture(g)
	return true
}

// TouchPointer receives a touch contact's pointer emulation: the host
// routes it through its own pointer path (window-frame grabs and drag
// sources included), or RouterPointer drives the router directly.
type TouchPointer interface {
	TouchMove(p Point)
	TouchPress(p Point)
	TouchRelease(p Point)
	// TouchCancel ends the emulated press without a click: a gesture
	// took the contact over.
	TouchCancel()
}

// RouterPointer is TouchPointer straight onto a router: move, left
// press and release, and a cancelled press. The emulated pointer
// leaves after a release - a finger does not hover.
type RouterPointer struct{ R *Router }

// TouchMove implements TouchPointer.
func (p RouterPointer) TouchMove(at Point) { p.R.Move(at) }

// TouchPress implements TouchPointer.
func (p RouterPointer) TouchPress(at Point) {
	p.R.Move(at)
	p.R.Press(BTNLeft, at, 0)
}

// TouchRelease implements TouchPointer.
func (p RouterPointer) TouchRelease(at Point) {
	p.R.Release(BTNLeft, at)
	p.R.Leave()
}

// TouchCancel implements TouchPointer.
func (p RouterPointer) TouchCancel() {
	p.R.CancelPress()
	p.R.Leave()
}

const (
	// longPressDelay is how long a still contact waits for a long press.
	longPressDelay = 500 * time.Millisecond
	// touchSlop is how far a contact may drift and still count as
	// still (a tap, a long press).
	touchSlop = 8
)

// touchMode is what the contacts are doing.
type touchMode uint8

const (
	touchIdle    touchMode = iota
	touchPointer           // one contact emulating the pointer
	touchTwo               // two contacts: a pinch or a two-finger scroll
	touchDone              // a gesture ended the sequence; wait for lift
)

// contact is one finger.
type contact struct {
	start, at Point
}

// TouchTracker turns touch contacts into input the rest of the toolkit
// already speaks. One contact emulates the pointer - a tap clicks, a
// drag drags, a hold past the long-press delay offers GestureLongPress
// - through Pointer. A second contact cancels that and makes the pair
// a gesture: a pinch (scale and rotation) for a widget that claims
// GesturePinch, otherwise a two-finger scroll down the precise-axis
// path, ending in a kinetic glide when the fingers lift.
type TouchTracker struct {
	Router  *Router
	Pointer TouchPointer

	contacts  map[int32]*contact
	order     []int32 // contact ids, oldest first
	mode      touchMode
	pinching  bool
	d0, a0    float64 // the pair's starting distance and angle
	centroid  Point
	longPress anim.Cancel
}

// pointer is the emulation sink.
func (t *TouchTracker) pointer() TouchPointer {
	if t.Pointer != nil {
		return t.Pointer
	}
	return RouterPointer{R: t.Router}
}

// Down adds a contact at p.
func (t *TouchTracker) Down(id int32, p Point) {
	if t.contacts == nil {
		t.contacts = map[int32]*contact{}
	}
	t.contacts[id] = &contact{start: p, at: p}
	t.order = append(t.order, id)
	switch len(t.contacts) {
	case 1:
		if t.mode != touchIdle {
			return
		}
		t.mode = touchPointer
		t.pointer().TouchPress(p)
		t.armLongPress(id)
	case 2:
		if t.mode == touchPointer {
			t.cancelLongPress()
			t.pointer().TouchCancel()
		}
		if t.mode == touchDone {
			return
		}
		t.mode = touchTwo
		a, b := t.pair()
		t.d0, t.a0 = pairDistance(a, b), pairAngle(a, b)
		t.centroid = midpoint(a, b)
		t.pinching = t.Router.Gesture(Gesture{Kind: GesturePinch, Phase: GestureBegin, At: t.centroid, Fingers: 2, Scale: 1})
	}
}

// Motion moves contact id to p.
func (t *TouchTracker) Motion(id int32, p Point) {
	c := t.contacts[id]
	if c == nil {
		return
	}
	c.at = p
	switch t.mode {
	case touchPointer:
		if dx, dy := p.X-c.start.X, p.Y-c.start.Y; max(dx, -dx) > touchSlop || max(dy, -dy) > touchSlop {
			t.cancelLongPress()
		}
		t.pointer().TouchMove(p)
	case touchTwo:
		a, b := t.pair()
		mid := midpoint(a, b)
		dx, dy := float64(mid.X-t.centroid.X), float64(mid.Y-t.centroid.Y)
		t.centroid = mid
		if t.pinching {
			scale := 1.0
			if t.d0 > 0 {
				scale = pairDistance(a, b) / t.d0
			}
			t.Router.Gesture(Gesture{
				Kind: GesturePinch, Phase: GestureUpdate, At: mid, Fingers: 2,
				DX: dx, DY: dy, Scale: scale, Rotation: pairAngle(a, b) - t.a0,
			})
			return
		}
		// Content follows the fingers: dragging down scrolls up.
		t.Router.Move(mid)
		t.Router.AxisPixels(-dx, -dy, 0)
	}
}

// Up lifts contact id.
func (t *TouchTracker) Up(id int32) {
	c := t.contacts[id]
	if c == nil {
		return
	}
	switch t.mode {
	case touchPointer:
		t.cancelLongPress()
		t.pointer().TouchRelease(c.at)
		t.mode = touchDone
	case touchTwo:
		if t.pinching {
			t.Router.Gesture(Gesture{Kind: GesturePinch, Phase: GestureEnd, At: t.centroid, Fingers: 2, Scale: 1})
		} else {
			t.Router.AxisEnd()
		}
		t.mode = touchDone
	}
	t.remove(id)
}

// Cancel drops every contact: the compositor took the sequence.
func (t *TouchTracker) Cancel() {
	switch t.mode {
	case touchPointer:
		t.pointer().TouchCancel()
	case touchTwo:
		if t.pinching {
			t.Router.Gesture(Gesture{Kind: GesturePinch, Phase: GestureCancel, Fingers: 2})
		}
	}
	t.cancelLongPress()
	clear(t.contacts)
	t.order = t.order[:0]
	t.mode = touchIdle
}

// remove forgets a lifted contact; the sequence ends with the last.
func (t *TouchTracker) remove(id int32) {
	delete(t.contacts, id)
	for i, o := range t.order {
		if o == id {
			t.order = append(t.order[:i], t.order[i+1:]...)
			break
		}
	}
	if len(t.contacts) == 0 {
		t.mode = touchIdle
		t.pinching = false
	}
}

// pair is the two oldest contacts' positions.
func (t *TouchTracker) pair() (Point, Point) {
	return t.contacts[t.order[0]].at, t.contacts[t.order[1]].at
}

// armLongPress schedules the long-press check on the animation clock.
func (t *TouchTracker) armLongPress(id int32) {
	t.cancelLongPress()
	t.longPress = anim.Play(anim.Delay(longPressDelay), anim.Animate(0, func(float64) {
		t.longPress = nil
		c := t.contacts[id]
		if t.mode != touchPointer || c == nil {
			return
		}
		if t.Router.Gesture(Gesture{Kind: GestureLongPress, Phase: GestureEnd, At: c.at, Fingers: 1}) {
			t.pointer().TouchCancel()
			t.mode = touchDone
		}
	}))
}

// cancelLongPress drops a pending long-press check.
func (t *TouchTracker) cancelLongPress() {
	if t.longPress != nil {
		t.longPress()
		t.longPress = nil
	}
}

func midpoint(a, b Point) Point { return Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2} }

func pairDistance(a, b Point) float64 { return math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)) }

// pairAngle is the pair's angle in degrees, clockwise on screen.
func pairAngle(a, b Point) float64 {
	return math.Atan2(float64(b.Y-a.Y), float64(b.X-a.X)) * 180 / math.Pi
}

// StylusTool is a tablet tool's kind.
type StylusTool uint8

// Stylus tools.
const (
	StylusPen StylusTool = iota
	StylusEraser
	StylusBrush
	StylusPencil
	StylusAirbrush
	StylusFinger
	StylusMouse
	StylusLens
)

// Stylus is one tablet tool sample: where the tool is, its axes, and
// whether its tip is down. Pressure and Distance are 0..1; tilt and
// rotation are degrees. Button reports a stylus button that changed
// with this sample (0: none) and Pressed its state.
type Stylus struct {
	Tool               StylusTool
	At                 Point
	Pressure, Distance float64
	TiltX, TiltY       float64
	Rotation           float64
	Down               bool
	Button             uint32
	Pressed            bool
}

// StylusHandler takes tablet samples - a canvas drawing with pressure,
// a slider dragging finer with a light touch. The tool also drives the
// pointer (hover, press, drag), so widgets that ignore pressure just
// work under a pen.
type StylusHandler interface {
	Stylus(s Stylus)
}

// Stylus delivers a tablet sample: to the pressed widget's chain while
// a press is held (a stroke stays with the widget it began on),
// otherwise the hovered one's, the first StylusHandler up the tree.
func (r *Router) Stylus(s Stylus) {
	from := r.pressed
	if from == nil {
		from = r.hover
	}
	for w := from; w != nil; w = parentOf(w) {
		if h, ok := w.(StylusHandler); ok && IsEnabled(w) {
			h.Stylus(s)
			return
		}
	}
}
