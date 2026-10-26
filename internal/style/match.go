package style

import (
	"slices"
	"strings"
)

// Attr is one app-set data attribute a widget carries: attribute
// selectors test it by name.
type Attr struct {
	Name  string
	Value string
}

// Target is one node's selector-relevant facts, filled by the widget
// layer for the node being styled and for each node a combinator walk
// crosses.
type Target struct {
	Element string
	ID      string
	Classes []string
	State   State
	// Role is the node's semantic role name (a11y), the [role=...]
	// selector's value.
	Role string
	// Attrs are the app-set data attributes (SetAttr), [name] and
	// [name=value] selectors' subject.
	Attrs []Attr
	// HasText reports text content: a widget showing text has a text
	// child in the DOM's terms, so :empty does not match it.
	HasText bool
	// Inline is the node's own declaration block (a widget-scoped style,
	// GTK's per-widget provider), applied at InlinePriority. It is read
	// only for the node being styled.
	Inline         *Block
	InlinePriority int
}

// attrValue reads one attribute, the built-in names (id, class, role)
// included.
func (t *Target) attrValue(name string) (string, bool) {
	switch name {
	case "id":
		return t.ID, t.ID != ""
	case "class":
		return strings.Join(t.Classes, " "), len(t.Classes) > 0
	case "role":
		return t.Role, t.Role != ""
	}
	for i := range t.Attrs {
		if t.Attrs[i].Name == name {
			return t.Attrs[i].Value, true
		}
	}
	return "", false
}

// nodeIDer is the optional identity a Node can expose when one
// widget is wrapped in several adapters: identity then compares by
// it, not by adapter pointer.
type nodeIDer interface{ StyleID() any }

// sameNode reports whether two Node values wrap the same node.
func sameNode(a, b Node) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a == b {
		return true
	}
	ai, ok := a.(nodeIDer)
	if !ok {
		return false
	}
	bi, ok := b.(nodeIDer)
	return ok && ai.StyleID() == bi.StyleID()
}

// Node is one node of the widget tree as the matcher sees it.
type Node interface {
	// StyleTarget fills t with the node's selector facts.
	StyleTarget(t *Target)
	// StyleParent returns the parent node, or nil at the root.
	StyleParent() Node
	// StylePrev returns the previous visible sibling, or nil.
	StylePrev() Node
	// StylePosition returns the node's 1-based index among its visible
	// siblings and their count; a root reports (1, 1).
	StylePosition() (index, count int)
	// StyleTypedPosition returns the 1-based index among the visible
	// siblings of element name and their count (:nth-of-type and
	// friends).
	StyleTypedPosition(element string) (index, count int)
	// StyleChildren returns the visible children, or nil for a leaf;
	// :empty and :has() read it.
	StyleChildren() []Node
}

// Layer is one stylesheet in a cascade with its priority: GTK's
// provider priority, compared before specificity. Among equal
// priorities, a later layer wins ties.
type Layer struct {
	Sheet    *Sheet
	Priority int
}

// Priorities, GTK's STYLE_PROVIDER_PRIORITY_* values.
const (
	PriorityFallback    = 1
	PriorityTheme       = 200
	PrioritySettings    = 400
	PriorityApplication = 600
	PriorityUser        = 800
)

