package style

import (
	"iter"
	"slices"
	"strconv"
	"strings"
)

// tokKind classifies one CSS token (CSS Syntax Level 3, the subset a
// GTK-flavored stylesheet produces: no unicode-range, no url() tokens
// beyond an ordinary function, no CDO/CDC).
type tokKind uint8

const (
	tkIdent   tokKind = iota + 1 // foo, --foo, -gtk-icon-size
	tkFunc                       // foo( — s holds the name, the '(' is consumed
	tkAt                         // @foo
	tkHash                       // #foo — s holds foo
	tkString                     // "foo" or 'foo' — s holds the contents
	tkNumber                     // 12, -1.5
	tkPercent                    // 50% — num holds 50
	tkDim                        // 12px — num holds 12, s the unit (lowercased)
	tkDelim                      // any other single byte — s holds it
	tkComma                      // ,
	tkColon                      // :
	tkSemi                       // ;
	tkLParen                     // (
	tkRParen                     // )
	tkLBrack                     // [
	tkRBrack                     // ]
	tkLBrace                     // {
	tkRBrace                     // }
	tkWS                         // one or more whitespace bytes or comments
)

// token is one lexed token. pos and end delimit its source bytes, for
// warnings and for the an+b microsyntax that re-reads raw text.
type token struct {
	kind tokKind
	s    string
	num  float64
	pos  int
	end  int
}

// is reports whether t is the delimiter c.
func (t token) is(c byte) bool { return t.kind == tkDelim && len(t.s) == 1 && t.s[0] == c }

// ident reports whether t is the identifier name, ASCII case-insensitive.
func (t token) ident(name string) bool { return t.kind == tkIdent && strings.EqualFold(t.s, name) }

// tokenize lexes src into tokens. Comments fold into whitespace, so a
// comment between two compound selectors still separates them; runs of
// whitespace collapse into one tkWS token.
func tokenize(src string) []token {
	lx := lexer{src: src}
	out := make([]token, 0, len(src)/4)
	for {
		t, ok := lx.next()
		if !ok {
			return out
		}
		if t.kind == tkWS && len(out) > 0 && out[len(out)-1].kind == tkWS {
			out[len(out)-1].end = t.end
			continue
		}
		out = append(out, t)
	}
}

type lexer struct {
	src string
	pos int
}

func (l *lexer) peek(off int) byte {
	if l.pos+off < len(l.src) {
		return l.src[l.pos+off]
	}
	return 0
}

// next returns the following token; ok is false at the end of input.
func (l *lexer) next() (token, bool) {
	if l.pos >= len(l.src) {
		return token{}, false
	}
	start := l.pos
	c := l.src[l.pos]
	switch {
	case isSpace(c) || c == '/' && l.peek(1) == '*':
		for l.pos < len(l.src) {
			switch {
			case isSpace(l.src[l.pos]):
				l.pos++
			case l.src[l.pos] == '/' && l.peek(1) == '*':
				end := strings.Index(l.src[l.pos+2:], "*/")
				if end < 0 {
					l.pos = len(l.src)
				} else {
					l.pos += 2 + end + 2
				}
			default:
				return token{kind: tkWS, pos: start, end: l.pos}, true
			}
		}
		return token{kind: tkWS, pos: start, end: l.pos}, true
	case c == '"' || c == '\'':
		return l.str(c), true
	case c == '#':
		l.pos++
		if l.pos < len(l.src) && isNameByte(l.src[l.pos]) {
			name := l.name()
			return token{kind: tkHash, s: name, pos: start, end: l.pos}, true
		}
		return token{kind: tkDelim, s: "#", pos: start, end: l.pos}, true
	case c == '@':
		l.pos++
		if l.startsIdent() {
			name := l.name()
			return token{kind: tkAt, s: strings.ToLower(name), pos: start, end: l.pos}, true
		}
		return token{kind: tkDelim, s: "@", pos: start, end: l.pos}, true
	case l.startsNumber():
		return l.numeric(), true
	case l.startsIdent():
		name := l.name()
		if l.pos < len(l.src) && l.src[l.pos] == '(' {
			l.pos++
			return token{kind: tkFunc, s: strings.ToLower(name), pos: start, end: l.pos}, true
		}
		return token{kind: tkIdent, s: name, pos: start, end: l.pos}, true
	}
	l.pos++
	kind := tkDelim
	switch c {
	case ',':
		kind = tkComma
	case ':':
		kind = tkColon
	case ';':
		kind = tkSemi
	case '(':
		kind = tkLParen
	case ')':
		kind = tkRParen
	case '[':
		kind = tkLBrack
	case ']':
		kind = tkRBrack
	case '{':
		kind = tkLBrace
	case '}':
		kind = tkRBrace
	}
	return token{kind: kind, s: l.src[start:l.pos], pos: start, end: l.pos}, true
}

// str lexes a quoted string; an unterminated string runs to the end of
// the line, per the spec's bad-string recovery (kept as a string here:
// the declaration using it fails to parse anyway).
func (l *lexer) str(q byte) token {
	start := l.pos
	l.pos++
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == q:
			l.pos++
			return token{kind: tkString, s: b.String(), pos: start, end: l.pos}
		case c == '\n':
			return token{kind: tkString, s: b.String(), pos: start, end: l.pos}
		case c == '\\' && l.pos+1 < len(l.src):
			l.pos++
			b.WriteByte(l.src[l.pos])
			l.pos++
		default:
			b.WriteByte(c)
			l.pos++
		}
	}
	return token{kind: tkString, s: b.String(), pos: start, end: l.pos}
}

// startsIdent reports whether an identifier starts at the cursor.
func (l *lexer) startsIdent() bool {
	c := l.peek(0)
	if c == '-' {
		n := l.peek(1)
		return isNameStart(n) || n == '-'
	}
	return isNameStart(c)
}

