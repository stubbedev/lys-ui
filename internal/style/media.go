package style

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Media is the environment @media conditions evaluate against: the
// viewport's size and density, and the appearance preferences the
// theme carries (dark polarity, high contrast, motion). The widget
// layer fills it per compute: the viewport from SetMediaViewport, the
// preferences derived from the active Theme (IsDark, Animations) and
// SetMediaContrast, so a theme swap re-evaluates prefers-* rules.
type Media struct {
	Width, Height float64
	DPI           float64
	Dark          bool
	HighContrast  bool
	ReducedMotion bool
}

type mediaFeat uint8

const (
	featWidth mediaFeat = iota + 1
	featHeight
	featAspect
	featOrientation
	featResolution
	featScheme
	featContrast
	featMotion
)

type mediaCmp uint8

const (
	cmpKeyword mediaCmp = iota
	cmpLT
	cmpLE
	cmpEQ
	cmpGE
	cmpGT
)

type mediaCond struct {
	feat mediaFeat
	cmp  mediaCmp
	// a is the compared value, or the keyword code: orientation 1
	// landscape 2 portrait, scheme 1 dark 2 light, contrast and motion
	// 1 the preference 2 no-preference.
	a float64
}

func (c mediaCond) match(m *Media) bool {
	if c.cmp == cmpKeyword {
		var want, have bool
		switch c.feat {
		case featOrientation:
			want, have = c.a == 1, m.Width >= m.Height
		case featScheme:
			want, have = c.a == 1, m.Dark
		case featContrast:
			want, have = c.a == 1, m.HighContrast
		case featMotion:
			want, have = c.a == 1, m.ReducedMotion
		}
		return want == have
	}
	var v float64
	switch c.feat {
	case featWidth:
		v = m.Width
	case featHeight:
		v = m.Height
	case featAspect:
		if m.Height <= 0 {
			return false
		}
		v = m.Width / m.Height
	case featResolution:
		v = m.DPI
	default:
		return false
	}
	switch c.cmp {
	case cmpLT:
		return v < c.a
	case cmpLE:
		return v <= c.a
	case cmpEQ:
		return v == c.a
	case cmpGE:
		return v >= c.a
	case cmpGT:
		return v > c.a
	}
	return false
}

// tri is a media query's three-valued (Kleene) truth value: a
// feature the engine does not know, or a <general-enclosed> form,
// evaluates unknown, which `not` keeps unknown and a whole query
// reads as false (mediaqueries-4 §3).
type tri int8

const (
	triFalse tri = iota
	triUnknown
	triTrue
)

func triOf(b bool) tri {
	if b {
		return triTrue
	}
	return triFalse
}

func (t tri) not() tri { return triTrue - t }

// mqOp is one node kind of a media condition tree.
type mqOp uint8

const (
	// mqConst is a fixed value: a media type, or an unknown form.
	mqConst mqOp = iota
	// mqFeature is one feature comparison.
	mqFeature
	mqAnd
	mqOr
	mqNot
)

// mqExpr is one node of a media (or container) condition tree.
type mqExpr struct {
	op   mqOp
	val  tri
	cond mediaCond
	kids []mqExpr
}

// mediaQuery is one @media prelude: its comma-separated queries, any
// of which matching makes the condition true. nil matches everything.
type mediaQuery []mqExpr

// eval evaluates the tree against the media m, or — when cont is not
// nil — against a query container, whose block-axis features read
// unknown unless it is a size container.
func (e *mqExpr) eval(m *Media, cont *Container) tri {
	switch e.op {
	case mqConst:
		return e.val
	case mqFeature:
		if cont == nil {
			return triOf(e.cond.match(m))
		}
		if blockFeats[e.cond.feat] && !cont.Size {
			return triUnknown
		}
		return triOf(e.cond.matchContainer(*cont))
	case mqNot:
		return e.kids[0].eval(m, cont).not()
	case mqAnd:
		out := triTrue
		for i := range e.kids {
			out = min(out, e.kids[i].eval(m, cont))
		}
		return out
	case mqOr:
		out := triFalse
		for i := range e.kids {
			out = max(out, e.kids[i].eval(m, cont))
		}
		return out
	}
	return triFalse
}

// usesBlock reports whether the tree queries a block-axis feature,
// which only a size container can answer.
func (e *mqExpr) usesBlock() bool {
	if e.op == mqFeature {
		return blockFeats[e.cond.feat]
	}
	return slices.ContainsFunc(e.kids, func(k mqExpr) bool { return k.usesBlock() })
}

