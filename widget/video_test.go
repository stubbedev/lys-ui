package widget

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/media"
)

func halves(left, right color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := range 2 {
		for x := range 4 {
			c := left
			if x >= 2 {
				c = right
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

var (
	red  = color.RGBA{255, 0, 0, 255}
	blue = color.RGBA{0, 0, 255, 255}
)

func testStream(t *testing.T, frames int) *media.Stream {
	t.Helper()
	a := &render.Animation{}
	for range frames {
		a.Frames = append(a.Frames, render.NewStillImage(halves(red, blue)))
		a.Delays = append(a.Delays, 100*time.Millisecond)
	}
	src, err := media.NewAnimation(a)
	if err != nil {
		t.Fatal(err)
	}
	s := media.NewStream(src, "test")
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
		animclock.Reset()
	})
	return s
}

func pumpUntil(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("the stream never got there")
		}
		animclock.Tick(time.Now())
		time.Sleep(time.Millisecond)
	}
}

func pixel(data []byte, stride, x, y int) render.Color {
	return render.ColorFromBytes(data[y*stride+4*x:])
}

func TestVideoFitsTheFrameIntoItsBounds(t *testing.T) {
	s := testStream(t, 1)
	v := NewVideo(testFace(t), 13, s)
	pumpUntil(t, func() bool { return s.Frame().RGBA != nil })
	if got := v.Measure(Constraints{Max: Size{W: 400, H: 400}}); got.W < 4 || got.H < 2 {
		t.Errorf("natural size %v, want at least the 4x2 frame", got)
	}
	const w, h = 80, 80
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	v.Measure(Constraints{Max: Size{W: w, H: h}})
	v.Arrange(render.Rect{W: w, H: h})
	v.view.Paint(cv)
	if got := pixel(data, render.Stride(w), 10, 40); got != render.RGB(255, 0, 0) {
		t.Errorf("left half %08x, want red", uint32(got))
	}
	if got := pixel(data, render.Stride(w), 70, 40); got != render.RGB(0, 0, 255) {
		t.Errorf("right half %08x, want blue", uint32(got))
	}
	if got := pixel(data, render.Stride(w), 40, 5); got != 0 {
		t.Errorf("the letterbox above a 2:1 frame is painted: %08x", uint32(got))
	}
	v.SetScale(ImageCover)
	clear(data)
	v.view.Paint(cv)
	if got := pixel(data, render.Stride(w), 40, 5); got == 0 {
		t.Error("ImageCover left the top uncovered")
	}
}

func TestVideoShowsItsControlsWhilePausedOrHovered(t *testing.T) {
	s := testStream(t, 20)
	v := NewVideo(testFace(t), 13, s)
	v.Measure(Constraints{Max: Size{W: 300, H: 200}})
	if !v.reveal.Revealed() {
		t.Error("the controls of a paused video are hidden")
	}
	v.SetAutoplay(true)
	if !s.Playing() || v.reveal.Revealed() {
		t.Errorf("autoplay: playing %v, controls revealed %v", s.Playing(), v.reveal.Revealed())
	}
	setHoverChain(nil, v.view)
	if !v.reveal.Revealed() {
		t.Error("hovering a playing video kept its controls hidden")
	}
	setHoverChain(v.view, nil)
	if v.reveal.Revealed() {
		t.Error("the controls stayed after the pointer left")
	}
	v.SetStream(nil)
	if v.reveal.Revealed() || v.Stream() != nil {
		t.Error("a video without a stream shows controls")
	}
}

func TestMediaControlsDriveTheStream(t *testing.T) {
	s := testStream(t, 20)
	m := NewMediaControls(testFace(t), 13, s)
	m.Measure(Constraints{Max: Size{W: 600, H: 100}})
	pumpUntil(t, func() bool { return s.Frame().RGBA != nil })
	if IsVisible(m.mute) || IsVisible(m.volume) {
		t.Error("a silent stream shows sound controls")
	}
	if got := m.clock.Text(); got != "0:00 / 0:02" {
		t.Errorf("clock %q", got)
	}
	m.play.OnClick()
	if !s.Playing() || m.playIcon.Name() != "media-playback-pause-symbolic" {
		t.Errorf("play: playing %v icon %q", s.Playing(), m.playIcon.Name())
	}
	m.play.OnClick()
	if s.Playing() || m.playIcon.Name() != "media-playback-start-symbolic" {
		t.Error("pause did not pause")
	}
	m.seek.SetValue(1.5)
	if s.Position() != 1500*time.Millisecond {
		t.Errorf("seek bar moved to 1.5 left the stream at %v", s.Position())
	}
	pumpUntil(t, func() bool { return m.seek.Value() == 1.5 && m.clock.Text() == "0:01 / 0:02" })
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if IsEnabled(m.play) {
		t.Error("controls of a closed stream stay live")
	}
}

func TestClockText(t *testing.T) {
	for _, c := range []struct {
		pos, dur time.Duration
		want     string
	}{
		{65 * time.Second, 200 * time.Second, "1:05 / 3:20"},
		{65 * time.Second, 2 * time.Hour, "0:01:05 / 2:00:00"},
		{7 * time.Second, 0, "0:07"},
		{-time.Second, time.Minute, "0:00 / 1:00"},
	} {
		if got := clockText(c.pos, c.dur); got != c.want {
			t.Errorf("clockText(%v, %v) = %q, want %q", c.pos, c.dur, got, c.want)
		}
	}
}
