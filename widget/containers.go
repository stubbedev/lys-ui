package widget

import (
	"math"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

// StackTransition is how a Stack moves between pages.
type StackTransition uint8

const (
	// StackNone switches at once.
	StackNone StackTransition = iota
	// StackCrossfade cross-fades the old page into the new one.
	StackCrossfade
	// StackSlideLeftRight slides the pages sideways: a page added
	// later comes in from the right, an earlier one from the left.
	StackSlideLeftRight
)

// Stack holds named children and shows exactly one at a time, switching
// through its transition (none by default).
type Stack struct {
	node
	kids     map[string]Widget
	order    []string
	visible  string
	measured map[string]Size

	transition StackTransition
	duration   time.Duration
	// prev is the page leaving and progress how far the switch is
	// (eased); cancel stops the running switch.
	prev     string
	progress float64
	cancel   anim.Cancel
	layers   [2]*render.Layer
	// heterogeneous sizes the stack to its visible page instead of its
	// largest (GTK's homogeneous off); interpolate tweens that size
	// through a switch (GTK's interpolate-size).
	heterogeneous bool
	interpolate   bool
}

// NewStack returns an empty stack.
func NewStack() *Stack {
	return &Stack{
		kids:     make(map[string]Widget),
		measured: make(map[string]Size),
	}
}

// Add puts a child under name; adding an existing name replaces it.
// The replaced widget detaches like Stack.Remove — parent link
// cleared, removal hook fired — instead of staying parented to a
// stack it no longer belongs to.
func (s *Stack) Add(name string, w Widget) *Stack {
	if old, ok := s.kids[name]; ok && old != w {
		notifyRemoved(old)
		clearParents(old)
	}
	if _, ok := s.kids[name]; !ok {
		s.order = append(s.order, name)
	}
	s.kids[name] = w
	if s.visible == "" {
		s.visible = name
	}
	s.InvalidateLayout()
	return s
}

// Insert puts a child under name at position i of the page order
// (clamped), with Add's replacement rule for an existing name.
func (s *Stack) Insert(i int, name string, w Widget) *Stack {
	s.Add(name, w)
	s.Move(name, i)
	return s
}

// Move moves the page under name to position i of the page order
// (clamped); the page stays attached. An unknown name is a no-op.
func (s *Stack) Move(name string, i int) {
	from := slices.Index(s.order, name)
	if from < 0 {
		return
	}
	i = min(max(i, 0), len(s.order)-1)
	if from == i {
		return
	}
	s.order = slices.Insert(slices.Delete(s.order, from, from+1), i, name)
	s.Invalidate()
}

// Remove deletes the child under name and reports whether there was
// one. Removing the visible child leaves the stack showing nothing —
// Visible reads "" until the next Show or Add names a child; removal
// never silently promotes a sibling (pinned). The removed widget's
// parent link clears and the removal hook fires, so routers drop it;
// an unknown name is a no-op.
func (s *Stack) Remove(name string) bool {
	w, ok := s.kids[name]
	if !ok {
		return false
	}
	notifyRemoved(w)
	delete(s.kids, name)
	s.order = slices.DeleteFunc(s.order, func(n string) bool { return n == name })
	delete(s.measured, name)
	if s.visible == name {
		s.visible = ""
	}
	if s.prev == name {
		s.prev = ""
	}
	clearParents(w)
	s.InvalidateLayout()
	return true
}

// Children exposes the visible child for focus traversal.
func (s *Stack) Children() []Widget {
	if k, ok := s.kids[s.visible]; ok {
		return []Widget{k}
	}
	return nil
}

// appendChildren appends the visible child, matching Children.
func (s *Stack) appendChildren(buf []Widget) []Widget {
	if k, ok := s.kids[s.visible]; ok {
		return append(buf, k)
	}
	return buf
}

// appendStyleKids is every page, shown or not (styleKids): a hidden
// page keeps its cascade, so a restyle reaches it before it shows.
func (s *Stack) appendStyleKids(buf []Widget) []Widget {
	for _, name := range s.order {
		buf = append(buf, s.kids[name])
	}
	return buf
}

// SetEnabled turns the visible child on or off through the per-query
// enable walk, like Box (hidden children stay untouched; they paint
// nothing, so they need no repaint either).
func (s *Stack) SetEnabled(enabled bool) {
	s.node.SetEnabled(enabled)
	invalidateTree(s)
}

// Children exposes a snapshot of the overlay's children in add order
// for focus traversal. A copy, like Box.Children: tree walks must be
// able to survive a callback that mutates the container.
func (o *Overlay) Children() []Widget { return slices.Clone(o.kids) }

// appendChildren appends the overlay's children in add order, matching
// Children.
func (o *Overlay) appendChildren(buf []Widget) []Widget {
	return append(buf, o.kids...)
}

// SetEnabled turns the overlay's whole stack on or off through the
// per-query enable walk, like Box.
func (o *Overlay) SetEnabled(enabled bool) {
	o.node.SetEnabled(enabled)
	invalidateTree(o)
}

// SetTransition picks how later Show calls switch pages and how long a
// switch takes (GTK's stack transition type and duration); a zero
// duration switches at once.
func (s *Stack) SetTransition(t StackTransition, d time.Duration) {
	s.transition, s.duration = t, max(0, d)
}

// SetHomogeneous picks whether the stack is as large as its largest
// page (true, the default) or as its visible one (GTK's hhomogeneous
// and vhomogeneous off), so pages of different sizes resize what holds
// the stack.
func (s *Stack) SetHomogeneous(h bool) {
	if s.heterogeneous == !h {
		return
	}
	s.heterogeneous = !h
	s.InvalidateLayout()
}

// SetInterpolateSize makes a stack sized to its visible page tween
// between the two pages' sizes through a switch, on the switch's own
// easing (GTK's interpolate-size); off, the size jumps.
func (s *Stack) SetInterpolateSize(on bool) { s.interpolate = on }

// Homogeneous reports whether the stack is as large as its largest
// page (SetHomogeneous).
func (s *Stack) Homogeneous() bool { return !s.heterogeneous }

// InterpolateSize reports whether a switch tweens the stack's size
// (SetInterpolateSize).
func (s *Stack) InterpolateSize() bool { return s.interpolate }

// Page is the child added under name, visible or not (GtkStack's
// get_child_by_name); nil for an unknown name.
func (s *Stack) Page(name string) Widget { return s.kids[name] }

// Order returns the page names in add order.
func (s *Stack) Order() []string { return slices.Clone(s.order) }

// Show makes the child under name the visible one through the stack's
// transition; unknown names are ignored. A switch during a running one
// starts from the page then showing.
func (s *Stack) Show(name string) {
	if _, ok := s.kids[name]; !ok || name == s.visible {
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	prev := s.visible
	s.visible = name
	s.Invalidate()
	if s.heterogeneous {
		// The visible page sizes the stack: the holder relayouts.
		s.InvalidateLayout()
	}
	if s.transition == StackNone || s.duration == 0 || prev == "" {
		s.prev = ""
		return
	}
	s.prev, s.progress = prev, 0
	s.cancel = anim.Play(anim.Animate(s.duration, func(t float64) {
		s.progress = t
		if t >= 1 {
			s.prev, s.cancel = "", nil
		}
		s.Invalidate()
		if s.heterogeneous && s.interpolate {
			s.InvalidateLayout()
		}
	}).Easing(anim.EaseOutCubic))
}

// Switching reports whether a page transition is running.
func (s *Stack) Switching() bool { return s.prev != "" }

// Visible returns the visible child's name.
func (s *Stack) Visible() string {
	return s.visible
}

// Measure measures every child once and reports the largest - or,
// sized to its visible page, that page, tweened from the page leaving
// while an interpolating switch runs - inside the stack's CSS box
// (GtkStack's padding, border and min sizes), clamped to con.
func (s *Stack) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	v := s.style(s)
	return s.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		best := Size{}
		for _, name := range s.order {
			nat := measureChild(s, s.kids[name], inner)
			s.measured[name] = nat
			best.W = max(best.W, nat.W)
			best.H = max(best.H, nat.H)
		}
		if s.heterogeneous {
			best = s.visibleSize()
		}
		return clampSize(best, inner)
	}))
}

