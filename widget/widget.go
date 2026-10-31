// Package widget is lys-ui's retained-mode widget layer: a tree of widgets
// measured against constraints, arranged into pixel rects, and painted onto
// a render.Canvas. Input handling arrives in M4; HitTest is already here so
// event dispatch can hook into the same geometry.
//
// # API conventions
//
// Constructors are positional, in one fixed order: the font first,
// then the pixel size (sizePx, logical, when the widget paints text at
// a configurable size), then the widget's content and initial state,
// then colors. A nil face panics in the constructor with a message
// naming the argument (see requireFace) — it never fails later, deep
// in shaping.
//
// Event hooks are exported function fields set after construction
// (Button.OnClick, Entry.OnChanged, Dropdown.OnSelect, ...), one per
// event, fired synchronously from whatever produced it. The Router
// methods and the handler interfaces they dispatch to (Clicker,
// KeyActionHandler, HoverSetter, ...) are input plumbing, not app
// hooks, and are not part of this convention.
//
// State comes in getter/setter pairs with no Get prefix: Text and
// SetText, On and SetOn, Alignment and SetAlignment. Every Set* whose
// state is app-meaningful has a matching getter; the router-facing
// Set* methods (SetHovered, SetPressed, SetDragOver) are interface
// plumbing, not state API. Bounds returns the last arranged rect, and
// the zero rect before the first Arrange — a defined value, never a
// panic.
//
// Every coordinate, size, and pixel parameter is logical pixels; the
// device scale is applied exactly once, at the buffer boundary in the
// app and render packages.
package widget

