// Package sysfont resolves system fonts through fontscan's CSS font
// selection rules, replacing per-app guesswork. Family matching here
// understands generic families, so "sans-serif" can never resolve to a
// serif face.
//
// The system store is scanned once per process and its coverage map is
// reused for every resolution after that; Fallback builds on it to
// shape mixed-script text (CJK, emoji, Cyrillic in a latin face) by
// picking a covering face per rune instead of drawing .notdef boxes.
package sysfont

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"

	"github.com/stubbedev/lys-ui/internal/uifont"
	"github.com/stubbedev/lys-ui/render"
)

// quietLogger discards fontscan warnings.
type quietLogger struct{}

// Printf implements fontscan.Logger.
func (quietLogger) Printf(string, ...any) {}

// regAspect is the regular aspect every default resolution asks for.
var regAspect = font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal}

// The process-wide font store: one fontscan scan, one face cache. The
// FontMap is stateful (query, script, LRU) and not safe for concurrent
// use, so every lookup holds the mutex; like render.Typeface, the
// resolved fonts are for single-threaded UI use.
var (
	storeOnce sync.Once
	storeFM   *fontscan.FontMap
	storeErr  error
	storeMu   sync.Mutex
	faceCache = map[*font.Face]*render.Typeface{}
	// directFaces caches the faces loaded outside the matcher's
	// coverage filter (see lookup); a nil value marks an unreadable
	// file so failures are not retried on every lookup.
	directFaces = map[fontscan.Location]*render.Typeface{}
)

// fontmap builds the shared store once; the first error sticks.
func fontmap() (*fontscan.FontMap, error) {
	storeOnce.Do(func() {
		fm := fontscan.NewFontMap(quietLogger{})
		// An empty cache path lets fontscan pick its platform default.
		if err := fm.UseSystemFonts(""); err != nil {
			storeErr = fmt.Errorf("sysfont: scan system fonts: %w", err)
			return
		}
		for _, f := range []struct {
			family string
			data   []byte
		}{{uifont.Family, uifont.Sans}, {uifont.MonoFamily, uifont.Mono}} {
			if err := fm.AddFont(bytes.NewReader(f.data), "embedded:"+f.family, f.family); err != nil {
				storeErr = fmt.Errorf("sysfont: register embedded %s: %w", f.family, err)
				return
			}
		}
		storeFM = fm
	})
	return storeFM, storeErr
}