// Arrange assigns the content box to every child.
func (s *Stack) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(s.style(s), render.Insets{}), r)
	s.ArrangeRoot(border)
	for _, name := range s.order {
		arrangeChild(s.kids[name], inner)
		setParents(s, s.kids[name])
	}
}

// ArrangeRoot records the stack's own rect.
func (s *Stack) ArrangeRoot(r render.Rect) {
	s.node.Arrange(r)
}

// Paint draws the stack's own CSS layers, then the visible child, or
// both pages mid-switch.
func (s *Stack) Paint(cv *render.Canvas) {
	paintCSSBox(cv, s.style(s), s.bounds)
	w, ok := s.kids[s.visible]
	if !ok {
		return
	}
	old, switching := s.kids[s.prev]
	if !switching {
		// At rest the pages paint directly: the switch's layers, a
		// full-canvas buffer each, go until the next switch.
		s.layers = [2]*render.Layer{}
		PaintChild(cv, w)
		return
	}
	dev := cv.MapRect(s.bounds)
	s.layers[0] = cv.Layer(s.layers[0], dev)
	s.layers[1] = cv.Layer(s.layers[1], dev)
	old.Paint(s.layers[0].Canvas())
	w.Paint(s.layers[1].Canvas())
	switch s.transition {
	case StackSlideLeftRight:
		// Moving to a later page pushes the old one out to the left.
		dir := 1.0
		if slices.Index(s.order, s.visible) < slices.Index(s.order, s.prev) {
			dir = -1
		}
		off := math.Round(float64(dev.W) * s.progress)
		prev := cv.PushClipDevice(dev)
		cv.Composite(s.layers[0], dev, render.Translate(-dir*off, 0), 1)
		cv.Composite(s.layers[1], dev, render.Translate(dir*(float64(dev.W)-off), 0), 1)
		cv.PopClip(prev)
	default:
		s.layers[0].CrossFade(s.layers[1], dev, s.progress)
		cv.Composite(s.layers[0], dev, render.Identity, 1)
	}
}