import (
	"slices"
	"sync/atomic"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

// Size is a width and height in pixels.
type Size struct {
	W, H int
}

// Along is s's extent along a: its width along a Row, its height down
// a Column.
func (s Size) Along(a Axis) int {
	if a == Row {
		return s.W
	}
	return s.H
}

// Point is a position in pixels.
type Point struct {
	X, Y int
}

// Widget is one node in the retained widget tree. Measure reports what
// a widget asks for, Arrange gives it its final rect, Paint draws it. A
// container measures its children where it arranges them, so a root
// needs only Arrange.
type Widget interface {
	// Measure reports the widget's request along axis - its width for
	// Row, its height for Column - given forSize, its extent across
	// that axis, or Unknown: GTK's height-for-width measure. The
	// request never depends on the room a parent will offer; the
	// parent decides at Arrange (docs/architecture.md).
	Measure(axis Axis, forSize int) Sizes
	// Arrange assigns the widget's final pixel rect and recurses into
	// children.
	Arrange(r render.Rect)
	// Paint draws the widget, honoring any clip already set on the
	// canvas.
	Paint(cv *render.Canvas)
	// HitTest returns the deepest widget whose area contains p, or nil
	// when p misses the tree below this widget.
	HitTest(p Point) Widget
}

// node carries the arranged bounds, parent link, and tooltip text
// shared by every implementation. Embed it; call HitLeaf from leaf
// HitTests and ArrangeRoot from implementations that position children
// themselves.
//
// It also carries the two caches the frame loop leans on: the
// invalidation flag the damage collector turns into repaint rects, and
// the memoized Measure result, dropped by InvalidateLayout when a
// mutation changes what a widget wants.
type node struct {
	layout layoutProps
	bounds render.Rect
	parent Widget
	// layoutMotion is the programmatic layout motion (SetLayoutMotion)
	// and motion the running one. shownWalk is the damage walk the
	// widget (or a child of it) last painted in, zero before any: a
	// first paint inside a parent shown in an earlier walk is an
	// insertion.
	layoutMotion LayoutMotion
	motion       *layoutMotionState
	shownWalk    uint64
	// departingFrom is the box the widget is painting its way out of
	// after a removal (departure.go), nil otherwise.
	departingFrom *Box
	// sharedTag is the shared-element tag (SetSharedElement) and shared
	// the morph running into this widget.
	sharedTag string
	shared    *sharedTransition
	tooltip   string
	// tooltipMarkup holds the markup when the tooltip was set as
	// markup; tooltip then holds its plain text.
	tooltipMarkup string
	// debugName labels the widget for the inspector's dump and
	// overlay; plain data the toolkit itself never reads.
	debugName string

	// paint damage: invalid marks the bounds as needing a repaint;
	// extras are additional rects (in root coordinates) a widget owes a
	// repaint for; subInvalid mirrors that somewhere below this widget
	// in the tree is dirty, so containers that do not expose children
	// still join the damage union.
	invalid    bool
	subInvalid bool
	extras     []render.Rect

	// measure cache: the requests answered since the cache was last
	// fresh, most recently used first, keyed by axis and forSize. Four
	// slots hold a frame's questions: a width, and heights at the widths
	// a container tries. measureDirty forces a recompute after
	// InvalidateLayout. The style marks join the key: a stale cascade
	// (or one marked by a class or state change) drops every entry,
	// since padding, font-size, and min-* change what the widget wants.
	measureFresh bool
	measureDirty bool
	// subStyle is set while a descendant's cascade is stale
	// (markStyleDirty): the cached measure re-runs down to it.
	subStyle bool
	measures [4]measureEntry
	// measureMisses tallies the measures the cache could not answer;
	// the static-tree pin test reads it.
	measureMisses int

	// themeSeen stamps the theme generation this widget last joined a
	// frame with; SetTheme bumps themeGen so every widget repaints once.
	themeSeen uint64

	// computed style (docs/css.md): the cascade result plus generation
	// stamps. csGen is the cascade's own freshness: style() recomputes
	// until restyle runs under the current generation. styleSeen is the
	// measure cache's: a stale value drops the cached measure (a load
	// relayouts the tree by stamp), and measureStore re-stamps it even
	// for widgets whose measure never reads the cascade. styleDmg gates
	// the damage collector, which drains it — the three together let a
	// stylesheet load relayout and repaint everything without walking
	// the tree. styleDirty is the fine-grained half: set by class, id,
	// state, and inherited-value changes, it forces the next read to
	// recompute and diff-invalidates the smallest correct region.
	cs         style.Values
	csGen      uint64
	styleSeen  uint64
	styleDmg   uint64
	styleDirty bool
	// styleRoot ends the style chain here: the node keeps its layout
	// parent, but :root, inheritance and scoped-sheet lookup stop at it
	// (SetStyleRoot).
	styleRoot bool
	// accName is the accessible name SetAccessibleName set.
	accName string
	// noFocusOnClick keeps a press on the widget from moving the focus
	// (SetFocusOnClick).
	noFocusOnClick bool
	// surfaceHost opens the popups of the tree under this node
	// (SetSurfaceHost): set on a surface's root.
	surfaceHost SurfaceHost
	// styleAnchor continues a parentless tree's style chain at another
	// widget (SetStyleAnchor); anchored lists the trees anchored here,
	// which inherit from this node like its children.
	styleAnchor Widget
	anchored    []Widget
	// viewport is the @media viewport a style root's window reported
	// (SetMediaViewportFor); zero falls back to the global one.
	viewport struct{ w, h int }

	// style identity: the element name (a constructor-set override and
	// the type-derived default, resolved once), the `#id`, the classes
	// `.name` selectors match, and the app-set data attributes [name]
	// selectors test.
	element  string
	elemName string
	id       string
	classes  []string
	attrs    []style.Attr
	// focused is the router's focus bit for :focus matching; hover and
	// press stay in each widget's own fields, which the router already
	// drives. focusVisible marks a focus that arrived by keyboard
	// traversal (:focus-visible); focusWithin counts the focused widgets
	// at or below this one (:focus-within); hoverChain is set on the
	// hovered widget and every ancestor (:hover, GTK's rule).
	focused      bool
	focusVisible bool
	focusWithin  int
	hoverChain   bool
	// onHoverWithin hears hoverChain flip (SetOnHoverWithin).
	onHoverWithin func(on bool)
	// onFocus hears the widget gain or lose keyboard focus
	// (SetOnFocusChanged).
	onFocus func(focused bool)
	// onClickWithin hears clicks on non-clicking descendants
	// (SetOnClickWithin).
	onClickWithin func()
	// onPointer and onScroll see presses and wheel steps first
	// (SetOnPointer, SetOnScroll).
	onPointer func(e PointerEvent) bool
	onScroll  func(e ScrollEvent) bool
	// flags are the application-driven states (SetState).
	flags StateFlags
	// inline is the widget-scoped declaration block (SetInlineStyle),
	// kept with its source and priority.
	inline     *style.Block
	inlineSrc  string
	inlinePrio int
	// sheets are the stylesheets attached to this subtree.
	sheets []*Stylesheet
	// cursorName is the hover pointer shape (SetCursorName), empty for
	// the widget default.
	cursorName string
	// ink is how far the computed style paints outside the border box
	// (outer shadows, the outline): damage grows by it.
	ink render.Insets
	// trCancel runs while a transition tween drives the cache's
	// animated channels (transition.go); nil when none is in flight.
	trCancel anim.Cancel
	// anims is the CSS animation state (animations.go), nil while the
	// node runs no animation.
	anims *nodeAnim
	// self is the widget embedding this node, recorded by the first
	// arranging container or, for a root no container holds, its first
	// style, so a state flip can walk its own subtree.
	self Widget
	// sibs is this node's filtered child list as the sibling selectors
	// read it, memoized per damage generation (visibleSiblings); kids
	// are this node's own children as :has()/:empty walk them, adapters
	// and all, memoized the same way (kidsAdaps are their reused
	// targeters).
	sibs      []Widget
	sibsGen   uint64
	kids      []style.Node
	kidsAdaps []*styleTargeter
	kidsGen   uint64

	// pseudo holds the computed ::before/::after/::first-letter styles
	// (stylepart pseudo-elements style through the parts themselves),
	// stamped with the cascade generation; nil while no rule addresses
	// any of them, which is nearly every widget, as each set is 13 KB.
	pseudo    *[len(pseudoNames)]style.Values
	pseudoGen uint64

	// disabled is the widget's own half of the enable state: false (the
	// zero value) means enabled, so plain widgets accept input without
	// opting in. The other half is inherited: widget.IsEnabled walks the
	// ancestors, so a disabled container disables its subtree by query
	// — see SetEnabled.
	disabled bool

	// hidden is the widget's own half of the visibility state: false
	// (the zero value) means visible. Containers skip hidden children
	// in Measure, Arrange, Paint, and HitTest, so the space reflows to
	// the siblings; widget.IsVisible folds the ancestors in.
	hidden bool
}

// SetOnHoverWithin registers fn to hear the pointer enter or leave the
// widget or any descendant (the :hover chain; GTK's motion controller
// on a container): a row that reveals its actions on hover whichever
// child the pointer is over. nil unregisters.
func (n *node) SetOnHoverWithin(fn func(on bool)) { n.onHoverWithin = fn }

// HoverWithin reports whether the pointer is over the widget or a
// descendant.
func (n *node) HoverWithin() bool { return n.hoverChain }

// SetOnFocusChanged registers fn to hear the widget gain or lose
// keyboard focus, however focus moved (a click elsewhere, Tab, a
// programmatic SetFocus, the window losing the focused widget): an
// entry that commits its text when the user leaves it. nil
// unregisters.
func (n *node) SetOnFocusChanged(fn func(focused bool)) { n.onFocus = fn }

// SetOnClickWithin registers fn to hear a click (press and release on
// the same widget) anywhere in the widget that nothing nearer takes:
// GTK's GestureClick on a container. A button, entry or other
// clicking widget inside keeps its own clicks, so a card can open on
// a click and still hold a working close button. The nearest
// registered ancestor of the clicked widget hears it; nil
// unregisters.
func (n *node) SetOnClickWithin(fn func()) { n.onClickWithin = fn }

// clickWithin is the hook SetOnClickWithin registered.
func (n *node) clickWithin() func() { return n.onClickWithin }

// SetTooltip sets hover text shown after a dwell; empty clears it.
func (n *node) SetTooltip(s string) {
	checkLoop("SetTooltip")
	n.tooltip, n.tooltipMarkup = s, ""
}

// SetTooltipMarkup sets a tooltip in RichLabel markup (bold, italic,
// links' look): the card renders it rich, while TooltipText - the
// accessible name - is its plain text. Markup ParseMarkup rejects
// shows literally.
func (n *node) SetTooltipMarkup(markup string) {
	checkLoop("SetTooltipMarkup")
	runs, ok := ParseMarkup(markup)
	if !ok {
		n.SetTooltip(markup)
		return
	}
	n.tooltip, _ = spansText(runs)
	n.tooltipMarkup = markup
}

// TooltipText returns the hover text, empty when none is set.
func (n *node) TooltipText() string { return n.tooltip }

// TooltipMarkup returns the tooltip's markup and whether it was set as
// markup (TooltipMarkupper).
func (n *node) TooltipMarkup() (string, bool) { return n.tooltipMarkup, n.tooltipMarkup != "" }

// SetCursorName requests the pointer shape while hovered, an xcursor
// name ("pointer", "text" — what set_cursor_from_name takes); empty
// falls back to the widget default. A widget's own CursorName
// implementation outranks it.
func (n *node) SetCursorName(s string) {
	checkLoop("SetCursorName")
	n.cursorName = s
}

// CursorNameOf returns the hover pointer shape SetCursorName put on w,
// empty when none — the fallback cursorFor reads after the widget's
// own CursorName.
func CursorNameOf(w Widget) string {
	if n := nodeOf(w); n != nil {
		return n.cursorName
	}
	return ""
}

// SetAnimationsInstant collapses every transition and animation tween
// to its end state (GTK's gtk-enable-animations off) and returns the
// restore function. Already-running tweens keep their schedules; only
// launches after the call are affected. Hosts that screenshot, test,
// or honor reduced motion set it.
func SetAnimationsInstant(on bool) (restore func()) { return anim.SetInstant(on) }

// SetEnabled turns the widget's own interactivity on or off; every
// node-embedding widget inherits it. The zero state is enabled.
//
// Propagation model (pinned): a container's flag does NOT rewrite its
// children's. Disable a Box and the whole subtree reads as disabled
// through widget.IsEnabled, the per-query ancestor walk — the same
// shape as IsInteractive. Per-query beats a recursive flag because it
// keeps every widget's own decision intact: re-enabling the container
// never resurrects a child the app disabled on purpose, and a child
// disabled after its container keeps its state when the container
// comes back. Each widget's own flag only ever changes through its own
// SetEnabled.
//
// The flip invalidates self and — for containers that expose Children,
// which override this with invalidateTree — every descendant, so the
// muted paint lands on the next frame without a relayout: enabled
// state changes what Paint draws, never what Measure wants. The style
// marks go stale with the flags: :disabled rules may match now.
func (n *node) SetEnabled(enabled bool) {
	checkLoop("SetEnabled")
	if n.disabled == !enabled {
		return
	}
	n.disabled = !enabled
	n.markStyleDirty()
	n.Invalidate()
}

// Enabled reports the widget's own flag, NOT the effective state: a
// widget inside a disabled container still reads true here. Input
// paths and Paint must consult widget.IsEnabled, which folds the
// ancestors in.
func (n *node) Enabled() bool { return !n.disabled }

// SetVisible hides or shows the widget in its container's layout. An
// invisible widget measures and arranges to zero, paints nothing, and
// drops out of hit testing - the container's other children reclaim
// the space. State changes what the container does with the widget,
// so the parent relayouts.
func (n *node) SetVisible(visible bool) {
	checkLoop("SetVisible")
	if n.hidden == !visible {
		return
	}
	n.hidden = !visible
	n.InvalidateLayout()
	if n.parent != nil {
		restyleChildren(n.parent)
	}
}

// Visible reports the widget's own visibility flag, not the effective
// state: a widget inside an invisible container still reads true.
// Layout, Paint, and HitTest paths consult widget.IsVisible, which
// folds the ancestors in.
func (n *node) Visible() bool { return !n.hidden }

// IsVisible folds the visibility of every ancestor: false when the
// widget or any container above it is hidden, by SetVisible or by a
// computed display: none — either way the widget is not laid out,
// painted, or hit. CSS visibility: hidden keeps the widget laid out
// and is not folded here (shown).
func IsVisible(w Widget) bool {
	for node := w; node != nil; node = parentOf(node) {
		if !ownVisible(node) {
			return false
		}
	}
	return true
}

// ownVisible is IsVisible's per-widget half: w's own SetVisible flag
// and computed display, its ancestors aside. A container testing its
// children reads it alone — its own visibility is settled by its being
// laid out, painted, or hit at all.
func ownVisible(w Widget) bool {
	if n, ok := w.(interface{ Visible() bool }); ok && !n.Visible() {
		return false
	}
	n := nodeOf(w)
	if n == nil {
		return true
	}
	// An unparented widget's cascade cannot resolve yet; its last one
	// stands until a container parents it, whose relayout then reads
	// the fresh value.
	v := &n.cs
	if n.parent != nil || n.styleRoot {
		v = n.style(w)
	}
	return !v.Has(style.PropDisplay) || v.Layout.Display != style.DisplayNone
}

// shown reports whether w's computed visibility shows it: hidden and
// collapse keep the widget's space but paint nothing and take no hits.
func shown(w Widget) bool {
	n := nodeOf(w)
	return n == nil || n.style(w).Layout.Visibility == style.VisibilityVisible
}

// invalidateTree marks self and every descendant for repaint.
// Containers call it after a state flip (enable/disable) that the
// query walks project onto the whole subtree at once. Each node's
// computed style goes stale with it — the effective disabled state
// changed subtree-wide, so :disabled rules may match now.
func invalidateTree(self Widget) {
	walkTree(self, 0, func(w Widget, _ int) {
		if v, ok := w.(interface{ Invalidate() }); ok {
			v.Invalidate()
		}
		if n := nodeOf(w); n != nil {
			n.markStyleDirty()
		}
	})
	markDamage()
}

// Invalidate schedules a repaint of the widget's arranged bounds. Call
// it after any state change that alters what Paint draws. Layout is
// untouched; mutations that change the wanted size need InvalidateLayout.
func (n *node) Invalidate() {
	checkLoop("Invalidate")
	n.invalid = true
	if n.unseen() {
		return
	}
	markDamage()
	n.markSub()
}

// unseen reports a widget nothing on screen derives from yet: never
// styled and in no tree. Changing it owes no frame and stales no other
// style; its first style and paint read it as it is then.
func (n *node) unseen() bool { return n.parent == nil && n.csGen == 0 }

// InvalidateRect schedules a repaint of r, an arbitrary rect in root
// coordinates - for widgets that owe pixels outside their arranged
// bounds, such as a scrollbar strip beside a viewport. Unlike
// Invalidate it does not mark the arranged bounds themselves.
func (n *node) InvalidateRect(r render.Rect) {
	checkLoop("InvalidateRect")
	if r.Empty() || slices.Contains(n.extras, r) {
		return
	}
	n.extras = append(n.extras, r)
	markDamage()
	n.markSub()
}

// markSub flags ancestors that some descendant needs a repaint, so
// the damage collector notices through widgets that hide their
// children from it (a composite, a Button, a virtualized List): the
// collector descends only through Children, so every ancestor without
// it is flagged, all the way to the root. A Children container above
// a hiding one does not stop the walk, and neither does an ancestor
// already flagged: the collector drains only the outermost hiding
// ancestor it reaches, so inner flags can be stale. Parent links come
// from the last Arrange.
func (n *node) markSub() {
	for p := n.parent; p != nil; p = parentOf(p) {
		if _, ok := p.(childser); ok {
			continue
		}
		if s, ok := p.(interface{ markSubInvalid() }); ok {
			s.markSubInvalid()
		}
	}
}

// markSubInvalid records a descendant invalidation.
func (n *node) markSubInvalid() {
	n.subInvalid = true
	markDamage()
}

// InvalidateLayout drops the cached Measure result here and in every
// ancestor, so the next frame remeasures the affected branch only, and
// schedules a repaint of the bounds (a size change usually alters the
// painting too).
func (n *node) InvalidateLayout() {
	n.measureDirty = true
	n.dropMeasureCache()
	n.layout.dropExpand()
	n.Invalidate()
	n.markSubLayout()
}

// markSubLayout propagates the measure drop upward: every container
// cache between here and the root depends on this widget's natural
// size, whatever its child exposure. It climbs to the root even past an
// ancestor already dirty: a custom widget that measures without the
// cache (a Base embedder) stays dirty for good, and stopping there left
// every cache above it stale.
func (n *node) markSubLayout() {
	p := n.parent
	for p != nil {
		s, ok := p.(interface{ markMeasureDirty() bool })
		if !ok {
			return
		}
		s.markMeasureDirty()
		p = parentOf(p)
	}
}

// markMeasureDirty drops this widget's cached measure because a
// descendant needs remeasuring; reports whether it was already dropped.
func (n *node) markMeasureDirty() bool {
	was := n.measureDirty
	n.measureDirty = true
	n.dropMeasureCache()
	n.layout.dropExpand()
	return was
}

// measureEntry is one slot of the measure cache.
type measureEntry struct {
	axis    Axis
	forSize int
	out     Sizes
	ok      bool
}

func (n *node) dropMeasureCache() {
	n.measureFresh = false
	n.measures = [4]measureEntry{}
}

// measureHit returns the cached request along axis at forSize when it
// is still valid. Widget Measures open with it to skip recomputation on
// the static-tree fast path. A stale or marked cascade misses too: a
// stylesheet load relayouts the tree by stamp, with no walk.
func (n *node) measureHit(axis Axis, forSize int) (Sizes, bool) {
	if !n.measureFresh || n.measureDirty || n.styleDirty || n.styleSeen != styleGen {
		n.measureMiss(axis, forSize)
		// Every entry predates the invalidation: none may come back
		// once the recompute below stamps the cache fresh.
		n.dropMeasureCache()
		return Sizes{}, false
	}
	if n.subStyle {
		n.measureMiss(axis, forSize)
		return Sizes{}, false
	}
	for i, e := range n.measures {
		if e.ok && e.axis == axis && e.forSize == forSize {
			copy(n.measures[1:i+1], n.measures[:i])
			n.measures[0] = e
			return e.out, true
		}
	}
	n.measureMiss(axis, forSize)
	return Sizes{}, false
}

// measureMiss counts a measure the cache could not answer, and names it
// in a lysdebug build.
func (n *node) measureMiss(axis Axis, forSize int) {
	n.measureMisses++
	if debug.Enabled {
		noteMeasureMiss(n, axis, forSize)
	}
}

// measureMissCount reports how many measures this widget computed past
// its cache; the static-tree pin test reads it.
func (n *node) measureMissCount() int { return n.measureMisses }

// measureStore records s as the cache entry for axis at forSize and
// returns it, closing the measureHit pair. The cache is stamped with the
// style generation it measured under.
func (n *node) measureStore(axis Axis, forSize int, s Sizes) Sizes {
	if !n.measureFresh {
		n.measures = [4]measureEntry{}
	}
	copy(n.measures[1:], n.measures[:len(n.measures)-1])
	n.measures[0] = measureEntry{axis: axis, forSize: forSize, out: s, ok: true}
	n.measureFresh = true
	n.measureDirty = false
	n.subStyle = false
	n.styleSeen = styleGen
	return s
}

// takeDamage drains one widget's pending repaint: it reports the
// arranged bounds when the bounds themselves are owed (flag set, a
// descendant hidden from the collector is dirty, or the theme changed)
// plus any extra rects, and clears the flags. Extras alone - scrollbar
// strips beside a viewport, say - do not drag the whole bounds in. The
// damage collector calls it walking down the tree.
func (n *node) takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool) {
	boundsDirty := n.invalid || n.subInvalid || n.themeSeen != themeGen || n.styleDmg != styleGen
	n.styleDmg = styleGen
	n.invalid = false
	n.subInvalid = false
	n.themeSeen = themeGen
	extra, n.extras = n.extras, n.extras[:0]
	if !boundsDirty && len(extra) == 0 {
		return render.Rect{}, nil, false
	}
	if !boundsDirty {
		return render.Rect{}, extra, true
	}
	return n.ink.Grow(n.bounds), extra, true
}

