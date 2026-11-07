package widget

import (
	"errors"
	"image"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/media"
)

type fakeVideoSurface struct {
	format    VideoFormat
	wake      func()
	dst       render.Rect
	src       image.Rectangle
	presented []uint64
	stream    *media.Stream
	drop      bool
	fail      error
	closed    bool
}

func (f *fakeVideoSurface) Place(dst render.Rect, src image.Rectangle) { f.dst, f.src = dst, src }

func (f *fakeVideoSurface) Present(fr *media.YUVFrame) (bool, error) {
	if f.fail != nil {
		return false, f.fail
	}
	if f.drop || f.dst.Empty() || fr.Width != f.format.Width || fr.Height != f.format.Height {
		return false, nil
	}
	f.presented = append(f.presented, f.stream.FrameSeq())
	return true, nil
}

func (f *fakeVideoSurface) Close() { f.closed = true }

func withVideoHost(t *testing.T, host VideoHost) {
	t.Helper()
	SetVideoHost(host)
	t.Cleanup(func() { SetVideoHost(nil) })
}

func offloadedVideo(t *testing.T) (*Video, *fakeVideoSurface) {
	t.Helper()
	t.Cleanup(anim.SetInstant(true))
	s := y4mStream(t, 32, 16, 40)
	fake := &fakeVideoSurface{stream: s}
	withVideoHost(t, func(v *Video, f VideoFormat, wake func()) (VideoSurface, error) {
		fake.format, fake.wake = f, wake
		return fake, nil
	})
	v := NewVideo(goldenFace(t), 13, s)
	pumpUntil(t, func() bool { return s.FrameYUV() != nil })
	SizeWithin(v, Size{W: 80, H: 80})
	v.Arrange(render.Rect{X: 10, Y: 20, W: 80, H: 80})
	return v, fake
}

func TestAVideoOnASurfacePresentsEachFrameWithoutRepaintingTheWindow(t *testing.T) {
	v, fake := offloadedVideo(t)
	s := v.Stream()
	if err := v.OffloadErr(); err != nil {
		t.Fatal(err)
	}
	if fake.format != (VideoFormat{Width: 32, Height: 16, Color: media.YUVColor{Matrix: media.BT601, Range: media.LimitedRange}}) {
		t.Errorf("surface for %+v", fake.format)
	}
	if want := (render.Rect{X: 10, Y: 40, W: 80, H: 40}); fake.dst != want || fake.src != image.Rect(0, 0, 32, 16) {
		t.Errorf("placed %v from %v, want %v from the whole frame", fake.dst, fake.src, want)
	}
	if len(fake.presented) != 1 {
		t.Fatalf("%d frames presented on landing, want the first", len(fake.presented))
	}

	const w, h = 100, 120
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	cv.Clear(cv.Rect(), render.RGB(1, 2, 3))
	v.view.Paint(cv)
	if got := pixel(data, render.Stride(w), 50, 60); got != 0 {
		t.Errorf("the frame's rect painted %08x, want the hole the surface shows through", uint32(got))
	}
	if got := pixel(data, render.Stride(w), 50, 25); got != render.RGB(1, 2, 3) {
		t.Errorf("the letterbox painted %08x, want it left alone", uint32(got))
	}

	s.Play()
	pumpUntil(t, func() bool { return !v.reveal.Revealed() })
	v.view.invalid = false
	presented := len(fake.presented)
	seq := s.FrameSeq()
	pumpUntil(t, func() bool { return s.FrameSeq() >= seq+5 })
	if len(fake.presented) < presented+5 {
		t.Errorf("%d frames presented while %d were shown", len(fake.presented)-presented, s.FrameSeq()-seq)
	}
	if v.view.invalid {
		t.Error("an offloaded frame invalidated the video's picture")
	}
}

func TestAVideoRetriesADroppedFrameWhenABufferFrees(t *testing.T) {
	v, fake := offloadedVideo(t)
	fake.drop = true
	v.view.shown = 0
	v.view.place()
	if len(fake.presented) != 1 {
		t.Fatalf("a dropped frame was counted as presented: %v", fake.presented)
	}
	fake.drop = false
	fake.wake()
	if len(fake.presented) != 2 || fake.presented[1] != v.Stream().FrameSeq() {
		t.Errorf("waking after the drop presented %v, want the current frame again", fake.presented)
	}
	fake.wake()
	if len(fake.presented) != 2 {
		t.Error("a wake with the frame already shown presented it again")
	}
}

func TestAFailingVideoSurfaceFallsBackToPaintingTheFrames(t *testing.T) {
	v, fake := offloadedVideo(t)
	boom := errors.New("buffer rejected")
	fake.fail = boom
	v.view.shown = 0
	v.view.place()
	if !errors.Is(v.OffloadErr(), boom) || !fake.closed || v.view.offload != nil {
		t.Fatalf("after a failed present: err %v closed %v", v.OffloadErr(), fake.closed)
	}
	const w, h = 100, 120
	data := make([]byte, render.Stride(w)*h)
	v.view.Paint(render.New(data, render.Stride(w), w, h))
	if got := pixel(data, render.Stride(w), 50, 60); got == 0 {
		t.Error("the fallen-back video painted no frame")
	}
}

func TestDetachingAVideoClosesItsSurfaceAndShowingItReopensOne(t *testing.T) {
	v, fake := offloadedVideo(t)
	Detach(v)
	if !fake.closed || v.view.offload != nil || v.OffloadErr() != nil {
		t.Fatalf("detached: closed %v offload %v err %v", fake.closed, v.view.offload, v.OffloadErr())
	}
	fake.closed = false
	v.Arrange(render.Rect{X: 10, Y: 20, W: 80, H: 40})
	if v.view.offload == nil {
		t.Error("the video shown again has no surface")
	}
}

func TestAStreamWithoutYUVFramesPaintsIntoTheWindow(t *testing.T) {
	called := false
	withVideoHost(t, func(*Video, VideoFormat, func()) (VideoSurface, error) {
		called = true
		return nil, nil
	})
	s := testStream(t, 2)
	v := NewVideo(testFace(t), 13, s)
	SizeWithin(v, Size{W: 80, H: 80})
	v.Arrange(render.Rect{W: 80, H: 80})
	if called || !errors.Is(v.OffloadErr(), errRGBAStream) {
		t.Errorf("an RGBA stream: host called %v, err %v", called, v.OffloadErr())
	}
}

func TestAVideoWithNoRoomHidesItsSurfaceAndShowsTheFrameAgain(t *testing.T) {
	v, fake := offloadedVideo(t)
	v.Arrange(render.Rect{X: 10, Y: 20})
	if !fake.dst.Empty() {
		t.Fatalf("a video with no room keeps its surface at %v", fake.dst)
	}
	before := len(fake.presented)
	v.Arrange(render.Rect{X: 10, Y: 20, W: 80, H: 40})
	if fake.dst.Empty() || len(fake.presented) != before+1 {
		t.Errorf("shown again at %v after %d presents, want the frame presented again", fake.dst, len(fake.presented)-before)
	}
}
