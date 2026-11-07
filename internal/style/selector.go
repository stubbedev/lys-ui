package style

import (
	"math/bits"
	"strconv"
	"strings"
)

// State is a widget's interactive state the selector subset matches
// with: one bit per state pseudo-class.
type State uint16

// The state bits, in specificity-neutral order (each counts as one
// class-level step in a selector's rank).
const (
	Hover State = 1 << iota
	Focus
	Active
	Disabled
	// FocusVisible is focus that arrived by keyboard traversal.
	FocusVisible
	// FocusWithin is set on the focused widget and all its ancestors.
	FocusWithin
	// Checked is a toggled-on control, or a menu button whose popup is
	// open (GTK's :checked).
	Checked
	// Selected is a selected row or item.
	Selected
	// Indeterminate is a check or progress in its mixed state.
	Indeterminate
	// Backdrop is a widget whose window is not the active one.
	Backdrop
	// ReadOnly is a text field the user cannot edit (:read-only).
	ReadOnly
	// PlaceholderShown is an empty field showing its placeholder
	// (:placeholder-shown).
	PlaceholderShown
	// Targeted is the widget the application marks as the navigation
	// target (:target): the widget tree has no URL fragment, so the
	// app names the target the way a fragment would.
	Targeted
)

// stateByName maps a pseudo-class name to its bit.
func stateByName(name string) (State, bool) {
	switch name {
	case "hover":
		return Hover, true
	case "focus":
		return Focus, true
	case "active":
		return Active, true
	case "disabled":
		return Disabled, true
	case "focus-visible":
		return FocusVisible, true
	case "focus-within":
		return FocusWithin, true
	case "checked":
		return Checked, true
	case "selected":
		return Selected, true
	case "indeterminate":
		return Indeterminate, true
	case "backdrop":
		return Backdrop, true
	case "read-only":
		return ReadOnly, true
	case "placeholder-shown":
		return PlaceholderShown, true
	case "target":
		return Targeted, true
	default:
		return 0, false
	}
}

// structural pseudo-classes: position among visible siblings, plus the
// negated-state forms :enabled and :read-write.
type structural uint16

const (
	structFirst structural = 1 << iota
	structLast
	structOnly
	structRoot
	structEmpty
	structFirstOf
	structLastOf
	structOnlyOf
	// structEnabled is :enabled, the negation of the Disabled state.
	structEnabled
	// structReadWrite is :read-write, the negation of ReadOnly.
	structReadWrite
)

// nth is one :nth-child(an+b) (and -last, -of-type) test, with the
// Level 4 `of S` selector-scoped sibling filter.
type nth struct {
	a, b int
	last bool
	// ofType counts only siblings of the subject's element name.
	ofType bool
	// of holds the `of S` selector list: an+b counts only the
	// siblings matching it, nil for the plain form.
	of []selector
}

// matches reports whether the 1-based position satisfies an+b for some
// n >= 0.
func (x nth) matches(index, count int) bool {
	pos := index
	if x.last {
		pos = count - index + 1
	}
	if x.a == 0 {
		return pos == x.b
	}
	d := pos - x.b
	return d%x.a == 0 && d/x.a >= 0
}

// attrTest is one attribute selector: presence, or an operator against
// the attribute's value, with the optional case-insensitivity flag.
type attrTest struct {
	name string
	// op is 0 for a bare presence test, else one of = ~ | ^ $ *.
	op  byte
	val string
	ci  bool
}

// isList is one :is()/:where() argument list.
type isList struct {
	sels []selector
	// zero marks :where(), whose specificity is always 0.
	zero bool
}

// hasArg is one :has() relative selector argument: the combinator
// that anchors it to the :has element (' ' descendant, '>' child,
// '+' next sibling, '~' following sibling) and the selector list to
// its right.
type hasArg struct {
	comb byte
	sel  selector
}

// compound is one compound selector: an optional element name or `*`,
// then classes, id, attribute tests, state and structural
// pseudo-classes, :not() negations, :is()/:where() lists, and :has()
// relational arguments.
type compound struct {
	element string
	id      string
	classes []string
	attrs   []attrTest
	state   State
	structs structural
	nths    []nth
	// not holds the :not() argument lists, one per :not(): the
	// compound matches only when no member of any of them does.
	not []isList
	// isLists holds :is()/:where() argument lists, all of which must
	// match.
	isLists []isList
	// has holds the :has() argument lists, one per :has(): each needs
	// one of its relative arguments to match.
	has [][]hasArg
	// scopes counts the compound's :scope pseudo-classes inside an
	// @scope rule, each a pseudo-class's specificity on top of the
	// zero-specificity root test it adds to isLists.
	scopes int
	// pseudo names a ::before/::after pseudo-element on this compound,
	// the subject of the rule's declarations.
	pseudo string
	// partElem names a desugared part pseudo-element (::placeholder,
	// ::selection, ::marker): the selector grows a descendant compound
	// of that element name, so the existing part machinery styles it.
	partElem string
}

