package widget

import (
	"github.com/stubbedev/lys-lib/render"
)

// The tree half of #90: a parent/child row model flattened into the
// visible sequence a ColumnView (or List) renders - the relm4
// TreeListModel analog. Expansion state lives in the FlatTree, the
// source tree is never touched; the expander column is a Column.Cell
// built by TreeCell, and indent guides paint one hairline per depth.

// TreeRow is one node of a static tree: a value and its children.
type TreeRow[T any] struct {
	Value    T
	Children []TreeRow[T]
}

// FlatRow is one visible tree row: its visible index, its depth, its
// value, whether it can expand, and whether it currently is.
type FlatRow[T any] struct {
	Index    int
	Depth    int
	Value    T
	Leaf     bool
	Expanded bool
}

// FlatTree flattens a TreeRow forest into visible rows and owns the
// expansion state. It implements ColumnModel[FlatRow[T]] directly:
//
//	tree := widget.NewFlatTree(roots)
//	view := widget.NewColumnView(face, 14, tree, []widget.TableColumn[widget.FlatRow[item]]{{
//		Title: "Name", Expand: true,
//		Cell: widget.TreeCell(tree, 14, func(it item) Widget {
//			return widget.NewLabel(face, 14, it.Name, 0)
//		}),
//	}})
type FlatTree[T any] struct {
	roots []TreeRow[T]
	// expanded holds the all-nodes index of every expanded row. A
	// node's index among ALL nodes is stable under expansion changes -
	// only visibility moves - so a toggle survives re-flattening.
	expanded map[int]bool
	notify   func()
	rows     []flatNode[T]
}

type flatNode[T any] struct {
	row      FlatRow[T]
	children []TreeRow[T]
}

// NewFlatTree flattens roots with every collapsible row expanded.
func NewFlatTree[T any](roots []TreeRow[T]) *FlatTree[T] {
	f := &FlatTree[T]{roots: roots, expanded: map[int]bool{}}
	f.walk(func(_ int, _ TreeRow[T], i int, leaf bool) {
		if !leaf {
			f.expanded[i] = true
		}
	})
	f.reflatten()
	return f
}

// walk visits every visible node depth-first, pre-order, handing the
// walk's all-nodes index i to fn. The walk consults expanded itself,
// so the visited sequence is exactly the flattened rows.
func (f *FlatTree[T]) walk(fn func(depth int, row TreeRow[T], i int, leaf bool)) {
	var i int
	var descend func(depth int, nodes []TreeRow[T])
	descend = func(depth int, nodes []TreeRow[T]) {
		for _, n := range nodes {
			leaf := len(n.Children) == 0
			fn(depth, n, i, leaf)
			idx := i
			i++
			if !leaf && f.expanded[idx] {
				descend(depth+1, n.Children)
			}
		}
	}
	descend(0, f.roots)
}

// reflatten rebuilds the visible rows from the walk.
func (f *FlatTree[T]) reflatten() {
	f.rows = f.rows[:0]
	f.walk(func(depth int, row TreeRow[T], i int, leaf bool) {
		f.rows = append(f.rows, flatNode[T]{
			row: FlatRow[T]{
				Index:    len(f.rows),
				Depth:    depth,
				Value:    row.Value,
				Leaf:     leaf,
				Expanded: !leaf && f.expanded[i],
			},
			children: row.Children,
		})
	})
}

// Len implements ColumnModel.
func (f *FlatTree[T]) Len() int { return len(f.rows) }

// Row implements ColumnModel.
func (f *FlatTree[T]) Row(i int) FlatRow[T] {
	if i < 0 || i >= len(f.rows) {
		return FlatRow[T]{}
	}
	return f.rows[i].row
}

// Toggle flips visible row i's expansion (leaves stay). The caller
// refreshes the view (its List's Reset, or the view's own recompute
// through whatever owns it) - the tree never reaches into a view it
// does not own.
func (f *FlatTree[T]) Toggle(i int) {
	if i < 0 || i >= len(f.rows) || f.rows[i].row.Leaf {
		return
	}
	node := -1
	visible := 0
	f.walk(func(_ int, _ TreeRow[T], idx int, _ bool) {
		if visible == i {
			node = idx
		}
		visible++
	})
	if node < 0 {
		return
	}
	if f.expanded[node] {
		delete(f.expanded, node)
	} else {
		f.expanded[node] = true
	}
	f.reflatten()
}

// indentStep is one depth level's horizontal inset, and the guide line
// the level draws.
const (
	indentStep = 16
	guideX     = 6
)

// TreeCell builds the expander column's Cell: indent guides for every
// ancestor level, the expander toggle (a leaf paints a dot-sized gap
// instead), then the value's own cell. Activating the toggle calls
// tree.Toggle(i) and then refresh - the default refresh re-lists the
// owning List; pass your own when the tree feeds something else.
func TreeCell[T any](tree *FlatTree[T], sizePx float64, cell func(T) Widget) func(FlatRow[T]) Widget {
	return func(row FlatRow[T]) Widget {
		out := NewBox(Row, 2, 0)
		out.Append(&indentGuides{depth: row.Depth}, false)
		if !row.Leaf {
			arrow := SymbolChevronRight
			if row.Expanded {
				arrow = SymbolChevronDown
			}
			toggle := NewButton(NewSymbol(arrow, int(sizePx)), 4, 2)
			which := row.Index
			toggle.OnClick = func() {
				tree.Toggle(which)
				tree.Notify()
			}
			out.Append(toggle, false)
		} else {
			out.Append(NewSpacer(18, 0), false)
		}
		if cell != nil {
			out.Append(cell(row.Value), false)
		}
		return out
	}
}

// SetRefresh installs the hook TreeCell's toggle runs after a
// collapse or expand (a ColumnView's owner wires it to the view's
// List().Reset).
func (f *FlatTree[T]) SetRefresh(fn func()) { f.notify = fn }

// Notify runs the installed refresh hook.
func (f *FlatTree[T]) Notify() {
	if f.notify != nil {
		f.notify()
	}
}

// indentGuides paints one hairline per ancestor depth level, the tree
// view's indentation rails.
type indentGuides struct {
	node
	depth int
}

func (g *indentGuides) Measure(axis Axis, forSize int) Sizes {
	return FixedSizes(Size{W: indentStep * g.depth, H: 1}.Along(axis))
}

func (g *indentGuides) Arrange(r render.Rect) { g.node.Arrange(r) }

func (g *indentGuides) Paint(cv *render.Canvas) {
	th := Current()
	b := g.Bounds()
	for d := range g.depth {
		x := b.X + d*indentStep + guideX
		cv.FillRect(render.Rect{X: x, Y: b.Y, W: 1, H: b.H}, th.Border)
	}
}

func (g *indentGuides) HitTest(p Point) Widget {
	if g.Bounds().Contains(p.X, p.Y) {
		return g
	}
	return nil
}
