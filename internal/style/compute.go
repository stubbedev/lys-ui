package style

import (
	"slices"
	"strconv"
	"strings"

	"github.com/stubbedev/lys-ui/render"
)

// Env is the unit environment a compute resolves against.
type Env struct {
	// Rem is the root font size in logical pixels: what 1rem is.
	Rem float64
	// FontPx is the font size a node computes when nothing in its
	// ancestry sets one (CSS's `medium`); zero means Rem.
	FontPx float64
	// Font is the node's face metrics the ch, ex, cap, ic and lh units
	// resolve against, as ratios of the font size (render.Font.Metrics).
	// The zero value falls back to the CSS default ratios.
	Font FontMetrics
	// RootLineHeight is what 1rlh is; zero falls back to the natural
	// line height of Font at Rem.
	RootLineHeight float64
	// Media is what @media conditions evaluate against (the viewport
	// and appearance state); the zero value is a 0×0 viewport at 96dpi.
	// Its Width and Height also resolve the vw/vh unit family and its
	// Dark flag picks light-dark()'s half.
	Media Media
}

// maxVarDepth bounds var() expansion: a chain deeper than this is
// treated as a cycle.
const maxVarDepth = 32

// Compute runs the cascade for n: match every layer (and n's inline
// block), resolve the node's custom properties over parent's, substitute
// var() and parse the winning declarations, then inherit the unset
// inherited longhands from parent. parent is nil at the root. old, when
// non-nil, is the node's previous result: an unchanged custom-property
// environment keeps its old pointer so equality stays cheap.
func Compute(layers []Layer, n Node, parent, old *Values, env Env, sc *Scratch, v *Values) {
	sc.pseudo = ""
	sc.computeAll(layers, n, parent, old, env, v)
}

// ComputePseudo is Compute for one pseudo-element (::before, ::after)
// of n: only the rules addressing that pseudo-element cascade, and the
// result inherits from parent, the element's own computed style. A
// content declaration's quote keywords move the element's inherited
// quote depth, so descendants' open-quote continues the count.
func ComputePseudo(layers []Layer, n Node, parent *Values, env Env, sc *Scratch, v *Values, name string) {
	sc.pseudo = name
	sc.computeAll(layers, n, parent, nil, env, v)
	if parent != nil && v.HasContent {
		parent.QuoteDepth = v.QuoteDepth
	}
}

func (sc *Scratch) computeAll(layers []Layer, n Node, parent, old *Values, env Env, v *Values) {
	*v = Values{}
	if env.Rem <= 0 {
		env.Rem = 16
	}
	if env.FontPx <= 0 {
		env.FontPx = env.Rem
	}
	sc.media = env.Media
	if sc.media.DPI <= 0 {
		sc.media.DPI = 96
	}
	sc.collect(layers, n)
	sc.layers = layers

	var pv *Values
	if parent != nil {
		pv = parent
	} else {
		pv = &Values{}
	}
	v.Vars = sc.resolveVars(pv.Vars)
	if old != nil && old.Vars != v.Vars && v.Vars.equalOwn(old.Vars) {
		v.Vars = old.Vars
	}

	// The context lives in the scratch: the parsers are function values,
	// so a stack ctx would escape and allocate per compute.
	sc.cx = ctx{
		rem: env.Rem, em: env.FontPx, weight: 400, self: &sc.self,
		vpW: env.Media.Width, vpH: env.Media.Height,
		font: env.Font, rlh: env.RootLineHeight, dark: env.Media.Dark, dpi: env.Media.DPI,
		conts: sc.conts,
	}
	if sc.cx.rlh == 0 {
		sc.cx.rlh = fontRatio(env.Font.LineHeight, fallbackLH) * env.Rem
	}
	cx := &sc.cx
	if pv.Has(PropFontSize) {
		cx.em = pv.FontSize
	}
	if pv.Has(PropFontWeight) {
		cx.weight = pv.FontWeight
	}
	if pv.Has(PropColor) {
		cx.color = pv.Color
	}
	if pv.Has(PropLineHeight) {
		cx.lh = pv.LineHeight
	}
	sc.parsed = sc.parsed[:0]

	// font-size first: em lengths everywhere else resolve against it.
	// color next: currentColor everywhere else resolves against it.
	// line-height third: the lh unit reads it.
	var blocked PropSet
	blocked = blocked.Or(sc.computeProp(PropFontSize, v, pv, cx))
	if v.Has(PropFontSize) {
		cx.em = v.FontSize
	}
	blocked = blocked.Or(sc.computeProp(PropColor, v, pv, cx))
	if v.Has(PropColor) {
		cx.color = v.Color
	}
	blocked = blocked.Or(sc.computeProp(PropLineHeight, v, pv, cx))
	if v.Has(PropLineHeight) {
		cx.lh = v.LineHeight
	} else if pv.LineHeightFactor > 0 {
		// An inherited number resolves against this element's
		// font-size, for the lh unit as for the property.
		cx.lh = pv.LineHeightFactor * cx.emPx()
	}
	if cx.lh == 0 {
		cx.lh = fontRatio(cx.font.LineHeight, fallbackLH) * cx.em
	}
	// The rest in Prop order, the declared ones alone: computeProp
	// leaves a longhand without a winning declaration untouched.
	sc.offered.AndNot(setOf(PropFontSize, PropColor, PropLineHeight)).Each(func(p Prop) {
		blocked = blocked.Or(sc.computeProp(p, v, pv, cx))
	})

	// Inheritance: unset inherited longhands take the parent's value,
	// unless an explicit `initial` blocked it. The quote depth
	// inherits too: descendants' open-quote continues the count.
	v.Own = v.Set
	inherit := inheritedProps.AndNot(v.Set).AndNot(blocked).And(pv.Set)
	inherit.Each(func(p Prop) { copyProp(v, pv, p) })
	if inherit.Has(PropLineHeight) && v.LineHeightFactor > 0 {
		v.LineHeight = cx.lh
	}
	v.Set = v.Set.Or(inherit)
	v.QuoteDepth = pv.QuoteDepth
	sc.applyMeta(v, pv, cx, n)
	sc.prevCounters, sc.srcCounters = nil, nil
	v.Animation = v.Anim.animations(layers)
	sc.computeStarting(v, cx)
}

