package widget

import (
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/transfer"
)

// chipMime is the drag mime of a chip moving between ChipBoxes.
const chipMime = "application/x-lys-ui-chip"

// chipDismissPx is the side of a chip's dismiss button's cross.
const chipDismissPx = 12

// Chip is a badge-shaped control: an optional leading widget, its text
// (or any content in its place, an entry for a renamable chip), and,
// while OnDismiss is set, a dismiss button that is keyboard reachable.
// It styles as the chip element, the dismissible class while it has
// the button. In a ChipBox it drags by its body.
type Chip struct {
	composite
	row       *Box
	label     *Label
	leading   Widget
	content   Widget
	dismiss   *Button
	onDismiss func()
	// box is the ChipBox holding the chip, nil outside one.
	box *ChipBox
}

// NewChip returns a chip reading text.
func NewChip(face render.Font, text string) *Chip {
	face = requireFace("widget.NewChip", face)
	c := &Chip{row: NewBox(Row, Current().Spacing/2, 0)}
	c.SetElement("chip")
	c.label = NewLabel(face, badgeTextPx, text, 0)
	c.content = c.label
	c.row.AppendAligned(c.label, false, AlignCenter)
	c.dismiss = NewButton(NewSymbol(SymbolClose, chipDismissPx), 0, 0)
	c.dismiss.AddClass("dismiss")
	c.dismiss.OnClick = func() {
		if c.onDismiss != nil {
			c.onDismiss()
		}
	}
	c.syncDismissName()
	c.initComposite(c, c.row)
	c.boxed = true
	return c
}

// SetText replaces the text.
func (c *Chip) SetText(text string) {
	c.label.SetText(text)
	c.syncDismissName()
}

// Text returns the text.
func (c *Chip) Text() string { return c.label.Text() }

// SetLeading puts w before the content, an icon or a swatch; nil
// removes it.
func (c *Chip) SetLeading(w Widget) {
	if c.leading != nil {
		c.row.Remove(c.leading)
	}
	c.leading = w
	if w != nil {
		c.row.InsertAligned(0, w, false, AlignCenter)
	}
}

// SetContent shows w in place of the text, an entry or nested chips;
// nil restores the text.
func (c *Chip) SetContent(w Widget) {
	if w == nil {
		w = c.label
	}
	at := 0
	if c.leading != nil {
		at = 1
	}
	c.row.Remove(c.content)
	c.content = w
	c.row.InsertAligned(at, w, false, AlignCenter)
}

// SetOnDismiss shows the dismiss button, which runs fn; nil hides it.
func (c *Chip) SetOnDismiss(fn func()) {
	had := c.onDismiss != nil
	c.onDismiss = fn
	switch {
	case fn != nil && !had:
		c.row.AppendAligned(c.dismiss, false, AlignCenter)
		c.AddClass("dismissible")
	case fn == nil && had:
		c.row.Remove(c.dismiss)
		c.RemoveClass("dismissible")
	}
}

// syncDismissName names the dismiss button after the chip.
func (c *Chip) syncDismissName() { c.dismiss.SetAccessibleName("Remove " + c.label.Text()) }

// HitTest keeps the dismiss button's and the content's controls their
// presses; anywhere else is the chip's own, its drag handle.
func (c *Chip) HitTest(p Point) Widget {
	hit := c.composite.HitTest(p)
	if hit == nil || interactiveWithin(hit, c) {
		return hit
	}
	return c
}

// DragContent implements DragSource: a chip in a ChipBox offers its
// move, marked dragging while it travels.
func (c *Chip) DragContent() *DragContent {
	if c.box == nil {
		return nil
	}
	chipDrag.nonce++
	chipDrag.chip = c
	token := strconv.FormatUint(chipDrag.nonce, 10)
	c.AddClass("dragging")
	return &DragContent{
		Mimes: []string{chipMime},
		Write: func(_ string, w io.Writer) error {
			_, err := io.WriteString(w, token)
			return err
		},
		Actions: transfer.ActionMove,
		OnDone: func(transfer.Action) {
			c.RemoveClass("dragging")
			if chipDrag.chip == c {
				chipDrag.chip = nil
			}
		},
	}
}

// a11yName is the text.
func (c *Chip) a11yName() string { return c.label.Text() }

// chipDrag is the chip in flight: a drag's payload names it by nonce,
// so only this process's own chip moves on a drop.
var chipDrag struct {
	nonce uint64
	chip  *Chip
}

// ChipBox wraps its chips into lines, as a FlowBox does, and reorders
// them by drag. It is the chipbox element, a CSS box whose padding is
// part of its drop area: a chip dragged within the box or from another ChipBox
// lands where the drop indicator shows, and OnMoved reports the move
// after the box made it.
type ChipBox struct {
	composite
	flow *FlowBox
	// dropAt is where a hovering chip would land, -1 while none hovers.
	dropAt int
	// OnMoved hears a chip moved from index from of box src (the box
	// itself for a reorder) to index to of this box.
	OnMoved func(src *ChipBox, from, to int)
}

