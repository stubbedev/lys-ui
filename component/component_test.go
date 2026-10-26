package component_test

import (
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/component/componenttest"
	"github.com/stubbedev/lys-ui/widget"
)

func face(t *testing.T) *render.Typeface {
	t.Helper()
	f, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

type counterMsg int

const (
	increment counterMsg = iota
	reset
)

type counterOut struct{ n int }

type counter struct {
	face    *render.Typeface
	n       int
	label   *widget.Label
	updates []counterMsg
	views   int
	stopped *[]string
	name    string
}

func (c *counter) Init(cx *component.Context[counterMsg, counterOut]) widget.Widget {
	c.label = widget.NewLabel(c.face, 12, "", widget.Current().Text)
	cx.Watch(func() { c.label.SetText(strconv.Itoa(c.n)) })
	return c.label
}

func (c *counter) Update(cx *component.Context[counterMsg, counterOut], msg counterMsg) {
	c.updates = append(c.updates, msg)
	switch msg {
	case increment:
		c.n++
		cx.Output(counterOut{n: c.n})
	case reset:
		c.n = 0
	}
}

func (c *counter) UpdateView(*component.Context[counterMsg, counterOut]) { c.views++ }

func (c *counter) Shutdown(cx *component.Context[counterMsg, counterOut]) {
	if c.stopped != nil {
		*c.stopped = append(*c.stopped, c.name)
	}
	cx.Output(counterOut{n: -1})
}

func TestUpdateRunsOnLoopInOrderAndRefreshesOncePerBatch(t *testing.T) {
	var loop componenttest.Loop
	c := &counter{face: face(t)}
	ctrl := component.Launch(&loop, c)
	if ctrl.Widget() != c.label || c.label.Text() != "0" {
		t.Fatalf("root = %v with text %q, want the label showing 0", ctrl.Widget(), c.label.Text())
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 3 {
			ctrl.Send(increment)
		}
	})
	wg.Wait()
	if len(c.updates) != 0 {
		t.Fatal("Update ran off the loop")
	}
	if ran := loop.Pass(); ran != 1 {
		t.Errorf("three sends cost %d loop invokes, want 1", ran)
	}
	if c.n != 3 || c.label.Text() != "3" {
		t.Errorf("after one pass n=%d label=%q, want 3", c.n, c.label.Text())
	}
	if c.views != 1 {
		t.Errorf("UpdateView ran %d times for one batch, want 1", c.views)
	}
}

type selfSender struct {
	order []int
}

func (s *selfSender) Init(*component.Context[int, struct{}]) widget.Widget {
	return widget.NewSpacer(1, 1)
}

func (s *selfSender) Update(cx *component.Context[int, struct{}], msg int) {
	s.order = append(s.order, msg)
	if msg == 1 {
		cx.Input(3)
	}
}

func TestSelfInputLandsOnTheNextPass(t *testing.T) {
	var loop componenttest.Loop
	s := &selfSender{}
	ctrl := component.Launch(&loop, s)
	ctrl.Send(1)
	ctrl.Send(2)
	loop.Pass()
	if !slices.Equal(s.order, []int{1, 2}) {
		t.Fatalf("first pass delivered %v, want [1 2]", s.order)
	}
	loop.Pass()
	if !slices.Equal(s.order, []int{1, 2, 3}) {
		t.Errorf("self input delivered %v, want 3 on the next pass", s.order)
	}
}

func TestOutputsForwardOnTheLoop(t *testing.T) {
	var loop componenttest.Loop
	var got []int
	ctrl := component.Launch(&loop, &counter{face: face(t)}).Forward(func(o counterOut) { got = append(got, o.n) })
	ctrl.Send(increment)
	ctrl.Send(increment)
	loop.Settle()
	if !slices.Equal(got, []int{1, 2}) {
		t.Errorf("forwarded %v, want [1 2]", got)
	}
}

type parentMsg struct{ child int }

type parent struct {
	face     *render.Typeface
	box      *widget.Box
	children []*component.Controller[counterMsg, counterOut]
	heard    []int
	stopped  *[]string
}

func (p *parent) Init(cx *component.Context[parentMsg, struct{}]) widget.Widget {
	p.box = widget.NewBox(widget.Column, 0, 0)
	for i := range 2 {
		child := cx.Launch(&counter{face: p.face, stopped: p.stopped, name: "child" + strconv.Itoa(i)}).
			ForwardTo(cx.Sender(), func(o counterOut) parentMsg { return parentMsg{child: o.n} })
		p.children = append(p.children, child)
		p.box.Append(child.Widget(), false)
	}
	cx.OnShutdown(func() { *p.stopped = append(*p.stopped, "parent hook") })
	return p.box
}

func (p *parent) Update(_ *component.Context[parentMsg, struct{}], msg parentMsg) {
	p.heard = append(p.heard, msg.child)
}

