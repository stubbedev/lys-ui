package style

import (
	"slices"
	"strconv"
	"strings"
)

// FontFace is one @font-face rule: an app font a stylesheet declares,
// loaded from its src and matched by font-family and font-weight. The
// widget layer loads the file and serves it to the font-family
// resolver; format hints outside the truetype/opentype family warn
// and drop that src.
type FontFace struct {
	// Family is the font-family descriptor, the name a font-family
	// declaration matches.
	Family string
	// Src is the resolved url() path of the first loadable src entry.
	Src string
	// Format is that src's format() hint, empty when none was given.
	Format string
	// WeightLo and WeightHi are the font-weight descriptor's range
	// (equal for a single weight); 400 when unset.
	WeightLo, WeightHi int
	// Italic is the font-style descriptor: italic or oblique.
	Italic bool
}

// knownFontFormats are the format() hints the loader can honor: the
// go-text parser reads plain truetype and opentype outlines.
var knownFontFormats = map[string]bool{
	"": true, "truetype": true, "opentype": true,
}

// fontFaceRule parses one @font-face: a block of descriptors, of
// which font-family, src, font-weight and font-style are read and
// font-stretch is accepted and unused. A face without a family or a
// loadable src warns and drops; the cycle and read errors of @import
// apply to src resolution the same way.
func (p *parser) fontFaceRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@font-face")
	if !ok {
		return end
	}
	if len(prelude) > 0 {
		p.warnf("@font-face skipped: want a block")
		return end
	}
	if p.dead > 0 {
		return end
	}
	f := FontFace{WeightLo: 400, WeightHi: 400}
	var srcs []token
	for _, raw := range splitTop(body, tkSemi) {
		raw = trimWS(raw)
		if len(raw) < 2 || raw[0].kind != tkIdent {
			continue
		}
		name := strings.ToLower(raw[0].s)
		j := 1
		for j < len(raw) && raw[j].kind == tkWS {
			j++
		}
		if j >= len(raw) || raw[j].kind != tkColon {
			continue
		}
		val := trimWS(raw[j+1:])
		switch name {
		case "font-family":
			fam, okf := familyName(val)
			if !okf {
				p.warnf("@font-face skipped: bad font-family")
				return end
			}
			f.Family = fam
		case "src":
			srcs = append(srcs, val...)
		case "font-weight":
			lo, hi, okw := weightRange(val)
			if !okw {
				p.warnf("@font-face skipped: bad font-weight")
				return end
			}
			f.WeightLo, f.WeightHi = lo, hi
		case "font-style":
			if len(val) != 1 || val[0].kind != tkIdent {
				p.warnf("@font-face skipped: bad font-style")
				return end
			}
			switch strings.ToLower(val[0].s) {
			case "normal":
			case "italic", "oblique":
				f.Italic = true
			default:
				p.warnf("@font-face skipped: bad font-style")
				return end
			}
		case "font-stretch", "font-display", "unicode-range", "font-feature-settings", "font-variation-settings":
			// Accepted and unused: the loader has no matching knob.
		default:
			p.warnf("@font-face descriptor skipped: unknown %q", raw[0].s)
		}
	}
	if f.Family == "" {
		p.warnf("@font-face skipped: want a font-family")
		return end
	}
	for _, part := range splitTop(srcs, tkComma) {
		part = trimWS(part)
		if len(part) == 0 || part[0].kind != tkFunc || part[0].s != "url" {
			continue
		}
		urlEnd := blockEnd(part, 0)
		inner := trimWS(funcArgs(part[:urlEnd]))
		if len(inner) == 0 {
			continue
		}
		ref := ""
		if len(inner) == 1 && inner[0].kind == tkString {
			ref = inner[0].s
		} else {
			ref = strings.Trim(rawText(p.src, inner), "\"'")
		}
		format := ""
		if rest := trimWS(part[urlEnd:]); len(rest) > 0 && rest[0].kind == tkFunc && rest[0].s == "format" {
			fEnd := blockEnd(rest, 0)
			if args := trimWS(funcArgs(rest[:fEnd])); len(args) == 1 && args[0].kind == tkString {
				format = strings.ToLower(args[0].s)
			}
		}
		if !knownFontFormats[format] {
			p.warnf("@font-face src skipped: format %q is not supported", format)
			continue
		}
		path := ref
		if p.resolve != nil || p.base != "" {
			resolve := p.resolve
			if resolve == nil {
				resolve = FileImport
			}
			rp, _, err := resolve(ref, p.base)
			if err != nil {
				p.warnf("@font-face %s: src %s: %v", f.Family, ref, err)
				continue
			}
			path = rp
		}
		f.Src, f.Format = path, format
		break
	}
	if f.Src == "" {
		p.warnf("@font-face %s skipped: want a loadable src", f.Family)
		return end
	}
	p.sheet.fontFaces = append(p.sheet.fontFaces, &f)
	return end
}

