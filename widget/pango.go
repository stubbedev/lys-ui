package widget

import (
	"math"
	"strconv"
	"strings"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Pango's markup constants: the weight <b> sets and the weight at
// which a run counts as bold, the factor each size step scales by, and
// the baseline shifts of <sup> and <sub> as fractions of the size.
const (
	weightBold      = 700
	weightBoldFrom  = 600
	pangoSizeStep   = 1.2
	superscriptRise = 0.35
	subscriptDrop   = -0.2
)

// pointPx is one typographic point in pixels: lys-ui's pixels are CSS
// pixels, 96 to the inch against 72 points.
const pointPx = 96.0 / 72.0

// pangoWeights are Pango's named weights.
var pangoWeights = map[string]int{
	"thin": 100, "ultralight": 200, "light": 300, "semilight": 350, "book": 380,
	"normal": 400, "medium": 500, "semibold": 600, "bold": 700, "ultrabold": 800,
	"heavy": 900, "ultraheavy": 1000,
}

// pangoSizes are Pango's absolute named sizes, as scales of the
// label's size.
var pangoSizes = map[string]float64{
	"xx-small": 1 / (pangoSizeStep * pangoSizeStep * pangoSizeStep),
	"x-small":  1 / (pangoSizeStep * pangoSizeStep),
	"small":    1 / pangoSizeStep,
	"medium":   1,
	"large":    pangoSizeStep,
	"x-large":  pangoSizeStep * pangoSizeStep,
	"xx-large": pangoSizeStep * pangoSizeStep * pangoSizeStep,
}

// withWeight sets the run's numeric weight, keeping Bold in step.
func withWeight(st TextStyle, w int) TextStyle {
	st.Weight, st.Bold = w, w >= weightBoldFrom
	return st
}

// scaled multiplies the run's size by f, compounding with the scales
// the open tags already set.
func scaled(st TextStyle, f float64) TextStyle {
	if st.Scale == 0 {
		st.Scale = 1
	}
	st.Scale *= f
	return st
}

// convenienceTag applies Pango's shorthand tags: <big> and <small> a
// size step, <tt> monospace, <sup> and <sub> a step smaller off the
// baseline.
func convenienceTag(name string, st TextStyle) TextStyle {
	switch name {
	case "big":
		return scaled(st, pangoSizeStep)
	case "small":
		return scaled(st, 1/pangoSizeStep)
	case "tt":
		st.Family = "monospace"
	case "sup":
		st = scaled(st, 1/pangoSizeStep)
		st.Shift = superscriptRise
	case "sub":
		st = scaled(st, 1/pangoSizeStep)
		st.Shift = subscriptDrop
	}
	return st
}

// spanAttr applies one <span> attribute; false for an attribute Pango
// does not name or a value it would reject.
func spanAttr(st TextStyle, attr, v string) (TextStyle, bool) {
	switch attr {
	case "foreground", "fgcolor", "color":
		c, ok := pangoColor(v)
		st.Color = c
		return st, ok
	case "background", "bgcolor":
		c, ok := pangoColor(v)
		st.Background = c
		return st, ok
	case "alpha", "fgalpha":
		a, ok := pangoAlpha(v)
		st.Alpha = a
		return st, ok
	case "weight":
		w, ok := pangoWeights[v]
		if !ok {
			n, err := strconv.Atoi(v)
			if err != nil || n < 100 || n > 1000 {
				return st, false
			}
			w = n
		}
		return withWeight(st, w), true
	case "style":
		switch v {
		case "normal":
			st.Italic = false
		case "italic", "oblique":
			st.Italic = true
		default:
			return st, false
		}
		return st, true
	case "size":
		return pangoSize(st, v)
	case "font_family", "face":
		st.Family = v
		return st, v != ""
	case "underline":
		switch v {
		case "none":
			st.Underline = false
		case "single", "low":
			st.Underline, st.UnderlineStyle = true, render.DecorationSolid
		case "double":
			st.Underline, st.UnderlineStyle = true, render.DecorationDouble
		case "error":
			st.Underline, st.UnderlineStyle = true, render.DecorationWavy
		default:
			return st, false
		}
		return st, true
	case "strikethrough":
		on, err := strconv.ParseBool(v)
		st.Strikethrough = on
		return st, err == nil && (v == "true" || v == "false")
	case "letter_spacing":
		n, err := strconv.Atoi(v)
		st.LetterSpacing = float64(n) / 1024 * pointPx
		return st, err == nil
	case "font_features":
		spec, ok := pangoFeatures(v)
		st.Features = spec
		return st, ok
	}
	return st, false
}

// pangoColor reads a CSS color name or a hex color.
func pangoColor(v string) (render.Color, bool) {
	if c, ok := style.NamedColor(strings.ToLower(v)); ok {
		return c, true
	}
	return parseHexColor(v)
}

// pangoAlpha reads an alpha as a percentage or out of 65535.
func pangoAlpha(v string) (float64, bool) {
	if pct, ok := strings.CutSuffix(v, "%"); ok {
		n, err := strconv.ParseFloat(pct, 64)
		return n / 100, err == nil && n > 0 && n <= 100
	}
	n, err := strconv.Atoi(v)
	return float64(n) / 65535, err == nil && n > 0 && n <= 65535
}

// pangoSize applies a size: a named absolute size (a scale of the
// label's), smaller or larger (a step, compounding), a percentage
// (compounding), points, or 1024ths of a point.
func pangoSize(st TextStyle, v string) (TextStyle, bool) {
	if f, ok := pangoSizes[v]; ok {
		st.SizePx, st.Scale = 0, f
		return st, true
	}
	switch v {
	case "smaller":
		return scaled(st, 1/pangoSizeStep), true
	case "larger":
		return scaled(st, pangoSizeStep), true
	}
	if pct, ok := strings.CutSuffix(v, "%"); ok {
		n, err := strconv.ParseFloat(pct, 64)
		return scaled(st, n/100), err == nil && n > 0
	}
	if pt, ok := strings.CutSuffix(v, "pt"); ok {
		n, err := strconv.ParseFloat(pt, 64)
		st.SizePx, st.Scale = n*pointPx, 0
		return st, err == nil && n > 0
	}
	n, err := strconv.Atoi(v)
	st.SizePx, st.Scale = float64(n)/1024*pointPx, 0
	return st, err == nil && n > 0
}

// pangoFeatures converts Pango's font_features (HarfBuzz's feature
// syntax, comma separated: tag, +tag, -tag, tag=N, tag on, tag off)
// to the canonical tag=value;tag=value list.
func pangoFeatures(v string) (string, bool) {
	var out []string
	for item := range strings.SplitSeq(v, ",") {
		item = strings.TrimSpace(item)
		value := "1"
		switch {
		case strings.HasPrefix(item, "-"):
			item, value = item[1:], "0"
		case strings.HasPrefix(item, "+"):
			item = item[1:]
		}
		if tag, n, ok := strings.Cut(item, "="); ok {
			item, value = strings.TrimSpace(tag), strings.TrimSpace(n)
		} else if tag, word, ok := strings.Cut(item, " "); ok {
			item = tag
			switch strings.TrimSpace(word) {
			case "on":
			case "off":
				value = "0"
			default:
				return "", false
			}
		}
		item = strings.Trim(item, `"`)
		if len(item) != 4 {
			return "", false
		}
		if n, err := strconv.ParseFloat(value, 64); err != nil || n < 0 || n != math.Trunc(n) {
			return "", false
		}
		out = append(out, item+"="+value)
	}
	return strings.Join(out, ";"), len(out) > 0
}
