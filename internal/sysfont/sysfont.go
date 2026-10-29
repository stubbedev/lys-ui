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

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/typesetting/font"
	"github.com/stubbedev/lys-lib/third_party/typesetting/font/opentype"
	"github.com/stubbedev/lys-lib/third_party/typesetting/fontscan"
	"github.com/stubbedev/lys-lib/third_party/typesetting/language"

	"github.com/stubbedev/lys-ui/internal/uifont"
)

// quietLogger discards fontscan warnings.
type quietLogger struct{}

// Printf implements fontscan.Logger.
func (quietLogger) Printf(string, ...any) {}

// regAspect is the regular aspect every default resolution asks for.
var regAspect = font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal}

// The process-wide font store: one fontscan scan, one typeface per
// font file. The FontMap is stateful (query, script, LRU) and not safe
// for concurrent use, so every lookup holds the mutex; like
// render.Typeface, the resolved fonts are for single-threaded UI use.
var (
	storeOnce sync.Once
	storeFM   *fontscan.FontMap
	storeErr  error
	storeMu   sync.Mutex
	// typefaces holds the typeface of every face the store has handed
	// out, keyed by its file location, whichever path loaded it: a
	// family resolved by name, the matcher, or a fallback chain. A
	// file reached by two paths is parsed and wrapped once.
	typefaces = map[fontscan.Location]*render.Typeface{}
	// installed indexes the system faces by the normalized family
	// fontscan files each under.
	installed map[string][]systemFace
)

// matcherFamilies are the normalized families only the CSS matcher
// resolves: the generics, which name no face, and the embedded
// families, which the store holds as its own faces.
var matcherFamilies = func() map[string]bool {
	out := map[string]bool{}
	for _, f := range []string{
		fontscan.SansSerif, fontscan.Serif, fontscan.Monospace, fontscan.Cursive,
		fontscan.Fantasy, fontscan.Math, fontscan.Emoji, uifont.Family, uifont.MonoFamily,
	} {
		out[font.NormalizeFamily(f)] = true
	}
	return out
}()

