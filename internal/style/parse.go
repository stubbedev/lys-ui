// Package style is lys-ui's CSS engine: a tokenizer and parser for the
// GTK-flavored subset docs/css.md scopes, the rule index, and the
// matcher/cascader that computes one widget's style — custom properties
// with var() substitution, calc(), color-mix(), and per-longhand
// cascade across prioritized stylesheets and per-widget inline
// declarations. It knows nothing about widgets; the widget package
// fills the Target, walks the tree, and layers the result over the
// typed Theme.
//
// The error policy is CSS's: a malformed declaration drops that
// declaration, a malformed selector drops that rule, an unknown
// at-rule drops its block, and everything around them still applies.
// A declaration whose var() references make it invalid is invalid at
// computed-value time and behaves as `unset`. Warnings go to the
// injected library logger at Warn, silent by default — the same
// contract as SetTheme's contrast warnings.
package style

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/logutil"
)

// parseWarn is the parse-warning sink: warnings route to the injected
// library logger at Warn level. Tests swap it to capture.
var parseWarn = func(msg string) { logutil.L().Warn(msg) }

// SetParseWarn installs f as the parse-warning sink; nil restores the
// library logger. One sink per process, like the widget package's
// theme warning sink.
func SetParseWarn(f func(msg string)) {
	if f == nil {
		f = func(msg string) { logutil.L().Warn(msg) }
	}
	parseWarn = f
}

// warnf reports one stylesheet problem at Warn and lets the parser
// carry on with the surrounding rules.
func warnf(format string, args ...any) {
	parseWarn("css: " + fmt.Sprintf(format, args...))
}

// Warn reports one stylesheet problem through the parse-warning sink
// outside a parse (a hot-reload stat failure, say), with the css:
// prefix warnf's messages carry.
func Warn(msg string) { parseWarn("css: " + msg) }

// cssWide is a CSS-wide keyword a declaration's whole value can be.
type cssWide uint8

const (
	wideNone cssWide = iota
	wideInherit
	wideInitial
	wideUnset
	wideRevert
	wideRevertLayer
)

// decl is one parsed declaration: which longhands it covers, its value
// tokens, and whether they reference var() (then the value is parsed
// per node at compute time, after substitution).
type decl struct {
	name   string // the property name, or the custom property's --name
	custom bool
	// meta marks the declarations that cascade outside the longhand
	// table: content, counter-reset, counter-increment.
	meta   bool
	covers PropSet
	parse  func([]token, *ctx, *Values) bool
	val    []token
	hasVar bool
	wide   cssWide
	// important marks a `!important` value: it outranks every normal
	// declaration, whatever the priority or specificity.
	important bool
	// order is the declaration's source position within its sheet:
	// later wins among equal priority and specificity.
	order int
	// pre is the parsed value of a var-free declaration that read no
	// node-dependent input (em, currentColor, the inherited weight): it
	// is the same for every node, so compute copies it instead of
	// re-parsing.
	pre *Values
	// preRem is the rem base pre was computed with, zero when it read
	// none: a compute under another base re-parses.
	preRem float64
}

// Block is a parsed declaration list: one rule's body, or a widget's
// inline style.
type Block struct {
	decls []decl
}

// Len returns the number of valid declarations.
func (b *Block) Len() int {
	if b == nil {
		return 0
	}
	return len(b.decls)
}

// ParseDeclarations parses a bare declaration list — `color: red;
// --x: 1px` — the body a widget's inline style carries. Malformed
// declarations are warned about and dropped.
func ParseDeclarations(css string) *Block {
	p := parser{src: css}
	ts := tokenize(css)
	return &Block{decls: p.declarations(ts)}
}

// Sheet is a parsed stylesheet: the rules in source order plus the
// match index. It is immutable after Parse and safe for concurrent
// readers.
type Sheet struct {
	rules []rule
	// buckets key rules by their rightmost compound — the only buckets
	// a widget can plausibly match. A rule is filed under the id when
	// the rightmost compound names one, else its first class, else its
	// element, else the universal bucket.
	byElement map[string][]int32
	byClass   map[string][]int32
	byID      map[string][]int32
	universal []int32

	// mediaQs holds the distinct @media conditions the sheet's rules
	// carry, by first use; a rule's mq indexes it, -1 when the rule is
	// unconditional.
	mediaQs []mediaQuery

	// containerQs holds the distinct @container conditions the sheet's
	// rules carry, by first use; a rule's cq indexes it, -1 when the
	// rule is not container-conditional.
	containerQs []containerQuery

	// layerNames is the sheet's @layer paths of first appearance;
	// rules carry the index of their layer, 255 when unlayered.
	// layerRanks maps those indices onto the cascade order dotted
	// names define: nested layers sort below their parent.
	layerNames []string
	layerRanks []uint8

	// fontFaces holds the sheet's @font-face rules in source order.
	fontFaces []*FontFace

	// properties holds the sheet's @property registrations by custom
	// property name; a later registration replaces an earlier one.
	properties map[string]*PropertyReg

	// keyframes holds the sheet's @keyframes rules by (lowercased)
	// name; a later rule of the same name replaces an earlier one.
	keyframes map[string]*Keyframes

	// defines holds the sheet's @define-color values by custom-property
	// name (--name); a later define replaces an earlier one. They are
	// global, not inherited: the cascade consults them after the
	// node's inherited custom properties (Scratch.lookup).
	defines map[string][]token

	sens Sensitivity
}

