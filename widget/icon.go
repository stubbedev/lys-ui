package widget

import (
	"image"
	"os"
	"path/filepath"

	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/icons"
	"github.com/stubbedev/lys-ui/internal/style"
)

// iconKind says where an icon's pixels come from.
type iconKind uint8

const (
	iconStatic iconKind = iota // rasterized up front by the caller
	iconTheme                  // looked up in the icon theme by name
	iconFile                   // loaded from a file path
	iconSVG                    // rasterized from embedded SVG bytes
)

// Icon paints an icon at its natural size. The pixels come from one of
// four sources:
//
//   - NewIcon: a raster the caller already made (render.LoadSVG and
//     friends); the widget draws it as is.
//   - NewThemeIcon: an icon theme name, resolved through the shared
//     icons.Cache at the canvas device scale and re-resolved when the
//     theme or the accent moves.
//   - NewFileIcon: an image file (png, svg, svgz).
//   - NewSVGIcon: embedded SVG bytes, for apps that ship their own
//     glyphs.
//
// A dynamic icon never reads or decodes on the UI loop: a paint draws
// what is decoded and otherwise asks for the decode off the loop, keeping
// the raster it has (nothing at first) until the result invalidates it.
// Without a running Application the decode runs inline at the paint.
//
// Symbolic sources - -symbolic theme files and SVGs painting with
// currentColor - recolor to the theme Accent so glyphs follow the
// palette; SetTint pins a different color. The recoloring is a mask
// pass (render.Icon.Tint): alpha is kept, ink takes the tint, so
// gradients collapse to their alpha ramp - the symbolic contract.
type Icon struct {
	node
	ic *render.Icon

	kind iconKind
	name string // theme icon name
	// fallbacks resolves name through its dash-shortened forms
	// (NewThemeIconFallback).
	fallbacks bool
	path      string // file path
	svg       []byte // embedded SVG bytes
	size      int    // logical box for the non-static kinds

	// tint pins the recolor color; zero follows the theme Accent.
	tint Color

	// resolution state: the tint and 120-based device scale the
	// current raster was made for, and the icons.Cache generation it
	// was resolved at (theme kind only). err holds the last resolution
	// failure; it sticks until a generation bump so a missing icon
	// costs one lookup, not one per frame.
	resolvedTint Color
	resolvedFrac uint32
	resolvedSize int
	resolvedGen  uint64
	err          error

	// base is the natural raster ic or retint was derived from.
	base *render.Icon
	// retint is the raster a mask recolor writes to, in place of ic.
	retint iconRetint
	// decoded is a file or SVG source's natural raster and the box it
	// was decoded for; a theme icon's lives in the icons.Cache.
	decoded iconDecoded
}

// iconBox is what a decode rasterizes for: the source's name when it has
// one, size logical pixels, and a 120-based device scale.
type iconBox struct {
	name string
	size int
	frac uint32
}

type iconDecoded struct {
	box    iconBox
	raster icons.Raster
}

// iconSelfKey names the decode of one widget's own source.
type iconSelfKey struct {
	icon *Icon
	box  iconBox
}

// iconThemeKey names the decode of a theme icon.
type iconThemeKey struct {
	box       iconBox
	fallbacks bool
}

// iconRetint is a raster the widget recolors in place, so a tint moving
// each frame reads no file and makes no raster. The version tells a GPU
// renderer to upload it again.
type iconRetint struct {
	img     *image.RGBA
	id      render.ImageID
	version uint64
	drawn   image.Image
	active  bool
}

// fill makes the raster base recolored to tint.
func (r *iconRetint) fill(base *render.Icon, tint Color) {
	src := base.Image()
	if r.img == nil || r.img.Rect != src.Rect {
		r.img = image.NewRGBA(src.Rect)
		r.id = render.NewImageID()
	}
	icons.TintInto(r.img, src, tint)
	r.version++
	r.drawn = render.VersionedImage{RGBA: r.img, ID: r.id, Version: r.version}
	r.active = true
}

