package highlight

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stubbedev/lys-ui/widget"
)

// lexicon is a language told by its words and delimiters, which one
// scanner highlights: comments, strings with their escapes, numbers,
// and word lists, with the few extras a family adds (a preprocessor
// line, decorators, attributes, shell variables). Most languages are
// one of these; those that are not (Markdown, YAML, HTML) have their
// own state machines.
type lexicon struct {
	// lineComments start a comment to the line's end. A comment marker
	// that starts with a word rune ("REM") must stand alone as a word.
	lineComments []string
	// hashAfterSpace makes '#' a comment only at the line's start or
	// after a space, as the shells read it ($#, a#b are not comments).
	hashAfterSpace bool
	// blockComments are delimited comments, which may span lines.
	blockComments []delims
	// strings are the string forms, longest opener first.
	strings []stringForm
	// char is a character literal ('a', '\n') where a quote alone is
	// something else (a Rust lifetime).
	char *regexp.Regexp
	// preprocessor starts a preprocessor line at the line's first rune
	// ('#' in C).
	preprocessor string
	// decorators style @name (Python, Java) as def:preprocessor.
	decorators bool
	// attributes style #[...] and #![...] (Rust) as def:preprocessor.
	attributes bool
	// variables style $name, ${...} and $(...) as def:identifier.
	variables bool
	// macros style a word followed by '!' (Rust's println!) as
	// def:preprocessor.
	macros bool
	// targets style "name:" at a line's start (make) as def:function.
	targets bool
	// foldCase matches words in any case (SQL, Dockerfile).
	foldCase bool
	// keywords, types, constants, booleans and builtins are the word
	// lists; builtins are styled only before '(' when callsOnly.
	keywords, types, constants, booleans, builtins set
	callsOnly                                      bool
	// numbers are the numeric forms, longest first; nil is cNumbers.
	numbers []form
	// identPart is the identifier rune; nil is isIdent.
	identPart func(rune) bool
}

// lexiconSpans is the spans a line is scanned into before growing them:
// most code lines style no more.
const lexiconSpans = 8

// charWindow and numberWindow bound the runes a character literal and a
// number are matched over.
const (
	charWindow   = 16
	numberWindow = 64
)

// delims is an opening and a closing delimiter.
type delims struct{ open, close string }

// stringForm is a string's delimiters, whether backslash escapes
// apply, and whether it may span lines.
type stringForm struct {
	open, close string
	escapes     bool
	multiline   bool
}

// cNumbers are the C family's numeric literals: hex, octal and binary
// with digit separators, floats with exponents, and integer and float
// suffixes (u, l, f, i32, f64, n).
var cNumbers = []form{
	{regexp.MustCompile(`^0[xX][\da-fA-F_']+(?:\.[\da-fA-F_]*)?(?:[pP][+-]?\d+)?[a-zA-Z\d]*|^0[oObB][0-7_']+[a-zA-Z\d]*`), styleBaseN},
	{regexp.MustCompile(`^(?:\d[\d_']*\.[\d_']*(?:[eE][+-]?\d+)?|\d[\d_']*[eE][+-]?\d+|\.\d[\d_']*(?:[eE][+-]?\d+)?)[a-zA-Z\d]*`), styleFloat},
	{regexp.MustCompile(`^\d[\d_']*[a-zA-Z\d]*`), styleDecimal},
}

var _ widget.Highlighter = (*lexicon)(nil)

// The lexicon's line state: 0 outside, then one id per block comment
// and per multi-line string form, in order.
func (l *lexicon) commentState(i int) int { return 1 + i }
func (l *lexicon) stringState(i int) int  { return 1 + len(l.blockComments) + i }

// Highlight implements widget.Highlighter.
func (l *lexicon) Highlight(line []rune, state int) ([]widget.TextSpan, int) {
	s := &scanner{rs: line, spans: make([]widget.TextSpan, 0, lexiconSpans)}
	if state > 0 && !l.resume(s, state) {
		return s.spans, state
	}
	for !s.done() {
		if end, open := l.step(s); open {
			return s.spans, end
		}
	}
	return s.spans, 0
}

// resume carries on the construct state names; true when it closed on
// this line.
func (l *lexicon) resume(s *scanner, state int) bool {
	if i := state - 1; i < len(l.blockComments) {
		return s.until(l.blockComments[i].close, styleComment)
	}
	f := l.strings[state-1-len(l.blockComments)]
	return s.stringTo(f.close, f.escapes, styleString)
}

