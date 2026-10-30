package app

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/widget"
)

type presentedHarness struct {
	*paintHarness
	progress *widget.ProgressBar
	fired    int
}

func newPresentedHarness(presentation bool) *presentedHarness {
	progress := widget.NewProgressBar(0)
	root := widget.NewBox(widget.Column, 8, 8)
	root.Append(progress, false)
	h := &presentedHarness{paintHarness: newPaintHarness(root, 200, 80), progress: progress}
	h.surf.presentation = presentation
	h.wnd.cfg.onPresented = func() { h.fired++ }
	return h
}

func (h *presentedHarness) commit() {
	h.progress.SetValue(h.progress.Value() + 0.1)
	h.frame()
}

func TestOnPresentedFiresOnceWhenPresented(t *testing.T) {
	h := newPresentedHarness(true)
	h.commit()
	if len(h.surf.feedback) != 1 || !slices.Equal(h.surf.calls[len(h.surf.calls)-2:], []string{"feedback", "commit"}) {
		t.Fatalf("feedback %d, calls %v, want one feedback armed before the commit", len(h.surf.feedback), h.surf.calls)
	}
	h.surf.feedback[0](true)
	h.commit()
	h.surf.feedback[0](true)
	if h.fired != 1 || len(h.surf.feedback) != 1 {
		t.Errorf("fired %d, feedback %d, want one call and no feedback once presented", h.fired, len(h.surf.feedback))
	}
}

func TestOnPresentedRetriesAfterDiscard(t *testing.T) {
	h := newPresentedHarness(true)
	h.commit()
	h.surf.feedback[0](false)
	if h.fired != 0 {
		t.Fatalf("fired %d on a discarded buffer", h.fired)
	}
	h.commit()
	if len(h.surf.feedback) != 2 {
		t.Fatalf("feedback %d, want the next commit to arm another", len(h.surf.feedback))
	}
	h.surf.feedback[1](true)
	if h.fired != 1 {
		t.Errorf("fired %d, want 1 once the retried commit was presented", h.fired)
	}
}

func TestOnPresentedFallsBackToFrameCallback(t *testing.T) {
	h := newPresentedHarness(false)
	h.commit()
	if h.fired != 0 {
		t.Fatalf("fired %d at the commit, want it to wait for the frame callback", h.fired)
	}
	h.wnd.takeFrame()
	h.commit()
	h.wnd.takeFrame()
	if h.fired != 1 || len(h.surf.feedback) != 0 {
		t.Errorf("fired %d, feedback %d, want one call from the frame callback", h.fired, len(h.surf.feedback))
	}
}

func TestLayerOnPresentedDoesNotFireAtCommit(t *testing.T) {
	for _, presentation := range []bool{true, false} {
		h := newPresentedHarness(presentation)
		h.commit()
		if h.surf.commits != 1 || h.fired != 0 {
			t.Errorf("presentation %v: commits %d, fired %d, want the commit without the callback", presentation, h.surf.commits, h.fired)
		}
	}
}