// Sensitivity summarizes what a sheet's selectors read beyond a node's
// own facts, so a state or class flip restyles no more than it must: a
// state bit or class named left of a combinator makes descendants (or
// following siblings) depend on it.
type Sensitivity struct {
	// AncestorStates are the state bits some selector tests on a
	// non-rightmost compound.
	AncestorStates State
	// AncestorClasses are the classes some selector tests on a
	// non-rightmost compound.
	AncestorClasses map[string]bool
	// Structural reports structural pseudo-classes or sibling
	// combinators anywhere: a sibling's add, removal, reorder or
	// visibility can then restyle its neighbors.
	Structural bool
	// SiblingFacts reports sibling combinators or an :nth-*(... of S)
	// anywhere: a sibling's class or state can then restyle its
	// neighbors. Position alone (:last-child) reads neither.
	SiblingFacts bool
	// HasRelational reports a :has() anywhere: a descendant or
	// following sibling's facts then feed an ancestor's match, so class
	// and state changes restyle up the tree too.
	HasRelational bool
	// Pseudo reports a ::before/::after rule: the widget layer then
	// computes pseudo-element styles.
	Pseudo bool
	// Counters reports counter-reset or counter-increment in play: the
	// widget layer then chains siblings' counter scopes.
	Counters bool
	// Container reports a @container rule and CqUnits a
	// container-query unit: either makes the per-compute container
	// resolution (and a viewport change's restyle) necessary, since
	// containers resize with the window.
	Container bool
	CqUnits   bool
	// Revert reports a revert or revert-layer value: the cascade then
	// records every declaration, the chain the two roll back along.
	Revert bool
}

// Sensitivity returns the sheet's selector summary.
func (s *Sheet) Sensitivity() *Sensitivity { return &s.sens }

// MediaQueries reports how many distinct @media conditions the
// sheet's rules carry: zero when nothing in it is media-conditional.
func (s *Sheet) MediaQueries() int { return len(s.mediaQs) }

// ContainerQueries reports how many distinct @container conditions
// the sheet's rules carry: zero when nothing in it is
// container-conditional.
func (s *Sheet) ContainerQueries() int { return len(s.containerQs) }

// FontFaces returns the sheet's @font-face rules in source order, for
// the widget layer to load and register by family.
func (s *Sheet) FontFaces() []*FontFace { return s.fontFaces }

// Keyframes returns the sheet's @keyframes rule of that name, nil when
// it declares none.
func (s *Sheet) Keyframes(name string) *Keyframes {
	if s.keyframes == nil {
		return nil
	}
	return s.keyframes[name]
}

// addKeyframes files one @keyframes rule; a later rule of the same
// name replaces an earlier one.
func (s *Sheet) addKeyframes(kf *Keyframes) {
	if s.keyframes == nil {
		s.keyframes = make(map[string]*Keyframes)
	}
	s.keyframes[kf.Name] = kf
}

// rule is one selector of one stylesheet rule with its specificity,
// declarations, @media condition, cascade layer, @container
// condition, scoping, and — when the rule carries one — its
// @starting-style block.
type rule struct {
	sel   selector
	spec  spec
	block *Block
	mq    int32
	layer uint8
	// cq indexes the sheet's @container conditions, -1 when the rule
	// is not container-conditional.
	cq int32
	// scope is the @scope rule the rule sits in, nil outside one: the
	// cascade ranks it by the proximity of its scoping root.
	scope *scopeCtx
	// pseudo names the ::before/::after/::first-letter pseudo-element
	// this rule addresses; empty styles the element itself.
	pseudo string
	// start is the rule's @starting-style block, the entry
	// transition's declarations; nil when the rule carries none.
	start *Block
}

// Parse compiles css into a Sheet. Malformed declarations and rules are
// warned about and skipped; the valid remainder still applies. An
// empty (or all-invalid) input yields an empty Sheet — never nil.
func Parse(css string) *Sheet {
	return parseSheet(css, "", nil, nil)
}

// ParseCollecting parses css like Parse and returns the sheet plus
// every warning the parse produced, without routing them to the sink:
// the caller reports them its own way (a file loader turns them into
// an error carrying the path).
func ParseCollecting(css string) (*Sheet, []string) {
	return ParseImporting(css, "", nil)
}

// ParseImporting parses css whose base is the importing file's path
// and whose @import targets load through resolve (nil: the
// filesystem, relative to the importing file). It is the file-backed
// loader's parse: imports resolve beside the file, and a cycle or a
// missing import reports precisely.
func ParseImporting(css, base string, resolve ImportResolver) (*Sheet, []string) {
	var warns []string
	sheet := parseSheet(css, base, resolve, func(msg string) { warns = append(warns, msg) })
	return sheet, warns
}

func parseSheet(css, base string, resolve ImportResolver, warn func(string)) *Sheet {
	p := parser{src: css, warn: warn, resolve: resolve, base: base, layerPath: "", cq: -1}
	p.stylesheet(tokenize(css))
	s := &p.sheet
	s.orderLayers()
	for i := range s.rules {
		r := &s.rules[i]
		if r.layer != 255 && int(r.layer) < len(s.layerRanks) {
			r.layer = s.layerRanks[r.layer]
		}
	}
	return s
}

// parser walks the token stream once.
type parser struct {
	src   string
	sheet Sheet
	order int
	// startBlock is the @starting-style block of the rule body being
	// parsed; ruleBody attaches it to the rules it files.
	startBlock *Block
	// warn, when set, receives this parse's warnings instead of the
	// sink (ParseCollecting).
	warn func(msg string)
	// mq is the active @media condition (nil: everything), cq the
	// active @container condition (-1: none), and layerPath the
	// active cascade layer's dotted path ("": unlayered); dead is the
	// nesting depth of @supports branches that evaluated false.
	mq        mediaQuery
	cq        int32
	layerPath string
	dead      int
	// scope is the active @scope rule, nil outside one; anonSeq names
	// fresh anonymous layers.
	scope   *scopeCtx
	anonSeq int
	// resolve loads @import targets; base is the importing file's
	// path, importStack the active import chain for cycle detection.
	resolve     ImportResolver
	base        string
	importStack []string
	// rulesSeen reports a rule in the stylesheet being parsed: an
	// @import after one is invalid (css-cascade-5 §2).
	rulesSeen bool
}

