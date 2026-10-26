package style

// The rendering-effect properties: filter and backdrop-filter's
// function lists, mix-blend-mode, isolation, clip-path's shapes, the
// mask-image group, and the 3-D-lite transform settings (perspective,
// transform-style, backface-visibility). The values resolve against
// the widget's border box at paint time, so lengths and percentages
// travel together as NumVal and the renderer does the arithmetic.

import (
	"slices"

	"github.com/stubbedev/lys-lib/render"
)

// NumVal is one length-or-percentage argument: the value and whether
// it is a percentage of the box dimension it applies to.
type NumVal struct {
	V   float64
	Pct bool
}

// resolve returns the value against box dimension d: the percentage
// of d, or the raw length.
func (n NumVal) resolve(d float64) float64 {
	if n.Pct {
		return n.V / 100 * d
	}
	return n.V
}

// Resolve is resolve for callers outside the style package.
func (n NumVal) Resolve(d float64) float64 { return n.resolve(d) }

// numValOf evaluates one component as a length or percentage.
func numValOf(ts []token, cx *ctx) (NumVal, bool) {
	comps := components(ts)
	if len(comps) != 1 {
		return NumVal{}, false
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok {
		return NumVal{}, false
	}
	switch n.kind {
	case numNumber, numLength:
		return NumVal{V: n.v}, true
	case numPercent:
		return NumVal{V: n.v, Pct: true}, true
	}
	return NumVal{}, false
}

// ClipShapeKind names one clip-path shape.
type ClipShapeKind uint8

// The clip-path shapes.
const (
	ClipNone ClipShapeKind = iota
	ClipInset
	ClipCircle
	ClipEllipse
	ClipPolygon
	ClipPathData
)

// ClipShape is the computed clip-path: the shape's kind and its
// geometry against the border box — the inset's per-side insets (with
// rounded corner radii), the circle and ellipse radii and centers, the
// polygon's points, or the path() data string.
type ClipShape struct {
	Kind ClipShapeKind
	// Inset is top, right, bottom, left.
	Inset    [4]NumVal
	InsetRad [4]int
	// Rad is circle()'s radius; RadX and RadY ellipse()'s pair.
	Rad, RadX, RadY  NumVal
	CenterX, CenterY NumVal
	// Poly is polygon()'s points as x, y pairs.
	Poly []NumVal
	Path string
}

// Equal reports whether two clip shapes are the same.
func (c *ClipShape) Equal(o *ClipShape) bool {
	return c.Kind == o.Kind && c.Inset == o.Inset && c.InsetRad == o.InsetRad && c.Rad == o.Rad &&
		c.RadX == o.RadX && c.RadY == o.RadY && c.CenterX == o.CenterX && c.CenterY == o.CenterY &&
		c.Path == o.Path && slices.Equal(c.Poly, o.Poly)
}

// Empty reports a shape that clips nothing.
func (c ClipShape) Empty() bool { return c.Kind == ClipNone }

// parseFilterList parses a filter or backdrop-filter value: `none` or
// a function list, every function CSS's filter-effects defines here.
// brightness() also folds its product into Values.Brightness, the
// PushBrightness fast path's channel.
func parseFilterList(ts []token, cx *ctx, v *Values, out *render.Filters) bool {
	comps := components(ts)
	if len(comps) == 1 && comps[0][0].ident("none") {
		return true
	}
	bright := 1.0
	for _, c := range comps {
		if len(c) == 0 || c[0].kind != tkFunc {
			return false
		}
		name := c[0].s
		var args [][]token
		if name == "drop-shadow" || name == "-lys-refract" {
			args = splitTop(funcArgs(c), tkWS)
		} else {
			args = splitTop(funcArgs(c), tkComma)
		}
		one := func(i int) (float64, bool) {
			if i >= len(args) {
				return 0, false
			}
			comps := components(args[i])
			if len(comps) != 1 {
				return 0, false
			}
			n, ok := evalNumeric(comps[0], cx)
			if !ok {
				return 0, false
			}
			switch n.kind {
			case numNumber, numPercent:
				if n.kind == numPercent {
					n.v /= 100
				}
				return n.v, true
			}
			return 0, false
		}
		switch name {
		case "blur":
			r, ok := lengthArg(args, cx, 0)
			if !ok {
				return false
			}
			out.Append(render.FilterOp{Kind: render.FilterBlur, Amount: r})
		case "brightness", "contrast", "grayscale", "invert", "opacity", "saturate", "sepia":
			amt, ok := one(0)
			if !ok {
				return false
			}
			var kind render.FilterKind
			switch name {
			case "brightness":
				kind = render.FilterBrightness
				bright *= amt
			case "contrast":
				kind = render.FilterContrast
			case "grayscale":
				kind = render.FilterGrayscale
			case "invert":
				kind = render.FilterInvert
			case "opacity":
				kind = render.FilterOpacity
			case "saturate":
				kind = render.FilterSaturate
			case "sepia":
				kind = render.FilterSepia
			}
			out.Append(render.FilterOp{Kind: kind, Amount: amt})
		case "hue-rotate":
			if len(args) != 1 {
				return false
			}
			comps := components(args[0])
			if len(comps) != 1 {
				return false
			}
			n, ok := evalNumeric(comps[0], cx)
			if !ok || (n.kind != numAngle && (n.kind != numNumber || n.v != 0)) {
				return false
			}
			out.Append(render.FilterOp{Kind: render.FilterHueRotate, Amount: n.v})
		case "-lys-refract":
			// lys-ui's liquid-glass bend: the bezel width along the edge
			// and the displacement at the edge, both lengths.
			var lens []float64
			for _, a := range args {
				if len(trimWS(a)) == 0 {
					continue
				}
				l, ok := lengthArg([][]token{a}, cx, 0)
				if !ok || l < 0 {
					return false
				}
				lens = append(lens, l)
			}
			if len(lens) != 2 {
				return false
			}
			out.Append(render.FilterOp{Kind: render.FilterRefract, Bezel: lens[0], Amount: lens[1]})
		case "drop-shadow":
			sh, ok := parseDropShadow(args, cx)
			if !ok {
				return false
			}
			out.Append(render.FilterOp{Kind: render.FilterDropShadow, Shadow: sh})
		default:
			return false
		}
	}
	if out.N > render.MaxFilters {
		out.N = render.MaxFilters
	}
	v.Brightness = bright
	return true
}

// lengthArg evaluates args[i] as a length in px.
func lengthArg(args [][]token, cx *ctx, i int) (float64, bool) {
	if i >= len(args) {
		return 0, false
	}
	comps := components(args[i])
	if len(comps) != 1 {
		return 0, false
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || (n.kind != numLength && n.kind != numNumber) {
		return 0, false
	}
	return n.v, true
}

// parseDropShadow reads drop-shadow()'s arguments: the two offsets,
// an optional blur radius, an optional color (before or after the
// lengths, as CSS allows either order).
func parseDropShadow(args [][]token, cx *ctx) (render.DropShadow, bool) {
	var sh render.DropShadow
	var col *colorVal
	i := 0
	if len(args) > i && len(args)-i >= 2 {
		// A leading color (a function token or a named color) is legal.
		if c, ok := parseColor(args[i], cx); ok {
			col = &c
			i++
		}
	}
	x, ok := lengthArg(args, cx, i)
	if !ok {
		return sh, false
	}
	y, ok := lengthArg(args, cx, i+1)
	if !ok {
		return sh, false
	}
	i += 2
	if len(args) > i {
		if b, ok := lengthArg(args, cx, i); ok {
			sh.Blur = b
			i++
		}
	}
	if col == nil && len(args) > i {
		if c, ok := parseColor(args[i], cx); ok {
			col = &c
		}
	}
	if col != nil {
		sh.Color = cx.resolve(*col)
	} else {
		sh.Color = render.RGB(0, 0, 0)
	}
	sh.DX, sh.DY = x, y
	return sh, true
}

// parseFilterInto is parseFilterList's entry for the filter property.
func parseFilter(ts []token, cx *ctx, v *Values) bool {
	v.Filter = render.Filters{}
	v.Brightness = 1
	return parseFilterList(ts, cx, v, &v.Filter)
}

// parseBackdropFilter parses the backdrop-filter property: the same
// function list as filter, applied to the backdrop.
func parseBackdropFilter(ts []token, cx *ctx, v *Values) bool {
	v.Backdrop = render.Filters{}
	return parseFilterList(ts, cx, v, &v.Backdrop)
}

// parseMixBlendMode parses the mix-blend-mode keywords.
func parseMixBlendMode(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	m, ok := render.BlendModeNamed(rawName(comps[0]))
	if !ok {
		return false
	}
	v.Blend = m
	return true
}

// parseIsolation parses isolation: isolate or auto.
func parseIsolation(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	switch rawName(comps[0]) {
	case "isolate":
		v.Isolate = true
	case "auto":
		v.Isolate = false
	default:
		return false
	}
	return true
}

// parseClipPath parses the clip-path property: none or one shape —
// inset(), circle(), ellipse(), polygon() or path().
func parseClipPath(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 1 && comps[0][0].ident("none") {
		v.Clip = ClipShape{}
		return true
	}
	if len(comps) != 1 || comps[0][0].kind != tkFunc {
		return false
	}
	name := comps[0][0].s
	var args [][]token
	for _, g := range splitTop(funcArgs(comps[0]), tkComma) {
		for _, a := range splitTop(g, tkWS) {
			if len(a) > 0 {
				args = append(args, a)
			}
		}
	}
	switch name {
	case "inset":
		return parseClipInset(args, cx, v)
	case "circle":
		return parseClipRad(args, cx, v, false)
	case "ellipse":
		return parseClipRad(args, cx, v, true)
	case "polygon":
		return parseClipPolygon(args, cx, v)
	case "path":
		return parseClipPathData(args, v)
	}
	return false
}

// clipPos reads one circle/ellipse position component: a length, a
// percentage, or the position keywords.
func clipPos(ts []token, cx *ctx) (NumVal, bool) {
	if len(ts) == 1 && ts[0].kind == tkIdent {
		switch rawName(ts) {
		case "left", "top":
			return NumVal{}, true
		case "center":
			return NumVal{V: 50, Pct: true}, true
		case "right", "bottom":
			return NumVal{V: 100, Pct: true}, true
		}
	}
	return numValOf(ts, cx)
}

// parseClipInset reads inset()'s space-separated side list (1 to 4
// values) and an optional `round <radius>` corner.
func parseClipInset(args [][]token, cx *ctx, v *Values) bool {
	var vals []NumVal
	i := 0
	for ; i < len(args); i++ {
		if len(args[i]) > 0 && args[i][0].ident("round") {
			break
		}
		nv, ok := numValOf(args[i], cx)
		if !ok {
			return false
		}
		vals = append(vals, nv)
	}
	if len(vals) < 1 || len(vals) > 4 {
		return false
	}
	// The CSS side shorthand: 1 = all, 2 = tb/lr, 3 = t/lr/b, 4 = trbl.
	var out [4]NumVal
	switch len(vals) {
	case 1:
		out = [4]NumVal{vals[0], vals[0], vals[0], vals[0]}
	case 2:
		out = [4]NumVal{vals[0], vals[1], vals[0], vals[1]}
	case 3:
		out = [4]NumVal{vals[0], vals[1], vals[2], vals[1]}
	case 4:
		out = [4]NumVal{vals[0], vals[1], vals[2], vals[3]}
	}
	clip := ClipShape{Kind: ClipInset, Inset: out}
	if i < len(args) && args[i][0].ident("round") && i+1 < len(args) {
		comps := components(args[i+1])
		if len(comps) == 1 {
			if n, ok := evalNumeric(comps[0], cx); ok && n.kind == numLength {
				clip.InsetRad = [4]int{int(n.v), int(n.v), int(n.v), int(n.v)}
			}
		}
	}
	v.Clip = clip
	return true
}

// parseClipRad reads circle() or ellipse(): one or two radii and an
// optional `at <x> <y>` center.
func parseClipRad(args [][]token, cx *ctx, v *Values, ellipse bool) bool {
	c := ClipShape{CenterX: NumVal{V: 50, Pct: true}, CenterY: NumVal{V: 50, Pct: true}}
	if ellipse {
		c.Kind = ClipEllipse
	} else {
		c.Kind = ClipCircle
	}
	i := 0
	if len(args) == 0 || len(args[0]) == 0 || !args[0][0].ident("at") {
		if ellipse {
			if len(args) < 2 {
				return false
			}
			rx, ok := numValOf(args[0], cx)
			if !ok {
				return false
			}
			ry, ok := numValOf(args[1], cx)
			if !ok {
				return false
			}
			c.RadX, c.RadY = rx, ry
			i = 2
		} else {
			r, ok := numValOf(args[0], cx)
			if !ok {
				return false
			}
			c.Rad = r
			i = 1
		}
	}
	if len(args)-i >= 2 && len(args[i]) > 0 && args[i][0].ident("at") {
		x, ok := clipPos(args[i+1], cx)
		if !ok {
			return false
		}
		y := x
		if len(args)-i >= 3 {
			if y, ok = clipPos(args[i+2], cx); !ok {
				return false
			}
		}
		c.CenterX, c.CenterY = x, y
	} else if len(args)-i != 0 {
		return false
	}
	v.Clip = c
	return true
}

// parseClipPolygon reads polygon()'s point list; the optional
// fill-rule keyword is accepted and ignored (every path here fills
// nonzero).
func parseClipPolygon(args [][]token, cx *ctx, v *Values) bool {
	i := 0
	if len(args) > 0 && len(args[0]) == 1 {
		switch rawName(args[0]) {
		case "nonzero", "evenodd":
			i = 1
		}
	}
	var pts []NumVal
	for ; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return false
		}
		x, ok := numValOf(args[i], cx)
		if !ok {
			return false
		}
		y, ok := numValOf(args[i+1], cx)
		if !ok {
			return false
		}
		pts = append(pts, x, y)
	}
	if len(pts) < 6 {
		return false
	}
	v.Clip = ClipShape{Kind: ClipPolygon, Poly: pts}
	return true
}

