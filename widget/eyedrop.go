package widget

import (
	"image"

	"github.com/stubbedev/lys-lib/render"
)

// ScreenPick displays one captured screen and reports the pixel color
// under clicks - the eyedropper's view (#84). The capture arrives as a
// plain image (the app's capture pipeline grabbed it off the loop),
// paints scaled to fit, and a click maps back to the exact source
// pixel, so colors are picked from what the screen really showed. The
// cursor is a crosshair; the picked color lands in OnPick exactly once
// per click.
type ScreenPick struct {
	node
	img image.Image
	// fit is the drawn rect inside the last arranged bounds, in
	// logical pixels.
	fit render.Rect
	// scaled is the image resampled for the last fit, from source rect
	// scaledSrc; a new fit resamples into a new raster.
	scaled    render.StillImage
	scaledSrc image.Rectangle
	// OnPick receives the clicked pixel's color.
	OnPick func(render.Color)
}

// NewScreenPick returns a pick view over img, handed over read-only.
func NewScreenPick(img image.Image) *ScreenPick {
	s := &ScreenPick{img: heldImage(img)}
	s.cursorName = "crosshair"
	return s
}

// Measure is the image's pictureRequest: the dialog scales it down to
// fit.
func (s *ScreenPick) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := s.measureHit(axis, forSize); ok {
		return sz
	}
	b := s.img.Bounds()
	return s.measureStore(axis, forSize, pictureRequest(Size{W: b.Dx(), H: b.Dy()}, axis, forSize))
}

// Arrange records the rect; the drawn fit is computed in Paint from
// the mapped device rect, the same math every image draw uses.
func (s *ScreenPick) Arrange(r render.Rect) { s.node.Arrange(r) }

// Paint draws the image scaled to fit, letterboxed inside the bounds.
func (s *ScreenPick) Paint(cv *render.Canvas) {
	b := s.img.Bounds()
	box := cv.MapRect(s.bounds)
	src, dst := render.PlaceImage(b.Dx(), b.Dy(), box.W, box.H, ImageFit, render.ImageCentered)
	if dst.Empty() {
		return
	}
	dw, dh := dst.W, dst.H
	s.fit = render.Rect{X: box.X + dst.X, Y: box.Y + dst.Y, W: dw, H: dh}
	if sub, ok := s.img.(interface {
		SubImage(r image.Rectangle) image.Image
	}); ok && src.Dx() == dw && src.Dy() == dh {
		cv.DrawImageDevice(sub.SubImage(src), s.fit.X, s.fit.Y)
		return
	}
	if s.scaled.RGBA == nil || s.scaledSrc != src || s.scaled.Rect.Dx() != dw || s.scaled.Rect.Dy() != dh {
		s.scaled, s.scaledSrc = render.NewStillImage(render.Resample(s.img, src, dw, dh)), src
	}
	cv.DrawImageDevice(s.scaled, s.fit.X, s.fit.Y)
}

// HitTest resolves inside the bounds.
func (s *ScreenPick) HitTest(p Point) Widget {
	if s.bounds.Contains(p.X, p.Y) {
		return s
	}
	return nil
}

// ClickAt picks the source pixel under the point and hands its color
// to OnPick. Points in the letterbox clamp to the nearest edge pixel.
func (s *ScreenPick) ClickAt(p Point) {
	b := s.img.Bounds()
	x := b.Min.X + (p.X-s.fit.X)*b.Dx()/max(1, s.fit.W)
	y := b.Min.Y + (p.Y-s.fit.Y)*b.Dy()/max(1, s.fit.H)
	x = min(max(x, b.Min.X), b.Max.X-1)
	y = min(max(y, b.Min.Y), b.Max.Y-1)
	r, g, bl, a := s.img.At(x, y).RGBA()
	col := render.RGBA(uint8(r>>8&0xff), uint8(g>>8&0xff), uint8(bl>>8&0xff), uint8(a>>8&0xff))
	if s.OnPick != nil {
		s.OnPick(col)
	}
}
