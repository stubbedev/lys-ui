package style

import "github.com/stubbedev/lys-lib/render"

// The replaced-content properties (docs/css.md): object-fit picks how
// an image fills its widget's box and object-position where the
// concrete object sits in it. They are layout longhands: undeclared,
// the widget's programmatic scale policy stands.

// AxisPos is one axis of a CSS <position>: the point Frac of the free
// space along the axis plus Px pixels.
type AxisPos struct {
	Frac, Px float64
}

// Within is the offset along the axis of an object placed with free
// pixels of free space: its fraction of the free space plus its pixels.
func (a AxisPos) Within(free float64) float64 { return a.Frac*free + a.Px }

// objectFitNames are the object-fit keywords, each computing to the
// render scale policy that implements it.
var objectFitNames = map[string]render.ImageScale{
	"fill":       render.ImageStretch,
	"contain":    render.ImageFit,
	"cover":      render.ImageCover,
	"none":       render.ImageNone,
	"scale-down": render.ImageScaleDown,
}

// parseObjectFit parses one object-fit keyword.
func parseObjectFit(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	fit, ok := objectFitNames[s]
	if ok {
		v.Layout.ObjectFit = fit
	}
	return ok
}

// parseObjectPosition parses object-position as a CSS <position>.
func parseObjectPosition(ts []token, cx *ctx, v *Values) bool {
	pos, ok := parsePosition(components(ts), cx)
	if ok {
		v.Layout.ObjectPosition = pos
	}
	return ok
}

// posPart is one component of a <position>: an edge keyword's axis
// (0 either, 1 horizontal, 2 vertical) and its point, or an offset.
type posPart struct {
	axis    int
	pos     AxisPos
	keyword bool
	far     bool
}

// posKeywords are the <position> keywords: the axis each names (0 for
// center, either) and its fraction.
var posKeywords = map[string]posPart{
	"left":   {axis: 1, pos: AxisPos{}, keyword: true},
	"right":  {axis: 1, pos: AxisPos{Frac: 1}, keyword: true, far: true},
	"top":    {axis: 2, pos: AxisPos{}, keyword: true},
	"bottom": {axis: 2, pos: AxisPos{Frac: 1}, keyword: true, far: true},
	"center": {axis: 0, pos: AxisPos{Frac: 0.5}, keyword: true},
}

// posPartOf reads one component: a keyword, or a length or percentage.
func posPartOf(c []token, cx *ctx) (posPart, bool) {
	if s, ok := identOf([][]token{c}); ok {
		p, ok := posKeywords[s]
		return p, ok
	}
	n, ok := numValOf(c, cx)
	if !ok {
		return posPart{}, false
	}
	if n.Pct {
		return posPart{pos: AxisPos{Frac: n.V / 100}}, true
	}
	return posPart{pos: AxisPos{Px: n.V}}, true
}

// parsePosition parses a CSS <position> (css-values-4 §8.3): one value
// (the other axis centered), two values (horizontal then vertical, or
// two keywords in either order), and the three- and four-value forms of
// edge keywords each followed by an optional offset from that edge.
// It returns the horizontal then the vertical axis.
func parsePosition(comps [][]token, cx *ctx) ([2]AxisPos, bool) {
	center := AxisPos{Frac: 0.5}
	parts := make([]posPart, 0, 4)
	for _, c := range comps {
		p, ok := posPartOf(c, cx)
		if !ok {
			return [2]AxisPos{}, false
		}
		parts = append(parts, p)
	}
	switch len(parts) {
	case 1:
		if parts[0].axis == 2 {
			return [2]AxisPos{center, parts[0].pos}, true
		}
		return [2]AxisPos{parts[0].pos, center}, true
	case 2:
		a, b := parts[0], parts[1]
		if a.axis == 2 || b.axis == 1 {
			if !a.keyword || !b.keyword || a.axis == b.axis {
				return [2]AxisPos{}, false
			}
			a, b = b, a
		}
		if a.axis == 2 || b.axis == 1 {
			return [2]AxisPos{}, false
		}
		return [2]AxisPos{a.pos, b.pos}, true
	case 3, 4:
		return edgeOffsets(parts)
	}
	return [2]AxisPos{}, false
}

// edgeOffsets reads the three- and four-value <position> forms: two
// edge keywords, one per axis, each but center optionally followed by
// its offset from that edge. A center takes the axis the other keyword
// leaves.
func edgeOffsets(parts []posPart) ([2]AxisPos, bool) {
	var groups [2]posPart
	n := 0
	for i := 0; i < len(parts); i++ {
		k := parts[i]
		if !k.keyword || n == 2 {
			return [2]AxisPos{}, false
		}
		if i+1 < len(parts) && !parts[i+1].keyword {
			if k.axis == 0 {
				return [2]AxisPos{}, false
			}
			off := parts[i+1].pos
			if k.far {
				off = AxisPos{Frac: -off.Frac, Px: -off.Px}
			}
			k.pos = AxisPos{Frac: k.pos.Frac + off.Frac, Px: off.Px}
			i++
		}
		groups[n] = k
		n++
	}
	a, b := groups[0], groups[1]
	switch {
	case n != 2 || a.axis != 0 && a.axis == b.axis:
		return [2]AxisPos{}, false
	case a.axis == 2 || b.axis == 1:
		a, b = b, a
	}
	return [2]AxisPos{a.pos, b.pos}, true
}