// spec is a selector's specificity: ids, classes (with pseudo-classes,
// attribute tests, and functional pseudo-class arguments), and
// elements.
type spec struct{ ids, cls, els int }

func (c *compound) spec() spec {
	s := spec{cls: len(c.classes) + len(c.attrs) + bits.OnesCount32(uint32(c.state)) +
		bits.OnesCount32(uint32(c.structs)) + len(c.nths) + c.scopes}
	if c.id != "" {
		s.ids++
	}
	if c.element != "" {
		s.els++
	}
	// Each :not(), :is() and :has() counts its most specific argument,
	// and :nth-child(of S) its most specific S on top of the
	// pseudo-class; :where() counts nothing (Selectors Level 4 §17).
	for i := range c.not {
		s = s.plus(c.not[i].best())
	}
	for i := range c.isLists {
		if !c.isLists[i].zero {
			s = s.plus(c.isLists[i].best())
		}
	}
	for i := range c.has {
		var best spec
		for j := range c.has[i] {
			if ns := c.has[i][j].sel.spec(); ns.greater(best) {
				best = ns
			}
		}
		s = s.plus(best)
	}
	for i := range c.nths {
		s = s.plus(isList{sels: c.nths[i].of}.best())
	}
	return s
}

// best is the specificity of the list's most specific member.
func (l isList) best() spec {
	var best spec
	for i := range l.sels {
		if ns := l.sels[i].spec(); ns.greater(best) {
			best = ns
		}
	}
	return best
}

// plus adds two specificities.
func (s spec) plus(o spec) spec {
	return spec{ids: s.ids + o.ids, cls: s.cls + o.cls, els: s.els + o.els}
}

func (s spec) greater(o spec) bool {
	if s.ids != o.ids {
		return s.ids > o.ids
	}
	if s.cls != o.cls {
		return s.cls > o.cls
	}
	return s.els > o.els
}

// selector is a compound sequence with combinators, left to right.
type selector struct {
	parts []compound
	// combs holds len(parts)-1 entries: ' ' descendant, '>' child, '+'
	// adjacent sibling, '~' general sibling.
	combs []byte
	// upper holds a @scope rule's upper bound selectors: a
	// descendant walk crossing a node matching one leaves the scope,
	// and the scope root cannot lie beyond it. nil outside @scope.
	upper []selector
}

func (s *selector) spec() spec {
	var out spec
	for i := range s.parts {
		p := s.parts[i].spec()
		out.ids += p.ids
		out.cls += p.cls
		out.els += p.els
	}
	return out
}

// selParser parses selectors from a rule prelude.
type selParser struct {
	src   string
	ts    []token
	pos   int
	depth int
	// scope is the enclosing @scope rule's root list, nil outside
	// one; usedScope reports a :scope anywhere in the selector, shared
	// with the nested argument parsers.
	scope     []selector
	usedScope *bool
}

func (p *selParser) peek() (token, bool) {
	if p.pos < len(p.ts) {
		return p.ts[p.pos], true
	}
	return token{}, false
}

func (p *selParser) skipWS() {
	for p.pos < len(p.ts) && p.ts[p.pos].kind == tkWS {
		p.pos++
	}
}

// parseSelectorList parses a comma-separated selector list; any invalid
// member invalidates the whole list, per CSS.
func parseSelectorList(src string, ts []token) ([]selector, bool) {
	return parseSelectorListAt(src, ts, 0)
}

// parseScopedList parses a style rule's selector list inside an
// @scope with root list root (css-cascade-6 §2.5.2): `:scope` matches
// the scoping root, and a selector without one is relative to it —
// prefixed with a zero-specificity root test and a descendant
// combinator, `:where(:scope) sel`.
func parseScopedList(src string, ts []token, root []selector) ([]selector, bool) {
	var out []selector
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) == 0 {
			return nil, false
		}
		used := false
		p := selParser{src: src, ts: part, scope: root, usedScope: &used}
		sel, ok := p.selector()
		if !ok {
			return nil, false
		}
		if !used {
			sel.parts = append([]compound{{isLists: []isList{{sels: root, zero: true}}}}, sel.parts...)
			sel.combs = append([]byte{' '}, sel.combs...)
		}
		out = append(out, sel)
	}
	return out, len(out) > 0
}