// warnf reports one stylesheet problem and lets the parser carry on
// with the surrounding rules.
func (p *parser) warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if p.warn != nil {
		p.warn(msg)
		return
	}
	parseWarn("css: " + msg)
}

// stylesheet consumes rule after rule to the end of the input.
func (p *parser) stylesheet(ts []token) {
	for i := 0; i < len(ts); {
		t := ts[i]
		switch t.kind {
		case tkWS, tkSemi:
			i++
		case tkRBrace:
			i++ // a stray closer: drop it, keep parsing
		case tkAt:
			if !importPrelude(ts, i) {
				p.rulesSeen = true
			}
			i = p.atRule(ts, i)
		default:
			p.rulesSeen = true
			i = p.qualifiedRule(ts, i)
		}
	}
}

// importPrelude reports whether the at-rule at ts[i] may precede an
// @import (css-cascade-5 §2): @import itself, @charset, and a @layer
// statement — plus GTK's @define-color, which GTK accepts ahead of its
// imports as well.
func importPrelude(ts []token, i int) bool {
	switch ts[i].s {
	case "import", "charset", "define-color":
		return true
	case "layer":
		for j := i + 1; j < len(ts); j++ {
			switch ts[j].kind {
			case tkSemi:
				return true
			case tkLBrace:
				return false
			}
		}
	}
	return false
}

// atRule consumes one at-rule: through its block, or to its
// semicolon. @keyframes, @define-color, @media, @supports, @layer,
// @import, @container, @font-face, @property and @scope participate;
// the rest skip with a warning.
func (p *parser) atRule(ts []token, i int) int {
	name := ts[i].s
	switch name {
	case "keyframes", "-gtk-keyframes":
		return p.keyframes(ts, i)
	case "define-color", "-lys-define":
		// GTK's @define-color names a color; lys-ui's @-lys-define
		// names any value (a duration, a curve) the same way: a
		// sheet-wide var() fallback, declared on no element.
		return p.defineColor(ts, i)
	case "media":
		return p.mediaRule(ts, i)
	case "supports":
		return p.supportsRule(ts, i)
	case "layer":
		return p.layerRule(ts, i)
	case "import":
		return p.importRule(ts, i)
	case "container":
		return p.containerRule(ts, i)
	case "font-face":
		return p.fontFaceRule(ts, i)
	case "property":
		return p.propertyRule(ts, i)
	case "scope":
		return p.scopeRule(ts, i)
	}
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			j++
			p.warnAt(name)
			return j
		case tkLBrace:
			end := blockEnd(ts, j)
			p.warnAt(name)
			return end
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnAt(name)
	return j
}

// defineColor files GTK's `@define-color name value;` as a sheet-level
// define under the custom-property name --name. References (`@name`,
// compiled to var(--name)) resolve through the ordinary var()
// substitution: a custom property on the node's chain first - so a
// subtree's --name overrides - then the defines, the highest-priority
// layer winning; defines may reference each other, and a cycle is
// invalid like any var() cycle. No second color resolver exists.
func (p *parser) defineColor(ts []token, i int) int {
	j := i + 1
	for j < len(ts) && ts[j].kind != tkSemi && ts[j].kind != tkLBrace {
		if closer(ts[j].kind) != 0 {
			j = blockEnd(ts, j) - 1
		}
		j++
	}
	if j < len(ts) && ts[j].kind == tkLBrace {
		p.warnf("@define-color skipped: want `name value;`")
		return blockEnd(ts, j)
	}
	body := trimWS(ts[i+1 : min(j, len(ts))])
	next := min(j+1, len(ts))
	if len(body) < 2 || body[0].kind != tkIdent {
		p.warnf("@define-color skipped: want `name value;`")
		return next
	}
	val := colorRefs(trimWS(body[1:]))
	if len(val) == 0 {
		p.warnf("@define-color skipped: %s has no value", body[0].s)
		return next
	}
	if p.sheet.defines == nil && p.dead == 0 {
		p.sheet.defines = make(map[string][]token)
	}
	if p.dead == 0 {
		p.sheet.defines["--"+body[0].s] = val
	}
	return next
}

// colorRefs rewrites GTK named-color references (`@name`) into
// `var(--name)`, the form the cascade resolves - what @define-color
// declares and what the theme layer provides for the Adwaita names.
func colorRefs(ts []token) []token {
	if !slices.ContainsFunc(ts, func(t token) bool { return t.kind == tkAt }) {
		return ts
	}
	out := make([]token, 0, len(ts)+2)
	for _, t := range ts {
		if t.kind != tkAt {
			out = append(out, t)
			continue
		}
		out = append(out,
			token{kind: tkFunc, s: "var", pos: t.pos, end: t.pos},
			token{kind: tkIdent, s: "--" + t.s, pos: t.pos, end: t.end},
			token{kind: tkRParen, pos: t.end, end: t.end})
	}
	return out
}

// keyframes parses one @keyframes rule into the sheet: the name, then
// stops of selector list (`from`, `to`, percentages) over declaration
// blocks. A stop keeps its phase even when it declares nothing the
// engine animates; declarations that do not interpolate warn and drop.
func (p *parser) keyframes(ts []token, i int) int {
	j := i + 1
	for ; j < len(ts); j++ {
		if ts[j].kind == tkWS {
			continue
		}
		if ts[j].kind == tkLBrace {
			break
		}
		if ts[j].kind == tkSemi {
			return j + 1 // `@keyframes name;`: dropped
		}
	}
	if j >= len(ts) {
		p.warnf("@keyframes skipped: want a block")
		return j
	}
	name := ""
	for k := i + 1; k < j; k++ {
		if ts[k].kind == tkWS {
			continue
		}
		name = strings.ToLower(ts[k].s)
		break
	}
	end := blockEnd(ts, j)
	if name == "" {
		p.warnf("@keyframes skipped: want a name")
		return min(end, len(ts))
	}
	kf := &Keyframes{Name: name}
	body := ts[j+1 : end-1]
	for x := 0; x < len(body); {
		if body[x].kind == tkWS || body[x].kind == tkSemi || body[x].kind == tkRBrace {
			x++
			continue
		}
		start := x
		for x < len(body) && body[x].kind != tkLBrace {
			if closer(body[x].kind) != 0 {
				x = blockEnd(body, x)
				continue
			}
			x++
		}
		if x >= len(body) {
			p.warnf("@keyframes stop skipped: want '{'")
			break
		}
		prelude := trimWS(body[start:x])
		stopEnd := blockEnd(body, x)
		frame := p.keyframe(p.declarations(body[x+1 : stopEnd-1]))
		for _, off := range keyframeOffsets(prelude) {
			f := frame
			f.Offset = off
			kf.Frames = append(kf.Frames, f)
		}
		x = stopEnd
	}
	sortFrames(kf)
	if p.dead == 0 {
		p.sheet.addKeyframes(kf)
	}
	return end
}