func mediaMatches(q mediaQuery, m *Media) bool {
	for i := range q {
		if q[i].eval(m, nil) == triTrue {
			return true
		}
	}
	return len(q) == 0
}

// exprEqual compares two condition trees.
func exprEqual(a, b *mqExpr) bool {
	if a.op != b.op || a.val != b.val || a.cond != b.cond || len(a.kids) != len(b.kids) {
		return false
	}
	for i := range a.kids {
		if !exprEqual(&a.kids[i], &b.kids[i]) {
			return false
		}
	}
	return true
}

func queryEqual(a, b mediaQuery) bool {
	return slices.EqualFunc(a, b, func(x, y mqExpr) bool { return exprEqual(&x, &y) })
}

// intersectMedia ands two queries by crossing their queries; nil is
// the always-true identity.
func intersectMedia(a, b mediaQuery) mediaQuery {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := make(mediaQuery, 0, len(a)*len(b))
	for _, x := range a {
		for _, y := range b {
			out = append(out, mqExpr{op: mqAnd, kids: []mqExpr{x, y}})
		}
	}
	return out
}

// parseMediaQuery parses an @media prelude (mediaqueries-4 §3): a
// comma list of queries, each a <media-condition> or a media type
// with an optional `not`/`only` prefix and an `and`-joined condition.
// A nil query is always true; ok is false when the prelude is
// malformed.
func parseMediaQuery(ts []token) (mediaQuery, bool) {
	if len(ts) == 0 {
		return nil, true
	}
	var out mediaQuery
	for _, raw := range splitTop(ts, tkComma) {
		e, ok := parseOneQuery(trimWS(raw))
		if !ok {
			return nil, false
		}
		out = append(out, e)
	}
	return out, true
}

// parseOneQuery parses one comma member.
func parseOneQuery(ts []token) (mqExpr, bool) {
	if len(ts) == 0 {
		return mqExpr{}, false
	}
	if ts[0].kind != tkIdent || ts[0].ident("not") && !typeFollows(ts) {
		return parseCondition(ts, true)
	}
	neg := false
	i := 0
	switch {
	case ts[0].ident("not"):
		neg, i = true, 1
	case ts[0].ident("only"):
		i = 1
	}
	for i < len(ts) && ts[i].kind == tkWS {
		i++
	}
	if i >= len(ts) || ts[i].kind != tkIdent {
		return mqExpr{}, false
	}
	e := mqExpr{op: mqConst, val: mediaType(ts[i].s)}
	rest := trimWS(ts[i+1:])
	if len(rest) > 0 {
		if !rest[0].ident("and") {
			return mqExpr{}, false
		}
		cond, ok := parseCondition(trimWS(rest[1:]), false)
		if !ok {
			return mqExpr{}, false
		}
		e = mqExpr{op: mqAnd, kids: []mqExpr{e, cond}}
	}
	if neg {
		e = mqExpr{op: mqNot, kids: []mqExpr{e}}
	}
	return e, true
}

// typeFollows reports whether a leading `not` negates a media type
// (`not print`) rather than a condition (`not (width > 1px)`).
func typeFollows(ts []token) bool {
	for _, t := range ts[1:] {
		if t.kind != tkWS {
			return t.kind == tkIdent
		}
	}
	return false
}

// mediaType is a media type's value: screen and all match, the other
// types (print, speech, ...) and unknown ones do not.
func mediaType(name string) tri {
	switch strings.ToLower(name) {
	case "screen", "all":
		return triTrue
	}
	return triFalse
}

// parseCondition parses a <media-condition>: `not` and one
// parenthesized term, or terms joined all by `and` or (when allowOr)
// all by `or`.
func parseCondition(ts []token, allowOr bool) (mqExpr, bool) {
	ts = trimWS(ts)
	if len(ts) == 0 {
		return mqExpr{}, false
	}
	if ts[0].ident("not") {
		term, rest, ok := parseInParens(trimWS(ts[1:]))
		if !ok || len(trimWS(rest)) > 0 {
			return mqExpr{}, false
		}
		return mqExpr{op: mqNot, kids: []mqExpr{term}}, true
	}
	var kids []mqExpr
	op := mqConst
	for {
		term, rest, ok := parseInParens(ts)
		if !ok {
			return mqExpr{}, false
		}
		kids = append(kids, term)
		rest = trimWS(rest)
		if len(rest) == 0 {
			break
		}
		next := mqAnd
		switch {
		case rest[0].ident("and"):
		case rest[0].ident("or") && allowOr:
			next = mqOr
		default:
			return mqExpr{}, false
		}
		if op != mqConst && op != next {
			return mqExpr{}, false
		}
		op = next
		ts = trimWS(rest[1:])
	}
	if len(kids) == 1 {
		return kids[0], true
	}
	return mqExpr{op: op, kids: kids}, true
}

