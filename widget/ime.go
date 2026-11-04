// Input method support: the widget side of zwp_text_input_v3. Editable
// widgets show composing (preedit) text and receive committed text
// through IMEClient, and expose their caret state through IMETracker so
// the host can push surrounding text and the caret rectangle.
package widget

import (
	"unicode/utf8"

	"github.com/stubbedev/lys-lib/render"
)

// IMEClient is the widget side of an input method: an editable widget
// that shows composing (preedit) text and receives committed text. The
// methods are named after the zwp_text_input_v3 events they apply, and
// the host calls them in the done event's order.
type IMEClient interface {
	// IMEPreedit shows composing text at the caret; cursorBegin and
	// cursorEnd are byte offsets into text marking the composing
	// caret (both -1 hide it). Empty text ends composing. The
	// composing text is display-only: Text and OnChanged stay
	// untouched until a commit lands.
	IMEPreedit(text string, cursorBegin, cursorEnd int)
	// IMEDelete removes up to before bytes before the caret and
	// after bytes after it, replacing any composing display.
	IMEDelete(before, after int)
	// IMECommit inserts committed text at the caret, replacing any
	// composing display.
	IMECommit(text string)
}

// IMETracker exposes an editable widget's text-input state for the
// set_surrounding_text and set_cursor_rectangle requests.
type IMETracker interface {
	// IMERevision names the text and caret state IMESurrounding reports:
	// while it stays equal, IMESurrounding would answer the same, so a
	// host compares it instead of building the text.
	IMERevision() IMERevision
	// IMESurrounding returns at most limit bytes of text around the
	// caret, on rune boundaries, with the cursor and anchor as byte
	// offsets into it (an anchor outside the text sits at its edge),
	// composing text excluded.
	IMESurrounding(limit int) (text string, cursor, anchor int)
	// IMECursorRect returns the caret rectangle in root coordinates,
	// for the input method's candidate window.
	IMECursorRect() render.Rect
	// IMEMultiline reports multi-line content.
	IMEMultiline() bool
}

// IMERevision is a comparable stamp of one editable widget's text and
// caret; see IMETracker.IMERevision.
type IMERevision struct {
	text           uint64
	cursor, anchor pos
}

// imeRunesForBytes counts the whole runes covering the first n bytes of
// s; negative n means a hidden caret and stays -1.
func imeRunesForBytes(s string, n int) int {
	if n < 0 {
		return -1
	}
	return utf8.RuneCountInString(s[:min(n, len(s))])
}

// imePrev is the position one rune or line break before p and that
// step's bytes, zero at the start of lines.
func imePrev(lines [][]rune, p pos) (pos, int) {
	switch {
	case p.col > 0:
		return pos{p.line, p.col - 1}, utf8.RuneLen(lines[p.line][p.col-1])
	case p.line > 0:
		return pos{p.line - 1, len(lines[p.line-1])}, 1
	}
	return p, 0
}

// imeNext is the position one rune or line break after p and that
// step's bytes, zero at the end of lines.
func imeNext(lines [][]rune, p pos) (pos, int) {
	switch {
	case p.col < len(lines[p.line]):
		return pos{p.line, p.col + 1}, utf8.RuneLen(lines[p.line][p.col])
	case p.line < len(lines)-1:
		return pos{p.line + 1, 0}, 1
	}
	return p, 0
}

// imeStep is imePrev or imeNext.
type imeStep func(lines [][]rune, p pos) (pos, int)

// imeSpan walks from p until n bytes are covered or lines end; a rune
// straddling n is covered whole.
func imeSpan(lines [][]rune, p pos, n int, step imeStep) pos {
	for n > 0 {
		next, size := step(lines, p)
		if size == 0 {
			break
		}
		p, n = next, n-size
	}
	return p
}

// imeFit walks from p while the next rune still fits n bytes, returning
// where it stopped and the bytes left over.
func imeFit(lines [][]rune, p pos, n int, step imeStep) (pos, int) {
	for {
		next, size := step(lines, p)
		if size == 0 || size > n {
			return p, n
		}
		p, n = next, n-size
	}
}

func posBefore(a, b pos) bool {
	return a.line < b.line || (a.line == b.line && a.col < b.col)
}

// imeWindow is at most limit bytes of lines around cursor: half the
// budget behind it, the rest ahead, and what ahead left over behind.
// Lines join with newlines, and a document under the limit comes back
// whole.
func imeWindow(lines [][]rune, cursor, anchor pos, limit int) (string, int, int) {
	lo, spare := imeFit(lines, cursor, limit/2, imePrev)
	hi, spare := imeFit(lines, cursor, limit-limit/2+spare, imeNext)
	lo, spare = imeFit(lines, lo, spare, imePrev)
	if posBefore(anchor, lo) {
		anchor = lo
	}
	if posBefore(hi, anchor) {
		anchor = hi
	}
	buf := make([]byte, 0, limit-spare)
	cursorAt, anchorAt := 0, 0
	for p := lo; ; {
		if p == cursor {
			cursorAt = len(buf)
		}
		if p == anchor {
			anchorAt = len(buf)
		}
		if p == hi {
			return string(buf), cursorAt, anchorAt
		}
		if p.col < len(lines[p.line]) {
			buf = utf8.AppendRune(buf, lines[p.line][p.col])
		} else {
			buf = append(buf, '\n')
		}
		p, _ = imeNext(lines, p)
	}
}