// computeStarting parses the winning @starting-style declarations
// into v.Starting, nil when no matched rule carries any: the entry
// transition's start values. Only the properties the blocks declare
// are read; the rest of the struct is meaningless.
func (sc *Scratch) computeStarting(v *Values, cx *ctx) {
	if sc.offeredStart.Empty() {
		return
	}
	st := initialValues()
	sc.offeredStart.Each(func(p Prop) {
		if src, ok := sc.parse(sc.winStart[p], v.Vars, cx); ok {
			copyProp(&st, src, p)
			st.Set = st.Set.With(p)
		}
	})
	v.Starting = &st
}

// computeProp computes one longhand from its winning declaration. It
// returns p's bit when an explicit `initial` must keep the inherited
// value out.
func (sc *Scratch) computeProp(p Prop, v, parent *Values, cx *ctx) PropSet {
	d := sc.win[p]
	if d == nil {
		return PropSet{}
	}
	wide := d.wide
	if wide == wideRevertLayer {
		if lo := sc.revertBelow(p, int(sc.bestLayer[p])); lo != nil && lo.wide != wideRevertLayer {
			if src, ok := sc.parse(lo, v.Vars, cx); ok {
				copyProp(v, src, p)
				v.Set = v.Set.With(p)
				return PropSet{}
			}
		}
		wide = wideUnset
	}
	if wide == wideRevert {
		if lo := sc.revertOrigin(p, sc.best[p]); lo != nil {
			if src, ok := sc.parse(lo, v.Vars, cx); ok {
				copyProp(v, src, p)
				v.Set = v.Set.With(p)
				return PropSet{}
			}
		}
		wide = wideUnset
	}
	if wide == wideNone {
		src, ok := sc.parse(d, v.Vars, cx)
		if ok {
			copyProp(v, src, p)
			v.Set = v.Set.With(p)
			return PropSet{}
		}
		// Invalid at computed-value time: the declaration acts as unset.
		wide = wideUnset
	}
	if wide == wideUnset {
		if inheritedProps.Has(p) {
			wide = wideInherit
		} else {
			wide = wideInitial
		}
	}
	switch wide {
	case wideInherit:
		if parent.Has(p) {
			copyProp(v, parent, p)
			v.Set = v.Set.With(p)
		} else if !inheritedProps.Has(p) {
			sc.setInitial(p, v, cx)
		}
	case wideInitial:
		// An inherited property's initial value is the theme's (the
		// consumer's default), so it stays unset; the rest take the CSS
		// initial value, which is what strips theme styling under
		// `all: unset`.
		if !inheritedProps.Has(p) {
			sc.setInitial(p, v, cx)
			return PropSet{}
		}
		return setOf(p)
	}
	return PropSet{}
}

// setInitial writes p's initial value. Border and outline colors start
// as currentColor. A layout longhand or a widget-set text longhand
// stays undeclared instead (layoutWideProps, textWideProps): its
// initial value is the widget's programmatic setting.
func (sc *Scratch) setInitial(p Prop, v *Values, cx *ctx) {
	if layoutWideProps.Has(p) || textWideProps.Has(p) {
		return
	}
	init := initialValues()
	init.BorderColor = [4]render.Color{cx.color, cx.color, cx.color, cx.color}
	init.OutlineColor, init.OutlineAuto = cx.color, true
	copyProp(v, &init, p)
	v.Set = v.Set.With(p)
}

// revertBelow walks the revert-layer chain of p down from cascade
// layer from: the best declaration in the next layer below, and when
// that one is itself a revert-layer, the layer below it, until a
// concrete declaration or the bottom (nil).
func (sc *Scratch) revertBelow(p Prop, from int) *decl {
	for {
		var best *decl
		var bestRk uint64
		bestLayer := -1
		for i := range sc.chain {
			e := &sc.chain[i]
			if e.p != p || int(e.layer) >= from {
				continue
			}
			if bestLayer < 0 || e.rk > bestRk {
				best, bestRk, bestLayer = e.d, e.rk, int(e.layer)
			}
		}
		if best == nil || best.wide != wideRevertLayer {
			return best
		}
		from = bestLayer
	}
}

// revertOrigin walks the revert chain of p down from the rank from:
// the best declaration at a lower provider priority — the providers
// standing in for CSS's origins, so the theme sheets are what an
// application sheet's revert rolls back to — continuing below one that
// is itself a revert, until a concrete declaration or the bottom
// (nil: the theme's own values, unset).
func (sc *Scratch) revertOrigin(p Prop, from uint64) *decl {
	for {
		prio := rankPriority(from)
		var best *chainEntry
		for i := range sc.chain {
			e := &sc.chain[i]
			if e.p == p && rankPriority(e.rk) < prio && (best == nil || e.rk > best.rk) {
				best = e
			}
		}
		switch {
		case best == nil || best.d.wide == wideRevertLayer:
			return nil
		case best.d.wide != wideRevert:
			return best.d
		}
		from = best.rk
	}
}

