package widget

// Background and mask image layers (css-backgrounds-3 §3): each layer
// sizes its image against a positioning area, positions it there,
// repeats it, and paints clipped to a box. One geometry serves both
// the background stack and the mask.

import (
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// layerBoxes are a box's three layer boxes and their rounded corners.
type layerBoxes struct {
	rect  [3]render.Rect
	radii [3]render.Corners
}

// boxesOf derives the border, padding and content boxes of the border
// box border under v: the borders' widths, and the CSS padding where
// the stylesheet declares it.
func boxesOf(v *style.Values, border render.Rect, radii render.Corners, bw render.Insets) layerBoxes {
	var pad render.Insets
	if v.HasAny(style.PropPaddingTop, style.PropPaddingRight, style.PropPaddingBottom, style.PropPaddingLeft) {
		pad = render.Insets{Top: v.Padding.Top, Right: v.Padding.Right, Bottom: v.Padding.Bottom, Left: v.Padding.Left}
	}
	padding := bw.Shrink(border)
	paddingRadii := insetCorners(radii, bw)
	return layerBoxes{
		rect:  [3]render.Rect{border, padding, pad.Shrink(padding)},
		radii: [3]render.Corners{radii, paddingRadii, insetCorners(paddingRadii, pad)},
	}
}

// layerImage is a layer's resolved image: the gradient, or the decoded
// file with its natural size.
type layerImage struct {
	gradient   *style.Gradient
	grain      float64
	icon       *render.Icon
	natW, natH float64
}

// resolveLayerImage loads src; ok is false for none or a file that
// does not load.
func resolveLayerImage(src style.ImageSource) (layerImage, bool) {
	if src.Gradient != nil {
		return layerImage{gradient: src.Gradient}, true
	}
	if src.Grain > 0 {
		return layerImage{grain: src.Grain}, true
	}
	ic := bgImageFor(src.URL)
	if ic == nil {
		return layerImage{}, false
	}
	w, h := ic.LogicalSize()
	return layerImage{icon: ic, natW: float64(w), natH: float64(h)}, w > 0 && h > 0
}

// layerTile resolves one layer's tiling over its positioning area
// (css-backgrounds-3 §3.6–§3.9): the tile size from the layer size
// (the natural size where auto, the area for a gradient, which has
// none; cover and contain scaling the natural ratio), round rescaling
// the tile to a whole count across the area, the position placing it,
// and space spreading whole tiles with even gaps when two or more fit.
func layerTile(img layerImage, size style.LayerSize, pos [2]style.AxisPos, rep style.LayerRepeat, area render.Rect) render.Tile {
	aw, ah := float64(area.W), float64(area.H)
	w, h := tileSize(img, size, aw, ah)
	roundX, roundY := rep[0] == style.TileRound, rep[1] == style.TileRound
	if roundX {
		w = aw / math.Max(1, math.Round(aw/w))
	}
	if roundY {
		h = ah / math.Max(1, math.Round(ah/h))
	}
	ratio := img.icon != nil
	switch {
	case roundX && !roundY && ratio && size.Kind == style.SizeExplicit && size.H.Auto:
		h = w * img.natH / img.natW
	case roundY && !roundX && ratio && size.Kind == style.SizeExplicit && size.W.Auto:
		w = h * img.natW / img.natH
	}
	t := render.Tile{W: w, H: h}
	t.X, t.RepeatX, t.GapX = tileAxis(rep[0], pos[0], float64(area.X), aw, w)
	t.Y, t.RepeatY, t.GapY = tileAxis(rep[1], pos[1], float64(area.Y), ah, h)
	return t
}

// tileSize is the layer image's size in the positioning area.
func tileSize(img layerImage, size style.LayerSize, aw, ah float64) (float64, float64) {
	natural := img.icon != nil
	switch size.Kind {
	case style.SizeCover, style.SizeContain:
		if !natural {
			return aw, ah
		}
		s := math.Max(aw/img.natW, ah/img.natH)
		if size.Kind == style.SizeContain {
			s = math.Min(aw/img.natW, ah/img.natH)
		}
		return img.natW * s, img.natH * s
	case style.SizeExplicit:
		w, h := size.W.Len.Resolve(aw), size.H.Len.Resolve(ah)
		switch {
		case size.W.Auto && natural:
			w = h * img.natW / img.natH
		case size.W.Auto:
			w = aw
		case size.H.Auto && natural:
			h = w * img.natH / img.natW
		case size.H.Auto:
			h = ah
		}
		return w, h
	}
	if natural {
		return img.natW, img.natH
	}
	return aw, ah
}

// tileAxis places a layer's tiles along one axis: the positioned
// start, whether the tiles repeat, and the gap between them.
func tileAxis(k style.TileKind, pos style.AxisPos, start, area, size float64) (float64, bool, float64) {
	if k == style.TileSpace {
		if n := math.Floor(area / size); n >= 2 {
			return start, true, (area - n*size) / (n - 1)
		}
		k = style.TileNoRepeat
	}
	return start + pos.Within(area-size), k != style.TileNoRepeat, 0
}

// paintLayerImage paints one layer's image over its tiles, clipped to
// the rounded clip box.
func paintLayerImage(cv *render.Canvas, img layerImage, t render.Tile, clip render.Rect, radii render.Corners) {
	if img.icon != nil {
		cv.PaintImageTiles(img.icon.Still(), t, clip, radii)
		return
	}
	if img.grain > 0 {
		cv.PaintGrain(clip, radii, img.grain, int(math.Round(t.X)), int(math.Round(t.Y)))
		return
	}
	var buf [style.MaxResolvedStops]render.GradientStop
	g := cssGradient(img.gradient, render.Rect{W: int(math.Round(t.W)), H: int(math.Round(t.H))}, &buf)
	if t.X == float64(clip.X) && t.Y == float64(clip.Y) && t.W == float64(clip.W) && t.H == float64(clip.H) {
		cv.PaintGradient(clip, radii, g)
		return
	}
	cv.PaintGradientTiles(g, t, clip, radii)
}

// paintBackground paints the box's background (css-backgrounds-3
// §3): the color clipped to the bottom layer's clip box, then every
// image layer from the bottom up, each sized and positioned in its
// origin box — the viewport for a fixed one — and clipped to its clip
// box.
func paintBackground(cv *render.Canvas, v *style.Values, border render.Rect, radii render.Corners, bw render.Insets, bg render.Color) {
	b := &v.Bg
	if bg == 0 && !b.Painted() {
		return
	}
	boxes := boxesOf(v, border, radii, bw)
	if bg != 0 {
		clip := b.Layer(b.Len() - 1).Clip
		cv.RoundedRectCorners(boxes.rect[clip], boxes.radii[clip], bg)
	}
	for i := b.Len() - 1; i >= 0; i-- {
		l := b.Layer(i)
		img, ok := resolveLayerImage(l.Image)
		if !ok {
			continue
		}
		area := boxes.rect[l.Origin]
		if l.Attach == style.AttachFixed {
			area = canvasViewport(cv)
		}
		paintLayerImage(cv, img, layerTile(img, l.Size, l.Pos, l.Repeat, area), boxes.rect[l.Clip], boxes.radii[l.Clip])
	}
}

// canvasViewport is the canvas's extent in logical pixels: the window a
// fixed background positions against.
func canvasViewport(cv *render.Canvas) render.Rect {
	num, den := cv.DeviceScale()
	r := cv.Rect()
	return render.Rect{W: r.W * den / num, H: r.H * den / num}
}
