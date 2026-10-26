package widget

import (
	"slices"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// flowMaxPerLine is GtkFlowBox's default max-children-per-line.
const flowMaxPerLine = 7

// FlowBox lays its children out in lines, starting a new line when the
// next child would not fit or the line holds its maximum
// (GtkFlowBox, horizontal and non-homogeneous). It selects like a
// List - SetSelectionMode, single by default (GTK's), with clicks,
// rubber bands, and the keyboard moving by item and by line - and a
// click activates (OnActivate) unless SetSingleClickActivate(false)
// leaves activation to a double click. Controls inside a child keep
// their own clicks.
// Each child sits in a FlowBoxChild, the `flowboxchild` node a
// stylesheet addresses (`flowbox > flowboxchild.drop-before`). Its
// natural width is the widest line at the maximum per line; offered
// less, it wraps and grows taller.
type FlowBox struct {
	node
	selection
	kids                   []*FlowBoxChild
	colSpacing, rowSpacing int
	maxPerLine             int
	// justify distributes each line's leftover width (WrapBox).
	justify Justify
	// measuredW and floorW are the last Measure's width and the
	// narrowest it lays out at (its widest child), for ShrinkableWidth.
	measuredW, floorW int
	// arranged is the last Arrange's lines, the keyboard's geometry.
	arranged [][]int
	// singleClick activates on a plain click (GTK's default).
	singleClick bool
	// gesture state: the press's child and whether a drag left it.
	anchor int
	moved  bool

	// OnSelectionChanged fires with the selected indices once per
	// gesture; OnActivate fires for an activated child.
	OnSelectionChanged func(children []int)
	OnActivate         func(i int)
}

// FlowBoxChild wraps one child of a FlowBox: a CSS box around it.
type FlowBoxChild struct {
	node
	box   *FlowBox
	child Widget
	nat   Size
}

// NewFlowBox returns an empty flow box with colSpacing between the
// children of a line and rowSpacing between lines.
func NewFlowBox(colSpacing, rowSpacing int) *FlowBox {
	f := &FlowBox{colSpacing: colSpacing, rowSpacing: rowSpacing, maxPerLine: flowMaxPerLine, singleClick: true}
	f.SetElement("flowbox")
	f.initSelection(f, SelectionSingle)
	return f
}

// SetSingleClickActivate makes a plain click activate (on, GTK's
// default) or leaves activation to a double click.
func (f *FlowBox) SetSingleClickActivate(on bool) { f.singleClick = on }

// count implements selectionHost.
func (f *FlowBox) count() int { return len(f.kids) }

// selectionSync implements selectionHost: :selected follows membership.
func (f *FlowBox) selectionSync() {
	for i, c := range f.kids {
		c.SetState(StateSelected, f.isSelected(i))
	}
	f.Invalidate()
}

// selectionNotify implements selectionHost: OnSelectionChanged.
func (f *FlowBox) selectionNotify() {
	if f.OnSelectionChanged != nil {
		f.OnSelectionChanged(f.Selection())
	}
}

// selectionSingle implements selectionHost; the set notification
// covers it.
func (f *FlowBox) selectionSingle(int) {}

// reveal implements selectionHost: child i scrolls into view in the
// Scrolls above the box.
func (f *FlowBox) reveal(i int) {
	if c := f.ChildAt(i); c != nil {
		f.revealItem(i, c)
	}
}

// item is child i as a Widget, nil when there is none.
func (f *FlowBox) item(i int) Widget {
	if c := f.ChildAt(i); c != nil {
		return c
	}
	return nil
}

// activate implements selectionHost: OnActivate.
func (f *FlowBox) activate(i int) {
	if i >= 0 && i < len(f.kids) && f.OnActivate != nil {
		f.OnActivate(i)
	}
}

// KeyAction moves and extends the selection: left and right by a
// child, up and down to the nearest child of the neighboring line.
func (f *FlowBox) KeyAction(a KeyAction, mods Mods) {
	if IsEnabled(f) {
		f.key(a, mods, f.navigate)
	}
}

// navigate maps a motion key to its target child through the last
// arranged lines.
func (f *FlowBox) navigate(from int, a KeyAction) (int, bool) {
	switch a {
	case KeyLeft:
		return from - 1, true
	case KeyRight:
		return from + 1, true
	case KeyUp, KeyDown:
	default:
		return 0, false
	}
	if from < 0 {
		return 0, true
	}
	for li, line := range f.arranged {
		for _, i := range line {
			if i != from {
				continue
			}
			to := li - 1
			if a == KeyDown {
				to = li + 1
			}
			if to < 0 || to >= len(f.arranged) {
				return from, true
			}
			return f.nearest(f.arranged[to], f.kids[from].bounds), true
		}
	}
	return from, true
}

// nearest is the child of line whose center is closest across to r's.
func (f *FlowBox) nearest(line []int, r render.Rect) int {
	best, dist := line[0], -1
	cx := r.X + r.W/2
	for _, i := range line {
		b := f.kids[i].bounds
		d := b.X + b.W/2 - cx
		if d = max(d, -d); dist < 0 || d < dist {
			best, dist = i, d
		}
	}
	return best
}

// childClick is a click on child i: the selection gesture, then
// activation under single-click activate. The release ending a rubber
// band is not a click.
func (f *FlowBox) childClick(i int) {
	if f.moved || i < 0 {
		return
	}
	f.click(i)
	if f.singleClick && f.mode != SelectionMultiple {
		f.activate(i)
	}
}

// SetMaxChildrenPerLine caps a line's children (GTK's
// max-children-per-line, 7 by default); below 1 it acts as 1.
func (f *FlowBox) SetMaxChildrenPerLine(n int) {
	if f.maxPerLine != n {
		f.maxPerLine = n
		f.InvalidateLayout()
	}
}

// Append adds w at the end, returning its FlowBoxChild.
func (f *FlowBox) Append(w Widget) *FlowBoxChild { return f.Insert(len(f.kids), w) }

// Insert adds w at index i (clamped), returning its FlowBoxChild.
func (f *FlowBox) Insert(i int, w Widget) *FlowBoxChild {
	c := &FlowBoxChild{box: f, child: w}
	c.SetElement("flowboxchild")
	i = min(max(i, 0), len(f.kids))
	f.kids = append(f.kids, nil)
	copy(f.kids[i+1:], f.kids[i:])
	f.kids[i] = c
	f.inserted(i)
	setParents(f, c)
	setParents(c, w)
	f.InvalidateLayout()
	restyleChildren(f)
	return c
}

// RemoveAt detaches the child at index i; out of range does nothing.
func (f *FlowBox) RemoveAt(i int) {
	if i < 0 || i >= len(f.kids) {
		return
	}
	c := f.kids[i]
	f.kids = append(f.kids[:i], f.kids[i+1:]...)
	f.removed(i)
	notifyRemoved(c.child)
	clearParents(c, c.child)
	f.InvalidateLayout()
	restyleChildren(f)
}

// Move moves the child at index from to index to (both clamped), in
// place: the child stays attached and its selection follows it.
func (f *FlowBox) Move(from, to int) {
	if from < 0 || from >= len(f.kids) {
		return
	}
	to = min(max(to, 0), len(f.kids)-1)
	if from == to {
		return
	}
	c := f.kids[from]
	f.kids = slices.Insert(slices.Delete(f.kids, from, from+1), to, c)
	f.selection.moved(from, to)
	f.InvalidateLayout()
	restyleChildren(f)
}

// Clear detaches every child.
func (f *FlowBox) Clear() {
	for len(f.kids) > 0 {
		f.RemoveAt(len(f.kids) - 1)
	}
}

// Len is the number of children.
func (f *FlowBox) Len() int { return len(f.kids) }

// ChildAt is the FlowBoxChild at index i, nil out of range
// (gtk_flow_box_get_child_at_index).
func (f *FlowBox) ChildAt(i int) *FlowBoxChild {
	if i < 0 || i >= len(f.kids) {
		return nil
	}
	return f.kids[i]
}

// IndexAt is the index of the child under p, -1 over none
// (gtk_flow_box_get_child_at_pos).
func (f *FlowBox) IndexAt(p Point) int {
	for i, c := range f.kids {
		if IsVisible(c) && c.bounds.Contains(p.X, p.Y) {
			return i
		}
	}
	return -1
}

// Children exposes the FlowBoxChild wrappers.
func (f *FlowBox) Children() []Widget {
	out := make([]Widget, len(f.kids))
	for i, c := range f.kids {
		out[i] = c
	}
	return out
}

// lines splits the visible children into lines within width.
func (f *FlowBox) lines(width int) [][]int {
	var out [][]int
	var line []int
	used := 0
	for i, c := range f.kids {
		if !IsVisible(c) {
			continue
		}
		w := c.nat.W
		if len(line) > 0 && (len(line) >= f.maxPerLine || used+f.colSpacing+w > width) {
			out = append(out, line)
			line, used = nil, 0
		}
		if len(line) > 0 {
			used += f.colSpacing
		}
		line = append(line, i)
		used += w
	}
	if len(line) > 0 {
		out = append(out, line)
	}
	return out
}

// lineSize is a line's width and height.
func (f *FlowBox) lineSize(line []int) (w, h int) {
	for k, i := range line {
		if k > 0 {
			w += f.colSpacing
		}
		w += f.kids[i].nat.W
		h = max(h, f.kids[i].nat.H)
	}
	return w, h
}

// Measure lays the lines out within the offered width: as wide as its
// widest line, as tall as its lines.
func (f *FlowBox) Measure(con Constraints) Size {
	if sz, ok := f.measureHit(con); ok {
		return sz
	}
	v := f.style(f)
	box := boxOf(v, render.Insets{})
	o := box.outer()
	sz := f.measureStore(con, measureBox(v, box, con, func(inner Constraints) Size {
		widest := 0
		for _, c := range f.kids {
			c.nat = measureChild(f, c, Constraints{Max: inner.Max})
			if IsVisible(c) {
				widest = max(widest, c.nat.W)
			}
		}
		f.floorW = widest + o.Left + o.Right
		var sz Size
		for k, line := range f.lines(inner.Max.W) {
			w, h := f.lineSize(line)
			sz.W = max(sz.W, w)
			if k > 0 {
				sz.H += f.rowSpacing
			}
			sz.H += h
		}
		return clampSize(sz, inner)
	}))
	f.measuredW = sz.W
	return sz
}

// ShrinkableWidth implements WidthShrinker: a flow box narrows to its
// widest child, wrapping (GtkFlowBox's minimum width).
func (f *FlowBox) ShrinkableWidth() int { return max(0, f.measuredW-f.floorW) }

// Arrange places each line under the last, its children at their
// natural widths from the start edge, as tall as the line.
func (f *FlowBox) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(f.style(f), render.Insets{}), r)
	f.node.Arrange(border)
	y := inner.Y
	f.arranged = f.lines(inner.W)
	for _, line := range f.arranged {
		_, h := f.lineSize(line)
		w, _ := f.lineSize(line)
		x, grow, gap := f.justify.distribute(inner.X, inner.W-w, len(line))
		for k, i := range line {
			c := f.kids[i]
			cw := c.nat.W + grow
			if last := k == len(line)-1; last && f.justify == JustifyFill {
				cw = inner.X + inner.W - x // the fill's rounding remainder
			} else if last && k > 0 && f.justify == JustifySpread {
				x = inner.X + inner.W - cw // the spread's rounding remainder
			}
			c.Arrange(render.Rect{X: x, Y: y, W: cw, H: h})
			setParents(f, c)
			x += cw + f.colSpacing + gap
		}
		y += h + f.rowSpacing
	}
	f.revealArranged(f.item)
}