// parse substitutes and parses d once per compute.
func (sc *Scratch) parse(d *decl, vars *Vars, cx *ctx) (*Values, bool) {
	if d.pre != nil && (d.preRem == 0 || d.preRem == cx.rem) {
		return d.pre, true
	}
	for i := range sc.parsed {
		if sc.parsed[i].d == d {
			return &sc.parsed[i].v, sc.parsed[i].ok
		}
	}
	ts := d.val
	ok := true
	if d.hasVar {
		ts, ok = substitute(sc.subBuf[:0], d.val, func(name string) ([]token, bool) { return sc.lookup(vars, name) }, 0)
		sc.subBuf = ts[:0]
		ts = trimWS(ts)
		ok = ok && len(ts) > 0
	}
	sc.parsed = append(sc.parsed, parsedDecl{d: d})
	e := &sc.parsed[len(sc.parsed)-1]
	e.v = initialValues()
	if ok {
		e.ok = d.parse(ts, cx, &e.v)
	}
	return &e.v, e.ok
}

// applyMeta resolves the winning content/counter-reset/
// counter-increment declarations — the meta declarations, which
// cascade outside the longhand table. The counter set follows
// css-lists-3 §4.4: inherited from the parent and the previous
// sibling (sc.prevCounters, supplied by the widget layer when
// counters are in play) with the values of the element preceding the
// node in tree order (sc.srcCounters); counter-reset instantiates,
// counter-increment bumps the innermost counter of its name. Content
// resolves against the node's own target (attr()) and counters.
func (sc *Scratch) applyMeta(v, pv *Values, cx *ctx, n Node) {
	v.Counters = inheritCounters(pv.Counters, sc.prevCounters, sc.srcCounters)
	if len(sc.metas) == 0 {
		return
	}
	var reset, incr, content *decl
	for i := range sc.metas {
		switch sc.metas[i].d.name {
		case "counter-reset":
			reset = sc.metas[i].d
		case "counter-increment":
			incr = sc.metas[i].d
		case "content":
			content = sc.metas[i].d
		}
	}
	if reset != nil || incr != nil {
		var self any = n
		if ni, ok := n.(nodeIDer); ok {
			self = ni.StyleID()
		}
		cs := v.Counters.clone()
		if reset != nil {
			if ts, ok := sc.metaTokens(reset, v.Vars); ok {
				cs.apply(ts, true, self)
			}
		}
		if incr != nil {
			if ts, ok := sc.metaTokens(incr, v.Vars); ok {
				cs.apply(ts, false, self)
			}
		}
		v.Counters = cs
	}
	if content == nil {
		return
	}
	ts, ok := sc.metaTokens(content, v.Vars)
	if !ok {
		return
	}
	cx.counters = v.Counters
	cx.quoteDepth = pv.QuoteDepth
	if c, ok := parseContentTokens(ts, cx); ok {
		v.Content, v.ContentURL, v.HasContent = c.text, c.url, true
		v.QuoteDepth = c.depth
	}
}

// metaTokens substitutes a meta declaration's var() references.
func (sc *Scratch) metaTokens(d *decl, vars *Vars) ([]token, bool) {
	if !d.hasVar {
		return d.val, true
	}
	ts, ok := substitute(nil, d.val, func(name string) ([]token, bool) { return sc.lookup(vars, name) }, 0)
	ts = trimWS(ts)
	return ts, ok && len(ts) > 0
}

// CounterEntry is one counter of a node's counter set: its name, its
// value at the node, and the element that instantiated it.
type CounterEntry struct {
	Name  string
	Value int
	// creator is the instantiating element's identity; sibling marks
	// an entry inherited from the previous sibling rather than the
	// parent (a reset there replaces it instead of nesting).
	creator any
	sibling bool
}

// CounterScope is a node's CSS counters set, outermost counter first.
// A computed set is never mutated: a node that changes it clones it.
type CounterScope struct {
	own []CounterEntry
}

// inheritCounters builds a node's inherited counters set (css-lists-3
// §4.4.1): the parent's counters, then the previous sibling's that the
// parent lacks (its own resets scope over its following siblings),
// each taking its value from src, the element preceding the node in
// tree order, when src holds the same counter.
func inheritCounters(parent, prev, src *CounterScope) *CounterScope {
	if prev == nil && src == nil {
		return parent
	}
	cs := parent.clone()
	if prev != nil {
		for _, e := range prev.own {
			if cs.index(e.Name, e.creator) < 0 {
				e.sibling = true
				cs.own = append(cs.own, e)
			}
		}
	}
	if src != nil {
		for i := range cs.own {
			if j := src.index(cs.own[i].Name, cs.own[i].creator); j >= 0 {
				cs.own[i].Value = src.own[j].Value
			}
		}
	}
	return cs
}

// Equal reports whether two counter sets hold the same counters with
// the same values.
func (cs *CounterScope) Equal(o *CounterScope) bool {
	if cs == nil || o == nil {
		return cs.len() == o.len()
	}
	return slices.Equal(cs.own, o.own)
}