// draw blends the raster, made for frac and covering base's logical size,
// as render.Icon.Draw places one.
func (r *iconRetint) draw(cv *render.Canvas, base *render.Icon, frac uint32, x, y int) {
	num, denom := cv.DeviceScale()
	if int(frac)*denom == num*iconScaleDenom {
		cv.DrawImageDevice(r.drawn, floorDiv(x*num, denom), floorDiv(y*num, denom))
		return
	}
	w, h := base.LogicalSize()
	cv.DrawImageRect(r.drawn, render.Rect{X: x, Y: y, W: w, H: h})
}

// iconScaleDenom is the denominator of the 120-based device scale.
const iconScaleDenom = 120

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// NewIcon returns an icon widget around a rasterized icon.
func NewIcon(ic *render.Icon) *Icon {
	return &Icon{ic: ic}
}

// NewThemeIcon returns an icon widget that resolves name from the icon
// theme at size logical pixels, rasterized at the canvas device scale.
// Symbolic icons recolor to the theme Accent. Resolution failures paint
// nothing and surface through Err.
func NewThemeIcon(name string, size int) *Icon {
	return &Icon{kind: iconTheme, name: name, size: size}
}

// NewThemeIconFallback is NewThemeIcon with GTK's default fallbacks
// for app icons (GThemedIcon): a missing name resolves to its
// dash-shortened forms in turn, preferences-system-network to
// preferences-system, then preferences. A desktop entry's Icon= names
// a specific icon a theme may lack; the fallback re-resolves when the
// theme changes.
func NewThemeIconFallback(name string, size int) *Icon {
	i := NewThemeIcon(name, size)
	i.fallbacks = true
	return i
}

// ThemeIconExists reports whether name resolves in the current icon
// theme chain (hicolor included) or the unthemed fallback
// directories, at any size - GTK's IconTheme.has_icon. Apps use it to
// pick between candidate names (a distro logo, then a generic one)
// before building a NewThemeIcon, since a missing theme icon paints
// nothing.
func ThemeIconExists(name string) bool {
	if name == "" {
		return false
	}
	// The spec's nearest pass accepts any size, so one lookup at the
	// common 16px request at 1x (frac 120) answers for every size.
	_, err := icons.Lookup(name, 16, 120)
	return err == nil
}

// IconSearchPaths returns the base directories theme icons resolve
// from: what SetIconSearchPaths set, else the XDG default (GTK's
// IconTheme.search_path).
func IconSearchPaths() []string {
	return icons.Default().SearchPaths()
}

// SetIconSearchPaths replaces the base directories theme icons resolve
// from, in priority order (GTK's IconTheme.set_search_path); nil
// restores the XDG default. Live themed icons re-resolve on the next
// repaint, which the application requests.
func SetIconSearchPaths(paths []string) {
	icons.Default().SetSearchPaths(paths)
}

// SetIconNamespace declares icon names starting with one of prefixes
// and a dash to be the application's own, kept in dir (relative) below
// the icon search paths, such as "hicolor/scalable/actions". The first
// themed icon an application draws otherwise lists every directory of
// the theme chain; these names need only dir, so an application drawing
// nothing else never lists the chain. A theme cannot override them. A
// name dir does not hold resolves as usual, so ThemeIconExists stays
// exact.
func SetIconNamespace(dir string, prefixes []string) {
	icons.Default().SetNamespace(dir, prefixes)
}

// RefreshIcons forgets every resolved theme icon, a found file and a
// miss alike, so icons installed or removed under the search paths
// since are picked up; live themed icons re-resolve on the repaint the
// application requests. Safe from any goroutine.
func RefreshIcons() {
	icons.Default().InvalidateTheme()
}

// NewFileIcon returns an icon widget that loads an image file (png,
// svg, svgz) and draws it at size logical pixels.
func NewFileIcon(path string, size int) *Icon {
	return &Icon{kind: iconFile, path: path, size: size}
}