// Arrange records the widget's rect. A rect that moved or resized from
// the previous frame also invalidates the old pixels, so partial
// damage does not leave the widget's old paint behind.
func (n *node) Arrange(r render.Rect) {
	if !n.bounds.Empty() && r != n.bounds {
		n.InvalidateRect(n.ink.Grow(n.bounds))
		n.Invalidate()
	}
	// A resized query container's descendants recompute their
	// @container conditions and container units against the new
	// bounds; any other widget's resize leaves every style as it was.
	if r != n.bounds && sensNow.containers && !n.unseen() && n.queryContainer() {
		restyleSubtree(n.self)
	}
	n.bounds = r
}

// queryContainer reports whether the node's computed container-type
// makes it a query container; before its container records it, the
// last computed style answers.
func (n *node) queryContainer() bool {
	v := &n.cs
	if n.self != nil {
		v = n.style(n.self)
	}
	return v.ContainerType != style.ContainerNormal
}

// Bounds returns the last arranged rect. Before the first Arrange —
// and that includes a widget built but never put in a tree — it is the
// zero rect, a defined value, never a panic and never stale geometry.
func (n *node) Bounds() render.Rect {
	return n.bounds
}

// Parent returns the container that arranged this widget, or nil for the
// tree root.
func (n *node) Parent() Widget {
	return n.parent
}