// len is the number of counters in the set.
func (cs *CounterScope) len() int {
	if cs == nil {
		return 0
	}
	return len(cs.own)
}

// clone copies the set for a node that changes it.
func (cs *CounterScope) clone() *CounterScope {
	if cs == nil {
		return &CounterScope{}
	}
	return &CounterScope{own: slices.Clone(cs.own)}
}

// index finds the counter instantiated by creator under name, -1 when
// the set has none.
func (cs *CounterScope) index(name string, creator any) int {
	if cs == nil {
		return -1
	}
	for i := range cs.own {
		if cs.own[i].Name == name && cs.own[i].creator == creator {
			return i
		}
	}
	return -1
}

// innermost is the index of the last counter named name, -1 for none.
func (cs *CounterScope) innermost(name string) int {
	if cs == nil {
		return -1
	}
	for i, e := range slices.Backward(cs.own) {
		if e.Name == name {
			return i
		}
	}
	return -1
}

// get returns the innermost counter's value, 0 without one.
func (cs *CounterScope) get(name string) int {
	if i := cs.innermost(name); i >= 0 {
		return cs.own[i].Value
	}
	return 0
}

// chain returns the values of every counter named name, outermost
// first: counters()'s list.
func (cs *CounterScope) chain(name string) []int {
	if cs == nil {
		return nil
	}
	var out []int
	for _, e := range cs.own {
		if e.Name == name {
			out = append(out, e.Value)
		}
	}
	return out
}

// apply folds one counter-reset (reset) or counter-increment
// declaration into the set for the element self. A reset
// instantiates a counter, replacing an innermost one of the same name
// that self or a previous sibling created; an increment bumps the
// innermost counter, instantiating one at 0 when the set has none.
func (cs *CounterScope) apply(ts []token, reset bool, self any) {
	for i := 0; i < len(ts); i++ {
		if ts[i].kind != tkIdent {
			continue
		}
		name := ts[i].s
		n := 0
		if !reset {
			n = 1
		}
		j := i + 1
		for j < len(ts) && (ts[j].kind == tkWS || ts[j].kind == tkComma) {
			j++
		}
		if j < len(ts) && ts[j].kind == tkNumber {
			n = int(ts[j].num)
			i = j
		} else {
			i = j - 1
		}
		k := cs.innermost(name)
		if reset {
			if k >= 0 && (cs.own[k].sibling || cs.own[k].creator == self) {
				cs.own = slices.Delete(cs.own, k, k+1)
			}
			cs.own = append(cs.own, CounterEntry{Name: name, Value: n, creator: self})
			continue
		}
		if k < 0 {
			cs.own = append(cs.own, CounterEntry{Name: name, creator: self})
			k = len(cs.own) - 1
		}
		cs.own[k].Value += n
	}
}

// parseContentTokens resolves a content value: strings, attr(),
// counter() and counters(), url() (an image in the generated
// content), and the quote keywords open-quote/close-quote/
// no-open-quote/no-close-quote over the inherited quote depth.
// Anything else is invalid at computed-value time and drops the
// declaration.
type parsedContent struct {
	text  string
	url   string
	depth int
}

// defaultQuotes are the quote pairs the quote keywords emit without
// a quotes property: the typographic double and single pairs.
var defaultQuotes = [][2]string{{"\u201c", "\u201d"}, {"\u2018", "\u2019"}}

func parseContentTokens(ts []token, cx *ctx) (parsedContent, bool) {
	var c parsedContent
	c.depth = cx.quoteDepth
	if len(ts) == 1 && ts[0].kind == tkIdent {
		switch strings.ToLower(ts[0].s) {
		case "none", "normal":
			return c, true
		}
		return c, false
	}
	var b strings.Builder
	quote := func(closing bool) string {
		pair := defaultQuotes[min(c.depth, len(defaultQuotes)-1)]
		if closing {
			return pair[1]
		}
		return pair[0]
	}
	for i := 0; i < len(ts); i++ {
		t := ts[i]
		switch {
		case t.kind == tkString:
			b.WriteString(t.s)
		case t.kind == tkIdent:
			switch strings.ToLower(t.s) {
			case "open-quote":
				b.WriteString(quote(false))
				c.depth++
			case "close-quote":
				c.depth = max(0, c.depth-1)
				b.WriteString(quote(true))
			case "no-open-quote":
				c.depth++
			case "no-close-quote":
				c.depth = max(0, c.depth-1)
			default:
				return c, false
			}
		case t.kind == tkFunc && t.s == "url":
			end := blockEnd(ts, i)
			inner := trimWS(funcArgs(ts[i:end]))
			if len(inner) == 0 {
				return c, false
			}
			if len(inner) == 1 && inner[0].kind == tkString {
				c.url = inner[0].s
			} else if len(inner) == 1 && inner[0].kind == tkIdent {
				c.url = inner[0].s
			} else {
				c.url = tokensText(inner)
			}
			i = end - 1
		case t.kind == tkFunc && t.s == "attr" && cx.self != nil:
			end := blockEnd(ts, i)
			if a := trimWS(funcArgs(ts[i:end])); len(a) > 0 && a[0].kind == tkIdent {
				if v, ok := cx.self.attrValue(a[0].s); ok {
					b.WriteString(v)
				}
			}
			i = end - 1
		case t.kind == tkFunc && (t.s == "counter" || t.s == "counters"):
			end := blockEnd(ts, i)
			a := splitTop(trimWS(funcArgs(ts[i:end])), tkComma)
			if len(a) > 0 && trimWS(a[0]) != nil && trimWS(a[0])[0].kind == tkIdent {
				name := trimWS(a[0])[0].s
				if t.s == "counter" || len(a) < 2 {
					b.WriteString(cx.counterValue(name, ""))
				} else if sep := trimWS(a[1]); len(sep) == 1 && sep[0].kind == tkString {
					b.WriteString(cx.counterValue(name, sep[0].s))
				}
			}
			i = end - 1
		case t.kind == tkWS || t.kind == tkComma:
		default:
			return parsedContent{}, false
		}
	}
	c.text = b.String()
	return c, true
}