// NewSVGIcon returns an icon widget that rasterizes embedded SVG bytes
// at size logical pixels.
func NewSVGIcon(svg []byte, size int) *Icon {
	return &Icon{kind: iconSVG, svg: svg, size: size}
}

// SetTint pins the symbolic recolor color; the zero color (the default)
// follows the theme Accent. On a static icon the raster is already
// final, so the tint is applied once, immediately, and cannot be
// undone.
func (i *Icon) SetTint(tint Color) {
	if tint == i.tint {
		return
	}
	i.tint = tint
	if i.kind == iconStatic {
		if i.ic != nil && tint != 0 {
			i.ic = i.ic.Tint(tint)
		}
		return
	}
	i.expire()
}

// Tint returns the pinned recolor color; the zero color (the default)
// follows the theme Accent.
func (i *Icon) Tint() Color { return i.tint }

// Name returns the theme icon's name; empty for the other kinds.
func (i *Icon) Name() string { return i.name }

// SetThemeName swaps a theme-resolved icon's name - the state-icon
// pattern (battery, volume, network): one widget, the glyph follows
// the state. A non-theme icon ignores the call; the next paint
// rasterizes the new glyph.
func (i *Icon) SetThemeName(name string) {
	if i.kind != iconTheme || i.name == name {
		return
	}
	i.name = name
	i.err = nil
	i.expire()
}

// Err returns the resolution failure of a dynamic icon: an unknown
// theme name, a missing file, or undecodable data. Nil until a decode a
// paint asked for has failed.
func (i *Icon) Err() error { return i.err }

// effSize is the icon box: the stylesheet's -gtk-icon-size when set
// (it inherits, GTK's rule), else the constructor size.
func (i *Icon) effSize() int {
	return picki(i.style(i), style.PropIconSize, i.size)
}

// natural is the icon's content size.
func (i *Icon) natural() Size {
	if i.kind == iconStatic && i.ic != nil {
		w, h := i.ic.LogicalSize()
		return Size{W: w, H: h}
	}
	s := i.effSize()
	return Size{W: s, H: s}
}

// Measure returns the icon's pixel size inside the CSS box, clamped to
// con.
func (i *Icon) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := i.measureHit(axis, forSize); ok {
		return sz
	}
	v := i.style(i)
	return i.measureStore(axis, forSize, measureBox(v, boxOf(v, render.Insets{}), axis, forSize, func(axis Axis, forSize int) Sizes {
		return FixedSizes(i.natural().Along(axis))
	}))
}

// Arrange records the border box inside r, the margin box.
func (i *Icon) Arrange(r render.Rect) {
	border, _ := boxRects(boxOf(i.style(i), render.Insets{}), r)
	i.node.Arrange(border)
}

// Paint draws the icon centered in the content box, as GtkImage
// centers its paintable (the CSS box layers first, when the stylesheet
// gives the icon any). Dynamic icons
// resolve first, re-rasterizing when the canvas device scale, the tint,
// the stylesheet's size, or the icon theme cache moved on since the
// last frame, with what is already decoded.
func (i *Icon) Paint(cv *render.Canvas) {
	v := i.style(i)
	b := boxOf(v, render.Insets{})
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, i.bounds, radii)
	cv = fx.cv
	defer fx.pop()
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, i.bounds, radii, b.border, bg)
	}
	defer paintOutline(cv, v, i.bounds, radii)
	if i.kind != iconStatic {
		i.resolveFor(cv)
		if i.ic == nil && !i.retint.active {
			return
		}
	}
	content := b.padding.Shrink(b.border.Shrink(i.bounds))
	nat := i.natural()
	x, y := content.X+(content.W-nat.W)/2, content.Y+(content.H-nat.H)/2
	if v.Has(style.PropIconTransform) && v.IconXform.M != render.Identity {
		if i.retint.active {
			i.ic, i.retint.active = i.base.Tint(i.resolvedTint), false
		}
		i.ic.DrawXformed(cv, x, y, v.IconXform.M)
		return
	}
	if i.retint.active {
		i.retint.draw(cv, i.base, i.resolvedFrac, x, y)
		return
	}
	i.ic.Draw(cv, x, y)
}

