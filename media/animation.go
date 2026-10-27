package media

import (
	"fmt"
	"image"
	"io"
	"sort"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

type animationSource struct {
	frames []render.StillImage
	starts []time.Duration
	total  time.Duration
	next   int
}

// NewAnimation plays a decoded GIF or APNG (render.DecodeImage) as a
// seekable source; the image's own loop count is left to the Stream's
// SetLoop.
func NewAnimation(a *render.Animation) (Source, error) {
	if len(a.Frames) == 0 || len(a.Frames) != len(a.Delays) {
		return nil, fmt.Errorf("media: animation has %d frames and %d delays", len(a.Frames), len(a.Delays))
	}
	s := &animationSource{frames: a.Frames, starts: make([]time.Duration, len(a.Frames))}
	for i, d := range a.Delays {
		if d <= 0 {
			return nil, fmt.Errorf("media: animation frame %d has delay %v", i, d)
		}
		s.starts[i] = s.total
		s.total += d
	}
	return s, nil
}

func (s *animationSource) Info() Info {
	b := s.frames[0].Rect
	return Info{Width: b.Dx(), Height: b.Dy(), Duration: s.total}
}

func (s *animationSource) ReadFrame(dst *image.RGBA) (time.Duration, error) {
	if s.next >= len(s.frames) {
		return 0, io.EOF
	}
	src := s.frames[s.next]
	if dst.Rect.Size() != src.Rect.Size() {
		return 0, fmt.Errorf("media: animation frame is %v, the buffer %v", src.Rect.Size(), dst.Rect.Size())
	}
	for y := range src.Rect.Dy() {
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+4*src.Rect.Dx()], src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+y):])
	}
	pts := s.starts[s.next]
	s.next++
	return pts, nil
}

func (s *animationSource) Seek(t time.Duration) error {
	s.next = max(sort.Search(len(s.starts), func(i int) bool { return s.starts[i] > t })-1, 0)
	return nil
}

func (s *animationSource) Close() error { return nil }
