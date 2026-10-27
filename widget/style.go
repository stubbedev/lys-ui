// The CSS layer (docs/css.md): classes, ids, states, and inline styles
// on the shared node; prioritized stylesheets with hot reload; and the
// per-widget computed style the render path reads where it reads Theme
// fields.
//
// The data flow is one direction. AddStylesheet and LoadStylesheet
// parse once and install the layer; a style generation stamp goes stale
// everywhere at once. Each widget's cascade is computed lazily on first
// read after an invalidation — never per paint — and the diff against
// the previous values drives the smallest correct invalidation: repaint
// the widget, relayout it when a layout-affecting property moved, and
// mark the subtree when an inherited one (a custom property included)
// did. A state, class, or id flip restyles the widget alone unless a
// loaded selector reads that fact left of a combinator; then the
// subtree (and, for sibling selectors, the neighbors) restyle too. With
// no stylesheet loaded the cascade is empty, every consumer falls
// through to its theme-derived value, and the pixels are exactly the
// pre-CSS ones.
package widget

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/internal/sysfont"
)

// Style priorities, GTK's provider priorities: a higher priority wins
// over any specificity below it; equal priorities fall to specificity,
// then to load order.
const (
	// StylePriorityFallback is for defaults below everything.
	StylePriorityFallback = style.PriorityFallback
	// StylePriorityTheme is for a theme's stylesheet.
	StylePriorityTheme = style.PriorityTheme
	// StylePrioritySettings is for settings-derived rules.
	StylePrioritySettings = style.PrioritySettings
	// StylePriorityApplication is the application's own stylesheet;
	// LoadStylesheet installs at it.
	StylePriorityApplication = style.PriorityApplication
	// StylePriorityUser is for user overrides, and the default priority
	// of a widget's inline style.
	StylePriorityUser = style.PriorityUser
)

// Stylesheet is one parsed stylesheet at a priority, reloadable in
// place. Installed with AddStylesheet it applies to every widget;
// created with NewStylesheet and attached to widgets
// (AttachStylesheet) it applies to those subtrees only — one surface's
// styling that must not reach another's. The zero value is not usable.
type Stylesheet struct {
	sheet    *style.Sheet
	priority int
	live     bool
	// scoped marks a NewStylesheet sheet: it styles only the subtrees it
	// is attached to and is never installed process-wide.
	scoped bool
	// attached counts the widgets the sheet is attached to.
	attached int
	// files are the file the sheet was loaded from and the files its
	// @import rules loaded, for the hot-reload poll; nil for a
	// string-loaded sheet. Guarded by watchMu.
	files []styleFile
}

// sheets is every installed stylesheet in install order; layersNow is
// the snapshot the cascade reads, rebuilt on every change. Both are
// loop-goroutine state, like the widget tree.
var (
	sheets    []*Stylesheet
	layersNow []style.Layer
	sensNow   sensitivity
)

// sensitivity is the union of the loaded sheets' selector summaries.
type sensitivity struct {
	states  style.State
	classes map[string]bool
	// structural and siblingFacts are style.Sensitivity's Structural
	// and SiblingFacts.
	structural, siblingFacts bool
	// media reports whether any loaded sheet carries @media or
	// @container rules: the viewport and preference setters restyle
	// only then (containers resize with the window).
	media bool
	// containers reports @container rules or container units: a query
	// container's resize restyles then.
	containers bool
	// has reports a :has() selector: a descendant or following
	// sibling's facts feed an ancestor's match, so class and state
	// changes restyle up the tree too.
	has bool
	// pseudo reports a ::before/::after rule: restyle then computes the
	// pseudo-element styles as well.
	pseudo bool
	// counters reports counter-reset/counter-increment in play: restyle
	// then chains the previous sibling's counter scope.
	counters bool
}

// styleGen bumps on every stylesheet (re)load and environment change.
// Every widget's computed style and measure cache compare their stamps
// against it, so a load restyles and relayouts the whole tree without
// walking it, and the damage collector repaints each widget once (the
// themeGen pattern).
var styleGen uint64 = 1

// NewStylesheet parses css into a stylesheet at priority without
// installing it: attach it to the widgets whose subtrees it styles.
func NewStylesheet(css string, priority int) *Stylesheet {
	return &Stylesheet{sheet: style.Parse(css), priority: priority, scoped: true}
}

// SetParseWarn installs f as the CSS engine's parse-warning sink; nil
// restores the library logger. One sink per process. Hosts that want
// stylesheet problems visible — a settings app showing theme errors —
// route the messages to their log or interface.
func SetParseWarn(f func(msg string)) {
	style.SetParseWarn(f)
}

// AddStylesheet parses css and installs it at priority, above or below
// the other stylesheets by priority and, at equal priority, after every
// earlier one. The next frame repaints with it; call it from the
// event-loop goroutine, like SetTheme. Malformed rules are skipped with
// a warning on the library logger — the valid remainder still applies.
func AddStylesheet(css string, priority int) *Stylesheet {
	s := &Stylesheet{sheet: style.Parse(css), priority: priority, live: true}
	sheets = append(sheets, s)
	relayer()
	return s
}

// Load replaces the stylesheet's rules with css, keeping its priority
// and position, and stops its file poll: the sheet is string-backed
// from here on. Loading into an installed stylesheet that was removed
// (and is attached nowhere) reinstalls it; a NewStylesheet sheet stays
// scoped, styling only the subtrees it is attached to.
func (s *Stylesheet) Load(css string) {
	s.sheet = style.Parse(css)
	unwatch(s)
	if !s.live && !s.scoped && s.attached == 0 {
		s.live = true
		sheets = append(sheets, s)
	}
	relayer()
}

// Remove uninstalls the stylesheet and stops its file poll (a scoped
// NewStylesheetFile sheet keeps styling where it is attached, but no
// longer reloads); removing twice is a no-op.
func (s *Stylesheet) Remove() {
	unwatch(s)
	if !s.live {
		return
	}
	s.live = false
	sheets = slices.DeleteFunc(sheets, func(o *Stylesheet) bool { return o == s })
	relayer()
}

// Priority returns the priority the stylesheet was installed at.
func (s *Stylesheet) Priority() int { return s.priority }

// relayer rebuilds the cascade snapshot and the sensitivity union and
// stamps every widget stale.
func relayer() {
	resensitize()
	bumpStyleGen("stylesheet")
}

// resensitize rebuilds the cascade snapshot and the sensitivity union
// from the stylesheets in use, installed and attached, and loads their
// font faces. It restyles nothing.
func resensitize() {
	layersNow = layersNow[:0]
	sensNow = sensitivity{}
	for _, s := range sheets {
		layersNow = append(layersNow, style.Layer{Sheet: s.sheet, Priority: s.priority})
	}
	for _, s := range append(slices.Clone(sheets), scopedSheets...) {
		if s.sheet.ContainerQueries() > 0 || s.sheet.Sensitivity().CqUnits {
			sensNow.media, sensNow.containers = true, true
		}
		if s.sheet.MediaQueries() > 0 {
			sensNow.media = true
		}
		sn := s.sheet.Sensitivity()
		sensNow.states |= sn.AncestorStates
		sensNow.structural = sensNow.structural || sn.Structural
		sensNow.siblingFacts = sensNow.siblingFacts || sn.SiblingFacts
		sensNow.has = sensNow.has || sn.HasRelational
		sensNow.pseudo = sensNow.pseudo || sn.Pseudo
		sensNow.counters = sensNow.counters || sn.Counters
		for c := range sn.AncestorClasses {
			if sensNow.classes == nil {
				sensNow.classes = map[string]bool{}
			}
			sensNow.classes[c] = true
		}
	}
	loadFontFaces()
}

// scopedSheets are the stylesheets attached to at least one widget.
var scopedSheets []*Stylesheet

// AttachStylesheet applies s to this widget and its descendants, on top
// of the installed stylesheets (a tie at equal priority and specificity
// goes to the attached one). Attaching the same stylesheet twice is a
// no-op; one stylesheet can style many subtrees, and a Load restyles
// them all.
func (n *node) AttachStylesheet(s *Stylesheet) {
	checkLoop("AttachStylesheet")
	if slices.Contains(n.sheets, s) {
		return
	}
	n.sheets = append(n.sheets, s)
	if s.attached++; s.attached == 1 {
		scopedSheets = append(scopedSheets, s)
		resensitize()
	}
	n.restyleTree()
}

// DetachStylesheet stops s from styling this subtree; detaching one
// that is not attached is a no-op.
func (n *node) DetachStylesheet(s *Stylesheet) {
	checkLoop("DetachStylesheet")
	i := slices.Index(n.sheets, s)
	if i < 0 {
		return
	}
	n.sheets = slices.Delete(n.sheets, i, i+1)
	if s.attached--; s.attached == 0 {
		scopedSheets = slices.DeleteFunc(scopedSheets, func(o *Stylesheet) bool { return o == s })
		resensitize()
	}
	n.restyleTree()
}

// restyleTree restyles n and its subtree, after a change only they
// read (a scoped stylesheet coming or going, their window's viewport);
// every other tree keeps its styles. A widget never styled and in no
// tree needs nothing: its first cascade reads the change.
func (n *node) restyleTree() {
	if n.unseen() {
		return
	}
	n.markStyleDirty()
	restyleSubtree(n.self)
}

// layerBuf is layersFor's reusable buffer (loop-goroutine only).
var layerBuf []style.Layer

// layersFor returns the cascade layers for n: the installed sheets,
// then the sheets attached to n and its ancestors, outermost first.
func layersFor(n *node) []style.Layer {
	if len(scopedSheets) == 0 {
		return layersNow
	}
	layerBuf = append(layerBuf[:0], layersNow...)
	mark := len(layerBuf)
	for p := n; p != nil; p = styleParentNodeOf(p) {
		for _, s := range slices.Backward(p.sheets) {
			layerBuf = append(layerBuf, style.Layer{Sheet: s.sheet, Priority: s.priority})
		}
	}
	slices.Reverse(layerBuf[mark:])
	return layerBuf
}

// appSheet is the stylesheet LoadStylesheet manages.
var appSheet *Stylesheet

// LoadStylesheet parses css and installs it as the application
// stylesheet (StylePriorityApplication), replacing whatever
// LoadStylesheet loaded before. An empty string removes it. Other
// stylesheets added with AddStylesheet are untouched.
func LoadStylesheet(css string) {
	if css == "" {
		if appSheet != nil {
			appSheet.Remove()
		}
		return
	}
	if appSheet == nil {
		appSheet = AddStylesheet(css, StylePriorityApplication)
		return
	}
	appSheet.Load(css)
}

// styleEnv is the unit environment: what 1rem is, and the font size a
// widget computes when nothing in its ancestry sets one.
var styleEnv = style.Env{Rem: 16, FontPx: 16}

// SetRootFontSize sets the root font size stylesheet units resolve
// against: 1rem, and the default font size (CSS's `medium`) of widgets
// nothing sets one for. The default is 16 logical pixels, the CSS
// default; GTK derives its own from the desktop font setting. The whole
// tree restyles.
func SetRootFontSize(px float64) {
	if px <= 0 || px == styleEnv.Rem {
		return
	}
	styleEnv = style.Env{Rem: px, FontPx: px}
	bumpStyleGen("root font size")
}

// RootFontSize returns the current root font size.
func RootFontSize() float64 { return styleEnv.Rem }