// setParent records the arranging container; containers call it on their
// children during Arrange. The first link drops the style stamps: the
// cascade walks the ancestor chain, which only exists once a container
// has arranged this widget, so the next read recomputes — and repaints,
// since an inherited value may now resolve differently.
func (n *node) setParent(p, self Widget) {
	if self != nil {
		n.self = self
	}
	if b := n.departingFrom; b != nil && p != Widget(b) {
		b.endDepartureOf(n)
	}
	if n.parent == nil && p != nil {
		n.csGen = 0
		n.styleSeen = 0
		n.styleDmg = 0
		n.parent = p
		// Measured before it had ancestors, the widget resolved no scoped
		// stylesheet and inherited nothing: its cached size, and every
		// container cache above it, may be wrong now.
		n.InvalidateLayout()
		// So may every descendant styled meanwhile: its chain stopped
		// here (a Paned arranges its panes before it is parented).
		if n.self != nil {
			restyleSubtree(n.self)
		}
		hasRelink(p)
		return
	}
	n.parent = p
}

// hasRelink marks what a :has() match may have read wrong before a
// child of p was linked: p and its ancestors (their relative
// selectors walk down through the new link) and p's other children
// (their sibling forms walk across it). Without :has() selectors
// loaded it is free.
func hasRelink(p Widget) {
	if !sensNow.has {
		return
	}
	for a := nodeOf(p); a != nil; a = parentNodeOf(a) {
		a.markStyleDirty()
	}
	for _, k := range appendStyleKids(p, nil) {
		if kn := nodeOf(k); kn != nil {
			kn.markStyleDirty()
		}
	}
	markDamage()
}

