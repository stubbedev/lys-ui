package style

import (
	"math"
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// numKind is the type of a numeric value after unit resolution.
type numKind uint8

const (
	numNumber    numKind = iota + 1 // unitless
	numLength                       // pixels
	numPercent                      // percent (not resolved against anything)
	numAngle                        // degrees
	numTime                         // seconds
	numLengthPct                    // px plus a percentage, calc()'s mix
)

// numeric is a typed number: lengths in px, angles in degrees, times in
// seconds. A numLengthPct is a calc() sum of a length and a
// percentage: v pixels plus pct percent of a basis only the layout
// knows.
type numeric struct {
	v    float64
	pct  float64
	kind numKind
}

// mixable reports whether a sum of kinds a and b is a length-percentage
// mix: lengths and percentages add into one, which only the properties
// resolving percentages at layout time accept.
func mixable(a, b numKind) bool {
	lp := func(k numKind) bool { return k == numLength || k == numPercent || k == numLengthPct }
	return a != b && lp(a) && lp(b)
}

// asMix lifts a length or a percentage into the mixed kind.
func asMix(n numeric) numeric {
	switch n.kind {
	case numLength:
		return numeric{v: n.v, kind: numLengthPct}
	case numPercent:
		return numeric{pct: n.v, kind: numLengthPct}
	}
	return n
}

// settleMix folds a mix whose percentage or length cancelled back into
// the plain kind, so calc(50% + 1px - 1px) is a percentage again.
func settleMix(n numeric) numeric {
	switch {
	case n.kind != numLengthPct:
		return n
	case n.pct == 0:
		return numeric{v: n.v, kind: numLength}
	case n.v == 0:
		return numeric{v: n.pct, kind: numPercent}
	}
	return n
}

// FontMetrics is a face's font-relative unit bases as ratios of the
// font size: what one ch, ex, cap, ic and lh of the node's font is,
// per em. A zero field falls back to the CSS default ratio. The
// widget layer fills it from the resolved face (render.Font.Metrics).
type FontMetrics struct {
	Ch, Ex, Cap, Ic, LineHeight float64
}

// cssFontFallbacks are the ratios the spec prescribes when the face
// does not report a metric: half an em for ch and ex, one em for ic,
// and 1.2 for the natural line height.
const (
	fallbackCh  = 0.5
	fallbackEx  = 0.5
	fallbackCap = 0.5
	fallbackIc  = 1
	fallbackLH  = 1.2
)

// ctx is the unit-resolution context of one node's compute: the rem
// base, the font size em resolves against, the inherited weight bolder
// and lighter step from, the node's color for currentColor, the
// viewport the vw/vh family reads, the node's font metrics and line
// height, and the scheme light-dark() picks by; attr() reads the
// node's attributes off self. dep records whether a parse read any node-dependent
// input, so a value that read none can be cached on its declaration.
type ctx struct {
	rem    float64
	em     float64
	weight int
	color  render.Color
	dep    bool
	// self is the node being computed, the attr() of a content value
	// reads its attributes from; nil at parse-time validation.
	self *Target
	// counters is the node's counter scope, counter()/counters() read.
	counters *CounterScope
	// quoteDepth is the inherited open-quote nesting depth the
	// content quote keywords count from.
	quoteDepth int
	// usedRem records a rem read: a cached value is only valid under
	// the base it was computed with.
	usedRem bool

	// cq records a container-query unit read, for the sheet summary
	// that gates the per-compute container resolution.
	cqRead bool

	vpW, vpH float64
	font     FontMetrics
	lh, rlh  float64
	dark     bool
	// dpi is the device resolution image-set() picks a candidate for.
	dpi float64
	// conts are the node's query containers, nearest first, the
	// container units resolve against.
	conts []Container
}

// dppx is the device's pixels per CSS pixel, image-set()'s pick
// criterion: 1 where the compute knows no resolution (parse time).
func (cx *ctx) dppx() float64 {
	if cx.dpi <= 0 {
		return 1
	}
	return cx.dpi / 96
}

// emPx returns the em base, noting the node dependence.
func (cx *ctx) emPx() float64 {
	cx.dep = true
	return cx.em
}

// fontRatio returns metric's ratio to the font size, applying the CSS
// fallback when the face does not report it.
func fontRatio(v, fallback float64) float64 {
	if v <= 0 {
		return fallback
	}
	return v
}

// wt returns the inherited font weight, noting the node dependence.
func (cx *ctx) wt() int {
	cx.dep = true
	return cx.weight
}

// cur returns currentColor, noting the node dependence.
func (cx *ctx) cur() render.Color {
	cx.dep = true
	return cx.color
}

// resolve turns a parsed color into a concrete one.
func (cx *ctx) resolve(c colorVal) render.Color {
	if c.current {
		return cx.cur()
	}
	return c.c
}

// unitScale converts a dimension unit to its canonical kind and factor.
func (cx *ctx) unitScale(unit string) (numKind, float64, bool) {
	switch strings.ToLower(unit) {
	case "px":
		return numLength, 1, true
	case "rem":
		cx.usedRem = true
		return numLength, cx.rem, true
	case "em":
		return numLength, cx.emPx(), true
	case "ch":
		cx.dep = true
		return numLength, fontRatio(cx.font.Ch, fallbackCh) * cx.em, true
	case "ex":
		cx.dep = true
		return numLength, fontRatio(cx.font.Ex, fallbackEx) * cx.em, true
	case "cap":
		cx.dep = true
		return numLength, fontRatio(cx.font.Cap, fallbackCap) * cx.em, true
	case "ic":
		cx.dep = true
		return numLength, fontRatio(cx.font.Ic, fallbackIc) * cx.em, true
	case "lh":
		cx.dep = true
		return numLength, cx.lh, true
	case "rlh":
		cx.dep = true
		return numLength, cx.rlh, true
	case "vw", "svw", "lvw", "dvw", "vi", "svi", "lvi", "dvi":
		cx.dep = true
		return numLength, cx.vpW / 100, true
	case "vh", "svh", "lvh", "dvh", "vb", "svb", "lvb", "dvb":
		cx.dep = true
		return numLength, cx.vpH / 100, true
	case "vmin", "svmin", "lvmin", "dvmin":
		cx.dep = true
		return numLength, math.Min(cx.vpW, cx.vpH) / 100, true
	case "vmax", "svmax", "lvmax", "dvmax":
		cx.dep = true
		return numLength, math.Max(cx.vpW, cx.vpH) / 100, true
	case "cqw", "cqh", "cqi", "cqb", "cqmin", "cqmax":
		cx.dep = true
		cx.cqRead = true
		v, _ := cqUnit(unit, cx.conts, cx.vpW, cx.vpH)
		return numLength, v, true
	case "pt":
		return numLength, 96.0 / 72, true
	case "pc":
		return numLength, 16, true
	case "in":
		return numLength, 96, true
	case "cm":
		return numLength, 96 / 2.54, true
	case "mm":
		return numLength, 96 / 25.4, true
	case "q":
		return numLength, 96 / 101.6, true
	case "deg":
		return numAngle, 1, true
	case "rad":
		return numAngle, 180 / math.Pi, true
	case "grad":
		return numAngle, 0.9, true
	case "turn":
		return numAngle, 360, true
	case "s":
		return numTime, 1, true
	case "ms":
		return numTime, 0.001, true
	}
	return 0, 0, false
}

// evalNumeric evaluates one component value — a number, percentage,
// dimension, or calc()/min()/max()/clamp() expression — to a typed
// number. The tokens must be exactly one component.
func evalNumeric(ts []token, cx *ctx) (numeric, bool) {
	ts = trimWS(ts)
	if len(ts) == 0 {
		return numeric{}, false
	}
	e := calcParser{ts: ts, cx: cx}
	n, ok := e.term()
	if !ok || e.pos != len(ts) {
		return numeric{}, false
	}
	n.v, n.pct = calcFinite(n.v), calcFinite(n.pct)
	return n, true
}

// calcLimit is the magnitude an infinite calculation clamps to: far
// past any real length, angle or count, and safe to round to an int.
const calcLimit = 1 << 30

// calcFinite applies CSS Values 4's top-level rule to a calculation's
// result: NaN becomes zero and an infinity the largest representable
// value of its sign.
func calcFinite(v float64) float64 {
	switch {
	case math.IsNaN(v):
		return 0
	case v > calcLimit:
		return calcLimit
	case v < -calcLimit:
		return -calcLimit
	}
	return v
}

// calcParser evaluates calc() expressions over tokens: sums, products,
// parentheses, and nested math functions, with CSS's type rules
// (lengths add to lengths, a product needs a unitless side, a quotient
// a unitless divisor).
type calcParser struct {
	ts  []token
	pos int
	cx  *ctx
	// refs, when set, resolves channel keywords (relative color
	// syntax) to the origin color's channel values.
	refs map[string]numeric
}

func (p *calcParser) skipWS() {
	for p.pos < len(p.ts) && p.ts[p.pos].kind == tkWS {
		p.pos++
	}
}

// term parses one leaf: a literal or a math function.
func (p *calcParser) term() (numeric, bool) {
	if p.pos >= len(p.ts) {
		return numeric{}, false
	}
	t := p.ts[p.pos]
	switch t.kind {
	case tkNumber:
		p.pos++
		return numeric{v: t.num, kind: numNumber}, true
	case tkPercent:
		p.pos++
		return numeric{v: t.num, kind: numPercent}, true
	case tkDim:
		p.pos++
		kind, f, ok := p.cx.unitScale(t.s)
		if !ok {
			return numeric{}, false
		}
		return numeric{v: t.num * f, kind: kind}, true
	case tkLParen:
		end := blockEnd(p.ts, p.pos)
		sub := calcParser{ts: p.ts[p.pos+1 : end-1], cx: p.cx}
		n, ok := sub.sum()
		if !ok || sub.pos != len(sub.ts) {
			return numeric{}, false
		}
		p.pos = end
		return n, true
	case tkFunc:
		end := blockEnd(p.ts, p.pos)
		args := funcArgs(p.ts[p.pos:end])
		p.pos = end
		return mathFunc(t.s, args, p.cx, p.refs)
	case tkIdent:
		if n, ok := p.refs[t.s]; ok {
			p.pos++
			return n, true
		}
		if v, ok := calcConstant(t.s); ok {
			p.pos++
			return numeric{v: v, kind: numNumber}, true
		}
	}
	return numeric{}, false
}

// calcConstant is the value of a CSS Values 4 numeric constant: e,
// pi, infinity, -infinity and NaN, matched ASCII case-insensitively.
func calcConstant(name string) (float64, bool) {
	switch strings.ToLower(name) {
	case "e":
		return math.E, true
	case "pi":
		return math.Pi, true
	case "infinity":
		return math.Inf(1), true
	case "-infinity":
		return math.Inf(-1), true
	case "nan":
		return math.NaN(), true
	}
	return 0, false
}

// mathNums evaluates a comma-separated argument list, each argument
// a full math expression; every value must share the first's kind.
func mathNums(args []token, cx *ctx, refs map[string]numeric, kinds ...numKind) ([]numeric, bool) {
	parts := splitTop(args, tkComma)
	if len(parts) == 0 {
		return nil, false
	}
	out := make([]numeric, 0, len(parts))
	for _, a := range parts {
		sub := calcParser{ts: a, cx: cx, refs: refs}
		sub.skipWS()
		n, ok := sub.sum()
		sub.skipWS()
		// A length-percentage mix compares only at layout time, which
		// the compute-time comparison functions never see.
		if !ok || sub.pos != len(a) || n.kind == numLengthPct {
			return nil, false
		}
		if len(out) > 0 && out[0].kind != n.kind {
			return nil, false
		}
		out = append(out, n)
	}
	if len(kinds) > 0 {
		ok := false
		for _, k := range kinds {
			if out[0].kind == k {
				ok = true
			}
		}
		if !ok {
			return nil, false
		}
	}
	return out, true
}

// mathOne evaluates one single-argument math function.
func mathOne(args []token, cx *ctx, refs map[string]numeric) (numeric, bool) {
	ns, ok := mathNums(args, cx, refs)
	if !ok || len(ns) != 1 {
		return numeric{}, false
	}
	return ns[0], true
}

// mathFunc evaluates calc(), min(), max(), clamp() and the CSS Values 4
// math functions over them. refs, when set, resolves the channel
// keywords relative color syntax substitutes.
func mathFunc(name string, args []token, cx *ctx, refs map[string]numeric) (numeric, bool) {
	switch name {
	case "calc":
		sub := calcParser{ts: args, cx: cx, refs: refs}
		sub.skipWS()
		n, ok := sub.sum()
		sub.skipWS()
		return n, ok && sub.pos == len(args)
	case "min", "max", "clamp":
		ns, ok := mathNums(args, cx, refs)
		if !ok || len(ns) == 0 || name == "clamp" && len(ns) != 3 {
			return numeric{}, false
		}
		out := ns[0]
		switch name {
		case "min":
			for _, v := range ns[1:] {
				out.v = math.Min(out.v, v.v)
			}
		case "max":
			for _, v := range ns[1:] {
				out.v = math.Max(out.v, v.v)
			}
		case "clamp":
			out.v = math.Max(ns[0].v, math.Min(ns[1].v, ns[2].v))
		}
		return out, true
	case "round":
		args = trimWS(args)
		strategy := "nearest"
		if len(args) > 0 && args[0].kind == tkIdent {
			switch args[0].s {
			case "nearest", "up", "down", "to-zero":
				strategy = args[0].s
				args = args[indexOfIdent(args):]
				if len(args) > 0 && args[0].kind == tkComma {
					args = args[1:]
				}
			default:
				return numeric{}, false
			}
		}
		ns, ok := mathNums(args, cx, refs)
		if !ok {
			return numeric{}, false
		}
		if len(ns) == 1 {
			ns = append(ns, numeric{v: 1, kind: ns[0].kind})
		}
		if len(ns) != 2 {
			return numeric{}, false
		}
		return numeric{v: roundTo(strategy, ns[0].v, ns[1].v), kind: ns[1].kind}, true
	case "mod", "rem":
		ns, ok := mathNums(args, cx, refs, numNumber, numLength, numAngle, numTime, numPercent)
		if !ok || len(ns) != 2 {
			return numeric{}, false
		}
		if ns[1].v == 0 || math.IsInf(ns[0].v, 0) {
			return numeric{v: math.NaN(), kind: ns[0].kind}, true
		}
		if math.IsInf(ns[1].v, 0) {
			return numeric{v: modInfinite(name, ns[0].v, ns[1].v), kind: ns[0].kind}, true
		}
		q := ns[0].v / ns[1].v
		if name == "mod" {
			q = math.Floor(q)
		} else {
			q = math.Trunc(q)
		}
		return numeric{v: ns[0].v - ns[1].v*q, kind: ns[0].kind}, true
	case "abs":
		n, ok := mathOne(args, cx, refs)
		if !ok {
			return numeric{}, false
		}
		n.v = math.Abs(n.v)
		return n, true
	case "sign":
		n, ok := mathOne(args, cx, refs)
		if !ok {
			return numeric{}, false
		}
		return numeric{v: signOf(n.v), kind: numNumber}, true
	case "sin", "cos", "tan":
		n, ok := mathOne(args, cx, refs)
		if !ok || (n.kind != numNumber && n.kind != numAngle) {
			return numeric{}, false
		}
		a := n.v
		if n.kind != numAngle {
			a *= 180 / math.Pi
		}
		a *= math.Pi / 180
		var v float64
		switch name {
		case "sin":
			v = math.Sin(a)
		case "cos":
			v = math.Cos(a)
		default:
			v = math.Tan(a)
		}
		return numeric{v: v, kind: numNumber}, true
	case "asin", "acos", "atan", "atan2":
		ns, ok := mathNums(args, cx, refs)
		if !ok {
			return numeric{}, false
		}
		var a float64
		switch name {
		case "asin":
			if len(ns) != 1 || ns[0].kind != numNumber {
				return numeric{}, false
			}
			a = math.Asin(ns[0].v)
		case "acos":
			if len(ns) != 1 || ns[0].kind != numNumber {
				return numeric{}, false
			}
			a = math.Acos(ns[0].v)
		case "atan":
			if len(ns) != 1 || ns[0].kind != numNumber {
				return numeric{}, false
			}
			a = math.Atan(ns[0].v)
		default:
			if len(ns) != 2 {
				return numeric{}, false
			}
			a = math.Atan2(ns[0].v, ns[1].v)
		}
		return numeric{v: a * 180 / math.Pi, kind: numAngle}, true
	case "pow", "log":
		ns, ok := mathNums(args, cx, refs, numNumber)
		if !ok || len(ns) < 1 || len(ns) > 2 {
			return numeric{}, false
		}
		if name == "pow" {
			if len(ns) != 2 {
				return numeric{}, false
			}
			return numeric{v: math.Pow(ns[0].v, ns[1].v), kind: numNumber}, true
		}
		if len(ns) == 2 {
			return numeric{v: math.Log(ns[0].v) / math.Log(ns[1].v), kind: numNumber}, true
		}
		return numeric{v: math.Log(ns[0].v), kind: numNumber}, true
	case "sqrt", "exp":
		n, ok := mathOne(args, cx, refs)
		if !ok || n.kind != numNumber {
			return numeric{}, false
		}
		if name == "sqrt" {
			return numeric{v: math.Sqrt(n.v), kind: numNumber}, true
		}
		return numeric{v: math.Exp(n.v), kind: numNumber}, true
	case "hypot":
		ns, ok := mathNums(args, cx, refs)
		if !ok || len(ns) == 0 {
			return numeric{}, false
		}
		sq := 0.0
		for _, n := range ns {
			sq += n.v * n.v
		}
		return numeric{v: math.Sqrt(sq), kind: ns[0].kind}, true
	case "env":
		return envFunc(args, cx, refs)
	case "attr":
		return attrNumeric(args, cx, refs)
	}
	return numeric{}, false
}

// roundTo rounds a to a multiple of b under a round() strategy;
// nearest takes the upper multiple on a tie, as CSS Values 4 says, and
// a zero step is NaN.
func roundTo(strategy string, a, b float64) float64 {
	if b == 0 {
		return math.NaN()
	}
	q := a / b
	switch strategy {
	case "up":
		q = math.Ceil(q)
	case "down":
		q = math.Floor(q)
	case "to-zero":
		q = math.Trunc(q)
	default:
		q = math.Floor(q + 0.5)
	}
	return q * b
}

// modInfinite is mod() and rem() of a finite value by an infinite
// one: rem() and a same-signed mod() keep the value, a mod() of
// opposite signs is NaN.
func modInfinite(name string, a, b float64) float64 {
	if name == "mod" && a != 0 && math.Signbit(a) != math.Signbit(b) {
		return math.NaN()
	}
	return a
}

func signOf(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// indexOfIdent returns the index past a leading strategy ident: the
// round() args slice holds it as its first token.
func indexOfIdent(args []token) int {
	for i, t := range args {
		if t.kind == tkWS {
			continue
		}
		if t.kind == tkIdent {
			return i + 1
		}
		return 0
	}
	return 0
}

// envNames are the environment variables the engine answers: the
// safe-area and keyboard inset family, zero on a desktop compositor.
var envNames = map[string]bool{
	"safe-area-inset-top": true, "safe-area-inset-right": true,
	"safe-area-inset-bottom": true, "safe-area-inset-left": true,
	"titlebar-area-height": true, "titlebar-area-width": true,
	"keyboard-inset-top": true, "keyboard-inset-right": true,
	"keyboard-inset-bottom": true, "keyboard-inset-left": true,
	"keyboard-inset-height": true, "keyboard-inset-width": true,
}

// envFunc evaluates env(name, fallback?): every supported variable is
// a zero length on the desktop; an unknown name takes the fallback,
// and is invalid without one.
func envFunc(args []token, cx *ctx, refs map[string]numeric) (numeric, bool) {
	parts := splitTop(args, tkComma)
	name := trimWS(parts[0])
	if len(parts) > 2 || len(name) != 1 || name[0].kind != tkIdent {
		return numeric{}, false
	}
	if envNames[name[0].s] {
		return numeric{v: 0, kind: numLength}, true
	}
	return fallbackNumeric(parts[1:], cx, refs)
}

// fallbackNumeric evaluates an env() or attr() fallback, the one math
// expression after the comma; none is invalid.
func fallbackNumeric(rest [][]token, cx *ctx, refs map[string]numeric) (numeric, bool) {
	if len(rest) != 1 {
		return numeric{}, false
	}
	ts := trimWS(rest[0])
	sub := calcParser{ts: ts, cx: cx, refs: refs}
	n, ok := sub.sum()
	return n, ok && sub.pos == len(ts)
}

// attrCall is a parsed typed attr(): the attribute name, its type —
// a unit, a type keyword (`number`, `length`, `color`, ...), or the
// syntax name of type(<syntax>) — and the fallback after the comma.
type attrCall struct {
	name, typ string
	fallback  [][]token
}

// parseAttrCall reads attr( <name> <type>? , <fallback>? ).
func parseAttrCall(args []token) (attrCall, bool) {
	parts := splitTop(args, tkComma)
	comps := components(parts[0])
	if len(parts) > 2 || len(comps) < 1 || len(comps) > 2 || len(comps[0]) != 1 || comps[0][0].kind != tkIdent {
		return attrCall{}, false
	}
	c := attrCall{name: comps[0][0].s, fallback: parts[1:]}
	if len(comps) == 1 {
		return c, true
	}
	t := comps[1]
	switch {
	case len(t) == 1 && t[0].kind == tkIdent:
		c.typ = strings.ToLower(t[0].s)
	case len(t) == 1 && t[0].is('%'):
		c.typ = "%"
	case t[0].kind == tkFunc && t[0].s == "type":
		syntax := strings.TrimSpace(tokensText(funcArgs(t)))
		inner, ok := strings.CutPrefix(syntax, "<")
		inner, ok2 := strings.CutSuffix(inner, ">")
		if !ok || !ok2 {
			return attrCall{}, false
		}
		c.typ = inner
	default:
		return attrCall{}, false
	}
	return c, true
}

// attrNumeric evaluates typed attr() where a number, length, angle or
// time is taken; a missing or mistyped attribute takes the fallback.
func attrNumeric(args []token, cx *ctx, refs map[string]numeric) (numeric, bool) {
	c, ok := parseAttrCall(args)
	if !ok {
		return numeric{}, false
	}
	if n, ok := c.numeric(cx); ok {
		return n, true
	}
	return fallbackNumeric(c.fallback, cx, refs)
}

// attrKinds maps the type keywords and type(<syntax>) names onto the
// numeric kind the attribute must parse to.
var attrKinds = map[string]numKind{
	"number": numNumber, "integer": numNumber, "length": numLength,
	"percentage": numPercent, "angle": numAngle, "time": numTime,
}

// numeric parses the attribute under the call's type: untyped as the
// CSS value it holds, a type keyword as a value of that type, and a
// unit type (`px`, `%`, `deg`, ...) as a bare number in that unit.
func (c attrCall) numeric(cx *ctx) (numeric, bool) {
	val, ok := cx.attrOf(c.name)
	if !ok {
		return numeric{}, false
	}
	n, ok := evalNumeric(tokenize(val), cx)
	if !ok {
		return numeric{}, false
	}
	if c.typ == "" {
		return n, true
	}
	if want, ok := attrKinds[c.typ]; ok {
		return n, n.kind == want || want == numLength && n.kind == numNumber && n.v == 0
	}
	if n.kind != numNumber {
		return numeric{}, false
	}
	if c.typ == "%" {
		return numeric{v: n.v, kind: numPercent}, true
	}
	kind, f, ok := cx.unitScale(c.typ)
	return numeric{v: n.v * f, kind: kind}, ok
}

// attrOf reads one attribute of the node being computed — the
// built-ins and its data attributes, the store selectors and content
// attr() read — marking the node dependence. Parse-time validation has
// no node and reads nothing.
func (cx *ctx) attrOf(name string) (string, bool) {
	cx.dep = true
	if cx.self == nil {
		return "", false
	}
	return cx.self.attrValue(name)
}

// sum parses product (('+'|'-') product)*. CSS requires whitespace
// around + and -; a signed number token after whitespace (`a -1px`) is
// read as a subtraction too, since that is what the source means.
func (p *calcParser) sum() (numeric, bool) {
	left, ok := p.product()
	if !ok {
		return numeric{}, false
	}
	for {
		save := p.pos
		p.skipWS()
		if p.pos >= len(p.ts) {
			return left, true
		}
		t := p.ts[p.pos]
		var sign float64
		switch {
		case t.is('+'):
			sign = 1
			p.pos++
		case t.is('-'):
			sign = -1
			p.pos++
		case (t.kind == tkNumber || t.kind == tkDim || t.kind == tkPercent) && t.num < 0 && p.pos > save:
			// `x -2px`: the sign lexed into the number.
			sign = 1
		default:
			p.pos = save
			return left, true
		}
		p.skipWS()
		right, ok := p.product()
		if !ok || right.kind != left.kind {
			// A unitless zero adds to anything, as in `calc(0 + 1px)`.
			switch {
			case ok && right.v == 0 && right.kind == numNumber:
				right.kind = left.kind
			case ok && left.v == 0 && left.kind == numNumber:
				left.kind = right.kind
			case ok && mixable(left.kind, right.kind):
				left, right = asMix(left), asMix(right)
			default:
				return numeric{}, false
			}
		}
		left.v += sign * right.v
		left.pct += sign * right.pct
		left = settleMix(left)
	}
}

// product parses term (('*'|'/') term)*.
func (p *calcParser) product() (numeric, bool) {
	left, ok := p.term()
	if !ok {
		return numeric{}, false
	}
	for {
		save := p.pos
		p.skipWS()
		if p.pos >= len(p.ts) {
			return left, true
		}
		t := p.ts[p.pos]
		if !t.is('*') && !t.is('/') {
			p.pos = save
			return left, true
		}
		p.pos++
		p.skipWS()
		right, ok := p.term()
		if !ok {
			return numeric{}, false
		}
		if t.is('*') {
			switch {
			case right.kind == numNumber:
				left.v *= right.v
				left.pct *= right.v
			case left.kind == numNumber:
				left = numeric{v: left.v * right.v, pct: left.v * right.pct, kind: right.kind}
			default:
				return numeric{}, false
			}
			continue
		}
		if right.kind != numNumber {
			return numeric{}, false
		}
		left.v /= right.v
		left.pct /= right.v
	}
}

// lengthOf evaluates a length component: a dimension, a unitless zero,
// or a math function yielding a length. Unitless non-zero numbers are
// accepted as pixels too — GTK stylesheets in the wild write them, and
// the pre-var engine took them.
func lengthOf(ts []token, cx *ctx) (float64, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok {
		return 0, false
	}
	switch n.kind {
	case numLength, numNumber:
		return n.v, true
	}
	return 0, false
}

// px rounds a length to whole logical pixels, halves away from zero.
func px(v float64) int { return int(math.Round(v)) }