// startsNumber reports whether a number starts at the cursor.
func (l *lexer) startsNumber() bool {
	c := l.peek(0)
	if c == '+' || c == '-' {
		n := l.peek(1)
		return isDigit(n) || n == '.' && isDigit(l.peek(2))
	}
	if c == '.' {
		return isDigit(l.peek(1))
	}
	return isDigit(c)
}

// name consumes a run of name bytes (escapes are passed through
// verbatim — the stylesheets this engine reads never use them).
func (l *lexer) name() string {
	start := l.pos
	for l.pos < len(l.src) && isNameByte(l.src[l.pos]) {
		l.pos++
	}
	return l.src[start:l.pos]
}

// numeric lexes a number, percentage, or dimension.
func (l *lexer) numeric() token {
	start := l.pos
	if c := l.src[l.pos]; c == '+' || c == '-' {
		l.pos++
	}
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	if l.pos+1 < len(l.src) && l.src[l.pos] == '.' && isDigit(l.src[l.pos+1]) {
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		k := l.pos + 1
		if k < len(l.src) && (l.src[k] == '+' || l.src[k] == '-') {
			k++
		}
		if k < len(l.src) && isDigit(l.src[k]) {
			l.pos = k
			for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
				l.pos++
			}
		}
	}
	num, _ := strconv.ParseFloat(strings.TrimPrefix(l.src[start:l.pos], "+"), 64)
	if l.pos < len(l.src) && l.src[l.pos] == '%' {
		l.pos++
		return token{kind: tkPercent, num: num, pos: start, end: l.pos}
	}
	if l.startsIdent() {
		unit := l.name()
		return token{kind: tkDim, num: num, s: strings.ToLower(unit), pos: start, end: l.pos}
	}
	return token{kind: tkNumber, num: num, pos: start, end: l.pos}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isNameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func isNameByte(c byte) bool { return isNameStart(c) || isDigit(c) || c == '-' }

// trimWS drops leading and trailing whitespace tokens.
func trimWS(ts []token) []token {
	for len(ts) > 0 && ts[0].kind == tkWS {
		ts = ts[1:]
	}
	for len(ts) > 0 && ts[len(ts)-1].kind == tkWS {
		ts = ts[:len(ts)-1]
	}
	return ts
}

// closer returns the token kind closing an opener, or 0.
func closer(k tokKind) tokKind {
	switch k {
	case tkFunc, tkLParen:
		return tkRParen
	case tkLBrack:
		return tkRBrack
	case tkLBrace:
		return tkRBrace
	}
	return 0
}

// blockEnd returns the index just past the block that opens at ts[i]
// (a function, paren, bracket, or brace), honoring nesting; an
// unterminated block ends at len(ts).
func blockEnd(ts []token, i int) int {
	var stack []tokKind
	for ; i < len(ts); i++ {
		if c := closer(ts[i].kind); c != 0 {
			stack = append(stack, c)
			continue
		}
		if len(stack) > 0 && ts[i].kind == stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1
			}
		}
	}
	return len(ts)
}

// splitTop splits ts on the separator kind at nesting depth zero.
func splitTop(ts []token, sep tokKind) [][]token { return slices.Collect(eachTop(ts, sep)) }

// eachTop yields splitTop's parts without collecting them: the parsers
// that run per node (a value with a var() or a named color) iterate
// instead of allocating.
func eachTop(ts []token, sep tokKind) iter.Seq[[]token] {
	return func(yield func([]token) bool) {
		start := 0
		for i := 0; i < len(ts); {
			if closer(ts[i].kind) != 0 {
				i = blockEnd(ts, i)
				continue
			}
			if ts[i].kind == sep {
				if !yield(ts[start:i]) {
					return
				}
				start = i + 1
			}
			i++
		}
		yield(ts[start:])
	}
}

// splitArgs splits a function's arguments at their top-level commas
// into exactly len(parts) parts, filling parts in place: the per-node
// color functions use it with a fixed array, so a named color inside
// alpha() or mix() costs no allocation per restyle. False when the
// count differs.
func splitArgs(args []token, parts [][]token) bool {
	n := 0
	for p := range eachTop(args, tkComma) {
		if n == len(parts) {
			return false
		}
		parts[n] = p
		n++
	}
	return n == len(parts)
}

// components splits a value into its top-level component values,
// dropping the whitespace between them: `1px solid calc(2 * 3px)`
// yields three components, the function kept whole.
func components(ts []token) [][]token { return slices.Collect(eachComponent(ts)) }

// eachComponent yields components' values without collecting them.
func eachComponent(ts []token) iter.Seq[[]token] {
	return func(yield func([]token) bool) {
		for i := 0; i < len(ts); {
			if ts[i].kind == tkWS {
				i++
				continue
			}
			end := i + 1
			if closer(ts[i].kind) != 0 {
				end = blockEnd(ts, i)
			}
			if !yield(ts[i:end]) {
				return
			}
			i = end
		}
	}
}

// funcArgs returns the tokens between a function token at ts[0] and
// its closing paren.
func funcArgs(ts []token) []token {
	if len(ts) < 2 || ts[len(ts)-1].kind != tkRParen {
		if len(ts) > 0 {
			return ts[1:]
		}
		return nil
	}
	return ts[1 : len(ts)-1]
}

// rawText re-reads the source bytes a token span covers: from its
// earliest token to its latest, since a desugared nested selector
// splices its parent's tokens out of source order.
func rawText(src string, ts []token) string {
	if len(ts) == 0 {
		return ""
	}
	lo, hi := ts[0].pos, ts[0].end
	for _, t := range ts[1:] {
		lo, hi = min(lo, t.pos), max(hi, t.end)
	}
	return src[lo:hi]
}