// parseInParens parses one <media-in-parens> at the start of ts and
// returns the tokens after it: a nested condition, a feature, or a
// <general-enclosed> form (unknown).
func parseInParens(ts []token) (mqExpr, []token, bool) {
	if len(ts) == 0 {
		return mqExpr{}, nil, false
	}
	if ts[0].kind == tkFunc {
		end := blockEnd(ts, 0)
		return mqExpr{op: mqConst, val: triUnknown}, ts[end:], true
	}
	if ts[0].kind != tkLParen {
		return mqExpr{}, nil, false
	}
	end := blockEnd(ts, 0)
	if end > len(ts) {
		return mqExpr{}, nil, false
	}
	inner := trimWS(ts[1 : end-1])
	if len(inner) > 0 && (inner[0].kind == tkLParen || inner[0].ident("not")) {
		e, ok := parseCondition(inner, true)
		return e, ts[end:], ok
	}
	conds, ok, known := parseMediaGroup(inner)
	if !ok {
		return mqExpr{}, nil, false
	}
	if !known {
		return mqExpr{op: mqConst, val: triUnknown}, ts[end:], true
	}
	if len(conds) == 1 {
		return mqExpr{op: mqFeature, cond: conds[0]}, ts[end:], true
	}
	kids := make([]mqExpr, len(conds))
	for i, c := range conds {
		kids[i] = mqExpr{op: mqFeature, cond: c}
	}
	return mqExpr{op: mqAnd, kids: kids}, ts[end:], true
}

// parseMediaGroup parses one parenthesized condition. ok is false
// when the condition is malformed; known is false for a feature the
// engine does not understand, which evaluates false.
func parseMediaGroup(ts []token) (conds []mediaCond, ok, known bool) {
	if len(ts) == 0 {
		return nil, false, true
	}
	if len(ts) >= 2 && ts[0].kind == tkIdent && ts[1].kind == tkColon {
		return parseMediaFeature(strings.ToLower(ts[0].s), trimWS(ts[2:]))
	}
	return parseMediaRange(ts)
}

// parseMediaFeature parses `name: value`.
func parseMediaFeature(name string, val []token) ([]mediaCond, bool, bool) {
	minmax := byte(0)
	switch {
	case strings.HasPrefix(name, "min-"):
		minmax, name = 'm', name[4:]
	case strings.HasPrefix(name, "max-"):
		minmax, name = 'x', name[4:]
	}
	switch name {
	case "width", "height":
		feat := featWidth
		if name == "height" {
			feat = featHeight
		}
		v, ok := mediaLength(val)
		if !ok {
			return nil, false, true
		}
		return rangeConds(feat, minmax, v), true, true
	case "aspect-ratio":
		v, ok := mediaRatio(val)
		if !ok {
			return nil, false, true
		}
		return rangeConds(featAspect, minmax, v), true, true
	case "resolution":
		v, ok := mediaResolution(val)
		if !ok {
			return nil, false, true
		}
		return rangeConds(featResolution, minmax, v), true, true
	case "orientation":
		if len(val) != 1 || val[0].kind != tkIdent {
			return nil, false, true
		}
		switch strings.ToLower(val[0].s) {
		case "landscape":
			return []mediaCond{{feat: featOrientation, a: 1}}, true, true
		case "portrait":
			return []mediaCond{{feat: featOrientation, a: 2}}, true, true
		}
		return nil, false, true
	case "prefers-color-scheme":
		if len(val) != 1 || val[0].kind != tkIdent {
			return nil, false, true
		}
		switch strings.ToLower(val[0].s) {
		case "dark":
			return []mediaCond{{feat: featScheme, a: 1}}, true, true
		case "light":
			return []mediaCond{{feat: featScheme, a: 2}}, true, true
		}
		return nil, false, true
	case "prefers-contrast":
		if len(val) != 1 || val[0].kind != tkIdent {
			return nil, false, true
		}
		switch strings.ToLower(val[0].s) {
		case "more", "high", "custom":
			return []mediaCond{{feat: featContrast, a: 1}}, true, true
		case "no-preference":
			return []mediaCond{{feat: featContrast, a: 2}}, true, true
		}
		return nil, false, true
	case "prefers-reduced-motion":
		if len(val) != 1 || val[0].kind != tkIdent {
			return nil, false, true
		}
		switch strings.ToLower(val[0].s) {
		case "reduce":
			return []mediaCond{{feat: featMotion, a: 1}}, true, true
		case "no-preference":
			return []mediaCond{{feat: featMotion, a: 2}}, true, true
		}
		return nil, false, true
	}
	return nil, true, false
}