// maxPseudoDepth bounds functional pseudo-class nesting (:is inside
// :has inside :is, ...): deeper arguments fail to parse, keeping
// pathological stylesheets from recursing.
const maxPseudoDepth = 8

func parseSelectorListAt(src string, ts []token, depth int) ([]selector, bool) {
	var out []selector
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) == 0 {
			return nil, false
		}
		p := selParser{src: src, ts: part, depth: depth}
		sel, ok := p.selector()
		if !ok {
			return nil, false
		}
		out = append(out, sel)
	}
	return out, len(out) > 0
}

// sub is a parser for a functional pseudo-class's argument tokens one
// nesting level down, under the same @scope.
func (p *selParser) sub(ts []token) selParser {
	return selParser{src: p.src, ts: ts, depth: p.depth + 1, scope: p.scope, usedScope: p.usedScope}
}

// list parses ts as a selector list one level down; any invalid
// member invalidates it.
func (p *selParser) list(ts []token) ([]selector, bool) {
	var out []selector
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) == 0 {
			return nil, false
		}
		sp := p.sub(part)
		sel, ok := sp.selector()
		if !ok {
			return nil, false
		}
		out = append(out, sel)
	}
	return out, len(out) > 0
}

// forgivingList parses ts as a <forgiving-selector-list> one level
// down (:is() and :where()): an invalid member drops alone, and an
// empty result matches nothing.
func (p *selParser) forgivingList(ts []token) []selector {
	var out []selector
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) == 0 {
			continue
		}
		sp := p.sub(part)
		if sel, ok := sp.selector(); ok {
			out = append(out, sel)
		}
	}
	return out
}

// selector parses compounds joined by combinators.
func (p *selParser) selector() (selector, bool) {
	var sel selector
	for {
		c, ok := p.compound()
		if !ok {
			return selector{}, false
		}
		if c.partElem != "" {
			// A desugared part pseudo-element (::placeholder, ...)
			// ends the selector: the part is the subject, a descendant
			// compound of that element name. A compound that was only the
			// pseudo-element contributes no part of its own.
			if compoundHasSimple(&c) {
				sel.parts = append(sel.parts, c)
				sel.combs = append(sel.combs, ' ')
			}
			sel.parts = append(sel.parts, compound{element: c.partElem})
			if p.pos < len(p.ts) {
				return selector{}, false
			}
			return sel, true
		}
		sel.parts = append(sel.parts, c)
		if c.pseudo != "" && p.pos < len(p.ts) {
			return selector{}, false
		}
		sawWS := p.pos < len(p.ts) && p.ts[p.pos].kind == tkWS
		p.skipWS()
		t, more := p.peek()
		if !more {
			return sel, true
		}
		comb := byte(' ')
		switch {
		case t.is('>'), t.is('+'), t.is('~'):
			comb = t.s[0]
			p.pos++
			p.skipWS()
		case !sawWS:
			return selector{}, false
		}
		sel.combs = append(sel.combs, comb)
	}
}

// compoundHasSimple reports whether the compound carries a simple
// selector beyond a part pseudo-element.
func compoundHasSimple(c *compound) bool {
	return c.element != "" || c.id != "" || len(c.classes) > 0 || len(c.attrs) > 0 ||
		c.state != 0 || c.structs != 0 || len(c.nths) > 0 || len(c.not) > 0 ||
		len(c.isLists) > 0 || len(c.has) > 0 || c.scopes > 0
}