// counterValue renders counter(name) or counters(name, sep).
func (cx *ctx) counterValue(name, sep string) string {
	if cx.counters == nil {
		return "0"
	}
	if sep == "" {
		return strconv.Itoa(cx.counters.get(name))
	}
	vals := cx.counters.chain(name)
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, sep)
}

// lookup resolves a var() reference: the node's inherited custom
// properties first, then the @define-color defines, then the @property
// registrations' initial values, of the compute's layers.
func (sc *Scratch) lookup(vs *Vars, name string) ([]token, bool) {
	if ts, ok := vs.lookup(name); ok {
		return ts, true
	}
	if ts, ok := sc.define(name, 0); ok {
		return ts, true
	}
	return sc.registered(name)
}

// registered resolves a @property registration: the highest-priority
// layer's (the later on a tie), whose initial value answers for a
// name nothing declares. A universal syntax without an initial value
// has none, like an unregistered property.
func (sc *Scratch) registered(name string) ([]token, bool) {
	reg := sc.registration(name)
	if reg == nil {
		return nil, false
	}
	return reg.Initial, reg.Initial != nil
}

// registration is the @property registration of name that applies:
// the highest-priority layer's, the later on a tie; nil for none.
func (sc *Scratch) registration(name string) *PropertyReg {
	var reg *PropertyReg
	best := -1
	for i, l := range sc.layers {
		if l.Sheet == nil || l.Sheet.properties == nil {
			continue
		}
		if r, ok := l.Sheet.properties[name]; ok && (best < 0 || l.Priority >= sc.layers[best].Priority) {
			reg, best = r, i
		}
	}
	return reg
}

// define resolves a @define-color: the highest-priority layer's value
// (the later layer on a tie), its own references substituted against
// the defines - defines name colors, never a node's properties.
func (sc *Scratch) define(name string, depth int) ([]token, bool) {
	var ts []token
	best := -1
	for i, l := range sc.layers {
		if l.Sheet == nil {
			continue
		}
		if d, ok := l.Sheet.defines[name]; ok && (best < 0 || l.Priority >= sc.layers[best].Priority) {
			ts, best = d, i
		}
	}
	if best < 0 || depth > maxVarDepth {
		return nil, false
	}
	if !containsVar(ts) {
		return ts, true
	}
	out, ok := substitute(nil, ts, func(n string) ([]token, bool) { return sc.define(n, depth+1) }, depth)
	return trimWS(out), ok
}

// resolveVars computes the node's custom-property environment: the
// winning declarations with their own var() references substituted
// (against each other and the inherited chain), cycles made invalid.
func (sc *Scratch) resolveVars(parent *Vars) *Vars {
	if len(sc.customs) == 0 && !sc.hasNonInheritedProps() {
		return parent
	}
	sc.seedRegistrations()
	own := make(map[string][]token, len(sc.customs))
	const (
		pending = iota
		resolving
		done
	)
	state := make(map[string]uint8, len(sc.customs))
	var resolve func(name string, depth int) ([]token, bool)
	resolve = func(name string, depth int) ([]token, bool) {
		i, mine := sc.byName[name]
		if !mine {
			return sc.lookup(parent, name)
		}
		switch state[name] {
		case done:
			ts := own[name]
			return ts, ts != nil
		case resolving:
			return nil, false // a cycle: every member is invalid
		}
		if depth > maxVarDepth {
			return nil, false
		}
		state[name] = resolving
		d := sc.customs[i].d
		var ts []token
		ok := true
		switch d.wide {
		case wideInherit, wideUnset, wideRevert, wideRevertLayer:
			ts, ok = sc.lookup(parent, name)
		case wideInitial:
			ok = false
		default:
			ts = d.val
			if d.hasVar {
				ts, ok = substitute(nil, d.val, func(n string) ([]token, bool) { return resolve(n, depth+1) }, depth)
				ts = trimWS(ts)
			}
			// A registered property's value must match its syntax; one
			// that does not is invalid at computed-value time and acts
			// as unset — inherited, or the initial value when the
			// registration does not inherit.
			if reg := sc.registration(name); ok && reg != nil && reg.Syntax != "*" && !validInitial(reg.Syntax, ts) {
				if reg.Inherits {
					ts, ok = sc.lookup(parent, name)
				} else {
					ts, ok = reg.Initial, reg.Initial != nil
				}
			}
		}
		state[name] = done
		if !ok {
			own[name] = nil
			return nil, false
		}
		if ts == nil {
			ts = []token{} // an empty value is valid and distinct from invalid
		}
		own[name] = ts
		return ts, true
	}
	for _, w := range sc.customs {
		resolve(w.d.name, 0)
	}
	return &Vars{parent: parent, own: own}
}

