package widget

import (
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/text"
)

// TextSelector is a text widget whose caret and selection code - and
// assistive technologies, through the AT-SPI bridge - can set, in rune
// offsets into its Text (the offsets A11yState reports). Positions
// snap to grapheme-cluster starts, composing text is dropped, and the
// view pans to the caret.
type TextSelector interface {
	// SetCaret moves the caret to offset, collapsing any selection.
	SetCaret(offset int)
	// Select selects start..end, the caret at end.
	Select(start, end int)
}

// TextGeometry reports where a text widget draws each character, in
// root coordinates: what a screen reader's review cursor and
// magnifiers follow.
type TextGeometry interface {
	// CharExtents is the box of the character at offset; false outside
	// the text or when nothing is drawn (a hidden password).
	CharExtents(offset int) (render.Rect, bool)
	// OffsetAt is the offset of the character under p, -1 outside the
	// text area.
	OffsetAt(p Point) int
}

// SetCaret implements TextSelector.
func (e *Entry) SetCaret(offset int) { e.Select(offset, offset) }

// Select implements TextSelector.
func (e *Entry) Select(start, end int) {
	e.clearPreedit()
	snap := func(o int) int { return text.SnapCluster(e.runes, min(max(o, 0), len(e.runes))) }
	e.anchor, e.cursor = snap(start), snap(end)
	e.panToCaret()
	e.Invalidate()
}

// CharExtents implements TextGeometry.
func (e *Entry) CharExtents(offset int) (render.Rect, bool) {
	if offset < 0 || offset >= len(e.runes) || e.face == nil || e.echo == EchoNone && !e.reveal {
		return render.Rect{}, false
	}
	d := e.displayOffset(offset)
	sh := e.shape(e.displayText())
	x0, x1 := e.caretX(sh, d), e.caretX(sh, d+1)
	if x1 < x0 {
		x0, x1 = x1, x0 // a right-to-left run
	}
	y, h := e.caretBand()
	return render.Rect{X: x0, Y: y, W: x1 - x0, H: h}, true
}

// OffsetAt implements TextGeometry.
func (e *Entry) OffsetAt(p Point) int {
	if e.face == nil || !e.contentRect().Contains(p.X, p.Y) {
		return -1
	}
	sh := e.shape(e.displayText())
	d := sh.CaretAt(float64(p.X - e.lineX(sh)))
	if e.composing() && d > e.peAt {
		d = max(d-len(e.peText), e.peAt)
	}
	return charAt(e, d, p, len(e.runes))
}

// charAt resolves the caret boundary nearest p to the character whose
// box holds p: the one before the boundary or the one after it, in
// either reading direction. n is the text's length.
func charAt(g TextGeometry, caret int, p Point, n int) int {
	for _, i := range [...]int{caret - 1, caret} {
		if r, ok := g.CharExtents(i); ok && (render.Rect{X: r.X, Y: r.Y, W: max(r.W, 1), H: r.H}).Contains(p.X, p.Y) {
			return i
		}
	}
	return min(max(caret, 0), max(n-1, 0))
}

// displayOffset maps a text offset to the display, where composing
// text sits spliced in at the insertion point.
func (e *Entry) displayOffset(o int) int {
	if e.composing() && o >= e.peAt {
		return o + len(e.peText)
	}
	return o
}

// SetCaret implements TextSelector.
func (t *TextArea) SetCaret(offset int) { t.Select(offset, offset) }

// Select implements TextSelector.
func (t *TextArea) Select(start, end int) {
	t.clearPreedit()
	snap := func(o int) pos {
		p := t.posOf(o)
		return pos{p.line, text.SnapCluster(t.lines[p.line], p.col)}
	}
	t.anchor, t.cursor = snap(start), snap(end)
	t.hasPref = false
	t.panToCaret()
	t.Invalidate()
}

// CharExtents implements TextGeometry. A line's newline reports a
// zero-width box at the line's end.
func (t *TextArea) CharExtents(offset int) (render.Rect, bool) {
	if offset < 0 || offset >= t.runeCount() {
		return render.Rect{}, false
	}
	p := t.displayPos(t.posOf(offset))
	t.ensureRows(t.wrapWidth())
	ri := t.rowOf(p)
	row := t.rows[ri]
	sh, base := t.rowLayout(ri)
	col := p.col - row.startCol
	x0 := base + int(sh.CaretX(col)+0.5)
	x1 := x0
	if p.col < row.endCol {
		x1 = base + int(sh.CaretX(col+1)+0.5)
	}
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	lh := t.lineHeight()
	return render.Rect{X: x0, Y: t.textRect().Y + ri*lh, W: x1 - x0, H: lh}, true
}

// OffsetAt implements TextGeometry.
func (t *TextArea) OffsetAt(p Point) int {
	if !t.textRect().Contains(p.X, p.Y) {
		return -1
	}
	at := t.posAt(p)
	if t.composing() && at.line == t.peAt.line && at.col > t.peAt.col {
		at.col = max(at.col-len(t.peText), t.peAt.col)
	}
	return charAt(t, t.offsetOf(t.clamp(at)), p, t.runeCount())
}

// posOf maps a rune offset into Text (every line's runes plus a
// newline) to a document position, clamped into the document.
func (t *TextArea) posOf(o int) pos {
	for l, line := range t.lines {
		if o <= len(line) {
			return pos{l, max(o, 0)}
		}
		o -= len(line) + 1
	}
	last := len(t.lines) - 1
	return pos{last, len(t.lines[last])}
}

// offsetOf is posOf's inverse.
func (t *TextArea) offsetOf(p pos) int {
	off := 0
	for l := 0; l < p.line && l < len(t.lines); l++ {
		off += len(t.lines[l]) + 1
	}
	return off + p.col
}

// runeCount is Text's length in runes.
func (t *TextArea) runeCount() int {
	last := len(t.lines) - 1
	return t.offsetOf(pos{last, len(t.lines[last])})
}

// displayPos maps a document position to the display, where composing
// text sits spliced into its line.
func (t *TextArea) displayPos(p pos) pos {
	if t.composing() && p.line == t.peAt.line && p.col >= t.peAt.col {
		p.col += len(t.peText)
	}
	return p
}