// keyframeOffsets maps a stop's selector list to offsets: `from` is 0,
// `to` is 1, `N%` is N/100.
func keyframeOffsets(ts []token) []float64 {
	var out []float64
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) != 1 {
			continue
		}
		switch {
		case part[0].ident("from"):
			out = append(out, 0)
		case part[0].ident("to"):
			out = append(out, 1)
		case part[0].kind == tkPercent:
			if v := part[0].num; v >= 0 && v <= 100 {
				out = append(out, v/100)
			}
		}
	}
	return out
}

// keyframe computes one stop's declarations (css-animations-1 §3):
// each through its property's parser, against the parse-time root
// context, so a stop animates every longhand a rule can set. Its
// animation-timing-function eases the interval the stop starts; the
// animation and transition longhands, !important declarations, custom
// properties and var() values do not animate and drop with a warning.
func (p *parser) keyframe(decls []decl) Keyframe {
	k := Keyframe{V: new(Values)}
	*k.V = initialValues()
	cx := ctx{rem: parseRem, em: parseRem}
	for i := range decls {
		d := &decls[i]
		switch {
		case d.name == "animation-timing-function":
			tm, ok := timingOf(trimWS(d.val), &cx)
			if !ok {
				p.warnf("keyframe declaration skipped: animation-timing-function: %s", badText(rawText(p.src, d.val)))
				continue
			}
			k.Timing = &tm
		case d.important || d.custom || d.meta || d.hasVar || d.wide != wideNone:
			p.warnf("keyframe declaration skipped: %q does not animate here", d.name)
		case !d.covers.And(transitionProps.Or(animationProps).With(PropAnimComposition)).Empty():
			p.warnf("keyframe declaration skipped: %q is not animatable", d.name)
		case !d.parse(d.val, &cx, k.V):
			p.warnf("keyframe declaration skipped: %s: %s", d.name, badText(rawText(p.src, d.val)))
		default:
			k.Props = k.Props.Or(d.covers)
		}
	}
	k.V.Set = k.Props
	return k
}

func (p *parser) warnAt(name string) {
	switch name {
	case "keyframes", "-gtk-keyframes":
		return
	}
	p.warnf("at-rule @%s ignored", name)
}

// mediaRule parses one @media: the condition becomes the context of
// every rule in the block, intersecting any enclosing condition.
func (p *parser) mediaRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@media")
	if !ok {
		return end
	}
	q, okq := parseMediaQuery(prelude)
	if !okq {
		p.warnf("@media skipped: bad query %q", badText(rawText(p.src, prelude)))
		return end
	}
	save := p.mq
	p.mq = intersectMedia(p.mq, q)
	p.stylesheet(body)
	p.mq = save
	return end
}

// supportsRule parses one @supports: the condition evaluates against
// the engine's actual support at parse time, so a false block's rules
// never compile.
func (p *parser) supportsRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@supports")
	if !ok {
		return end
	}
	save := p.dead
	if !evalSupports(prelude) {
		p.dead++
	}
	p.stylesheet(body)
	p.dead = save
	return end
}

// layerRule parses one @layer: `@layer a, b.c;` fixes the order of
// first appearance (registering the dotted prefixes), `@layer name {
// ... }` styles in name's layer (the block's last name, nested under
// any enclosing layer), `@layer { ... }` in a fresh anonymous layer.
// Unlayered rules outrank every layer; a nested layer sorts below
// its parent.
func (p *parser) layerRule(ts []token, i int) int {
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			for _, name := range splitTop(trimWS(ts[i+1:j]), tkComma) {
				if n := dottedName(name); n != "" {
					p.layerOf(joinLayer(p.layerPath, n))
				}
			}
			return j + 1
		case tkLBrace:
			goto block
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnf("@layer skipped: want a block")
	return j
block:
	names := splitTop(trimWS(ts[i+1:j]), tkComma)
	end := blockEnd(ts, j)
	if end > len(ts) || ts[end-1].kind != tkRBrace {
		p.warnf("@layer skipped: unterminated block")
		return len(ts)
	}
	last := ""
	for _, name := range names {
		n := dottedName(name)
		if n == "" {
			n = p.anonLayer()
		}
		last = n
		p.layerOf(joinLayer(p.layerPath, n))
	}
	if last == "" {
		last = p.anonLayer()
		p.layerOf(joinLayer(p.layerPath, last))
	}
	save := p.layerPath
	p.layerPath = joinLayer(p.layerPath, last)
	p.stylesheet(ts[j+1 : end-1])
	p.layerPath = save
	return end
}

// rawName lowercases a layer name token span.
func rawName(ts []token) string {
	if len(ts) != 1 || ts[0].kind != tkIdent {
		return ""
	}
	return strings.ToLower(ts[0].s)
}

