package widget

import (
	"math"
	"strings"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/internal/text"
	"github.com/stubbedev/lys-ui/render"
)

// TextArea is a multi-line editor: logical lines split on newlines,
// per-line cursor motion with a sticky preferred column, and a
// selection model shared with Entry. With wrap enabled (the default)
// long logical lines flow across visual rows built from shaped advance
// widths; editing and Text stay logical regardless.
type TextArea struct {
	node
	face        render.Font
	faces       faceCache
	sizePx      float64
	color       render.Color
	dir         Direction
	placeholder string
	wrap        bool
	indent      int // spaces per TrapTab insertion; 0 inserts a tab

	lines   [][]rune
	cursor  pos
	anchor  pos
	prefX   float64 // preferred visual column for vertical motion
	hasPref bool

	// panX is the per-logical-line horizontal pan for lines wider than
	// the viewport (only possible with wrap off): the display x of the
	// line's first on-show pixel. View state — never an undo entry;
	// wrap-on rows fit the width and read it as zero.
	panX map[int]int

	// MaxWidth caps the natural width Measure reports when not
	// wrapping: a long line stops growing the layout at the cap and
	// pans inside it instead — the text widgets' counterpart of
	// Scroll's bounded natural size. Zero (the default) reports the
	// content-hugging width. Set it before the first Measure, or
	// follow with InvalidateLayout.
	MaxWidth int

	// Composing (input-method preedit) display: peText shows at peAt
	// with the composing caret peCur runes into it (-1 hidden). It
	// lives outside the lines until a commit; displayLine splices it
	// in so the row cache and painting see what is on show.
	peText []rune
	peAt   pos
	peCur  int
	// peLine memoizes the composing line's splice; nil until the first
	// read after a preedit change.
	peLine []rune

	// hist is the undo/redo history; every mutation records into it.
	hist undoStack[areaState]

	// readOnly freezes user edits the same way Entry.readOnly does:
	// typing, paste, composition, delete, and undo/redo no-op, while
	// selection, copy, caret motion, and pan keep working. See Entry
	// .SetReadOnly for the contract; SetText stays programmatic.
	readOnly bool

	// Visual row cache, built lazily for the wrap width it was built
	// with. rows always covers the document: with wrap off it is the
	// identity mapping (one row per logical line).
	rows      []visualRow
	rowsWidth int
	rowsPx    float64
	rowsValid bool

	// text is GtkTextView's text node (`textview > text`), with its
	// selection child.
	text entryText

	// OnChanged fires after the contents change, whatever the source:
	// typing, editing keys, paste, undo and redo, or SetText (GTK's
	// TextBuffer changed). Text reads the new contents.
	OnChanged func()

	// The code view (codeview.go): the line-number gutter, Enter's
	// indent copy, the highlighter with its scheme and styled faces,
	// the per-line highlight cache, and the caret last revealed.
	lineNumbers bool
	autoIndent  bool
	highlighter Highlighter
	scheme      TextScheme
	variants    VariantFunc
	hl          []hlLine
	revealed    pos
	revealedOK  bool

	// bands is the selection/underline scratch the paint reuses across
	// frames, so the render path stays allocation-free in the steady
	// state.
	bands [][2]float64
}

// visualRow maps one painted row to a rune range of a logical line.
type visualRow struct {
	line     int // logical line index
	startCol int // first rune column (inclusive)
	endCol   int // end rune column (exclusive)
}

// SetWrap toggles soft wrapping; the default is on.
func (t *TextArea) SetWrap(on bool) {
	if t.wrap == on {
		return
	}
	t.wrap = on
	t.rowsValid = false
	t.hasPref = false
	t.InvalidateLayout()
}

// Wrap reports whether soft wrapping is on.
func (t *TextArea) Wrap() bool { return t.wrap }

// SetDirection selects the base paragraph direction every line
// resolves and lays out with — each line is one paragraph. DirectionAuto
// (the default) reads it off the line's first strong character;
// DirectionRTL additionally starts short lines at the right edge and
// mirrors the pan. Arrow and word motion always move visually, through
// the same resolution the paint uses, while cursor, anchor, and
// selection stay logical line/column positions. Changing the direction
// drops the row cache and re-pans.
func (t *TextArea) SetDirection(d Direction) {
	if t.dir == d {
		return
	}
	t.dir = d
	t.rowsValid = false
	t.hasPref = false
	t.panToCaret()
	t.InvalidateLayout()
}

// Direction returns the base paragraph direction the area resolves
// with.
func (t *TextArea) Direction() Direction { return t.dir }

// shapeLine shapes line rs under the area's base direction — the one
// text path the caret, selection, and paint share. Rune-keyed in the
// shaping cache, so it never converts the runes to a string per call.
func (t *TextArea) shapeLine(rs []rune) *render.ShapedText {
	return t.font().ShapeDirRunes(rs, t.px(), t.dir)
}

// SetIndent sets how many spaces Tab inserts while focused; zero (the
// default) inserts a tab character.
func (t *TextArea) SetIndent(n int) { t.indent = n }

// Indent returns how many spaces Tab inserts; zero means a tab
// character.
func (t *TextArea) Indent() int { return t.indent }

// TrapTab implements the tab-trap rule: a plain Tab inside the area
// inserts indentation instead of moving focus. Ctrl and shift variants
// are routed to focus movement before this is asked. Read-only and
// disabled areas decline the trap, so Tab moves focus instead.
func (t *TextArea) TrapTab(bool) bool {
	if !t.editable() {
		return false
	}
	if t.indent > 0 {
		t.Insert(strings.Repeat(" ", t.indent))
	} else {
		t.Insert("\t")
	}
	return true
}

// NewTextArea returns an empty area painted with face at sizePx. Face
// may be a render.Chain for mixed-script fallback. A nil face panics
// here (see requireFace) instead of failing later, in shaping.
func NewTextArea(face render.Font, sizePx float64, color render.Color) *TextArea {
	t := &TextArea{
		face:   requireFace("widget.NewTextArea", face),
		sizePx: sizePx,
		color:  color,
		lines:  [][]rune{{}},
		wrap:   true,
	}
	t.text.SetElement("text")
	t.text.placeholder.SetElement("placeholder")
	t.text.selection.SetElement("selection")
	setParents(t, &t.text)
	setParents(&t.text, &t.text.placeholder, &t.text.selection)
	return t
}

// Placeholder returns the text shown when the area is empty.
func (t *TextArea) Placeholder() string { return t.placeholder }