// familyName reads a font-family descriptor: one quoted string or a
// whitespace-joined run of identifiers.
func familyName(ts []token) (string, bool) {
	if len(ts) == 1 && ts[0].kind == tkString {
		return ts[0].s, true
	}
	var parts []string
	for _, t := range ts {
		if t.kind == tkWS {
			continue
		}
		if t.kind != tkIdent {
			return "", false
		}
		parts = append(parts, t.s)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

// weightRange parses the font-weight descriptor: a keyword, a number,
// or a two-number range.
func weightRange(ts []token) (int, int, bool) {
	comps := components(ts)
	switch {
	case len(comps) == 1 && len(comps[0]) == 1 && comps[0][0].kind == tkIdent:
		switch strings.ToLower(comps[0][0].s) {
		case "normal":
			return 400, 400, true
		case "bold":
			return 700, 700, true
		}
		return 0, 0, false
	case len(comps) == 1 && len(comps[0]) == 1 && comps[0][0].kind == tkNumber:
		return int(comps[0][0].num), int(comps[0][0].num), true
	case len(comps) == 2 && len(comps[0]) == 1 && len(comps[1]) == 1 &&
		comps[0][0].kind == tkNumber && comps[1][0].kind == tkNumber:
		return int(comps[0][0].num), int(comps[1][0].num), true
	}
	return 0, 0, false
}

// PropertyReg is one @property registration: a typed custom property
// with its syntax string, inheritance control, and initial value.
// Registration fixes var() resolution for the name everywhere the
// sheet applies; animating a registered property is deferred — the
// transition engine keys on the longhand table, which custom
// properties sit outside.
type PropertyReg struct {
	// Name is the custom property name, leading dashes included.
	Name string
	// Syntax is the descriptor's syntax string ("<length>", "*",
	// "<length> | <percentage>", ...).
	Syntax string
	// Inherits is the inherits descriptor: false makes the property
	// resolve to its initial value on nodes that do not declare it,
	// however their ancestors set it.
	Inherits bool
	// Initial is the initial-value descriptor's tokens, nil for a
	// universal syntax without one (then the name resolves to
	// nothing, like an unregistered custom property).
	Initial []token
}

// knownSyntaxes are the syntax strings whose initial values the
// engine can validate.
var knownSyntaxes = map[string]bool{
	"<length>": true, "<percentage>": true, "<length-percentage>": true,
	"<number>": true, "<integer>": true, "<color>": true, "<angle>": true,
	"<time>": true, "<resolution>": true, "<image>": true, "<url>": true,
	"<custom-ident>": true, "<string>": true,
}

// propertyRule parses one @property block: the custom property name
// in the prelude, then the syntax, inherits and initial-value
// descriptors. A malformed registration warns and drops.
func (p *parser) propertyRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@property")
	if !ok {
		return end
	}
	name := rawName(trimWS(prelude))
	if name == "" || !strings.HasPrefix(name, "--") {
		p.warnf("@property skipped: want a --name")
		return end
	}
	reg := PropertyReg{Name: name, Syntax: "*"}
	sawSyntax := false
	for _, raw := range splitTop(body, tkSemi) {
		raw = trimWS(raw)
		if len(raw) < 2 || raw[0].kind != tkIdent {
			continue
		}
		descriptor := strings.ToLower(raw[0].s)
		j := 1
		for j < len(raw) && raw[j].kind == tkWS {
			j++
		}
		if j >= len(raw) || raw[j].kind != tkColon {
			continue
		}
		val := trimWS(raw[j+1:])
		switch descriptor {
		case "syntax":
			if len(val) != 1 || val[0].kind != tkString {
				p.warnf("@property %s skipped: bad syntax", name)
				return end
			}
			if !validSyntax(val[0].s) {
				p.warnf("@property %s skipped: unknown syntax %q", name, val[0].s)
				return end
			}
			reg.Syntax, sawSyntax = val[0].s, true
		case "inherits":
			if len(val) != 1 || val[0].kind != tkIdent {
				p.warnf("@property %s skipped: bad inherits", name)
				return end
			}
			switch strings.ToLower(val[0].s) {
			case "true":
				reg.Inherits = true
			case "false":
				reg.Inherits = false
			default:
				p.warnf("@property %s skipped: bad inherits", name)
				return end
			}
		case "initial-value":
			reg.Initial = val
		default:
			p.warnf("@property descriptor skipped: unknown %q", raw[0].s)
		}
	}
	if !sawSyntax {
		p.warnf("@property %s skipped: want a syntax", name)
		return end
	}
	if reg.Initial != nil && !validInitial(reg.Syntax, reg.Initial) {
		p.warnf("@property %s skipped: initial-value does not match %q", name, reg.Syntax)
		return end
	}
	if reg.Initial == nil && reg.Syntax != "*" {
		p.warnf("@property %s skipped: want an initial-value", name)
		return end
	}
	if p.dead == 0 {
		if p.sheet.properties == nil {
			p.sheet.properties = map[string]*PropertyReg{}
		}
		p.sheet.properties[name] = &reg
	}
	return end
}

// validSyntax checks a syntax descriptor: "*", or a pipe list of
// known type names (a trailing "+" multi-id flag is accepted and
// recorded only through validation).
func validSyntax(s string) bool {
	if s == "*" {
		return true
	}
	for part := range strings.SplitSeq(s, "|") {
		part = strings.TrimSuffix(strings.TrimSpace(part), "+")
		if !knownSyntaxes[part] {
			return false
		}
	}
	return true
}

// validInitial checks an initial value against its registration's
// syntax: every component must match one of the pipe alternatives
// (a trailing + accepts a space-separated list of them).
func validInitial(syntax string, ts []token) bool {
	if syntax == "*" {
		return true
	}
	cx := ctx{rem: parseRem, em: parseRem}
	comps := components(ts)
	if len(comps) == 0 {
		return false
	}
	for alt := range strings.SplitSeq(syntax, "|") {
		alt = strings.TrimSpace(alt)
		multi := strings.HasSuffix(alt, "+")
		name := strings.TrimSuffix(alt, "+")
		if !multi && len(comps) != 1 {
			continue
		}
		ok := true
		for _, c := range comps {
			if !initialComponentMatches(name, c, &cx) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// initialComponentMatches checks one initial-value component
// against one syntax type name.
func initialComponentMatches(name string, c []token, cx *ctx) bool {
	switch name {
	case "<length>", "<percentage>", "<length-percentage>", "<number>", "<integer>", "<angle>", "<time>", "<resolution>":
		n, ok := evalNumeric(c, cx)
		if !ok {
			return false
		}
		switch name {
		case "<length>":
			return n.kind == numLength
		case "<percentage>":
			return n.kind == numPercent
		case "<length-percentage>":
			return n.kind == numLength || n.kind == numPercent
		case "<integer>":
			return n.kind == numNumber && n.v == float64(int(n.v))
		case "<angle>":
			return n.kind == numAngle
		case "<time>":
			return n.kind == numTime
		default:
			return n.kind == numNumber
		}
	case "<color>":
		_, ok := parseColor(c, cx)
		return ok
	case "<string>":
		return len(c) == 1 && c[0].kind == tkString
	case "<url>":
		return len(c) == 1 && c[0].kind == tkFunc && c[0].s == "url"
	case "<custom-ident>":
		return len(c) == 1 && c[0].kind == tkIdent
	case "<image>":
		return len(c) == 1 && c[0].kind == tkFunc
	}
	return false
}

// scopeCtx is one @scope rule: its scoping root selector list (a
// nested @scope's roots are themselves scoped to the outer one) and
// its scoping limits.
type scopeCtx struct {
	root, upper []selector
}

// scopeRule parses one @scope: a root selector list in parens, an
// optional `to (upper)` limit, and a block of rules whose selectors
// are scoped (css-cascade-6 §2.5) — each subject must be the root or
// descend from it without crossing a limit, `:scope` in a selector
// names that root, and the root adds no specificity. The cascade
// ranks a scoped rule by its scoping proximity below specificity: of
// two equally specific rules, the one whose root is fewer generations
// up wins.
func (p *parser) scopeRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@scope")
	if !ok {
		return end
	}
	rootTs, rest, okr := scopePrelude(prelude)
	if !okr {
		p.warnf("@scope skipped: want a (root) selector")
		return end
	}
	root, okr := p.selectorList(rootTs)
	if !okr {
		p.warnf("@scope skipped: bad root selector")
		return end
	}
	var upper []selector
	if len(rest) > 0 {
		if !slices.ContainsFunc(rest, func(t token) bool { return t.ident("to") }) {
			p.warnf("@scope skipped: want `to (upper)`")
			return end
		}
		up, oku := scopePreludeRest(rest)
		if !oku {
			p.warnf("@scope skipped: bad upper bound")
			return end
		}
		upper, oku = parseSelectorList(p.src, up)
		if !oku {
			p.warnf("@scope skipped: bad upper selector")
			return end
		}
	}
	save := p.scope
	p.scope = &scopeCtx{root: root, upper: upper}
	p.stylesheet(body)
	p.scope = save
	return end
}

// scopePrelude splits a @scope prelude into the root selector's
// tokens and the remainder after it.
func scopePrelude(ts []token) (root, rest []token, ok bool) {
	if len(ts) == 0 || ts[0].kind != tkLParen {
		return nil, nil, false
	}
	end := blockEnd(ts, 0)
	if end > len(ts) {
		return nil, nil, false
	}
	return trimWS(funcArgs(ts[0:end])), trimWS(ts[end:]), true
}

// scopePreludeRest reads the `to (upper)` clause after the root.
func scopePreludeRest(ts []token) ([]token, bool) {
	for i, t := range ts {
		if !t.ident("to") {
			continue
		}
		j := i + 1
		for j < len(ts) && ts[j].kind == tkWS {
			j++
		}
		if j >= len(ts) || ts[j].kind != tkLParen {
			return nil, false
		}
		end := blockEnd(ts, j)
		if end > len(ts) {
			return nil, false
		}
		return trimWS(funcArgs(ts[j:end])), true
	}
	return nil, false
}

// layerTree is one node of the cascade layer name tree: dotted names
// nest, and a subtree sorts before its parent (the parent outranks
// its nested layers).
type layerTree struct {
	kids []*layerTree
	name string
	idx  int
}

// orderLayers computes each registered layer's cascade rank from the
// first-appearance name tree: nested layers are weaker than their
// parent, later siblings stronger than earlier ones, and unlayered
// (255) outranks everything.
func (s *Sheet) orderLayers() {
	if len(s.layerNames) == 0 {
		return
	}
	root := &layerTree{idx: -1}
	for i, path := range s.layerNames {
		n := root
		for part := range strings.SplitSeq(path, ".") {
			found := false
			for _, k := range n.kids {
				if k.name == part {
					n = k
					found = true
					break
				}
			}
			if !found {
				k := &layerTree{name: part, idx: -1}
				n.kids = append(n.kids, k)
				n = k
			}
		}
		n.idx = i
	}
	s.layerRanks = make([]uint8, len(s.layerNames))
	var next uint8
	var emit func(n *layerTree)
	emit = func(n *layerTree) {
		for _, k := range n.kids {
			emit(k)
		}
		if n.idx >= 0 && next < 254 {
			s.layerRanks[n.idx] = next
			next++
		}
	}
	emit(root)
}

// anonLayer names a fresh anonymous layer: the NUL byte cannot appear
// in a dotted name, so anonymous layers never collide with named
// ones.
func (p *parser) anonLayer() string {
	p.anonSeq++
	return "\x00" + strconv.Itoa(p.anonSeq)
}

// dottedName joins a dotted layer name's tokens; "" when malformed.
func dottedName(ts []token) string {
	var b strings.Builder
	for i, t := range ts {
		if t.kind == tkWS {
			continue
		}
		if t.kind == tkDelim && t.s == "." {
			if i+1 >= len(ts) {
				return ""
			}
			b.WriteByte('.')
			continue
		}
		if t.kind != tkIdent {
			return ""
		}
		if b.Len() > 0 && !strings.HasSuffix(b.String(), ".") {
			return ""
		}
		b.WriteString(strings.ToLower(t.s))
	}
	s := b.String()
	if s == "" || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") || strings.Contains(s, "..") {
		return ""
	}
	return s
}

// joinLayer nests name under path.
func joinLayer(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