// rank packs one declaration's cascade standing into a comparable
// value, most significant first (css-cascade-6 §6.1, with GTK's
// provider priority standing in for the origins): importance, the
// provider priority, the cascade layer (unlayered outranks every
// layer for normal declarations; important ones reverse the layer
// order, so an earlier layer's important declaration wins and a
// layered one beats an unlayered one), specificity, @scope proximity
// (prox counts the generations up to the scoping root, nearer
// winning; unscoped rules have none, the weakest), then the sheet's
// install position and source order.
func rank(prio int, sp spec, layer, sheet, order int, imp bool, prox int) uint64 {
	clampTo := func(v, bits int) uint64 {
		return min(uint64(max(v, 0)), 1<<bits-1)
	}
	switch {
	case layer == inlineLayer:
		layer = unlayered
	case imp && layer == unlayered:
		layer = 0
	case imp:
		layer = unlayered - 1 - min(layer, unlayered-2)
	}
	r := clampTo(prio, 10)<<53 | clampTo(layer, 8)<<45 | clampTo(sp.ids, 6)<<39 |
		clampTo(sp.cls, 8)<<31 | clampTo(sp.els, 6)<<25 | proxKey(prox)<<21 |
		clampTo(sheet, 6)<<15 | clampTo(order, 15)
	if imp {
		r |= 1 << 63
	}
	return r
}

// The layer keys rank reads: unlayered is the key of a rule outside
// every @layer, inlineLayer marks a widget's inline declarations,
// which are element-attached and outrank every layer whatever their
// importance.
const (
	unlayered   = 255
	inlineLayer = 256
)

// rankPriority reads the provider priority back out of a rank.
func rankPriority(rk uint64) int { return int(rk >> 53 & (1<<10 - 1)) }

// noProximity is the proximity of an unscoped rule.
const noProximity = -1

// proxKey ranks a scoping proximity: nearer roots rank higher, every
// scoped rule above an unscoped one, and proximities past the field's
// range tie.
func proxKey(prox int) uint64 {
	if prox < 0 {
		return 0
	}
	return uint64(15 - min(prox, 14))
}

// maxHasBudget bounds the :has() work of one compute: node visits a
// relational argument may spend before it stops matching
// (conservatively false). :has is the one selector that walks down
// into subtrees, so without a budget a sheet could make matching
// quadratic in tree size; with it, one compute stays O(rules +
// budget).
const maxHasBudget = 4096

// Scratch holds one matcher's reusable buffers. Style work runs on the
// event-loop goroutine, so one Scratch per process serves every
// compute; it is not safe for concurrent use.
type Scratch struct {
	self Target
	// bufs are the nested-matching targets: one per functional
	// pseudo-class nesting level (:is inside :has inside ...), so a
	// nested evaluation never clobbers the target it runs within.
	bufs [maxPseudoDepth + 2]Target
	// sibBufs are the sibling-fill targets of :nth-child(of S), one
	// per depth like bufs: a selector inside the of-list runs one
	// level deeper, so its own nth-of never clobbers this one's.
	sibBufs [maxPseudoDepth + 2]Target
	// hasBudget is the remaining :has() node visits of this compute.
	hasBudget int
	// pseudo, when not empty, restricts the compute to the rules of
	// that pseudo-element (::before, ::after) instead of the element's
	// own rules.
	pseudo string
	// prevCounters and srcCounters are the previous sibling's counter
	// set and the set of the element immediately preceding the node in
	// tree order, which the widget layer supplies when counters are in
	// play (Compute resets them).
	prevCounters, srcCounters *CounterScope
	// media is the environment the sheets' @media conditions evaluate
	// against, copied from the compute's Env.
	media Media
	// conts are the node's query containers, nearest first, that its
	// @container conditions and container units resolve against.
	conts []Container
	// revertChain records every declaration per layer when a loaded
	// sheet carries a revert-layer value: the chain computeProp walks
	// down, layer below layer.
	revertChain bool
	chain       []chainEntry

	best      [numProps]uint64
	win       [numProps]*decl
	bestLayer [numProps]uint8
	// winStart/bestStart track the winning @starting-style
	// declaration per longhand — the entry transition's values.
	bestStart [numProps]uint64
	winStart  [numProps]*decl
	// offered and offeredStart are the longhands holding a winner in
	// win and winStart: the compute visits those alone.
	offered, offeredStart PropSet

	customs []customWin
	byName  map[string]int

	// metas are the winning content/counter-reset/counter-increment
	// declarations (metaByName their index), which apply outside the
	// longhand cascade.
	metas      []customWin
	metaByName map[string]int

	parsed []parsedDecl
	// subBuf is the var() substitution buffer the per-node parse
	// reuses: a parse reads the substituted tokens and keeps none.
	subBuf []token
	cx     ctx
	// layers is the compute's cascade input, kept for the define
	// fallback of var() lookups.
	layers []Layer
}

