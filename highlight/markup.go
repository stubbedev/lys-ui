package highlight

import (
	"regexp"
	"slices"
	"strings"

	"github.com/stubbedev/lys-ui/widget"
)

// Tags highlights HTML and XML as GtkSourceView's html.lang and
// xml.lang do: tags and their names (def:keyword), attribute names
// (def:type) and quoted values (def:string), character references
// (def:special-char), comments, and declarations and processing
// instructions (def:preprocessor). A comment, a tag whose attributes
// run over lines, and a quoted value carry over through the line state.
type Tags struct{}

var _ widget.Highlighter = Tags{}

// The Tags line states.
const (
	tagsText = iota
	tagsComment
	tagsInTag
	tagsDouble
	tagsSingle
)

var tagsEntity = regexp.MustCompile(`^&(?:#\d+|#[xX][\da-fA-F]+|[A-Za-z][A-Za-z\d]*);`)

// isTagName is a tag or attribute name's rune.
func isTagName(r rune) bool { return isIdent(r) || r == '-' || r == ':' || r == '.' }

// Highlight implements widget.Highlighter.
func (Tags) Highlight(line []rune, state int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	switch state {
	case tagsComment:
		if !s.until("-->", styleComment) {
			return s.spans, tagsComment
		}
		state = tagsText
	case tagsDouble, tagsSingle:
		quote := `"`
		if state == tagsSingle {
			quote = "'"
		}
		if !s.stringTo(quote, false, styleString) {
			return s.spans, state
		}
		state = tagsInTag
	}
	for !s.done() {
		if state == tagsInTag {
			state = tagsAttribute(s)
			if state != tagsInTag && state != tagsText {
				return s.spans, state
			}
			continue
		}
		switch r := s.rs[s.i]; {
		case s.at("<!--"):
			if !s.delimited("<!--", "-->", styleComment) {
				return s.spans, tagsComment
			}
		case s.at("<!") || s.at("<?"):
			s.until(">", stylePreproc)
		case r == '<' && (isTagName(s.peek(1)) || s.peek(1) == '/' && isTagName(s.peek(2))):
			start := s.i
			s.i++
			if s.peek(0) == '/' {
				s.i++
			}
			s.skip(isTagName)
			s.span(start, s.i, styleKeyword)
			state = tagsInTag
		case r == '&' && s.matchShort(tagsEntity, 32) > 0:
			s.take(s.matchShort(tagsEntity, 32), styleEscape)
		default:
			s.i++
		}
	}
	return s.spans, state
}

// tagsAttribute scans one step inside a tag and returns the state after
// it: still in the tag, back in text past its end, or in a quoted value
// the line leaves open.
func tagsAttribute(s *scanner) int {
	switch r := s.rs[s.i]; {
	case s.at("/>"):
		s.take(2, styleKeyword)
		return tagsText
	case r == '>':
		s.take(1, styleKeyword)
		return tagsText
	case r == '"' || r == '\'':
		if !s.quoted(r, true, false, styleString) {
			if r == '"' {
				return tagsDouble
			}
			return tagsSingle
		}
	case isTagName(r):
		start := s.i
		s.skip(isTagName)
		s.span(start, s.i, styleType)
	default:
		s.i++
	}
	return tagsInTag
}

// CSS highlights CSS as GtkSourceView's css.lang does: comments,
// at-rules (def:preprocessor), selectors' element names (def:keyword),
// classes and ids (def:type), property names (def:type), numbers with
// their units, colors (def:constant), strings, functions (def:builtin)
// and !important (def:keyword). A comment carries over through the
// line state, as does how deep in braces the line starts.
type CSS struct{}

var _ widget.Highlighter = CSS{}

// The CSS line state: bit 0 an open comment, the rest the brace depth.
const (
	cssComment    = 1
	cssDepthShift = 1
)

var (
	cssNumber = regexp.MustCompile(`^-?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?(?:%|[a-zA-Z]+)?`)
	cssColor  = regexp.MustCompile(`^#[\da-fA-F]{3,8}\b`)
)

func isCSSIdent(r rune) bool { return isIdent(r) || r == '-' }