// HitTest returns the visible child under p, or the stack inside its
// bounds.
func (s *Stack) HitTest(p Point) Widget {
	if w, ok := s.kids[s.visible]; ok {
		if hit := HitChild(w, p); hit != nil {
			return hit
		}
	}
	return s.HitLeaf(s, p)
}

// Overlay stacks all children on the same rect and paints them in add
// order. The last child paints on top and wins hit tests. A hidden
// child takes no part: it neither sizes, paints, nor hits, as in a Box.
type Overlay struct {
	node
	kids []Widget
	// aligns holds the per-axis alignment of each child (index-matched
	// with kids); AlignFill on both axes is a plain Append.
	aligns [][2]Align
	// nats is each child's height at the last Measure, zero for a
	// hidden one (index-matched with kids), for Shrinkable.
	nats []int
}

// NewOverlay returns an empty overlay.
func NewOverlay() *Overlay { return &Overlay{} }

// Append adds a child on top.
func (o *Overlay) Append(w Widget) *Overlay {
	return o.AppendAligned(w, AlignFill, AlignFill)
}

// AppendAligned adds a child on top, sized and placed per axis like a
// GtkOverlay child: AlignFill takes the overlay's extent, the others
// keep the child's natural size pinned to the start, center or end.
// The child hits only inside its own rect, so a corner button leaves
// the rest of the content clickable.
func (o *Overlay) AppendAligned(w Widget, h, v Align) *Overlay {
	o.kids = append(o.kids, w)
	o.aligns = append(o.aligns, [2]Align{h, v})
	o.InvalidateLayout()
	return o
}

// Measure reports the largest child inside the overlay's CSS box:
// padding, border and min sizes, clamped to con.
func (o *Overlay) Measure(con Constraints) Size {
	if sz, ok := o.measureHit(con); ok {
		return sz
	}
	v := o.style(o)
	return o.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		best := Size{}
		o.nats = slices.Grow(o.nats[:0], len(o.kids))[:len(o.kids)]
		for i, k := range o.kids {
			o.nats[i] = 0
			if !IsVisible(k) {
				continue
			}
			nat := measureChild(o, k, inner)
			o.nats[i] = nat.H
			best.W = max(best.W, nat.W)
			best.H = max(best.H, nat.H)
		}
		return clampSize(best, inner)
	}))
}

// Arrange records the border box and assigns the content box to every
// filling child, placing the aligned ones within it.
func (o *Overlay) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(o.style(o), render.Insets{}), r)
	o.ArrangeRoot(border)
	for i, k := range o.kids {
		cell := inner
		if a := o.aligns[i]; a != [2]Align{} {
			nat := measureChild(o, k, Constraints{Max: Size{W: inner.W, H: inner.H}})
			cell = alignRect(inner, nat, a[0], a[1])
		}
		arrangeChild(k, cell)
		setParents(o, k)
	}
}

// ArrangeRoot records the overlay's own rect.
func (o *Overlay) ArrangeRoot(r render.Rect) {
	o.node.Arrange(r)
}

// Paint draws the overlay's CSS box, then every child bottom to top.
func (o *Overlay) Paint(cv *render.Canvas) {
	v := o.style(o)
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, o.bounds, radiusOr(v, 0), borderOf(v), bg)
	}
	for _, k := range o.kids {
		if !IsVisible(k) {
			continue
		}
		PaintChild(cv, k)
	}
}

// HitTest returns the topmost child under p, or the overlay inside its
// bounds.
func (o *Overlay) HitTest(p Point) Widget {
	for i := range slices.Backward(o.kids) {
		if hit := HitChild(o.kids[i], p); hit != nil {
			return hit
		}
	}
	return o.HitLeaf(o, p)
}