// layerOf returns path's index in the sheet's layer list,
// registering the path and every dotted prefix on first use.
func (p *parser) layerOf(path string) uint8 {
	if path == "" {
		return 255
	}
	if i := slices.Index(p.sheet.layerNames, path); i >= 0 && i <= math.MaxUint8 {
		return uint8(i)
	}
	full := ""
	for part := range strings.SplitSeq(path, ".") {
		if full == "" {
			full = part
		} else {
			full += "." + part
		}
		if !slices.Contains(p.sheet.layerNames, full) {
			if n := len(p.sheet.layerNames); n < 254 {
				p.sheet.layerNames = append(p.sheet.layerNames, full)
			} else {
				return 253
			}
		}
	}
	if i := slices.Index(p.sheet.layerNames, path); i >= 0 && i <= math.MaxUint8 {
		return uint8(i)
	}
	return 253
}

// curLayer is the index of the parser's active layer path.
func (p *parser) curLayer() uint8 {
	if p.layerPath == "" {
		return 255
	}
	return p.layerOf(p.layerPath)
}

// containerRule parses one @container: its condition list becomes the
// context of every rule in the block, joining any enclosing
// @container's conditions (each level selects its own query
// container).
func (p *parser) containerRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@container")
	if !ok {
		return end
	}
	q, okq := parseContainerQuery(prelude)
	if !okq {
		p.warnf("@container skipped: bad query %q", badText(rawText(p.src, prelude)))
		return end
	}
	saveCQ := p.cq
	var full containerQuery
	if p.cq >= 0 {
		full = slices.Clone(p.sheet.containerQs[p.cq])
	}
	full = append(full, q)
	p.cq = intconv.Int32(len(p.sheet.containerQs))
	for i, other := range p.sheet.containerQs {
		if slices.EqualFunc(other, full, condsEqual) {
			p.cq = int32(i)
			break
		}
	}
	if int(p.cq) == len(p.sheet.containerQs) {
		p.sheet.containerQs = append(p.sheet.containerQs, full)
	}
	p.stylesheet(body)
	p.cq = saveCQ
	return end
}

// importRule loads one `@import "file" layer? supports? media?;`
// through the resolver and inlines its rules in place, tracking the
// import chain so a cycle reports precisely instead of looping. The
// layer() qualifier (bare or named, dotted names nest) files the
// imported rules in that cascade layer; supports() evaluates at
// parse time like @supports, and a false import drops with a
// warning.
func (p *parser) importRule(ts []token, i int) int {
	if p.rulesSeen {
		end := i + 1
		for end < len(ts) && ts[end].kind != tkSemi {
			if closer(ts[end].kind) != 0 {
				end = blockEnd(ts, end)
				continue
			}
			end++
		}
		p.warnf("@import ignored: it must precede every rule")
		return end + 1
	}
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			goto found
		case tkLBrace, tkRBrace:
			p.warnf("@import skipped: want `\"file\";`")
			return blockEnd(ts, j)
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnf("@import skipped: want `;`")
	return j
found:
	prelude := trimWS(ts[i+1 : j])
	if len(prelude) == 0 {
		p.warnf("@import skipped: want a target")
		return j + 1
	}
	var ref string
	var rest []token
	t := prelude[0]
	switch {
	case t.kind == tkString:
		ref, rest = t.s, prelude[1:]
	case t.kind == tkFunc && t.s == "url":
		end := blockEnd(prelude, 0)
		inner := trimWS(funcArgs(prelude[:end]))
		ref = strings.Trim(rawText(p.src, inner), "\"'")
		rest = prelude[end:]
	default:
		p.warnf("@import skipped: want a string or url() target")
		return j + 1
	}
	var mqTokens []token
	layerName, hasLayer := "", false
	for _, comp := range components(rest) {
		if len(comp) == 0 {
			continue
		}
		switch {
		case comp[0].kind == tkIdent && strings.EqualFold(comp[0].s, "layer") && len(comp) == 1:
			hasLayer = true
			layerName = p.anonLayer()
		case comp[0].kind == tkFunc && comp[0].s == "layer":
			end := blockEnd(comp, 0)
			layerName = dottedName(trimWS(funcArgs(comp[:end])))
			hasLayer = true
			if layerName == "" {
				layerName = p.anonLayer()
			}
		case comp[0].kind == tkFunc && comp[0].s == "supports":
			end := blockEnd(comp, 0)
			cond := append([]token{{kind: tkLParen}}, trimWS(funcArgs(comp[:end]))...)
			cond = append(cond, token{kind: tkRParen})
			if !evalSupports(cond) {
				p.warnf("@import %s skipped: supports() is false", ref)
				return j + 1
			}
		default:
			mqTokens = append(mqTokens, comp...)
		}
	}
	q, okq := parseMediaQuery(trimWS(mqTokens))
	if !okq {
		p.warnf("@import %s skipped: bad query", ref)
		return j + 1
	}
	resolve := p.resolve
	if resolve == nil {
		resolve = FileImport
	}
	path, content, err := resolve(ref, p.base)
	if err != nil {
		p.warnf("@import %s: %v", ref, err)
		return j + 1
	}
	if path != "" {
		if slices.Contains(p.importStack, path) {
			chain := strings.Join(append(slices.Clone(p.importStack), path), " -> ")
			p.warnf("@import cycle: %s", chain)
			return j + 1
		}
	}
	saveMQ, saveBase, saveLayer := p.mq, p.base, p.layerPath
	p.mq = intersectMedia(p.mq, q)
	p.base = path
	if hasLayer {
		p.layerPath = joinLayer(p.layerPath, layerName)
		p.layerOf(p.layerPath)
	}
	p.importStack = append(p.importStack, path)
	p.rulesSeen = false
	p.stylesheet(tokenize(content))
	p.rulesSeen = false
	p.importStack = p.importStack[:len(p.importStack)-1]
	p.mq, p.base, p.layerPath = saveMQ, saveBase, saveLayer
	return j + 1
}