// lookup resolves family+aspect to a face covering r, shaped under the
// given script hint (0 for none). The CSS matcher applies fallback
// rules, so the result honors the user's font configuration; an empty
// database resolves to nil and errors.
func lookup(family string, aspect font.Aspect, script language.Script, r rune) (*render.Typeface, error) {
	fm, err := fontmap()
	if err != nil {
		return nil, err
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	fm.SetQuery(fontscan.Query{Families: []string{family}, Aspect: aspect})
	fm.SetScript(script)
	face := fm.ResolveFace(r)
	if face == nil {
		return nil, fmt.Errorf("sysfont: no %s face found", family)
	}
	// A per-rune lookup (script hint set) intentionally crosses
	// families: return whatever covering face the matcher found. A
	// family lookup probes with 'x', and the matcher filters its
	// candidates by the probe rune's coverage - a filter that symbol
	// faces (color emoji, icon fonts, no latin glyphs) can never
	// pass. When such a lookup fell through to another family and
	// the exact family is installed, load its face directly; a family
	// the store knows only by another of its names (see aliasIndex)
	// loads its closest face to the aspect.
	if script == 0 && font.NormalizeFamily(face.Describe().Family) != font.NormalizeFamily(family) {
		if direct := faceForFamily(family); direct != nil {
			return direct, nil
		}
		if faces := aliasFaces(family); len(faces) > 0 {
			if direct := directFace(closestFace(faces, aspect).loc); direct != nil {
				return direct, nil
			}
		}
	}
	return typeface(face)
}

// faceForFamily loads the first system face of family outside the
// matcher's coverage filter. Caller holds storeMu.
// nil when the family is not installed or its file is unreadable.
func faceForFamily(family string) *render.Typeface {
	loc, ok := storeFM.FindSystemFont(family)
	if !ok {
		return nil
	}
	return directFace(loc)
}

// directFace loads the system face at loc, caching per location (a
// collection's faces share a file). Caller holds storeMu. nil when
// the file is unreadable.
func directFace(loc fontscan.Location) *render.Typeface {
	if tf, ok := directFaces[loc]; ok {
		return tf
	}
	tf, err := loadFaceFile(loc.File, loc.Index)
	if err != nil {
		tf = nil // cache the failure
	}
	directFaces[loc] = tf
	return tf
}

// loadFaceFile parses and wraps the face at path:index.
func loadFaceFile(path string, index uint16) (*render.Typeface, error) {
	face, err := faceFromFile(path, index)
	if err != nil {
		return nil, err
	}
	return render.NewTypeface(face)
}

// faceFromFile parses a font file into the face at index (0 for plain
// single-font files). ParseTTC handles plain files too. The path comes
// from fontscan's system font index, never from user input.
func faceFromFile(path string, index uint16) (*font.Face, error) {
	f, err := os.Open(path) //nolint:gosec // path is fontscan's trusted font index, not user input
	if err != nil {
		return nil, err
	}
	defer f.Close()
	faces, err := font.ParseTTC(f)
	if err != nil {
		return nil, err
	}
	if int(index) >= len(faces) {
		return nil, fmt.Errorf("sysfont: %s has no face %d", path, index)
	}
	return faces[index], nil
}

// typeface wraps face in a render.Typeface, sharing one instance per
// underlying face so chains can group runs by identity and shaping
// state stays warm.
func typeface(face *font.Face) (*render.Typeface, error) {
	if tf, ok := faceCache[face]; ok {
		return tf, nil
	}
	tf, err := render.NewTypeface(face)
	if err != nil {
		return nil, err
	}
	faceCache[face] = tf
	return tf, nil
}

// Sans returns the system's default sans-serif face, resolved with the
// same rules a browser applies to font-family: sans-serif.
func Sans() (*render.Typeface, error) {
	return lookup(fontscan.SansSerif, regAspect, 0, 'x')
}

// UI returns the face lys-ui's default look is drawn in: Geist,
// embedded in the binary, so every desktop renders the same UI face.
func UI() (*render.Typeface, error) { return embedded(uifont.Family) }

// UIMono returns the default look's monospace face: Geist Mono,
// embedded beside UI.
func UIMono() (*render.Typeface, error) { return embedded(uifont.MonoFamily) }

// embedded resolves an embedded family by name. The store registers
// them at its first use, so a resolution to another family is a store
// defect, not a missing install, and errors.
func embedded(family string) (*render.Typeface, error) {
	tf, err := lookup(family, regAspect, 0, 'x')
	if err != nil {
		return nil, err
	}
	if got := tf.Family(); got != family {
		return nil, fmt.Errorf("sysfont: embedded %s resolved to %s", family, got)
	}
	return tf, nil
}

// Monospace returns the system's default monospace face.
func Monospace() (*render.Typeface, error) {
	return lookup(fontscan.Monospace, regAspect, 0, 'x')
}

// Serif returns the system's default serif face.
func Serif() (*render.Typeface, error) {
	return lookup(fontscan.Serif, regAspect, 0, 'x')
}

// FaceFile loads a font file's first face — the @font-face loader's
// entry point for app fonts declared in stylesheets. The path is the
// stylesheet's own src, app input like every stylesheet path.
func FaceFile(path string) (*render.Typeface, error) {
	return loadFaceFile(path, 0)
}

// Best resolves the family a widget asks for: any installed family
// name (symbol and emoji faces included, even though they lack latin
// coverage), or a CSS generic - sans-serif, serif, monospace. Size is
// the pixel size the face will shape at; resolution itself is
// size-independent, because outlines scale at shape time and the
// rasterizer scales to the canvas's device scale (see render.Canvas).
// A non-positive size is a caller bug and errors.
func Best(family string, size float64) (*render.Typeface, error) {
	if size <= 0 {
		return nil, fmt.Errorf("sysfont: Best(%q, %v): size must be positive", family, size)
	}
	return lookup(family, regAspect, 0, 'x')
}

// Variant resolves a bold, italic, or bold-italic face of the same
// family base was resolved from, with the CSS matcher's fallback: a
// family without the requested face returns its closest face, usually
// the regular one. RichLabel wiring compares identity with base and
// downgrades to plain rendering when they match.
func Variant(base *render.Typeface, bold, italic bool) (*render.Typeface, error) {
	aspect := font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal}
	if bold {
		aspect.Weight = font.WeightBold
	}
	if italic {
		aspect.Style = font.StyleItalic
	}
	tf, err := lookup(base.Family(), aspect, 0, 'x')
	if err != nil {
		return nil, fmt.Errorf("sysfont: no %s variant (bold=%v, italic=%v) found", base.Family(), bold, italic)
	}
	return instantiate(tf, aspect), nil
}