// Paint draws the box's own CSS layers and its children.
func (f *FlowBox) Paint(cv *render.Canvas) {
	paintCSSBox(cv, f.style(f), f.bounds)
	for _, c := range f.kids {
		if IsVisible(c) {
			PaintChild(cv, c)
		}
	}
}

// HitTest is the child under p, else the box.
func (f *FlowBox) HitTest(p Point) Widget {
	for _, c := range f.kids {
		if !IsVisible(c) {
			continue
		}
		if hit := c.HitTest(p); hit != nil {
			return hit
		}
	}
	return f.HitLeaf(f, p)
}

// Child is the wrapped widget.
func (c *FlowBoxChild) Child() Widget { return c.child }

// Index is the child's position in its FlowBox, -1 when detached.
func (c *FlowBoxChild) Index() int {
	if c.box != nil {
		for i, k := range c.box.kids {
			if k == c {
				return i
			}
		}
	}
	return -1
}

// Measure is the child inside the wrapper's CSS box.
func (c *FlowBoxChild) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	v := c.style(c)
	return c.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return measureChild(c, c.child, inner)
	}))
}

// Arrange gives the child the content box.
func (c *FlowBoxChild) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(c.style(c), render.Insets{}), r)
	c.node.Arrange(border)
	arrangeChild(c.child, inner)
	setParents(c, c.child)
}