// NewChipBox returns an empty chip box.
func NewChipBox() *ChipBox {
	th := Current()
	b := &ChipBox{flow: NewFlowBox(th.Spacing/2, th.Spacing/2), dropAt: -1}
	b.flow.SetSelectionMode(SelectionNone)
	b.SetElement("chipbox")
	b.initComposite(b, b.flow)
	b.boxed = true
	b.fillWidth = true
	return b
}

// Append adds c after the chips already there.
func (b *ChipBox) Append(c *Chip) { b.Insert(b.flow.Len(), c) }

// Insert adds c at index i (clamped).
func (b *ChipBox) Insert(i int, c *Chip) {
	if c.box != nil {
		c.box.Remove(c)
	}
	c.box = b
	b.flow.Insert(i, c)
}

// Remove takes c out and reports whether it was here.
func (b *ChipBox) Remove(c *Chip) bool {
	i := b.IndexOf(c)
	if i < 0 {
		return false
	}
	b.flow.RemoveAt(i)
	c.box = nil
	return true
}

// Clear removes every chip.
func (b *ChipBox) Clear() {
	for _, c := range b.Chips() {
		c.box = nil
	}
	b.flow.Clear()
}

// Len is the chip count.
func (b *ChipBox) Len() int { return b.flow.Len() }

// ChipAt is the chip at index i, nil out of range.
func (b *ChipBox) ChipAt(i int) *Chip {
	k := b.flow.ChildAt(i)
	if k == nil {
		return nil
	}
	c, ok := k.Child().(*Chip)
	if !ok {
		panic(fmt.Sprintf("widget: ChipBox holds a %T at %d", k.Child(), i))
	}
	return c
}

// Chips are the chips in order.
func (b *ChipBox) Chips() []*Chip {
	out := make([]*Chip, b.flow.Len())
	for i := range out {
		out[i] = b.ChipAt(i)
	}
	return out
}

// IndexOf is c's index, -1 when it is not here.
func (b *ChipBox) IndexOf(c *Chip) int {
	return slices.Index(b.Chips(), c)
}

// DragEnter implements DragEnterer: this process's chips are taken.
func (b *ChipBox) DragEnter(mimes []string, p Point) string {
	if !slices.Contains(mimes, chipMime) || chipDrag.chip == nil {
		return ""
	}
	b.DragHover(p)
	return chipMime
}

// DragHover implements DragHoverer: the indicator follows the pointer.
func (b *ChipBox) DragHover(p Point) {
	if at := b.dropPosition(p); at != b.dropAt {
		b.dropAt = at
		b.Invalidate()
	}
}

// DragLeave implements DragLeaver.
func (b *ChipBox) DragLeave() {
	b.dropAt = -1
	b.Invalidate()
}

// Drop implements Dropper: the chip in flight moves to the indicated
// place. A payload that is not the chip in flight's is ignored.
func (b *ChipBox) Drop(_ string, data []byte, p Point) {
	to := b.dropPosition(p)
	b.DragLeave()
	c := chipDrag.chip
	if c == nil || c.box == nil || string(data) != strconv.FormatUint(chipDrag.nonce, 10) {
		return
	}
	chipDrag.chip = nil
	src := c.box
	from := src.IndexOf(c)
	if src == b {
		if from < to {
			to--
		}
		b.flow.Move(from, to)
	} else {
		src.Remove(c)
		b.Insert(to, c)
	}
	if b.OnMoved != nil {
		b.OnMoved(src, from, to)
	}
}

// dropPosition is where a chip dropped at p lands: before or after the
// chip under p by its center, else after the last chip starting at or
// above p, else the start.
func (b *ChipBox) dropPosition(p Point) int {
	if i := b.flow.IndexAt(p); i >= 0 {
		r := b.flow.ChildAt(i).Bounds()
		if p.X < r.X+r.W/2 {
			return i
		}
		return i + 1
	}
	last := -1
	for i := range b.flow.Len() {
		if b.flow.ChildAt(i).Bounds().Y <= p.Y {
			last = i
		}
	}
	return last + 1
}

// chipIndicatorW is the drop indicator's width.
const chipIndicatorW = 2

// Paint draws the chips, then the drop indicator: a bar in the accent
// in the gap before the chip a drop lands in front of, or after the
// last.
func (b *ChipBox) Paint(cv *render.Canvas) {
	b.composite.Paint(cv)
	if b.dropAt < 0 || b.flow.Len() == 0 {
		return
	}
	gap := b.flow.colSpacing
	var x int
	var r render.Rect
	if k := b.flow.ChildAt(b.dropAt); k != nil {
		r = k.Bounds()
		x = r.X - (gap+chipIndicatorW)/2
	} else {
		r = b.flow.ChildAt(b.flow.Len() - 1).Bounds()
		x = r.X + r.W + (gap-chipIndicatorW)/2
	}
	cv.FillRect(render.Rect{X: x, Y: r.Y, W: chipIndicatorW, H: r.H}, accentOf(b.style(b), Current().Accent))
}