// SetPlaceholder sets the text shown when the area is empty.
func (t *TextArea) SetPlaceholder(s string) {
	if t.placeholder == s {
		return
	}
	t.placeholder = s
	t.Invalidate()
}

// SetReadOnly toggles the read-only mode — the same contract as
// Entry.SetReadOnly: user edits (typing, paste, composition, delete,
// undo/redo, and Tab indentation through TrapTab) no-op, while
// selection, copy, caret motion, and pan keep working. A history built
// before the flip waits untouched. The visual distinguishes itself
// from disabled the same way: normal text, muted caret.
func (t *TextArea) SetReadOnly(ro bool) {
	if t.readOnly == ro {
		return
	}
	t.readOnly = ro
	if ro {
		t.clearPreedit()
	}
	t.Invalidate()
}

// ReadOnly reports whether the area is in the read-only mode.
func (t *TextArea) ReadOnly() bool { return t.readOnly }

// editable reports whether user edits may land: enabled throughout
// the ancestor chain and not read-only. Every mutation entry point
// opens with it; SetText is the deliberate exception.
func (t *TextArea) editable() bool { return t.Enabled() && !t.readOnly }

// pos is a line/column cursor or anchor position in the logical
// document.
type pos struct {
	line, col int
}

// CursorName reports the text caret shape while hovered.
func (t *TextArea) CursorName() string { return "xterm" }

