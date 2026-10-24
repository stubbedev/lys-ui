package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// The adaptive vocabulary of #94: Clamp, BreakpointBin, ViewSwitcher,
// StackSwitcher, and Carousel - the pieces an app reshapes itself
// with when the window narrows. NavigationView/SplitView and
// OverlaySplitView live in navigation.go.

// Clamp centers its child at a maximum width: wider allocations pad
// the sides, narrower ones pass through untouched (the adw Clamp, a
// pure measure wrapper).
type Clamp struct {
	composite
	max int
}

// NewClamp returns a clamp over child capping its width at max.
func NewClamp(max int, child Widget) *Clamp {
	c := &Clamp{max: max}
	c.initComposite(c, child)
	return c
}

// SetMaximum changes the cap.
func (c *Clamp) SetMaximum(max int) {
	c.max = max
	c.InvalidateLayout()
}

// Maximum reports the cap.
func (c *Clamp) Maximum() int { return c.max }

// Measure measures the child capped.
func (c *Clamp) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	inner := con
	if c.max > 0 {
		inner.Max.W = min(inner.Max.W, c.max)
	}
	sz := c.root.Measure(inner)
	return c.measureStore(con, clampSize(Size{W: con.Max.W, H: sz.H}, con))
}

// Arrange centers the child's rect inside r.
func (c *Clamp) Arrange(r render.Rect) {
	c.node.Arrange(r)
	w := r.W
	if c.max > 0 {
		w = min(w, c.max)
	}
	c.root.Arrange(render.Rect{X: r.X + (r.W-w)/2, Y: r.Y, W: w, H: r.H})
	setParents(c, c.root)
}

// Breakpoint applies while the allocated width is at or below Max
// (or at or above Min, when Max is zero) - the adw 1.4 condition
// model narrowed to the width axis the adaptive shell needs.
type Breakpoint struct {
	Max int
	Min int
	// Apply runs when the breakpoint starts holding; Unapply, when it
	// stops. Batches of setters are just closures.
	Apply   func()
	Unapply func()
}

// holds reports whether width satisfies the condition.
func (b Breakpoint) holds(width int) bool {
	switch {
	case b.Max > 0 && b.Min > 0:
		return width <= b.Max && width >= b.Min
	case b.Max > 0:
		return width <= b.Max
	case b.Min > 0:
		return width >= b.Min
	}
	return false
}

// breakpoints tracks a set of Breakpoints against an allocated width:
// the one condition engine BreakpointBin and the split views share.
// evaluate runs each breakpoint's Apply or Unapply on the transitions
// only - an unchanged width, or a width on the same side of every
// condition, runs nothing.
type breakpoints struct {
	points []Breakpoint
	held   []bool
	width  int
	seen   bool
}

// add appends a breakpoint; evaluation order is add order.
func (b *breakpoints) add(bp Breakpoint) {
	b.points = append(b.points, bp)
	b.held = append(b.held, false)
	b.seen = false // re-evaluate the new point at the next width
}

// evaluate applies the conditions to width.
func (b *breakpoints) evaluate(width int) {
	if b.seen && b.width == width {
		return
	}
	b.seen, b.width = true, width
	for i, bp := range b.points {
		now := bp.holds(width)
		if now == b.held[i] {
			continue
		}
		b.held[i] = now
		if now && bp.Apply != nil {
			bp.Apply()
		} else if !now && bp.Unapply != nil {
			bp.Unapply()
		}
	}
}

// BreakpointBin wraps one widget and runs its breakpoints' apply
// batches as the allocated width crosses them - the seam an adaptive
// shell swaps layouts through. Conditions evaluate at the top of
// Arrange, before the child is laid out, so a batch's changes land in
// the same pass; a batch that changes what the child wants calls
// InvalidateLayout and the engine re-measures on demand.
type BreakpointBin struct {
	composite
	bps breakpoints
}

// NewBreakpointBin returns a bin over child with no breakpoints.
func NewBreakpointBin(child Widget) *BreakpointBin {
	b := &BreakpointBin{}
	b.initComposite(b, child)
	return b
}

