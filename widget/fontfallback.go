package widget

import (
	"sync/atomic"

	"github.com/stubbedev/lys-lib/render"
)

var (
	faceEpoch      atomic.Uint64
	faceAnswerOwed atomic.Pointer[loopBridge]
)

// FontFaceResolved tells the widgets a fallback chain has a new face: text
// shaped before it shapes again, and the windows lay out and paint on the
// loop's next pass. It is a render.Chain's onResolved, safe on any goroutine.
// Answers that land before the loop has taken the last one share one relayout.
// Without a loop, only text painted from here on is shaped anew.
func FontFaceResolved() {
	p := invoker.Load()
	if p == nil {
		faceEpoch.Add(1)
		return
	}
	if faceAnswerOwed.Swap(p) == p {
		return
	}
	p.deliver(func() {
		faceAnswerOwed.CompareAndSwap(p, nil)
		faceEpoch.Add(1)
		bumpStyleGen("font fallback")
	})
}

func settleFaces(face render.Font, s string) {
	c, ok := face.(*render.Chain)
	if !ok || hasInvoker() {
		return
	}
	for _, r := range s {
		c.Prewarm(r)
	}
}