// compound parses one compound selector. At least one simple selector is
// required.
func (p *selParser) compound() (compound, bool) {
	var c compound
	seen := false
	for p.pos < len(p.ts) {
		t := p.ts[p.pos]
		switch {
		case t.is('*'):
			if seen {
				return compound{}, false
			}
			p.pos++
		case t.kind == tkIdent:
			if seen {
				return compound{}, false
			}
			c.element = strings.ToLower(t.s)
			p.pos++
		case t.is('.'):
			p.pos++
			n, ok := p.peek()
			if !ok || n.kind != tkIdent {
				return compound{}, false
			}
			c.classes = append(c.classes, n.s)
			p.pos++
		case t.is('&') && p.scope != nil:
			// Inside @scope, `&` is the scoping root with its
			// selector's specificity, :is(<scope-start>)
			// (css-cascade-6 §2.5.2).
			c.isLists = append(c.isLists, isList{sels: p.scope})
			*p.usedScope = true
			p.pos++
		case t.kind == tkHash:
			if c.id != "" {
				return compound{}, false
			}
			c.id = t.s
			p.pos++
		case t.kind == tkLBrack:
			a, ok := p.attr()
			if !ok {
				return compound{}, false
			}
			c.attrs = append(c.attrs, a)
		case t.kind == tkColon:
			p.pos++
			if !p.pseudo(&c) {
				return compound{}, false
			}
			if c.pseudo != "" || c.partElem != "" {
				return c, true
			}
		default:
			return c, seen
		}
		seen = true
	}
	return c, seen
}

// attr parses one attribute selector after a '['.
func (p *selParser) attr() (attrTest, bool) {
	p.pos++
	t, ok := p.peek()
	if !ok || t.kind != tkIdent {
		return attrTest{}, false
	}
	a := attrTest{name: t.s}
	p.pos++
	p.skipWS()
	t, ok = p.peek()
	if !ok {
		return attrTest{}, false
	}
	if t.kind == tkRBrack {
		p.pos++
		return a, true
	}
	if t.kind != tkDelim || len(t.s) != 1 {
		return attrTest{}, false
	}
	switch t.s[0] {
	case '=':
		a.op = '='
	case '~', '|', '^', '$', '*':
		p.pos++
		n, more := p.peek()
		if !more || !n.is('=') {
			return attrTest{}, false
		}
		a.op = t.s[0]
	default:
		return attrTest{}, false
	}
	p.pos++
	p.skipWS()
	t, ok = p.peek()
	if !ok || (t.kind != tkString && t.kind != tkIdent) {
		return attrTest{}, false
	}
	a.val = t.s
	p.pos++
	p.skipWS()
	if t, ok := p.peek(); ok && t.kind == tkIdent {
		switch strings.ToLower(t.s) {
		case "i":
			a.ci = true
			p.pos++
			p.skipWS()
		case "s":
			p.pos++
			p.skipWS()
		default:
			return attrTest{}, false
		}
	}
	if t, ok := p.peek(); !ok || t.kind != tkRBrack {
		return attrTest{}, false
	}
	p.pos++
	return a, true
}

// pseudo parses the pseudo-class after a ':' (or, after a second ':',
// a pseudo-element).
func (p *selParser) pseudo(c *compound) bool {
	t, ok := p.peek()
	if !ok {
		return false
	}
	if t.kind == tkColon {
		p.pos++
		return p.pseudoElem(c)
	}
	switch t.kind {
	case tkIdent:
		p.pos++
		name := strings.ToLower(t.s)
		if st, ok := stateByName(name); ok {
			c.state |= st
			return true
		}
		switch name {
		case "first-child":
			c.structs |= structFirst
		case "last-child":
			c.structs |= structLast
		case "only-child":
			c.structs |= structOnly
		case "root":
			c.structs |= structRoot
		case "empty":
			c.structs |= structEmpty
		case "first-of-type":
			c.structs |= structFirstOf
		case "last-of-type":
			c.structs |= structLastOf
		case "only-of-type":
			c.structs |= structOnlyOf
		case "enabled":
			c.structs |= structEnabled
		case "read-write":
			c.structs |= structReadWrite
		case "scope":
			// Inside @scope, the scoping root; elsewhere :root
			// (css-cascade-6 §2.5.2, selectors-4 §14.5).
			if p.scope == nil {
				c.structs |= structRoot
				return true
			}
			c.isLists = append(c.isLists, isList{sels: p.scope, zero: true})
			c.scopes++
			*p.usedScope = true
		default:
			return false
		}
		return true
	case tkFunc:
		end := blockEnd(p.ts, p.pos)
		args := trimWS(funcArgs(p.ts[p.pos:end]))
		p.pos = end
		switch t.s {
		case "not":
			sels, ok := p.subList(args)
			if !ok {
				return false
			}
			c.not = append(c.not, isList{sels: sels})
			return true
		case "is", "where", "-webkit-any", "-moz-any":
			if p.depth >= maxPseudoDepth {
				return false
			}
			c.isLists = append(c.isLists, isList{sels: p.forgivingList(args), zero: t.s == "where"})
			return true
		case "has":
			args, ok := p.hasArgs(args)
			if !ok {
				return false
			}
			c.has = append(c.has, args)
			return true
		case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
			micro, of, ok := splitNthOf(args)
			if !ok {
				return false
			}
			x, ok := parseNth(rawText(p.src, micro))
			if !ok {
				return false
			}
			if of != nil {
				if p.depth >= maxPseudoDepth {
					return false
				}
				sels, okl := p.list(of)
				if !okl {
					return false
				}
				x.of = sels
			}
			x.last = t.s == "nth-last-child" || t.s == "nth-last-of-type"
			x.ofType = t.s == "nth-of-type" || t.s == "nth-last-of-type"
			c.nths = append(c.nths, x)
			return true
		}
	}
	return false
}