// Paint draws the wrapper's CSS layers - a selected child over the
// theme's selection tint where the stylesheet names no background -
// and the child.
func (c *FlowBoxChild) Paint(cv *render.Canvas) {
	v := c.style(c)
	if c.HasState(StateSelected) && !v.Declares(style.PropBackgroundColor) {
		cv.RoundedRect(c.bounds, radiusOr(v, 6).TopLeft, selectedTint())
	}
	paintCSSBox(cv, v, c.bounds)
	PaintChild(cv, c.child)
}

// HitTest resolves through the child; in a selectable box the wrapper
// takes every press but those a control inside the child consumes.
func (c *FlowBoxChild) HitTest(p Point) Widget {
	if hit := c.child.HitTest(p); hit != nil && (c.box == nil || c.box.mode == SelectionNone || interactiveWithin(hit, c)) {
		return hit
	}
	return c.HitLeaf(c, p)
}

// ClickAt selects (and maybe activates) the child.
func (c *FlowBoxChild) ClickAt(Point) { c.box.childClick(c.Index()) }

// DoubleClickAt activates the child when a single click does not.
func (c *FlowBoxChild) DoubleClickAt(Point) {
	if !c.box.singleClick && c.box.mode != SelectionNone {
		c.box.activate(c.Index())
	}
}

