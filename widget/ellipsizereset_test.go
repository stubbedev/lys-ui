package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A GTK-style `* { all: unset; }` reset keeps a label's programmatic
// ellipsizing: in a never-policy list the label gives up its width.
func TestAllUnsetKeepsALabelsEllipsize(t *testing.T) {
	loadCSS(t, `* { all: unset; }`)
	l := NewLabel(goldenFace(t), 14, "a fairly long network name that should ellipsize", render.RGB(255, 255, 255))
	l.SetEllipsize(EllipsizeEnd)
	nat := SizeWithin(l, Size{W: 2000, H: 100})
	if got := widthGive(l); got <= nat.W/2 {
		t.Errorf("ShrinkableWidth = %d of %d: the reset turned the ellipsis off", got, nat.W)
	}
}
