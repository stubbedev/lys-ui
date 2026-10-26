package widget

import "slices"

// selectionHost is the container a selection model drives: List and
// FlowBox. The model owns which items are selected and how gestures
// change that; the host owns how items look, scroll, and announce.
type selectionHost interface {
	// count is the number of items.
	count() int
	// selectionSync mirrors membership into the items (their :selected
	// state, a multiple-mode checkbox) and repaints.
	selectionSync()
	// selectionNotify reports a changed set; a host holds it while a
	// pointer gesture is in flight so the gesture reports once.
	selectionNotify()
	// selectionSingle hears every single- or browse-mode move.
	selectionSingle(i int)
	// reveal scrolls item i into view.
	reveal(i int)
	// activate fires item i's activation.
	activate(i int)
}

// selection is the selection model selectable containers share: the
// mode, the single selection (the multiple-mode anchor), the multiple
// set, and the keyboard cursor, with every gesture's transition -
// click, rubber band, keys - defined once.
type selection struct {
	host   selectionHost
	mode   SelectionMode
	sel    int
	multi  map[int]struct{}
	cursor int
	// held defers notifications while a pointer gesture is in flight;
	// pending records one was owed.
	held, pending bool
	// revealAt is item revealAt-1, asked to be revealed before it had a
	// place, for the host's next Arrange; 0 for none.
	revealAt int
}

// revealItem scrolls item i, w, into view in every Scroll above it: now,
// or once the host's next Arrange (revealArranged) gives it a place.
func (s *selection) revealItem(i int, w Widget) {
	s.revealAt = 0
	if b := boundsOf(w); !b.Empty() {
		RevealRect(w, b)
		return
	}
	s.revealAt = i + 1
}

// revealArranged runs a reveal asked for before layout, once, with
// item(i) the host's item i (nil when gone): a later scroll of the
// user's own stands.
func (s *selection) revealArranged(item func(i int) Widget) {
	if s.revealAt == 0 {
		return
	}
	i := s.revealAt - 1
	s.revealAt = 0
	if w := item(i); w != nil {
		if b := boundsOf(w); !b.Empty() {
			RevealRect(w, b)
		}
	}
}

// initSelection binds the model to its host, empty, in mode.
func (s *selection) initSelection(host selectionHost, mode SelectionMode) {
	s.host, s.sel, s.cursor = host, -1, -1
	s.mode = mode
	if mode == SelectionMultiple {
		s.multi = map[int]struct{}{}
	}
}

// SetSelectionMode switches the selection model. Leaving multiple
// mode keeps the lowest selected item as the single selection (or
// clears it when the set was empty); entering it seeds the set with
// the current selection and anchors there. The change notification
// fires when the switch changes the set.
func (s *selection) SetSelectionMode(mode SelectionMode) {
	if mode == s.mode {
		return
	}
	before := s.Selection()
	switch mode {
	case SelectionMultiple:
		s.multi = make(map[int]struct{})
		if s.sel >= 0 {
			s.multi[s.sel] = struct{}{}
		}
		s.cursor = s.sel
	default:
		s.sel = -1
		if len(s.multi) > 0 {
			s.sel = s.Selection()[0]
		}
		s.multi = nil
		s.cursor = -1
	}
	s.mode = mode
	s.host.selectionSync()
	if after := s.Selection(); !slices.Equal(before, after) {
		s.notify()
	}
}

// notify reports a changed set to the host - once per gesture: while
// a pointer gesture holds notifications the report lands at its end,
// so a rubber band repainting through many sets reports exactly once,
// with the final set.
func (s *selection) notify() {
	if s.held {
		s.pending = true
		return
	}
	s.host.selectionNotify()
}

// hold begins a pointer gesture's notification hold.
func (s *selection) hold() { s.held = true }

// release ends the hold, landing an owed notification.
func (s *selection) release() {
	s.held = false
	if s.pending {
		s.pending = false
		s.host.selectionNotify()
	}
}

// SelectionMode returns the current selection model.
func (s *selection) SelectionMode() SelectionMode { return s.mode }