// rangeConds maps a min-/max- prefix onto >= / <= comparisons.
func rangeConds(feat mediaFeat, minmax byte, v float64) []mediaCond {
	switch minmax {
	case 'm':
		return []mediaCond{{feat: feat, cmp: cmpGE, a: v}}
	case 'x':
		return []mediaCond{{feat: feat, cmp: cmpLE, a: v}}
	}
	return []mediaCond{{feat: feat, cmp: cmpEQ, a: v}}
}

// parseMediaRange parses Level 4 range syntax: `name op value`,
// `value op name`, or `value op name op value`.
func parseMediaRange(ts []token) ([]mediaCond, bool, bool) {
	comps, okc := rangeComponents(ts)
	if !okc || (len(comps) != 2 && len(comps) != 3) {
		return nil, false, true
	}
	nameAt := -1
	for i, c := range comps {
		if c.t.kind == tkIdent {
			if nameAt >= 0 {
				return nil, false, true
			}
			nameAt = i
		}
	}
	if nameAt < 0 {
		return nil, false, true
	}
	feat, known := rangeFeature(strings.ToLower(comps[nameAt].t.s))
	if !known {
		return nil, true, false
	}
	value := func(i int) (float64, bool) {
		switch feat {
		case featAspect:
			return mediaRatio([]token{comps[i].t})
		case featResolution:
			return mediaResolution([]token{comps[i].t})
		default:
			return mediaLength([]token{comps[i].t})
		}
	}
	cond := func(op mediaCmp, flip bool, vi int) (mediaCond, bool) {
		if op == 0 {
			return mediaCond{}, false
		}
		if flip {
			op = flipCmp(op)
		}
		v, ok := value(vi)
		if !ok {
			return mediaCond{}, false
		}
		return mediaCond{feat: feat, cmp: op, a: v}, true
	}
	var out []mediaCond
	if len(comps) == 3 {
		if nameAt != 1 {
			return nil, false, true
		}
		lo, ok := cond(comps[0].op, true, 0)
		if !ok {
			return nil, false, true
		}
		hi, ok := cond(comps[1].op, false, 2)
		if !ok {
			return nil, false, true
		}
		return append(out, lo, hi), true, true
	}
	flip := nameAt == 1
	vi := 1 - nameAt
	c, ok := cond(comps[0].op, flip, vi)
	if !ok {
		return nil, false, true
	}
	return append(out, c), true, true
}

func flipCmp(c mediaCmp) mediaCmp {
	switch c {
	case cmpLT:
		return cmpGT
	case cmpLE:
		return cmpGE
	case cmpGT:
		return cmpLT
	case cmpGE:
		return cmpLE
	}
	return c
}

func rangeFeature(name string) (mediaFeat, bool) {
	switch name {
	case "width":
		return featWidth, true
	case "height":
		return featHeight, true
	case "aspect-ratio":
		return featAspect, true
	case "resolution":
		return featResolution, true
	}
	return 0, false
}

// rangeComp is one range-syntax component: a value token with the
// comparison operator that follows it (0 when none does).
type rangeComp struct {
	t  token
	op mediaCmp
}

func rangeComponents(ts []token) ([]rangeComp, bool) {
	var out []rangeComp
	for _, part := range components(ts) {
		if op, isOp := rangeOp(part); isOp {
			if len(out) == 0 {
				return nil, false
			}
			last := &out[len(out)-1]
			switch {
			case last.op == 0:
				last.op = op
			case op == cmpEQ && last.op == cmpLT:
				last.op = cmpLE
			case op == cmpEQ && last.op == cmpGT:
				last.op = cmpGE
			default:
				return nil, false
			}
			continue
		}
		if len(part) != 1 {
			return nil, false
		}
		out = append(out, rangeComp{t: part[0]})
	}
	return out, true
}

func rangeOp(part []token) (mediaCmp, bool) {
	if len(part) == 0 || len(part) > 2 || len(part) == 2 && !part[1].is('=') {
		return 0, false
	}
	switch {
	case part[0].is('<') && len(part) == 2:
		return cmpLE, true
	case part[0].is('>') && len(part) == 2:
		return cmpGE, true
	case part[0].is('<'):
		return cmpLT, true
	case part[0].is('>'):
		return cmpGT, true
	case part[0].is('=') && len(part) == 1:
		return cmpEQ, true
	}
	return 0, false
}

