package widget

import (
	"runtime"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// Widgets with a subtree effect share pooled scratch layers: the first
// paint at a new canvas size costs one layer buffer for twenty
// translucent buttons, where each kept a canvas-sized buffer of its own
// and reallocated it on every resize; a smaller size costs none.
func TestSubtreeEffectsShareScratchLayers(t *testing.T) {
	loadCSS(t, `.faded { opacity: 0.5; }`)
	col := NewBox(Column, 4, 4)
	for range 20 {
		b := NewButton(NewLabel(goldenFace(t), 14, "Reset", 0), 8, 4)
		b.AddClass("faded")
		col.Append(b, false)
	}
	paintAt := func(w, h int) uint64 {
		cv := render.New(make([]byte, render.Stride(w)*h), render.Stride(w), w, h)
		arrangeTree(t, col, w, h)
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		col.Paint(cv)
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	paintAt(400, 900)
	const w, h = 420, 900
	if got, layer := paintAt(w, h), uint64(render.Stride(w)*h); got >= 2*layer {
		t.Errorf("the first paint at a new size allocated %d bytes, %d layers' worth; want one", got, got/layer)
	}
	if got, layer := paintAt(390, 880), uint64(render.Stride(390)*880); got >= layer {
		t.Errorf("a paint at a smaller size allocated %d bytes, want the pooled layer reused", got)
	}
}
