package widget

import (
	"math"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
)

// SetSharedElement tags the widget as one end of a shared-element
// transition. A tag names one widget shown at a time: when a widget
// paints after another widget with its tag was shown in its stead - a
// stack page switch, a card opening into its detail view - it morphs
// out of the other: its bounds and corner radius travel from the
// other's to its own on a spring while the other's last rendering
// fades out and its own fades in, both scaled into the moving rounded
// shape, so fill, border and content blend across. Reduced motion
// cross-fades in place. An empty tag removes it.
func (n *node) SetSharedElement(tag string) {
	n.sharedTag = tag
	n.Invalidate()
}

// SharedElement returns the widget's shared-element tag.
func (n *node) SharedElement() string { return n.sharedTag }

// sharedShot is a tagged widget's last rendering: where it was shown,
// its corner radii and its pixels, and the surface it showed on with
// that surface's origin (placed, when the host knew it).
type sharedShot struct {
	owner  *node
	root   Widget
	origin Point
	placed bool
	rect   render.Rect
	radii  render.Corners
	layer  *render.Layer
	// dev is the layer's device rect holding the rendering.
	dev render.Rect
}

// sharedShots holds each tag's last rendering. Tags are few - a handful
// of hero elements - so the map stays small.
var sharedShots = map[string]*sharedShot{}

// sharedTransition is a running shared-element morph into a widget.
type sharedTransition struct {
	from     sharedShot
	progress float64
	spring   *anim.Spring
	fade     anim.Cancel
	geometry bool
	mix      *render.Layer
}

// paintShared paints a tagged widget: through its layer, recording the
// rendering for the tag, and through a running morph when it has
// just appeared after another widget with the tag.
func (n *node) paintShared(cv *render.Canvas, w Widget) {
	shot := sharedShots[n.sharedTag]
	root := RootOf(w)
	// Another widget's rendering under the tag means it was shown since
	// this one last painted: this one takes over from it, out of the
	// other's rect in this surface's coordinates. A morph across
	// surfaces needs both placed; without that the surface's own reveal
	// (#200's anchor morph) stands in.
	if shot != nil && shot.owner != n && shot.layer != nil {
		if from, ok := shotIn(*shot, root); ok {
			n.startShared(from)
		}
	}
	if shot == nil || shot.owner != n {
		shot = &sharedShot{owner: n}
		sharedShots[n.sharedTag] = shot
	}
	dev := cv.MapRect(n.bounds)
	if dev.Empty() {
		return
	}
	shot.layer = cv.Layer(shot.layer, dev)
	w.Paint(shot.layer.Canvas())
	shot.rect, shot.dev = n.bounds, dev
	shot.root = root
	shot.origin, shot.placed = surfaceOrigin(root)
	shot.radii = CornerRadii(w)
	if tr := n.shared; tr != nil {
		n.paintTransition(cv, tr, shot)
		return
	}
	cv.DrawLayer(shot.layer, dev, render.BlendNormal)
}

// shotIn is shot as seen from the surface root: its rect moved into
// root's coordinates through both surfaces' origins, false when the
// shot showed on another surface and either origin is unknown.
func shotIn(shot sharedShot, root Widget) (sharedShot, bool) {
	if shot.root == root {
		return shot, true
	}
	at, ok := surfaceOrigin(root)
	if !ok || !shot.placed {
		return sharedShot{}, false
	}
	shot.rect.X += shot.origin.X - at.X
	shot.rect.Y += shot.origin.Y - at.Y
	return shot, true
}

// originResolver is the host's answer to where a surface's widget tree
// sits: the root's surface origin in its toplevel's coordinates.
var originResolver func(root Widget) (Point, bool)

// SetSurfaceOriginResolver installs the host's surface placement:
// resolve maps a widget tree's root (RootOf) to where its surface's
// (0,0) sits in its toplevel surface's coordinates, false while the
// compositor has not placed it. Shared-element morphs cross surfaces
// through it, a button into the popover it opens; one process-wide
// resolver, nil for none (morphs then stay inside one surface).
func SetSurfaceOriginResolver(resolve func(root Widget) (Point, bool)) { originResolver = resolve }

// surfaceOrigin asks the host where root's surface sits.
func surfaceOrigin(root Widget) (Point, bool) {
	if originResolver == nil || root == nil {
		return Point{}, false
	}
	return originResolver(root)
}