// Add appends a breakpoint; apply order is add order.
func (b *BreakpointBin) Add(bp Breakpoint) {
	b.bps.add(bp)
	b.InvalidateLayout()
}

// Width reports the last allocated width (the condition input).
func (b *BreakpointBin) Width() int { return b.bps.width }

// Arrange evaluates every breakpoint against the allocated width,
// then lays the child out.
func (b *BreakpointBin) Arrange(r render.Rect) {
	b.bps.evaluate(r.W)
	b.composite.Arrange(r)
}

// ViewSwitcher is a ToggleGroup bound to a Stack: one toggle per page
// (title, and an icon when given), the visible page's toggle active,
// activating a toggle showing its page - the adw pill for bottom bars
// and headers alike.
type ViewSwitcher struct {
	*ToggleGroup
	stack  *Stack
	face   render.Font
	sizePx float64
	titles map[string]string
	icons  map[string]string
	policy StackSwitcherPolicy
	names  []string
}

// StackSwitcherPolicy decides what switcher toggles show.
type StackSwitcherPolicy uint8

// Switcher policies.
const (
	// SwitcherAll shows icon and title where both exist.
	SwitcherAll StackSwitcherPolicy = iota
	// SwitcherText titles only (no icons), the narrow variant.
	SwitcherText
	// SwitcherIcons icons only, the bottom-bar variant.
	SwitcherIcons
)

// NewViewSwitcher returns a strip driving stack; titles names the
// pages (missing names fall back to the page names), icons is
// optional.
func NewViewSwitcher(face render.Font, sizePx float64, stack *Stack, titles, icons map[string]string) *ViewSwitcher {
	face = requireFace("widget.NewViewSwitcher", face)
	v := &ViewSwitcher{ToggleGroup: NewToggleGroup(), stack: stack, face: face, sizePx: sizePx, titles: titles, icons: icons}
	v.self = v
	v.SetElement("viewswitcher")
	v.fillWidth = true
	v.OnChanged = func(i int) { v.stack.Show(v.names[i]) }
	v.Sync()
	return v
}

// SetPolicy changes what the toggles show and rebuilds.
func (v *ViewSwitcher) SetPolicy(p StackSwitcherPolicy) {
	v.policy = p
	v.Sync()
}

// Sync rebuilds the toggles over the stack's pages (the stack has no
// signal: the app calls this after adding pages).
func (v *ViewSwitcher) Sync() {
	v.Clear()
	v.names = v.stack.Order()
	for _, name := range v.names {
		title := name
		if t, ok := v.titles[name]; ok {
			title = t
		}
		icon := v.icons[name]
		switch v.policy {
		case SwitcherIcons:
			if icon != "" {
				title = ""
			}
		case SwitcherText:
			icon = ""
		}
		v.Append(NewButtonContent(v.face, v.sizePx, icon, title))
	}
	v.ReflectVisible()
}

// ReflectVisible activates the visible page's toggle after a
// programmatic stack.Show (the app drives, the switcher shows).
func (v *ViewSwitcher) ReflectVisible() {
	for i, name := range v.names {
		if name == v.stack.Visible() {
			v.SetActive(i)
		}
	}
}

// StackSwitcher is GTK's classic stack switcher: a text-only
// ViewSwitcher, the variant for windows that title pages in words.
type StackSwitcher struct {
	*ViewSwitcher
}

// NewStackSwitcher returns a tab strip driving stack.
func NewStackSwitcher(face render.Font, sizePx float64, stack *Stack, titles map[string]string) *StackSwitcher {
	s := &StackSwitcher{ViewSwitcher: NewViewSwitcher(face, sizePx, stack, titles, nil)}
	s.self = s
	s.SetPolicy(SwitcherText)
	return s
}

// carouselNav is the previous and next buttons' size and their inset
// from the carousel's sides.
const (
	carouselNav      = 32
	carouselNavInset = 8
)