// SetPrevCounters supplies the widget layer's half of CSS's counter
// inheritance (css-lists-3 §4.4.1): prev is the previous sibling's
// counter set (nil without one), and src the set of the element
// immediately preceding the node in tree order — the previous
// sibling's last descendant — whose values the inherited counters
// take (nil reads the parent's). Compute consumes and clears both.
func (sc *Scratch) SetPrevCounters(prev, src *CounterScope) {
	sc.prevCounters, sc.srcCounters = prev, src
}

// customWin is the winning declaration of one custom property.
type customWin struct {
	d    *decl
	rank uint64
}

// chainEntry is one declaration's per-layer record on the
// revert-layer chain of one longhand.
type chainEntry struct {
	p     Prop
	d     *decl
	rk    uint64
	layer uint8
}

// parsedDecl caches one declaration's parse within a compute, so a
// shorthand winning several longhands parses once.
type parsedDecl struct {
	d  *decl
	v  Values
	ok bool
}

// collect runs the match phase: every rule of every layer that matches
// n offers its declarations; the per-longhand and per-custom-property
// winners land in sc.
func (sc *Scratch) collect(layers []Layer, n Node) {
	// Only the longhands the last compute offered hold winners: the
	// rest of the tables stay zero between computes.
	sc.offered.Each(func(p Prop) { sc.best[p], sc.win[p], sc.bestLayer[p] = 0, nil, 0 })
	sc.offeredStart.Each(func(p Prop) { sc.bestStart[p], sc.winStart[p] = 0, nil })
	sc.offered, sc.offeredStart = PropSet{}, PropSet{}
	sc.customs = sc.customs[:0]
	sc.metas = sc.metas[:0]
	sc.chain = sc.chain[:0]
	sc.revertChain = false
	sc.conts = sc.conts[:0]
	var anyContainer bool
	for _, l := range layers {
		if l.Sheet == nil {
			continue
		}
		if len(l.Sheet.containerQs) > 0 || l.Sheet.sens.CqUnits {
			anyContainer = true
		}
		sc.revertChain = sc.revertChain || l.Sheet.sens.Revert
	}
	if anyContainer {
		if cn, ok := n.(containerNode); ok {
			sc.conts = cn.StyleContainers(sc.conts)
		}
	}
	sc.hasBudget = maxHasBudget
	if sc.byName == nil {
		sc.byName = map[string]int{}
	}
	clear(sc.byName)
	if sc.metaByName == nil {
		sc.metaByName = map[string]int{}
	}
	clear(sc.metaByName)
	n.StyleTarget(&sc.self)
	for li, l := range layers {
		s := l.Sheet
		if s == nil || len(s.rules) == 0 {
			continue
		}
		sc.bucket(s.universal, s, l.Priority, li, n)
		if rules, ok := s.byElement[sc.self.Element]; ok {
			sc.bucket(rules, s, l.Priority, li, n)
		}
		if sc.self.ID != "" {
			if rules, ok := s.byID[sc.self.ID]; ok {
				sc.bucket(rules, s, l.Priority, li, n)
			}
		}
		for _, c := range sc.self.Classes {
			if rules, ok := s.byClass[c]; ok {
				sc.bucket(rules, s, l.Priority, li, n)
			}
		}
	}
	if sc.pseudo == "" {
		if b := sc.self.Inline; b != nil {
			// Inline declarations match the node itself with no specificity,
			// like a `*` rule in a widget-scoped provider; they rank as the
			// last layer at their priority.
			for i := range b.decls {
				sc.offer(&b.decls[i], rank(sc.self.InlinePriority, spec{}, inlineLayer, 63, b.decls[i].order, b.decls[i].important, noProximity), unlayered)
			}
		}
	}
}

