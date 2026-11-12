package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

func TestAWidgetsAnimationWaitsForAHostedTreeAndRunsOnItsClock(t *testing.T) {
	c := pinAnimClock(t)
	root := NewBox(Column, 0, 0)
	c.host(t, root)
	late := NewBox(Row, 0, 0)
	root.Append(late, false)
	landed := 0.0
	anim.Play(late, anim.Animate(100*time.Millisecond, func(p float64) { landed = p }).Easing(anim.Linear))
	if c.active() {
		t.Fatal("a widget no hosted tree holds yet launched onto the host's clock")
	}
	SizeWithin(root, Size{W: 100, H: 100})
	root.Arrange(render.Rect{W: 100, H: 100})
	AttachAnimations()
	if !c.active() {
		t.Fatal("the widget's animation did not start once its tree was hosted and laid out")
	}
	c.drive()
	if landed != 1 {
		t.Errorf("the attached animation stands at %v, want it landed", landed)
	}
}

func TestAnUnhostedTreesLaunchNeverRunsAndACancelledOneNeverStarts(t *testing.T) {
	c := pinAnimClock(t)
	root := NewBox(Column, 0, 0)
	leaf := NewBox(Row, 0, 0)
	root.Append(leaf, false)
	unhost := Host(root, &c.clock)
	SizeWithin(root, Size{W: 100, H: 100})
	root.Arrange(render.Rect{W: 100, H: 100})
	unhost()
	waited, cancelled := 0.0, 0
	anim.Start(leaf, time.Second, func(p float64) { waited = p })
	cancel := anim.Start(root, time.Second, func(float64) { cancelled++ })
	cancel()
	AttachAnimations()
	if c.active() {
		t.Fatal("a tree whose host ended launched onto the clock")
	}
	c.host(t, root)
	AttachAnimations()
	c.drive()
	if waited != 1 {
		t.Errorf("the launch waiting on the unhosted tree stands at %v once it was hosted again, want it landed", waited)
	}
	if cancelled != 0 {
		t.Errorf("the launch cancelled while it waited ran %d frames", cancelled)
	}
}

func TestAWidgetUnderANodelessRootRunsOnThatRootsHost(t *testing.T) {
	c := pinAnimClock(t)
	leaf := NewBox(Row, 0, 0)
	root := &nodelessRoot{child: leaf}
	SetParents(root, leaf)
	c.host(t, root)
	anim.Start(leaf, time.Second, func(float64) {})
	if !c.active() {
		t.Error("a widget under a hosted root without a node did not launch onto its clock")
	}
}

type nodelessRoot struct{ child Widget }

func (r *nodelessRoot) Measure(axis Axis, forSize int) Sizes { return r.child.Measure(axis, forSize) }
func (r *nodelessRoot) Arrange(rect render.Rect)             { r.child.Arrange(rect) }
func (r *nodelessRoot) Paint(cv *render.Canvas)              { r.child.Paint(cv) }
func (r *nodelessRoot) HitTest(p Point) Widget               { return r.child.HitTest(p) }