// atBlock locates an at-rule's `{`, returning its trimmed prelude and
// body tokens and the index past the block.
func (p *parser) atBlock(ts []token, i int, what string) (prelude, body []token, end int, ok bool) {
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			p.warnf("%s skipped: want a block", what)
			return nil, nil, j + 1, false
		case tkLBrace:
			end := blockEnd(ts, j)
			if end > len(ts) || ts[end-1].kind != tkRBrace {
				p.warnf("%s skipped: unterminated block", what)
				return nil, nil, len(ts), false
			}
			return trimWS(ts[i+1 : j]), ts[j+1 : end-1], end, true
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnf("%s skipped: want a block", what)
	return nil, nil, j, false
}

// qualifiedRule parses a selector list and its block starting at ts[i]
// and returns the index past it.
func (p *parser) qualifiedRule(ts []token, i int) int {
	start := i
	for i < len(ts) && ts[i].kind != tkLBrace {
		if ts[i].kind == tkSemi || ts[i].kind == tkRBrace {
			p.warnf("rule skipped: want '{' at %q", badText(rawText(p.src, ts[start:i+1])))
			return i + 1
		}
		if closer(ts[i].kind) != 0 {
			i = blockEnd(ts, i)
			continue
		}
		i++
	}
	if i >= len(ts) {
		p.warnf("rule skipped: want '{' at %q", badText(rawText(p.src, ts[start:])))
		return i
	}
	prelude := trimWS(ts[start:i])
	end := blockEnd(ts, i)
	if end > len(ts) || ts[end-1].kind != tkRBrace {
		p.warnf("rule skipped: unterminated block at %q", badText(rawText(p.src, prelude)))
		return len(ts)
	}
	body := ts[i+1 : end-1]
	p.ruleBody(body, prelude)
	return end
}

// ruleBody parses a rule's body — its declarations plus any nested
// rules and at-rules — under parent, the selector list the
// declarations and nested rules belong to. A nested selector desugars
// its `&`s to the parent list (an absent `&` is the descendant form).
func (p *parser) ruleBody(ts []token, parent []token) {
	parent = trimWS(parent)
	var decls []decl
	start, seg := 0, 0
	i := 0
	for i < len(ts) {
		switch ts[i].kind {
		case tkLBrace:
			decls = append(decls, p.declarationsHead(ts[start:i])...)
			prelude := trimWS(ts[seg:i])
			end := blockEnd(ts, i)
			inner := ts[min(i+1, len(ts)):max(end-1, 0)]
			p.nestedRule(prelude, inner, parent)
			i, start, seg = end, end, end
		case tkFunc, tkLParen, tkLBrack:
			i = blockEnd(ts, i)
		case tkSemi:
			i++
			seg = i
		case tkAt:
			if atSegmentStart(ts, seg, i) {
				decls = append(decls, p.declarationsHead(ts[start:i])...)
				i = p.nestedAt(ts, i, parent)
				start, seg = i, i
			} else {
				i++
			}
		default:
			i++
		}
	}
	decls = append(decls, p.declarations(ts[start:])...)
	if len(decls) == 0 || p.dead > 0 {
		return
	}
	block := &Block{decls: decls}
	starting := p.startBlock
	p.startBlock = nil
	sels, ok := p.selectorList(parent)
	if !ok {
		p.warnf("rule skipped: bad selector %q", badText(rawText(p.src, parent)))
		return
	}
	for _, sel := range sels {
		p.add(sel, block, starting)
	}
}

// selectorList parses a style rule's selector list, scoped to the
// active @scope; one invalid member invalidates the list.
func (p *parser) selectorList(ts []token) ([]selector, bool) {
	if p.scope != nil {
		return parseScopedList(p.src, ts, p.scope.root)
	}
	return parseSelectorList(p.src, ts)
}

// atSegmentStart reports whether the token at i begins a fresh
// statement: nothing but whitespace since the last `;` or block. An
// `@` mid-declaration (GTK's `color: @fg`) is a value, not an at-rule.
func atSegmentStart(ts []token, seg, i int) bool {
	for _, t := range ts[seg:i] {
		if t.kind != tkWS {
			return false
		}
	}
	return true
}

// nestedRule parses one nested `& ... { }` block under parent, the
// enclosing rule's selector list: each member desugars against it, and
// the joined list parses as one (an invalid member drops the nested
// rule alone).
func (p *parser) nestedRule(prelude, inner []token, parent []token) {
	if len(prelude) == 0 {
		p.warnf("rule skipped: want a selector before '{'")
		return
	}
	var combined []token
	for k, raw := range splitTop(prelude, tkComma) {
		if k > 0 {
			combined = append(combined, token{kind: tkComma})
		}
		combined = append(combined, spliceParent(trimWS(raw), parent)...)
	}
	p.ruleBody(inner, combined)
}

// spliceParent desugars n's `&`s to the parent selector list par
// (css-nesting-1 §3): `&` means `:is(par)`, with :is()'s matching and
// specificity, and a selector without one is relative to it, a
// descendant. Where the text substitution means the same — a single
// parent selector standing in for one `&` at n's start — it splices
// par itself, which keeps the rule bucketed by its own subject.
func spliceParent(n, par []token) []token {
	if len(n) == 0 {
		return nil
	}
	ref := par
	if len(splitTop(par, tkComma)) > 1 || !ampLeads(n) {
		at := token{pos: n[0].pos, end: n[0].pos}
		ref = make([]token, 0, len(par)+3)
		ref = append(ref, token{kind: tkColon, pos: at.pos, end: at.end}, token{kind: tkFunc, s: "is", pos: at.pos, end: at.end})
		ref = append(ref, par...)
		ref = append(ref, token{kind: tkRParen, pos: at.pos, end: at.end})
	}
	if !slices.ContainsFunc(n, func(t token) bool { return t.is('&') }) {
		out := make([]token, 0, len(ref)+1+len(n))
		out = append(out, ref...)
		out = append(out, token{kind: tkWS, pos: n[0].pos, end: n[0].pos})
		return append(out, n...)
	}
	out := make([]token, 0, len(n)+len(ref))
	for _, t := range n {
		if t.is('&') {
			out = append(out, ref...)
			continue
		}
		out = append(out, t)
	}
	return out
}