// mediaViewport is the @media viewport (SetMediaViewport) a tree
// computes against when its style root reports none of its own
// (SetMediaViewportFor). mediaContrast and mediaDPI are the appearance
// inputs SetTheme cannot derive: prefers-contrast and resolution. All
// loop-goroutine state, like styleEnv.
var (
	mediaViewport struct{ w, h int }
	mediaContrast bool
	mediaDPI      = 96.0
)

// SetMediaViewport sets the size @media width/height/aspect-ratio/
// orientation conditions evaluate against for trees whose window
// reported no viewport of its own — the window's logical size, which
// the app layer feeds from its resize callback. The whole tree
// restyles when the size changes. Call it from the event-loop
// goroutine, like SetTheme.
func SetMediaViewport(w, h int) {
	if w == mediaViewport.w && h == mediaViewport.h {
		return
	}
	mediaViewport.w, mediaViewport.h = w, h
	if !sensNow.media {
		return
	}
	bumpStyleGen("media viewport")
}

// SetMediaViewportFor sets the @media viewport of one window's tree:
// root is the widget SetStyleRoot marked (the app layer passes each
// window's content root), and every widget below it computes @media
// conditions, the vw/vh family, and container units against this
// size, so two windows of one application style independently off
// one stylesheet. A tree whose root reports no viewport falls back to
// SetMediaViewport's global value; 0×0 clears the tree's own size.
func SetMediaViewportFor(root Widget, w, h int) {
	n := nodeOf(root)
	if n == nil {
		SetMediaViewport(w, h)
		return
	}
	if n.viewport.w == w && n.viewport.h == h {
		return
	}
	n.viewport.w, n.viewport.h = w, h
	if sensNow.media {
		n.restyleTree()
	}
}

// MediaViewportOf returns the @media viewport w's tree computes
// against: its style root's own size, else the global one; 0×0 until
// either is set.
func MediaViewportOf(w Widget) (int, int) {
	vp := mediaViewport
	if n := nodeOf(w); n != nil {
		vp = rootViewport(styleRootOf(n))
	}
	return vp.w, vp.h
}

// SetMediaContrast turns the prefers-contrast media feature on or
// off; the desktop's contrast preference, which no Theme field
// carries. The whole tree restyles on a change.
func SetMediaContrast(on bool) {
	if on == mediaContrast {
		return
	}
	mediaContrast = on
	if !sensNow.media {
		return
	}
	bumpStyleGen("media contrast")
}

// SetMediaResolution sets the resolution the @media resolution feature
// reports, in dpi. The default is 96.
func SetMediaResolution(dpi float64) {
	if dpi <= 0 || dpi == mediaDPI {
		return
	}
	mediaDPI = dpi
	if !sensNow.media {
		return
	}
	bumpStyleGen("media resolution")
}

// defaultFontMetrics loads the UI face's metrics once; the
// ch/ex/cap/ic/lh units resolve against them (per-font-size ratios,
// so one face serves every size), falling back to the CSS default
// ratios when the font store cannot load.
var defaultFontMetrics = sync.OnceValue(func() style.FontMetrics {
	face, err := sysfont.UI()
	if err != nil {
		return style.FontMetrics{}
	}
	m := face.Metrics()
	return style.FontMetrics{Ch: m.Ch, Ex: m.Ex, Cap: m.Cap, Ic: m.Ic, LineHeight: m.LineHeight}
})

// themeDark memoizes the active theme's polarity by its background:
// every compute reads it, and the WCAG luminance behind it linearizes
// three channels.
var themeDark struct {
	bg   render.Color
	dark bool
	ok   bool
}

// currentDark reports whether the active theme is dark.
func currentDark() bool {
	if !themeDark.ok || themeDark.bg != current.Bg {
		themeDark.bg, themeDark.dark, themeDark.ok = current.Bg, current.IsDark(), true
	}
	return themeDark.dark
}

// envNow is the compute environment: the unit state plus the @media
// environment, its appearance half derived from the active Theme
// (prefers-color-scheme from the palette's polarity, reduced motion
// from the animation gate) so a theme swap re-evaluates every
// prefers-* rule.
func envNow() style.Env {
	m := defaultFontMetrics()
	env := style.Env{Rem: styleEnv.Rem, FontPx: styleEnv.FontPx, Font: m, Media: style.Media{
		Width: float64(mediaViewport.w), Height: float64(mediaViewport.h),
		DPI: mediaDPI, Dark: currentDark(), HighContrast: mediaContrast,
		ReducedMotion: !current.Animations,
	}}
	env.RootLineHeight = env.Rem * ratioOr(m.LineHeight, 1.2)
	return env
}

// envFor is envNow for n's tree: the viewport its style root reported
// (the global value otherwise), and the root-relative units read off
// the style root's computed style — rem is its font-size and rlh its
// line-height, when it declares them (SetRootFontSize is the initial
// value). The root itself resolves them against the initial values,
// and its style is fresh here: a node's ancestry computes first.
func envFor(n *node) style.Env {
	env := envNow()
	r := styleRootOf(n)
	vp := rootViewport(r)
	env.Media.Width, env.Media.Height = float64(vp.w), float64(vp.h)
	if r == n || r.csGen != styleGen {
		return env
	}
	if r.cs.Has(style.PropFontSize) && r.cs.FontSize > 0 {
		env.Rem = r.cs.FontSize
		env.RootLineHeight = env.Rem * ratioOr(env.Font.LineHeight, 1.2)
	}
	if r.cs.Has(style.PropLineHeight) && r.cs.LineHeight > 0 {
		env.RootLineHeight = r.cs.LineHeight
	}
	return env
}

// rootViewport is the @media viewport the tree under style root r
// computes against: r's own size, else the global one.
func rootViewport(r *node) struct{ w, h int } {
	if r.viewport.w > 0 && r.viewport.h > 0 {
		return r.viewport
	}
	return mediaViewport
}

// styleRootOf is n's style root: the topmost node of its styling
// chain.
func styleRootOf(n *node) *node {
	for {
		p := styleParentNodeOf(n)
		if p == nil {
			return n
		}
		n = p
	}
}

// ratioOr falls back to the CSS default ratio when the face reports
// none.
func ratioOr(v, fallback float64) float64 {
	if v <= 0 {
		return fallback
	}
	return v
}

// styleFile is one loaded stylesheet file: what the hot-reload poll
// stats and compares against.
type styleFile struct {
	path string
	size int64
	mod  time.Time
}

// statStyleFile stats path into the styleFile the poll compares.
func statStyleFile(path string) (styleFile, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return styleFile{}, fmt.Errorf("stylesheet: stat %s: %w", path, err)
	}
	return styleFile{path: path, size: fi.Size(), mod: fi.ModTime()}, nil
}

// changed reports whether f no longer matches the file on disk.
func (f styleFile) changed() bool {
	now, err := statStyleFile(f.path)
	return err != nil || now != f
}

// watches are the file-backed stylesheets, in load order; pollStarted
// is whether the one poller is already scheduled. watchMu guards both
// and each sheet's file.
var (
	watchMu     sync.Mutex
	watches     []*Stylesheet
	pollStarted bool
)

// pollHook is the app-installed scheduler the poll rides: it runs fn
// once per second on the event-loop goroutine (app.Application.Every).
// Widget code cannot reach the loop timer itself, and the poll must
// not exist at all until a file is loaded.
var pollHook atomic.Pointer[func(fn func())]

// SetStylesheetPoller installs the scheduler the stylesheet files'
// hot-reload poll ticks on: poll schedules fn once per second on the
// event-loop goroutine, for the life of the process. One hook per
// process, like SetRemovedHook; the app installs it once at startup.
// Without a hook (no running Application) a loaded file simply never
// hot-reloads.
func SetStylesheetPoller(poll func(fn func())) {
	if poll == nil {
		pollHook.Store(nil)
		return
	}
	pollHook.Store(&poll)
}

// AddStylesheetFile loads a stylesheet from path and installs it at
// priority as its own Stylesheet, with the hot-reload poll on it: a
// stat per second reloads it on change, and Remove stops the poll. A
// file that cannot be read or whose parse reports a problem fails
// with an error naming the path, instead of half-loading.
func AddStylesheetFile(path string, priority int) (*Stylesheet, error) {
	sheet, f, err := readStylesheetFile(path)
	if err != nil {
		return nil, err
	}
	s := &Stylesheet{sheet: sheet, priority: priority, live: true}
	sheets = append(sheets, s)
	watchSheet(s, f)
	relayer()
	return s, nil
}

// NewStylesheetFile reads the stylesheet at path into a scoped sheet
// at priority, like NewStylesheet: attach it to the widgets whose
// subtrees it styles. Its @import rules resolve relative to path, and
// the hot-reload poll AddStylesheetFile starts reloads it on change
// until Remove. A file that cannot be read or whose parse reports a
// problem fails with an error naming the path.
func NewStylesheetFile(path string, priority int) (*Stylesheet, error) {
	sheet, f, err := readStylesheetFile(path)
	if err != nil {
		return nil, err
	}
	s := &Stylesheet{sheet: sheet, priority: priority, scoped: true}
	watchSheet(s, f)
	return s, nil
}

// LoadStylesheetFile loads a stylesheet from path as the application
// stylesheet (StylePriorityApplication), replacing whatever
// LoadStylesheetFile loaded before, with the same hot-reload poll
// AddStylesheetFile starts.
func LoadStylesheetFile(path string) error {
	s, err := AddStylesheetFile(path, StylePriorityApplication)
	if err != nil {
		return err
	}
	if appSheet != nil {
		appSheet.Remove()
	}
	appSheet = s
	return nil
}