// LayoutPending reports whether w's cached measure is stale - a
// descendant was parented, restyled or resized since - so a host that
// just arranged it must measure and arrange again before painting.
func LayoutPending(w Widget) bool {
	if n := nodeOf(w); n != nil {
		return n.measureDirty || n.styleDirty || n.styleSeen != styleGen
	}
	// A pass-through wrapper (the inspector overlay) owns no cache:
	// what it wraps decides.
	if c, ok := w.(childser); ok {
		return slices.ContainsFunc(c.Children(), LayoutPending)
	}
	return false
}

// parentOf returns w's parent, or nil.
func parentOf(w Widget) Widget {
	if p, ok := w.(interface{ Parent() Widget }); ok {
		return p.Parent()
	}
	return nil
}

// outerOf is the widget the tree holds for w: a type embedding a
// container (a drop zone around a FlowBox, a panel around a Box)
// records itself as that node's self when it is parented, so the
// children's parent links name the outer type, whose interfaces the
// ancestor walks (drop targets, click hooks, scroll handlers) see.
func outerOf(w Widget) Widget {
	if n := nodeOf(w); n != nil && n.self != nil {
		return n.self
	}
	return w
}

// setParents records parent as the arranging container of every child.
func setParents(parent Widget, kids ...Widget) {
	parent = outerOf(parent)
	for _, k := range kids {
		if k == nil {
			continue
		}
		if s, ok := k.(interface{ setParent(p, self Widget) }); ok {
			s.setParent(parent, k)
		}
	}
}