func mediaLength(ts []token) (float64, bool) {
	if len(ts) != 1 {
		return 0, false
	}
	t := ts[0]
	switch t.kind {
	case tkNumber:
		return t.num, true
	case tkDim:
		if t.s == "px" || t.s == "" {
			return t.num, true
		}
	}
	return 0, false
}

func mediaRatio(ts []token) (float64, bool) {
	if len(ts) == 3 {
		var t [3]token
		copy(t[:], ts)
		a, slash, c := t[0], t[1], t[2]
		if a.kind == tkNumber && slash.is('/') && c.kind == tkNumber {
			if c.num == 0 {
				return 0, false
			}
			return a.num / c.num, true
		}
	}
	if len(ts) == 1 && ts[0].kind == tkNumber {
		return ts[0].num, true
	}
	return 0, false
}

func mediaResolution(ts []token) (float64, bool) {
	if len(ts) != 1 {
		return 0, false
	}
	t := ts[0]
	switch t.kind {
	case tkNumber:
		return t.num, true
	case tkDim:
		switch t.s {
		case "dpi":
			return t.num, true
		case "x", "dppx":
			return t.num * 96, true
		case "dpcm":
			return t.num * 2.54, true
		}
	}
	return 0, false
}

// evalSupports evaluates an @supports prelude at parse time against
// the engine's actual property and selector support.
func evalSupports(ts []token) bool {
	orParts := splitOnIdent(ts, "or")
	if len(orParts) > 1 {
		return slices.ContainsFunc(orParts, evalSupports)
	}
	andParts := splitOnIdent(ts, "and")
	if len(andParts) > 1 {
		for _, p := range andParts {
			if !evalSupports(p) {
				return false
			}
		}
		return true
	}
	ts = trimWS(ts)
	if len(ts) == 0 {
		return false
	}
	if ts[0].kind == tkIdent && strings.EqualFold(ts[0].s, "not") {
		return !evalSupports(ts[1:])
	}
	if ts[0].kind == tkLParen && blockEnd(ts, 0) == len(ts) {
		inner := trimWS(ts[1 : len(ts)-1])
		if len(inner) > 0 && inner[0].kind == tkFunc && inner[0].s == "selector" {
			args := trimWS(funcArgs(inner))
			_, ok := parseSelectorList("", args)
			return ok
		}
		if len(inner) > 1 && inner[0].kind == tkIdent && inner[1].kind == tkColon {
			return supportsDecl(inner[0].s, trimWS(inner[2:]))
		}
		return evalSupports(inner)
	}
	if ts[0].kind == tkFunc && ts[0].s == "selector" {
		args := trimWS(funcArgs(ts))
		_, ok := parseSelectorList("", args)
		return ok
	}
	return false
}

// supportsDecl reports whether `name: value` parses in this engine.
func supportsDecl(name string, val []token) bool {
	lname := strings.ToLower(name)
	def, ok := propTable[lname]
	if !ok {
		return ignoredProps[lname]
	}
	if len(val) == 0 || wideOf(val) != wideNone || containsVar(val) {
		return false
	}
	cx := ctx{rem: parseRem, em: parseRem, weight: 400}
	scratch := initialValues()
	return def.parse(val, &cx, &scratch)
}

// splitOnIdent splits ts at nesting depth zero on the identifier name.
func splitOnIdent(ts []token, name string) [][]token {
	var out [][]token
	start := 0
	for i := 0; i < len(ts); {
		if closer(ts[i].kind) != 0 {
			i = blockEnd(ts, i)
			continue
		}
		if ts[i].kind == tkIdent && strings.EqualFold(ts[i].s, name) {
			out = append(out, ts[start:i])
			start = i + 1
		}
		i++
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, ts[start:])
}

// ImportResolver loads an @import target: ref resolved against base,
// the importing stylesheet's path ("" for a string stylesheet, when
// ref stands alone). An error drops the import with a warning.
type ImportResolver func(ref, base string) (path, content string, err error)

// FileImport is the default resolver: the filesystem, relative to the
// importing file's directory.
func FileImport(ref, base string) (string, string, error) {
	p := ref
	if base != "" && !filepath.IsAbs(ref) {
		p = filepath.Join(filepath.Dir(base), ref)
	}
	b, err := os.ReadFile(p) //nolint:gosec // an app stylesheet names its imports
	if err != nil {
		return "", "", err
	}
	return p, string(b), nil
}