// Selection returns the whole selected set, ascending; empty when
// nothing is selected.
func (s *selection) Selection() []int {
	switch s.mode {
	case SelectionMultiple:
		rows := make([]int, 0, len(s.multi))
		for i := range s.multi {
			rows = append(rows, i)
		}
		slices.Sort(rows)
		return rows
	case SelectionBrowse, SelectionSingle:
		if s.sel >= 0 {
			return []int{s.sel}
		}
	}
	return nil
}

// Selected returns the selected item in single and browse mode; in
// multiple mode the anchor the last gesture or Select settled on, -1
// when the set is empty. Selection reports the whole set.
func (s *selection) Selected() int { return s.sel }

// Select moves the selection to item i (clamped, -1 clears). In
// multiple mode it collapses the set to the one item (-1 clears it);
// in none mode it does nothing.
func (s *selection) Select(i int) {
	switch s.mode {
	case SelectionNone:
		return
	case SelectionMultiple:
		i = min(max(i, -1), s.host.count()-1)
		set := make(map[int]struct{})
		if i >= 0 {
			set[i] = struct{}{}
		}
		s.sel, s.cursor = i, i
		s.applyMulti(set)
		s.notify()
	default:
		s.selectOne(i)
	}
}

// SelectRows makes rows the multiple-mode selection (out-of-range
// rows are dropped) and notifies; outside multiple mode it does
// nothing.
func (s *selection) SelectRows(rows ...int) {
	if s.mode != SelectionMultiple {
		return
	}
	n := s.host.count()
	set := make(map[int]struct{}, len(rows))
	for _, r := range rows {
		if r >= 0 && r < n {
			set[r] = struct{}{}
		}
	}
	s.applyMulti(set)
	s.notify()
}

// SelectAll selects every item. It is the ctrl+a target through
// Router.SelectAll and does nothing outside multiple mode, where
// "all" is not a representable selection.
func (s *selection) SelectAll() {
	n := s.host.count()
	if s.mode != SelectionMultiple || n == 0 {
		return
	}
	set := make(map[int]struct{}, n)
	for i := range n {
		set[i] = struct{}{}
	}
	s.applyMulti(set)
	s.notify()
}

// selectOne is the single- and browse-mode core: clamp, move, reveal,
// sync, and notify on change. Browse keeps one item: a clearing move
// pins to the first instead.
func (s *selection) selectOne(i int) {
	n := s.host.count()
	i = min(max(i, -1), n-1)
	if s.mode == SelectionBrowse && i < 0 && n > 0 {
		i = 0
	}
	if i == s.sel {
		return
	}
	s.sel = i
	if i >= 0 {
		s.host.reveal(i)
	}
	s.host.selectionSync()
	s.host.selectionSingle(i)
	s.notify()
}

// isSelected reports membership under the active mode.
func (s *selection) isSelected(i int) bool {
	if s.mode == SelectionMultiple {
		_, ok := s.multi[i]
		return ok
	}
	return i == s.sel
}

// applyMulti replaces the multiple-mode set wholesale and syncs;
// callers notify.
func (s *selection) applyMulti(set map[int]struct{}) {
	s.multi = set
	s.host.selectionSync()
}

// toggle flips one item's membership and anchors there.
func (s *selection) toggle(i int) {
	set := make(map[int]struct{}, len(s.multi)+1)
	for j := range s.multi {
		set[j] = struct{}{}
	}
	if _, ok := set[i]; ok {
		delete(set, i)
	} else {
		set[i] = struct{}{}
	}
	s.sel, s.cursor = i, i
	s.applyMulti(set)
	s.notify()
}

// band selects exactly the items from a to b inclusive (a rubber band,
// a shift extension); the anchor stays.
func (s *selection) band(a, b int) {
	set := make(map[int]struct{}, max(a, b)-min(a, b)+1)
	for i := min(a, b); i <= max(a, b); i++ {
		set[i] = struct{}{}
	}
	s.applyMulti(set)
	s.notify()
}

// click applies a click on item i: select it, or toggle it in
// multiple mode.
func (s *selection) click(i int) {
	switch s.mode {
	case SelectionSingle, SelectionBrowse:
		s.selectOne(i)
	case SelectionMultiple:
		s.toggle(i)
	}
}