// Text returns the contents, lines joined with newlines.
func (t *TextArea) Text() string {
	parts := make([]string, len(t.lines))
	for i, l := range t.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

// recordEdit records an edit from before to now in the history and
// reports the change.
func (t *TextArea) recordEdit(before areaState) {
	t.hist.record(before, t.snapshot())
	t.notifyChanged()
}

// notifyChanged fires OnChanged.
func (t *TextArea) notifyChanged() {
	if t.OnChanged != nil {
		t.OnChanged()
	}
}

// splitLines replaces the logical lines with s split on newlines,
// leaving cursor and caches alone.
func (t *TextArea) splitLines(s string) {
	t.lines = nil
	for part := range strings.SplitSeq(s, "\n") {
		t.lines = append(t.lines, []rune(part))
	}
	if len(t.lines) == 0 {
		t.lines = [][]rune{{}}
	}
}

// SetText replaces the contents, splitting on newlines, and clears the
// selection and composing display. An app-driven replacement is not an
// edit to back out of: the undo history starts fresh.
func (t *TextArea) SetText(s string) {
	t.clearPreedit()
	t.splitLines(s)
	t.cursor = pos{0, 0}
	t.anchor = t.cursor
	t.hasPref = false
	t.rowsValid = false
	t.panX = nil // a fresh document starts unpanned
	t.hist.reset()
	t.InvalidateLayout()
	t.notifyChanged()
}

// Undo restores the state before the most recent edit — contents,
// caret, and selection included — reporting whether there was anything
// to undo. Composing text drops first, like before any other edit.
// Undo is a user edit: disabled or read-only, it no-ops and returns
// false while the history waits untouched.
func (t *TextArea) Undo() bool {
	if !t.editable() {
		return false
	}
	t.clearPreedit()
	return t.hist.undo(t.applyState)
}

// Redo reapplies the most recently undone edit, reporting whether
// there was one. Like Undo it is a user edit and no-ops while
// disabled or read-only.
func (t *TextArea) Redo() bool {
	if !t.editable() {
		return false
	}
	t.clearPreedit()
	return t.hist.redo(t.applyState)
}

// areaState is one snapshot of the document for the undo history.
type areaState struct {
	lines  [][]rune
	cursor pos
	anchor pos
}

// same reports whether two snapshots describe identical editing state.
func (s areaState) same(o areaState) bool {
	if s.cursor != o.cursor || s.anchor != o.anchor || len(s.lines) != len(o.lines) {
		return false
	}
	for i := range s.lines {
		if string(s.lines[i]) != string(o.lines[i]) {
			return false
		}
	}
	return true
}

// snapshot captures the current state for the history, deep-copying
// the lines so later edits cannot rewrite history.
func (t *TextArea) snapshot() areaState {
	lines := make([][]rune, len(t.lines))
	for i, l := range t.lines {
		lines[i] = append([]rune{}, l...)
	}
	return areaState{lines: lines, cursor: t.cursor, anchor: t.anchor}
}

// applyState restores a snapshot exactly like the edit that produced
// it: contents, caret, and selection, plus the same caches dropped and
// damage scheduled. The lines are copied in — the snapshot stays in the
// history for a later redo. The pan is not restored: it is view state,
// so it follows the restored caret instead.
func (t *TextArea) applyState(s areaState) {
	defer t.notifyChanged()
	t.lines = make([][]rune, len(s.lines))
	for i, l := range s.lines {
		t.lines[i] = append([]rune{}, l...)
	}
	t.cursor, t.anchor = s.cursor, s.anchor
	t.hasPref = false
	t.rowsValid = false
	t.panToCaret()
	t.InvalidateLayout()
}

// SetCursor places the cursor and anchor at a line/column, clearing any
// selection. Columns beyond the line clamp.
func (t *TextArea) SetCursor(line, col int) {
	t.clearPreedit()
	t.cursor = t.clamp(pos{line, col})
	t.anchor = t.cursor
	t.hasPref = false
	t.panToCaret()
	t.Invalidate()
}

// clamp keeps p inside the document.
func (t *TextArea) clamp(p pos) pos {
	if p.line < 0 {
		p.line = 0
	}
	if p.line >= len(t.lines) {
		p.line = len(t.lines) - 1
	}
	if p.col < 0 {
		p.col = 0
	}
	if p.col > len(t.lines[p.line]) {
		p.col = len(t.lines[p.line])
	}
	return p
}

// ordered returns the selection as start-before-end.
func (t *TextArea) ordered() (pos, pos) {
	if t.cursor.line < t.anchor.line || (t.cursor.line == t.anchor.line && t.cursor.col < t.anchor.col) {
		return t.cursor, t.anchor
	}
	return t.anchor, t.cursor
}

// Selection reports the selected rune range in line/col pairs.
func (t *TextArea) Selection() (start, end pos, active bool) {
	start, end = t.ordered()
	return start, end, start != end
}

// SelectedText implements SelectedTexter.
func (t *TextArea) SelectedText() (string, bool) {
	start, end, active := t.Selection()
	if !active {
		return "", false
	}
	var b strings.Builder
	for l := start.line; l <= end.line; l++ {
		from, to := 0, len(t.lines[l])
		if l == start.line {
			from = start.col
		}
		if l == end.line {
			to = end.col
		}
		if l > start.line {
			b.WriteByte('\n')
		}
		b.WriteString(string(t.lines[l][from:to]))
	}
	return b.String(), true
}

// collapse removes the selected text and leaves both ends at its start.
func (t *TextArea) collapse() {
	t.clearPreedit()
	start, end, active := t.Selection()
	if !active {
		t.cursor, t.anchor = t.clamp(t.cursor), t.clamp(t.cursor)
		return
	}
	head := append([]rune{}, t.lines[start.line][:start.col]...)
	tail := append([]rune{}, t.lines[end.line][end.col:]...)
	merged := append(head, tail...)
	rest := append([][]rune{}, t.lines[:start.line]...)
	rest = append(rest, merged)
	rest = append(rest, t.lines[end.line+1:]...)
	t.lines = rest
	t.cursor, t.anchor = start, start
	t.rowsValid = false
	t.panToCaret()
	t.InvalidateLayout()
}

// TextLen returns the number of logical lines, for tests and callers.
func (t *TextArea) TextLen() int { return len(t.lines) }

// runeWidth returns one rune's shaped advance. ShapeRune keeps the
// per-rune probes of the wrap walk allocation-free (one probe per rune
// per rebuild, every one of which used to build a one-rune string).
func (t *TextArea) runeWidth(r rune) float64 {
	return t.font().ShapeRune(r, t.px()).Advance()
}

// visualStep moves column at one visual step along line — left for
// negative delta, right for positive — the mapping the arrow keys move
// by over mixed-direction lines, through the shared resolution
// (internal/text). The column it lands on stays a logical index,
// snapped to a grapheme cluster.
func (t *TextArea) visualStep(line []rune, at, delta int) int {
	order := text.VisualOrder(line, text.BidiRuns(string(line), t.dir))
	return text.VisualStep(line, order, at, delta)
}

// visualWordStep returns the column one visual word away from at along
// line: the shared word segmentation walked in visual order, so
// ctrl+arrows land on the word edge the eye sees.
func (t *TextArea) visualWordStep(line []rune, at, delta int) int {
	order := text.VisualOrder(line, text.BidiRuns(string(line), t.dir))
	return text.VisualWordStep(line, order, at, delta)
}

// ensureRows rebuilds the visual row cache for the given available
// width when the text or width changed since the last build. With wrap
// off, rows are the identity mapping. A row always keeps at least one
// rune, so an unbreakable token wider than the viewport wraps one rune
// at a time instead of disappearing.
func (t *TextArea) ensureRows(availW int) {
	if t.rowsValid && availW == t.rowsWidth && t.px() == t.rowsPx {
		return
	}
	t.rows = t.rows[:0]
	for l := range t.lines {
		line := t.displayLine(l)
		if !t.wrap {
			t.rows = append(t.rows, visualRow{l, 0, len(line)})
			continue
		}
		start, x := 0, 0.0
		limit := float64(availW)
		for i, r := range line {
			adv := t.runeWidth(r)
			if i > start && x+adv > limit {
				t.rows = append(t.rows, visualRow{l, start, i})
				start = i
				x = 0
			}
			x += adv
		}
		t.rows = append(t.rows, visualRow{l, start, len(line)})
	}
	t.rowsWidth, t.rowsPx = availW, t.px()
	t.rowsValid = true
}

// rowOf returns the visual row the caret renders on: at a wrap
// boundary (a column that ends one row and starts the next) it is the
// start of the following row, which keeps downward motion progressing
// and matches the caret rendering.
func (t *TextArea) rowOf(col pos) int {
	first, withStart, last := -1, -1, -1
	for i, r := range t.rows {
		if r.line != col.line {
			if last >= 0 {
				break
			}
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
		if col.col >= r.startCol {
			withStart = i
		}
	}
	if withStart >= 0 {
		return withStart
	}
	if first >= 0 {
		return first
	}
	return len(t.rows) - 1
}

// splice is the insert primitive: it drops the composing display,
// replaces any selection, and puts s at the caret, snapping the caret
// forward to a grapheme cluster boundary — appending runes, counting
// clusters (#57). History recording is the callers' business — Insert
// lands as one entry, InsertRune as a coalescable typing run.
func (t *TextArea) splice(s string) {
	t.clearPreedit()
	t.collapse()
	for _, r := range s {
		switch r {
		case '\n':
			line := t.lines[t.cursor.line]
			head := append([]rune{}, line[:t.cursor.col]...)
			tail := append([]rune{}, line[t.cursor.col:]...)
			rest := append([][]rune{}, t.lines[:t.cursor.line]...)
			rest = append(rest, head, tail)
			rest = append(rest, t.lines[t.cursor.line+1:]...)
			t.lines = rest
			t.cursor.line++
			t.cursor.col = 0
		default:
			line := t.lines[t.cursor.line]
			line = append(line, 0)
			copy(line[t.cursor.col+1:], line[t.cursor.col:])
			line[t.cursor.col] = r
			t.lines[t.cursor.line] = line
			t.cursor.col++
		}
	}
	t.cursor.col = text.SnapClusterForward(t.lines[t.cursor.line], t.cursor.col)
	t.anchor = t.cursor
	t.hasPref = false
	t.panToCaret()
	t.rowsValid = false
	t.InvalidateLayout()
}

// Insert inserts s at the cursor; an active selection is replaced, and
// the whole insertion — a paste, a drop, a committed composition — is
// one undo entry. Blocked while disabled or read-only.
func (t *TextArea) Insert(s string) {
	if !t.editable() {
		return
	}
	before := t.snapshot()
	t.splice(s)
	t.recordEdit(before)
}

// deleteAt removes the selection, or one grapheme cluster/line break
// forward (#57), without touching history.
func (t *TextArea) deleteAt() {
	t.clearPreedit()
	if _, _, active := t.Selection(); active {
		t.collapse()
		return
	}
	c := t.clamp(t.cursor)
	line := t.lines[c.line]
	switch {
	case c.col < len(line):
		end := text.NextCluster(line, c.col)
		t.lines[c.line] = append(line[:c.col], line[end:]...)
	case c.line < len(t.lines)-1:
		next := t.lines[c.line+1]
		t.lines[c.line] = append(append([]rune{}, line...), next...)
		t.lines = append(t.lines[:c.line+1], t.lines[c.line+2:]...)
	}
	t.anchor = t.clamp(t.cursor)
	t.rowsValid = false
	t.panToCaret()
	t.InvalidateLayout()
}

// Delete removes the selection, or one grapheme cluster/line break
// forward (#57); one undo entry per call. Blocked while disabled or
// read-only.
func (t *TextArea) Delete() {
	if !t.editable() {
		return
	}
	before := t.snapshot()
	t.deleteAt()
	t.recordEdit(before)
}

// Backspace removes the selection, or one grapheme cluster/line break
// backward — an emoji family, a flag, a base rune with its marks go as
// one whole (#57); one undo entry per call. Blocked while disabled or
// read-only.
func (t *TextArea) Backspace() {
	if !t.editable() {
		return
	}
	before := t.snapshot()
	t.clearPreedit()
	if _, _, active := t.Selection(); active {
		t.collapse()
		t.recordEdit(before)
		return
	}
	c := t.clamp(t.cursor)
	switch {
	case c.col > 0:
		t.cursor.col = text.PrevCluster(t.lines[c.line], c.col)
	case c.line > 0:
		t.cursor.line--
		t.cursor.col = len(t.lines[t.cursor.line])
	default:
		return
	}
	t.anchor = t.clamp(t.cursor)
	t.panToCaret()
	t.Invalidate()
	t.deleteAt()
	t.recordEdit(before)
}

// DeleteWordBackward removes the word before the cursor — ctrl+
// backspace, with alt+backspace as the macOS alias — or the selection
// when one is active; the whole word is one undo entry. At a line
// start it is a no-op: word deletion stays inside the line, it never
// joins the previous one. Blocked while disabled or read-only.
func (t *TextArea) DeleteWordBackward() {
	if !t.editable() {
		return
	}
	before := t.snapshot()
	t.clearPreedit()
	if _, _, active := t.Selection(); active {
		t.collapse()
		t.recordEdit(before)
		return
	}
	c := t.clamp(t.cursor)
	start := text.WordStart(t.lines[c.line], c.col)
	if start == c.col {
		return // line start: nothing behind the caret on this line
	}
	t.cursor = pos{c.line, start}
	t.anchor = c
	t.panToCaret()
	t.Invalidate()
	t.deleteAt()
	t.recordEdit(before)
}

// DeleteWordForward removes the word after the cursor — ctrl+delete —
// or the selection when one is active; the whole word is one undo
// entry. At a line end it is a no-op, the mirror of
// DeleteWordBackward at a line start. Blocked while disabled or
// read-only.
func (t *TextArea) DeleteWordForward() {
	if !t.editable() {
		return
	}
	before := t.snapshot()
	t.clearPreedit()
	if _, _, active := t.Selection(); active {
		t.collapse()
		t.recordEdit(before)
		return
	}
	c := t.clamp(t.cursor)
	end := text.WordEnd(t.lines[c.line], c.col)
	if end == c.col {
		return // line end: nothing ahead of the caret on this line
	}
	t.anchor = t.cursor
	t.cursor = pos{c.line, end}
	t.collapse()
	t.recordEdit(before)
}

// CursorPos returns the cursor as line/column.
func (t *TextArea) CursorPos() (line, col int) { return t.cursor.line, t.cursor.col }

// move collapses the selection, then moves the cursor without touching
// the anchor when extend is set. A horizontal delta steps one visual
// unit — a grapheme cluster, mapped through the line's resolved
// direction so the arrow keys move where the eye looks (#57, #68);
// vertical motion carries the column, and the landed column snaps to
// its line's clusters.
func (t *TextArea) move(delta pos, extend bool) {
	t.clearPreedit()
	if !extend {
		if _, _, active := t.Selection(); active {
			start, _ := t.ordered()
			t.cursor, t.anchor = start, start
			if delta == (pos{}) {
				t.panToCaret()
				t.Invalidate()
				return
			}
		}
	}
	c := t.clamp(t.cursor)
	c.line += delta.line
	line := t.lines[t.clamp(c).line]
	switch {
	case delta.col > 0:
		c.col = t.visualStep(line, c.col, 1)
	case delta.col < 0:
		c.col = t.visualStep(line, c.col, -1)
	default:
		c.col = min(c.col, len(line))
	}
	t.cursor = t.clamp(c)
	if !extend {
		t.anchor = t.cursor
	}
	t.hasPref = false
	t.panToCaret()
	t.Invalidate()
}

// moveVertical moves the cursor one visual row up or down, honoring
// the preferred visual column set by a prior horizontal motion or
// click. With wrap on, a visual row is a wrapped segment of a logical
// line.
func (t *TextArea) moveVertical(dline int, extend bool) {
	t.clearPreedit()
	if _, _, active := t.Selection(); active && !extend {
		start, _ := t.ordered()
		t.cursor, t.anchor = start, start
	}
	c := t.clamp(t.cursor)
	if !t.hasPref {
		t.prefX = t.shapeLine(t.lines[c.line]).CaretX(c.col)
		t.hasPref = true
	}
	if t.wrap {
		t.ensureRows(t.wrapWidth())
		row := max(min(t.rowOf(c)+dline, len(t.rows)-1), 0)
		r := t.rows[row]
		t.cursor = pos{r.line, r.startCol + t.caretIn(r, t.prefX)}
	} else {
		nl := max(min(c.line+dline, len(t.lines)-1), 0)
		t.cursor = pos{nl, t.colForX(nl, t.prefX)}
	}
	t.cursor.col = text.SnapCluster(t.lines[t.cursor.line], t.cursor.col)
	if !extend {
		t.anchor = t.cursor
	}
	t.panToCaret()
	t.Invalidate()
}

// wordTo moves the cursor to the word edge the visual direction delta
// points at, inside the cursor's line: over the shared word
// segmentation (internal/text) walked in the line's resolved visual
// order — words are runs of grapheme clusters (#56, #57) — so motion,
// deletion, and double-click agree on what a word is while arrows land
// where the eye looks (#68). Word motion stays inside the line — a
// line start or end is a no-op, it never jumps across the line break.
// With extend the selection grows or shrinks from its anchor; without
// it an active selection first collapses to the visually-directed
// edge, the standard first-press behavior.
func (t *TextArea) wordTo(delta int, extend bool) {
	t.clearPreedit()
	if !extend {
		if _, _, active := t.Selection(); active {
			start, end := t.ordered()
			edge := end
			if t.visualEdge(delta, start, end) == start {
				edge = start
			}
			t.cursor, t.anchor = edge, edge
			t.hasPref = false
			t.panToCaret()
			t.Invalidate()
			return
		}
	}
	c := t.clamp(t.cursor)
	col := t.visualWordStep(t.lines[c.line], c.col, delta)
	if col == c.col {
		return // line boundary: nothing to step to on this line
	}
	t.cursor = pos{c.line, col}
	if !extend {
		t.anchor = t.cursor
	}
	t.hasPref = false
	t.panToCaret()
	t.Invalidate()
}

// visualEdge returns which of two logical positions on one line sits
// further in the visual direction delta points: the caret table's x,
// which follows mixed-direction lines, decides.
func (t *TextArea) visualEdge(delta int, a, b pos) pos {
	xa := t.shapeLine(t.lines[a.line]).CaretX(a.col)
	xb := t.shapeLine(t.lines[b.line]).CaretX(b.col)
	if delta < 0 == (xa < xb) {
		return a
	}
	return b
}

// MoveWord moves the cursor one visual word within its line: the word
// edge the direction points at, gaps (whitespace, punctuation runs)
// skipped whole. An active selection collapses to the visually-
// directed edge first, without moving; the line boundaries are no-ops.
func (t *TextArea) MoveWord(delta int) { t.wordTo(delta, false) }

// MoveWordExtending is MoveWord with shift held: the selection
// extends from its anchor to the word edge.
func (t *TextArea) MoveWordExtending(delta int) { t.wordTo(delta, true) }

// wrapWidth returns the pixel width available for wrapping inside the
// current bounds.
func (t *TextArea) wrapWidth() int {
	return t.textRect().W
}

// linePan returns line l's horizontal pan, clamped to the line's
// overflow against the current width: shorter text or a wider viewport
// pulls a stale pan back without waiting for the caret to visit.
// Wrapping keeps every row inside the width, so it always reads zero.
func (t *TextArea) linePan(l int) int {
	p, ok := t.panX[l]
	if !ok {
		return 0
	}
	return t.clampPan(l, p)
}

// clampPan bounds a pan for line l: zero when the line fits the
// viewport, at most the line width plus the caret margin past the
// right edge so a scrolled-to-end caret keeps its margin. The value
// slides the reading window the same way for either direction: an RTL
// line starts flush with the right edge and the pan reveals its
// reading end.
func (t *TextArea) clampPan(l, x int) int {
	avail := t.wrapWidth()
	if t.wrap || avail <= 0 || t.face == nil || l < 0 || l >= len(t.lines) {
		return 0
	}
	lineW := int(t.shapeLine(t.displayLine(l)).Advance() + 0.5)
	return min(max(x, 0), max(0, lineW+caretPad-avail))
}

// caretX returns the caret bar's root-space x on line l: the caret's
// visual row's shaped slice — which follows mixed-direction lines
// visually — at the line origin linePan positions.
func (t *TextArea) caretX(l, col int) int {
	t.ensureRows(t.wrapWidth())
	ri := t.rowOf(pos{l, min(max(col, 0), len(t.lines[l]))})
	sh, x := t.rowLayout(ri)
	return x + int(sh.CaretX(col-t.rows[ri].startCol)+0.5)
}

// rowLayout is visual row ri shaped, with the screen x its caret
// offsets count from (the reading edge, pan applied).
func (t *TextArea) rowLayout(ri int) (*render.ShapedText, int) {
	row := t.rows[ri]
	sh := t.shapeLine(t.displayLine(row.line)[row.startCol:row.endCol])
	x := t.textRect().X - t.linePan(row.line)
	if sh.RTL() {
		x += t.wrapWidth() - int(sh.Advance()+0.5)
	}
	return sh, x
}

// panToCaret adjusts the caret line's pan so the caret stays visible:
// flush with the reading's start edge, or caretPad of following text
// inside the far edge — measured from the left for an LTR line, from
// the right for an RTL one. A view mutation — callers own the
// repaint.
func (t *TextArea) panToCaret() {
	if t.face == nil {
		return
	}
	c := t.caretPos()
	l := min(max(c.line, 0), len(t.lines)-1)
	avail := t.wrapWidth()
	if avail <= 0 {
		t.setPan(l, 0)
		return
	}
	// The shape carries the line's resolved direction, so neither read
	// converts the runes.
	line := t.displayLine(l)
	sh := t.shapeLine(line)
	cx := int(sh.CaretX(c.col) + 0.5)
	if sh.RTL() {
		cx = int(sh.Advance()+0.5) - cx // the caret's distance from the reading start edge
	}
	p := t.linePan(l)
	switch {
	case cx < p:
		t.setPan(l, cx)
	case cx > p+avail-caretPad:
		t.setPan(l, cx-avail+caretPad)
	}
}

// setPan stores line l's pan, creating the map on first use. The value
// is clamped, so stale entries cannot outlive a text or width change.
func (t *TextArea) setPan(l, x int) {
	if t.panX == nil {
		t.panX = map[int]int{}
	}
	t.panX[l] = t.clampPan(l, x)
}

// caretIn maps a visual x offset to a column within the rune range of
// one visual row, returning a row-relative column.
func (t *TextArea) caretIn(r visualRow, x float64) int {
	line := t.lines[r.line]
	k := t.font().ShapeRunes(line[r.startCol:r.endCol], t.px()).CaretAt(x)
	return min(k, r.endCol-r.startCol)
}

// colForX maps a visual x offset within line l to a rune column.
func (t *TextArea) colForX(l int, x float64) int {
	if l < 0 || l >= len(t.lines) {
		return 0
	}
	return t.font().ShapeRunes(t.lines[l], t.px()).CaretAt(x)
}

// Measure reports the natural widest-line size, or, when wrapping and
// a width is offered, the offered width by the wrapped row count.

// Measure reports the natural widest-line size, or, when wrapping and
// a width is offered, the offered width by the wrapped row count, the
// text insets (CSS box, text node, gutter) around it. Composing text
// counts toward the widest line and the row count. MaxWidth caps the
// reported width when not wrapping — a long line then stops widening
// the layout at the cap and pans inside it.
func (t *TextArea) Measure(con Constraints) Size {
	lineH := t.lineHeight()
	v := t.style(t)
	in, m := t.textInsets(), marginOf(v)
	hIn, vIn := in.Left+in.Right, in.Top+in.Bottom
	if t.wrap && con.Max.W-m.Left-m.Right > hIn {
		t.ensureRows(con.Max.W - m.Left - m.Right - hIn)
		h := lineH*len(t.rows) + vIn
		return clampSize(Size{W: con.Max.W, H: max(h, picki(v, style.PropMinHeight, 0)) + m.Top + m.Bottom}, con)
	}
	w := 16
	for l := range t.lines {
		if adv := int(t.shapeLine(t.displayLine(l)).Advance() + 0.5); adv > w {
			w = adv
		}
	}
	w += hIn
	if t.MaxWidth > 0 {
		w = min(w, max(t.MaxWidth, 16))
	}
	h := lineH*len(t.lines) + vIn
	// The stylesheet's min-* floors hold before the constraints clamp.
	w = max(w, picki(v, style.PropMinWidth, 0))
	h = max(h, picki(v, style.PropMinHeight, 0))
	return clampSize(Size{W: w + m.Left + m.Right, H: h + m.Top + m.Bottom}, con)
}

// px is the effective text size: the stylesheet's font-size when
// matched, else the constructor's.
func (t *TextArea) px() float64 { return fontPx(t.style(t), t.sizePx) }

// font is the face the text shapes with: the constructor face styled
// by the cascade (letter-spacing, font-variation-settings, tnum).
func (t *TextArea) font() render.Font { return t.faces.get(t.face, t.style(t), 0, int(t.px())) }

// textPad is an unstyled area's padding.
var textPad = render.Insets{Top: 8, Right: 12, Bottom: 8, Left: 12}

// textInsets are the text's insets inside the area's bounds: the
// border and padding (textPad where unstyled), the line-number gutter,
// and the text node's margin, border and padding.
func (t *TextArea) textInsets() render.Insets {
	v := t.style(t)
	b, p := borderOf(v), paddingOr(v, textPad)
	tx := boxOf(t.text.style(&t.text), render.Insets{}).outer()
	return render.Insets{
		Top:    b.Top + p.Top + tx.Top,
		Right:  b.Right + p.Right + tx.Right,
		Bottom: b.Bottom + p.Bottom + tx.Bottom,
		Left:   b.Left + p.Left + t.gutterWidth() + tx.Left,
	}
}

// textRect is where the rows lay out: the bounds less textInsets.
func (t *TextArea) textRect() render.Rect {
	in := t.textInsets()
	return render.Rect{
		X: t.bounds.X + in.Left, Y: t.bounds.Y + in.Top,
		W: max(t.bounds.W-in.Left-in.Right, 0), H: max(t.bounds.H-in.Top-in.Bottom, 0),
	}
}

// MinSize implements MinSizer: the stylesheet's min-* floors when set,
// no floor otherwise (the zero Size).
func (t *TextArea) MinSize() Size {
	v := t.style(t)
	return Size{
		W: picki(v, style.PropMinWidth, 0),
		H: picki(v, style.PropMinHeight, 0),
	}
}

// lineHeight returns the integer line height of the font.
func (t *TextArea) lineHeight() int {
	return t.font().Shape("lg", t.px()).LineHeight()
}

// Paint draws the wrapped rows, the selection highlight, the composing
// text with its underline, and the cursor. An unwrapped line wider than
// the viewport pans: its content draws at -panX[line], clipped to the
// inner rect, per logical line. The placeholder only shows on an empty
// area and never pans. State colors follow Entry: disabled fills with
// the derived disabled surface and fades the text; read-only keeps the
// text and fades only the caret. The stylesheet's background-color,
// color, and border-radius override the theme's.
func (t *TextArea) Paint(cv *render.Canvas) {
	th := Current()
	v := t.style(t)
	enabled := IsEnabled(t)
	bg := th.Surface
	if !enabled {
		bg = th.DisabledSurface()
	}
	bg = pickc(0, v, style.PropBackgroundColor, bg)
	radii := radiusOr(v, th.Radius)
	// The text node's color, its own or the area's it inherits; a
	// scheme's text and cursor styles color it over both.
	textCol := pickc(t.color, t.text.style(&t.text), style.PropColor, pickc(0, v, style.PropColor, inkOr(t.color)))
	if st, ok := t.scheme["text"]; ok && st.Color != 0 {
		textCol = st.Color
	}
	caretCol := textCol
	if v.Has(style.PropCaretColor) {
		caretCol = v.CaretColor
	}
	if st, ok := t.scheme["cursor"]; ok && st.Color != 0 {
		caretCol = st.Color
	}
	fade := func(c render.Color) render.Color {
		if !enabled {
			return scaleAlpha(c, disabledFade)
		}
		return c
	}
	textCol = fade(textCol)
	if !enabled || t.readOnly {
		caretCol = scaleAlpha(caretCol, disabledFade)
	}
	paintBoxBehind(cv, v, t.bounds, radii, borderOf(v), bg)
	lineH := t.lineHeight()
	start, end, active := t.Selection()
	t.ensureRows(t.wrapWidth())
	c := t.textRect()
	t.paintGutter(cv, c, lineH, fade)

	if len(t.lines) == 1 && len(t.lines[0]) == 0 && !t.composing() && t.placeholder != "" {
		col := th.Border
		if pv := t.text.placeholder.style(&t.text.placeholder); pv.Declares(style.PropColor) {
			col = pv.Color
		}
		ph := t.font().ShapeDir(t.placeholder, t.px(), t.dir)
		x := c.X
		if text.RTL(t.placeholder, t.dir) {
			x = c.X + c.W - int(ph.Advance()+0.5)
		}
		prev := cv.PushClip(render.Rect{X: c.X, Y: t.bounds.Y, W: c.W, H: t.bounds.H})
		ph.Draw(cv, x, c.Y+int(math.Round((float64(lineH)-float64(ph.LineHeight()))/2+ph.Ascent())), fade(col))
		cv.PopClip(prev)
		return
	}
	selFill, selFg := t.selectionStyle()
	t.updateHighlight()
	prev := cv.PushClip(render.Rect{X: c.X, Y: t.bounds.Y, W: c.W, H: t.bounds.H})
	for i, r := range t.rows {
		pan := t.linePan(r.line)
		line := t.displayLine(r.line)
		y := c.Y + i*lineH
		if len(line) == 0 || r.startCol >= r.endCol {
			continue
		}
		sh := t.shapeLine(line[r.startCol:r.endCol])
		// The row's line origin: the text rect's left for LTR, slid by
		// the pan; flush right minus the pan for RTL, so short rows hug
		// the edge their reading starts at.
		rowX := c.X - pan
		rtl := sh.RTL()
		if rtl {
			rowX += t.wrapWidth() - int(sh.Advance()+0.5)
		}
		box := render.Rect{X: c.X, Y: y, W: c.W, H: lineH}
		// Selection band for the portion of this row inside the
		// selection — one band per visual run, so a span crossing
		// directions highlights disjoint pieces.
		var bands [][2]float64
		if active {
			from, to := r.startCol, r.endCol
			if r.line == start.line {
				from = max(from, start.col)
			}
			if r.line == end.line {
				to = min(to, end.col)
			}
			if from < to {
				bands = sh.AppendCaretBands(t.bands[:0], from-r.startCol, to-r.startCol)
				for _, band := range bands {
					cv.FillRect(bandAt(rowX, band, y, lineH), selFill)
				}
			}
		}
		rowClip := cv.PushClip(box)
		baseline := box.Y + int(math.Round((float64(box.H)-float64(sh.LineHeight()))/2+sh.Ascent()))
		t.drawRow(cv, r, line, sh, rowX, baseline, textCol, rtl, fade)
		if deco := cssDecoration(t.style(t)); deco.Lines != 0 {
			sh.DrawDecoration(cv, rowX, baseline, deco, textCol)
		}
		if selFg != 0 {
			for _, band := range bands {
				clip := cv.PushClip(bandAt(rowX, band, y, lineH))
				sh.Draw(cv, rowX, baseline, fade(selFg))
				cv.PopClip(clip)
			}
		}
		cv.PopClip(rowClip)
	}
	if t.composing() {
		// Accent underline under the composing range, across every
		// visual row it touches.
		pb, pe := t.peAt.col, t.peAt.col+len(t.peText)
		a := th.Accent
		ul := render.RGBA(a.R(), a.G(), a.B(), 200)
		for i, r := range t.rows {
			if r.line != t.peAt.line || r.endCol <= pb || r.startCol >= pe {
				continue
			}
			line := t.displayLine(r.line)
			sh := t.shapeLine(line[r.startCol:r.endCol])
			pan := t.linePan(r.line)
			rowX := c.X - pan
			if sh.RTL() {
				rowX += t.wrapWidth() - int(sh.Advance()+0.5)
			}
			for _, band := range sh.AppendCaretBands(t.bands[:0], max(r.startCol, pb)-r.startCol, min(r.endCol, pe)-r.startCol) {
				y := c.Y + i*lineH
				cv.FillRect(render.Rect{
					X: rowX + int(band[0]+0.5), Y: y + lineH - 4,
					W: max(int(band[1]+0.5)-int(band[0]+0.5), 2), H: 2,
				}, ul)
			}
		}
	}
	// Caret in its CSS shape on the caret's visual row, while the area
	// has keyboard focus; hidden while the input method hides its
	// composing caret. Every frame reads the same pan, so a repaint
	// (blink or otherwise) never jumps it.
	caret := t.caretPos()
	if t.focused && (!t.composing() || t.peCur >= 0) {
		row := t.rowOf(caret)
		adv := 0
		if caret.col < len(t.lines[caret.line]) {
			lsh := t.shapeLine(t.lines[caret.line])
			for _, band := range lsh.AppendCaretBands(t.bands[:0], caret.col, caret.col+1) {
				adv = int(band[1] - band[0] + 0.5)
			}
		}
		cv.FillRect(caretShapeRect(t.caretX(caret.line, caret.col), c.Y+row*lineH+2, lineH-4, adv, v.CaretShape), caretCol)
	}
	cv.PopClip(prev)
}

// bandAt is a selection band's rect on the row at y.
func bandAt(rowX int, band [2]float64, y, lineH int) render.Rect {
	return render.Rect{X: rowX + int(band[0]+0.5), Y: y, W: int(band[1]+0.5) - int(band[0]+0.5), H: lineH}
}

// selectionStyle is the selection's fill and text color: the scheme's
// selection style, else the text node's selection rules, else the
// translucent accent over the text's own color (0).
func (t *TextArea) selectionStyle() (fill, fg render.Color) {
	a := Current().Accent
	fill = render.RGBA(a.R(), a.G(), a.B(), 90)
	if sv := t.text.selection.style(&t.text.selection); sv != nil {
		if sv.Declares(style.PropBackgroundColor) {
			fill = sv.Background
		}
		if sv.Declares(style.PropColor) {
			fg = sv.Color
		}
	}
	if st, ok := t.scheme["selection"]; ok {
		if st.Background != 0 {
			fill = st.Background
		}
		if st.Color != 0 {
			fg = st.Color
		}
	}
	return fill, fg
}

// Arrange pins the rect and re-pans the caret's line: a resize changes
// the visible width, so the caret may need pulling back into view.
// Stale pans on other lines re-clamp when read.
func (t *TextArea) Arrange(r render.Rect) {
	t.node.Arrange(marginOf(t.style(t)).Shrink(r))
	t.panToCaret()
	t.revealCaret()
}

// Role implements Roleer.
func (t *TextArea) Role() Role { return RoleTextArea }

// HitTest returns the area when p is inside its bounds.
func (t *TextArea) HitTest(p Point) Widget { return t.HitLeaf(t, p) }

// posAt maps a root-space point to a document position, resolving
// through the visual rows when wrapping and through the clicked
// line's pan so a click on a half-visible rune lands on that rune,
// snapped to the start of its grapheme cluster (#57).
func (t *TextArea) posAt(p Point) pos {
	lineH := t.lineHeight()
	t.ensureRows(t.wrapWidth())
	c := t.textRect()
	row := max(min((p.Y-c.Y)/lineH, len(t.rows)-1), 0)
	r := t.rows[row]
	x := float64(p.X - c.X + t.linePan(r.line))
	var col int
	if !t.wrap {
		col = t.colForX(r.line, x)
	} else {
		col = r.startCol + t.caretIn(r, x)
	}
	return pos{line: r.line, col: text.SnapCluster(t.lines[r.line], col)}
}

// ClickAt places the cursor (and anchor) at the clicked position,
// dropping the composing display.
func (t *TextArea) ClickAt(p Point) {
	t.clearPreedit()
	t.cursor = t.posAt(p)
	t.anchor = t.cursor
	switch userSelectOf(t) {
	case style.SelectAll:
		t.SelectAll()
		return
	}
	t.hasPref = false
}

// DragMove extends the selection to the dragged position, dropping the
// composing display. A motion past the field's left or right edge
// auto-pans the clicked line one step so the drag can reach text
// outside the viewport.
func (t *TextArea) DragMove(p Point) {
	t.clearPreedit()
	t.autoPan(p)
	t.cursor = t.posAt(p)
	if userSelectOf(t) == style.SelectNone {
		t.anchor = t.cursor
	}
	t.hasPref = false
}

// autoPan pans the clicked line one drag step when p passes the
// field's left or right edge.
func (t *TextArea) autoPan(p Point) {
	left, right := t.bounds.X+8, t.bounds.X+8+t.wrapWidth()
	l := t.posAt(p).line
	t.setPan(l, edgePan(p.X, left, right, t.linePan(l)))
}

// DoubleClickAt selects the same-class run under the pointer, over
// the shared word segmentation — the same words the ctrl+arrows steps
// land on.
func (t *TextArea) DoubleClickAt(p Point) {
	t.clearPreedit()
	if userSelectOf(t) == style.SelectNone {
		t.ClickAt(p)
		return
	}
	at := t.posAt(p)
	line := t.lines[at.line]
	if len(line) == 0 {
		t.cursor, t.anchor = at, at
		return
	}
	start, end := text.WordRun(line, at.col)
	t.cursor, t.anchor = pos{at.line, end}, pos{at.line, start}
	t.hasPref = false
	t.panToCaret()
	t.Invalidate()
}

// SelectAll selects the entire document; the pan follows the caret to
// the end of the last line.
func (t *TextArea) SelectAll() {
	t.clearPreedit()
	t.anchor = pos{0, 0}
	t.cursor = pos{len(t.lines) - 1, len(t.lines[len(t.lines)-1])}
	t.hasPref = false
	t.panToCaret()
	t.Invalidate()
}

// InsertRune implements RuneHandler. Typed runes coalesce into one
// undo entry until a word boundary, an idle gap, or a different edit
// breaks the run. Blocked while disabled or read-only.
func (t *TextArea) InsertRune(r rune) {
	if !t.editable() {
		return
	}
	before := t.snapshot()
	t.splice(string(r))
	t.hist.recordTyping(before, t.snapshot(), r)
	t.notifyChanged()
}

// KeyAction implements KeyActionHandler. Shift-extended motion grows
// the selection from its anchor; ctrl turns arrows and backspace/delete
// word-wise, alt+backspace aliasing ctrl+backspace. The bare keys edit
// one grapheme cluster per press (#57), so emoji and accented pairs
// edit whole. Interaction order: while composing, the input method owns
// the text and the bare backspace unwinds the composing display one
// rune first — that is compose state (#54), orthogonal to committed
// text; after it, ctrl/alt make backspace and delete word-wise over the
// cluster-refined words (#56); the bare keys apply to the contents one
// cluster at a time. A disabled area ignores keys outright; a
// read-only one keeps the motion and selection keys and drops only the
// mutating ones.
func (t *TextArea) KeyAction(a KeyAction, mods Mods) {
	if !t.Enabled() {
		return
	}
	shift := mods&ModShift != 0
	ctrl := mods&ModCtrl != 0
	if a == KeyBackspace && mods&(ModCtrl|ModAlt) == 0 && t.composing() && t.editable() {
		t.peText = t.peText[:len(t.peText)-1]
		t.peCur = min(t.peCur, len(t.peText))
		if len(t.peText) == 0 {
			t.clearPreedit()
		}
		return
	}
	// Every action except the trim above drops the composing display.
	t.clearPreedit()
	switch a {
	case KeyBackspace:
		if t.editable() {
			if ctrl || mods&ModAlt != 0 {
				t.DeleteWordBackward()
			} else {
				t.Backspace()
			}
		}
	case KeyDelete:
		if t.editable() {
			if ctrl {
				t.DeleteWordForward()
			} else {
				t.Delete()
			}
		}
	case KeyLeft:
		if ctrl {
			t.wordTo(-1, shift)
			return
		}
		c := t.clamp(t.cursor)
		if !shift {
			if _, _, active := t.Selection(); active {
				start, _ := t.ordered()
				t.cursor, t.anchor = start, start
				t.Invalidate()
				return
			}
		}
		switch {
		case c.col > 0:
			t.move(pos{col: -1}, shift)
		case c.line > 0:
			t.cursor = t.clamp(pos{c.line - 1, len(t.lines[c.line-1])})
			if !shift {
				t.anchor = t.cursor
			}
			t.hasPref = false
			t.panToCaret()
			t.Invalidate()
		}
	case KeyRight:
		if ctrl {
			t.wordTo(1, shift)
			return
		}
		c := t.clamp(t.cursor)
		if !shift {
			if _, _, active := t.Selection(); active {
				end := t.cursor
				if t.cursor.line < t.anchor.line || (t.cursor.line == t.anchor.line && t.cursor.col < t.anchor.col) {
					end = t.anchor
				}
				t.cursor, t.anchor = end, end
				t.Invalidate()
				return
			}
		}
		switch {
		case c.col < len(t.lines[c.line]):
			t.move(pos{col: 1}, shift)
		case c.line < len(t.lines)-1:
			t.cursor = t.clamp(pos{c.line + 1, 0})
			if !shift {
				t.anchor = t.cursor
			}
			t.hasPref = false
			t.panToCaret()
			t.Invalidate()
		}
	case KeyUp:
		t.moveVertical(-1, shift)
	case KeyDown:
		t.moveVertical(1, shift)
	case KeyHome:
		if t.wrap {
			t.ensureRows(t.wrapWidth())
			t.cursor.col = t.rows[t.rowOf(t.clamp(t.cursor))].startCol
		} else {
			t.cursor.col = 0
		}
		t.cursor = t.clamp(t.cursor)
		if !shift {
			t.anchor = t.cursor
		}
		t.hasPref = false
		t.panToCaret()
		t.Invalidate()
	case KeyEnd:
		if t.wrap {
			t.ensureRows(t.wrapWidth())
			t.cursor.col = t.rows[t.rowOf(t.clamp(t.cursor))].endCol
		} else {
			t.cursor.col = len(t.lines[t.clamp(t.cursor).line])
		}
		t.cursor = t.clamp(t.cursor)
		t.cursor.col = text.SnapCluster(t.lines[t.cursor.line], t.cursor.col)
		if !shift {
			t.anchor = t.cursor
		}
		t.hasPref = false
		t.panToCaret()
		t.Invalidate()
	case KeyEnter:
		if t.editable() {
			t.Insert("\n" + t.enterIndent())
		}
	}
}
