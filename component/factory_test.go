package component_test

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/component/componenttest"
	"github.com/stubbedev/lys-ui/widget"
)

type rowMsg int

const (
	rowBump rowMsg = iota
	rowDelete
)

type rowOut struct{ remove bool }

type row struct {
	face    *render.Typeface
	name    string
	n       int
	label   *widget.Label
	index   *component.Index
	stopped *[]string
}

func (r *row) Init(cx *component.Context[rowMsg, rowOut]) widget.Widget {
	r.index = cx.Index()
	r.label = widget.NewLabel(r.face, 12, "", widget.Current().Text)
	cx.Watch(func() { r.label.SetText(r.name + strconv.Itoa(r.n)) })
	return r.label
}

func (r *row) Update(cx *component.Context[rowMsg, rowOut], msg rowMsg) {
	switch msg {
	case rowBump:
		r.n++
	case rowDelete:
		cx.Output(rowOut{remove: true})
	}
}

func (r *row) Shutdown(*component.Context[rowMsg, rowOut]) {
	if r.stopped != nil {
		*r.stopped = append(*r.stopped, r.name)
	}
}

type list struct {
	face    *render.Typeface
	box     *widget.Box
	rows    *component.Factory[*row, rowMsg, rowOut]
	stopped []string
	removed []int
}

type listMsg struct{ remove int }

func (l *list) Init(cx *component.Context[listMsg, struct{}]) widget.Widget {
	l.box = widget.NewBox(widget.Column, 0, 0)
	l.rows = cx.NewFactory(component.BoxView[*row](l.box, false)).
		ForwardTo(cx.Sender(), func(x *component.Index, o rowOut) listMsg { return listMsg{remove: x.Current()} })
	for _, name := range []string{"a", "b", "c"} {
		l.rows.PushBack(&row{face: l.face, name: name, stopped: &l.stopped})
	}
	return l.box
}

func (l *list) Update(_ *component.Context[listMsg, struct{}], msg listMsg) {
	l.removed = append(l.removed, msg.remove)
	l.rows.Remove(msg.remove)
}

func names(f *component.Factory[*row, rowMsg, rowOut]) []string {
	var out []string
	for _, r := range f.All() {
		out = append(out, r.name)
	}
	return out
}

func widgets(rs ...*row) []widget.Widget {
	out := make([]widget.Widget, len(rs))
	for i, r := range rs {
		out[i] = r.label
	}
	return out
}

func TestFactoryEditsApplyInPlaceWithStableIndices(t *testing.T) {
	var loop componenttest.Loop
	l := &list{face: face(t)}
	ctrl := component.Launch(&loop, l)
	a, b, c := l.rows.Get(0), l.rows.Get(1), l.rows.Get(2)
	if got := l.box.Children(); !slices.Equal(got, widgets(a, b, c)) {
		t.Fatalf("box holds %v, want the three rows in order", got)
	}

	d := &row{face: l.face, name: "d", stopped: &l.stopped}
	l.rows.PushFront(d)
	l.rows.Move(3, 1)
	if got := names(l.rows); !slices.Equal(got, []string{"d", "c", "a", "b"}) {
		t.Fatalf("order %v after PushFront and Move, want [d c a b]", got)
	}
	if got := l.box.Children(); !slices.Equal(got, widgets(d, c, a, b)) {
		t.Errorf("the box does not follow the factory: %v", got)
	}
	if c.index.Current() != 1 || a.index.Current() != 2 || d.index.Current() != 0 {
		t.Errorf("indices d=%d c=%d a=%d, want 0 1 2", d.index.Current(), c.index.Current(), a.index.Current())
	}

	l.rows.Swap(0, 3)
	if got := names(l.rows); !slices.Equal(got, []string{"b", "c", "a", "d"}) {
		t.Errorf("order %v after Swap(0, 3), want [b c a d]", got)
	}

	l.rows.Send(1, rowBump)
	l.rows.Broadcast(rowBump)
	loop.Settle()
	if c.label.Text() != "c2" || a.label.Text() != "a1" {
		t.Errorf("labels c=%q a=%q, want c2 and a1", c.label.Text(), a.label.Text())
	}

	l.rows.Send(2, rowDelete)
	loop.Settle()
	if !slices.Equal(l.removed, []int{2}) || a.index.Current() != -1 {
		t.Fatalf("an item's output reached the parent as %v, index now %d", l.removed, a.index.Current())
	}
	if got := names(l.rows); !slices.Equal(got, []string{"b", "c", "d"}) || d.index.Current() != 2 {
		t.Errorf("after the removal order=%v d at %d", got, d.index.Current())
	}
	if !slices.Equal(l.stopped, []string{"a"}) {
		t.Errorf("stopped %v, want the removed row only", l.stopped)
	}

	ctrl.Shutdown()
	if !slices.Equal(l.stopped, []string{"a", "d", "c", "b"}) {
		t.Errorf("parent shutdown stopped %v, want the rest last first", l.stopped)
	}
}