// readStylesheetFile reads and parses the stylesheet at path, with
// its @import rules resolved beside it; the returned files are the
// stats the poll compares against: path's, then each import's. A
// missing or unreadable file, or one whose parse reports a problem,
// fails with an error carrying the path.
func readStylesheetFile(path string) (*style.Sheet, []styleFile, error) {
	main, err := statStyleFile(path)
	if err != nil {
		return nil, nil, err
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is the app's stylesheet argument
	if err != nil {
		return nil, nil, fmt.Errorf("stylesheet: read %s: %w", path, err)
	}
	files := []styleFile{main}
	var statErr error
	sheet, warns := style.ParseImporting(string(b), path, func(ref, base string) (string, string, error) {
		p, content, err := style.FileImport(ref, base)
		if err != nil {
			return p, content, err
		}
		f, err := statStyleFile(p)
		if err != nil {
			statErr = err
		}
		files = append(files, f)
		return p, content, nil
	})
	if len(warns) > 0 {
		return nil, nil, fmt.Errorf("stylesheet: parse %s: %s", path, strings.Join(warns, "; "))
	}
	if statErr != nil {
		return nil, nil, statErr
	}
	return sheet, files, nil
}

// watchSheet adds s to the hot-reload poll under its files, scheduling the
// poller once for the life of the process.
func watchSheet(s *Stylesheet, files []styleFile) {
	watchMu.Lock()
	defer watchMu.Unlock()
	s.files = files
	watches = append(watches, s)
	if pollStarted {
		return
	}
	pollStarted = true
	if h := pollHook.Load(); h != nil {
		(*h)(pollStylesheets)
	}
}

// unwatch takes s out of the hot-reload poll.
func unwatch(s *Stylesheet) {
	watchMu.Lock()
	defer watchMu.Unlock()
	s.files = nil
	watches = slices.DeleteFunc(watches, func(o *Stylesheet) bool { return o == s })
}

// pollStylesheets stats every watched file once and reloads the
// changed ones — the whole hot-reload cost is one stat per file per
// second. A file that vanished or no longer parses keeps its last
// good rules, reported through the parse-warning sink with its path.
func pollStylesheets() {
	watchMu.Lock()
	ws := slices.Clone(watches)
	watchMu.Unlock()
	for _, s := range ws {
		s.pollFile()
	}
}

// pollFile stats the stylesheet's files once and reloads it when any
// changed. A file that vanished or no longer parses keeps the last
// good rules, reported through the parse-warning sink with its path.
func (s *Stylesheet) pollFile() {
	watchMu.Lock()
	files := s.files
	watchMu.Unlock()
	if len(files) == 0 || !slices.ContainsFunc(files, styleFile.changed) {
		return
	}
	sheet, now, err := readStylesheetFile(files[0].path)
	if err != nil {
		style.Warn("hot reload: " + err.Error())
		return
	}
	watchMu.Lock()
	s.files = now
	watchMu.Unlock()
	s.sheet = sheet
	relayer()
}

// faceResolver resolves a stylesheet font-family/font-weight to a
// face — the app's font store (internal/sysfont) owns real families.
// Nil by default: family and weight declarations then keep the
// widget's constructor face, the same graceful no-op as an unknown
// element name.
var faceResolver atomic.Pointer[func(family string, weight int) (render.Font, bool)]

// SetFaceResolver installs the family/weight → face lookup the
// font-family and font-weight declarations shape with. One resolver
// per process; nil restores the default no-op. It is consulted at
// style-invalidation time, never per paint.
func SetFaceResolver(f func(family string, weight int) (render.Font, bool)) {
	if f == nil {
		faceResolver.Store(nil)
		return
	}
	faceResolver.Store(&f)
}

// resolveFace consults the @font-face registry first, then the
// installed resolver; ok is false when the registry has no match and
// no resolver is installed or it declines the family. An empty family
// (a font-weight with no font-family in the cascade) is base's own:
// the weight applies to the face the widget already shapes with, never
// to the resolver's idea of no family.
func resolveFace(family string, weight int, base render.Font) (render.Font, bool) {
	if family == "" {
		family = fontFamilyOf(base)
	}
	if f := fontFaceOf(family, weight); f != nil {
		return f, true
	}
	if f := faceResolver.Load(); f != nil {
		return (*f)(family, weight)
	}
	return nil, false
}

// fontFamilyOf is the family f shapes with: a typeface's own, a
// fallback chain's primary's; "" for anything else.
func fontFamilyOf(f render.Font) string {
	switch f := f.(type) {
	case *render.Typeface:
		return f.Family()
	case *render.Chain:
		if p := f.Primary(); p != nil {
			return p.Family()
		}
	}
	return ""
}

// fontFaceEntry is one loaded @font-face: the family it serves, its
// weight range, and the loaded face. The style half of the
// font-style descriptor is recorded but never matched — the
// font-family resolver's signature carries family and weight only.
type fontFaceEntry struct {
	family             string
	weightLo, weightHi int
	italic             bool
	face               render.Font
}

// fontFaceList is the loaded @font-face registry, rebuilt on every
// relayer. Loop-goroutine state, like sheets.
var fontFaceList []fontFaceEntry

// loadFontFaces loads every loaded sheet's @font-face src files and
// rebuilds the family registry the font-family resolver consults
// first. A src that cannot be read or parsed reports through the
// parse-warning sink naming the family and path, and that face alone
// drops — the same policy a failing hot reload follows.
func loadFontFaces() {
	fontFaceList = fontFaceList[:0]
	for _, s := range append(slices.Clone(sheets), scopedSheets...) {
		for _, f := range s.sheet.FontFaces() {
			tf, err := sysfont.FaceFile(f.Src)
			if err != nil {
				style.Warn(fmt.Sprintf("@font-face %s: load %s: %v", f.Family, f.Src, err))
				continue
			}
			fontFaceList = append(fontFaceList, fontFaceEntry{
				family: f.Family, weightLo: f.WeightLo, weightHi: f.WeightHi,
				italic: f.Italic, face: tf,
			})
		}
	}
}

// fontFaceOf resolves a family and weight through the @font-face
// registry: a face whose weight range holds the weight, else the
// family's first face, else nil.
func fontFaceOf(family string, weight int) render.Font {
	if weight == 0 {
		weight = 400
	}
	var first render.Font
	for _, e := range fontFaceList {
		if !strings.EqualFold(e.family, family) {
			continue
		}
		if weight >= e.weightLo && weight <= e.weightHi {
			return e.face
		}
		if first == nil {
			first = e.face
		}
	}
	return first
}

// effShadow resolves the box-shadow cascade for a floating surface:
// the stylesheet's first outer layer when it declares a shadow, else
// the theme's. Blur 0 means off. A zero color defaults like the theme's
// unset one does.
func effShadow(w Widget, t *Theme) (render.Color, int) {
	n := nodeOf(w)
	if n == nil {
		return t.shadowPaint()
	}
	v := n.style(w)
	if v.Has(style.PropBoxShadow) {
		for _, sh := range v.Shadow.List() {
			if sh.Inset || sh.Blur <= 0 {
				continue
			}
			col := sh.Color
			if col == 0 {
				col = defaultShadowColor
			}
			return col, sh.Blur
		}
		return 0, 0
	}
	return t.shadowPaint()
}

// StateFlags are the widget states a stylesheet matches that no router
// drives: an application sets them to reflect its own model.
type StateFlags uint8

// The application-driven states.
const (
	// StateChecked matches :checked — a toggled-on control, or a menu
	// button whose popup is open.
	StateChecked StateFlags = 1 << iota
	// StateSelected matches :selected.
	StateSelected
	// StateIndeterminate matches :indeterminate.
	StateIndeterminate
	// StateBackdrop matches :backdrop — the widget's window is inactive.
	StateBackdrop
	// StateTarget matches :target — the widget an in-app navigation
	// points at, the role a URL fragment's element plays on the web.
	StateTarget
)

// styleBits maps flags onto the matcher's state bits.
func (f StateFlags) styleBits() style.State {
	var s style.State
	if f&StateChecked != 0 {
		s |= style.Checked
	}
	if f&StateSelected != 0 {
		s |= style.Selected
	}
	if f&StateIndeterminate != 0 {
		s |= style.Indeterminate
	}
	if f&StateBackdrop != 0 {
		s |= style.Backdrop
	}
	if f&StateTarget != 0 {
		s |= style.Targeted
	}
	return s
}

// SetState turns application-driven state flags on or off. The widget
// restyles when the set changed.
func (n *node) SetState(flags StateFlags, on bool) {
	next := n.flags &^ flags
	if on {
		next |= flags
	}
	if next == n.flags {
		return
	}
	changed := n.flags ^ next
	n.flags = next
	n.invalidateState(changed.styleBits())
}

// HasState reports whether every flag in flags is on.
func (n *node) HasState(flags StateFlags) bool { return n.flags&flags == flags }

// SetInlineStyle sets the widget's inline declarations — `color: red;
// --accent: #f80` — the widget-scoped provider GTK attaches per widget.
// They apply to this widget alone (custom properties then inherit into
// its subtree) at StylePriorityUser, with no specificity. Empty clears.
// Malformed declarations are skipped with a warning.
func (n *node) SetInlineStyle(css string) {
	n.SetInlineStylePriority(css, StylePriorityUser)
}

// SetInlineStylePriority is SetInlineStyle at an explicit priority.
func (n *node) SetInlineStylePriority(css string, priority int) {
	checkLoop("SetInlineStyle")
	if css == n.inlineSrc && priority == n.inlinePrio && (n.inline != nil) == (css != "") {
		return
	}
	n.inlineSrc, n.inlinePrio = css, priority
	if css == "" {
		n.inline = nil
	} else {
		n.inline = style.ParseDeclarations(css)
	}
	n.invalidateStyle()
}

// InlineStyle returns the inline declarations as set.
func (n *node) InlineStyle() string { return n.inlineSrc }

// AddClass adds style classes; a widget matches `.class` selectors
// naming any of them. Adding a class already present is a no-op. The
// change restyles the widget (and, when a selector tests the class on
// an ancestor or sibling, the widgets that depend on it) on the next
// read.
func (n *node) AddClass(names ...string) {
	var changed []string
	for _, name := range names {
		if name == "" || n.hasClass(name) {
			continue
		}
		n.classes = append(n.classes, name)
		changed = append(changed, name)
	}
	if len(changed) > 0 {
		n.invalidateClasses(changed)
	}
}

// RemoveClass drops style classes; dropping one the widget does not
// carry is a no-op.
func (n *node) RemoveClass(names ...string) {
	var changed []string
	for _, name := range names {
		for i, c := range n.classes {
			if c == name {
				n.classes = append(n.classes[:i], n.classes[i+1:]...)
				changed = append(changed, name)
				break
			}
		}
	}
	if len(changed) > 0 {
		n.invalidateClasses(changed)
	}
}

// SetClass adds the class name when on and removes it otherwise.
func (n *node) SetClass(name string, on bool) {
	if on {
		n.AddClass(name)
		return
	}
	n.RemoveClass(name)
}

// SetClasses replaces the class list wholesale — the GTK
// set_css_classes idiom for widgets whose modifiers are recomputed at
// once. Order is irrelevant to matching.
func (n *node) SetClasses(names ...string) {
	var next []string
	for _, name := range names {
		if name != "" && !slices.Contains(next, name) {
			next = append(next, name)
		}
	}
	var changed []string
	for _, c := range n.classes {
		if !slices.Contains(next, c) {
			changed = append(changed, c)
		}
	}
	for _, c := range next {
		if !slices.Contains(n.classes, c) {
			changed = append(changed, c)
		}
	}
	if len(changed) == 0 {
		return
	}
	n.classes = next
	n.invalidateClasses(changed)
}

// Classes returns a copy of the widget's classes.
func (n *node) Classes() []string { return slices.Clone(n.classes) }

// HasClass reports whether the widget carries the style class.
func (n *node) HasClass(name string) bool { return n.hasClass(name) }

func (n *node) hasClass(name string) bool {
	return slices.Contains(n.classes, name)
}

// HasClass reports whether w carries the style class — the read side
// for tests and accessibility tooling.
func HasClass(w Widget, class string) bool {
	n := nodeOf(w)
	return n != nil && n.hasClass(class)
}

// SetID sets the widget's style id, which `#id` selectors match. It is
// distinct from the accessible name: a11y names stay semantic text,
// not style hooks. Empty clears.
func (n *node) SetID(id string) {
	checkLoop("SetID")
	if n.id == id {
		return
	}
	n.id = id
	n.invalidateScope(true, true)
}

// ID returns the style id, empty when none is set.
func (n *node) ID() string { return n.id }

// SetAttr sets one data attribute, the `[attr]` and `[attr=value]`
// selectors' subject — app-attached metadata, GTK has no analog. An
// empty name is a no-op; an empty value sets a present-but-empty
// attribute. The change restyles the widget and, conservatively, its
// subtree.
func (n *node) SetAttr(name, value string) {
	checkLoop("SetAttr")
	if name == "" {
		return
	}
	for i := range n.attrs {
		if n.attrs[i].Name == name {
			if n.attrs[i].Value == value {
				return
			}
			n.attrs[i].Value = value
			n.invalidateScope(true, true)
			return
		}
	}
	n.attrs = append(n.attrs, style.Attr{Name: name, Value: value})
	n.invalidateScope(true, true)
}

// RemoveAttr drops one data attribute; dropping one the widget does
// not carry is a no-op.
func (n *node) RemoveAttr(name string) {
	for i := range n.attrs {
		if n.attrs[i].Name == name {
			n.attrs = slices.Delete(n.attrs, i, i+1)
			n.invalidateScope(true, true)
			return
		}
	}
}

// Attr returns one data attribute's value and whether it is set.
func (n *node) Attr(name string) (string, bool) {
	for i := range n.attrs {
		if n.attrs[i].Name == name {
			return n.attrs[i].Value, true
		}
	}
	return "", false
}

// StyleBackground returns the widget's computed background color, the
// diagnostic view a paint test reads.
func StyleBackground(w Widget) render.Color {
	if n := nodeOf(w); n != nil {
		return pickc(0, n.style(w), style.PropBackgroundColor, 0)
	}
	return 0
}

// StylesBackground reports whether the stylesheet declares w's
// background - a color (transparent included), an image, or another
// box layer - so a host that paints a default plate behind w (a
// popover's) can yield to it, as GTK leaves a popover's look to its
// CSS.
func StylesBackground(w Widget) bool {
	n := nodeOf(w)
	if n == nil {
		return false
	}
	v := n.style(w)
	return v.Has(style.PropBackgroundColor) || hasBoxLayers(v)
}

// InkOutset is how far w paints outside its bounds: its box shadows,
// outline, border image and filters. A surface whose card w is
// reserves that room around it.
func InkOutset(w Widget) render.Insets {
	n := nodeOf(w)
	if n == nil {
		return render.Insets{}
	}
	return inkOf(n.style(w))
}

// CascadeColor resolves the widget's computed color: the CSS `color`,
// inherited down the tree, which the canvas-drawn primitives take as
// their ink (the progress ring's stroke). Zero paints nothing — the
// no-stylesheet behavior, same as everywhere else.
func CascadeColor(w Widget) render.Color {
	n := nodeOf(w)
	if n == nil {
		return 0
	}
	return pickc(0, n.style(w), style.PropColor, 0)
}

// CascadeBorder resolves the widget's computed border widths: the
// stroke a canvas-drawn primitive takes from the stylesheet (the
// progress ring reads the top width as its stroke).
func CascadeBorder(w Widget) render.Insets {
	n := nodeOf(w)
	if n == nil {
		return render.Insets{}
	}
	return borderOf(n.style(w))
}

// PaintBoxLayers paints the widget's computed box layers — background,
// image, borders, shadows, and the outline — at its bounds. A custom
// widget's Paint calls it for its own chrome before drawing content,
// so the container styling stays in the cascade (the hovered row's
// background, the :hover rule, is this). The opacity and brightness
// in force wrap the layers.
func PaintBoxLayers(cv *render.Canvas, w Widget) {
	n := nodeOf(w)
	if n == nil {
		return
	}
	v := n.style(w)
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, n.bounds, radii)
	cv = fx.cv
	defer fx.pop()
	bg := pickc(0, v, style.PropBackgroundColor, 0)
	if bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, n.bounds, radii, borderOf(v), bg)
	}
	paintPseudos(cv, w, n, v)
	paintPseudoImage(cv, n, v)
	paintOutline(cv, v, n.bounds, radii)
}

