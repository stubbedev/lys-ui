package style

// The 2-D transform math the CSS layer interpolates. A computed
// transform is an Xform: the composed affine the widget paints
// through, plus the parsed primitive list. Tweens (transitions and
// keyframes) interpolate the primitive lists function by function when
// they match - the only way rotate(0deg) to rotate(360deg) can turn -
// and fall back to the affines' decomposition (translation, rotation,
// scale, shear) when they do not, because lerping matrix entries of
// two rotations mid-way shrinks the shape instead of turning it.

import (
	"math"

	"github.com/stubbedev/lys-lib/render"
)

// XformOp names one transform primitive.
type XformOp uint8

// The primitives, matching transformPrim's functions.
const (
	XformRotate XformOp = iota
	XformScale
	XformTranslate
	XformSkew
	XformMatrix
	XformPerspective
	XformRotateX
	XformRotateY
	XformTranslateZ
)

// XformPart is one parsed transform function: its op and the numbers
// that rebuild it (rotate's degrees; scale's sx, sy; translate's dx,
// dy; skew's two angles; matrix's six entries; perspective's distance;
// rotateX/rotateY's degrees; translateZ's dz).
type XformPart struct {
	Op XformOp
	N  [6]float64
}

// Xform is one computed transform: the composed affine, the projective
// composition (which carries the 3-D functions), whether the composed
// map shows the element's back, and the primitives that built it.
type Xform struct {
	M render.Affine
	P render.M3
	// m4 is the full 4x4 composition the plane map derives from: the
	// z the 3-D functions leave matters to the next function, so the
	// homography cannot be composed per-part.
	m4 render.Mat4
	// Backface reports the composed projective map with a negative
	// determinant - the flat-pipeline reading of the element's back
	// facing the viewer.
	Backface bool
	Parts    []XformPart
}

// Equal reports whether two transforms map the same: the composed
// affine and plane map, and which face shows.
func (x *Xform) Equal(o *Xform) bool {
	return x.M == o.M && x.P == o.P && x.Backface == o.Backface
}

// XformIdentity moves nothing.
var XformIdentity = Xform{M: render.Identity, P: render.Identity3, m4: render.Identity4}

// partAffine rebuilds one primitive's affine from its numbers; the
// 3-D primitives have no affine of their own (their action lives in
// the projective composition), so they fall back to the identity.
func partAffine(p XformPart) render.Affine {
	switch p.Op {
	case XformRotate:
		return render.Rotate(p.N[0])
	case XformScale:
		return render.Scale(p.N[0], p.N[1])
	case XformTranslate:
		return render.Translate(p.N[0], p.N[1])
	case XformSkew:
		return skewAffine(p.N[0], p.N[1])
	case XformMatrix:
		return render.Affine{A: p.N[0], B: p.N[1], C: p.N[2], D: p.N[3], E: p.N[4], F: p.N[5]}
	}
	return render.Identity
}

// partM4 rebuilds one primitive's 4x4 map: the affines embedded, the
// 3-D functions as themselves.
func partM4(p XformPart) render.Mat4 {
	switch p.Op {
	case XformPerspective:
		return render.Perspective4(p.N[0])
	case XformRotateX:
		return render.RotateX4(p.N[0])
	case XformRotateY:
		return render.RotateY4(p.N[0])
	case XformTranslateZ:
		return render.TranslateZ4(p.N[0])
	}
	return render.PlaneRotZ(partAffine(p))
}

// compose folds one more primitive into x: the affine chain for the
// 2-D fast path, the 4x4 chain (whose plane restriction is the
// projective map), and the backface flag off the plane determinant.
func (x *Xform) compose(p XformPart) {
	x.M = x.M.Mul(partAffine(p))
	x.m4 = x.m4.Mul(partM4(p))
	x.P = x.m4.Plane3()
	x.Backface = x.P[0]*x.P[4]-x.P[1]*x.P[3] < 0
	x.Parts = append(x.Parts, p)
}