// Scroll embeds a child larger than the viewport and shifts it by an
// offset. With ShowBars, overflowing axes reserve a scrollbar gutter;
// the bars fade in on hover or scrolling and out after a dwell, drag 1:1, and gutter
// clicks page. A child smaller than the viewport is centered, or
// stretched along an axis when the matching Fill flag is set.
type Scroll struct {
	node
	child      Widget
	nat        Size
	measured   Size // the size Measure last returned, for Shrinkable
	offX, offY int
	// arrOffX, arrOffY are the offset the child was last arranged at:
	// its descendants' bounds sit there until the next arrange, so
	// content positions read from them subtract this, not the live
	// offset.
	arrOffX, arrOffY int
	maxH             int
	measuredAt       int // the width a VerticalOnly child was last measured at

	// ShowBars enables the auto-hiding scrollbar indicators.
	ShowBars bool
	// pxFrac is pixel scrolling below a whole pixel (ScrollPixels).
	pxFrac [2]float64
	// OnScrolled fires when a wheel, bar drag, or keyboard scroll lands
	// at a NEW offset (same-offset scrolls stay silent). Nil means
	// nobody listens.
	OnScrolled func(x, y int)
	// FillX and FillY stretch a smaller child across the viewport
	// instead of centering it.
	FillX, FillY bool
	// VerticalOnly scrolls up and down only (GTK's hscrollbar-policy
	// never): the child is measured and laid out at the viewport's
	// width, so a wrapping label wraps there instead of at infinity.
	VerticalOnly bool
	// PropagateNaturalWidth (GTK's propagate-natural-width) makes a
	// VerticalOnly scroll ask for its child's natural width; without
	// it the scroll asks for the child's minimum, as GTK's does.
	PropagateNaturalWidth bool
	// PropagateNaturalHeight (GTK's propagate-natural-height): the
	// scroll asks for its content's height, capped by
	// SetMaxContentHeight, giving nothing up to a short parent. Without
	// it, as GTK's, the scroll asks for no height of its own: it takes
	// what its parent gives it (an expanding slot, FillY) and scrolls
	// the rest.
	PropagateNaturalHeight bool

	// view is the viewport, the content box inside the stylesheet's
	// border and padding; viewW/viewH is its child area inside the
	// reserved gutters.
	view           render.Rect
	viewW, viewH   int
	childX, childY int

	// drag state: which handle is grabbed and where in it the press
	// landed, so the content tracks the pointer 1:1.
	dragV, dragH  bool
	dragGrab      int
	dragStartOffX int
	dragStartOffY int

	// auto-hide: bar alpha 0..1, the time of the last scroll or hover
	// change, and the running fade's Cancel so a fresh fade replaces
	// the previous one mid-flight. fadeTarget is the target the running
	// fade aims at: a second request for the same target would restart
	// the same animation, so the wheel path (one fadeTo per tick)
	// skips it.
	alpha      float64
	fadeTarget float64
	hovered    bool
	lastInput  time.Time
	cancelFade anim.Cancel

	// watchers hear every range change (WatchRange); ranges is the
	// pair they last heard, so an unchanged relayout stays silent.
	watchers []func()
	ranges   [2]ScrollRange

	// kin is the touchpad glide; overshoot is how far (0..1, signed:
	// negative at the start edge) a finger scroll or glide pushed past
	// each axis's end, shown as an edge glow that fades when released.
	kin        kinetic
	overshoot  [2]float64
	cancelGlow anim.Cancel
	// cancelScroll stops the scroll-behavior: smooth tween.
	cancelScroll anim.Cancel
}

// gutter is the scrollbar strip width reserved inside the viewport.
const gutter = 8

// barInset keeps a bar's track off the viewport's outer edge and off
// both ends of its axis.
const barInset = 1

// barDwell is how long the bars stay after the last scroll or hover;
// barFade is the fade duration through the anim package.
const (
	barDwell = 1200 * time.Millisecond
	barFade  = 220 * time.Millisecond
)

// Children exposes the wrapped child for focus traversal; an empty
// scroll (SetChild(nil)) exposes none.
func (s *Scroll) Children() []Widget {
	if s.child == nil {
		return nil
	}
	return []Widget{s.child}
}

// appendChildren appends the wrapped child, matching Children.
func (s *Scroll) appendChildren(buf []Widget) []Widget {
	if s.child != nil {
		return append(buf, s.child)
	}
	return buf
}

// SetEnabled turns the viewport and everything inside it on or off
// through the per-query enable walk, like Box. A disabled scroll
// ignores wheel, bar drags, and gutter clicks; its child is disabled
// with it.
func (s *Scroll) SetEnabled(enabled bool) {
	s.node.SetEnabled(enabled)
	invalidateTree(s)
}

// NewScroll wraps child in a scrollable viewport.
func NewScroll(child Widget) *Scroll {
	return &Scroll{child: child}
}

// SetChild replaces the wrapped child. The old child detaches like any
// removal — parent link cleared, removal hook fired — and the scroll
// forgets the old content's position: offsets reset to the top-left
// (new content keeps no memory of the old scroll) and any bar drag
// ends. Re-setting the current child is a no-op. nil clears the
// content: an empty scroll measures empty, paints nothing, and hit
// tests as itself.
func (s *Scroll) SetChild(child Widget) {
	if s.child == child {
		return
	}
	old := s.child
	notifyRemoved(old)
	s.child = child
	s.offX, s.offY = 0, 0
	s.dragV, s.dragH = false, false
	s.stopSmooth()
	s.nat = Size{}
	clearParents(old)
	s.InvalidateLayout()
}

// Offset returns the current scroll offset.
func (s *Scroll) Offset() (int, int) {
	return s.offX, s.offY
}

// scrollMax reports the scrollable range per axis.
func (s *Scroll) scrollMax() (int, int) {
	return max(0, s.nat.W-s.viewW), max(0, s.nat.H-s.viewH)
}

// SetOffset scrolls to x, y, clamped so the child never leaves the
// viewport (overshoot clamps). A child smaller than the viewport stays
// pinned at 0. Scrolling invalidates the viewport bounds.
func (s *Scroll) SetOffset(x, y int) {
	maxX, maxY := s.scrollMax()
	x = min(max(0, x), maxX)
	y = min(max(0, y), maxY)
	if x == s.offX && y == s.offY {
		return
	}
	s.offX, s.offY = x, y
	s.Invalidate()
	s.notifyRange()
	if s.OnScrolled != nil {
		s.OnScrolled(x, y)
	}
}