func TestChildOutputsReachTheParentAndShutdownCascades(t *testing.T) {
	var loop componenttest.Loop
	var stopped []string
	p := &parent{face: face(t), stopped: &stopped}
	ctrl := component.Launch(&loop, p)
	if got := len(p.box.Children()); got != 2 {
		t.Fatalf("parent embeds %d children, want 2", got)
	}
	p.children[0].Send(increment)
	p.children[1].Send(increment)
	p.children[1].Send(increment)
	loop.Settle()
	if !slices.Equal(p.heard, []int{1, 1, 2}) {
		t.Errorf("parent heard %v, want [1 1 2]", p.heard)
	}

	ctrl.Shutdown()
	loop.Settle()
	if !slices.Equal(stopped, []string{"child1", "child0", "parent hook"}) {
		t.Errorf("shutdown order %v, want children last-first, then the parent's hooks", stopped)
	}
	if p.children[0].Alive() || ctrl.Alive() {
		t.Error("a controller is still alive after its parent shut down")
	}
	if !slices.Equal(p.heard, []int{1, 1, 2}) {
		t.Errorf("parent heard %v after shutdown, want nothing new", p.heard)
	}
	p.children[0].Send(increment)
	loop.Settle()
	if c := p.children[0]; c.Alive() {
		t.Error("a send revived a dead child")
	}
}

func TestShutdownOutputsAreDelivered(t *testing.T) {
	var loop componenttest.Loop
	var got []int
	ctrl := component.Launch(&loop, &counter{face: face(t)}).Forward(func(o counterOut) { got = append(got, o.n) })
	ctrl.Shutdown()
	ctrl.Shutdown()
	if !slices.Equal(got, []int{-1}) {
		t.Errorf("shutdown forwarded %v, want the one final output", got)
	}
}

type launcher struct {
	child *component.Controller[counterMsg, counterOut]
	face  *render.Typeface
	stop  *[]string
}

func (l *launcher) Init(cx *component.Context[int, struct{}]) widget.Widget {
	l.child = cx.Launch(&counter{face: l.face, stopped: l.stop, name: "detached"}).Detach()
	return widget.NewSpacer(1, 1)
}

func (l *launcher) Update(*component.Context[int, struct{}], int) {}

func TestDetachedChildOutlivesItsParentUntilTheLoopStops(t *testing.T) {
	var loop componenttest.Loop
	var stopped []string
	l := &launcher{face: face(t), stop: &stopped}
	ctrl := component.Launch(&loop, l)
	ctrl.Shutdown()
	if !l.child.Alive() || len(stopped) != 0 {
		t.Fatalf("the parent's shutdown reached a detached child: %v", stopped)
	}
	l.child.Send(increment)
	loop.Settle()
	loop.Stop()
	if l.child.Alive() || !slices.Equal(stopped, []string{"detached"}) {
		t.Errorf("loop stop left the detached child alive=%v, stopped=%v", l.child.Alive(), stopped)
	}
}

func TestLoopStopShutsTopLevelComponentsDown(t *testing.T) {
	var loop componenttest.Loop
	var stopped []string
	a := component.Launch(&loop, &counter{face: face(t), stopped: &stopped, name: "a"})
	b := component.Launch(&loop, &counter{face: face(t), stopped: &stopped, name: "b"})
	b.Shutdown()
	loop.Stop()
	if a.Alive() || !slices.Equal(stopped, []string{"b", "a"}) {
		t.Errorf("stopped %v, want b by hand then a at loop stop", stopped)
	}
}

type empty struct{}

func (empty) Init(*component.Context[int, int]) widget.Widget { return nil }
func (empty) Update(*component.Context[int, int], int)        {}

func TestInitWithoutRootPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a component without a root launched")
		}
	}()
	var loop componenttest.Loop
	component.Launch(&loop, empty{})
}

func TestZeroSenderPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a zero Sender sent")
		}
	}()
	var s component.Sender[int]
	s.Send(1)
}

type selfCloser struct{ stopped bool }

func (s *selfCloser) Init(*component.Context[int, struct{}]) widget.Widget {
	return widget.NewSpacer(1, 1)
}
func (s *selfCloser) Update(cx *component.Context[int, struct{}], _ int) { cx.Shutdown() }
func (s *selfCloser) Shutdown(*component.Context[int, struct{}])         { s.stopped = true }

func TestContextShutdownStopsTheComponentFromInside(t *testing.T) {
	var loop componenttest.Loop
	s := &selfCloser{}
	ctrl := component.Launch(&loop, s)
	ctrl.Send(1)
	loop.Settle()
	if !s.stopped || ctrl.Alive() {
		t.Errorf("stopped=%v alive=%v after a self Shutdown", s.stopped, ctrl.Alive())
	}
}