// bucket walks one bucket, fully matching each candidate rule and
// offering its declarations — the best-per-property rank comparison
// makes visiting order irrelevant, and a rule filed once is visited at
// most once per compute.
func (sc *Scratch) bucket(rules []int32, s *Sheet, prio, layer int, n Node) {
	for _, idx := range rules {
		r := &s.rules[idx]
		if r.mq >= 0 && !mediaMatches(s.mediaQs[r.mq], &sc.media) {
			continue
		}
		if r.pseudo != sc.pseudo {
			continue
		}
		if r.cq >= 0 && !s.containerQs[r.cq].matches(sc.conts) {
			continue
		}
		if !sc.selectorMatches(&r.sel, n) {
			continue
		}
		prox := noProximity
		if r.scope != nil {
			if prox = sc.scopeProximity(r.scope, n); prox < 0 {
				continue
			}
		}
		for i := range r.block.decls {
			d := &r.block.decls[i]
			rk := rank(prio, r.spec, int(r.layer), layer, d.order, d.important, prox)
			sc.offer(d, rk, r.layer)
		}
		if r.start != nil {
			for i := range r.start.decls {
				d := &r.start.decls[i]
				rk := rank(prio, r.spec, int(r.layer), layer, d.order, d.important, prox)
				sc.offerStart(d, rk)
			}
		}
	}
}

// offer records d for every longhand it covers where its rank wins;
// with a revert-layer value in play it also files the declaration on
// the longhand's per-layer chain, the record revert-layer walks down.
func (sc *Scratch) offer(d *decl, rk uint64, layer uint8) {
	if d.meta {
		if i, ok := sc.metaByName[d.name]; ok {
			if rk >= sc.metas[i].rank {
				sc.metas[i] = customWin{d: d, rank: rk}
			}
			return
		}
		sc.metaByName[d.name] = len(sc.metas)
		sc.metas = append(sc.metas, customWin{d: d, rank: rk})
		return
	}
	if d.custom {
		if i, ok := sc.byName[d.name]; ok {
			if rk >= sc.customs[i].rank {
				sc.customs[i] = customWin{d: d, rank: rk}
			}
			return
		}
		sc.byName[d.name] = len(sc.customs)
		sc.customs = append(sc.customs, customWin{d: d, rank: rk})
		return
	}
	cov := d.covers
	sc.offered = sc.offered.Or(cov)
	offers := func(p Prop) {
		if sc.revertChain {
			sc.chain = append(sc.chain, chainEntry{p: p, d: d, rk: rk, layer: layer})
		}
		if rk >= sc.best[p] {
			sc.best[p], sc.win[p], sc.bestLayer[p] = rk, d, layer
		}
	}
	cov.Each(offers)
}

// offerStart records one @starting-style declaration for every
// longhand it covers where its rank wins, in the starting-value
// competition of its own (the entry transition's cascade).
func (sc *Scratch) offerStart(d *decl, rk uint64) {
	if d.custom || d.meta {
		return
	}
	cov := d.covers
	sc.offeredStart = sc.offeredStart.Or(cov)
	offers := func(p Prop) {
		if rk >= sc.bestStart[p] {
			sc.bestStart[p], sc.winStart[p] = rk, d
		}
	}
	cov.Each(offers)
}

// selectorMatches evaluates sel against n: the rightmost compound
// against the node itself, then the combinators right to left, with
// backtracking so `.a > .b .c` tries every `.b` ancestor.
func (sc *Scratch) selectorMatches(sel *selector, n Node) bool {
	if !sc.compoundMatches(&sel.parts[len(sel.parts)-1], &sc.self, 0, n) {
		return false
	}
	_, ok := sc.matchFrom(sel, len(sel.parts)-1, 0, n)
	return ok
}