// AxisRange implements Adjustable: the offset, the visible extent, and
// the content's extent along axis.
func (s *Scroll) AxisRange(axis Axis) ScrollRange {
	if axis == Row {
		return ScrollRange{Offset: s.offX, Page: s.viewW, Total: s.nat.W}
	}
	return ScrollRange{Offset: s.offY, Page: s.viewH, Total: s.nat.H}
}

// SetAxisOffset implements Adjustable.
func (s *Scroll) SetAxisOffset(axis Axis, offset int) {
	if axis == Row {
		s.SetOffset(offset, s.offY)
		return
	}
	s.SetOffset(s.offX, offset)
}

// WatchRange implements Adjustable.
func (s *Scroll) WatchRange(fn func()) { s.watchers = append(s.watchers, fn) }

// notifyRange tells the watchers when either range moved.
func (s *Scroll) notifyRange() {
	now := [2]ScrollRange{s.AxisRange(Row), s.AxisRange(Column)}
	if now == s.ranges {
		return
	}
	s.ranges = now
	for _, fn := range s.watchers {
		fn()
	}
}

// ShowBarsOnce marks the bars visible now and schedules their fade-out:
// called on scroll and hover activity.
func (s *Scroll) showBars() {
	if !s.ShowBars {
		return
	}
	s.lastInput = time.Now()
	s.fadeTo(1)
}

// fadeTo animates the bar alpha toward to, canceling any fade still
// running: the new tween's first callback takes over from the old
// one's last value. Each step invalidates the gutter strips so the
// fading bars repaint. A fade already running toward the same target
// keeps running - a wheel tick per frame would otherwise restart the
// same tween (a fresh Tween per tick) and stretch the fade forever.
func (s *Scroll) fadeTo(to float64) {
	if (to == 1 && s.alpha == 1) || (to == 0 && s.alpha == 0) {
		return
	}
	if s.cancelFade != nil && s.fadeTarget == to {
		return
	}
	if s.cancelFade != nil {
		s.cancelFade()
	}
	from := s.alpha
	s.fadeTarget = to
	s.cancelFade = anim.Start(barFade, func(t float64) {
		s.alpha = from + (to-from)*t
		s.invalidateBars()
	})
}

// SetHovered implements HoverSetter: entering shows the bars, leaving
// starts the dwell before they fade. Hovering itself paints nothing;
// the fade tweens invalidate the gutter strips as alpha moves.
func (s *Scroll) SetHovered(on bool) {
	s.hovered = on
	s.invalidateStyle()
	if on {
		s.showBars()
	}
}

// invalidateBars schedules a repaint of just the scrollbar strips -
// the only pixels an alpha change touches - instead of the whole
// viewport.
func (s *Scroll) invalidateBars() {
	if !s.ShowBars {
		return
	}
	if track, _ := s.vBarGeometry(); track.W > 0 {
		s.InvalidateRect(track)
	}
	if track, _ := s.hBarGeometry(); track.H > 0 {
		s.InvalidateRect(track)
	}
}

// Measure reports the child's natural size clamped to the offered
// constraints. The viewport itself is sized by Arrange (expanding
// children and cross stretch), so the reported natural size must stay
// bounded: an unbounded appetite here would blow up natural measurement
// in parent boxes and lay children out past the window edge.
func (s *Scroll) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	v := s.style(s)
	s.measured = measureBox(v, boxOf(v, render.Insets{}), con, s.measureContent)
	return s.measureStore(con, s.measured)
}

// measureContent is Measure inside the CSS box.
func (s *Scroll) measureContent(con Constraints) Size {
	s.nat = Size{}
	overflows := false
	if s.child != nil {
		w := math.MaxInt
		if s.VerticalOnly {
			w = con.Max.W
		}
		s.nat = measureChild(s, s.child, Constraints{Max: Size{W: w, H: math.MaxInt}})
		if s.VerticalOnly {
			// The bar's column is reserved only once the content
			// overflows the height on offer and only for a bar that
			// shows: a list that fits, or one without bars, keeps its
			// full width and its insets even.
			limit := con.Max.H
			if s.maxH > 0 {
				limit = min(limit, s.maxH)
			}
			if overflows = s.ShowBars && s.nat.H > limit; overflows {
				w = max(0, w-gutter)
				s.nat = measureChild(s, s.child, Constraints{Max: Size{W: w, H: math.MaxInt}})
			}
			s.measuredAt = w
		}
	}
	want := s.nat
	if s.VerticalOnly && !s.PropagateNaturalWidth && s.child != nil {
		// GTK's never-policy scroll without propagate-natural-width
		// asks for its child's minimum width: a long line wraps or
		// ellipsizes at the width it is given instead of widening it.
		want.W = minContentOf(s.child, Row, s.nat)
	}
	stable, both := s.gutterStable()
	if stable {
		// scrollbar-gutter: stable reserves the bar's column up front,
		// both-edges a matching one on the far side, so content does not
		// reflow when a bar appears.
		want.W += gutter
		if both {
			want.W += gutter
		}
	}
	if overflows {
		want.W += gutter
	}
	switch {
	case !s.PropagateNaturalHeight:
		want.H = 0
	case s.maxH > 0 && want.H > s.maxH:
		// Capped, the content overflows: the vertical bar's gutter
		// comes on top of the content width, not out of it.
		if !s.VerticalOnly && !stable && s.ShowBars {
			want.W += gutter
		}
		want.H = s.maxH
	}
	return clampSize(want, con)
}

