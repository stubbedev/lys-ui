package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func probedList(rows ...Widget) (*List, *Box) {
	l := NewList[Widget](staticRows(rows), 0)
	host := NewBox(Column, 0, 0)
	host.Append(l, true)
	return l, host
}

func settleList(host *Box) {
	for range 3 {
		SizeWithin(host, Size{W: 100, H: 60})
		host.Arrange(render.Rect{W: 100, H: 60})
		CollectDamage(host)
	}
}

type countedRow struct {
	stub
	arranges int
}

func (c *countedRow) Arrange(r render.Rect) {
	c.arranges++
	c.stub.Arrange(r)
}

func TestASettledAutosizedListStopsArranging(t *testing.T) {
	first := &countedRow{stub: *newStub(20, 10)}
	l, host := probedList(first, newStub(20, 10))
	settleList(host)
	if l.rowH != 10 {
		t.Fatalf("row height %d, want the first row's 10", l.rowH)
	}
	if host.arrangeStale {
		t.Error("a list nothing changed in left its host marked for another arrange")
	}
	arranges := first.arranges
	settleList(host)
	if first.arranges != arranges {
		t.Errorf("idle frames arranged the first row %d more times", first.arranges-arranges)
	}
}

func TestAnAutosizedListProbesTheFirstRowInPlace(t *testing.T) {
	first := newStub(20, 10)
	l, host := probedList(first, newStub(20, 10))
	settleList(host)
	proxy := l.rows[0]
	if first.Parent() != Widget(proxy) {
		t.Fatalf("first row's parent %v, want its realized proxy", first.Parent())
	}
	l.measureRowH()
	if first.Parent() != Widget(proxy) {
		t.Error("measuring the row height took the first row from its realized proxy")
	}
}

func TestAnAutosizedListFollowsItsFirstRowsSize(t *testing.T) {
	first := newStub(20, 10)
	l, host := probedList(first, newStub(20, 10))
	settleList(host)
	first.nat.H = 25
	first.InvalidateLayout()
	settleList(host)
	if l.rowH != 25 {
		t.Errorf("row height %d after the first row grew, want 25", l.rowH)
	}
}

func TestAnAutosizedListFollowsItsRowsStyle(t *testing.T) {
	l, host := probedList(newStub(20, 10), newStub(20, 10))
	settleList(host)
	loadCSS(t, `row { padding: 5px 0; }`)
	settleList(host)
	if l.rowH != 20 {
		t.Errorf("row height %d after a stylesheet padded the rows, want 20", l.rowH)
	}
}

func TestAnAutosizedListScrolledPastItsFirstRowStillMeasuresIt(t *testing.T) {
	rows := make([]Widget, 40)
	for i := range rows {
		rows[i] = newStub(20, 10)
	}
	l, host := probedList(rows...)
	settleList(host)
	l.setOffset(200)
	settleList(host)
	if _, realized := l.rows[0]; realized {
		t.Fatal("the first row is still realized after scrolling past it")
	}
	if parentOf(rows[0]) != Widget(l.probe) {
		t.Errorf("first row's parent %v, want the probe once no proxy holds it", parentOf(rows[0]))
	}
	if host.arrangeStale {
		t.Error("a settled list scrolled past its first row left its host marked")
	}
	rows[0].(*stub).nat.H = 25
	rows[0].(*stub).InvalidateLayout()
	settleList(host)
	if l.rowH != 25 {
		t.Errorf("row height %d, want the first row's 25 with it out of view", l.rowH)
	}
	l.setOffset(0)
	settleList(host)
	if parentOf(rows[0]) != Widget(l.rows[0]) {
		t.Errorf("first row's parent %v, want its proxy back in view", parentOf(rows[0]))
	}
	if host.arrangeStale {
		t.Error("a settled list back at its first row left its host marked")
	}
}

type recyclingRows struct {
	n     int
	bound map[int]Widget
	pool  []Widget
}

func (m *recyclingRows) Len() int { return m.n }

func (m *recyclingRows) Row(i int) Widget {
	if w, ok := m.bound[i]; ok {
		return w
	}
	var w Widget = newStub(20, 10)
	if last := len(m.pool) - 1; last >= 0 {
		w, m.pool = m.pool[last], m.pool[:last]
	}
	m.bound[i] = w
	return w
}

func (m *recyclingRows) unbind(i int) {
	if w, ok := m.bound[i]; ok {
		delete(m.bound, i)
		m.pool = append(m.pool, w)
	}
}

func TestAnAutosizedListLeavesARecycledProbeRowWithItsNewProxy(t *testing.T) {
	model := &recyclingRows{n: 40, bound: map[int]Widget{}}
	l := NewList[Widget](model, 0)
	host := NewBox(Column, 0, 0)
	host.Append(l, true)
	settleList(host)
	recycled := l.probe.row
	l.setOffset(200)
	settleList(host)
	owner, ok := parentOf(recycled).(*listRow)
	if !ok || l.rows[owner.idx] != owner {
		t.Fatalf("recycled row's parent %v, want the proxy that took it", parentOf(recycled))
	}
	if host.arrangeStale {
		t.Error("a settled list whose probe row was recycled left its host marked")
	}
}

func TestAnAutosizedListsRowHeightDoesNotFollowTheSelection(t *testing.T) {
	loadCSS(t, `row:selected { padding: 5px 0; }`)
	l, host := probedList(newStub(20, 10), newStub(20, 10))
	l.SetSelectionMode(SelectionSingle)
	l.Select(0)
	settleList(host)
	if l.rowH != 10 {
		t.Errorf("row height %d with the first row selected as the probe was made, want the plain row's 10", l.rowH)
	}
	if !l.rows[0].HasState(StateSelected) {
		t.Error("the realized first row is not selected")
	}
	if l.probe.HasState(StateSelected) {
		t.Error("the probe took the first row's :selected")
	}
	l.Select(-1)
	settleList(host)
	if l.rowH != 10 {
		t.Errorf("row height %d after clearing the selection, want 10", l.rowH)
	}
	l.Select(0)
	settleList(host)
	if l.rowH != 10 {
		t.Errorf("row height %d after selecting the first row again, want 10", l.rowH)
	}
}