// Carousel is a horizontal swipeable page strip (shadcn/ui's
// Carousel): pages side by side at the carousel's width, pointer drags
// and touchpad swipes pan, releases snap to the nearest page on the
// animation clock, the round previous and next buttons step a page
// (disabled at the ends, Left and Right on them step too), and the
// painted dots index the position. It styles as the `carousel`
// element: the button.nav pair.
type Carousel struct {
	node
	face   render.Font
	sizePx float64
	pages  []Widget
	nav    [2]*Button
	offset float64 // in pages, fractional
	cancel anim.Cancel

	// drag state: the press position and the page offset it started
	// from.
	drag     dragGesture
	dragFrom float64

	// OnPage fires when the settled page changes.
	OnPage func(i int)
}

// NewCarousel returns an empty carousel.
func NewCarousel(face render.Font, sizePx float64) *Carousel {
	c := &Carousel{face: requireFace("widget.NewCarousel", face), sizePx: sizePx}
	c.SetElement("carousel")
	for i, kind := range [2]SymbolKind{SymbolArrowLeft, SymbolArrowRight} {
		b := NewButton(NewSymbol(kind, 16), 0, 0)
		b.AddClass(css.Outline)
		b.AddClass("nav")
		b.SetSizeRequest(carouselNav, carouselNav)
		step := 2*i - 1
		b.OnClick = func() { c.step(step) }
		c.nav[i] = b
	}
	c.nav[0].SetAccessibleName(Tr("Previous slide"))
	c.nav[1].SetAccessibleName(Tr("Next slide"))
	setParents(c, c.nav[0], c.nav[1])
	c.syncNav()
	return c
}

// Append adds a page at the end.
func (c *Carousel) Append(w Widget) {
	c.pages = append(c.pages, w)
	setParents(c, w)
	c.syncNav()
	c.InvalidateLayout()
}

// step snaps d pages on from the shown one.
func (c *Carousel) step(d int) { c.snapTo(c.Page() + d) }

// syncNav enables the previous and next buttons where there is a page
// to step to.
func (c *Carousel) syncNav() {
	c.nav[0].SetEnabled(c.Page() > 0)
	c.nav[1].SetEnabled(c.Page() < len(c.pages)-1)
}

// Children are the pages and the previous and next buttons.
func (c *Carousel) Children() []Widget { return c.appendChildren(nil) }

// appendChildren appends the pages and the buttons, matching Children.
func (c *Carousel) appendChildren(buf []Widget) []Widget {
	return append(append(buf, c.pages...), c.nav[0], c.nav[1])
}

// InterceptKey implements KeyInterceptor: Left and Right on the
// previous or next button step a page.
func (c *Carousel) InterceptKey(target Widget, a KeyAction, _ Mods) bool {
	if target != Widget(c.nav[0]) && target != Widget(c.nav[1]) {
		return false
	}
	switch a {
	case KeyLeft:
		c.step(-1)
	case KeyRight:
		c.step(1)
	default:
		return false
	}
	return true
}

// Pages reports the page count.
func (c *Carousel) Pages() int { return len(c.pages) }

// Page shows page i (animated snap).
func (c *Carousel) Page() int { return int(c.offset + 0.5) }

// SetPage snaps to page i without animation.
func (c *Carousel) SetPage(i int) {
	i = max(0, min(i, len(c.pages)-1))
	c.offset = float64(i)
	c.syncNav()
	c.Invalidate()
	if c.OnPage != nil {
		c.OnPage(i)
	}
}

// Measure wants each page's height at the full width, the tallest
// wins.
func (c *Carousel) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	best := Size{}
	for _, p := range c.pages {
		s := p.Measure(con)
		best.H = max(best.H, s.H)
	}
	best.W = con.Max.W
	return c.measureStore(con, clampSize(best, con))
}

// Arrange lays the pages side by side at the carousel's width (each
// aligned in its slot), offset by the fractional page position, and
// the previous and next
// buttons inside the sides, centered.
func (c *Carousel) Arrange(r render.Rect) {
	c.node.Arrange(r)
	x := r.X - int(c.offset*float64(r.W))
	for _, p := range c.pages {
		arrangeChild(p, render.Rect{X: x, Y: r.Y, W: r.W, H: r.H})
		x += r.W
	}
	y := r.Y + (r.H-carouselNav)/2
	c.nav[0].Arrange(render.Rect{X: r.X + carouselNavInset, Y: y, W: carouselNav, H: carouselNav})
	c.nav[1].Arrange(render.Rect{X: r.X + r.W - carouselNavInset - carouselNav, Y: y, W: carouselNav, H: carouselNav})
}