// matchFrom checks parts[:i] given that parts[i] matched n, returning
// the node the leftmost compound matched.
func (sc *Scratch) matchFrom(sel *selector, i, d int, n Node) (Node, bool) {
	if i == 0 {
		return n, true
	}
	part := &sel.parts[i-1]
	switch sel.combs[i-1] {
	case '>':
		p := n.StyleParent()
		if p == nil || !sc.nodeMatches(part, d, p) {
			return nil, false
		}
		return sc.matchFrom(sel, i-1, d, p)
	case '+':
		p := n.StylePrev()
		if p == nil || !sc.nodeMatches(part, d, p) {
			return nil, false
		}
		return sc.matchFrom(sel, i-1, d, p)
	case '~':
		for p := n.StylePrev(); p != nil; p = p.StylePrev() {
			if sc.nodeMatches(part, d, p) {
				if root, ok := sc.matchFrom(sel, i-1, d, p); ok {
					return root, true
				}
			}
		}
		return nil, false
	default: // descendant
		for p := n.StyleParent(); p != nil; p = p.StyleParent() {
			if len(sel.upper) > 0 && sc.scopeBlocks(sel, d, p) {
				return nil, false
			}
			if sc.nodeMatches(part, d, p) {
				if root, ok := sc.matchFrom(sel, i-1, d, p); ok {
					return root, true
				}
			}
		}
		return nil, false
	}
}

// scopeProximity is the scoping proximity of n under scope
// (css-cascade-6 §6.1): the generations from n up to the nearest
// element matching a scoping root, 0 when n is the root itself, or -1
// when n sits under a scoping limit before any root — out of scope.
func (sc *Scratch) scopeProximity(scope *scopeCtx, n Node) int {
	for hops, m := 0, n; m != nil; hops, m = hops+1, m.StyleParent() {
		m.StyleTarget(&sc.bufs[0])
		for i := range scope.root {
			if sc.matchesSub(&scope.root[i], &sc.bufs[0], 0, m) {
				return hops
			}
		}
		for i := range scope.upper {
			if sc.matchesSub(&scope.upper[i], &sc.bufs[0], 0, m) {
				return -1
			}
		}
	}
	return -1
}

// scopeBlocks reports whether n crosses a @scope upper boundary: the
// scope root cannot be n nor beyond it.
func (sc *Scratch) scopeBlocks(sel *selector, d int, n Node) bool {
	for i := range sel.upper {
		n.StyleTarget(&sc.bufs[d])
		if sc.matchesSub(&sel.upper[i], &sc.bufs[d], d, n) {
			return true
		}
	}
	return false
}

// nodeMatches fills the ancestor buffer for n and tests part.
func (sc *Scratch) nodeMatches(part *compound, d int, n Node) bool {
	n.StyleTarget(&sc.bufs[d])
	return sc.compoundMatches(part, &sc.bufs[d], d, n)
}

// matchesSub evaluates a nested selector list member (:not, :is,
// :where) against a target already filled for n.
func (sc *Scratch) matchesSub(sel *selector, t *Target, d int, n Node) bool {
	if d+1 >= len(sc.bufs) {
		return false
	}
	if !sc.compoundMatches(&sel.parts[len(sel.parts)-1], t, d+1, n) {
		return false
	}
	_, ok := sc.matchFrom(sel, len(sel.parts)-1, d+1, n)
	return ok
}

