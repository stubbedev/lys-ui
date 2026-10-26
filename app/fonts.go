package app

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/widget"
)

// Font resolves a system font family by name at the given logical
// pixel size. Malformed family names fall back per fontconfig; an
// unresolvable name is an error.
func Font(family string, sizePx float64) (*render.Typeface, error) {
	return sysfont.Best(family, sizePx)
}

// UIFont returns the default look's face: Geist, embedded in the
// binary, the family the theme's controls are designed around.
// sizePx follows Font.
func UIFont(sizePx float64) (*render.Typeface, error) {
	if sizePx <= 0 {
		return nil, fmt.Errorf("app: UIFont(%v): size must be positive", sizePx)
	}
	return sysfont.UI()
}

// UIMonoFont returns the default look's monospace face: Geist Mono,
// embedded beside UIFont.
func UIMonoFont(sizePx float64) (*render.Typeface, error) {
	if sizePx <= 0 {
		return nil, fmt.Errorf("app: UIMonoFont(%v): size must be positive", sizePx)
	}
	return sysfont.UIMono()
}

// FontWeighted resolves a system font family at a CSS numeric weight
// (400 regular, 700 bold, ...) and style. A family without that face
// resolves to its closest one, as a CSS font matcher does.
func FontWeighted(family string, sizePx float64, weight int, italic bool) (*render.Typeface, error) {
	return sysfont.Weighted(family, sizePx, weight, italic)
}

// FontVariant resolves the bold/italic face of base's family, falling
// back to the closest face the family has.
func FontVariant(base *render.Typeface, bold, italic bool) (*render.Typeface, error) {
	return sysfont.Variant(base, bold, italic)
}

// FontVariants is the widget.VariantFunc of base's family: each
// bold/italic style resolved through FontVariant (the font cache keeps
// the faces), base itself where the family has none.
func FontVariants(base *render.Typeface) widget.VariantFunc {
	return func(bold, italic bool) *render.Typeface {
		if !bold && !italic {
			return base
		}
		if f, err := FontVariant(base, bold, italic); err == nil && f != nil {
			return f
		}
		return base
	}
}

// FontVariantsOf is FontVariants for whatever face the app holds: a
// fallback chain resolves from its primary face, a bare typeface from
// itself. nil yields nil (a rich label without variants).
func FontVariantsOf(face render.Font) widget.VariantFunc {
	var base *render.Typeface
	switch f := face.(type) {
	case *render.Chain:
		base = f.Primary()
	case *render.Typeface:
		base = f
	}
	if base == nil {
		return nil
	}
	return FontVariants(base)
}

// FontFallback wraps a face in the glyph fallback chain, so glyphs the
// primary family lacks still render.
func FontFallback(face *render.Typeface) *render.Chain {
	return sysfont.Fallback(face)
}

// FontFamilies lists the installed font families by display name (the
// name a family resolves back from through Font), sorted
// case-insensitively and deduped: what a font picker offers. The list
// resolves once per process (each display name is a face lookup); the
// result is shared, so callers must not modify it.
func FontFamilies() ([]string, error) {
	familyNamesOnce.Do(func() { familyNames, familyNamesErr = fontFamilies() })
	return familyNames, familyNamesErr
}

var (
	familyNamesOnce sync.Once
	familyNames     []string
	familyNamesErr  error
)

func fontFamilies() ([]string, error) {
	families, err := sysfont.Families()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(families))
	out := make([]string, 0, len(families))
	for _, f := range families {
		name := sysfont.FamilyDisplay(f)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	slices.SortFunc(out, func(a, b string) int {
		if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	return out, nil
}
