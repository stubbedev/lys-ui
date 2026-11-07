package media

import (
	"fmt"
	"image"
	"time"
)

// YUVMatrix is the matrix a YUV frame's samples were encoded with.
type YUVMatrix uint8

// BT601 is the matrix every built-in source decodes to.
const BT601 YUVMatrix = iota

// YUVRange is the span a YUV frame's samples cover.
type YUVRange uint8

// The ranges: limited (16 to 235 for luma) and full (0 to 255).
const (
	LimitedRange YUVRange = iota
	FullRange
)

// YUVColor is how a YUV frame's samples become RGB.
type YUVColor struct {
	Matrix YUVMatrix
	Range  YUVRange
}

// YUVFrame is a frame as 8-bit 4:2:0 planes (I420): Pix holds the Y
// plane, Width by Height samples, then the U and the V plane, each
// ChromaSize, every row packed, so a plane's stride is its width.
type YUVFrame struct {
	Width, Height int
	Color         YUVColor
	Pix           []byte
}

// YUVFrameBytes is the size of a width x height YUVFrame's Pix:
// width x height x 1.5 for even sizes.
func YUVFrameBytes(width, height int) int {
	cw, ch := chroma420(width, height)
	return width*height + 2*cw*ch
}

func chroma420(width, height int) (cw, ch int) { return (width + 1) / 2, (height + 1) / 2 }

// ChromaSize is the size of the U and V planes: half the frame's,
// rounded up.
func (f *YUVFrame) ChromaSize() (w, h int) { return chroma420(f.Width, f.Height) }

// Planes returns the Y, U and V planes, slices of Pix.
func (f *YUVFrame) Planes() (y, u, v []byte) {
	cw, ch := f.ChromaSize()
	n := f.Width * f.Height
	return f.Pix[:n], f.Pix[n : n+cw*ch], f.Pix[n+cw*ch : n+2*cw*ch]
}

func (f *YUVFrame) layout() planeLayout {
	return planeLayout{width: f.Width, height: f.Height, xs: 1, ys: 1, full: f.Color.Range == FullRange}
}

func (f *YUVFrame) check(width, height int) error {
	if f.Width != width || f.Height != height || len(f.Pix) != YUVFrameBytes(width, height) {
		return fmt.Errorf("media: a %dx%d YUV frame does not fit a %dx%d buffer of %d bytes", width, height, f.Width, f.Height, len(f.Pix))
	}
	return nil
}

func (f *YUVFrame) toRGBA(dst *image.RGBA) { yuvToRGBA(dst, f.Pix, f.layout()) }

// YUVSource is a Source that decodes YUV: ReadFrameYUV takes the place
// of ReadFrame for a viewer that shows the planes as they are, on a
// YUV dmabuf, with no conversion to RGBA.
type YUVSource interface {
	// YUV reports the colour of the frames ReadFrameYUV decodes, false
	// when the source's frames are not 4:2:0 and it decodes none.
	YUV() (YUVColor, bool)
	// ReadFrameYUV decodes the next frame into dst, of Info's size,
	// and returns its presentation time. It returns io.EOF after the
	// last frame.
	ReadFrameYUV(dst *YUVFrame) (time.Duration, error)
}

func (m muted) YUV() (YUVColor, bool) {
	y, ok := m.Source.(YUVSource)
	if !ok {
		return YUVColor{}, false
	}
	return y.YUV()
}

func (m muted) ReadFrameYUV(dst *YUVFrame) (time.Duration, error) {
	y, ok := m.Source.(YUVSource)
	if !ok {
		return 0, errNoYUV
	}
	return y.ReadFrameYUV(dst)
}

// yuvOf is src as a YUVSource, nil when it decodes no YUV.
func yuvOf(src Source) YUVSource {
	y, ok := src.(YUVSource)
	if !ok {
		return nil
	}
	if _, ok := y.YUV(); !ok {
		return nil
	}
	return y
}