// drag applies a pointer drag from item anchor to item at: the rubber
// band in multiple mode, motion-select in single and browse.
func (s *selection) drag(anchor, at int) {
	switch s.mode {
	case SelectionMultiple:
		s.band(anchor, at)
	case SelectionSingle, SelectionBrowse:
		s.selectOne(at)
	}
}

// key applies a selection key. nav maps a motion key from an item to
// its target (the host's geometry: lines, pages); false is no motion.
// Home and End reach the ends, Enter activates the selection. In
// multiple mode plain motion collapses to the target, shift extends
// from the anchor, ctrl moves only the cursor, and ctrl+space toggles
// the cursor's item.
func (s *selection) key(a KeyAction, mods Mods, nav func(from int, a KeyAction) (int, bool)) {
	n := s.host.count()
	if n == 0 || s.mode == SelectionNone {
		return
	}
	multiple := s.mode == SelectionMultiple
	ctrl, shift := mods&ModCtrl != 0, mods&ModShift != 0
	from := s.sel
	if multiple {
		from = s.cursor
	}
	var target int
	switch a {
	case KeyEnter:
		for _, i := range s.Selection() {
			s.host.activate(i)
		}
		return
	case KeySpace:
		if multiple && ctrl {
			s.toggle(s.cursor)
		}
		return
	case KeyHome:
		target = 0
	case KeyEnd:
		target = n - 1
	default:
		t, ok := nav(from, a)
		if !ok {
			return
		}
		target = t
	}
	target = min(max(target, 0), n-1)
	if !multiple {
		s.Select(target)
		return
	}
	s.cursor = target
	switch {
	case ctrl && !shift:
		s.host.selectionSync()
	case shift:
		s.band(s.sel, target)
	default:
		s.sel = target
		s.applyMulti(map[int]struct{}{target: {}})
		s.notify()
	}
	s.host.reveal(target)
}

// clampTo drops what a shrunk item count no longer holds.
func (s *selection) clampTo(n int) {
	s.sel = min(s.sel, n-1)
	s.cursor = min(s.cursor, n-1)
	for i := range s.multi {
		if i >= n {
			delete(s.multi, i)
		}
	}
}

// clearAll empties the model without notifying; reports whether a
// selection was dropped.
func (s *selection) clearAll() bool {
	had := len(s.Selection()) > 0
	s.sel, s.cursor = -1, -1
	if s.multi != nil {
		s.multi = map[int]struct{}{}
	}
	return had
}

// inserted shifts the model past a new item at i.
func (s *selection) inserted(i int) {
	shift := func(j int) int {
		if j >= i {
			return j + 1
		}
		return j
	}
	s.sel, s.cursor = shift(s.sel), shift(s.cursor)
	if s.multi != nil {
		set := make(map[int]struct{}, len(s.multi))
		for j := range s.multi {
			set[shift(j)] = struct{}{}
		}
		s.multi = set
	}
	s.host.selectionSync()
}

// moved follows item from to index to, shifting the items between.
func (s *selection) moved(from, to int) {
	shift := func(j int) int {
		switch {
		case j == from:
			return to
		case from < to && j > from && j <= to:
			return j - 1
		case from > to && j >= to && j < from:
			return j + 1
		}
		return j
	}
	s.sel, s.cursor = shift(s.sel), shift(s.cursor)
	if s.multi != nil {
		set := make(map[int]struct{}, len(s.multi))
		for j := range s.multi {
			set[shift(j)] = struct{}{}
		}
		s.multi = set
	}
	s.host.selectionSync()
}

// removed drops item i and shifts the rest down, notifying when the
// removed item was selected.
func (s *selection) removed(i int) {
	was := s.isSelected(i)
	shift := func(j int) int {
		switch {
		case j == i:
			return -1
		case j > i:
			return j - 1
		}
		return j
	}
	s.sel, s.cursor = shift(s.sel), shift(s.cursor)
	if s.multi != nil {
		set := make(map[int]struct{}, len(s.multi))
		for j := range s.multi {
			if k := shift(j); k >= 0 {
				set[k] = struct{}{}
			}
		}
		s.multi = set
	}
	s.host.selectionSync()
	if was {
		s.notify()
	}
}