// clearParents drops the recorded parent of every ws — the bookkeeping
// a removal owes: a detached widget must stop claiming its old
// ancestor, or the IsEnabled/markSub walks (and a re-Append elsewhere)
// would read a tree it no longer belongs to.
func clearParents(ws ...Widget) {
	for _, w := range ws {
		if w == nil {
			continue
		}
		if s, ok := w.(interface{ setParent(p, self Widget) }); ok {
			s.setParent(nil, nil)
		}
	}
}

// removedHook is the callback the tree-mutation API (Box.Remove
// family, Stack.Remove, Scroll.SetChild, Grid.Remove, Notebook.CloseTab)
// fires with each detached widget. The Application installs one that
// drops the widget from every window router's hover, press, focus, and
// drop-target state, so input never reaches a ghost — and through
// router.Hovered() neither does a tooltip dwell (app's tooltipCtl).
var removedHook atomic.Pointer[func(Widget)]

// SetRemovedHook installs the hook fired when a widget is detached from
// its container; nil disables. One hook per process, like SetInvoker —
// the last install wins. It runs on the goroutine that mutated the tree
// (the loop goroutine, per docs/threading.md), while the widget is
// still linked to its container, so a router can find the focus
// traversal neighbor of a removed focus.
func SetRemovedHook(fn func(Widget)) {
	if fn == nil {
		removedHook.Store(nil)
		return
	}
	removedHook.Store(&fn)
}

