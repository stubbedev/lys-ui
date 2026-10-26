// The edit history shared by the editable text widgets (Entry,
// TextArea): every content mutation records a before/after state pair
// on a bounded stack, consecutive typed runes coalesce into one entry
// GTK-style, and undo/redo restore snapshots exactly as the edits left
// them. routeKey reaches it through Undoer.
package widget

import (
	"time"

	"github.com/stubbedev/lys-lib/text"
)

// undoLimit caps the undo stack; pushing past it drops the oldest
// entry.
const undoLimit = 100

// undoIdleGap is the typing inactivity that closes a coalesced run:
// after this long without a keystroke the next typed rune starts a new
// undo entry.
const undoIdleGap = 1200 * time.Millisecond

// Undoer is an editable widget with an undo history. Undo restores the
// state before the most recent edit, Redo reapplies the most recently
// undone one; both report whether anything changed.
type Undoer interface {
	Undo() bool
	Redo() bool
}

// textSnapshot is one captured editable-widget state: comparable
// against another snapshot of the same widget for coalescing and no-op
// detection.
type textSnapshot[T any] interface {
	same(T) bool
}

// undoChange is one reversible edit: the widget state before and after
// it. Typing runs carry coalescing bookkeeping; one-shot edits (delete,
// paste, replace) leave it zero.
type undoChange[T textSnapshot[T]] struct {
	before, after T
	typing        bool      // open to further coalesced runes
	at            time.Time // clock time of the run's last rune
	last          rune      // the run's last rune, for the word-boundary break
}

// undoStack is the bounded, coalescing edit history of one widget. now
// is the coalescing clock, injectable for tests; nil means time.Now.
type undoStack[T textSnapshot[T]] struct {
	done   []undoChange[T]
	undone []undoChange[T]
	now    func() time.Time
}

// clock reads the coalescing clock.
func (u *undoStack[T]) clock() time.Time {
	if u.now == nil {
		return time.Now()
	}
	return u.now()
}

// record adds a one-shot edit (delete, paste, replace). A no-op edit —
// one that left the state untouched — records nothing.
func (u *undoStack[T]) record(before, after T) {
	if before.same(after) {
		return
	}
	u.push(undoChange[T]{before: before, after: after})
}

// recordTyping adds one typed rune. It extends the open run on top of
// the stack when the rune continues it: same word class as the run's
// last rune — or a grapheme continuation of it (a combining mark
// completing a base rune, a skin-tone modifier, a ZWJ join: one
// cluster, one unit, #57) — within the idle gap, and no intervening
// edit — the state before this insert still matches what the run
// produced. Otherwise the rune opens a fresh run.
func (u *undoStack[T]) recordTyping(before, after T, r rune) {
	if before.same(after) {
		return
	}
	now := u.clock()
	if n := len(u.done); n > 0 {
		top := &u.done[n-1]
		if top.typing && (text.IsWordRune(top.last) == text.IsWordRune(r) || text.ContinuesCluster(top.last, r)) && now.Sub(top.at) <= undoIdleGap && top.after.same(before) {
			top.after = after
			top.last = r
			top.at = now
			return
		}
	}
	u.push(undoChange[T]{before: before, after: after, typing: true, at: now, last: r})
}

// push lands an entry, dropping any redo history — a new edit forks
// the timeline — and trimming the stack to undoLimit.
func (u *undoStack[T]) push(c undoChange[T]) {
	u.undone = nil
	u.done = append(u.done, c)
	if len(u.done) > undoLimit {
		u.done = u.done[1:]
	}
}

// undo restores the state before the most recent entry, reporting
// whether there was one; apply receives the restored snapshot.
func (u *undoStack[T]) undo(apply func(T)) bool {
	if len(u.done) == 0 {
		return false
	}
	c := u.done[len(u.done)-1]
	u.done = u.done[:len(u.done)-1]
	u.undone = append(u.undone, c)
	apply(c.before)
	return true
}

// redo reapplies the most recently undone entry, reporting whether
// there was one; apply receives the replayed after-state.
func (u *undoStack[T]) redo(apply func(T)) bool {
	if len(u.undone) == 0 {
		return false
	}
	c := u.undone[len(u.undone)-1]
	u.undone = u.undone[:len(u.undone)-1]
	u.done = append(u.done, c)
	apply(c.after)
	return true
}

// reset drops the whole history: an app-driven SetText is not an edit
// the user can back out of.
func (u *undoStack[T]) reset() {
	u.done = nil
	u.undone = nil
}
