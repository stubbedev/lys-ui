package widget

import (
	"os"
	"path/filepath"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/icons"
	"github.com/stubbedev/lys-ui/internal/intconv"
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
// theme name, a missing file, or undecodable data. Nil until a paint
// tried and failed.
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
// last frame.
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
		if i.ic == nil {
			return
		}
	}
	content := b.padding.Shrink(b.border.Shrink(i.bounds))
	nat := i.natural()
	x, y := content.X+(content.W-nat.W)/2, content.Y+(content.H-nat.H)/2
	if v.Has(style.PropIconTransform) && v.IconXform.M != render.Identity {
		i.ic.DrawXformed(cv, x, y, v.IconXform.M)
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
// see a clean widget and skip the switch.
func (i *Icon) takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool) {
	if i.kind == iconTheme && i.resolvedGen != icons.Default().Generation() {
		i.expire()
	}
	return i.node.takeDamage()
}

// expire drops the resolved raster so the next paint re-resolves, and
// schedules a repaint.
func (i *Icon) expire() {
	i.ic = nil
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
	if i.ic == nil {
		return true
	}
	return i.resolvedFrac != frac || i.resolvedTint != tint || i.resolvedSize != i.effSize()
}

// resolve rasterizes the source into a device box of
// icons.DeviceBox(size, frac) pixels and records what it was made for.
func (i *Icon) resolve(frac uint32, tint Color) {
	size := i.effSize()
	ic, err := i.rasterize(frac, tint, size)
	i.resolvedFrac, i.resolvedTint, i.resolvedSize = frac, tint, size
	if i.kind == iconTheme {
		i.resolvedGen = icons.Default().Generation()
	}
	if err != nil {
		i.err, i.ic = err, nil
		return
	}
	i.err, i.ic = nil, ic
}

// rasterize decodes the source for size logical pixels at frac,
// mask-recoloring it to tint when it is symbolic or a tint is pinned.
func (i *Icon) rasterize(frac uint32, tint Color, size int) (*render.Icon, error) {
	switch i.kind {
	case iconTheme:
		cache := icons.Default()
		name := i.name
		if i.fallbacks {
			name = cache.FallbackName(name)
		}
		if i.tint != 0 {
			return cache.Tinted(name, size, frac, tint)
		}
		return cache.SymbolicIcon(name, size, frac, tint)
	case iconFile:
		data, err := os.ReadFile(i.path)
		if err != nil {
			return nil, err
		}
		return tintedImage(data, filepath.Base(i.path), size, frac, tint, i.tint != 0 || icons.IsSymbolic(filepath.Base(i.path), data))
	default:
		return tintedImage(i.svg, "icon.svg", size, frac, tint, i.tint != 0 || icons.IsSymbolic("icon.svg", i.svg))
	}
}

// tintedImage decodes data for size logical pixels at frac and, when
// mask is set, recolors it: every pixel keeps its alpha and takes tint.
func tintedImage(data []byte, name string, size int, frac uint32, tint Color, mask bool) (*render.Icon, error) {
	ic, err := icons.DecodeIcon(data, name, size, frac)
	if err != nil {
		return nil, err
	}
	if mask {
		ic = ic.Tint(tint)
	}
	return ic, nil
}

// fracOf converts a canvas device scale (num device pixels per denom
// logical pixels) to the 120-based convention of internal/scale: 120 is
// 1x, 240 is 2x. The app pipeline always feeds canvases a denominator
// of 120, so the conversion is exact there.
func fracOf(num, denom int) uint32 {
	if num <= 0 || denom <= 0 {
		return 0
	}
	return intconv.Uint32((num*120 + denom/2) / denom)
}
