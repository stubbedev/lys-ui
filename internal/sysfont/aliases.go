package sysfont

import (
	"encoding/binary"
	"errors"
	"os"
	"unicode/utf16"

	"github.com/stubbedev/lys-lib/third_party/typesetting/font"
	"github.com/stubbedev/lys-lib/third_party/typesetting/font/opentype"
	"github.com/stubbedev/lys-lib/third_party/typesetting/font/opentype/tables"
	"github.com/stubbedev/lys-lib/third_party/typesetting/fontscan"
)

// familyNameIDs are the name records a family is matched by, as
// fontconfig indexes them: the legacy family (1), the typographic
// family (16), and the WWS family (21).
var familyNameIDs = [...]uint16{1, 16, 21}

// systemFace is one system face: where it is and its aspect.
type systemFace struct {
	loc    fontscan.Location
	aspect font.Aspect
}

// aliasIndex maps every normalized family name a system face
// declares, on any platform and in any language, to the faces
// declaring it. fontscan indexes a face under the single English
// record it selects (the last English Windows record wins), so a
// family fontconfig matches - JetBrainsMono Nerd Font, whose en-US
// typographic record is shadowed by an en-GB "JetBrainsMono NF" -
// is unknown to the store. Built once, on the first lookup the store
// cannot answer; guarded by storeMu.
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
			aliasIndex = buildAliasIndex(footprints, fileFamilyNames)
		}
	}
	return aliasIndex[font.NormalizeFamily(family)]
}

// buildAliasIndex indexes footprints by every family name names
// reads for their location. A face whose names cannot be read is left
// out: it stays reachable under the family the store gave it.
func buildAliasIndex(footprints []fontscan.Footprint, names func(fontscan.Location) ([]string, error)) map[string][]systemFace {
	index := make(map[string][]systemFace, len(footprints))
	for _, fp := range footprints {
		declared, err := names(fp.Location)
		if err != nil {
			continue
		}
		for _, name := range declared {
			index[name] = append(index[name], systemFace{fp.Location, fp.Aspect})
		}
	}
	return index
}

// fileFamilyNames reads the family names of the face at loc. The path
// comes from fontscan's system font index, never from user input.
func fileFamilyNames(loc fontscan.Location) ([]string, error) {
	f, err := os.Open(loc.File)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	loaders, err := opentype.NewLoaders(f)
	if err != nil {
		return nil, err
	}
	if int(loc.Index) >= len(loaders) {
		return nil, errors.New("sysfont: face index out of range")
	}
	raw, err := loaders[loc.Index].RawTable(opentype.MustNewTag("name"))
	if err != nil {
		return nil, err
	}
	return familyNames(raw)
}

// errNameTable reports a truncated or inconsistent 'name' table.
var errNameTable = errors.New("sysfont: malformed name table")

// familyNames decodes every family record of a raw 'name' table:
// Unicode and Windows records as UTF-16BE, Mac Roman records through
// the Macintosh encoding, normalized and deduped in table order.
// Records in other Mac encodings are skipped, as fontconfig skips
// encodings it cannot convert.
func familyNames(raw []byte) ([]string, error) {
	if len(raw) < 6 {
		return nil, errNameTable
	}
	count := int(binary.BigEndian.Uint16(raw[2:]))
	storage := int(binary.BigEndian.Uint16(raw[4:]))
	if len(raw) < 6+12*count || storage > len(raw) {
		return nil, errNameTable
	}
	var out []string
	seen := map[string]bool{}
	for i := range count {
		rec := raw[6+12*i:]
		platform := tables.PlatformID(binary.BigEndian.Uint16(rec))
		encoding := tables.EncodingID(binary.BigEndian.Uint16(rec[2:]))
		id := binary.BigEndian.Uint16(rec[6:])
		length := int(binary.BigEndian.Uint16(rec[8:]))
		offset := storage + int(binary.BigEndian.Uint16(rec[10:]))
		if !isFamilyNameID(id) || length == 0 {
			continue
		}
		if offset+length > len(raw) {
			return nil, errNameTable
		}
		name, ok := decodeName(platform, encoding, raw[offset:offset+length])
		if !ok {
			continue
		}
		if norm := font.NormalizeFamily(name); norm != "" && !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	return out, nil
}

// isFamilyNameID reports whether id is one of familyNameIDs.
func isFamilyNameID(id uint16) bool {
	for _, want := range familyNameIDs {
		if id == want {
			return true
		}
	}
	return false
}

// decodeName decodes one name record's bytes; false for an encoding
// it cannot convert.
func decodeName(platform tables.PlatformID, encoding tables.EncodingID, b []byte) (string, bool) {
	switch {
	case platform == tables.PlatformUnicode || platform == tables.PlatformMicrosoft:
		units := make([]uint16, len(b)/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(b[2*i:])
		}
		return string(utf16.Decode(units)), true
	case platform == tables.PlatformMac && encoding == tables.PEMacRoman:
		return tables.DecodeMacintosh(b), true
	}
	return "", false
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