// step scans one token at the cursor; open reports a construct left open
// at the line's end, end the state that carries it.
func (l *lexicon) step(s *scanner) (end int, open bool) {
	r := s.rs[s.i]
	part := l.identPart
	if part == nil {
		part = isIdent
	}
	switch {
	case r == ' ' || r == '\t':
		s.i++
	case l.preprocessor != "" && s.atLineStart() && s.at(l.preprocessor):
		s.rest(stylePreproc)
	case l.targets && s.i == 0 && targetEnd(s.rs) > 0:
		s.take(targetEnd(s.rs), styleFunction)
	case l.blockComment(s):
		for i, d := range l.blockComments {
			if s.at(d.open) {
				if !s.delimited(d.open, d.close, styleComment) {
					return l.commentState(i), true
				}
				break
			}
		}
	case l.lineComment(s, part):
		s.rest(styleComment)
	case l.attributes && (s.at("#[") || s.at("#![")):
		s.until("]", stylePreproc)
	case l.stringAt(s) >= 0:
		i := l.stringAt(s)
		f := l.strings[i]
		from := s.i
		s.i += len([]rune(f.open))
		if !s.stringFrom(from, f.close, f.escapes, styleString) && f.multiline {
			return l.stringState(i), true
		}
	case l.char != nil && r == '\'' && s.matchShort(l.char, charWindow) > 0:
		s.take(s.matchShort(l.char, charWindow), styleChar)
	case l.decorators && r == '@' && part(s.peek(1)):
		start := s.i
		s.i++
		s.skip(func(r rune) bool { return part(r) || r == '.' })
		s.span(start, s.i, stylePreproc)
	case l.variables && r == '$':
		l.variable(s, part)
	case r >= '0' && r <= '9' || r == '.' && s.peek(1) >= '0' && s.peek(1) <= '9':
		numbers := l.numbers
		if numbers == nil {
			numbers = cNumbers
		}
		if !s.firstShort(numbers, numberWindow) {
			s.i++
		}
	case part(r):
		l.classify(s, part)
	default:
		s.i++
	}
	return 0, false
}

// blockComment reports whether a block comment opens at the cursor.
func (l *lexicon) blockComment(s *scanner) bool {
	for _, d := range l.blockComments {
		if s.at(d.open) {
			return true
		}
	}
	return false
}

// lineComment reports whether a line comment starts at the cursor.
func (l *lexicon) lineComment(s *scanner, part func(rune) bool) bool {
	for _, c := range l.lineComments {
		if !s.at(c) {
			continue
		}
		if c == "#" && l.hashAfterSpace && s.i > 0 && s.rs[s.i-1] != ' ' && s.rs[s.i-1] != '\t' {
			continue
		}
		first := []rune(c)[0]
		if part(first) && (s.i > 0 && part(s.rs[s.i-1]) || !s.wordAt(c, part)) {
			continue
		}
		return true
	}
	return false
}

// stringAt is the index of the string form opening at the cursor, -1
// for none.
func (l *lexicon) stringAt(s *scanner) int {
	for i, f := range l.strings {
		if s.at(f.open) {
			return i
		}
	}
	return -1
}

// variable styles $name, ${...}, $(...) and the special $1, $@, $?.
func (l *lexicon) variable(s *scanner, part func(rune) bool) {
	start := s.i
	s.i++
	switch next := s.peek(0); {
	case next == '{':
		s.untilFrom(start, "}", styleIdentifier)
		return
	case next == '(':
		s.untilFrom(start, ")", styleIdentifier)
		return
	case part(next):
		s.skip(part)
	case next != 0 && strings.ContainsRune("@*#?$!-0123456789", next):
		s.i++
	}
	s.span(start, s.i, styleIdentifier)
}

// classify styles the word at the cursor from the word lists.
func (l *lexicon) classify(s *scanner, part func(rune) bool) {
	start := s.i
	s.skip(part)
	var buf [32]byte
	w := s.wordKey(buf[:], start, l.foldCase)
	switch {
	case l.macros && s.peek(0) == '!' && s.peek(1) != '=':
		s.i++
		s.span(start, s.i, stylePreproc)
	case l.keywords[string(w)]:
		s.span(start, s.i, styleKeyword)
	case l.types[string(w)]:
		s.span(start, s.i, styleType)
	case l.booleans[string(w)]:
		s.span(start, s.i, styleBoolean)
	case l.constants[string(w)]:
		s.span(start, s.i, styleSpecialConst)
	case l.builtins[string(w)] && (!l.callsOnly || s.peek(0) == '('):
		s.span(start, s.i, styleBuiltin)
	}
}