// ampLeads reports whether n's nesting selector stands where the text
// of a single parent selector means what `:is(parent)` does: n holds
// no `&` (the implied descendant form), or exactly one, in its first
// compound.
func ampLeads(n []token) bool {
	amps, first := 0, true
	for i := 0; i < len(n); i++ {
		t := n[i]
		switch {
		case t.is('&'):
			amps++
			if !first {
				return false
			}
		case t.kind == tkWS, t.is('>'), t.is('+'), t.is('~'):
			first = false
		case closer(t.kind) != 0:
			i = blockEnd(n, i) - 1
		}
	}
	return amps <= 1
}

// nestedAt applies a declaration-scoped at-rule inside a rule body —
// @media, @supports and @layer re-run the body's remaining content
// under their condition — and drops the rest with a warning.
func (p *parser) nestedAt(ts []token, i int, parent []token) int {
	prelude, body, end, ok := p.atBlock(ts, i, "at-rule @"+ts[i].s)
	if !ok {
		return end
	}
	switch ts[i].s {
	case "media":
		q, okq := parseMediaQuery(prelude)
		if !okq {
			p.warnf("@media skipped: bad query %q", badText(rawText(p.src, prelude)))
			return end
		}
		save := p.mq
		p.mq = intersectMedia(p.mq, q)
		p.ruleBody(body, parent)
		p.mq = save
	case "supports":
		save := p.dead
		if !evalSupports(prelude) {
			p.dead++
		}
		p.ruleBody(body, parent)
		p.dead = save
	case "layer":
		names := splitTop(prelude, tkComma)
		last := ""
		for _, name := range names {
			n := dottedName(name)
			if n == "" {
				n = p.anonLayer()
			}
			last = n
			p.layerOf(joinLayer(p.layerPath, n))
		}
		if last == "" {
			last = p.anonLayer()
			p.layerOf(joinLayer(p.layerPath, last))
		}
		save := p.layerPath
		p.layerPath = joinLayer(p.layerPath, last)
		p.ruleBody(body, parent)
		p.layerPath = save
	case "starting-style":
		p.startBlock = &Block{decls: p.declarations(body)}
	default:
		p.warnAt(ts[i].s)
	}
	return end
}

// declarationsHead parses the declarations ahead of a nested rule:
// a colon-less entry is the nested selector's start, not a malformed
// declaration, so it drops silently.
func (p *parser) declarationsHead(ts []token) []decl {
	var out []decl
	for _, raw := range splitTop(ts, tkSemi) {
		raw = trimWS(raw)
		if len(raw) == 0 || !hasTopColon(raw) || raw[0].is('&') {
			continue
		}
		if d, ok := p.declaration(raw); ok {
			out = append(out, d)
		}
	}
	return out
}

func hasTopColon(ts []token) bool {
	for i := 0; i < len(ts); i++ {
		if closer(ts[i].kind) != 0 {
			i = blockEnd(ts, i) - 1
			continue
		}
		if ts[i].kind == tkColon {
			return true
		}
	}
	return false
}

// declarations parses a declaration list, dropping malformed entries
// with a warning.
func (p *parser) declarations(ts []token) []decl {
	var out []decl
	for _, raw := range splitTop(ts, tkSemi) {
		raw = trimWS(raw)
		if len(raw) == 0 {
			continue
		}
		d, ok := p.declaration(raw)
		if ok {
			out = append(out, d)
		}
	}
	return out
}

