package widget

import "testing"

// Every symbol kind draws ink: a kind the switch misses paints an empty
// box, as an icon slot showing nothing.
func TestEverySymbolPaints(t *testing.T) {
	for kind := SymbolChevronRight; kind <= SymbolFolder; kind++ {
		px := paintPixels(t, NewSymbol(kind, 24), 24, 24)
		inked := false
		for i := 3; i < len(px) && !inked; i += 4 {
			inked = px[i] != 0
		}
		if !inked {
			t.Errorf("symbol %d paints nothing", kind)
		}
	}
}
