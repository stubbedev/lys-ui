package widget

import (
	"strings"
	"time"

	"github.com/stubbedev/lys-ui/anim"
)

// typeAheadTimeout is how long a typed prefix survives after the last
// key before the type-ahead chain resets.
const typeAheadTimeout = time.Second

// typeAhead is a list's type-ahead chain (GTK combobox parity): a
// printable key jumps to the first row whose label starts with the
// typed prefix, case-insensitively, a repeated key cycles among the
// rows sharing it, and the chain resets after typeAheadTimeout idle.
type typeAhead struct {
	typed  string
	cancel anim.Cancel
}

// next applies r to the chain over n rows, label(i) giving row i's
// label and whether it takes part, and returns the row to move to from
// current. A key that matches nothing reports false and leaves every
// bit of state alone, so it never fights the keys around it. Each
// accepted key restarts the idle timer.
func (t *typeAhead) next(r rune, current, n int, label func(i int) (string, bool)) (int, bool) {
	key := strings.ToLower(string(r))
	prefix := t.typed + key
	rows := prefixRows(prefix, n, label)
	cycling := false
	if len(rows) == 0 {
		// The extension matched nothing: a fresh single-letter chain
		// (the repeated-key case, or a stale multi-letter buffer) is the
		// retry; a letter with no rows at all is a pass-through.
		prefix = key
		rows = prefixRows(key, n, label)
		cycling = t.typed != ""
		if len(rows) == 0 {
			return 0, false
		}
	}
	target := rows[0]
	if cycling {
		for _, i := range rows {
			if i > current {
				target = i
				break
			}
		}
	}
	t.typed = prefix
	t.arm()
	return target, true
}

// prefixRows returns the rows among n whose taking-part label starts
// with the case-folded prefix, ascending.
func prefixRows(prefix string, n int, label func(i int) (string, bool)) []int {
	var rows []int
	for i := range n {
		if l, ok := label(i); ok && strings.HasPrefix(strings.ToLower(l), prefix) {
			rows = append(rows, i)
		}
	}
	return rows
}

// arm (re)starts the idle timer that clears the chain; the delay keeps
// its duration under reduced motion, like every timing skeleton.
func (t *typeAhead) arm() {
	if t.cancel != nil {
		t.cancel()
	}
	t.cancel = anim.Play(anim.Sequence(
		anim.Delay(typeAheadTimeout),
		anim.Animate(0, func(float64) {
			t.typed = ""
			t.cancel = nil
		}),
	))
}