func TestFactoryIndexOutOfRangePanics(t *testing.T) {
	var loop componenttest.Loop
	f := component.NewFactory(&loop, component.BoxView[*row](widget.NewBox(widget.Column, 0, 0), false))
	defer func() {
		if recover() == nil {
			t.Error("Remove on an empty factory did not panic")
		}
	}()
	f.Remove(0)
}

func fill(t *testing.T, f *component.Factory[*row, rowMsg, rowOut], names ...string) []*row {
	t.Helper()
	rows := make([]*row, len(names))
	for i, n := range names {
		rows[i] = &row{face: face(t), name: n}
		f.PushBack(rows[i])
	}
	return rows
}

func TestFlowBoxStackAndNotebookViewsFollowEdits(t *testing.T) {
	var loop componenttest.Loop

	flow := widget.NewFlowBox(0, 0)
	ff := component.NewFactory(&loop, component.FlowBoxView[*row](flow))
	fr := fill(t, ff, "a", "b", "c")
	ff.Move(0, 2)
	if flow.ChildAt(2).Child() != fr[0].label || flow.Len() != 3 {
		t.Errorf("flowbox did not follow the move")
	}

	stack := widget.NewStack()
	sf := component.NewFactory(&loop, component.StackView(stack, func(r *row) string { return "page-" + r.name }))
	fill(t, sf, "a", "b", "c")
	sf.Move(2, 0)
	sf.Remove(1)
	if got := stack.Order(); !slices.Equal(got, []string{"page-c", "page-b"}) {
		t.Errorf("stack pages %v, want [page-c page-b]", got)
	}

	nb := widget.NewNotebook(face(t))
	nf := component.NewFactory(&loop, component.NotebookView(nb, func(r *row) string { return "same" }))
	nr := fill(t, nf, "a", "b", "c")
	nf.Remove(0)
	nf.Move(1, 0)
	if kids := nb.Children(); len(kids) != 1 {
		t.Errorf("notebook shows %d pages", len(kids))
	}
	nb.SelectTab("same")
	if nb.Children()[0] != nr[2].label {
		t.Errorf("duplicate titles confused the notebook: first page is not row c")
	}
}