// SharedMorphPending reports whether a tree about to show on a surface
// of its own holds a tagged widget that will morph out of another
// surface's: a widget whose tag last showed, placed, on a different
// surface. A popover opening with one skips its own reveal, since the
// morph carries the opening.
func SharedMorphPending(tree Widget) bool {
	root := RootOf(tree)
	pending := false
	walkWidgets(tree, func(w Widget) bool {
		n := nodeOf(w)
		if n == nil || n.sharedTag == "" {
			return !pending
		}
		if shot := sharedShots[n.sharedTag]; shot != nil && shot.owner != n && shot.layer != nil && shot.placed && shot.root != root {
			pending = true
		}
		return !pending
	})
	return pending
}

// startShared begins the morph out of from: on a smooth spring with
// geometry, or a short cross-fade in place under reduced motion.
func (n *node) startShared(from sharedShot) {
	if n.shared != nil {
		n.shared.stop()
	}
	tr := &sharedTransition{from: from, geometry: Current().Animations}
	n.shared = tr
	step := func(p float64) {
		tr.progress = p
		n.InvalidateRect(from.rect.Union(n.bounds))
		if p >= 1 && n.shared == tr {
			n.shared = nil
		}
	}
	if anim.Instant() {
		n.shared = nil
		return
	}
	if !tr.geometry {
		tr.fade = anim.Start(sharedFade, step)
		return
	}
	tr.spring = anim.NewSpring(anim.SpringSmooth, []float64{0}, func(pos []float64) { step(pos[0]) })
	tr.spring.SetPrecision(0.001)
	tr.spring.SetTarget(1)
}

// sharedFade is the reduced-motion cross-fade's length.
const sharedFade = anim.DurationShort

func (tr *sharedTransition) stop() {
	if tr.spring != nil {
		tr.spring.Stop()
	}
	if tr.fade != nil {
		tr.fade()
	}
}

// paintTransition composites the morph's frame: both renderings scaled
// into the interpolated rect - the old fading out, the new fading in -
// inside the interpolated rounded shape.
func (n *node) paintTransition(cv *render.Canvas, tr *sharedTransition, now *sharedShot) {
	t := math.Max(0, tr.progress)
	cur, radii := now.rect, now.radii
	if tr.geometry {
		cur = lerpRect(tr.from.rect, now.rect, t)
		radii = lerpCorners(tr.from.radii, now.radii, t)
	}
	dev := cv.MapRect(cur.Union(now.rect).Union(tr.from.rect))
	mix := cv.Layer(tr.mix, dev)
	tr.mix = mix
	mc := mix.Canvas()
	devCur := cv.MapRect(cur)
	into := func(src render.Rect) render.Affine {
		sx := float64(devCur.W) / math.Max(1, float64(src.W))
		sy := float64(devCur.H) / math.Max(1, float64(src.H))
		return render.Translate(float64(devCur.X), float64(devCur.Y)).Mul(render.Scale(sx, sy)).Mul(render.Translate(-float64(src.X), -float64(src.Y)))
	}
	if tr.from.layer != nil && t < 1 {
		mc.Composite(tr.from.layer, tr.from.dev, into(tr.from.dev), 1-math.Min(1, t))
	}
	mc.Composite(now.layer, now.dev, into(now.dev), math.Min(1, t))
	p := &render.Path{}
	render.RoundedRectPath(p, cur, radii)
	mix.ApplyPathMask(p, dev)
	cv.DrawLayer(mix, dev, render.BlendNormal)
}

// lerpRect interpolates two rects at t.
func lerpRect(a, b render.Rect, t float64) render.Rect {
	l := func(x, y int) int { return int(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return render.Rect{X: l(a.X, b.X), Y: l(a.Y, b.Y), W: max(0, l(a.W, b.W)), H: max(0, l(a.H, b.H))}
}

// lerpCorners interpolates two corner sets at t.
func lerpCorners(a, b render.Corners, t float64) render.Corners {
	l := func(x, y int) int { return max(0, int(math.Round(float64(x)+(float64(y)-float64(x))*t))) }
	return render.Corners{
		TopLeft: l(a.TopLeft, b.TopLeft), TopRight: l(a.TopRight, b.TopRight),
		BottomRight: l(a.BottomRight, b.BottomRight), BottomLeft: l(a.BottomLeft, b.BottomLeft),
	}
}
