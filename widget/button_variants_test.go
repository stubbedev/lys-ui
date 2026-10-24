package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget/css"
)

// The icon size squares a button around its image, on any variant and
// with the small and large steps; a labelled button keeps its padding
// (#266).
func TestIconButtonSize(t *testing.T) {
	measure := func(child Widget, classes ...string) Size {
		b := NewButton(child, 0, 0)
		b.AddClass(classes...)
		host := NewBox(Row, 0, 0)
		host.AppendAligned(b, false, AlignStart)
		arrangeTree(t, host, 300, 100)
		r := b.Bounds()
		return Size{W: r.W, H: r.H}
	}
	for _, tc := range []struct {
		classes []string
		want    int
	}{
		{[]string{css.Ghost, css.Icon}, 36},
		{[]string{css.Outline, css.Icon}, 36},
		{[]string{css.Secondary, css.Icon, css.Small}, 32},
		{[]string{css.Ghost, css.Icon, css.Large}, 40},
	} {
		if got := measure(NewSymbol(SymbolClose, 16), tc.classes...); got != (Size{W: tc.want, H: tc.want}) {
			t.Errorf("%v icon button %v, want %dx%d", tc.classes, got, tc.want, tc.want)
		}
	}
	label := NewLabel(testFace(t), 14, "Archive", 0)
	textW := label.Measure(unbounded).W
	if got := measure(label, css.Ghost); got.W != textW+32 {
		t.Errorf("labelled ghost button %v wide, want the label %d plus 16px padding each side", got.W, textW)
	}
}