// SetMaxContentHeight caps the height the scroll asks for (GTK's
// max-content-height) and turns PropagateNaturalHeight on, the only
// height a cap applies to: up to h it is as tall as its content, past
// it h tall and scrolling. 0 is no cap; a negative h is 0.
func (s *Scroll) SetMaxContentHeight(h int) {
	h = max(h, 0)
	if h == s.maxH && s.PropagateNaturalHeight {
		return
	}
	s.maxH = h
	s.PropagateNaturalHeight = true
	s.InvalidateLayout()
}

// MaxContentHeight is the cap SetMaxContentHeight set, 0 for none.
func (s *Scroll) MaxContentHeight() int { return s.maxH }

// Arrange pins the viewport to r, reserving a gutter for each
// overflowing axis that shows a bar, and places the child at the negative offset:
// stretched across the viewport when Fill is set and the child is
// smaller, centered when it is smaller without fill, otherwise at its
// natural size shifted by the offset.
func (s *Scroll) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(s.style(s), render.Insets{}), r)
	s.ArrangeRoot(border)
	s.view = inner
	r = inner
	gutterV, gutterH := 0, 0
	stable, both := s.gutterStable()
	if s.ShowBars && s.nat.H > r.H && r.H > 40 || stable {
		gutterV = gutter
	}
	if s.ShowBars && s.nat.W > r.W && r.W > 40 {
		gutterH = gutter
	}
	if s.VerticalOnly {
		gutterV, gutterH = 0, 0
		if stable {
			gutterV = gutter
		}
		if s.child != nil {
			w := r.W - gutterV
			if w != s.measuredAt {
				s.nat = measureChild(s, s.child, Constraints{Max: Size{W: w, H: math.MaxInt}})
				s.measuredAt = w
			}
			if gutterV == 0 && s.ShowBars && s.nat.H > r.H {
				// Overflowing, the bar takes its column; the content
				// keeps the narrower width even if it now fits, so the
				// layout cannot flip back and forth.
				gutterV = gutter
				w = max(0, r.W-gutter)
				s.nat = measureChild(s, s.child, Constraints{Max: Size{W: w, H: math.MaxInt}})
				s.measuredAt = w
			}
		}
	}
	s.viewW = r.W - gutterV
	if both {
		// both-edges: the matching empty column on the near edge.
		s.viewW -= gutter
	}
	s.viewH = r.H - gutterH
	// Mandatory snapping re-settles through a relayout or resize: the
	// offset a viewport change invalidated lands back on its snap
	// position before the frame paints.
	if st := s.snapStyle(); st.Mandatory {
		if x, y := s.snapTarget(s.offX, s.offY); x != s.offX || y != s.offY {
			maxX, maxY := s.scrollMax()
			s.offX, s.offY = min(max(0, x), maxX), min(max(0, y), maxY)
			s.Invalidate()
		}
	}

	childW, childH := s.nat.W, s.nat.H
	if s.VerticalOnly {
		childW = s.viewW
	}
	if s.FillX && childW < s.viewW {
		childW = s.viewW
	}
	if s.FillY && childH < s.viewH {
		childH = s.viewH
	}
	childX := 0
	if !s.FillX && childW < s.viewW {
		childX = (s.viewW - childW) / 2
	}
	childY := 0
	if !s.FillY && childH < s.viewH {
		childY = (s.viewH - childH) / 2
	}
	s.childX, s.childY = childX, childY

	if s.child != nil {
		arrangeChild(s.child, render.Rect{
			X: r.X + childX - s.offX,
			Y: r.Y + childY - s.offY,
			W: childW,
			H: childH,
		})
		s.arrOffX, s.arrOffY = s.offX, s.offY
		setParents(s, s.child)
	}
	s.notifyRange()
}

// ArrangeRoot records the viewport rect.
func (s *Scroll) ArrangeRoot(r render.Rect) {
	s.node.Arrange(r)
}

// Paint clips to the viewport, paints the shifted child, and draws
// the auto-hiding bars in their gutters at the current fade alpha.
func (s *Scroll) Paint(cv *render.Canvas) {
	v := s.style(s)
	radii := radiusOr(v, 0)
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, s.bounds, radii, borderOf(v), bg)
	}
	defer paintOutline(cv, v, s.bounds, radii)
	prev := cv.PushClip(s.view)
	if s.child != nil {
		PaintChild(cv, s.child)
	}
	s.paintGlow(cv)
	if s.ShowBars && s.alpha > 0 {
		spec := barSpecOf(v)
		s.tickFade()
		if track, handle := s.vBarGeometry(); track.W > 0 {
			paintScrollbar(cv, track, handle, s.alpha, spec)
		}
		if track, handle := s.hBarGeometry(); track.W > 0 {
			paintScrollbar(cv, track, handle, s.alpha, spec)
		}
	}
	cv.PopClip(prev)
}

