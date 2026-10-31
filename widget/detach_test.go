package widget

import (
	"maps"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

type detachProbe struct {
	node
	detached int
}

func (p *detachProbe) Measure(Axis, int) Sizes { return FixedSizes(4) }
func (p *detachProbe) HitTest(pt Point) Widget { return p.HitLeaf(p, pt) }
func (p *detachProbe) detach()                 { p.detached++ }
func (p *detachProbe) Paint(*render.Canvas)    {}

func TestDetachReachesChildrenThatAreNotShown(t *testing.T) {
	face := testFace(t)
	probes := map[string]*detachProbe{}
	probe := func(name string) *detachProbe {
		p := &detachProbe{}
		probes[name] = p
		return p
	}
	nb := NewNotebook(face)
	nb.AppendTab("shown", probe("notebook shown page"))
	nb.AppendTab("hidden", probe("notebook unselected page"))
	nb.SelectTab("shown")
	rev := NewRevealer(probe("collapsed revealer"))
	rev.SetRevealed(false)
	st := NewStack().Add("a", probe("stack shown child")).Add("b", probe("stack hidden child"))
	ex := NewExpander(face, "more", probe("closed expander"))
	ex.SetOpen(false)
	tv := NewTabView()
	tv.Append(probe("tab view shown page"), "a")
	tv.Append(probe("tab view unselected page"), "b")
	sh := NewSheet(probe("sheet content"), probe("hidden sheet panel"), SheetBottom)
	root := NewBox(Column, 0, 0).Append(nb, false).Append(rev, false).Append(st, false).
		Append(ex, false).Append(tv, false).Append(sh, false)
	frame(t, root, 400, 600)
	Detach(root)
	for _, name := range slices.Sorted(maps.Keys(probes)) {
		if p := probes[name]; p.detached != 1 {
			t.Errorf("%s: detached %d times on teardown, want once", name, p.detached)
		}
	}

	again := &detachProbe{}
	nb.AppendTab("removed", again)
	nb.CloseTab("removed")
	if again.detached != 1 {
		t.Errorf("a closed unselected notebook page detached %d times, want once", again.detached)
	}
}