// imePreeditCaret maps a preedit event's byte cursor range to a rune
// offset: the end offset when only the end is set, the begin otherwise,
// and -1 (hidden) when both are unset.
func imePreeditCaret(text string, cursorBegin, cursorEnd int) int {
	switch {
	case cursorBegin < 0 && cursorEnd < 0:
		return -1
	case cursorBegin < 0:
		return imeRunesForBytes(text, cursorEnd)
	default:
		return imeRunesForBytes(text, cursorBegin)
	}
}

// composing reports whether composing text is on show.
func (e *Entry) composing() bool { return e.peText != nil }

// Composing reports whether composing (input-method preedit) text is
// on show.
func (e *Entry) Composing() bool { return e.composing() }

// clearPreedit drops the composing display, restoring the caret to the
// insertion point it was composing at.
func (e *Entry) clearPreedit() {
	if !e.composing() {
		return
	}
	e.peText = nil
	e.peDisp, e.peDispStr = nil, ""
	e.cursor, e.anchor = e.peAt, e.peAt
}

// displayRunes is the text on show: the contents with any composing
// text spliced in at the caret. The splice and its string form are
// memoized — the composing frame reads them several times — and every
// mutation clears the preedit first, so the memo cannot outlive the
// text it was built from.
func (e *Entry) displayRunes() []rune {
	if !e.composing() {
		return e.runes
	}
	if e.peDisp == nil {
		out := make([]rune, 0, len(e.runes)+len(e.peText))
		out = append(out, e.runes[:e.peAt]...)
		out = append(out, e.peText...)
		out = append(out, e.runes[e.peAt:]...)
		e.peDisp, e.peDispStr = out, string(out)
	}
	return e.peDisp
}

// caretRune is the caret position in display coordinates, -1 while the
// input method hides its composing caret.
func (e *Entry) caretRune() int {
	if !e.composing() {
		return e.cursor
	}
	if e.peCur < 0 {
		return -1
	}
	return e.peAt + e.peCur
}

// IMEPreedit implements IMEClient. A disabled or read-only entry never
// enters composing: the input method is told nothing back, so it stops
// sending preedit after the first empty commit.
func (e *Entry) IMEPreedit(text string, cursorBegin, cursorEnd int) {
	if text == "" {
		e.clearPreedit()
		return
	}
	if !e.editable() {
		return
	}
	// The composing text replaces a selection: the protocol removes
	// the selected text when the preedit applies.
	if _, _, active := e.Selection(); active {
		e.collapse()
	}
	e.peText = []rune(text)
	e.peAt = e.cursor
	e.peCur = imePreeditCaret(text, cursorBegin, cursorEnd)
	e.peDisp, e.peDispStr = nil, ""
	e.panToCaret()
}

// IMEDelete implements IMEClient: input-method deletion of up to
// before bytes before and after bytes after the caret. Blocked while
// disabled or read-only.
func (e *Entry) IMEDelete(before, after int) {
	if !e.editable() {
		return
	}
	snap := e.snapshot()
	e.clearPreedit()
	lines := [][]rune{e.runes}
	start := imeSpan(lines, pos{0, e.cursor}, before, imePrev).col
	end := imeSpan(lines, pos{0, e.cursor}, after, imeNext).col
	if start >= end {
		return
	}
	e.setRunes(append(e.runes[:start], e.runes[end:]...))
	e.cursor, e.anchor = start, start
	e.panToCaret()
	e.hist.record(snap, e.snapshot())
	e.changed()
}

// IMECommit implements IMEClient; the insertion inside is what
// actually honors the disabled and read-only gates.
func (e *Entry) IMECommit(text string) {
	e.clearPreedit()
	if text != "" {
		e.Insert(text)
	}
}

// IMERevision implements IMETracker.
func (e *Entry) IMERevision() IMERevision {
	return IMERevision{text: e.textRev, cursor: pos{0, e.cursor}, anchor: pos{0, e.anchor}}
}

// IMESurrounding implements IMETracker.
func (e *Entry) IMESurrounding(limit int) (string, int, int) {
	return imeWindow([][]rune{e.runes}, pos{0, e.cursor}, pos{0, e.anchor}, limit)
}

// IMECursorRect implements IMETracker: the caret band in root
// coordinates, for the input method's candidate window. Pan-aware, so
// the window tracks the caret inside a panning field, on either
// direction's reading edge.
func (e *Entry) IMECursorRect() render.Rect {
	caret := e.caretRune()
	if caret < 0 {
		caret = e.peAt + len(e.peText)
	}
	x := e.caretX(e.shape(e.displayText()), caret)
	y, h := e.caretBand()
	return render.Rect{X: x, Y: y, W: 2, H: h}
}