// SetElement overrides the element name the widget matches in
// stylesheets. The default is the widget's own type name lowercased;
// the app-level surfaces (dialog, popover, tooltip, toast) name
// themselves with this in their constructors.
func (n *node) SetElement(name string) {
	checkLoop("SetElement")
	if name == "" || n.element == name {
		return
	}
	n.element = name
	n.invalidateScope(true, true)
}

// Element returns the element name the widget matches: the override
// set with SetElement, else the lowercased Go type name.
func (n *node) Element() string {
	if n.element != "" || n.self == nil {
		return n.element
	}
	return n.elementOf(n.self)
}

// elementOf is Element for the widget w embedding n: it resolves the
// type name from w's style owner, so a sibling read before any
// container arranged it (and recorded its self) still reports its
// name, and a struct wrapping a widget reports the widget's.
func (n *node) elementOf(w Widget) string {
	if n.element != "" {
		return n.element
	}
	if n.elemName == "" {
		n.elemName = typeElementName(styleOwner(w))
	}
	return n.elemName
}

// styleNoder is implemented by every node-embedding widget.
type styleNoder interface{ styleNode() *node }

func (n *node) styleNode() *node { return n }

// nodeOf returns the shared node behind w, or nil.
func nodeOf(w Widget) *node {
	if n, ok := w.(styleNoder); ok {
		return n.styleNode()
	}
	return nil
}

// styleTargeter adapts one widget (and, through StyleParent and
// StylePrev, its neighborhood) to the matcher. Instances are reused —
// each link's adapters hang off the previous one and are rewritten per
// match — so walking the tree allocates only on first use of a depth.
type styleTargeter struct {
	n    *node
	w    Widget
	anc  *styleTargeter // the reused adapter for the parent link
	prev *styleTargeter // the reused adapter for the previous sibling
}

func (s *styleTargeter) StyleID() any { return s.n }

func (s *styleTargeter) StyleTarget(t *style.Target) { s.n.fillTarget(t, s.w) }

func (s *styleTargeter) StyleParent() style.Node {
	p := styleParentOf(s.n)
	if p == nil {
		return nil
	}
	pn := nodeOf(p)
	if pn == nil {
		return nil
	}
	if s.anc == nil {
		s.anc = &styleTargeter{}
	}
	s.anc.n, s.anc.w = pn, p
	return s.anc
}

func (s *styleTargeter) StylePrev() style.Node {
	sib, idx := visibleSiblings(s.n, s.w)
	if idx <= 0 {
		return nil
	}
	pw := sib[idx-1]
	pn := nodeOf(pw)
	if pn == nil {
		return nil
	}
	if s.prev == nil {
		s.prev = &styleTargeter{}
	}
	s.prev.n, s.prev.w = pn, pw
	return s.prev
}

func (s *styleTargeter) StylePosition() (int, int) {
	if p, ok := s.w.(modelPositioner); ok {
		return p.modelPosition()
	}
	sib, idx := visibleSiblings(s.n, s.w)
	if idx < 0 {
		return 1, 1
	}
	return idx + 1, len(sib)
}

func (s *styleTargeter) StyleTypedPosition(element string) (int, int) {
	if p, ok := s.w.(modelPositioner); ok {
		return p.modelPosition()
	}
	sib, idx := visibleSiblings(s.n, s.w)
	if idx < 0 {
		return 1, 1
	}
	count, mine := 0, 0
	for i, k := range sib {
		if kn := nodeOf(k); kn != nil && kn.elementOf(k) == element {
			count++
			if i == idx {
				mine = count
			}
		}
	}
	return mine, count
}

func (s *styleTargeter) StyleChildren() []style.Node {
	gen := DamageGeneration()
	if s.n.kidsGen != gen {
		var kids []Widget
		switch t := s.w.(type) {
		case childBuf:
			kids = t.appendChildren(kids)
		case childser:
			kids = append(kids, t.Children()...)
		}
		if cap(s.n.kidsAdaps) < len(kids) {
			s.n.kidsAdaps = make([]*styleTargeter, len(kids))
		}
		adaps := s.n.kidsAdaps[:0]
		out := s.n.kids[:0]
		for _, k := range kids {
			if k == nil {
				continue
			}
			kn := nodeOf(k)
			if kn == nil || kn.hidden {
				continue
			}
			var a *styleTargeter
			if len(adaps) < len(s.n.kidsAdaps) && s.n.kidsAdaps[len(adaps)] != nil {
				a = s.n.kidsAdaps[len(adaps)]
				a.n, a.w = kn, k
			} else {
				a = &styleTargeter{n: kn, w: k}
			}
			adaps = append(adaps, a)
			out = append(out, a)
		}
		s.n.kidsAdaps, s.n.kids, s.n.kidsGen = adaps, out, gen
	}
	return s.n.kids
}

// StyleContainers appends the node's query containers onto buf,
// nearest first: every style ancestor whose computed cascade sets
// container-type, with its arranged bounds (an ancestor's style is
// cached — a descendant computes after its ancestors — so this walk
// never reenters the matcher). Containers stop at a style root.
func (s *styleTargeter) StyleContainers(buf []style.Container) []style.Container {
	for pw := styleParentOf(s.n); pw != nil; {
		pn := nodeOf(pw)
		if pn == nil {
			return buf
		}
		v := pn.style(pw)
		if v.Has(style.PropContainerType) && v.ContainerType != style.ContainerNormal {
			buf = append(buf, style.Container{
				W: float64(pn.bounds.W), H: float64(pn.bounds.H),
				Size:  v.ContainerType == style.ContainerSize,
				Names: v.ContainerNames,
			})
		}
		pw = styleParentOf(pn)
	}
	return buf
}

// modelPositioner is a virtualized item whose structural position is
// its place in the model, not among the few items materialized beside
// it: a List row counts as the model's row, so row:nth-child(even)
// stripes the same items however far the list scrolled.
type modelPositioner interface {
	// modelPosition is the 1-based index and the model's length.
	modelPosition() (index, count int)
}

// visibleSiblings returns the visible children of n's parent and n's
// index among them; -1 when n has no parent or is not among them. A
// parent with no Children (a style-only part, like a notebook's tab
// row) lists its style kids, so its parts take structural selectors.
// The list is memoized on the parent node keyed by the damage
// generation: a sibling-selector restyle marks the whole sibling set
// in one pass, so the rebuild happens once per parent per generation,
// not once per restyled widget. Membership and visibility changes all
// mark damage (Append through InvalidateLayout, SetVisible through
// the parent's relayout), so a mark cannot be missed; the memo is
// loop-goroutine state like every other walk buffer.
func visibleSiblings(n *node, w Widget) ([]Widget, int) {
	if n.parent == nil {
		return nil, -1
	}
	pn := nodeOf(n.parent)
	if pn == nil {
		return nil, -1
	}
	gen := DamageGeneration()
	if pn.sibsGen != gen {
		out := pn.sibs[:0]
		switch p := n.parent.(type) {
		case childBuf:
			out = p.appendChildren(out)
		case childser:
			out = append(out, p.Children()...)
		case interface{ appendStyleKids([]Widget) []Widget }:
			out = p.appendStyleKids(out)
		default:
			pn.sibs, pn.sibsGen = nil, gen
			return nil, -1
		}
		filtered := out[:0]
		for _, k := range out {
			if k == nil {
				continue
			}
			if kn := nodeOf(k); kn != nil && kn.hidden {
				continue
			}
			filtered = append(filtered, k)
		}
		pn.sibs, pn.sibsGen = filtered, gen
	}
	idx := -1
	for i, k := range pn.sibs {
		if k == w {
			idx = i
			break
		}
	}
	return pn.sibs, idx
}

// fillTarget fills the selector-relevant facts of one widget.
func (n *node) fillTarget(t *style.Target, w Widget) {
	t.Element = n.elementOf(w)
	t.ID = n.id
	t.Classes = n.classes
	t.State = n.styleState(w)
	t.Role = roleAttr(w)
	t.Attrs = n.attrs
	t.HasText = hasTextContent(w)
	t.Inline = n.inline
	t.InlinePriority = n.inlinePrio
}

// hasTextContent reports whether w shows text of its own — the text
// child that keeps a DOM element from matching :empty.
func hasTextContent(w Widget) bool {
	switch t := w.(type) {
	case *Label:
		return t.text != ""
	case *RichLabel:
		return t.text != ""
	case *Entry:
		return len(t.runes) > 0
	case *TextArea:
		return len(t.lines) > 1 || len(t.lines) == 1 && len(t.lines[0]) > 0
	}
	return false
}