// instantiate serves a variable face at the requested aspect: the
// store matches a variable family as one face at its default instance,
// so a weight it does not describe is set on the wght axis (and italic
// on ital) - the family's real weight, as GTK renders it, instead of
// the default instance. Static faces return unchanged.
func instantiate(tf *render.Typeface, aspect font.Aspect) *render.Typeface {
	var vs []render.Variation
	desc := tf.Describe().Aspect
	if desc.Weight != aspect.Weight && tf.HasAxis("wght") {
		vs = append(vs, render.Variation{Tag: "wght", Value: float32(aspect.Weight)})
	}
	if aspect.Style == font.StyleItalic && desc.Style != font.StyleItalic && tf.HasAxis("ital") {
		vs = append(vs, render.Variation{Tag: "ital", Value: 1})
	}
	if len(vs) == 0 {
		return tf
	}
	return tf.WithVariations(vs...)
}

// Weighted resolves family at a CSS numeric weight (100-900) and
// style, with the same closest-face fallback as Variant: a family
// without that weight returns its nearest face. Size follows Best.
func Weighted(family string, size float64, weight int, italic bool) (*render.Typeface, error) {
	if size <= 0 {
		return nil, fmt.Errorf("sysfont: Weighted(%q, %v): size must be positive", family, size)
	}
	if weight < 1 || weight > 1000 {
		return nil, fmt.Errorf("sysfont: Weighted(%q): weight %d outside 1-1000", family, weight)
	}
	aspect := font.Aspect{Style: font.StyleNormal, Weight: font.Weight(weight)}
	if italic {
		aspect.Style = font.StyleItalic
	}
	tf, err := lookup(family, aspect, 0, 'x')
	if err != nil {
		return nil, err
	}
	return instantiate(tf, aspect), nil
}

// Fallback returns a chain that shapes with base and, for runes it
// lacks, first the standard Linux fallback families fontconfig itself
// prefers (Noto Sans CJK for Han, Noto Color Emoji for emoji), then
// the system store per rune and script. Pass the chain anywhere a
// render.Font is expected; a base with full coverage never consults
// the fallbacks. Runes nothing covers render .notdef, as before.
func Fallback(base *render.Typeface) *render.Chain {
	family := base.Family()
	chain := render.NewChain(base, preferenceFaces()...)
	return chain.WithResolver(func(r rune) *render.Typeface {
		tf, err := lookup(family, regAspect, language.LookupScript(r), r)
		if err != nil {
			return nil
		}
		return tf
	})
}

// preferenceFaces returns the installed faces of the standard Linux
// fallback families, in preference order. Their proper CJK and color
// emoji glyphs beat whatever else the store's script fallback digs up
// (bitmap-style Unifont, monochrome emoji outlines).
func preferenceFaces() []*render.Typeface {
	if _, err := fontmap(); err != nil {
		return nil
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	var out []*render.Typeface
	for _, fam := range []string{"Noto Sans CJK JP", "Noto Color Emoji"} {
		if tf := faceForFamily(fam); tf != nil {
			out = append(out, tf)
		}
	}
	return out
}

// FallbackFamilies returns the standard Linux fallback family names
// Fallback chains prefer, reporting only the ones installed, in
// preference order. Diagnostics (the doctor block) use it to show
// what mixed-script text actually falls back to.
func FallbackFamilies() []string {
	var out []string
	for _, tf := range preferenceFaces() {
		out = append(out, tf.Family())
	}
	return out
}

// familiesOnce caches the enumerated family list: one index re-read
// per process, whatever asks first.
var (
	familiesOnce sync.Once
	familiesList []string
	familiesErr  error
)

// FamilyKey is the key Families lists a family under: name
// normalized the way the matcher normalizes it (lowercased, spaces and
// separators dropped), so "JetBrains Mono" finds "jetbrainsmono".
func FamilyKey(name string) string { return font.NormalizeFamily(name) }

// Families enumerates the system store's installed font families,
// sorted and deduped: the normalized names the matcher - and Best -
// accept, the set a font chooser offers. The store's own scan shares
// the disk index this reads.
func Families() ([]string, error) {
	familiesOnce.Do(func() {
		if _, err := fontmap(); err != nil {
			familiesErr = err
			return
		}
		footprints, err := fontscan.SystemFonts(quietLogger{}, "")
		if err != nil {
			familiesErr = fmt.Errorf("sysfont: enumerate families: %w", err)
			return
		}
		seen := make(map[string]bool, len(footprints))
		familiesList = make([]string, 0, len(footprints))
		for _, fp := range footprints {
			if fp.Family == "" || seen[fp.Family] {
				continue
			}
			seen[fp.Family] = true
			familiesList = append(familiesList, fp.Family)
		}
		slices.Sort(familiesList)
	})
	return familiesList, familiesErr
}

// FamilyDisplay returns family's display-cased name, read from one of
// its faces (cached with the face itself); family unchanged when no
// face loads. Chooser rows and previews call this lazily, so a long
// family list never parses more than the faces actually shown.
func FamilyDisplay(family string) string {
	tf, err := lookup(family, regAspect, 0, 'x')
	if err != nil {
		return family
	}
	return tf.Family()
}