// Paint draws the visible pages clipped, then the indicator dots and
// the buttons.
func (c *Carousel) Paint(cv *render.Canvas) {
	th := Current()
	prev := cv.PushClip(c.bounds)
	for _, p := range c.pages {
		if b, ok := p.(Boundser); ok && b.Bounds().X < c.bounds.X+c.bounds.W && b.Bounds().X+b.Bounds().W > c.bounds.X {
			PaintChild(cv, p)
		}
	}
	cv.PopClip(prev)
	if n := len(c.pages); n > 1 {
		dot := 5
		total := n * (dot*2 + 4)
		x := c.bounds.X + (c.bounds.W-total)/2
		y := c.bounds.Y + c.bounds.H - dot*3
		for i := range n {
			col := th.Border
			if i == c.Page() {
				col = th.Accent
			}
			cv.RoundedRect(render.Rect{X: x + dot, Y: y, W: dot, H: dot}, dot, col)
			x += dot*2 + 4
		}
	}
	for _, b := range c.nav {
		PaintChild(cv, b)
	}
}

// HitTest resolves the buttons, else the carousel itself.
func (c *Carousel) HitTest(p Point) Widget {
	for _, b := range c.nav {
		if hit := b.HitTest(p); hit != nil {
			return hit
		}
	}
	if c.bounds.Contains(p.X, p.Y) {
		return c
	}
	return nil
}

// PressAt begins a drag from the current page offset.
func (c *Carousel) PressAt(p Point) {
	c.drag.begin(p)
	c.dragFrom = c.offset
}

// DragMove pans the pages with the pointer.
func (c *Carousel) DragMove(p Point) {
	dx, _, ok := c.drag.travel(p)
	if !ok || c.bounds.W == 0 {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.offset = clamp01pages(c.dragFrom-float64(dx)/float64(c.bounds.W), len(c.pages))
	c.Invalidate()
}

// PressEnd snaps to the nearest page when a drag ends - released,
// cancelled, or lost alike.
func (c *Carousel) PressEnd() {
	if c.drag.end() {
		c.snap()
	}
}

// Gesture implements GestureHandler: a touchpad swipe pans the pages
// like a drag, snapping when the fingers lift (touchscreen drags reach
// the carousel through the touch pointer emulation).
func (c *Carousel) Gesture(g Gesture) bool {
	if g.Kind != GestureSwipe || c.bounds.W == 0 {
		return false
	}
	switch g.Phase {
	case GestureBegin:
		if c.cancel != nil {
			c.cancel()
		}
	case GestureUpdate:
		c.offset = clamp01pages(c.offset-g.DX/float64(c.bounds.W), len(c.pages))
		c.Invalidate()
	case GestureEnd, GestureCancel:
		c.snap()
	}
	return true
}

// snap animates the offset to the nearest integer page.
func (c *Carousel) snap() { c.snapTo(int(c.offset + 0.5)) }

// snapTo animates the offset to page i, clamped.
func (c *Carousel) snapTo(i int) {
	page := max(0, min(len(c.pages)-1, i))
	target, from := float64(page), c.offset
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel = anim.Start(220*time.Millisecond, func(t float64) {
		c.offset = from + (target-from)*easeOut(t)
		c.syncNav()
		c.Invalidate()
	})
	if c.OnPage != nil {
		c.OnPage(page)
	}
}

// easeOut is the snap curve.
func easeOut(t float64) float64 { return 1 - (1-t)*(1-t) }

// clamp01pages keeps the fractional offset inside the pages.
func clamp01pages(v float64, pages int) float64 {
	if pages <= 1 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > float64(pages-1) {
		return float64(pages - 1)
	}
	return v
}
