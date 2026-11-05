package widget

import "github.com/stubbedev/lys-lib/render"

// Base is the embeddable shared state for widgets defined outside the
// kit: it carries the arranged bounds, parent link, invalidation flags,
// and measure cache the frame loop leans on, exactly what internal
// widgets get from node. Embed it, implement the Widget interface, and
// route leaf hit tests through HitLeaf.
type Base struct {
	node
	inkReach    render.Insets
	inkDeclared bool
}

// NewBase returns a zero Base for embedding:
//
//	type ruler struct {
//	    widget.Base
//	    ...
//	}
func NewBase() Base { return Base{} }

// SetInkBounds declares that the widget paints nothing outside its
// bounds grown by reach, and its children nothing outside theirs. A
// widget that declares it joins the kit's culling: a paint whose clip
// misses that ink skips it, a Scroll copies its pixels on a step instead
// of repainting it, and rows far outside a scrolled view are not
// arranged. Without it the widget may paint anywhere, and is always
// visited.
func (b *Base) SetInkBounds(reach render.Insets) {
	b.inkReach, b.inkDeclared = reach, true
	b.settleInk()
}

func (b *Base) ownInk() (Widget, render.Insets, bool) {
	return b.self, b.inkReach, b.inkDeclared
}

// ArrangeSelf records the widget's own rect. A container defined
// outside the kit shadows the promoted node.Arrange with its child
// recursion; call this first so the shadowed bookkeeping still runs.
func (b *Base) ArrangeSelf(r render.Rect) { b.Arrange(r) }

// SetParents records parent as the arranging container of every child,
// so containers defined outside the kit wire the cascade the same way
// the built-in ones do during Arrange.
func SetParents(parent Widget, kids ...Widget) { setParents(parent, kids...) }
