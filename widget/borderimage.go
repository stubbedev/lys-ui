package widget

// border-image painting (css-backgrounds-3 §6.7): the source cut into
// nine regions by the slices and drawn over the border image area —
// the border box grown by the outset — corners scaled into the
// corners, each edge scaled across its side and stretched, repeated,
// rounded or spaced along it, the middle only under fill.

import (
	"image"
	"math"
	"sync"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// borderImageOutset is the outset's extent per side (top, right,
// bottom, left) in pixels: a number multiplies the side's border width.
func borderImageOutset(bi *style.BorderImage, bw render.Insets) render.Insets {
	widths := [4]int{bw.Top, bw.Right, bw.Bottom, bw.Left}
	var out [4]int
	for i, o := range bi.Outset {
		v := o.V
		if o.Kind == style.ImageLenNumber {
			v *= float64(widths[i])
		}
		out[i] = int(math.Round(v))
	}
	return render.Insets{Top: out[0], Right: out[1], Bottom: out[2], Left: out[3]}
}

// paintBorderImage draws v's border image over the border box border
// with border widths bw. It reports false when the box has no border
// image or its source does not load: the border styles paint then.
func paintBorderImage(cv *render.Canvas, v *style.Values, border render.Rect, bw render.Insets) bool {
	if !v.Has(style.PropBorderImageSource) || v.BorderImage.Source.None() {
		return false
	}
	bi := v.UsedBorderImage()
	area := borderImageOutset(&bi, bw).Grow(border)
	img, ok := borderImageSource(bi.Source, area)
	if !ok {
		return false
	}
	ib := img.Bounds()
	iw, ih := float64(ib.Dx()), float64(ib.Dy())
	// The slices, clamped to the image (§6.2).
	var sl [4]float64
	for i, s := range bi.Slice {
		dim := ih
		if i%2 == 1 {
			dim = iw
		}
		sl[i] = math.Min(s.Resolve(dim), dim)
	}
	// The drawn widths (§6.3), scaled down together when opposite ones
	// overlap.
	aw, ah := float64(area.W), float64(area.H)
	widths := [4]float64{float64(bw.Top), float64(bw.Right), float64(bw.Bottom), float64(bw.Left)}
	var w [4]float64
	for i, l := range bi.Width {
		dim := ah
		if i%2 == 1 {
			dim = aw
		}
		switch l.Kind {
		case style.ImageLenNumber:
			w[i] = l.V * widths[i]
		case style.ImageLenLength:
			w[i] = l.V
		case style.ImageLenPercent:
			w[i] = l.V / 100 * dim
		case style.ImageLenAuto:
			w[i] = sl[i]
		}
	}
	f := 1.0
	if w[1]+w[3] > aw {
		f = aw / (w[1] + w[3])
	}
	if w[0]+w[2] > ah {
		f = math.Min(f, ah/(w[0]+w[2]))
	}
	for i := range w {
		w[i] *= f
	}
	src := [2][4]float64{
		{0, sl[3], iw - sl[1], iw},
		{0, sl[0], ih - sl[2], ih},
	}
	dst := [2][4]float64{
		{float64(area.X), float64(area.X) + w[3], float64(area.X) + aw - w[1], float64(area.X) + aw},
		{float64(area.Y), float64(area.Y) + w[0], float64(area.Y) + ah - w[2], float64(area.Y) + ah},
	}
	// The edge scale factors: an edge scales across its side by the
	// drawn width over the slice, and the middle takes the top's
	// (else the bottom's) horizontally and the left's (else the
	// right's) vertically.
	scale := func(d, s float64) float64 {
		if s <= 0 || d <= 0 {
			return 0
		}
		return d / s
	}
	topS, bottomS := scale(w[0], sl[0]), scale(w[2], sl[2])
	leftS, rightS := scale(w[3], sl[3]), scale(w[1], sl[1])
	midX, midY := cmp0(topS, bottomS), cmp0(leftS, rightS)
	for row := range 3 {
		for col := range 3 {
			if row == 1 && col == 1 && !bi.Fill {
				continue
			}
			sr := image.Rect(ib.Min.X+int(src[0][col]), ib.Min.Y+int(src[1][row]), ib.Min.X+int(src[0][col+1]), ib.Min.Y+int(src[1][row+1]))
			d0x, d1x, d0y, d1y := dst[0][col], dst[0][col+1], dst[1][row], dst[1][row+1]
			if sr.Empty() || d1x <= d0x || d1y <= d0y {
				continue
			}
			// The natural tile size along each axis: corners stretch,
			// an edge keeps its slice's ratio at the side's width.
			tw, th := d1x-d0x, d1y-d0y
			kx, ky := style.TileStretch, style.TileStretch
			if col == 1 {
				kx = bi.Repeat[0]
				switch row {
				case 0:
					tw = float64(sr.Dx()) * topS
				case 2:
					tw = float64(sr.Dx()) * bottomS
				default:
					tw = float64(sr.Dx()) * midX
				}
			}
			if row == 1 {
				ky = bi.Repeat[1]
				switch col {
				case 0:
					th = float64(sr.Dy()) * leftS
				case 2:
					th = float64(sr.Dy()) * rightS
				default:
					th = float64(sr.Dy()) * midY
				}
			}
			x, sx, gx, rx, okx := borderTileAxis(kx, d0x, d1x-d0x, tw)
			y, sy, gy, ry, oky := borderTileAxis(ky, d0y, d1y-d0y, th)
			if !okx || !oky {
				continue
			}
			clip := render.Rect{X: int(math.Round(d0x)), Y: int(math.Round(d0y))}
			clip.W, clip.H = int(math.Round(d1x))-clip.X, int(math.Round(d1y))-clip.Y
			t := render.Tile{X: x, Y: y, W: sx, H: sy, GapX: gx, GapY: gy, RepeatX: rx, RepeatY: ry}
			cv.PaintImageTiles(img.SubImage(sr), t, clip, render.Corners{})
		}
	}
	return true
}

// cmp0 is a, or b where a is zero.
func cmp0(a, b float64) float64 {
	if a > 0 {
		return a
	}
	return b
}

// borderTileAxis lays one region's tiles along an axis of length
// length from start (§6.7): stretch fills it, repeat centers whole
// tiles of the natural size, round rescales them to a whole count,
// space spreads whole tiles with even gaps around them. ok is false
// when no tile is drawn: a natural size of zero, or space with no room
// for one.
func borderTileAxis(k style.TileKind, start, length, natural float64) (pos, size, gap float64, repeat, ok bool) {
	if k == style.TileStretch {
		return start, length, 0, false, true
	}
	if natural <= 0 {
		return 0, 0, 0, false, false
	}
	switch k {
	case style.TileRound:
		size = length / math.Max(1, math.Round(length/natural))
		return start, size, 0, true, true
	case style.TileSpace:
		n := math.Floor(length / natural)
		if n < 1 {
			return 0, 0, 0, false, false
		}
		gap = (length - n*natural) / (n + 1)
		return start + gap, natural, gap, true, true
	}
	return start + (length-natural)/2, natural, 0, true, true
}

// borderImageSource is the border image's pixels: a file's at its
// natural size, a gradient's rendered over the border image area.
func borderImageSource(src style.ImageSource, area render.Rect) (render.StillImage, bool) {
	if src.Gradient == nil {
		ic := bgImageFor(src.URL)
		if ic == nil {
			return render.StillImage{}, false
		}
		return render.StillImage{RGBA: ic.Image()}, true
	}
	if area.Empty() {
		return render.StillImage{}, false
	}
	return gradientImages.get(src.Gradient, area.W, area.H), true
}

// gradientImageKey names one rendered gradient image.
type gradientImageKey struct {
	g    *style.Gradient
	w, h int
}

// gradientImageCache keeps the gradients border images rendered, so a
// steady frame re-renders none; it forgets everything when full.
type gradientImageCache struct {
	mu   sync.Mutex
	imgs map[gradientImageKey]render.StillImage
}

// gradientImageCap bounds the cache.
const gradientImageCap = 64

var gradientImages gradientImageCache

// get returns g rendered at w x h, rendering it on a miss.
func (c *gradientImageCache) get(g *style.Gradient, w, h int) render.StillImage {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := gradientImageKey{g, w, h}
	if img, ok := c.imgs[k]; ok {
		return img
	}
	if c.imgs == nil || len(c.imgs) >= gradientImageCap {
		c.imgs = map[gradientImageKey]render.StillImage{}
	}
	stride := render.Stride(w)
	data := make([]byte, stride*h)
	var buf [style.MaxResolvedStops]render.GradientStop
	box := render.Rect{W: w, H: h}
	render.New(data, stride, w, h).PaintGradient(box, render.Corners{}, cssGradient(g, box, &buf))
	img := render.Still(render.NRGBA(data, stride, w, h))
	c.imgs[k] = img
	return img
}
