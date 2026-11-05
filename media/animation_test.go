package media

import (
	"image"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

func tenFrameAnimation(t *testing.T) *render.Animation {
	t.Helper()
	a := &render.Animation{}
	for i := range 10 {
		rgba := image.NewRGBA(image.Rect(0, 0, 2, 2))
		rgba.Pix[0] = byte(i)
		a.Frames = append(a.Frames, render.NewStillImage(rgba))
		a.Delays = append(a.Delays, fakeStep)
	}
	return a
}

func TestALoopingAnimationShowsItsOwnFramesAndCopiesNone(t *testing.T) {
	a := tenFrameAnimation(t)
	src, err := NewAnimation(a)
	if err != nil {
		t.Fatal(err)
	}
	play(t, src, noAudio, func(h *harness) {
		h.s.SetLoop(true)
		h.s.Play()
		h.step(0)
		seen := map[render.ImageID]int{}
		last := h.s.FrameSeq()
		for range 25 {
			h.step(fakeStep)
			frame := h.s.Frame()
			if frame.RGBA == nil {
				t.Fatal("no frame")
			}
			i := int(frame.Pix[0])
			if frame.RGBA != a.Frames[i].RGBA || frame.ID != a.Frames[i].ID || frame.Version != 0 {
				t.Fatalf("frame %d shows raster %p id %v version %d, want the animation's own %p id %v at version 0", i, frame.RGBA, frame.ID, frame.Version, a.Frames[i].RGBA, a.Frames[i].ID)
			}
			if seq := h.s.FrameSeq(); seq != last {
				seen[frame.ID]++
				last = seq
			}
		}
		if len(seen) != len(a.Frames) {
			t.Errorf("played %d distinct frames over two loops, want all %d", len(seen), len(a.Frames))
		}
		for id, n := range seen {
			if n < 2 {
				t.Errorf("frame %v showed %d times over two loops, want it shown again on the second", id, n)
			}
		}
	})
}

func TestABufferedSourceStillVersionsItsPooledFrames(t *testing.T) {
	play(t, seekableFake{&fakeSource{frames: 5, failAt: -1}}, noAudio, func(h *harness) {
		h.s.Play()
		h.step(0)
		first := h.s.Frame()
		h.step(fakeStep + time.Millisecond)
		if next := h.s.Frame(); next.Version == first.Version || next.Version != h.s.FrameSeq() {
			t.Errorf("a pooled frame moved from version %d to %d (seq %d), want the version to follow the frame", first.Version, next.Version, h.s.FrameSeq())
		}
	})
}
