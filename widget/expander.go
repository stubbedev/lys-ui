package widget

import (
	"math"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

const (
	// expanderDuration is the reveal/collapse tween length.
	expanderDuration = 180 * time.Millisecond
	// headerPadX/headerPadY inset the header row's content.
	headerPadX = 8
	headerPadY = 5
	// chevronSize is the rotate glyph's box; headerGap spaces the
	// title from it.
	chevronSize = 12
	headerGap   = 6
	// bodyPadX insets the child from the expander's sides.
	bodyPadX = 2
)

// Expander is a collapsible section (shadcn/ui's Collapsible): a
// header row (the title, and a chevron at the end turning from down to
// up) toggles the child beneath it. Toggling animates the height over
// the animation clock, with the chevron turning on the same progress, and the open state is remembered — a collapsed
// expander reopens to where it was, and state survives being hidden
// by a parent. Children exposes the child only while open, so focus
// traversal never lands inside a closed section.
type Expander struct {
	node
	face   render.Font
	title  string
	sizePx float64
	child  Widget
	open   bool
	// progress is the reveal fraction: 0 closed, 1 open, animated
	// between on toggle. The painted height and the chevron angle hang
	// off it.
	progress float64
	cancel   anim.Cancel
	hovered  bool
	// titleNode and arrow are the header's style nodes (`expander >
	// title`, `title > arrow`): the title's color paints the caption,
	// the arrow's the chevron.
	titleNode stylePart
	arrow     stylePart
	// childRect is where the last Arrange put the child, the clip the
	// painter reveals through.
	childRect render.Rect

	// OnToggled fires after every open-state change, including
	// programmatic ones.
	OnToggled func(open bool)
}

// NewExpander returns a collapsed section titled title wrapping child,
// painted with face at the theme's text size. Face may be a
// render.Chain for mixed-script fallback. A nil face panics here (see
// requireFace) instead of failing later, in shaping.
func NewExpander(face render.Font, title string, child Widget) *Expander {
	e := &Expander{face: requireFace("widget.NewExpander", face), title: title, sizePx: Current().TextSize, child: child}
	e.titleNode.SetElement("title")
	e.arrow.SetElement("arrow")
	setParents(e, &e.titleNode)
	setParents(&e.titleNode, &e.arrow)
	return e
}

// appendStyleKids is the title and, while open, the child (styleKids).
func (e *Expander) appendStyleKids(buf []Widget) []Widget {
	buf = append(buf, &e.titleNode)
	if e.open && e.child != nil {
		buf = append(buf, e.child)
	}
	return buf
}

// Open reports the toggle state (the settled layout, not the animated
// progress).
func (e *Expander) Open() bool { return e.open }

// SetTitle replaces the header text.
func (e *Expander) SetTitle(title string) {
	if e.title == title {
		return
	}
	e.title = title
	e.InvalidateLayout()
}

// Title returns the header text.
func (e *Expander) Title() string { return e.title }

// SetOpen opens or closes the section, animating the height. Fires
// OnToggled when the state flipped.
func (e *Expander) SetOpen(open bool) {
	if open == e.open {
		return
	}
	e.open = open
	if e.OnToggled != nil {
		e.OnToggled(open)
	}
	e.animate()
}

// Toggle flips the open state.
func (e *Expander) Toggle() { e.SetOpen(!e.open) }

// Children exposes the child only while open, so keyboard traversal
// and tree walks skip a closed section's content.
func (e *Expander) Children() []Widget {
	if e.open && e.child != nil {
		return []Widget{e.child}
	}
	return nil
}

// appendChildren appends the open expander's child, matching Children.
func (e *Expander) appendChildren(buf []Widget) []Widget {
	if e.open && e.child != nil {
		return append(buf, e.child)
	}
	return buf
}

// SetEnabled turns the expander and its content on or off through the
// per-query enable walk, like Box.
func (e *Expander) SetEnabled(enabled bool) {
	e.node.SetEnabled(enabled)
	invalidateTree(e)
}

// animate moves progress toward the open state over expanderDuration.
// A hidden expander — nothing arranged — settles instantly and
// schedules no timer: nothing animates while hidden.
func (e *Expander) animate() {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	if e.progress == e.target() {
		e.InvalidateLayout()
		return
	}
	if e.bounds.Empty() {
		e.progress = e.target()
		e.InvalidateLayout()
		return
	}
	from := e.progress
	e.cancel = anim.Play(anim.Animate(expanderDuration, func(t float64) {
		if t >= 1 {
			e.progress = e.target() // the landing lands exactly
		} else {
			e.progress = from + (e.target()-from)*t
		}
		// The height change re-measures the branch; the repaint is
		// this expander's own bounds.
		e.InvalidateLayout()
	}))
}

// target is the progress the open state asks for.
func (e *Expander) target() float64 {
	if e.open {
		return 1
	}
	return 0
}

// Arrange records the rect, positions the child under the header at
// the animated height, and stops the reveal cold when the expander
// went off screen (a hidden widget holds no timer; the state keeps
// whatever it reached).
func (e *Expander) Arrange(r render.Rect) {
	e.node.Arrange(r)
	if r.Empty() {
		if e.cancel != nil {
			e.cancel()
			e.cancel = nil
		}
		e.childRect = render.Rect{}
		e.arrangeChild(r)
		return
	}
	e.arrangeChild(r)
}

// arrangeChild places the child in the revealed strip below the
// header, or off-tree when nothing is revealed.
func (e *Expander) arrangeChild(r render.Rect) {
	if e.child == nil {
		return
	}
	h := e.bodyH(r)
	if h <= 0 {
		e.child.Arrange(render.Rect{})
		e.childRect = render.Rect{}
		return
	}
	e.childRect = render.Rect{
		X: r.X + bodyPadX,
		Y: r.Y + e.headerH(),
		W: max(0, r.W-2*bodyPadX),
		H: h,
	}
	arrangeChild(e.child, e.childRect)
	setParents(e, e.child)
}

// headerH is the header row's height.
func (e *Expander) headerH() int {
	if e.face == nil {
		return 2 * headerPadY
	}
	return e.face.Shape("lg", e.sizePx).LineHeight() + 2*headerPadY
}

// bodyH maps the animated progress onto pixels of child height.
func (e *Expander) bodyH(r render.Rect) int {
	if e.child == nil || e.progress <= 0 {
		return 0
	}
	nat := measureChild(e, e.child, Constraints{Max: Size{W: max(0, r.W), H: max(0, r.H-e.headerH())}})
	return min(max(0, r.H-e.headerH()), int(float64(nat.H)*e.progress+0.5))
}

// headerRect is the clickable strip.
func (e *Expander) headerRect() render.Rect {
	return render.Rect{X: e.bounds.X, Y: e.bounds.Y, W: e.bounds.W, H: min(e.bounds.H, e.headerH())}
}

// Measure reports the header plus the revealed fraction of the child.
func (e *Expander) Measure(con Constraints) Size {
	if sz, ok := e.measureHit(con); ok {
		return sz
	}
	headH := e.headerH()
	w := 2*headerPadX + chevronSize + headerGap
	if e.face != nil {
		w += int(e.face.Shape(e.title, e.sizePx).Advance() + 0.5)
	}
	body := Size{}
	if e.child != nil {
		body = measureChild(e, e.child, Constraints{
			Max: Size{W: max(0, con.Max.W), H: max(0, con.Max.H-headH)},
		})
		w = max(w, body.W+2*bodyPadX)
	}
	h := headH + int(float64(body.H)*e.progress+0.5)
	return e.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Paint draws the header — hover shade, title, chevron at the reveal
// angle — then the child, clipped to the revealed strip.
func (e *Expander) Paint(cv *render.Canvas) {
	th := Current()
	head := e.headerRect()
	if head.Empty() {
		return
	}
	if e.hovered {
		cv.RoundedRect(head, th.Radius, th.SurfaceHover)
	}
	av := e.arrow.style(&e.arrow)
	cx := head.X + head.W - headerPadX - chevronSize/2
	cy := head.Y + head.H/2
	strokeChevron(cv, cx, cy, chevronSize/3, math.Pi/2+math.Pi*math01(e.progress), pickc(0, av, style.PropColor, th.TextMuted))
	if e.face != nil {
		sh := e.face.Shape(e.title, e.sizePx)
		baseline := head.Y + (head.H-sh.LineHeight())/2 + int(sh.Ascent()+0.5)
		col := th.Text
		if e.title == "" {
			col = th.TextMuted
		}
		col = pickc(0, e.titleNode.style(&e.titleNode), style.PropColor, col)
		e.face.Draw(cv, sh, head.X+headerPadX, baseline, col)
	}
	if e.progress > 0 && e.child != nil && !e.childRect.Empty() {
		prev := cv.PushClip(e.childRect)
		PaintChild(cv, e.child)
		cv.PopClip(prev)
	}
}

// HitTest resolves the header into the expander (clicks toggle) and
// otherwise delegates to the revealed child.
func (e *Expander) HitTest(p Point) Widget {
	if !e.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if e.headerRect().Contains(p.X, p.Y) {
		return e
	}
	if e.progress > 0 && e.child != nil {
		if hit := e.child.HitTest(p); hit != nil {
			return hit
		}
	}
	return e
}

// ClickAt toggles when the release landed on the header.
func (e *Expander) ClickAt(p Point) {
	if e.headerRect().Contains(p.X, p.Y) {
		e.Toggle()
	}
}

// SetHovered implements HoverSetter; the header shade repaints.
func (e *Expander) SetHovered(on bool) {
	if e.hovered == on {
		return
	}
	e.hovered = on
	e.invalidateStyle()
}

// KeyAction implements KeyActionHandler: Enter toggles when focused,
// which makes the section reachable and operable by keyboard only.
func (e *Expander) KeyAction(a KeyAction, _ Mods) {
	if a == KeyEnter {
		e.Toggle()
	}
}

// InsertRune implements RuneHandler: Space toggles, the other half of
// the GTK activation pair.
func (e *Expander) InsertRune(r rune) {
	if r == ' ' {
		e.Toggle()
	}
}