// roleAttr is the widget's semantic role as an attribute value: the
// a11y role's name, empty for plain containers.
func roleAttr(w Widget) string {
	r := RoleOf(w)
	if r == RoleNone {
		return ""
	}
	return r.String()
}

// styleState is the widget's interactive state for matching: the
// focus bits the router maintains, the hover chain (a hovered widget's
// ancestors are hovered too, GTK's rule), the hover and press visuals
// each widget tracks in its own fields, the application flags, and the
// effective disabled state — inherited, like IsEnabled reads it.
func (n *node) styleState(w Widget) style.State {
	s := n.flags.styleBits()
	if n.focused {
		s |= style.Focus
		if n.focusVisible {
			s |= style.FocusVisible
		}
	}
	if n.focusWithin > 0 {
		s |= style.FocusWithin
	}
	if n.hoverChain {
		s |= style.Hover
	}
	for p := n; p != nil; p = parentNodeOf(p) {
		if p.disabled {
			s |= style.Disabled
			break
		}
	}
	s |= hoverActiveOf(w)
	s |= textStateOf(w)
	return s
}

// textStateOf reads the text-field states a stylesheet's :read-only
// and :placeholder-shown match.
func textStateOf(w Widget) style.State {
	var s style.State
	switch t := w.(type) {
	case *Entry:
		if t.readOnly {
			s |= style.ReadOnly
		}
		if t.placeholder != "" && len(t.runes) == 0 && !t.composing() {
			s |= style.PlaceholderShown
		}
	}
	return s
}

// styleParentOf is n's parent for styling: its layout parent, or for
// a parentless tree the widget it is anchored to (SetStyleAnchor);
// none past a style root.
func styleParentOf(n *node) Widget {
	switch {
	case n.styleRoot:
		return nil
	case n.parent != nil:
		return n.parent
	}
	return n.styleAnchor
}

// styleParentNodeOf is the node behind styleParentOf.
func styleParentNodeOf(n *node) *node {
	if p := styleParentOf(n); p != nil {
		return nodeOf(p)
	}
	return nil
}

// StyleAnchor is the widget w's tree is anchored to (SetStyleAnchor),
// nil when none.
func StyleAnchor(w Widget) Widget {
	if n := nodeOf(w); n != nil {
		return n.styleAnchor
	}
	return nil
}

// SetStyleAnchor styles w's tree as a child of anchor while w has no
// layout parent: inheritance, selector ancestry, :root and the scoped
// sheets continue up anchor's chain, as a GTK popover's CSS node is a
// child of the widget it pops from. A change anchor's descendants
// would inherit restyles the tree too. nil unanchors it; a new anchor
// replaces the old.
func SetStyleAnchor(w, anchor Widget) {
	n := nodeOf(w)
	if n == nil || n.styleAnchor == anchor {
		return
	}
	if old := nodeOf(n.styleAnchor); old != nil {
		old.anchored = slices.DeleteFunc(old.anchored, func(k Widget) bool { return k == w })
	}
	n.styleAnchor = anchor
	if an := nodeOf(anchor); an != nil {
		an.anchored = append(an.anchored, w)
	}
	n.markStyleDirty()
	restyleSubtree(w)
}

// SetStyleRoot makes w the root of its tree for styling: :root matches
// it, and nothing above it is inherited or contributes scoped sheets,
// while it keeps its layout parent - the app wraps every window's tree
// in plumbing (a fader, the inspector overlay) that invalidations must
// climb through but stylesheets must not see.
func SetStyleRoot(w Widget) {
	if n := nodeOf(w); n != nil && !n.styleRoot {
		n.styleRoot = true
		n.markStyleDirty()
		markDamage()
	}
}

// parentNodeOf returns the node behind n's recorded parent.
func parentNodeOf(n *node) *node {
	if n.parent == nil {
		return nil
	}
	return nodeOf(n.parent)
}

// hoverActiveOf reads the hover and press visuals a widget tracks in
// its own fields, so the plain fields the router drives stay the one
// source of truth for :hover and :active, plus the intrinsic :checked
// of toggles.
func hoverActiveOf(w Widget) style.State {
	var s style.State
	switch t := w.(type) {
	case *Button:
		if t.Hovered {
			s |= style.Hover
		}
		if t.Pressed {
			s |= style.Active
		}
	case *Dropdown:
		if t.hovered {
			s |= style.Hover
		}
	case *Expander:
		if t.hovered {
			s |= style.Hover
		}
	case *Menu:
		if t.hovered >= 0 {
			s |= style.Hover
		}
	case *menuRow:
		if t.hovered {
			s |= style.Hover
		}
	case *notebookTabPart:
		if t.hovered {
			s |= style.Hover
		}
	case *Paned:
		if t.hovered {
			s |= style.Hover
		}
		if t.pressed {
			s |= style.Active
		}
	case *RichLabel:
		if t.hoverValid {
			s |= style.Hover
		}
	case *Scroll:
		if t.hovered {
			s |= style.Hover
		}
		if t.dragV || t.dragH {
			s |= style.Active
		}
	case *Slider:
		if t.hovered {
			s |= style.Hover
		}
		if t.Pressed {
			s |= style.Active
		}
	case *Toast:
		if t.hovered {
			s |= style.Hover
		}
	case *Switch:
		if t.On() {
			s |= style.Checked
		}
	case *CheckButton:
		if t.Checked() {
			s |= style.Checked
		}
		if t.Inconsistent() {
			s |= style.Indeterminate
		}
	}
	return s
}

// styleScratch is the matcher's shared workspace, styleSelf its
// shared adapter, and styleNext the values a restyle computes into:
// style work runs on the event-loop goroutine (docs/threading.md), so
// one buffer serves the process. styleBusy guards styleNext: a node's
// ancestry restyles before its compute starts, so nothing may restyle
// while one is in flight.
var (
	styleScratch style.Scratch
	styleSelf    styleTargeter
	styleNext    style.Values
	styleBusy    bool
)

// style returns the widget's computed style — the paint path's entry
// point, called exactly where Theme fields are read. Steady state is
// two integer compares; after an invalidation the first read
// recomputes and diff-invalidates, its style parts with it: a part
// whose layout changed (a progress bar's trough min-height) then drops
// the widget's measure before the widget answers from it.
func (n *node) style(w Widget) *style.Values {
	if !n.styleDirty && n.csGen == styleGen {
		return &n.cs
	}
	v := n.restyle(w)
	restyleParts(w)
	return v
}

// partStack is restyleParts' per-depth kid buffers, reused across
// restyles like styleStack; partDepth is the nesting in flight (a
// part's restyle refreshes its own parts a level down).
var (
	partStack [][]Widget
	partDepth int
)

// restyleParts refreshes w's style parts after w restyled.
func restyleParts(w Widget) {
	for len(partStack) <= partDepth {
		partStack = append(partStack, nil)
	}
	kids := appendStyleKids(w, partStack[partDepth][:0])
	partStack[partDepth] = kids
	partDepth++
	defer func() { partDepth-- }()
	for _, k := range kids {
		if p, ok := k.(styleParter); ok {
			p.partNode().style(k)
		}
	}
}

// restyle recomputes the cascade and invalidates the smallest correct
// region: the widget when its own values changed, the subtree when an
// inherited one did, a relayout when a layout-affecting one did. A
// recompute that leaves the values Equal — the common restyle — costs
// the compute and one comparison, and touches nothing.
func (n *node) restyle(w Widget) *style.Values {
	w = styleOwner(w)
	if n.self == nil {
		n.self = w
	}
	firstStyle := n.styleSeen == 0
	var parent *style.Values
	if pw := styleParentOf(n); pw != nil {
		if pn := nodeOf(pw); pn != nil {
			// The chain above must be fresh before it inherits: a stale
			// ancestor recomputes first, recursively. It runs before the
			// layers are gathered, which share a buffer with its compute.
			parent = pn.style(pw)
		}
	}
	inherits := parent != nil && (!parent.Set.Empty() || parent.Vars != nil)
	// The counter inputs read other widgets' styles, so they resolve
	// before the layers and the shared adapter are taken.
	var prevCounters, srcCounters *style.CounterScope
	if sensNow.counters {
		prevCounters, srcCounters = prevCountersOf(n, w)
	}
	if styleBusy {
		panic("widget: a restyle started inside another's compute")
	}
	styleBusy = true
	v := &styleNext
	if layers := layersFor(n); len(layers) > 0 || n.inline != nil || inherits {
		styleSelf.n, styleSelf.w = n, w
		styleScratch.SetPrevCounters(prevCounters, srcCounters)
		style.Compute(layers, &styleSelf, parent, &n.cs, envFor(n), &styleScratch, v)
		if sensNow.pseudo {
			n.computePseudos(layers, v)
		}
	} else {
		*v = style.Values{}
		if n.pseudoGen != styleGen {
			n.pseudo = nil
			n.pseudoGen = styleGen
		}
	}
	n.csGen = styleGen
	n.styleSeen = styleGen
	n.styleDirty = false
	if !firstStyle && n.trCancel == nil && n.anims == nil && n.cs.Equal(v) {
		styleBusy = false
		return &n.cs
	}
	old := n.cs
	n.cs = *v
	styleBusy = false
	n.ink = inkOf(&n.cs)
	n.transitionValues(&old, &n.cs, firstStyle)
	n.syncAnimation(&n.cs)
	if !old.Equal(&n.cs) {
		if inv, ok := w.(interface{ Invalidate() }); ok {
			inv.Invalidate()
		}
		if h, ok := w.(styleRestyler); ok {
			h.styleRestyled(old, n.cs)
		}
	}
	if layoutKey(&n.cs) != layoutKey(&old) {
		if inv, ok := w.(interface{ InvalidateLayout() }); ok {
			inv.InvalidateLayout()
		}
	}
	if inheritedKey(&n.cs) != inheritedKey(&old) {
		restyleSubtree(w)
	}
	if sensNow.counters && !n.cs.Counters.Equal(old.Counters) {
		restyleCountersAfter(w)
	}
	return &n.cs
}

// restyleCountersAfter marks what reads w's counter set: its subtree,
// which inherits it, and everything after w in tree order — the
// following siblings of w and of each ancestor, and their subtrees —
// which inherits its values (css-lists-3 §4.4.1). The walk only moves
// forward, so a chain of changed sets settles.
func restyleCountersAfter(w Widget) {
	restyleSubtree(w)
	for cur := w; cur != nil; cur = parentOf(cur) {
		cn := nodeOf(cur)
		if cn == nil || cn.styleRoot {
			return
		}
		sib, idx := visibleSiblings(cn, cur)
		if idx < 0 {
			continue
		}
		for _, k := range sib[idx+1:] {
			if kn := nodeOf(k); kn != nil {
				kn.markStyleDirty()
			}
			restyleSubtree(k)
		}
	}
}

// pseudoNames are the painted pseudo-elements, in cache order.
var pseudoNames = [...]string{"before", "after", "first-letter"}

// pseudoNext is what computePseudos cascades into before it keeps or
// drops a node's set (loop-goroutine only, like styleNext).
var pseudoNext [len(pseudoNames)]style.Values

// prevCountersOf reads the two counter sets CSS counter inheritance
// takes beside the parent's: the previous visible sibling's, and the
// one of the element immediately preceding w in tree order — that
// sibling's last visible descendant — whose values carry forward.
// Both nil without a previous sibling. Computed lazily like any style
// read; only reached when counters are in play.
func prevCountersOf(n *node, w Widget) (prev, src *style.CounterScope) {
	sib, idx := visibleSiblings(n, w)
	if idx <= 0 {
		return nil, nil
	}
	pw := sib[idx-1]
	pn := nodeOf(pw)
	if pn == nil {
		return nil, nil
	}
	prev = pn.style(pw).Counters
	src = prev
	for last := lastStyleKid(pw); last != nil; last = lastStyleKid(last) {
		src = nodeOf(last).style(last).Counters
	}
	return prev, src
}