// HitTest returns the icon when p is inside its bounds.
func (i *Icon) HitTest(p Point) Widget {
	return i.HitLeaf(i, p)
}

// takeDamage drains the icon's pending repaint. A themed icon first
// expires its raster when the icons.Cache generation moved on (a
// SetTheme or InvalidateTheme call - there is no live theme-change
// signal by design; see icons.Cache), so the repaint this pass owes
// draws the new theme. Without the check the damage collector would
// see a clean widget and skip the switch. An icon that never resolved holds
// nothing to expire: its first paint reads the current theme.
func (i *Icon) takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool) {
	if i.kind == iconTheme && i.resolvedFrac != 0 && i.resolvedGen != icons.Default().Generation() {
		i.expire()
	}
	return i.node.takeDamage()
}

// expire marks the resolved raster out of date, so the next paint
// re-resolves, and schedules a repaint. The raster stays on show until
// its replacement is decoded.
func (i *Icon) expire() {
	i.err = nil
	i.resolvedTint = 0
	i.resolvedFrac = 0
	i.resolvedGen = 0
	i.Invalidate()
}

// resolveFor re-resolves the source when the last raster no longer
// matches this canvas.
func (i *Icon) resolveFor(cv *render.Canvas) {
	num, denom := cv.DeviceScale()
	frac := fracOf(num, denom)
	tint := i.tint
	if tint == 0 {
		// The stylesheet's color recolors symbolic glyphs, GTK's rule;
		// without one they follow the theme accent.
		tint = pickc(0, i.style(i), style.PropColor, Current().Accent)
	}
	if !i.stale(frac, tint) {
		return
	}
	i.resolve(frac, tint)
}

// stale reports whether the resolved raster predates the canvas's
// device scale, the effective tint, or the icon theme cache generation.
func (i *Icon) stale(frac uint32, tint Color) bool {
	if i.kind == iconTheme && i.resolvedGen != icons.Default().Generation() {
		return true
	}
	if i.err != nil {
		return false
	}
	if i.ic == nil && !i.retint.active {
		return true
	}
	return i.resolvedFrac != frac || i.resolvedTint != tint || i.resolvedSize != i.effSize()
}

// resolve brings the raster up to date with what is decoded for this
// canvas, asking for the decode when nothing is.
func (i *Icon) resolve(frac uint32, tint Color) {
	box := iconBox{name: i.name, size: i.effSize(), frac: frac}
	var gen uint64
	if i.kind == iconTheme {
		gen = icons.Default().Generation()
	}
	src, ok := i.peek(box)
	if !ok {
		i.request(box)
		if src, ok = i.peek(box); !ok {
			i.showNearest(box, tint)
			return
		}
	}
	i.resolvedFrac, i.resolvedTint, i.resolvedSize, i.resolvedGen = frac, tint, box.size, gen
	if src.Err != nil {
		i.err, i.ic, i.base, i.retint.active = src.Err, nil, nil, false
		return
	}
	i.err = nil
	i.apply(src, tint)
}

func (i *Icon) showNearest(box iconBox, tint Color) {
	if i.kind != iconTheme || i.ic != nil || i.retint.active {
		return
	}
	name, ok := i.themeName(box.name)
	if !ok {
		return
	}
	if src, ok := icons.Default().PeekNearest(name, box.size, box.frac); ok {
		i.apply(src, tint)
	}
}

func (i *Icon) themeName(name string) (string, bool) {
	if !i.fallbacks {
		return name, true
	}
	return icons.Default().PeekFallback(name)
}

