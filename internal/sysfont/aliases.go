package sysfont

import (
	"github.com/stubbedev/lys-lib/third_party/typesetting/font"
	"github.com/stubbedev/lys-lib/third_party/typesetting/fontscan"
)

// systemFace is one system face: where it is and its aspect.
type systemFace struct {
	loc    fontscan.Location
	aspect font.Aspect
}

// aliasIndex maps every other family name a system face declares to the
// faces declaring it, from the store's index: fontscan files a face
// under the single English record it selects (the last English Windows
// record wins), so a family fontconfig matches - JetBrainsMono Nerd
// Font, whose en-US typographic record is shadowed by an en-GB
// "JetBrainsMono NF" - is only an alias there. Built once, on the first
// lookup the store cannot answer by family; guarded by storeMu.
var (
	aliasIndex      map[string][]systemFace
	aliasIndexBuilt bool
)

// aliasFaces returns the faces declaring family among their name
// records. Caller holds storeMu.
func aliasFaces(family string) []systemFace {
	if !aliasIndexBuilt {
		aliasIndexBuilt = true
		footprints, err := fontscan.SystemFonts(quietLogger{}, "")
		if err == nil {
			aliasIndex = buildAliasIndex(footprints)
		}
	}
	return aliasIndex[font.NormalizeFamily(family)]
}

// buildAliasIndex indexes footprints by their aliases.
func buildAliasIndex(footprints []fontscan.Footprint) map[string][]systemFace {
	index := map[string][]systemFace{}
	for _, fp := range footprints {
		for _, name := range fp.Aliases {
			index[name] = append(index[name], systemFace{fp.Location, fp.Aspect})
		}
	}
	return index
}

// closestFace picks the face of faces closest to want by the CSS
// font matching steps fontscan applies within a family (CSS Fonts 3
// §5.2): stretch, then style, then weight. faces must not be empty.
func closestFace(faces []systemFace, want font.Aspect) systemFace {
	want.SetDefaults()
	stretch := closest(faces, func(a font.Aspect) float32 { return float32(a.Stretch) }, float32(want.Stretch), float32(font.StretchNormal), float32(font.StretchNormal))
	faces = filterAspect(faces, func(a font.Aspect) bool { return a.Stretch == font.Stretch(stretch) })
	style := matchStyle(faces, want.Style)
	faces = filterAspect(faces, func(a font.Aspect) bool { return a.Style == style })
	weight := closest(faces, func(a font.Aspect) float32 { return float32(a.Weight) }, float32(want.Weight), 400, 500)
	faces = filterAspect(faces, func(a font.Aspect) bool { return a.Weight == font.Weight(weight) })
	return faces[0]
}

// closest returns the value of faces nearest to query under the CSS
// direction rule: a query within [lo, hi] prefers heavier values up to
// hi, then lighter, then heavier; below lo lighter first; above hi
// heavier first. Stretch passes lo = hi = normal (narrow first at or
// below normal), weight lo 400 and hi 500.
func closest(faces []systemFace, value func(font.Aspect) float32, query, lo, hi float32) float32 {
	var above, below float32
	var hasAbove, hasBelow bool
	for _, f := range faces {
		v := value(f.aspect)
		switch {
		case v == query:
			return query
		case v > query && (!hasAbove || v < above):
			above, hasAbove = v, true
		case v < query && (!hasBelow || v > below):
			below, hasBelow = v, true
		}
	}
	switch {
	case lo <= query && query <= hi && lo != hi:
		if hasAbove && above <= hi {
			return above
		}
		if hasBelow {
			return below
		}
		return above
	case query <= lo:
		if hasBelow {
			return below
		}
		return above
	default:
		if hasAbove {
			return above
		}
		return below
	}
}

// matchStyle is the CSS style step: the wanted style when a face has
// it, else the other (fontscan folds oblique into italic).
func matchStyle(faces []systemFace, want font.Style) font.Style {
	for _, f := range faces {
		if f.aspect.Style == want {
			return want
		}
	}
	return faces[0].aspect.Style
}

// filterAspect keeps the faces whose aspect satisfies keep.
func filterAspect(faces []systemFace, keep func(font.Aspect) bool) []systemFace {
	out := make([]systemFace, 0, len(faces))
	for _, f := range faces {
		if keep(f.aspect) {
			out = append(out, f)
		}
	}
	return out
}