func TestGridViewRePlacesShiftedItems(t *testing.T) {
	var loop componenttest.Loop
	grid := widget.NewGrid(0, 0)
	gf := component.NewFactory(&loop, component.GridView[*row](grid, func(i int) (int, int, int, int) { return i % 2, i / 2, 1, 1 }))
	removed := 0
	widget.SetRemovedHook(func(widget.Widget) { removed++ })
	defer widget.SetRemovedHook(nil)
	rows := fill(t, gf, "a", "b", "c", "d")
	z := &row{face: face(t), name: "z"}
	gf.PushFront(z)
	gf.Move(4, 0)
	a, b, c, d := rows[0], rows[1], rows[2], rows[3]
	if removed != 0 {
		t.Errorf("re-placing items detached %d widgets", removed)
	}
	if got := grid.Children(); !slices.Equal(got, widgets(d, z, a, b, c)) {
		t.Fatalf("grid in row-major order holds %v, want d z / a b / c", got)
	}
	gf.Remove(0)
	if got := grid.Children(); removed != 1 || !slices.Equal(got, widgets(z, a, b, c)) {
		t.Errorf("after removing d: detached %d, grid %v, want z a / b c", removed, got)
	}
}

func TestFixedViewPositionsItems(t *testing.T) {
	var loop componenttest.Loop
	fixed := widget.NewFixed()
	ff := component.NewFactory(&loop, component.FixedView[*row](fixed, func(i int) (int, int) { return i * 10, i * 5 }))
	removed := 0
	widget.SetRemovedHook(func(widget.Widget) { removed++ })
	defer widget.SetRemovedHook(nil)
	rows := fill(t, ff, "a", "b", "c")

	placed := func() []render.Rect {
		t.Helper()
		nat := fixed.Measure(widget.Constraints{Max: widget.Size{W: 300, H: 300}})
		fixed.Arrange(render.Rect{X: 0, Y: 0, W: nat.W, H: nat.H})
		out := make([]render.Rect, len(rows))
		for i, r := range rows {
			out[i] = r.label.Bounds()
		}
		return out
	}

	if got := placed(); !slices.EqualFunc(got, []render.Rect{
		{X: 0, Y: 0}, {X: 10, Y: 5}, {X: 20, Y: 10},
	}, func(a, b render.Rect) bool { return a.X == b.X && a.Y == b.Y }) {
		t.Fatalf("positions %v, want a b c at 10px steps", got)
	}

	ff.Move(2, 0)
	if got := placed(); !slices.EqualFunc(got, []render.Rect{
		{X: 10, Y: 5}, {X: 20, Y: 10}, {X: 0, Y: 0},
	}, func(a, b render.Rect) bool { return a.X == b.X && a.Y == b.Y }) {
		t.Errorf("after Move(2, 0) positions %v, want c a b at 10px steps", got)
	}
	if removed != 0 {
		t.Errorf("re-placing items detached %d widgets", removed)
	}

	ff.Remove(2)
	if got := fixed.Children(); !slices.Equal(got, widgets(rows[0], rows[2])) {
		t.Errorf("after removing b: fixed %v, want a c", got)
	}
}

type asyncRow struct {
	index *component.Index
	ready bool
}

func (r *asyncRow) Loading() widget.Widget { return widget.NewSpacer(1, 1) }
func (r *asyncRow) Load(context.Context)   { time.Sleep(time.Second) }

func (r *asyncRow) Init(cx *component.Context[int, int]) widget.Widget {
	r.index, r.ready = cx.Index(), true
	return widget.NewSpacer(2, 2)
}

func (r *asyncRow) Update(*component.Context[int, int], int) {}

func TestFactoryItemsMayLoadAsynchronously(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		box := widget.NewBox(widget.Column, 0, 0)
		f := component.NewFactory(&loop, component.BoxView[*asyncRow](box, false))
		first, second := &asyncRow{}, &asyncRow{}
		f.PushBack(first)
		f.PushBack(second)
		f.Move(1, 0)
		if first.ready || len(box.Children()) != 2 {
			t.Fatalf("items initialized before loading: ready=%v children=%d", first.ready, len(box.Children()))
		}
		time.Sleep(time.Second)
		synctest.Wait()
		loop.Settle()
		if !first.ready || first.index.Current() != 1 || second.index.Current() != 0 {
			t.Errorf("after loading ready=%v indices %d %d, want 1 0", first.ready, first.index.Current(), second.index.Current())
		}
	})
}