// atLineStart reports whether only spaces come before the cursor.
func (s *scanner) atLineStart() bool {
	for _, r := range s.rs[:s.i] {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

// stringTo styles from the cursor through close as str, backslash
// escapes apart as def:special-char when escapes is set, reporting
// whether close came on this line (else the rest is styled).
func (s *scanner) stringTo(close string, escapes bool, str string) bool {
	return s.stringFrom(s.i, close, escapes, str)
}

// stringFrom is stringTo styling from from, before the cursor: the
// string's opening delimiter.
func (s *scanner) stringFrom(from int, close string, escapes bool, str string) bool {
	n := len([]rune(close))
	for s.i < len(s.rs) {
		switch {
		case s.at(close):
			s.i += n
			s.span(from, s.i, str)
			return true
		case escapes && s.rs[s.i] == '\\':
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

// targetEnd is the rune length of a make rule's target at a line's start
// (the colon excluded), 0 when the line is no rule: it starts with a
// space, a tab or a comment, or assigns (=, :=, ::=) before any colon.
func targetEnd(rs []rune) int {
	if len(rs) == 0 || rs[0] == ' ' || rs[0] == '\t' || rs[0] == '#' {
		return 0
	}
	for i, r := range rs {
		switch r {
		case '=':
			return 0
		case ':':
			if i+1 < len(rs) && (rs[i+1] == '=' || rs[i+1] == ':' && i+2 < len(rs) && rs[i+2] == '=') {
				return 0
			}
			return i
		}
	}
	return 0
}

// words splits a space-separated word list into a set.
func words(list string) set { return newSet(strings.Fields(list)...) }

// The C family's shared forms.
var (
	cComments    = []delims{{"/*", "*/"}}
	cStrings     = []stringForm{{open: `"`, close: `"`, escapes: true}}
	cChar        = regexp.MustCompile(`^'(?:\\(?:x[\da-fA-F]+|u\{?[\da-fA-F]+\}?|.)|[^'\\])'`)
	cTypes       = "void char short int long float double signed unsigned bool _Bool size_t ssize_t ptrdiff_t intptr_t uintptr_t int8_t int16_t int32_t int64_t uint8_t uint16_t uint32_t uint64_t wchar_t FILE"
	cKeywords    = "auto break case const continue default do else enum extern for goto if inline register restrict return sizeof static struct switch typedef union volatile while _Alignas _Alignof _Atomic _Generic _Noreturn _Static_assert _Thread_local"
	cppKeywords  = cKeywords + " alignas alignof and asm catch class concept consteval constexpr constinit const_cast co_await co_return co_yield decltype delete dynamic_cast explicit export final friend mutable namespace new noexcept not operator or override private protected public reinterpret_cast requires static_assert static_cast template this thread_local throw try typeid typename using virtual xor"
	jsKeywords   = "as async await break case catch class const continue debugger default delete do else export extends finally for from function get if import in instanceof let new of return set static super switch this throw try typeof var void while with yield"
	jsStrings    = []stringForm{{open: `"`, close: `"`, escapes: true}, {open: "'", close: "'", escapes: true}, {open: "`", close: "`", escapes: true, multiline: true}}
	hashComments = []string{"#"}
)

// The languages the lexicon tells.
var (
	langC = &lexicon{
		lineComments: []string{"//"}, blockComments: cComments, strings: cStrings, char: cChar, preprocessor: "#",
		keywords: words(cKeywords), types: words(cTypes), constants: words("NULL"), booleans: words("true false"),
	}
	langCPP = &lexicon{
		lineComments: []string{"//"}, blockComments: cComments, strings: cStrings, char: cChar, preprocessor: "#",
		keywords: words(cppKeywords), types: words(cTypes + " char8_t char16_t char32_t"), constants: words("NULL nullptr"), booleans: words("true false"),
	}
	langRust = &lexicon{
		lineComments: []string{"//"}, blockComments: cComments, char: cChar, attributes: true, macros: true,
		strings: []stringForm{
			{open: `r#"`, close: `"#`, multiline: true},
			{open: `br"`, close: `"`, multiline: true},
			{open: `r"`, close: `"`, multiline: true},
			{open: `b"`, close: `"`, escapes: true, multiline: true},
			{open: `"`, close: `"`, escapes: true, multiline: true},
		},
		keywords:  words("as async await break const continue crate dyn else enum extern fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait type union unsafe use where while"),
		types:     words("i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64 bool char str String Vec Option Result Box Rc Arc"),
		constants: words("None Some Ok Err"), booleans: words("true false"),
	}
	langJavaScript = &lexicon{
		lineComments: []string{"//"}, blockComments: cComments, strings: jsStrings,
		keywords: words(jsKeywords), constants: words("null undefined NaN Infinity"), booleans: words("true false"),
		builtins: words("console require module exports Object Array String Number Boolean Promise Map Set JSON Math Date RegExp Error Symbol"),
	}
	langTypeScript = &lexicon{
		lineComments: []string{"//"}, blockComments: cComments, strings: jsStrings, decorators: true,
		keywords:  words(jsKeywords + " abstract asserts declare enum implements infer interface is keyof module namespace override private protected public readonly satisfies type"),
		types:     words("any bigint boolean never number object string symbol unknown void"),
		constants: words("null undefined NaN Infinity"), booleans: words("true false"),
		builtins: words("console require Object Array String Number Boolean Promise Map Set JSON Math Date RegExp Error Symbol Record Partial Readonly"),
	}
	langJava = &lexicon{
		lineComments: []string{"//"}, blockComments: cComments, strings: []stringForm{{open: `"""`, close: `"""`, escapes: true, multiline: true}, {open: `"`, close: `"`, escapes: true}},
		char: cChar, decorators: true,
		keywords:  words("abstract assert break case catch class const continue default do else enum extends final finally for goto if implements import instanceof interface native new package permits private protected public record return sealed static strictfp super switch synchronized this throw throws transient try var volatile while yield"),
		types:     words("boolean byte char double float int long short void String Object Integer Long Double Boolean List Map Set"),
		constants: words("null"), booleans: words("true false"),
	}
	langCSharp = &lexicon{
		lineComments: []string{"//"}, blockComments: cComments, strings: []stringForm{{open: `@"`, close: `"`, multiline: true}, {open: `"`, close: `"`, escapes: true}},
		char: cChar, preprocessor: "#",
		keywords:  words("abstract as async await base break case catch checked class const continue default delegate do else enum event explicit extern finally fixed for foreach get goto if implicit in init interface internal is lock namespace new operator out override params partial private protected public readonly record ref required return sealed set sizeof stackalloc static struct switch this throw try typeof unchecked unsafe using var virtual volatile when where while yield"),
		types:     words("bool byte char decimal double dynamic float int long nint nuint object sbyte short string uint ulong ushort void"),
		constants: words("null"), booleans: words("true false"),
	}
	langPython = &lexicon{
		lineComments: hashComments, decorators: true, callsOnly: true,
		strings: []stringForm{
			{open: `"""`, close: `"""`, escapes: true, multiline: true},
			{open: `'''`, close: `'''`, escapes: true, multiline: true},
			{open: `"`, close: `"`, escapes: true},
			{open: "'", close: "'", escapes: true},
		},
		keywords:  words("and as assert async await break case class continue def del elif else except finally for from global if import in is lambda match nonlocal not or pass raise return try while with yield"),
		types:     words("bool bytearray bytes complex dict float frozenset int list object set str tuple type"),
		constants: words("None NotImplemented Ellipsis self cls"), booleans: words("True False"),
		builtins: words("abs all any ascii bin breakpoint callable chr compile delattr dir divmod enumerate eval exec filter format getattr globals hasattr hash help hex id input isinstance issubclass iter len locals map max min next oct open ord pow print property range repr reversed round setattr slice sorted staticmethod classmethod sum super vars zip"),
	}
	langShell = &lexicon{
		lineComments: hashComments, hashAfterSpace: true, variables: true,
		strings:   []stringForm{{open: `"`, close: `"`, escapes: true, multiline: true}, {open: "'", close: "'", multiline: true}},
		keywords:  words("if then else elif fi for in do done while until case esac function select return break continue local export readonly declare typeset unset shift exit time coproc"),
		builtins:  words("alias bg bind builtin cd command compgen complete echo enable eval exec false fg getopts hash jobs kill let mapfile printf pushd popd pwd read set source test trap true type ulimit umask wait"),
		identPart: func(r rune) bool { return isIdent(r) || r == '-' },
	}
	langNix = &lexicon{
		lineComments: hashComments, blockComments: cComments,
		strings:   []stringForm{{open: "''", close: "''", multiline: true}, {open: `"`, close: `"`, escapes: true, multiline: true}},
		keywords:  words("assert else if in inherit let or rec then with"),
		builtins:  words("abort baseNameOf builtins derivation dirOf fetchGit fetchTarball fetchurl import isNull map placeholder removeAttrs throw toString"),
		constants: words("null"), booleans: words("true false"),
		identPart: func(r rune) bool { return isIdent(r) || r == '-' || r == '\'' },
	}
	langRuby = &lexicon{
		lineComments: hashComments,
		strings:      []stringForm{{open: `"`, close: `"`, escapes: true, multiline: true}, {open: "'", close: "'", multiline: true}},
		keywords:     words("alias and begin break case class def do else elsif end ensure for if in module next not or redo rescue retry return then undef unless until when while yield"),
		constants:    words("nil self __FILE__ __LINE__"), booleans: words("true false"),
		builtins: words("attr_accessor attr_reader attr_writer include extend lambda loop p print proc puts raise require require_relative"),
	}
	langLua = &lexicon{
		lineComments: []string{"--"}, blockComments: []delims{{"--[[", "]]"}},
		strings:   []stringForm{{open: "[[", close: "]]", multiline: true}, {open: `"`, close: `"`, escapes: true}, {open: "'", close: "'", escapes: true}},
		keywords:  words("and break do else elseif end for function goto if in local not or repeat return then until while"),
		constants: words("nil"), booleans: words("true false"),
		builtins: words("assert error ipairs next pairs pcall print require select setmetatable getmetatable tonumber tostring type unpack xpcall"),
	}
	langSQL = &lexicon{
		lineComments: []string{"--"}, blockComments: cComments, foldCase: true,
		strings:   []stringForm{{open: "'", close: "'", multiline: true}},
		keywords:  words("add all alter and as asc begin between by case check column commit constraint create cross database default delete desc distinct drop else end exists foreign from full group having if in index inner insert intersect into is join key left like limit not offset on or order outer primary references replace returning right rollback select set table then transaction trigger union unique update using values view when where with"),
		types:     words("bigint blob boolean char date datetime decimal double float int integer interval json jsonb numeric real serial smallint text time timestamp uuid varchar"),
		constants: words("null"), booleans: words("true false"),
	}
	langMake = &lexicon{
		lineComments: hashComments, variables: true, targets: true,
		keywords:  words("define else endef endif export ifdef ifeq ifndef ifneq include override unexport vpath -include sinclude"),
		identPart: func(r rune) bool { return isIdent(r) || r == '-' },
	}
	langDockerfile = &lexicon{
		lineComments: hashComments, variables: true, foldCase: true,
		strings:  []stringForm{{open: `"`, close: `"`, escapes: true}, {open: "'", close: "'"}},
		keywords: words("add arg cmd copy entrypoint env expose from healthcheck label maintainer onbuild run shell stopsignal user volume workdir as"),
	}
)

// skip steps over the run of runes in, as word does without building
// the word.
func (s *scanner) skip(in func(rune) bool) {
	for s.i < len(s.rs) && in(s.rs[s.i]) {
		s.i++
	}
}

// matchShort is match over at most n runes past the cursor: a token's
// pattern (a number, a character literal) never needs the whole line
// copied out.
func (s *scanner) matchShort(re *regexp.Regexp, n int) int {
	var buf [64]byte
	window := buf[:0]
	for _, r := range s.rs[s.i:min(s.i+n, len(s.rs))] {
		window = utf8.AppendRune(window, r)
	}
	loc := re.FindIndex(window)
	if loc == nil {
		return 0
	}
	return utf8.RuneCount(window[:loc[1]])
}

// firstShort is first over a window of n runes.
func (s *scanner) firstShort(forms []form, n int) bool {
	for _, f := range forms {
		if m := s.matchShort(f.re, n); m > 0 {
			s.take(m, f.class)
			return true
		}
	}
	return false
}

// wordKey is the runes [start, s.i) as UTF-8 in buf, lower-cased when
// fold is set: a word-list lookup key that allocates nothing for words
// that fit buf.
func (s *scanner) wordKey(buf []byte, start int, fold bool) []byte {
	key := buf[:0]
	for _, r := range s.rs[start:s.i] {
		if fold {
			r = unicode.ToLower(r)
		}
		key = utf8.AppendRune(key, r)
	}
	return key
}
