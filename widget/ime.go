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
	// IMESurrounding returns the text around the caret with the
	// cursor and anchor as byte offsets into it, composing text
	// excluded.
	IMESurrounding() (text string, cursor, anchor int)
	// IMECursorRect returns the caret rectangle in root coordinates,
	// for the input method's candidate window.
	IMECursorRect() render.Rect
	// IMEMultiline reports multi-line content.
	IMEMultiline() bool
}

// imeRunesForBytes counts the whole runes covering the first n bytes of
// s; negative n means a hidden caret and stays -1.
func imeRunesForBytes(s string, n int) int {
	if n < 0 {
		return -1
	}
	return utf8.RuneCountInString(s[:min(n, len(s))])
}

// imeByteOffset is the byte offset of rune index i in rs, clamped.
func imeByteOffset(rs []rune, i int) int {
	n := 0
	for _, r := range rs[:min(max(i, 0), len(rs))] {
		n += utf8.RuneLen(r)
	}
	return n
}

// imeRunesBack counts how many runes ending at index i span n bytes,
// stopping at the start of rs when the bytes run out.
func imeRunesBack(rs []rune, i, n int) int {
	cnt := 0
	for i > 0 && n > 0 {
		i--
		n -= utf8.RuneLen(rs[i])
		cnt++
	}
	return cnt
}

// imeRunesFwd counts how many runes starting at index i span n bytes,
// stopping at the end of rs when the bytes run out.
func imeRunesFwd(rs []rune, i, n int) int {
	cnt := 0
	for i < len(rs) && n > 0 {
		n -= utf8.RuneLen(rs[i])
		i++
		cnt++
	}
	return cnt
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
	start := e.cursor - imeRunesBack(e.runes, e.cursor, before)
	end := e.cursor + imeRunesFwd(e.runes, e.cursor, after)
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

// IMESurrounding implements IMETracker: the contents with the caret
// and anchor as byte offsets, composing text excluded.
func (e *Entry) IMESurrounding() (string, int, int) {
	return string(e.runes), imeByteOffset(e.runes, e.cursor), imeByteOffset(e.runes, e.anchor)
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
	doc := []rune(t.Text())
	at := t.docRuneIndex(t.cursor)
	start := at - imeRunesBack(doc, at, before)
	end := at + imeRunesFwd(doc, at, after)
	if start >= end {
		return
	}
	t.setDoc(string(doc[:start])+string(doc[end:]), start)
	t.recordEdit(snap)
}

// IMECommit implements IMEClient.
func (t *TextArea) IMECommit(text string) {
	t.clearPreedit()
	if text != "" {
		t.Insert(text)
	}
}

// docRuneIndex maps a line/column position to a rune index in Text().
func (t *TextArea) docRuneIndex(p pos) int {
	p = t.clamp(p)
	n := 0
	for _, l := range t.lines[:p.line] {
		n += len(l) + 1
	}
	return n + p.col
}

// setDoc replaces the contents and parks the caret at rune index at
// (newlines included in the count). Unlike SetText it leaves the undo
// history alone — callers record the edit themselves.
func (t *TextArea) setDoc(s string, at int) {
	t.splitLines(s)
	p := pos{0, 0}
	for at > 0 && p.line < len(t.lines) {
		switch take := min(len(t.lines[p.line])-p.col, at); {
		case take > 0:
			p.col += take
			at -= take
		default:
			p.line, p.col = p.line+1, 0
			at--
		}
	}
	t.cursor, t.anchor = t.clamp(p), t.clamp(p)
	t.hasPref = false
	t.rowsValid = false
	t.panToCaret()
	t.InvalidateLayout()
}

// IMESurrounding implements IMETracker: the document with the caret
// and anchor as byte offsets, composing text excluded.
func (t *TextArea) IMESurrounding() (string, int, int) {
	return t.Text(), t.byteOffset(t.cursor), t.byteOffset(t.anchor)
}

// byteOffset is the byte offset of a document position in Text().
func (t *TextArea) byteOffset(p pos) int {
	p = t.clamp(p)
	n := 0
	for _, l := range t.lines[:p.line] {
		n += imeByteOffset(l, len(l)) + 1
	}
	return n + imeByteOffset(t.lines[p.line], p.col)
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