// lastStyleKid is w's last visible styled child, nil for none.
func lastStyleKid(w Widget) Widget {
	for _, k := range slices.Backward(appendStyleKids(w, nil)) {
		if kn := nodeOf(k); kn != nil && !kn.hidden {
			return k
		}
	}
	return nil
}

// computePseudos cascades the pseudo-element rules of this widget,
// inheriting from its own computed style, and repaints when one moved.
// A widget no rule addresses keeps no set at all.
func (n *node) computePseudos(layers []style.Layer, self *style.Values) {
	addressed := false
	for i, name := range pseudoNames {
		if style.ComputePseudo(layers, &styleSelf, self, envFor(n), &styleScratch, &pseudoNext[i], name) {
			addressed = true
		}
	}
	n.pseudoGen = styleGen
	if !addressed {
		if n.pseudo != nil {
			n.pseudo = nil
			n.Invalidate()
		}
		return
	}
	if n.pseudo == nil {
		n.pseudo = new([len(pseudoNames)]style.Values)
		n.Invalidate()
	} else {
		for i := range pseudoNames {
			if !n.pseudo[i].Equal(&pseudoNext[i]) {
				n.Invalidate()
				break
			}
		}
	}
	*n.pseudo = pseudoNext
}

// PseudoStyle returns w's computed ::before/::after/::first-letter
// style — the cascade the pseudo-element's rules produced, inheriting
// from the widget's own. nil when no rule addresses any of w's
// pseudo-elements or the name is not one of them. The values are fresh
// only after a restyle; paint paths read them through PaintBoxLayers.
func PseudoStyle(w Widget, pseudo string) *style.Values {
	n := nodeOf(w)
	if n == nil || !sensNow.pseudo {
		return nil
	}
	for i, name := range pseudoNames {
		if name == pseudo {
			if n.pseudoGen != styleGen || n.styleDirty {
				n.style(w)
			}
			if n.pseudo == nil {
				return nil
			}
			return &n.pseudo[i]
		}
	}
	return nil
}

// PseudoContent returns w's resolved pseudo-element content — the
// strings, attr() values, and counter readings a `content:` value
// joined — empty when the pseudo-element declares none.
func PseudoContent(w Widget, pseudo string) string {
	if v := PseudoStyle(w, pseudo); v != nil {
		return v.Content
	}
	return ""
}

// pseudoFacer is implemented by the widgets that shape text and so
// can paint a pseudo-element's content string: ::before/::after
// content draws in the widget's face (or the pseudo-element's own
// font-size when it sets one).
type pseudoFacer interface {
	pseudoFace() (render.Font, float64)
}

// paintPseudos paints the ::before/::after layers over the widget's
// own background: the pseudo-element's box layers on the padding box,
// then its content string — before at the content start, after at the
// content end. Content needs a face; box layers paint for any widget.
func paintPseudos(cv *render.Canvas, w Widget, n *node, v *style.Values) {
	if !sensNow.pseudo || n.pseudoGen != styleGen || n.pseudo == nil {
		return
	}
	b := boxOf(v, render.Insets{})
	box := b.padding.Shrink(b.border.Shrink(n.bounds))
	for i := range pseudoNames {
		pv := &n.pseudo[i]
		if !pv.HasContent && !pv.HasAny(style.PropBackgroundColor) && !hasBoxLayers(pv) {
			continue
		}
		if bg := pickc(0, pv, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(pv) {
			paintBoxBehind(cv, pv, box, radiusOr(pv, 0), borderOf(pv), bg)
		}
		if pv.Content == "" {
			continue
		}
		f, ok := w.(pseudoFacer)
		if !ok {
			continue
		}
		face, px := f.pseudoFace()
		if pv.Has(style.PropFontSize) {
			px = pv.FontSize
		}
		sh := face.Shape(pv.Content, px)
		x := box.X
		if i == 1 {
			x = box.X + max(box.W-int(sh.Advance()), 0)
		}
		baseline := box.Y + (box.H-sh.LineHeight())/2 + int(sh.Ascent())
		col := pickc(0, pv, style.PropColor, v.Color)
		sh.Draw(cv, x, baseline, col)
	}
}

// paintPseudoImage paints a pseudo-element's content: url() image at
// the content start (::before) or end (::after), vertically centered
// at its natural size beside any text content.
func paintPseudoImage(cv *render.Canvas, n *node, v *style.Values) {
	if !sensNow.pseudo || n.pseudoGen != styleGen || n.pseudo == nil {
		return
	}
	b := boxOf(v, render.Insets{})
	box := b.padding.Shrink(b.border.Shrink(n.bounds))
	for i := range pseudoNames {
		pv := &n.pseudo[i]
		if pv.ContentURL == "" {
			continue
		}
		img := bgImageFor(pv.ContentURL)
		if img == nil {
			continue
		}
		iw, ih := img.LogicalSize()
		x, y := box.X, box.Y+max((box.H-ih)/2, 0)
		if i == 1 {
			x = box.X + max(box.W-iw, 0)
		}
		img.Draw(cv, x, y)
	}
}

// styleRestyler is the optional per-widget reaction to a changed
// cascade: Label reshapes when its effective face or size moved.
type styleRestyler interface {
	styleRestyled(old, new style.Values)
}

// The widgets that react to a changed cascade.
var _, _ styleRestyler = (*Label)(nil), (*RichLabel)(nil)

// layoutKey projects the values that change what a widget wants to
// measure or where its children go; a change requests a relayout
// through InvalidateLayout.
func layoutKey(v *style.Values) layoutFacts {
	eff := v.EffBorder()
	return layoutFacts{
		padding: v.Padding, margin: v.Margin, border: eff,
		gutter: v.BarGutter,
		setPad: v.HasAny(style.PropPaddingTop, style.PropPaddingRight, style.PropPaddingBottom, style.PropPaddingLeft),
		lay:    v.Layout, marginAuto: v.MarginAuto, spacingH: v.BorderSpacingH, spacingV: v.BorderSpacingV,
		setSpacing: v.Has(style.PropBorderSpacing),
		fontSize:   v.FontSize, fontFamily: v.FontFamily, fontWeight: v.FontWeight, italic: v.Italic,
		iconSize: v.IconSize, transform: v.TextTransform, letter: v.LetterSpacing,
		text: textFacts{
			indent: v.TextIndent, white: v.WhiteSpace, wordBreak: v.WordBreak, lineBreak: v.LineBreak,
			hyphens: v.Hyphens, word: v.WordSpacing, clamp: v.LineClamp, overflow: v.Overflow,
			optical: v.OpticalSizing, caps: v.VariantCaps, numeric: v.VariantNumeric,
			lig: v.VariantLig, eastAsian: v.VariantEA, position: v.VariantPos, alternates: v.VariantAlt,
		},
		set: v.Set.And(layoutKeyProps),
	}
}

// layoutKeyProps are the longhands whose presence alone changes what a
// widget measures: the layout group and the text metrics.
var layoutKeyProps = style.LayoutProps.Or(style.SetOf(style.PropFontSize, style.PropFontFamily,
	style.PropFontWeight, style.PropIconSize, style.PropTextTransform,
	style.PropTextIndent, style.PropWhiteSpace, style.PropWordBreak, style.PropLineBreak,
	style.PropHyphens, style.PropWordSpacing, style.PropLineClamp, style.PropTextOverflow,
	style.PropFontOpticalSizing, style.PropFontVariantCaps, style.PropFontVariantNumeric,
	style.PropFontVariantLigatures, style.PropFontVariantEastAsian,
	style.PropFontVariantPosition, style.PropFontVariantAlternates))

// textFacts projects the CSS Text and font-variant properties that
// change what a text widget measures, packed comparable.
type textFacts struct {
	indent     float64
	white      style.WhiteSpace
	wordBreak  style.WordBreak
	lineBreak  style.LineBreakK
	hyphens    style.HyphensK
	word       float64
	clamp      int
	overflow   style.TextOverflowK
	optical    bool
	caps       style.FontVariantCaps
	numeric    style.FontVariantNumeric
	lig        style.FontVariantLigatures
	eastAsian  style.FontVariantEastAsian
	position   style.FontVariantPosition
	alternates style.FontVariantAlternates
}

// layoutFacts is layoutKey's comparable projection.
type layoutFacts struct {
	padding, margin, border style.Sides
	setPad                  bool
	gutter                  uint8
	lay                     style.Layout
	marginAuto              [4]bool
	spacingH, spacingV      int
	setSpacing              bool
	fontSize                float64
	fontFamily              string
	fontWeight              int
	italic                  bool
	iconSize                int
	transform               style.TextTransform
	letter                  float64
	text                    textFacts
	set                     style.PropSet
}

// inheritedKey projects the values descendants inherit — the custom
// property environment included; a change restyles the subtree.
func inheritedKey(v *style.Values) inheritedFacts {
	return inheritedFacts{
		set: v.Set.And(inheritedKeyProps), color: v.Color, fontSize: v.FontSize, fontFamily: v.FontFamily,
		fontWeight: v.FontWeight, italic: v.Italic, letter: v.LetterSpacing,
		transform: v.TextTransform, iconSize: v.IconSize, vars: v.Vars, cursor: v.Cursor,
		barThumb: v.BarThumb, barTrack: v.BarTrack, barAuto: v.BarColorAuto, barWidth: v.BarWidth,
		text: textFacts{
			indent: v.TextIndent, white: v.WhiteSpace, wordBreak: v.WordBreak, lineBreak: v.LineBreak,
			hyphens: v.Hyphens, word: v.WordSpacing, optical: v.OpticalSizing,
			caps: v.VariantCaps, numeric: v.VariantNumeric, lig: v.VariantLig,
			eastAsian: v.VariantEA, position: v.VariantPos, alternates: v.VariantAlt,
		},
		align: v.TextAlign, dir: v.Direction, writing: v.WritingMode, visibility: v.Layout.Visibility,
	}
}

// inheritedKeyProps are the inherited longhands whose presence alone
// changes what descendants compute.
var inheritedKeyProps = style.SetOf(style.PropColor, style.PropFontSize, style.PropFontFamily,
	style.PropFontWeight, style.PropFontStyle, style.PropLetterSpacing,
	style.PropTextAlign, style.PropTextIndent, style.PropWhiteSpace, style.PropWordBreak,
	style.PropLineBreak, style.PropHyphens, style.PropWordSpacing,
	style.PropFontVariantCaps, style.PropFontVariantNumeric, style.PropFontVariantLigatures,
	style.PropFontVariantEastAsian, style.PropFontVariantPosition, style.PropFontVariantAlternates,
	style.PropFontOpticalSizing, style.PropDirection, style.PropWritingMode,
	style.PropScrollbarColor, style.PropScrollbarWidth, style.PropVisibility)

// inheritedFacts is inheritedKey's comparable projection.
type inheritedFacts struct {
	set        style.PropSet
	color      render.Color
	fontSize   float64
	fontFamily string
	fontWeight int
	italic     bool
	letter     float64
	transform  style.TextTransform
	iconSize   int
	cursor     style.CursorSpec
	barThumb   render.Color
	barTrack   render.Color
	barAuto    bool
	barWidth   uint8
	text       textFacts
	align      style.TextAlign
	dir        uint8
	writing    style.WritingMode
	visibility style.Visibility
	vars       *style.Vars
}

// inkOf is how far a widget paints outside its border box: outer box
// shadows and the outline. The damage collector grows the widget's
// repaint rect by it.
func inkOf(v *style.Values) render.Insets {
	var ink render.Insets
	for _, sh := range v.Shadow.List() {
		e := render.BoxShadow{X: sh.X, Y: sh.Y, Blur: sh.Blur, Spread: sh.Spread, Color: sh.Color, Inset: sh.Inset}.Extent()
		ink.Top, ink.Right = max(ink.Top, e.Top), max(ink.Right, e.Right)
		ink.Bottom, ink.Left = max(ink.Bottom, e.Bottom), max(ink.Left, e.Left)
	}
	if o := outlineWidth(v); o > 0 {
		reach := max(0, o+v.OutlineOffset)
		ink.Top, ink.Right = max(ink.Top, reach), max(ink.Right, reach)
		ink.Bottom, ink.Left = max(ink.Bottom, reach), max(ink.Left, reach)
	}
	// A border image draws over its outset.
	if v.Has(style.PropBorderImageSource) && !v.BorderImage.Source.None() {
		bi := v.UsedBorderImage()
		o := borderImageOutset(&bi, borderOf(v))
		ink.Top, ink.Right = max(ink.Top, o.Top), max(ink.Right, o.Right)
		ink.Bottom, ink.Left = max(ink.Bottom, o.Bottom), max(ink.Left, o.Left)
	}
	// The filter list paints outside the box too: blur's skirt and
	// every drop-shadow's reach.
	if e := v.Filter.Extent(); !e.Zero() {
		ink.Top, ink.Right = max(ink.Top, e.Top), max(ink.Right, e.Right)
		ink.Bottom, ink.Left = max(ink.Bottom, e.Bottom), max(ink.Left, e.Left)
	}
	return ink
}

// restyleSubtree marks every descendant's computed style stale:
// something above them changed what they match or inherit, so each
// recomputes on its next read — the damage walk reaches them
// pre-order — and repaints only if its values actually moved (the
// restyle diff invalidates).
func restyleSubtree(w Widget) { restyleInto(w, 0) }

// styleStack is restyleSubtree's per-depth snapshot buffers, reused
// across passes like the damage walk's walkStack: tree walks are
// loop-goroutine work, and a level's snapshot lives until its subtree
// is done.
var styleStack [][]Widget

// restyleInto is restyleSubtree's recursive body, snapshotting the
// kids at each depth into styleStack's slot.
func restyleInto(w Widget, depth int) {
	for len(styleStack) <= depth {
		styleStack = append(styleStack, nil)
	}
	styleStack[depth] = appendStyleKids(w, styleStack[depth][:0])
	if n := nodeOf(w); n != nil {
		styleStack[depth] = append(styleStack[depth], n.anchored...)
	}
	for _, k := range styleStack[depth] {
		if n := nodeOf(k); n != nil {
			n.markStyleDirty()
		}
		restyleInto(k, depth+1)
	}
	markDamage()
}

// styleKids are the widgets that inherit from w: its Children, or what
// a widget that keeps its children out of the Children walks (a
// Button's content, a List's rows, a Menu's icons: focus, hit
// testing and a11y treat the parent as the leaf) lists through
// appendStyleKids.
func styleKids(w Widget) []Widget { return appendStyleKids(w, nil) }

// appendStyleKids appends w's inheriting children onto buf, the
// buffered form the restyle pass reads: one buffer per depth instead
// of a fresh slice per subtree per pass.
func appendStyleKids(w Widget, buf []Widget) []Widget {
	switch c := w.(type) {
	case interface{ appendStyleKids([]Widget) []Widget }:
		return c.appendStyleKids(buf)
	case childBuf:
		return c.appendChildren(buf)
	case childser:
		return append(buf, c.Children()...)
	}
	return buf
}

// restyleChildren marks w's children (and their subtrees) stale: a
// child was added, removed, reordered, or hidden, and a loaded
// structural selector may now match differently. Without one it is
// free.
func restyleChildren(w Widget) {
	if !sensNow.structural {
		return
	}
	restyleSubtree(w)
}

// markStyleDirty marks n's cascade stale, and every ancestor's cached
// measure as owing a pass down to n: the cascade reruns lazily, inside
// n's next measure, so a cached ancestor would otherwise never ask it,
// and a change to what n wants would lay out a pass late. The re-run
// ancestors find their other children cached; a change that alters n's
// layout drops their caches through InvalidateLayout as ever.
func (n *node) markStyleDirty() {
	n.styleDirty = true
	for p := parentNodeOf(n); p != nil; p = parentNodeOf(p) {
		p.subStyle = true
	}
}

// invalidateStyle is the shared body of the self-only style mutators:
// mark the cascade stale and owe the repaint. The diff-driven subtree
// and relayout work happens at the next read, in restyle.
func (n *node) invalidateStyle() {
	checkLoop("invalidateStyle")
	n.markStyleDirty()
	n.Invalidate()
}

// invalidateState restyles after the state bits in changed flipped:
// the widget, plus its subtree when a selector reads one of them on an
// ancestor, plus its siblings when a selector reads a sibling's facts
// (style.Sensitivity.SiblingFacts). The
// widget repaints: its own paint may read the state (a theme hover
// shade).
func (n *node) invalidateState(changed style.State) {
	n.invalidateScope(sensNow.states&changed != 0 || sensNow.has, sensNow.siblingFacts)
}

// restyleState is invalidateState for the propagated states — the
// hover chain and focus-within — on n, which no widget paints from
// itself: the cascade is marked stale, and only a moved
// value repaints, the root's subtree included.
func (n *node) restyleState(changed style.State) {
	n.markStyleDirty()
	if n.unseen() {
		return
	}
	markDamage()
	n.scope(sensNow.states&changed != 0 || sensNow.has, sensNow.siblingFacts)
}

// invalidateClasses is invalidateState for a class-list change.
func (n *node) invalidateClasses(changed []string) {
	anc := sensNow.has
	for _, c := range changed {
		if sensNow.classes[c] {
			anc = true
			break
		}
	}
	n.invalidateScope(anc, sensNow.siblingFacts)
}

// invalidateScope marks n stale, and its subtree when sub, and its
// parent's other children when sib. Under :has() selectors an
// ancestor's match reads this node's facts, so its chain goes stale
// too. A widget that has never been arranged has no subtree links
// yet: its first restyle happens anyway.
func (n *node) invalidateScope(sub, sib bool) {
	n.invalidateStyle()
	if sensNow.has && !n.unseen() {
		for p := parentNodeOf(n); p != nil; p = parentNodeOf(p) {
			p.markStyleDirty()
		}
		markDamage()
	}
	n.scope(sub, sib)
}

// scope marks the dependents of a flipped fact on n: the subtree when
// sub, the parent's children (and theirs) when sib. A root, which has
// no siblings, restyles its own subtree; only its moved values repaint.
// A widget never styled and in no tree has no dependents yet.
func (n *node) scope(sub, sib bool) {
	if !sub && !sib || n.unseen() {
		return
	}
	if sib && n.parent != nil {
		restyleSubtree(n.parent)
		return
	}
	restyleSubtree(n.self)
}

// setHoverChain moves the hover chain from old's ancestry to new's: a
// hovered widget and all its ancestors match :hover. Nodes in both
// chains keep their bit and restyle nothing.
func setHoverChain(old, new Widget) {
	if old == new {
		return
	}
	in := map[*node]bool{}
	for w := new; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil {
			in[n] = true
		}
	}
	for w := old; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil && n.hoverChain && !in[n] {
			n.hoverChain = false
			n.restyleState(style.Hover)
			notifyHoverWithin(w, n, false)
		}
	}
	for w := new; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil && !n.hoverChain {
			n.hoverChain = true
			n.restyleState(style.Hover)
			notifyHoverWithin(w, n, true)
		}
	}
}