// tickFade runs the dwell: with the pointer away and the last scroll
// older than the dwell, it fades the bars out.
func (s *Scroll) tickFade() {
	if s.hovered || time.Since(s.lastInput) < barDwell {
		return
	}
	s.fadeTo(0)
}

// Role implements Roleer.
func (s *Scroll) Role() Role { return RoleScrollArea }

// HitTest returns the widget under p: the shown scrollbars belong to
// the scroll itself (a press there drags or pages, never hits content),
// the child's bounds already carry the scroll offset so hit-testing
// through a scrolled viewport lands on the right content, and anything
// else inside the bounds is the scroll.
func (s *Scroll) HitTest(p Point) Widget {
	if !s.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if _, handle := s.vBarGeometry(); handle.W > 0 && p.X >= handle.X {
		return s
	}
	if _, handle := s.hBarGeometry(); handle.H > 0 && p.Y >= handle.Y {
		return s
	}
	if s.child != nil {
		if hit := HitChild(s.child, p); hit != nil {
			return hit
		}
	}
	return s
}

// vBarGeometry returns the vertical track and handle rects, barInset
// from the viewport's right, top and bottom; the track width is zero
// without bars or vertical overflow.
func (s *Scroll) vBarGeometry() (track, handle render.Rect) {
	r := s.AxisRange(Column)
	if !s.ShowBars || !r.Overflows() {
		return track, handle
	}
	track = render.Rect{X: s.view.X + s.view.W - gutter, Y: s.view.Y + barInset, W: gutter - barInset, H: s.viewH - 2*barInset}
	return track, r.thumbRect(track, Column)
}

// hBarGeometry returns the horizontal track and handle rects, barInset
// from the viewport's bottom, left and right; the track height is zero
// without bars or horizontal overflow.
func (s *Scroll) hBarGeometry() (track, handle render.Rect) {
	r := s.AxisRange(Row)
	if !s.ShowBars || !r.Overflows() {
		return track, handle
	}
	track = render.Rect{X: s.view.X + barInset, Y: s.view.Y + s.view.H - gutter, W: s.viewW - 2*barInset, H: gutter - barInset}
	return track, r.thumbRect(track, Row)
}

// ScrollBy shifts the offset by dx, dy wheel steps and shows the bars.
// A step is GTK's wheel scroll: the visible extent raised to 2/3 (a
// taller page scrolls further per notch), at least one pixel. A
// disabled scroll does not scroll.
func (s *Scroll) ScrollBy(dx, dy int) {
	if !IsEnabled(s) {
		return
	}
	s.kin.stop()
	s.stopSmooth()
	// The wheel never animates (scroll-behavior scopes smooth to
	// programmatic scrolls), but snapping still settles the step.
	x, y := s.snapTarget(s.offX+dx*wheelStep(s.viewW), s.offY+dy*wheelStep(s.viewH))
	s.SetOffset(x, y)
	s.showBars()
}

// wheelStep is one wheel notch's distance along an axis.
func wheelStep(extent int) int {
	if extent <= 0 {
		return scrollStepPx
	}
	return max(1, int(math.Pow(float64(extent), 2.0/3.0)))
}

// SetPressed implements PressSetter: ending the press ends any drag.
func (s *Scroll) SetPressed(on bool) {
	if on {
		s.kin.stop() // a press catches a glide
		return       // the grab geometry arrives with the first DragMove
	}
	s.dragV, s.dragH = false, false
	s.invalidateStyle()
	s.snapCurrent()
}

// DragMove implements DragMover. The first call inside a scrollbar
// track grabs that handle (recording the press position and offset);
// later calls map pointer motion 1:1 onto content motion through the
// viewport-to-track ratio. Outside the tracks this is content drugging
// of the wrapped child, which the child handles. Disabled scrolls
// ignore drags.
func (s *Scroll) DragMove(p Point) {
	if !IsEnabled(s) {
		return
	}
	s.stopSmooth()
	vTrack, _ := s.vBarGeometry()
	hTrack, _ := s.hBarGeometry()
	switch {
	case !s.dragV && !s.dragH && vTrack.W > 0 && p.X >= vTrack.X:
		s.dragV = true
		s.dragGrab = p.Y
		s.dragStartOffY = s.offY
	case !s.dragV && !s.dragH && hTrack.H > 0 && p.Y >= hTrack.Y:
		s.dragH = true
		s.dragGrab = p.X
		s.dragStartOffX = s.offX
	case s.dragV:
		s.SetAxisOffset(Column, s.AxisRange(Column).dragged(s.dragStartOffY, p.Y-s.dragGrab, vTrack.H))
		s.showBars()
	case s.dragH:
		s.SetAxisOffset(Row, s.AxisRange(Row).dragged(s.dragStartOffX, p.X-s.dragGrab, hTrack.W))
		s.showBars()
	}
}

