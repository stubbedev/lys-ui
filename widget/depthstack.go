package widget

// depthStack hands each level of a recursive layout pass its own
// reusable buffer, the walkStack pattern: a level holds its buffer
// across its whole loop while the passes it runs for its children sit
// one level deeper, so no buffer is rewritten while it is still read,
// and a steady frame allocates nothing once the buffers have grown.
// Layout runs on the loop goroutine (docs/threading.md); one stack per
// buffer kind serves the process.
type depthStack[T any] struct {
	bufs  [][]T
	depth int
}

// push descends one level and returns that level's buffer, n zeroed
// elements long. The caller owns it until the matching pop.
func (s *depthStack[T]) push(n int) []T {
	for len(s.bufs) <= s.depth {
		s.bufs = append(s.bufs, nil)
	}
	b := s.bufs[s.depth]
	if cap(b) < n {
		b = make([]T, n)
	}
	b = b[:n]
	clear(b)
	s.bufs[s.depth] = b
	s.depth++
	return b
}

// pop releases the deepest level's buffer, cleared so what it held
// is not kept reachable until the level is next pushed.
func (s *depthStack[T]) pop() {
	s.depth--
	clear(s.bufs[s.depth])
}

// kidStack is the recursive tree walks' child snapshots: each nesting
// level of each keeps its own buffer across walks, a visit that
// recurses into each works one level deeper, and a level is cleared as
// its loop ends, so no widget stays reachable from a buffer after the
// walk that snapshotted it. Tree walks run on the loop goroutine
// (docs/threading.md).
type kidStack struct {
	levels [][]Widget
	depth  int
}

// each visits the snapshot fill appends to the level's buffer; a
// mutation of the tree during the visits changes the next walk, not
// this one.
func (s *kidStack) each(fill func(kids []Widget) []Widget, visit func(Widget)) {
	depth := s.depth
	if len(s.levels) == depth {
		s.levels = append(s.levels, nil)
	}
	kids := fill(s.levels[depth][:0])
	s.levels[depth] = kids
	s.depth++
	defer func() {
		s.depth--
		clear(kids)
	}()
	for _, k := range kids {
		visit(k)
	}
}

// appendKids appends w's children to kids: without the Children copy
// when w can append them itself.
func appendKids(w Widget, kids []Widget) []Widget {
	switch c := w.(type) {
	case childBuf:
		return c.appendChildren(kids)
	case childser:
		return append(kids, c.Children()...)
	}
	return kids
}