// After returns x composed with o after it: o's functions apply
// once x's have, the transform addition composite (u followed by b).
// The animation-composition add and accumulate modes both build it -
// matched-list accumulation needs the per-function lists two
// keyframes would have to share, and the channels here carry the
// composed map, so both modes compose.
func (x Xform) After(o Xform) Xform {
	out := x
	for _, p := range o.Parts {
		out.compose(p)
	}
	return out
}

// lerpParts walks two matching primitive lists function by function.
func lerpParts(a, b []XformPart, t float64) (render.Affine, render.M3, bool) {
	if len(a) != len(b) || len(a) == 0 {
		return render.Identity, render.Identity3, false
	}
	m := render.Identity
	m4 := render.Identity4
	for i := range a {
		if a[i].Op != b[i].Op {
			return render.Identity, render.Identity3, false
		}
		var part XformPart
		part.Op = a[i].Op
		for j := range a[i].N {
			part.N[j] = a[i].N[j] + (b[i].N[j]-a[i].N[j])*t
		}
		m = m.Mul(partAffine(part))
		m4 = m4.Mul(partM4(part))
	}
	return m, m4.Plane3(), true
}

// xformParts is one affine's decomposition: the translation, the
// rotation in degrees, the scale factors, and the shear (x slanted by
// kx relative to y). Recomposing the lerped parts reproduces every
// affine exactly, and pure rotations, scales, and translations tween
// along their natural axes.
type xformParts struct {
	tx, ty float64
	rot    float64
	sx, sy float64
	kx     float64
}

// decomposeXform splits an affine into its parts. A collapsed axis
// (a zero scale) keeps the parts of the fallback identity for that
// axis rather than NaNs.
func decomposeXform(m render.Affine) xformParts {
	p := xformParts{tx: m.E, ty: m.F, sx: 1, sy: 1}
	p.sx = math.Hypot(m.A, m.B)
	if p.sx < 1e-12 {
		return p
	}
	p.rot = math.Atan2(m.B, m.A)
	s, c := math.Sincos(p.rot)
	p.sy = -m.C*s + m.D*c
	if math.Abs(p.sy) < 1e-12 {
		p.sy = 0
		p.kx = 0
		return p
	}
	p.kx = (m.C*c + m.D*s) / p.sy
	return p
}

// recomposeXform rebuilds the affine from lerped parts: the
// translation, the rotation, the shear, then the scale.
func recomposeXform(p xformParts) render.Affine {
	s, c := math.Sincos(p.rot)
	return render.Affine{
		A: c * p.sx,
		B: s * p.sx,
		C: c*p.kx*p.sy - s*p.sy,
		D: s*p.kx*p.sy + c*p.sy,
		E: p.tx,
		F: p.ty,
	}
}

// shortestTurn takes the rotation delta the short way around, so a
// tween from 350deg to 10deg turns through north, not through south.
func shortestTurn(a, b float64) float64 {
	d := math.Mod(b-a, 360)
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return d
}

// LerpXform walks two transforms: the primitive lists when they match
// (a spin turns), the affines' decompositions otherwise.
func LerpXform(a, b Xform, t float64) Xform {
	if m, p, ok := lerpParts(a.Parts, b.Parts, t); ok {
		return Xform{M: m, P: p, Backface: p[0]*p[4]-p[1]*p[3] < 0}
	}
	pa, pb := decomposeXform(a.M), decomposeXform(b.M)
	m := recomposeXform(xformParts{
		tx:  pa.tx + (pb.tx-pa.tx)*t,
		ty:  pa.ty + (pb.ty-pa.ty)*t,
		rot: pa.rot + shortestTurn(pa.rot, pb.rot)*t,
		sx:  pa.sx + (pb.sx-pa.sx)*t,
		sy:  pa.sy + (pb.sy-pa.sy)*t,
		kx:  pa.kx + (pb.kx-pa.kx)*t,
	})
	return Xform{M: m, P: render.PlaneRotZ(m).Plane3()}
}