// apply derives the raster on show from src: the natural one, a shared
// recolor the first time a themed raster is tinted, and the widget's own
// reused one for every other recolor.
func (i *Icon) apply(src icons.Raster, tint Color) {
	switch mask := i.tint != 0 || src.Symbolic; {
	case !mask:
		i.ic, i.retint.active = src.Icon, false
	case i.kind == iconTheme && i.base != src.Icon:
		i.ic, i.retint.active = icons.Default().Tint(src, tint), false
	default:
		i.ic = nil
		i.retint.fill(src.Icon, tint)
	}
	i.base = src.Icon
}

// peek returns the source decoded for box if it is, reading nothing.
func (i *Icon) peek(box iconBox) (icons.Raster, bool) {
	if i.kind != iconTheme {
		return i.decoded.raster, i.decoded.box == box
	}
	name, ok := i.themeName(box.name)
	if !ok {
		return icons.Raster{}, false
	}
	return icons.Default().Peek(name, box.size, box.frac)
}

// request asks for box's decode off the loop; its answer repaints the
// widget, which then peeks again.
func (i *Icon) request(box iconBox) {
	answered := func() {
		if hasInvoker() {
			i.Invalidate()
		}
	}
	if i.kind == iconTheme {
		fallbacks := i.fallbacks
		loadAsset(iconThemeKey{box, fallbacks}, func() { preloadThemeBox(box, fallbacks) }, answered)
		return
	}
	kind, path, svg := i.kind, i.path, i.svg
	var raster icons.Raster
	loaded := false
	loadAsset(iconSelfKey{i, box}, func() {
		raster, loaded = decodeIconSource(kind, path, svg, box), true
	}, func() {
		if loaded {
			i.decoded = iconDecoded{box: box, raster: raster}
		}
		answered()
	})
}

// preloadThemeBox decodes a theme icon for box into the cache.
func preloadThemeBox(box iconBox, fallbacks bool) {
	cache := icons.Default()
	name := box.name
	if fallbacks {
		name = cache.FallbackName(name)
	}
	cache.Preload(name, box.size, box.frac)
}

var decodeIcon = icons.DecodeIcon

// decodeIconSource reads and decodes a file or SVG source; it reads only
// its arguments, so it may run off the loop.
func decodeIconSource(kind iconKind, path string, svg []byte, box iconBox) icons.Raster {
	data, name := svg, "icon.svg"
	if kind == iconFile {
		var err error
		name = filepath.Base(path)
		// #nosec G304 -- the path is the application's own NewFileIcon argument.
		if data, err = os.ReadFile(path); err != nil {
			return icons.Raster{Err: err}
		}
	}
	ic, err := decodeIcon(data, name, box.size, box.frac)
	return icons.Raster{Icon: ic, Symbolic: icons.IsSymbolic(name, data), Err: err}
}

// fracOf converts a canvas device scale (num device pixels per denom
// logical pixels) to the 120-based convention of internal/scale: 120 is
// 1x, 240 is 2x. The app pipeline always feeds canvases a denominator
// of 120, so the conversion is exact there.
func fracOf(num, denom int) uint32 {
	if num <= 0 || denom <= 0 {
		return 0
	}
	return intconv.Clamp[uint32]((num*120 + denom/2) / denom)
}

// PreloadThemeIcon decodes the icon NewThemeIconFallback(name, size)
// will paint at the device scale num/denom, so its first paint reads and
// rasterizes nothing. It reads the disk, so it belongs off the UI loop,
// and it is safe from any goroutine. name may also be an absolute path.
func PreloadThemeIcon(name string, size, num, denom int) {
	cache := icons.Default()
	frac := fracOf(num, denom)
	if fallback := cache.FallbackName(name); fallback != name {
		cache.Preload(fallback, size, frac)
	}
	cache.Preload(name, size, frac)
}

// ThemeIconPreloaded reports whether PreloadThemeIcon's work is done for
// this icon, which a paint also does. It never blocks, so the UI loop may
// ask: while the cache is busy the answer is no.
func ThemeIconPreloaded(name string, size, num, denom int) bool {
	return icons.Default().Cached(name, size, fracOf(num, denom))
}
