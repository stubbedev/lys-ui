package widget

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/media"
)

func y4mClip(w, h, frames int) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "YUV4MPEG2 W%d H%d F25:1 C420jpeg XCOLORRANGE=LIMITED\n", w, h)
	cw, ch := (w+1)/2, (h+1)/2
	for f := range frames {
		b.WriteString("FRAME\n")
		for y := range h {
			for x := range w {
				b.WriteByte(byte(16 + (x*160/max(w-1, 1)+y*59/max(h-1, 1)+f)%220))
			}
		}
		for y := range ch {
			for x := range cw {
				b.WriteByte(byte(16 + (x+y)*224/max(cw+ch-2, 1)))
			}
		}
		for y := range ch {
			for range cw {
				b.WriteByte(byte(240 - y*224/max(ch-1, 1)))
			}
		}
	}
	return b.Bytes()
}

func y4mStream(t *testing.T, w, h, frames int) *media.Stream {
	t.Helper()
	src, err := media.NewY4M(bytes.NewReader(y4mClip(w, h, frames)))
	if err != nil {
		t.Fatal(err)
	}
	s := media.NewStream(src, "test")
	t.Cleanup(func() {
		s.Close()
		if err := s.Wait(); err != nil {
			t.Error(err)
		}
		animclock.Reset()
	})
	return s
}

func TestGoldenVideoPaintsYUVFramesInTheWindow(t *testing.T) {
	s := y4mStream(t, 32, 16, 1)
	v := NewVideo(goldenFace(t), 13, s)
	pumpUntil(t, func() bool { return s.Frame().RGBA != nil })
	NewGolden(t, v, "video-yuv-window", goldenFrame(96, 64))
}

func TestGoldenVideoRefusedASurfacePaintsAsBefore(t *testing.T) {
	SetVideoHost(func(*Video, VideoFormat, func()) (VideoSurface, error) {
		return nil, errors.New("no YUV formats")
	})
	t.Cleanup(func() { SetVideoHost(nil) })
	s := y4mStream(t, 32, 16, 1)
	v := NewVideo(goldenFace(t), 13, s)
	pumpUntil(t, func() bool { return s.Frame().RGBA != nil })
	NewGolden(t, v, "video-yuv-window", goldenFrame(96, 64))
	if v.OffloadErr() == nil {
		t.Error("the refusal left no reason")
	}
}