// hasNonInheritedProps reports whether any loaded layer registers an
// inherits: false @property: those resolve on nodes that declare
// nothing for them, so the environment must be built even without a
// custom declaration.
func (sc *Scratch) hasNonInheritedProps() bool {
	for _, l := range sc.layers {
		if l.Sheet == nil {
			continue
		}
		for _, reg := range l.Sheet.properties {
			if !reg.Inherits {
				return true
			}
		}
	}
	return false
}

// seedRegistrations injects every inherits: false registration the
// node did not declare as a synthetic own declaration holding the
// registration's initial value, which stops the inherited chain:
// the CSS semantics of a non-inherited registered property.
func (sc *Scratch) seedRegistrations() {
	for _, l := range sc.layers {
		if l.Sheet == nil || l.Sheet.properties == nil {
			continue
		}
		for name, reg := range l.Sheet.properties {
			if reg.Inherits {
				continue
			}
			if _, mine := sc.byName[name]; mine {
				continue
			}
			d := &decl{name: name, custom: true, val: reg.Initial}
			sc.byName[name] = len(sc.customs)
			sc.customs = append(sc.customs, customWin{d: d})
		}
	}
}

// substitute appends ts to out with every var(--name[, fallback])
// replaced by the named value (or the substituted fallback). ok is false
// when a reference cannot resolve: the declaration is then invalid at
// computed-value time.
func substitute(out, ts []token, lookup func(string) ([]token, bool), depth int) ([]token, bool) {
	if depth > maxVarDepth {
		return nil, false
	}
	for i := 0; i < len(ts); i++ {
		t := ts[i]
		if t.kind != tkFunc || t.s != "var" {
			out = append(out, t)
			continue
		}
		end := blockEnd(ts, i)
		args := trimWS(funcArgs(ts[i:end]))
		if len(args) == 0 || args[0].kind != tkIdent || !strings.HasPrefix(args[0].s, "--") {
			return nil, false
		}
		name := args[0].s
		rest := trimWS(args[1:])
		val, found := lookup(name)
		switch {
		case found:
			out = append(out, val...)
		case len(rest) > 0 && rest[0].kind == tkComma:
			fb, ok := substitute(nil, rest[1:], lookup, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, trimWS(fb)...)
		default:
			return nil, false
		}
		i = end - 1
	}
	return out, true
}

// CopyLonghand copies longhand p's computed value from src to dst,
// whether src sets it included.
func CopyLonghand(dst, src *Values, p Prop) {
	copyProp(dst, src, p)
	dst.Set = dst.Set.Without(p)
	if src.Has(p) {
		dst.Set = dst.Set.With(p)
	}
}

