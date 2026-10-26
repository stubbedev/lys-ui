package widget

import (
	"slices"
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// The Entry completion half of #92: an inline match list below the
// field, the GTK EntryCompletion shape. The list is painted directly
// on the canvas (no child widgets): it is chrome of the field itself,
// appearing only while matches exist, so the entry's layout contract
// stays one widget's.

// completionMax bounds the list; more matches mean nobody is finding
// anything anyway.
const completionMax = 8

// completionPadX pads the row text; completionGapY lifts the list off
// the field's bottom edge.
const (
	completionPadX = 8
	completionGapY = 1
)

// recomputeCompletion refreshes the match list after every content
// change. A pick suppresses its own recompute (the completing guard);
// an exact single match, an empty prefix, and no matches all close the
// list - showing the typed text as its own only match teaches nothing.
func (e *Entry) recomputeCompletion() {
	if e.completing {
		return
	}
	if e.Completion == nil {
		e.closeCompletion()
		return
	}
	text := e.Text()
	if strings.TrimSpace(text) == "" {
		e.closeCompletion()
		return
	}
	matches := e.Completion(text)
	if len(matches) > completionMax {
		matches = matches[:completionMax]
	}
	if len(matches) == 0 || (len(matches) == 1 && matches[0] == text) {
		e.closeCompletion()
		return
	}
	if len(e.complete) != len(matches) || !slices.Equal(e.complete, matches) {
		e.InvalidateLayout()
	}
	e.complete = matches
	e.completeSel = 0
	e.Invalidate()
}

// closeCompletion drops the list without touching the text.
func (e *Entry) closeCompletion() {
	if len(e.complete) == 0 {
		return
	}
	e.complete = nil
	e.completeSel = 0
	e.InvalidateLayout()
	e.Invalidate()
}

// completionOpen reports whether the list is showing.
func (e *Entry) completionOpen() bool { return len(e.complete) > 0 }

// pickCompletion chooses row i: the text becomes the match, OnChanged
// delivers it, OnComplete announces the pick, and the list closes.
func (e *Entry) pickCompletion(i int) {
	if i < 0 || i >= len(e.complete) {
		return
	}
	chosen := e.complete[i]
	e.completing = true
	e.SetText(chosen)
	e.completing = false
	e.closeCompletion()
	if e.OnComplete != nil {
		e.OnComplete(chosen)
	}
}

// completionKeys intercepts the editing keys while the list is open:
// arrows move the highlight (clamped, GTK-style), Enter picks, Esc
// closes without changing the text. It reports whether the key was
// consumed so the entry's own handling - cursor moves, activate -
// stays out of the way.
func (e *Entry) completionKeys(a KeyAction, mods Mods) bool {
	if !e.completionOpen() || mods&(ModCtrl|ModAlt|ModShift) != 0 {
		return false
	}
	switch a {
	case KeyUp:
		e.completeSel = max(0, e.completeSel-1)
		e.Invalidate()
		return true
	case KeyDown:
		e.completeSel = min(len(e.complete)-1, e.completeSel+1)
		e.Invalidate()
		return true
	case KeyEnter:
		e.pickCompletion(e.completeSel)
		return true
	case KeyDismiss:
		e.closeCompletion()
		return true
	}
	return false
}

// completionRowH is one list row's height: the text line plus breathing.
func (e *Entry) completionRowH() int {
	return int(float64(e.font().Shape("lg", e.fontPx()).LineHeight())+0.5) + 8
}

// completionHeight is the open list's full height.
func (e *Entry) completionHeight() int {
	if !e.completionOpen() {
		return 0
	}
	return len(e.complete)*e.completionRowH() + completionGapY
}

// completionListRect places the list under the arranged field.
func (e *Entry) completionListRect() render.Rect {
	return render.Rect{
		X: e.bounds.X, Y: e.bounds.Y + e.bounds.H + completionGapY - 1,
		W: e.bounds.W, H: e.completionHeight(),
	}
}

// paintCompletion draws the list: a surface panel with a border, the
// highlight row filled, every row's text shaped and drawn.
func (e *Entry) paintCompletion(cv *render.Canvas) {
	if !e.completionOpen() {
		return
	}
	th := Current()
	r := e.completionListRect()
	e.completeRect = r
	cv.RoundedRect(r, 4, th.Surface)
	cv.BorderRect(r, 4, th.Border)
	rowH := e.completionRowH()
	for i, match := range e.complete {
		row := render.Rect{X: r.X, Y: r.Y + i*rowH, W: r.W, H: rowH}
		if i == e.completeSel {
			cv.RoundedRect(render.Rect{X: row.X + 2, Y: row.Y + 1, W: row.W - 4, H: row.H - 2}, 3, th.SurfaceHover)
		}
		e.font().DrawAligned(cv, match, render.Rect{
			X: row.X + completionPadX, Y: row.Y, W: row.W - 2*completionPadX, H: row.H,
		}, e.fontPx(), th.Text, render.AlignStart)
	}
}

// clickCompletion resolves a click inside the list into a pick; true
// means the click was the list's and never reaches the caret logic.
func (e *Entry) clickCompletion(p Point) bool {
	if !e.completionOpen() {
		return false
	}
	r := e.completionListRect()
	if !r.Contains(p.X, p.Y) {
		return false
	}
	e.pickCompletion((p.Y - r.Y) / e.completionRowH())
	return true
}