// notifyRemoved detaches every non-nil ws's tree (Detach) and fires
// the removal hook for it.
func notifyRemoved(ws ...Widget) {
	fn := removedHook.Load()
	for _, w := range ws {
		if w == nil {
			continue
		}
		Detach(w)
		if fn != nil {
			(*fn)(w)
		}
	}
}

// HitLeaf is the leaf hit test every widget without children routes
// through: the widget itself inside its bounds, unless it is hidden or
// the pointer passes through it (pointer-events: none on it or an
// ancestor).
func (n *node) HitLeaf(self Widget, p Point) Widget {
	if n.bounds.Contains(p.X, p.Y) && ownVisible(self) && !pointerEventsNone(self) && shown(self) && !clippedOut(self, p) {
		return self
	}
	return nil
}

// HitChild hit-tests a container's child at p, PaintChild's
// counterpart: nil for a child hidden by SetVisible or display: none,
// whoever its container is, else the child's own HitTest. Containers
// defined outside the kit call it for their children too.
func HitChild(w Widget, p Point) Widget {
	if w == nil || !ownVisible(w) {
		return nil
	}
	return w.HitTest(p)
}

// PaintChild paints a container's child and then, when the frame armed
// it for that child, the keyboard focus ring (render.Canvas.MarkFocus),
// so later siblings cover the ring. Containers defined outside the kit
// call it for their children too; one that does not still gets the ring,
// drawn over the whole frame at its end. A child hidden by SetVisible
// or display: none paints nothing, whoever its container is, and nor
// does one its computed visibility hides, except that a Box still
// paints its children, which may show themselves (visibility: visible
// inside a hidden box).
func PaintChild(cv *render.Canvas, w Widget) {
	if !ownVisible(w) {
		return
	}
	if !shown(w) {
		if _, ok := w.(contentsHost); !ok {
			return
		}
	}
	n := nodeOf(w)
	if n != nil && n.layout.maximum != (Size{}) {
		prev := cv.PushClip(n.bounds)
		defer cv.PopClip(prev)
	}
	switch {
	case n != nil && n.sharedTag != "":
		n.paintShared(cv, w)
	case n != nil && n.moving(w):
		n.paintMoving(cv, w)
	default:
		w.Paint(cv)
	}
	cv.Painted(w)
}

// measureChild measures a container's child, parented first: the
// child's cascade reaches the stylesheets above before its first
// measure, so a measure ahead of the first arrange is already styled.
func measureChild(parent, child Widget, axis Axis, forSize int) Sizes {
	setParents(parent, child)
	return measureWithMargin(child, axis, forSize)
}

func walkWidgets(w Widget, visit func(Widget) (descend bool)) {
	if !visit(w) {
		return
	}
	var kids []Widget
	switch p := w.(type) {
	case interface{ Children() []Widget }:
		kids = p.Children()
	default:
		kids = styleKids(p)
	}
	for _, c := range kids {
		walkWidgets(c, visit)
	}
}