// parseClipPathData reads path()'s SVG data string.
func parseClipPathData(args [][]token, v *Values) bool {
	if len(args) != 1 || len(args[0]) != 1 || args[0][0].kind != tkString {
		return false
	}
	v.Clip = ClipShape{Kind: ClipPathData, Path: args[0][0].s}
	return true
}

// parsePerspective parses the perspective property: none or a length.
func parsePerspective(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 1 && comps[0][0].ident("none") {
		v.Perspective = 0
		return true
	}
	if len(comps) != 1 {
		return false
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numLength || n.v <= 0 {
		return false
	}
	v.Perspective = n.v
	return true
}

// parseTransformStyle parses transform-style: flat or preserve-3d.
func parseTransformStyle(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	switch rawName(comps[0]) {
	case "preserve-3d":
		v.Preserve3D = true
	case "flat":
		v.Preserve3D = false
	default:
		return false
	}
	return true
}

// parseBackfaceVisibility parses backface-visibility: visible or
// hidden.
func parseBackfaceVisibility(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	switch rawName(comps[0]) {
	case "hidden":
		v.BackfaceHidden = true
	case "visible":
		v.BackfaceHidden = false
	default:
		return false
	}
	return true
}

// parseLayoutMotion parses lys-ui's -lys-layout-motion: none, or the
// spring a change of the box's laid-out rect animates on.
func parseLayoutMotion(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	switch rawName(comps[0]) {
	case "none":
		v.LayoutMotion = LayoutMotionNone
	case "smooth":
		v.LayoutMotion = LayoutMotionSmooth
	case "snappy":
		v.LayoutMotion = LayoutMotionSnappy
	case "bouncy":
		v.LayoutMotion = LayoutMotionBouncy
	default:
		return false
	}
	return true
}