// ClickAt implements Clicker: a gutter press above or below (or left
// or right of) a handle pages by one viewport; the first DragMove of
// such a press would have grabbed the handle instead. A click on a
// handle without motion does nothing. Disabled scrolls ignore clicks.
func (s *Scroll) ClickAt(p Point) {
	if !IsEnabled(s) {
		return
	}
	for _, axis := range [2]Axis{Column, Row} {
		track, _ := s.vBarGeometry()
		if axis == Row {
			track, _ = s.hBarGeometry()
		}
		if !track.Empty() && track.Contains(p.X, p.Y) {
			at, n := along(p, track, axis)
			target := s.AxisRange(axis).paged(at, n)
			if st := s.snapStyle(); st.Mandatory {
				target = s.snapAxisTo(axis, target, true)
			}
			s.SetAxisOffset(axis, target)
			return
		}
	}
}

// visibleSize is the visible page's last measured size, tweened from
// the page leaving while an interpolating switch runs.
func (s *Stack) visibleSize() Size {
	to := s.measured[s.visible]
	from, ok := s.measured[s.prev]
	if !ok || s.prev == "" || !s.interpolate {
		return to
	}
	return Size{W: s.lerp(from.W, to.W), H: s.lerp(from.H, to.H)}
}

// lerp is a at the switch's start to b at its end.
func (s *Stack) lerp(a, b int) int { return a + int(math.Round(float64(b-a)*s.progress)) }

// ScrollPixels implements PixelScroller: exact deltas, the fraction
// kept for the next one so slow finger motion still moves. The deltas
// feed the kinetic glide, and pushing past an end builds the edge glow.
func (s *Scroll) ScrollPixels(dx, dy float64) {
	if !IsEnabled(s) {
		return
	}
	s.kin.sample(dx, dy)
	s.movePixels(dx, dy)
	s.showBars()
}

// ScrollEnd implements ScrollEnder: the fingers lifted, so the content
// glides on from the gesture's velocity, and the glow fades once the
// motion is over.
func (s *Scroll) ScrollEnd() {
	s.kin.fling(func(dx, dy float64) bool {
		s.showBars()
		return s.movePixels(dx, dy)
	}, func() {
		s.snapCurrent()
		s.releaseGlow()
	})
}

// movePixels applies a pixel delta, keeping the sub-pixel remainder;
// what a clamp swallowed pushes the edge glow. Reports whether any of
// the motion moved the content.
func (s *Scroll) movePixels(dx, dy float64) bool {
	s.pxFrac[0] += dx
	s.pxFrac[1] += dy
	wx, wy := int(s.pxFrac[0]), int(s.pxFrac[1])
	s.pxFrac[0] -= float64(wx)
	s.pxFrac[1] -= float64(wy)
	if wx == 0 && wy == 0 {
		return true
	}
	x0, y0 := s.offX, s.offY
	s.SetOffset(s.offX+wx, s.offY+wy)
	lost := [2]int{x0 + wx - s.offX, y0 + wy - s.offY}
	for axis, l := range lost {
		if l != 0 {
			s.overscroll(axis, l)
		}
	}
	return s.offX != x0 || s.offY != y0
}

// glowSpan is the swallowed motion that fills the edge glow.
const glowSpan = 160.0

// pushGlow grows the glow at the edge the swallowed motion points at.
func (s *Scroll) pushGlow(axis int, lost float64) {
	if s.cancelGlow != nil {
		s.cancelGlow()
		s.cancelGlow = nil
	}
	o := s.overshoot[axis]
	if (o < 0) != (lost < 0) {
		o = 0 // the other edge: start over
	}
	s.overshoot[axis] = max(-1, min(1, o+lost/glowSpan))
	s.Invalidate()
}

// releaseGlow fades the glow out.
func (s *Scroll) releaseGlow() {
	from := s.overshoot
	if from == [2]float64{} {
		return
	}
	s.cancelGlow = anim.Start(barFade, func(t float64) {
		s.overshoot = [2]float64{from[0] * (1 - t), from[1] * (1 - t)}
		s.Invalidate()
	})
}

// paintGlow shades the viewport edge each axis overshot: an accent
// band that thins and fades with the overshoot.
func (s *Scroll) paintGlow(cv *render.Canvas) {
	a := Current().Accent
	for axis, o := range s.overshoot {
		if o == 0 {
			continue
		}
		mag := math.Abs(o)
		depth := max(1, int(32*mag))
		for k := range 4 {
			alpha := uint8(110 * mag * float64(4-k) / 4)
			band := depth * (k + 1) / 4
			r := render.Rect{X: s.view.X, Y: s.view.Y, W: s.view.W, H: band}
			switch {
			case axis == 1 && o > 0:
				r.Y = s.view.Y + s.view.H - band
			case axis == 0:
				r = render.Rect{X: s.view.X, Y: s.view.Y, W: band, H: s.view.H}
				if o > 0 {
					r.X = s.view.X + s.view.W - band
				}
			}
			cv.FillRect(r, render.RGBA(a.R(), a.G(), a.B(), alpha/4))
		}
	}
}