// hoverWithiner is a widget the hover chain tells, as it tells
// SetOnHoverWithin's hook, without taking that hook from the app: a
// hover card opening over its trigger.
type hoverWithiner interface{ hoverWithin(on bool) }

// notifyHoverWithin tells w, whose node is n, that the pointer entered
// (on) or left its subtree.
func notifyHoverWithin(w Widget, n *node, on bool) {
	if h, ok := w.(hoverWithiner); ok {
		h.hoverWithin(on)
	}
	if n.onHoverWithin != nil {
		n.onHoverWithin(on)
	}
}

// pickc resolves a color property: the programmatic widget color when
// set, else the stylesheet value when matched, else the
// theme-derived default — the cascade's three origins, in order.
func pickc(prog render.Color, v *style.Values, p style.Prop, def render.Color) render.Color {
	if prog != 0 {
		return prog
	}
	if v.Has(p) {
		switch p {
		case style.PropColor:
			return v.Color
		case style.PropBackgroundColor:
			return v.Background
		case style.PropBorderTopColor:
			return v.BorderColor[0]
		case style.PropOutlineColor:
			return v.OutlineColor
		}
	}
	return def
}

// picki resolves an integer property: the stylesheet value when
// matched, else the constructor or theme default. Widget metrics have
// no programmatic layer — a constructor size is the default the
// stylesheet overrides, not app intent above it.
func picki(v *style.Values, p style.Prop, def int) int {
	if v.Has(p) {
		switch p {
		case style.PropMinWidth:
			return v.Layout.MinWidth.Px()
		case style.PropMinHeight:
			return v.Layout.MinHeight.Px()
		case style.PropBorderTopLeftRadius:
			return v.Radius.TopLeft
		case style.PropIconSize:
			return v.IconSize
		}
	}
	return def
}

// pickf returns the stylesheet's float value for p, def when unset —
// the same precedence picki gives ints.
func pickf(v *style.Values, p style.Prop, def float64) float64 {
	if v.Has(p) {
		switch p {
		case style.PropLineHeight:
			return v.LineHeight
		}
	}
	return def
}

// styledFace applies the cascade's face-level settings to f: weight
// (font-weight the resolver did not already serve) on a variable
// face's wght axis, font-variation-settings over it (CSS lets the
// explicit axis win), the optical size on the opsz axis while
// font-optical-sizing is auto, the OpenType features of
// font-feature-settings and the font-variant family (explicit
// settings win per tag), and letter and word spacing. Every derived
// face is memoized, so the shaping cache keeps hitting.
func styledFace(f render.Font, v *style.Values, weight, px int) render.Font {
	vars := cascadeVariations(v, weight, px)
	feats := faceFeatureSpec(v)
	letter, word := 0.0, 0.0
	if v.Has(style.PropLetterSpacing) {
		letter = v.LetterSpacing
	}
	if v.Has(style.PropWordSpacing) {
		word = v.WordSpacing
	}
	switch t := f.(type) {
	case *render.Typeface:
		t = t.WithVariations(vars...).WithFeatureSpec(feats).Spaced(letter).WordSpaced(word)
		return t
	case *render.Chain:
		t = t.WithVariations(vars...).WithFeatureSpec(feats).Spaced(letter).WordSpaced(word)
		return t
	}
	return f
}

// faceKey is what a cascade-styled face depends on.
type faceKey struct {
	base                 render.Font
	weight               int
	px                   int
	letter, word         float64
	features, variations string
}

// faceCache memoizes a widget's cascade-styled faces (styledFace) per
// base face and the cascade values that shape them, so the measuring
// and painting paths reuse one derived face without re-deriving it.
type faceCache struct{ faces map[faceKey]render.Font }