// SetPressed opens a pointer gesture anchored on the child.
func (c *FlowBoxChild) SetPressed(on bool) {
	if on {
		c.box.anchor, c.box.moved = c.Index(), false
		c.box.hold()
	}
}

// DragMove rubber-bands (multiple) or motion-selects from the anchor.
func (c *FlowBoxChild) DragMove(p Point) {
	f := c.box
	if at := f.IndexAt(p); at >= 0 && at != f.anchor {
		f.moved = true
		f.drag(f.anchor, at)
	}
}

// PressEnd closes the gesture, landing its one notification.
func (c *FlowBoxChild) PressEnd() { c.box.release() }

// KeyAction is the box's: a focused child steers the selection.
func (c *FlowBoxChild) KeyAction(a KeyAction, mods Mods) { c.box.KeyAction(a, mods) }

// SelectAll is the box's (ctrl+a on a focused child).
func (c *FlowBoxChild) SelectAll() { c.box.SelectAll() }

// Children is the wrapped widget.
func (c *FlowBoxChild) Children() []Widget { return []Widget{c.child} }

// paintCSSBox paints a container's own stylesheet layers (background,
// gradient, border, shadow) when it has any.
func paintCSSBox(cv *render.Canvas, v *style.Values, bounds render.Rect) {
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, bounds, radiusOr(v, 0), borderOf(v), bg)
	}
}

// SetJustify sets how each line's leftover width is used.
func (f *FlowBox) SetJustify(j Justify) {
	f.justify = j
	f.InvalidateLayout()
}

// Justify is how a wrapping line uses the width its children leave
// over (adw WrapBox's justify, plus the alignment of the rest).
type Justify uint8

// Justifications.
const (
	// JustifyStart packs children at the start edge (the default).
	JustifyStart Justify = iota
	// JustifyCenter centers each line.
	JustifyCenter
	// JustifyEnd packs children at the end edge.
	JustifyEnd
	// JustifyFill grows every child of a line by an equal share.
	JustifyFill
	// JustifySpread widens the gaps between a line's children.
	JustifySpread
)

// distribute turns a line's leftover width into its start x, the
// width each child grows by, and the extra gap between children.
func (j Justify) distribute(x, leftover, n int) (start, grow, gap int) {
	leftover = max(leftover, 0)
	switch j {
	case JustifyCenter:
		return x + leftover/2, 0, 0
	case JustifyEnd:
		return x + leftover, 0, 0
	case JustifyFill:
		return x, leftover / max(n, 1), 0
	case JustifySpread:
		if n > 1 {
			return x, 0, leftover / (n - 1)
		}
	}
	return x, 0, 0
}

// NewWrapBox returns the adw WrapBox: a FlowBox wrapping at
// childSpacing/lineSpacing with justify deciding each line's leftover
// width - one wrapping layout, not two.
func NewWrapBox(childSpacing, lineSpacing int, justify Justify) *FlowBox {
	f := NewFlowBox(childSpacing, lineSpacing)
	f.justify = justify
	f.SetSelectionMode(SelectionNone)
	return f
}