// copyProp copies longhand p's computed value from src to dst.
func copyProp(dst, src *Values, p Prop) {
	switch p {
	case PropColor:
		dst.Color = src.Color
	case PropBackgroundColor:
		dst.Background = src.Background
	case PropBackgroundImage:
		dst.Bg.Images = src.Bg.Images
	case PropBackgroundSize:
		dst.Bg.Sizes = src.Bg.Sizes
	case PropBackgroundPosition:
		dst.Bg.Positions = src.Bg.Positions
	case PropBackgroundRepeat:
		dst.Bg.Repeats = src.Bg.Repeats
	case PropBackgroundClip:
		dst.Bg.Clips = src.Bg.Clips
	case PropBackgroundOrigin:
		dst.Bg.Origins = src.Bg.Origins
	case PropBackgroundAttachment:
		dst.Bg.Attachments = src.Bg.Attachments
	case PropBorderImageSource:
		dst.BorderImage.Source = src.BorderImage.Source
	case PropBorderImageSlice:
		dst.BorderImage.Slice, dst.BorderImage.Fill = src.BorderImage.Slice, src.BorderImage.Fill
	case PropBorderImageWidth:
		dst.BorderImage.Width = src.BorderImage.Width
	case PropBorderImageOutset:
		dst.BorderImage.Outset = src.BorderImage.Outset
	case PropBorderImageRepeat:
		dst.BorderImage.Repeat = src.BorderImage.Repeat
	case PropOpacity:
		dst.Opacity = src.Opacity
	case PropContainerType:
		dst.ContainerType = src.ContainerType
	case PropContainerName:
		dst.ContainerNames = src.ContainerNames
	case PropFilter:
		dst.Brightness = src.Brightness
		dst.Filter = src.Filter
	case PropPaddingTop:
		dst.Padding.Top = src.Padding.Top
	case PropPaddingRight:
		dst.Padding.Right = src.Padding.Right
	case PropPaddingBottom:
		dst.Padding.Bottom = src.Padding.Bottom
	case PropPaddingLeft:
		dst.Padding.Left = src.Padding.Left
	case PropMarginTop:
		dst.Margin.Top, dst.MarginAuto[0] = src.Margin.Top, src.MarginAuto[0]
	case PropMarginRight:
		dst.Margin.Right, dst.MarginAuto[1] = src.Margin.Right, src.MarginAuto[1]
	case PropMarginBottom:
		dst.Margin.Bottom, dst.MarginAuto[2] = src.Margin.Bottom, src.MarginAuto[2]
	case PropMarginLeft:
		dst.Margin.Left, dst.MarginAuto[3] = src.Margin.Left, src.MarginAuto[3]
	case PropBorderTopWidth:
		dst.BorderWidth.Top = src.BorderWidth.Top
	case PropBorderRightWidth:
		dst.BorderWidth.Right = src.BorderWidth.Right
	case PropBorderBottomWidth:
		dst.BorderWidth.Bottom = src.BorderWidth.Bottom
	case PropBorderLeftWidth:
		dst.BorderWidth.Left = src.BorderWidth.Left
	case PropBorderTopStyle:
		dst.BorderStyle[0] = src.BorderStyle[0]
	case PropBorderRightStyle:
		dst.BorderStyle[1] = src.BorderStyle[1]
	case PropBorderBottomStyle:
		dst.BorderStyle[2] = src.BorderStyle[2]
	case PropBorderLeftStyle:
		dst.BorderStyle[3] = src.BorderStyle[3]
	case PropBorderTopColor:
		dst.BorderColor[0] = src.BorderColor[0]
	case PropBorderRightColor:
		dst.BorderColor[1] = src.BorderColor[1]
	case PropBorderBottomColor:
		dst.BorderColor[2] = src.BorderColor[2]
	case PropBorderLeftColor:
		dst.BorderColor[3] = src.BorderColor[3]
	case PropBorderTopLeftRadius:
		dst.Radius.TopLeft = src.Radius.TopLeft
	case PropBorderTopRightRadius:
		dst.Radius.TopRight = src.Radius.TopRight
	case PropBorderBottomRightRadius:
		dst.Radius.BottomRight = src.Radius.BottomRight
	case PropBorderBottomLeftRadius:
		dst.Radius.BottomLeft = src.Radius.BottomLeft
	case PropBoxShadow:
		dst.Shadow = src.Shadow
	case PropOutlineWidth:
		dst.OutlineWidth = src.OutlineWidth
	case PropOutlineStyle:
		dst.OutlineStyle = src.OutlineStyle
	case PropOutlineColor:
		dst.OutlineColor, dst.OutlineAuto = src.OutlineColor, src.OutlineAuto
	case PropOutlineOffset:
		dst.OutlineOffset = src.OutlineOffset
	case PropBorderSpacing:
		dst.BorderSpacingH, dst.BorderSpacingV = src.BorderSpacingH, src.BorderSpacingV
	case PropFontFamily:
		dst.FontFamily = src.FontFamily
	case PropFontSize:
		dst.FontSize = src.FontSize
	case PropFontWeight:
		dst.FontWeight = src.FontWeight
	case PropFontStyle:
		dst.Italic = src.Italic
	case PropLetterSpacing:
		dst.LetterSpacing = src.LetterSpacing
	case PropLineHeight:
		dst.LineHeight, dst.LineHeightFactor = src.LineHeight, src.LineHeightFactor
	case PropFontFeatures:
		dst.Features = src.Features
	case PropFontVariations:
		dst.Variations = src.Variations
	case PropTransform:
		dst.Transform = src.Transform
	case PropTransformOrigin:
		dst.OriginFrac, dst.OriginPx = src.OriginFrac, src.OriginPx
	case PropTextTransform:
		dst.TextTransform = src.TextTransform
	case PropIconSize:
		dst.IconSize = src.IconSize
	case PropTransitionProperty:
		dst.Transition.Props = src.Transition.Props
	case PropTransitionDuration:
		dst.Transition.Durations = src.Transition.Durations
	case PropTransitionTiming:
		dst.Transition.Timings = src.Transition.Timings
	case PropTransitionDelay:
		dst.Transition.Delays = src.Transition.Delays
	case PropAnimation:
		dst.Anim.Names = src.Anim.Names
	case PropAnimationPlayState:
		dst.Anim.Running = src.Anim.Running
	case PropAnimDuration:
		dst.Anim.Durations = src.Anim.Durations
	case PropAnimDelay:
		dst.Anim.Delays = src.Anim.Delays
	case PropAnimDirection:
		dst.Anim.Directions = src.Anim.Directions
	case PropAnimFill:
		dst.Anim.Fills = src.Anim.Fills
	case PropAnimTiming:
		dst.Anim.Timings = src.Anim.Timings
	case PropAnimIteration:
		dst.Anim.Iterations = src.Anim.Iterations
	case PropIconTransform:
		dst.IconXform = src.IconXform
	case PropIconSource:
		dst.IconSource = src.IconSource
	case PropIconPalette:
		dst.PaletteTint = src.PaletteTint
	case PropCaretColor:
		dst.CaretColor = src.CaretColor
	case PropTextDecorationLine:
		dst.Decoration.Lines = src.Decoration.Lines
	case PropTextDecorationStyle:
		dst.Decoration.Style = src.Decoration.Style
	case PropTextDecorationColor:
		dst.Decoration.Color = src.Decoration.Color
	case PropTextShadow:
		dst.TextShadow = src.TextShadow
	case PropFontPalette:
		dst.Palette = src.Palette
	case PropOverflowWrap:
		dst.OverflowWrap = src.OverflowWrap
	case PropCursor:
		dst.Cursor = src.Cursor
	case PropPointerEvents:
		dst.PointerEvents = src.PointerEvents
	case PropUserSelect:
		dst.UserSelect = src.UserSelect
	case PropCaretShape:
		dst.CaretShape = src.CaretShape
	case PropAccentColor:
		dst.AccentColor, dst.AccentAuto = src.AccentColor, src.AccentAuto
	case PropAppearance:
		dst.Appearance = src.Appearance
	case PropTouchAction:
		dst.TouchAction = src.TouchAction
	case PropTextAlign:
		dst.TextAlign = src.TextAlign
	case PropTextIndent:
		dst.TextIndent = src.TextIndent
	case PropWhiteSpace:
		dst.WhiteSpace = src.WhiteSpace
	case PropWordBreak:
		dst.WordBreak = src.WordBreak
	case PropLineBreak:
		dst.LineBreak = src.LineBreak
	case PropHyphens:
		dst.Hyphens = src.Hyphens
	case PropWordSpacing:
		dst.WordSpacing = src.WordSpacing
	case PropVerticalAlign:
		dst.VerticalAlign, dst.VerticalAlignPx, dst.VerticalAlignPct = src.VerticalAlign, src.VerticalAlignPx, src.VerticalAlignPct
	case PropLineClamp:
		dst.LineClamp = src.LineClamp
	case PropTextOverflow:
		dst.Overflow = src.Overflow
	case PropFontVariantCaps:
		dst.VariantCaps = src.VariantCaps
	case PropFontVariantNumeric:
		dst.VariantNumeric = src.VariantNumeric
	case PropFontVariantLigatures:
		dst.VariantLig = src.VariantLig
	case PropFontVariantEastAsian:
		dst.VariantEA = src.VariantEA
	case PropFontVariantPosition:
		dst.VariantPos = src.VariantPos
	case PropFontVariantAlternates:
		dst.VariantAlt = src.VariantAlt
	case PropFontSynthesis:
		dst.Synthesis = src.Synthesis
	case PropFontOpticalSizing:
		dst.OpticalSizing = src.OpticalSizing
	case PropDirection:
		dst.Direction = src.Direction
	case PropUnicodeBidi:
		dst.UnicodeBidi = src.UnicodeBidi
	case PropWritingMode:
		dst.WritingMode = src.WritingMode
	case PropBackdropFilter:
		dst.Backdrop = src.Backdrop
	case PropMixBlendMode:
		dst.Blend = src.Blend
	case PropIsolation:
		dst.Isolate = src.Isolate
	case PropClipPath:
		dst.Clip = src.Clip
	case PropMaskImage:
		dst.Mask.Image = src.Mask.Image
	case PropMaskPosition:
		dst.Mask.Pos = src.Mask.Pos
	case PropMaskSize:
		dst.Mask.Size = src.Mask.Size
	case PropMaskRepeat:
		dst.Mask.Repeat = src.Mask.Repeat
	case PropMaskMode:
		dst.Mask.Luminance = src.Mask.Luminance
	case PropPerspective:
		dst.Perspective = src.Perspective
	case PropTransformStyle:
		dst.Preserve3D = src.Preserve3D
	case PropBackfaceVisibility:
		dst.BackfaceHidden = src.BackfaceHidden
	case PropLayoutMotion:
		dst.LayoutMotion = src.LayoutMotion
	case PropScrollBehavior:
		dst.ScrollSmooth = src.ScrollSmooth
	case PropScrollMarginTop:
		dst.ScrollMargin.Top = src.ScrollMargin.Top
	case PropScrollMarginRight:
		dst.ScrollMargin.Right = src.ScrollMargin.Right
	case PropScrollMarginBottom:
		dst.ScrollMargin.Bottom = src.ScrollMargin.Bottom
	case PropScrollMarginLeft:
		dst.ScrollMargin.Left = src.ScrollMargin.Left
	case PropScrollPaddingTop:
		dst.ScrollPad.Top = src.ScrollPad.Top
	case PropScrollPaddingRight:
		dst.ScrollPad.Right = src.ScrollPad.Right
	case PropScrollPaddingBottom:
		dst.ScrollPad.Bottom = src.ScrollPad.Bottom
	case PropScrollPaddingLeft:
		dst.ScrollPad.Left = src.ScrollPad.Left
	case PropScrollSnapType:
		dst.Snap = src.Snap
	case PropScrollSnapAlign:
		dst.SnapAlign = src.SnapAlign
	case PropScrollSnapStop:
		dst.SnapStopAlways = src.SnapStopAlways
	case PropOverscrollX:
		dst.Overscroll[0] = src.Overscroll[0]
	case PropOverscrollY:
		dst.Overscroll[1] = src.Overscroll[1]
	case PropScrollbarColor:
		dst.BarThumb, dst.BarTrack, dst.BarColorAuto = src.BarThumb, src.BarTrack, src.BarColorAuto
	case PropScrollbarWidth:
		dst.BarWidth = src.BarWidth
	case PropScrollbarGutter:
		dst.BarGutter = src.BarGutter
	case PropTransitionBehavior:
		dst.Transition.Discrete = src.Transition.Discrete
	case PropAnimComposition:
		dst.Anim.Compositions = src.Anim.Compositions
	default:
		copyLayoutProp(&dst.Layout, &src.Layout, p)
	}
}

// tokensText serializes tokens back to CSS source text.
func tokensText(ts []token) string {
	var b strings.Builder
	for _, t := range ts {
		switch t.kind {
		case tkIdent:
			b.WriteString(t.s)
		case tkFunc:
			b.WriteString(t.s)
			b.WriteByte('(')
		case tkAt:
			b.WriteByte('@')
			b.WriteString(t.s)
		case tkHash:
			b.WriteByte('#')
			b.WriteString(t.s)
		case tkString:
			b.WriteString(strconv.Quote(t.s))
		case tkNumber:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
		case tkPercent:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
			b.WriteByte('%')
		case tkDim:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
			b.WriteString(t.s)
		case tkWS:
			b.WriteByte(' ')
		default:
			b.WriteString(t.s)
		}
	}
	return b.String()
}