// pseudoElem parses a pseudo-element after '::'.
func (p *selParser) pseudoElem(c *compound) bool {
	t, ok := p.peek()
	if !ok || t.kind != tkIdent {
		return false
	}
	p.pos++
	switch strings.ToLower(t.s) {
	case "before":
		c.pseudo = "before"
	case "after":
		c.pseudo = "after"
	case "first-letter":
		c.pseudo = "first-letter"
	case "first-line":
		// Parsed, never matched: line fragmentation happens inside the
		// text shaper at paint time, and a widget's text is one shaped
		// run with no per-line box the cascade could address.
		c.pseudo = "first-line"
	case "placeholder", "selection", "marker":
		c.partElem = strings.ToLower(t.s)
	default:
		return false
	}
	return true
}

// subList parses a functional pseudo-class's selector-list argument at
// one deeper nesting level.
func (p *selParser) subList(args []token) ([]selector, bool) {
	if p.depth >= maxPseudoDepth {
		return nil, false
	}
	return p.list(args)
}

// hasArgs parses a :has() argument list: each member may lead with a
// relative combinator (default descendant).
func (p *selParser) hasArgs(args []token) ([]hasArg, bool) {
	if p.depth >= maxPseudoDepth {
		return nil, false
	}
	var out []hasArg
	for _, part := range splitTop(args, tkComma) {
		part = trimWS(part)
		comb := byte(' ')
		if len(part) > 0 && (part[0].is('>') || part[0].is('+') || part[0].is('~')) {
			comb = part[0].s[0]
			part = trimWS(part[1:])
		}
		if len(part) == 0 {
			return nil, false
		}
		sels, ok := p.list(part)
		if !ok || len(sels) != 1 {
			return nil, false
		}
		out = append(out, hasArg{comb: comb, sel: sels[0]})
	}
	return out, len(out) > 0
}

// splitNthOf splits a :nth-child argument at its top-level `of`:
// the an+b microsyntax's tokens and the `of S` selector's tokens (nil
// when absent). ok is false when an `of` clause is empty.
func splitNthOf(ts []token) (micro, of []token, ok bool) {
	for i := range ts {
		if !ts[i].ident("of") {
			continue
		}
		// Only a top-level `of` splits; one inside a selector's
		// functional pseudo-class does not.
		nesting := 0
		for _, t := range ts[:i] {
			switch t.kind {
			case tkFunc, tkLParen, tkLBrack:
				nesting++
			case tkRParen, tkRBrack, tkRBrace:
				nesting--
			}
		}
		if nesting != 0 {
			continue
		}
		of = trimWS(ts[i+1:])
		if len(of) == 0 {
			return nil, nil, false
		}
		return trimWS(ts[:i]), of, true
	}
	return ts, nil, true
}

// parseNth parses the an+b microsyntax: odd, even, an, b, an+b, an-b.
func parseNth(s string) (nth, bool) {
	s = strings.ToLower(strings.ReplaceAll(s, " ", ""))
	switch s {
	case "odd":
		return nth{a: 2, b: 1}, true
	case "even":
		return nth{a: 2, b: 0}, true
	}
	i := strings.IndexByte(s, 'n')
	if i < 0 {
		b, err := strconv.Atoi(s)
		return nth{b: b}, err == nil
	}
	var a int
	switch head := s[:i]; head {
	case "", "+":
		a = 1
	case "-":
		a = -1
	default:
		v, err := strconv.Atoi(head)
		if err != nil {
			return nth{}, false
		}
		a = v
	}
	var b int
	if tail := s[i+1:]; tail != "" {
		v, err := strconv.Atoi(tail)
		if err != nil {
			return nth{}, false
		}
		b = v
	}
	return nth{a: a, b: b}, true
}