// fontmap builds the shared store once; the first error sticks.
func fontmap() (*fontscan.FontMap, error) {
	storeOnce.Do(func() {
		fm := fontscan.NewFontMap(quietLogger{})
		if _, err := fontscan.DefaultFontDirectories(quietLogger{}); err == nil {
			// An empty cache path lets fontscan pick its platform default.
			if err := fm.UseSystemFonts(""); err != nil {
				storeErr = fmt.Errorf("sysfont: scan system fonts: %w", err)
				return
			}
			footprints, err := fontscan.SystemFonts(quietLogger{}, "")
			if err != nil {
				storeErr = fmt.Errorf("sysfont: index system fonts: %w", err)
				return
			}
			installed = indexFamilies(footprints)
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

// indexFamilies groups footprints by their family.
func indexFamilies(footprints []fontscan.Footprint) map[string][]systemFace {
	index := make(map[string][]systemFace, len(footprints))
	for _, fp := range footprints {
		index[fp.Family] = append(index[fp.Family], systemFace{fp.Location, fp.Aspect})
	}
	return index
}

// lookup resolves family at aspect. An installed family, known to
// the store by that name or by any other it declares (see
// aliasIndex), loads its closest face to aspect directly: the
// matcher filters candidates by the probe rune's coverage, a filter
// symbol faces (color emoji, icon fonts, no latin glyphs) never
// pass, and a family it does not know by that name falls through to
// a substitute it parses only for lookup to discard. Generics, the
// embedded families and families not installed go through the CSS
// matcher, whose fallback rules honor the user's font configuration;
// an empty database errors.
func lookup(family string, aspect font.Aspect) (*render.Typeface, error) {
	fm, err := fontmap()
	if err != nil {
		return nil, err
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	if faces := installedFaces(family); len(faces) > 0 {
		return typefaceAt(fm, closestFace(faces, aspect).loc)
	}
	fm.SetQuery(fontscan.Query{Families: []string{family}, Aspect: aspect})
	fm.SetScript(0)
	face := fm.ResolveFace('x')
	if face == nil {
		return nil, fmt.Errorf("sysfont: no %s face found", family)
	}
	if font.NormalizeFamily(family) == fontscan.Monospace {
		face = monospaced(fm, face, aspect, 'x')
	}
	return typeface(fm, face)
}

// installedFaces returns the system faces of family, nil for a family
// the matcher resolves (see matcherFamilies) or one not installed.
// Caller holds storeMu.
func installedFaces(family string) []systemFace {
	key := font.NormalizeFamily(family)
	if matcherFamilies[key] {
		return nil
	}
	if faces := installed[key]; len(faces) > 0 {
		return faces
	}
	return aliasFaces(key)
}

// monospaced keeps face, the store's answer to the monospace generic,
// when it is monospaced, else resolves the embedded mono instead: the
// generic falls through to any face when no family the matcher knows
// as monospace is installed, and text set in monospace must line up.
// Caller holds storeMu.
func monospaced(fm *fontscan.FontMap, face *font.Face, aspect font.Aspect, r rune) *font.Face {
	if face.IsMonospace() {
		return face
	}
	fm.SetQuery(fontscan.Query{Families: []string{uifont.MonoFamily}, Aspect: aspect})
	if mono := fm.ResolveFace(r); mono != nil {
		return mono
	}
	return face
}

// typefaceAt returns the typeface of the system face at loc, parsing
// the file on its first use. A parsed face is handed to fm, filed
// under its own location and description, so a later match or
// fallback that resolves to the same file reuses it instead of
// parsing the file again. Caller holds storeMu.
func typefaceAt(fm *fontscan.FontMap, loc fontscan.Location) (*render.Typeface, error) {
	if tf, ok := typefaces[loc]; ok {
		return tf, nil
	}
	face, err := faceFromFile(loc.File, loc.Index)
	if err != nil {
		return nil, err
	}
	fm.AddFace(face, loc, face.Describe())
	return remember(loc, face)
}

// typeface returns the typeface of face, a face fm resolved, sharing
// the one already handed out for its file. Caller holds storeMu.
func typeface(fm *fontscan.FontMap, face *font.Face) (*render.Typeface, error) {
	loc := fm.FontLocation(face.Font)
	if tf, ok := typefaces[loc]; ok {
		return tf, nil
	}
	return remember(loc, face)
}

// remember wraps face, loaded from loc, and caches the typeface.
// Caller holds storeMu.
func remember(loc fontscan.Location, face *font.Face) (*render.Typeface, error) {
	tf, err := render.NewTypeface(face)
	if err != nil {
		return nil, err
	}
	typefaces[loc] = tf
	return tf, nil
}

// faceFromFile parses the face at index of a font file (0 for plain
// single-font files). Only that face is read: a collection's other
// faces (a CJK .ttc holds ten) would each copy every table. The path
// comes from fontscan's system font index, never from user input.
func faceFromFile(path string, index uint16) (*font.Face, error) {
	f, err := os.Open(path) //nolint:gosec // path is fontscan's trusted font index, not user input
	if err != nil {
		return nil, err
	}
	defer f.Close()
	loaders, err := opentype.NewLoaders(f)
	if err != nil {
		return nil, fmt.Errorf("sysfont: read %s: %w", path, err)
	}
	if int(index) >= len(loaders) {
		return nil, fmt.Errorf("sysfont: %s has no face %d", path, index)
	}
	ft, err := font.NewFont(loaders[index])
	if err != nil {
		return nil, fmt.Errorf("sysfont: face %d of %s: %w", index, path, err)
	}
	return font.NewFace(ft), nil
}

// Sans returns the system's default sans-serif face, resolved with the
// same rules a browser applies to font-family: sans-serif.
func Sans() (*render.Typeface, error) {
	return lookup(fontscan.SansSerif, regAspect)
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
	tf, err := lookup(family, regAspect)
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
	return lookup(fontscan.Monospace, regAspect)
}

// Serif returns the system's default serif face.
func Serif() (*render.Typeface, error) {
	return lookup(fontscan.Serif, regAspect)
}

// FaceFile loads a font file's first face — the @font-face loader's
// entry point for app fonts declared in stylesheets. The path is the
// stylesheet's own src, app input like every stylesheet path.
func FaceFile(path string) (*render.Typeface, error) {
	face, err := faceFromFile(path, 0)
	if err != nil {
		return nil, err
	}
	return render.NewTypeface(face)
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
	return lookup(family, regAspect)
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
	tf, err := lookup(base.Family(), aspect)
	if err != nil {
		return nil, fmt.Errorf("sysfont: no %s variant (bold=%v, italic=%v) found", base.Family(), bold, italic)
	}
	return instantiate(tf, aspect), nil
}

// instantiate serves a variable face at the requested aspect: the
// store matches a variable family as one face at its default instance,
// so a weight it does not describe is set on the wght axis (and italic
// on ital) - the family's real weight, as GTK renders it, instead of
// the default instance. An italic the family has no face or axis for
// is a synthetic oblique; static faces otherwise return unchanged.
func instantiate(tf *render.Typeface, aspect font.Aspect) *render.Typeface {
	var vs []render.Variation
	desc := tf.Describe().Aspect
	if desc.Weight != aspect.Weight && tf.HasAxis("wght") {
		vs = append(vs, render.Variation{Tag: "wght", Value: float32(aspect.Weight)})
	}
	wantsItalic := aspect.Style == font.StyleItalic && desc.Style != font.StyleItalic
	if wantsItalic && tf.HasAxis("ital") {
		vs = append(vs, render.Variation{Tag: "ital", Value: 1})
	}
	if len(vs) > 0 {
		tf = tf.WithVariations(vs...)
	}
	if wantsItalic && !tf.HasAxis("ital") {
		// No italic face and no ital axis: a synthetic oblique, as CSS
		// synthesizes one by default (font-synthesis: style).
		tf = tf.Oblique()
	}
	return tf
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
	tf, err := lookup(family, aspect)
	if err != nil {
		return nil, err
	}
	return instantiate(tf, aspect), nil
}

// fallbackFamilies are the standard Linux fallback families
// fontconfig itself prefers, in order: their proper CJK and color emoji
// glyphs beat whatever else the store's script fallback digs up
// (bitmap-style Unifont, monochrome emoji outlines).
var fallbackFamilies = []string{"Noto Sans CJK JP", "Noto Color Emoji"}

// Fallback returns a chain that shapes with base and, for runes it
// lacks, a face from the system store: the first of base's family and
// the standard fallback families (fallbackFamilies) that covers the
// rune, else the store's own pick for its script. The matcher picks
// from the store's coverage index, so only a face some rune needs is
// ever loaded: text in one script never pulls in a CJK collection or
// an emoji font. Pass the chain anywhere a render.Font is expected; a
// base with full coverage never consults the store. Runes nothing
// covers render .notdef, as before.
func Fallback(base *render.Typeface) *render.Chain {
	families := append([]string{base.Family()}, fallbackFamilies...)
	return render.NewChain(base).WithResolver(func(r rune) *render.Typeface {
		tf, err := fallbackFace(families, r)
		if err != nil {
			return nil
		}
		return tf
	})
}

// fallbackFace resolves the store's face for r under families, with
// r's script as the matcher's hint. The face need not cover r: the
// matcher returns an arbitrary face when nothing does.
func fallbackFace(families []string, r rune) (*render.Typeface, error) {
	fm, err := fontmap()
	if err != nil {
		return nil, err
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	fm.SetQuery(fontscan.Query{Families: families, Aspect: regAspect})
	fm.SetScript(language.LookupScript(r))
	face := fm.ResolveFace(r)
	if face == nil {
		return nil, fmt.Errorf("sysfont: no face for %U", r)
	}
	return typeface(fm, face)
}

// FallbackFamilies returns the standard Linux fallback family names
// Fallback chains prefer, reporting only the ones installed, in
// preference order. Diagnostics (the doctor block) use it to show
// what mixed-script text falls back to; it loads no face.
func FallbackFamilies() []string {
	if _, err := fontmap(); err != nil {
		return nil
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	var out []string
	for _, fam := range fallbackFamilies {
		if _, ok := storeFM.FindSystemFont(fam); ok {
			out = append(out, fam)
		}
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
	tf, err := lookup(family, regAspect)
	if err != nil {
		return family
	}
	return tf.Family()
}
