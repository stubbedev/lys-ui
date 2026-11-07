package highlight

import (
	"regexp"
	"unicode"

	"github.com/stubbedev/lys-ui/widget"
)

// GtkSourceView def: style ids the languages share.
const (
	styleComment      = "def:comment"
	styleDocComment   = "def:doc-comment"
	stylePreproc      = "def:preprocessor"
	styleKeyword      = "def:keyword"
	styleType         = "def:type"
	styleString       = "def:string"
	styleChar         = "def:character"
	styleEscape       = "def:special-char"
	styleBoolean      = "def:boolean"
	styleSpecialConst = "def:special-constant"
	styleConstant     = "def:constant"
	styleDecimal      = "def:decimal"
	styleFloat        = "def:floating-point"
	styleBaseN        = "def:base-n-integer"
	styleComplex      = "def:complex"
	styleBuiltin      = "def:builtin"
	styleIdentifier   = "def:identifier"
	styleFunction     = "def:function"
	styleInserted     = "def:inserted"
	styleDeleted      = "def:deleted"
	styleError        = "def:error"
	styleHeading      = "def:heading"
	styleEmphasis     = "def:emphasis"
	styleStrong       = "def:strong-emphasis"
	styleInlineCode   = "def:inline-code"
	stylePreformatted = "def:preformatted-section"
	styleLinkText     = "def:link-text"
	styleLinkDest     = "def:link-destination"
	styleListMarker   = "def:list-marker"
	styleQuoteMarker  = "def:blockquote-marker"
	styleBreak        = "def:thematic-break"
)

// scanner is one line's scan - the cursor and span list every
// language's state machine runs on, with the lexing steps they share.
type scanner struct {
	rs    []rune
	i     int
	spans []widget.TextSpan
}

// done reports whether the line is consumed.
func (s *scanner) done() bool { return s.i >= len(s.rs) }

// peek is the rune k past the cursor, 0 off the line.
func (s *scanner) peek(k int) rune {
	if j := s.i + k; j >= 0 && j < len(s.rs) {
		return s.rs[j]
	}
	return 0
}

// span styles runes [start, end) with class; empty ranges add nothing.
func (s *scanner) span(start, end int, class string) {
	if end > start {
		s.spans = append(s.spans, widget.TextSpan{Start: start, End: end, Class: class})
	}
}

// at reports whether lit starts at the cursor.
func (s *scanner) at(lit string) bool {
	j := s.i
	for _, r := range lit {
		if j >= len(s.rs) || s.rs[j] != r {
			return false
		}
		j++
	}
	return true
}

// skipSpace steps over spaces and tabs.
func (s *scanner) skipSpace() {
	for s.i < len(s.rs) && (s.rs[s.i] == ' ' || s.rs[s.i] == '\t') {
		s.i++
	}
}

// rest styles the remainder of the line with class.
func (s *scanner) rest(class string) {
	s.span(s.i, len(s.rs), class)
	s.i = len(s.rs)
}

// take steps n runes (clamped) styled as class.
func (s *scanner) take(n int, class string) {
	end := min(s.i+n, len(s.rs))
	s.span(s.i, end, class)
	s.i = end
}

// word steps over the run of runes in, returning it.
func (s *scanner) word(in func(rune) bool) string {
	start := s.i
	for s.i < len(s.rs) && in(s.rs[s.i]) {
		s.i++
	}
	return string(s.rs[start:s.i])
}

// match is the rune length of re's match anchored at the cursor, 0 for
// none (re must begin with ^).
func (s *scanner) match(re *regexp.Regexp) int {
	rest := string(s.rs[s.i:])
	if loc := re.FindStringIndex(rest); loc != nil {
		return len([]rune(rest[:loc[1]]))
	}
	return 0
}

// form is one token shape: an anchored pattern and its style.
type form struct {
	re    *regexp.Regexp
	class string
}

// first styles the first of forms matching at the cursor (the caller
// orders them longest-first); false when none matches.
func (s *scanner) first(forms []form) bool {
	for _, f := range forms {
		if n := s.match(f.re); n > 0 {
			s.take(n, f.class)
			return true
		}
	}
	return false
}

// escape steps over one backslash escape: \xHH, \uHHHH, \UHHHHHHHH,
// three octal digits, or any single rune - the superset of the
// languages' escape forms.
func (s *scanner) escape() {
	s.i++
	if s.i >= len(s.rs) {
		return
	}
	switch r := s.rs[s.i]; {
	case r == 'x':
		s.i = min(s.i+3, len(s.rs))
	case r == 'u':
		s.i = min(s.i+5, len(s.rs))
	case r == 'U':
		s.i = min(s.i+9, len(s.rs))
	case r >= '0' && r <= '7':
		s.i = min(s.i+3, len(s.rs))
	default:
		s.i++
	}
}

// quoted styles a string from the cursor (at its opening quote, or
// mid-string when open is false) to its closing quote: str for the
// text, escapes apart as def:special-char when escapes is set. It
// reports whether the string closed on this line.
func (s *scanner) quoted(quote rune, open, escapes bool, str string) bool {
	from := s.i
	if open {
		s.i++
	}
	for s.i < len(s.rs) {
		switch r := s.rs[s.i]; {
		case r == quote:
			s.i++
			s.span(from, s.i, str)
			return true
		case r == '\\' && escapes:
			s.span(from, s.i, str)
			esc := s.i
			s.escape()
			s.span(esc, s.i, styleEscape)
			from = s.i
		default:
			s.i++
		}
	}
	s.span(from, s.i, str)
	return false
}

// skipQuoted steps over a string from its opening quote to its closing
// one, or the line's end (escapes skipped when set), without styling it.
func (s *scanner) skipQuoted(quote rune, escapes bool) {
	s.i++
	for s.i < len(s.rs) {
		switch s.rs[s.i] {
		case quote:
			s.i++
			return
		case '\\':
			if escapes {
				s.escape()
				continue
			}
		}
		s.i++
	}
}

// until styles from the cursor through the first close (inclusive) as
// class, reporting whether close came on this line (else the rest of
// the line is styled).
func (s *scanner) until(close, class string) bool { return s.untilFrom(s.i, close, class) }

// delimited styles a construct from its opening delimiter at the
// cursor through its closing one - a block comment, a raw string - so
// the opener never doubles as the closer ("/*/" stays open). It
// reports whether the construct closed on this line.
func (s *scanner) delimited(open, close, class string) bool {
	from := s.i
	s.i += len([]rune(open))
	return s.untilFrom(from, close, class)
}

// untilFrom scans from the cursor to the first close and styles from
// from.
func (s *scanner) untilFrom(from int, close, class string) bool {
	for s.i < len(s.rs) {
		if s.at(close) {
			s.i += len([]rune(close))
			s.span(from, s.i, class)
			return true
		}
		s.i++
	}
	s.span(from, s.i, class)
	return false
}

// isIdent is a letter, digit or underscore: the identifier rune of the
// C-family languages.
func isIdent(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// wordAt reports whether w starts at the cursor as a whole word.
func (s *scanner) wordAt(w string, part func(rune) bool) bool {
	if !s.at(w) {
		return false
	}
	end := s.i + len([]rune(w))
	return end == len(s.rs) || !part(s.rs[end])
}

// set is a word list.
type set map[string]bool

// newSet builds a set from words.
func newSet(words ...string) set {
	out := make(set, len(words))
	for _, w := range words {
		out[w] = true
	}
	return out
}