// compoundMatches tests one compound against a filled target.
func (sc *Scratch) compoundMatches(c *compound, t *Target, d int, n Node) bool {
	if c.element != "" && c.element != t.Element {
		return false
	}
	if c.id != "" && c.id != t.ID {
		return false
	}
	for _, cl := range c.classes {
		if !slices.Contains(t.Classes, cl) {
			return false
		}
	}
	if t.State&c.state != c.state {
		return false
	}
	if len(c.attrs) > 0 && !attrsMatch(c.attrs, t) {
		return false
	}
	if c.structs != 0 || len(c.nths) > 0 {
		if !sc.structsMatch(c, t, d, n) {
			return false
		}
	}
	for i := range c.not {
		for j := range c.not[i].sels {
			if sc.matchesSub(&c.not[i].sels[j], t, d, n) {
				return false
			}
		}
	}
	for i := range c.isLists {
		ok := false
		for j := range c.isLists[i].sels {
			if sc.matchesSub(&c.isLists[i].sels[j], t, d, n) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for i := range c.has {
		if !sc.anyHasMatches(c.has[i], d, n) {
			return false
		}
	}
	return true
}

// anyHasMatches reports whether one argument of a :has() list matches.
func (sc *Scratch) anyHasMatches(args []hasArg, d int, n Node) bool {
	for i := range args {
		if sc.hasMatches(&args[i], d, n) {
			return true
		}
	}
	return false
}

// attrsMatch tests the compound's attribute selectors.
func attrsMatch(attrs []attrTest, t *Target) bool {
	for _, a := range attrs {
		v, ok := t.attrValue(a.name)
		if !ok {
			return false
		}
		if a.op == 0 {
			continue
		}
		want, got := a.val, v
		if a.ci {
			want, got = strings.ToLower(want), strings.ToLower(got)
		}
		switch a.op {
		case '=':
			if got != want {
				return false
			}
		case '~':
			if !slices.Contains(strings.Fields(got), want) || want == "" {
				return false
			}
		case '|':
			if got != want && !strings.HasPrefix(got, want+"-") {
				return false
			}
		case '^':
			if want == "" || !strings.HasPrefix(got, want) {
				return false
			}
		case '$':
			if want == "" || !strings.HasSuffix(got, want) {
				return false
			}
		case '*':
			if want == "" || !strings.Contains(got, want) {
				return false
			}
		}
	}
	return true
}

// structsMatch tests the structural pseudo-classes and an+b tests,
// including the `of S` sibling filter of :nth-child(an+b of S).
func (sc *Scratch) structsMatch(c *compound, t *Target, d int, n Node) bool {
	st := c.structs
	if st&structRoot != 0 && n.StyleParent() != nil {
		return false
	}
	if st&structEnabled != 0 && t.State&Disabled != 0 {
		return false
	}
	if st&structReadWrite != 0 && t.State&ReadOnly != 0 {
		return false
	}
	if st&structEmpty != 0 {
		if t.HasText || len(n.StyleChildren()) != 0 {
			return false
		}
	}
	if st&(structFirst|structLast|structOnly) != 0 || hasPlainNth(c.nths) {
		idx, cnt := n.StylePosition()
		if !positionOk(st, idx, cnt) || !nthsMatch(c.nths, idx, cnt, false) {
			return false
		}
	}
	if st&(structFirstOf|structLastOf|structOnlyOf) != 0 || hasTypeNth(c.nths) {
		idx, cnt := n.StyleTypedPosition(t.Element)
		if !positionOk(st>>5, idx, cnt) || !nthsMatch(c.nths, idx, cnt, true) {
			return false
		}
	}
	return sc.nthsOfMatch(c.nths, d, n)
}

// nthsOfMatch evaluates the `of S`-scoped an+b tests: the index
// counts only the visible siblings matching the of-list, from the
// start (or the end for the -last forms).
func (sc *Scratch) nthsOfMatch(nths []nth, d int, n Node) bool {
	scoped := false
	for i := range nths {
		if nths[i].of == nil {
			continue
		}
		scoped = true
		break
	}
	if !scoped {
		return true
	}
	p := n.StyleParent()
	if p == nil {
		return false
	}
	for i := range nths {
		x := &nths[i]
		if x.of == nil {
			continue
		}
		count, mine := 0, 0
		for _, sib := range p.StyleChildren() {
			if sc.sibMatchesOf(x.of, d, sib) {
				count++
				if sameNode(sib, n) {
					mine = count
				}
			}
		}
		if mine == 0 || !x.matches(mine, count) {
			return false
		}
	}
	return true
}

// sibMatchesOf tests one sibling against an of-list, filling the
// depth's sibling buffer (one level deeper than the compound's, so a
// nested of-list cannot clobber it).
func (sc *Scratch) sibMatchesOf(sels []selector, d int, sib Node) bool {
	if d+1 >= len(sc.bufs) {
		return false
	}
	sib.StyleTarget(&sc.sibBufs[d])
	for i := range sels {
		if sc.matchesSub(&sels[i], &sc.sibBufs[d], d+1, sib) {
			return true
		}
	}
	return false
}

func hasPlainNth(nths []nth) bool {
	for _, x := range nths {
		if !x.ofType && x.of == nil {
			return true
		}
	}
	return false
}

func hasTypeNth(nths []nth) bool {
	for _, x := range nths {
		if x.ofType {
			return true
		}
	}
	return false
}

// positionOk tests the positional bits of st (already shifted to the
// of-type group when needed) against a 1-based index and count.
func positionOk(st structural, idx, cnt int) bool {
	if st&structFirst != 0 && idx != 1 {
		return false
	}
	if st&structLast != 0 && idx != cnt {
		return false
	}
	if st&structOnly != 0 && cnt != 1 {
		return false
	}
	return true
}

func nthsMatch(nths []nth, idx, cnt int, ofType bool) bool {
	for _, x := range nths {
		if x.ofType != ofType || x.of != nil {
			continue
		}
		if !x.matches(idx, cnt) {
			return false
		}
	}
	return true
}

// hasMatches evaluates one :has() argument against its element n.
func (sc *Scratch) hasMatches(ha *hasArg, d int, n Node) bool {
	if d+1 >= len(sc.bufs) {
		return false
	}
	sel := &ha.sel
	visit := func(m Node) bool {
		if sc.hasBudget <= 0 {
			return false
		}
		sc.hasBudget--
		m.StyleTarget(&sc.bufs[d+1])
		if !sc.compoundMatches(&sel.parts[len(sel.parts)-1], &sc.bufs[d+1], d+1, m) {
			return false
		}
		root, ok := sc.matchFrom(sel, len(sel.parts)-1, d+1, m)
		return ok && sc.relates(ha.comb, root, n)
	}
	var walk func(kids []Node) bool
	walk = func(kids []Node) bool {
		for _, k := range kids {
			if visit(k) {
				return true
			}
			if sc.hasBudget <= 0 {
				return false
			}
			if walk(k.StyleChildren()) {
				return true
			}
		}
		return false
	}
	switch ha.comb {
	case '+', '~':
		for sib := nextSibling(n); sib != nil; sib = nextSibling(sib) {
			if visit(sib) || (sc.hasBudget > 0 && walk(sib.StyleChildren())) {
				return true
			}
		}
		return false
	default:
		return walk(n.StyleChildren())
	}
}

// relates reports whether anchor stands in comb's relation to scope,
// the :has element.
func (sc *Scratch) relates(comb byte, anchor, scope Node) bool {
	switch comb {
	case '>':
		return sameNode(anchor.StyleParent(), scope)
	case '+':
		return sameNode(anchor.StylePrev(), scope)
	case '~':
		for p := anchor.StylePrev(); p != nil; p = p.StylePrev() {
			if sameNode(p, scope) {
				return true
			}
		}
		return false
	default:
		for p := anchor.StyleParent(); p != nil; p = p.StyleParent() {
			if sameNode(p, scope) {
				return true
			}
		}
		return false
	}
}

// nextSibling returns the following visible sibling, or nil.
func nextSibling(n Node) Node {
	p := n.StyleParent()
	if p == nil {
		return nil
	}
	kids := p.StyleChildren()
	for i, k := range kids {
		if sameNode(k, n) && i+1 < len(kids) {
			return kids[i+1]
		}
	}
	return nil
}