// caretBand is the vertical extent of the text line: the caret's, and
// every character's.
func (e *Entry) caretBand() (y, h int) { return e.bounds.Y + 6, max(e.bounds.H-12, 0) }

// IMEMultiline implements IMETracker.
func (e *Entry) IMEMultiline() bool { return false }

// composing reports whether composing text is on show.
func (t *TextArea) composing() bool { return t.peText != nil }

// Composing reports whether composing (input-method preedit) text is
// on show.
func (t *TextArea) Composing() bool { return t.composing() }

// clearPreedit drops the composing display, restoring the caret to the
// insertion point it was composing at and scheduling a row-cache
// rebuild over the plain contents.
func (t *TextArea) clearPreedit() {
	if !t.composing() {
		return
	}
	t.peText = nil
	t.peLine = nil
	t.cursor, t.anchor = t.peAt, t.peAt
	t.hasPref = false
	t.rowsValid = false
}

// displayLine returns logical line l as painted: the contents with any
// composing text spliced in at the caret, so the visual row cache and
// painting always describe what is on show. The splice is memoized —
// every frame reads it several times — and every content mutation
// clears the preedit first, so the memo cannot outlive the text it
// was built from.
func (t *TextArea) displayLine(l int) []rune {
	line := t.lines[l]
	if !t.composing() || l != t.peAt.line {
		return line
	}
	if t.peLine == nil {
		out := make([]rune, 0, len(line)+len(t.peText))
		out = append(out, line[:t.peAt.col]...)
		out = append(out, t.peText...)
		out = append(out, line[t.peAt.col:]...)
		t.peLine = out
	}
	return t.peLine
}

// caretPos is the caret as a display-space position: inside the
// composing text while composing.
func (t *TextArea) caretPos() pos {
	if !t.composing() || t.peCur < 0 {
		return t.cursor
	}
	return pos{t.peAt.line, t.peAt.col + t.peCur}
}

// IMEPreedit implements IMEClient. A disabled or read-only area never
// enters composing (see Entry.IMEPreedit).
func (t *TextArea) IMEPreedit(text string, cursorBegin, cursorEnd int) {
	if text == "" {
		t.clearPreedit()
		return
	}
	if !t.editable() {
		return
	}
	// The composing text replaces a selection: the protocol removes
	// the selected text when the preedit applies.
	if _, _, active := t.Selection(); active {
		t.collapse()
	}
	t.peText = []rune(text)
	t.peAt = t.clamp(t.cursor)
	t.peCur = imePreeditCaret(text, cursorBegin, cursorEnd)
	t.peLine = nil
	t.hasPref = false
	t.rowsValid = false
	t.panToCaret()
}

// IMEDelete implements IMEClient: input-method deletion of up to
// before bytes before and after bytes after the caret, possibly
// across line breaks. Blocked while disabled or read-only.
func (t *TextArea) IMEDelete(before, after int) {
	if !t.editable() {
		return
	}
	snap := t.snapshot()
	t.clearPreedit()
	if before <= 0 && after <= 0 {
		return
	}
	at := t.clamp(t.cursor)
	start := imeSpan(t.lines, at, before, imePrev)
	end := imeSpan(t.lines, at, after, imeNext)
	if start == end {
		return
	}
	t.anchor, t.cursor = start, end
	t.collapse()
	t.hasPref = false
	t.recordEdit(snap)
}

// IMECommit implements IMEClient.
func (t *TextArea) IMECommit(text string) {
	t.clearPreedit()
	if text != "" {
		t.Insert(text)
	}
}

// IMERevision implements IMETracker.
func (t *TextArea) IMERevision() IMERevision {
	return IMERevision{text: t.textRev, cursor: t.cursor, anchor: t.anchor}
}

// IMESurrounding implements IMETracker.
func (t *TextArea) IMESurrounding(limit int) (string, int, int) {
	return imeWindow(t.lines, t.clamp(t.cursor), t.clamp(t.anchor), limit)
}

// IMECursorRect implements IMETracker: the caret rectangle in root
// coordinates, for the input method's candidate window. Pan-aware so
// the window tracks the caret on a panned unwrapped line, on either
// direction's reading edge.
func (t *TextArea) IMECursorRect() render.Rect {
	t.ensureRows(t.wrapWidth())
	caret := t.caretPos()
	if t.composing() && t.peCur < 0 {
		caret = pos{t.peAt.line, t.peAt.col + len(t.peText)}
	}
	row := t.rowOf(caret)
	return render.Rect{
		X: t.caretX(caret.line, caret.col),
		Y: t.textRect().Y + row*t.lineHeight(),
		W: 2,
		H: t.lineHeight(),
	}
}

// IMEMultiline implements IMETracker.
func (t *TextArea) IMEMultiline() bool { return true }