// Highlight implements widget.Highlighter.
func (CSS) Highlight(line []rune, state int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	depth := state >> cssDepthShift
	if state&cssComment != 0 && !s.until("*/", styleComment) {
		return s.spans, state
	}
	for !s.done() {
		switch r := s.rs[s.i]; {
		case s.at("/*"):
			if !s.delimited("/*", "*/", styleComment) {
				return s.spans, cssComment | depth<<cssDepthShift
			}
		case r == '"' || r == '\'':
			s.quoted(r, true, true, styleString)
		case r == '{':
			depth++
			s.i++
		case r == '}':
			depth = max(depth-1, 0)
			s.i++
		case r == '@' && isCSSIdent(s.peek(1)):
			start := s.i
			s.i++
			s.skip(isCSSIdent)
			s.span(start, s.i, stylePreproc)
		case s.at("!important"):
			s.take(len("!important"), styleKeyword)
		case r == '#' && s.matchShort(cssColor, 10) > 0 && depth > 0 && cssInValue(s):
			s.take(s.matchShort(cssColor, 10), styleConstant)
		case (r == '.' || r == '#') && isCSSIdent(s.peek(1)) && (depth == 0 || !cssInValue(s)):
			start := s.i
			s.i++
			s.skip(isCSSIdent)
			s.span(start, s.i, styleType)
		case r >= '0' && r <= '9' || (r == '.' || r == '-') && s.peek(1) >= '0' && s.peek(1) <= '9':
			s.take(max(s.matchShort(cssNumber, 32), 1), styleDecimal)
		case isCSSIdent(r):
			start := s.i
			s.skip(isCSSIdent)
			switch {
			case s.peek(0) == '(':
				s.span(start, s.i, styleBuiltin)
			case depth > 0 && s.keyFollows():
				s.span(start, s.i, styleType)
			case depth == 0:
				s.span(start, s.i, styleKeyword)
			}
		default:
			s.i++
		}
	}
	return s.spans, depth << cssDepthShift
}

// cssInValue reports whether the cursor is in a declaration's value: a
// ':' before it on the line, after the last '{' or ';'.
func cssInValue(s *scanner) bool {
	for j := s.i - 1; j >= 0; j-- {
		switch s.rs[j] {
		case ':':
			return true
		case '{', ';', '}':
			return false
		}
	}
	return false
}

// INI highlights INI-style files - .ini, .conf, .desktop, systemd units,
// .gitconfig - as GtkSourceView's ini.lang does: comment lines (';' or
// '#'), [section] headers (def:keyword), keys (def:type), and values'
// booleans and numbers. Every line stands alone.
type INI struct{}

var _ widget.Highlighter = INI{}

var iniBooleans = words("true false yes no on off")

// Highlight implements widget.Highlighter.
func (INI) Highlight(line []rune, _ int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	s.skipSpace()
	if s.done() {
		return s.spans, 0
	}
	switch s.rs[s.i] {
	case ';', '#':
		s.rest(styleComment)
		return s.spans, 0
	case '[':
		s.until("]", styleKeyword)
		return s.spans, 0
	}
	start := s.i
	for !s.done() && s.rs[s.i] != '=' && s.rs[s.i] != ':' {
		s.i++
	}
	if s.done() {
		return s.spans, 0
	}
	end := s.i
	for end > start && (s.rs[end-1] == ' ' || s.rs[end-1] == '\t') {
		end--
	}
	s.span(start, end, styleType)
	s.i++
	s.skipSpace()
	value := strings.TrimSpace(string(s.rs[s.i:]))
	switch {
	case value == "":
	case iniBooleans[strings.ToLower(value)]:
		s.rest(styleBoolean)
	case s.match(cNumbers[2].re) == len(s.rs)-s.i:
		s.rest(styleDecimal)
	case value[0] == '"':
		s.quoted('"', true, true, styleString)
	}
	return s.spans, 0
}

// Diff highlights unified and context diffs as GtkSourceView's
// diff.lang does: file headers (def:keyword), hunk locations
// (def:preprocessor), and added and removed lines (def:inserted,
// def:deleted). Every line stands alone.
type Diff struct{}

var _ widget.Highlighter = Diff{}

var diffHeaders = []string{"diff ", "index ", "--- ", "+++ ", "*** ", "new file", "deleted file", "similarity ", "rename ", "old mode", "new mode", "Only in ", "Binary files "}

// Highlight implements widget.Highlighter.
func (Diff) Highlight(line []rune, _ int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line}
	switch {
	case slices.ContainsFunc(diffHeaders, s.at):
		s.rest(styleKeyword)
	case s.at("@@"):
		s.rest(stylePreproc)
	case s.at("+") || s.at(">"):
		s.rest(styleInserted)
	case s.at("-") || s.at("<"):
		s.rest(styleDeleted)
	case s.at("!"):
		s.rest(styleKeyword)
	}
	return s.spans, 0
}