// get is base styled by v (weight: a font-weight the caller did not
// already serve, 0 for none).
func (c *faceCache) get(base render.Font, v *style.Values, weight, px int) render.Font {
	k := faceKey{base: base, weight: weight, px: px}
	if v.Has(style.PropLetterSpacing) {
		k.letter = v.LetterSpacing
	}
	if v.Has(style.PropWordSpacing) {
		k.word = v.WordSpacing
	}
	if v.HasAny(style.PropFontFeatures, style.PropFontVariantCaps, style.PropFontVariantNumeric,
		style.PropFontVariantLigatures, style.PropFontVariantEastAsian,
		style.PropFontVariantPosition, style.PropFontVariantAlternates) {
		k.features = faceFeatureSpec(v)
	}
	if v.Has(style.PropFontVariations) {
		k.variations = v.Variations
	}
	if f, ok := c.faces[k]; ok {
		return f
	}
	if c.faces == nil || len(c.faces) > 16 {
		c.faces = map[faceKey]render.Font{}
	}
	f := styledFace(base, v, weight, px)
	c.faces[k] = f
	return f
}

// cssDecoration is the cascade's text decoration, zero when none.
func cssDecoration(v *style.Values) render.Decoration {
	if v.Has(style.PropTextDecorationLine) {
		return v.Decoration
	}
	return render.Decoration{}
}

// cascadeVariations is the axis settings the cascade implies: wght
// from weight (0 for none), the optical size in points while
// font-optical-sizing is auto (the CSS default - a face without an
// opsz axis ignores it), then font-variation-settings, a later tag
// replacing an earlier one.
func cascadeVariations(v *style.Values, weight, px int) []render.Variation {
	var out []render.Variation
	set := func(tag string, val float32) {
		for i := range out {
			if out[i].Tag == tag {
				out[i].Value = val
				return
			}
		}
		out = append(out, render.Variation{Tag: tag, Value: val})
	}
	if weight > 0 {
		set("wght", float32(weight))
	}
	if px > 0 && (!v.Has(style.PropFontOpticalSizing) || v.OpticalSizing) {
		set("opsz", float32(px)*72.0/96.0)
	}
	if v.Has(style.PropFontVariations) {
		for kv := range strings.SplitSeq(v.Variations, ";") {
			tag, num, ok := strings.Cut(kv, "=")
			if f, err := strconv.ParseFloat(num, 32); ok && err == nil {
				set(tag, float32(f))
			}
		}
	}
	return out
}

// faceFeatureSpec is the OpenType feature list the cascade implies:
// the font-variant properties' tags first, then font-feature-settings
// (an explicit setting wins per tag), sorted into the canonical
// `tag=value` form the shaper's WithFeatureSpec takes. Empty is no
// features. The variant tags the font stack honors are the caps
// (smcp, c2sc, pcap, c2pc, unic, titl), numeric (lnum, onum, pnum,
// tnum, frac, afrc, ordn, zero), ligatures (liga, dlig, hlig, calt —
// enabling only; the no-* forms are the font's default-off state),
// east-asian (jp78, jp83, jp90, jp04, smpl, trad, fwid, pwid, ruby),
// position (subs, sups), and alternates (hist).
func faceFeatureSpec(v *style.Values) string {
	tags := map[string]string{}
	add := func(tag string) {
		tags[tag] = "1"
	}
	if v.Has(style.PropFontVariantCaps) {
		switch v.VariantCaps {
		case style.CapsSmallCaps:
			add("smcp")
		case style.CapsAllSmallCaps:
			add("smcp")
			add("c2sc")
		case style.CapsPetiteCaps:
			add("pcap")
		case style.CapsAllPetiteCaps:
			add("pcap")
			add("c2pc")
		case style.CapsUnicase:
			add("unic")
		case style.CapsTitlingCaps:
			add("titl")
		}
	}
	if v.Has(style.PropFontVariantNumeric) {
		n := v.VariantNumeric
		if n&style.NumLining != 0 {
			add("lnum")
		}
		if n&style.NumOldStyle != 0 {
			add("onum")
		}
		if n&style.NumProportional != 0 {
			add("pnum")
		}
		if n&style.NumTabular != 0 {
			add("tnum")
		}
		if n&style.NumDiagonalFractions != 0 {
			add("frac")
		}
		if n&style.NumStackedFractions != 0 {
			add("afrc")
		}
		if n&style.NumOrdinal != 0 {
			add("ordn")
		}
		if n&style.NumSlashedZero != 0 {
			add("zero")
		}
	}
	if v.Has(style.PropFontVariantLigatures) {
		for _, g := range ligatureFeatures {
			switch {
			case v.VariantLig&g.on != 0:
				for _, tag := range g.tags {
					tags[tag] = "1"
				}
			case v.VariantLig&g.off != 0:
				for _, tag := range g.tags {
					tags[tag] = "0"
				}
			}
		}
	}
	if v.Has(style.PropFontVariantEastAsian) {
		ea := v.VariantEA
		if ea&style.EAJis78 != 0 {
			add("jp78")
		}
		if ea&style.EAJis83 != 0 {
			add("jp83")
		}
		if ea&style.EAJis90 != 0 {
			add("jp90")
		}
		if ea&style.EAJis04 != 0 {
			add("jp04")
		}
		if ea&style.EASimplified != 0 {
			add("smpl")
		}
		if ea&style.EATraditional != 0 {
			add("trad")
		}
		if ea&style.EAFullWidth != 0 {
			add("fwid")
		}
		if ea&style.EAProportional != 0 {
			add("pwid")
		}
		if ea&style.EARuby != 0 {
			add("ruby")
		}
	}
	if v.Has(style.PropFontVariantPosition) {
		if v.VariantPos == style.PositionSub {
			add("subs")
		}
		if v.VariantPos == style.PositionSuper {
			add("sups")
		}
	}
	if v.Has(style.PropFontVariantAlternates) && v.VariantAlt == style.AlternatesHistorical {
		add("hist")
	}
	if v.Has(style.PropFontFeatures) {
		for kv := range strings.SplitSeq(v.Features, ";") {
			if kv != "" {
				tags[strings.Split(kv, "=")[0]] = kv[strings.Index(kv, "=")+1:]
			}
		}
	}
	if len(tags) == 0 {
		return ""
	}
	spec := make([]string, 0, len(tags))
	for tag, val := range tags {
		spec = append(spec, tag+"="+val)
	}
	slices.Sort(spec)
	return strings.Join(spec, ";")
}

// ligatureFeatures maps each font-variant-ligatures group's on and off
// keywords onto its OpenType tags (CSS Fonts 4 §6.4).
var ligatureFeatures = [...]struct {
	on, off style.FontVariantLigatures
	tags    []string
}{
	{style.LigCommon, style.LigNoCommon, []string{"clig", "liga"}},
	{style.LigDiscretionary, style.LigNoDiscretionary, []string{"dlig"}},
	{style.LigHistorical, style.LigNoHistorical, []string{"hlig"}},
	{style.LigContextual, style.LigNoContextual, []string{"calt"}},
}

// radiusOr returns the stylesheet's corner radii when any corner is
// set, else def on every corner.
func radiusOr(v *style.Values, def int) render.Corners {
	if !v.HasAny(style.PropBorderTopLeftRadius, style.PropBorderTopRightRadius,
		style.PropBorderBottomRightRadius, style.PropBorderBottomLeftRadius) {
		return render.UniformCorners(def)
	}
	r := v.Radius
	c := render.UniformCorners(def)
	if v.Has(style.PropBorderTopLeftRadius) {
		c.TopLeft = r.TopLeft
	}
	if v.Has(style.PropBorderTopRightRadius) {
		c.TopRight = r.TopRight
	}
	if v.Has(style.PropBorderBottomRightRadius) {
		c.BottomRight = r.BottomRight
	}
	if v.Has(style.PropBorderBottomLeftRadius) {
		c.BottomLeft = r.BottomLeft
	}
	return c
}

// paddingOr returns the stylesheet's padding per side where set, def
// elsewhere.
func paddingOr(v *style.Values, def render.Insets) render.Insets {
	if v.Has(style.PropPaddingTop) {
		def.Top = v.Padding.Top
	}
	if v.Has(style.PropPaddingRight) {
		def.Right = v.Padding.Right
	}
	if v.Has(style.PropPaddingBottom) {
		def.Bottom = v.Padding.Bottom
	}
	if v.Has(style.PropPaddingLeft) {
		def.Left = v.Padding.Left
	}
	return def
}

// borderOf returns the used border widths.
func borderOf(v *style.Values) render.Insets {
	b := v.EffBorder()
	return render.Insets{Top: b.Top, Right: b.Right, Bottom: b.Bottom, Left: b.Left}
}

// ringOr is the border widths the cascade declares, else a uniform
// px ring: the themed default ring a widget draws until a stylesheet
// takes the border over (check indicators, frames).
func ringOr(v *style.Values, px int) render.Insets {
	if v.HasAny(style.PropBorderTopWidth, style.PropBorderRightWidth, style.PropBorderBottomWidth, style.PropBorderLeftWidth) {
		return borderOf(v)
	}
	return render.UniformInsets(px)
}

// marginOf returns the margins.
func marginOf(v *style.Values) render.Insets {
	m := v.Margin
	return render.Insets{Top: m.Top, Right: m.Right, Bottom: m.Bottom, Left: m.Left}
}

// fontPx resolves the text size: the stylesheet's when matched, else
// the constructor default.
func fontPx(v *style.Values, def float64) float64 {
	if v.Has(style.PropFontSize) {
		return v.FontSize
	}
	return def
}

// typeElementName returns the widget type's element name: GTK's CSS
// node name where lys-ui has the GTK widget (an Icon is GtkImage's
// image, a Scroll GtkScrolledWindow's scrolledwindow, a TextArea
// GtkTextView's textview, a Slider GtkScale's scale, a List GtkListView's
// listview, a RichLabel a label), so a GTK stylesheet applies as
// written; else the lowercased Go type name. The list is pinned by
// TestCSSElementNames; a type missing from the switch matches no
// element selector — the same graceful no-op as an unknown name in a
// stylesheet.
func typeElementName(w Widget) string {
	switch w.(type) {
	case *Box:
		return "box"
	case *Button, *ToggleButton, *ColorButton, *BarButton:
		return "button"
	case *Calendar:
		return "calendar"
	case *CheckButton:
		return "checkbutton"
	case *ColorChooser:
		return "colorchooser"
	case *Dropdown:
		return "dropdown"
	case *DropdownOf[any]:
		return "dropdown"
	case *Elevation:
		return "elevation"
	case *Entry:
		return "entry"
	case *Expander:
		return "expander"
	case *Fader:
		return "fader"
	case *Grid:
		return "grid"
	case *Icon:
		return "image"
	case *Image:
		return "image"
	case *Label:
		return "label"
	case *LevelBar:
		return "levelbar"
	case *List:
		return "listview"
	case *GridView:
		return "gridview"
	case *Inscription:
		return "inscription"
	case *listRow:
		return "row"
	case *Menu:
		return "menu"
	case *Notebook:
		return "notebook"
	case *Overlay:
		return "overlay"
	case *Paned:
		return "paned"
	case *ProgressBar:
		return "progressbar"
	case *RichLabel:
		return "label"
	case *Scroll:
		return "scrolledwindow"
	case *Separator:
		return "separator"
	case *Slider:
		return "scale"
	case *Spinner:
		return "spinner"
	case *Stack:
		return "stack"
	case *Switch:
		return "switch"
	case *TextArea:
		return "textview"
	case *Toast:
		return "toast"
	default:
		return ""
	}
}