// declaration parses `name: value`.
func (p *parser) declaration(ts []token) (decl, bool) {
	if ts[0].kind != tkIdent {
		p.warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	j := 1
	for j < len(ts) && ts[j].kind == tkWS {
		j++
	}
	if j >= len(ts) || ts[j].kind != tkColon {
		p.warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	name := ts[0].s
	val, imp := takeImportant(colorRefs(trimWS(ts[j+1:])))
	if val == nil {
		p.warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	p.order++
	d := decl{name: name, val: val, hasVar: containsVar(val), wide: wideOf(val), important: imp, order: p.order}
	if strings.HasPrefix(name, "--") {
		d.custom = true
		return d, true
	}
	lname := strings.ToLower(name)
	d.name = lname
	if lname == "content" || lname == "counter-reset" || lname == "counter-increment" {
		d.meta = true
		return d, true
	}
	if lname == "all" {
		if d.wide == wideNone {
			p.warnf("declaration skipped: all: %s", badText(rawText(p.src, val)))
			return decl{}, false
		}
		d.covers = allProps
		return d, true
	}
	def, ok := propTable[lname]
	if !ok {
		if !ignoredProps[lname] {
			p.warnf("declaration skipped: unknown property %q", name)
		}
		return decl{}, false
	}
	d.covers, d.parse = def.covers, def.parse
	if len(val) == 0 {
		p.warnf("declaration skipped: %s has no value", name)
		return decl{}, false
	}
	if d.wide != wideNone || d.hasVar {
		return d, true
	}
	// A var-free value is validated now: CSS drops an invalid
	// declaration at parse time, so it never wins the cascade.
	scratch := initialValues()
	cx := ctx{rem: parseRem, em: parseRem, weight: 400}
	if !d.parse(val, &cx, &scratch) {
		p.warnf("declaration skipped: %s: %s", name, badText(rawText(p.src, val)))
		return decl{}, false
	}
	if cx.cqRead {
		p.sheet.sens.CqUnits = true
	}
	if !cx.dep {
		d.pre = &scratch
		if cx.usedRem {
			d.preRem = parseRem
		}
	}
	return d, true
}

// takeImportant strips a trailing `!important` from a value; a nil
// return marks a malformed `!`.
func takeImportant(ts []token) ([]token, bool) {
	for i := range slices.Backward(ts) {
		if !ts[i].is('!') {
			continue
		}
		j := i + 1
		for j < len(ts) && ts[j].kind == tkWS {
			j++
		}
		if j == len(ts)-1 && ts[j].ident("important") {
			return trimWS(ts[:i]), true
		}
		return nil, false
	}
	return ts, false
}

// parseRem is the rem base declarations are validated (and, when
// node-independent, pre-computed) under: CSS's 16px default.
const parseRem = 16

// wideOf reports a CSS-wide keyword value.
func wideOf(val []token) cssWide {
	if len(val) != 1 || val[0].kind != tkIdent {
		return wideNone
	}
	switch strings.ToLower(val[0].s) {
	case "inherit":
		return wideInherit
	case "initial":
		return wideInitial
	case "unset":
		return wideUnset
	case "revert":
		return wideRevert
	case "revert-layer":
		return wideRevertLayer
	}
	return wideNone
}

// containsVar reports a var() or attr() call anywhere in the value:
// both resolve per node, so the declaration parses after substitution
// instead of at parse time.
func containsVar(ts []token) bool {
	for _, t := range ts {
		if t.kind == tkFunc && (t.s == "var" || t.s == "attr") {
			return true
		}
	}
	return false
}

// add files one compiled rule into the index and folds its selector
// into the sensitivity summary, carrying the parser's active @media
// condition, cascade layer, @container condition, @scope rule, and
// @starting-style block.
func (p *parser) add(sel selector, block, start *Block) {
	if p.dead > 0 {
		return
	}
	s := &p.sheet
	idx := intconv.Int32(len(s.rules))
	if p.scope != nil {
		sel.upper = p.scope.upper
	}
	r := rule{
		sel: sel, spec: sel.spec(), block: block, mq: -1, cq: -1, layer: p.curLayer(),
		scope:  p.scope,
		pseudo: sel.parts[len(sel.parts)-1].pseudo,
		start:  start,
	}
	if p.cq >= 0 {
		r.cq = p.cq
		s.sens.Container = true
	}
	if p.mq != nil {
		r.mq = intconv.Int32(len(s.mediaQs))
		for i, q := range s.mediaQs {
			if queryEqual(q, p.mq) {
				r.mq = int32(i)
				break
			}
		}
		if int(r.mq) == len(s.mediaQs) {
			s.mediaQs = append(s.mediaQs, p.mq)
		}
	}
	s.rules = append(s.rules, r)
	if r.pseudo != "" {
		s.sens.Pseudo = true
	}
	for i := range sel.parts {
		part := &sel.parts[i]
		if part.structs != 0 || len(part.nths) > 0 {
			s.sens.Structural = true
		}
		for _, n := range part.nths {
			if n.of != nil {
				s.sens.SiblingFacts = true
			}
		}
		s.noteCompound(part, i == len(sel.parts)-1)
	}
	for _, c := range sel.combs {
		if c == '+' || c == '~' {
			s.sens.Structural = true
			s.sens.SiblingFacts = true
		}
	}
	for i := range block.decls {
		if d := &block.decls[i]; d.meta {
			if d.name == "counter-reset" || d.name == "counter-increment" || counterContent(d.val) {
				s.sens.Counters = true
			}
			if d.wide == wideRevertLayer || d.wide == wideRevert {
				s.sens.Revert = true
			}
		} else if !d.custom && (d.wide == wideRevertLayer || d.wide == wideRevert) {
			s.sens.Revert = true
		}
	}
	right := &sel.parts[len(sel.parts)-1]
	switch {
	case right.id != "":
		if s.byID == nil {
			s.byID = map[string][]int32{}
		}
		s.byID[right.id] = append(s.byID[right.id], idx)
	case len(right.classes) > 0:
		if s.byClass == nil {
			s.byClass = map[string][]int32{}
		}
		s.byClass[right.classes[0]] = append(s.byClass[right.classes[0]], idx)
	case right.element != "":
		if s.byElement == nil {
			s.byElement = map[string][]int32{}
		}
		s.byElement[right.element] = append(s.byElement[right.element], idx)
	default:
		s.universal = append(s.universal, idx)
	}
}

// noteCompound records the facts one compound tests: its own facts
// for a non-rightmost compound (the ancestor sensitivity), and any
// :has() anywhere (a descendant's facts feed this node's match).
func (s *Sheet) noteCompound(c *compound, rightmost bool) {
	if len(c.has) > 0 {
		s.sens.HasRelational = true
	}
	if rightmost {
		s.noteNested(c)
		return
	}
	s.noteAncestor(c)
}

// counterContent reports a content value reading counters.
func counterContent(ts []token) bool {
	for _, t := range ts {
		if t.kind == tkFunc && (t.s == "counter" || t.s == "counters") {
			return true
		}
	}
	return false
}

// noteAncestor records the facts a non-rightmost compound tests.
func (s *Sheet) noteAncestor(c *compound) {
	s.sens.AncestorStates |= c.state
	for _, cl := range c.classes {
		if s.sens.AncestorClasses == nil {
			s.sens.AncestorClasses = map[string]bool{}
		}
		s.sens.AncestorClasses[cl] = true
	}
	c.eachNested(func(sel *selector) {
		for k := range sel.parts {
			s.noteAncestor(&sel.parts[k])
		}
	})
}

// noteNested records the ancestor facts inside a rightmost compound's
// functional pseudo-classes: an argument's non-rightmost compounds
// test the node's ancestors (`:is(.a > .b)` reads its parent's
// class), and its rightmost compound the node itself, recursively.
func (s *Sheet) noteNested(c *compound) {
	c.eachNested(func(sel *selector) {
		last := len(sel.parts) - 1
		for k := range sel.parts[:last] {
			s.noteAncestor(&sel.parts[k])
		}
		s.noteNested(&sel.parts[last])
	})
}

// badText truncates a source excerpt for a warning message.
func badText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}
