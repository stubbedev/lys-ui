package style

import (
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// The icon, caret and text-decoration property parsers.

// parseIconTransform parses -gtk-icon-transform over the same full
// 2-D set as transform, composed into the affine icons draw through.
func parseIconTransform(ts []token, cx *ctx, v *Values) bool {
	m, ok := composeTransform(ts, cx)
	if !ok {
		return false
	}
	v.IconXform = m
	return true
}

// parseIconSource parses -gtk-icon-source: -gtk-icontheme("name") or
// none.
func parseIconSource(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) == 1 && first[0].ident("none") {
		v.IconSource = ""
		return true
	}
	if len(first) == 0 || first[0].kind != tkFunc || first[0].s != "-gtk-icontheme" {
		return false
	}
	args := splitTop(funcArgs(first), tkComma)
	if len(args) != 1 || len(args[0]) != 1 || args[0][0].kind != tkString {
		return false
	}
	v.IconSource = args[0][0].s
	return true
}

// parseIconPalette parses -gtk-icon-palette's `success <color>` entry
// into the palette tint; other channels' entries keep their defaults.
func parseIconPalette(ts []token, cx *ctx, v *Values) bool {
	sawChannel := false
	for _, part := range splitTop(ts, tkComma) {
		words := components(part)
		if len(words) != 2 || words[0][0].kind != tkIdent {
			return false
		}
		cv, ok := parseColor(words[1], cx)
		if !ok {
			return false
		}
		if strings.EqualFold(words[0][0].s, "success") {
			v.PaletteTint = cx.resolve(cv)
		}
		sawChannel = true
	}
	return sawChannel
}

// parseCaretColor parses caret-color: a color or auto (the text
// color).
func parseCaretColor(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) == 1 && first[0].ident("auto") {
		v.CaretColor = 0
		return true
	}
	cv, ok := parseColor(first, cx)
	if !ok {
		return false
	}
	v.CaretColor = cx.resolve(cv)
	return true
}

// parseTextDecoration parses the text-decoration shorthand: none, or
// any of the lines (underline, overline, line-through), a style
// (solid, double, dotted, dashed, wavy), and a color, in any order.
func parseTextDecoration(ts []token, cx *ctx, v *Values) bool {
	var d render.Decoration
	none, lines, style, color := false, false, false, false
	for _, part := range components(ts) {
		if len(part) == 1 && part[0].kind == tkIdent {
			name := strings.ToLower(part[0].s)
			if line, ok := decorationLines[name]; ok {
				d.Lines |= line
				lines = true
				continue
			}
			if st, ok := decorationStyles[name]; ok && !style {
				d.Style, style = st, true
				continue
			}
			if name == "none" && !none {
				none = true
				continue
			}
		}
		cv, ok := parseColor(part, cx)
		if !ok || color {
			return false
		}
		d.Color, color = cx.resolve(cv), true
	}
	if none && lines {
		return false
	}
	if !none && !lines && !style && !color {
		return false
	}
	v.Decoration = d
	return true
}

// parseTextDecorationLine parses text-decoration-line: none or a set
// of underline, overline and line-through (blink is valid and draws
// nothing, as the spec lets user agents decide).
func parseTextDecorationLine(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if s, ok := identOf(comps); ok && s == "none" {
		v.Decoration.Lines = 0
		return true
	}
	var lines render.DecorationLine
	blink := false
	for _, c := range comps {
		s, ok := identOf([][]token{c})
		if !ok {
			return false
		}
		if s == "blink" && !blink {
			blink = true
			continue
		}
		line, ok := decorationLines[s]
		if !ok || lines&line != 0 {
			return false
		}
		lines |= line
	}
	v.Decoration.Lines = lines
	return true
}

// parseTextDecorationStyle parses text-decoration-style: solid,
// double, dotted, dashed or wavy.
func parseTextDecorationStyle(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	st, ok := decorationStyles[s]
	if !ok {
		return false
	}
	v.Decoration.Style = st
	return true
}

// decorationLines and decorationStyles are text-decoration's keywords.
var (
	decorationLines = map[string]render.DecorationLine{
		"underline": render.Underline, "overline": render.Overline, "line-through": render.LineThrough,
	}
	decorationStyles = map[string]render.DecorationStyle{
		"solid": render.DecorationSolid, "double": render.DecorationDouble, "dotted": render.DecorationDotted,
		"dashed": render.DecorationDashed, "wavy": render.DecorationWavy,
	}
)
