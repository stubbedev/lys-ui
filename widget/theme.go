package widget

import (
	"cmp"
	"fmt"
	"math"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/render"
)

// Theme holds the palette and metrics widgets paint with. Colors left at
// zero on a widget fall back to the theme at paint time, so replacing the
// theme restyles everything on the next frame.
//
// A Theme is an immutable value: the With* methods return a fresh copy
// and never touch the receiver, so chains compose from any preset —
//
//	widget.SetTheme(widget.DarkTheme().WithAccent(green).WithPadding(10))
//
// Colors arrive from config files as hex strings; parse them with
// ParseColor or set them with WithAccentHex, which errors on malformed
// input instead of defaulting silently.
type Theme struct {
	// Bg is the window background; Surface fills controls at rest.
	Bg, Surface Color
	// SurfaceHover and SurfacePressed restyle controls under the
	// pointer or while pressed. Leave them unset and HoverSurface
	// and PressedSurface derive them from Surface.
	SurfaceHover, SurfacePressed Color
	// Text paints primary labels; TextMuted groups and hints. Leave
	// TextMuted unset and DisabledText derives it from Text.
	Text, TextMuted Color
	// SurfaceDisabled fills disabled controls. Leave it unset and
	// DisabledSurface derives it from Surface and Bg.
	SurfaceDisabled Color
	// Accent drives fills that express state: progress, switches on,
	// sliders. HoverAccent derives the translucent hover wash from it.
	Accent Color
	// OnAccent paints glyphs and knobs on top of Accent.
	OnAccent Color
	// Border strokes control outlines.
	Border Color
	// Card fills cards and Popover floating surfaces (menus,
	// popovers, dropdown lists). Leave them unset and CardColor and
	// PopoverColor derive them from Surface.
	Card, Popover Color
	// Muted fills quiet regions: tab lists, skeletons, table headers,
	// code spans. Leave it unset and MutedColor derives it from
	// Surface.
	Muted Color
	// Secondary fills the secondary button and badge, shadcn's
	// --secondary: a fill of its own, so an app that paints panels in
	// Surface can keep the button visible on them. Leave it unset and
	// SecondaryColor derives it from MutedColor.
	Secondary Color
	// Sidebar fills an application sidebar. Leave it unset and
	// SidebarColor derives it from Surface.
	Sidebar Color
	// Chart colors a chart's series, shadcn's chart-1 to chart-5; an
	// unset one is ChartColor's fallback (Accent).
	Chart [5]Color
	// Input strokes the borders of text fields and other inputs.
	// Leave it unset and InputColor derives it from Border.
	Input Color
	// Ring paints the focus ring around the focused control. Leave it
	// unset and RingColor derives it from Accent.
	Ring Color
	// Destructive, Success, Warning, and Error are the status fills
	// (destructive-action buttons, banners, the @*_bg_color names).
	// Leave them unset to derive the Adwaita defaults for the
	// palette's polarity - the zero-falls-back-to-derived rule.
	Destructive, Success, Warning, Error Color
	// ShadowColor paints floating surfaces' box shadows (menus,
	// popovers, tooltips, toasts, dialogs). Leave it unset to derive
	// the default (neutral black at 110 alpha), the usual zero-falls-
	// back-to-derived rule.
	ShadowColor Color
	// ShadowBlur is the shadow's falloff radius in pixels around every
	// floating surface, and the on/off switch: zero disables client
	// shadows entirely — for compositors that already blur under
	// translucent surfaces, where a client shadow would double up.
	ShadowBlur int
	// Radius is the default corner rounding; Spacing and Padding are
	// layout defaults.
	Radius  int
	Spacing int
	Padding int
	// TextSize is the default label size in pixels.
	TextSize float64
	// MotionShort, MotionMedium and MotionLong are the motion tokens:
	// the durations of a state change (hover, press), a surface opening
	// or closing, and a large move. Stylesheets read them as
	// var(--lys-duration-short) and friends. Leave them unset for the
	// toolkit's (anim.DurationShort and friends).
	MotionShort, MotionMedium, MotionLong time.Duration
	// Animations gates the animated enter/exit (and open/reveal) motion:
	// surface tweens, toast slides, dropdown lists. False collapses the
	// durations to zero — the reduced-motion switch beside
	// LYS_UI_NO_ANIM=1 and surfx.SetEnabled(false); state machines run
	// unchanged, only the motion goes away.
	Animations bool
}

// Color aliases render.Color so themes read naturally without importing
// render at every call site.
type Color = render.Color

// themeGen bumps on every SetTheme so the damage collector repaints
// each widget once after a restyle (node.takeDamage compares stamps).
var themeGen uint64 = 1

var current = DarkTheme()

// Current returns the active theme.
func Current() *Theme { return current }

// SetTheme replaces the active theme. Passing nil restores DarkTheme.
// The next frame repaints every widget with the new palette. Any
// palette pair whose contrast falls below WCAG AA is reported at Warn
// on the injected library logger (degraded but running: the swap
// proceeds); with the default logger the warning is discarded and
// SetTheme never fails.
func SetTheme(t *Theme) {
	if t == nil {
		t = DarkTheme()
	}
	current = t
	themeGen++
	markDamage()
	syncThemeSheet(t)
	for _, w := range t.contrastWarnings() {
		themeWarn(w)
	}
}

// themeWarn is the contrast-warning sink: warnings route to the
// injected library logger at Warn level — the one app-facing event the
// widget package emits, so an application that opts into lys-ui's
// logging learns its palette is hard to read even in prod builds.
// Tests swap it to capture warnings.
var themeWarn = func(msg string) { logutil.L().Warn(msg) }

// DarkTheme returns the default dark palette: shadcn/ui v4's
// neutral, a light primary on near-black, translucent hairlines.
func DarkTheme() *Theme {
	return &Theme{
		Bg:             render.RGB(0x0A, 0x0A, 0x0A),
		Surface:        render.RGB(0x26, 0x26, 0x26),
		SurfaceHover:   render.RGB(0x21, 0x21, 0x21),
		SurfacePressed: render.RGB(0x1C, 0x1C, 0x1C),
		Text:           render.RGB(0xFA, 0xFA, 0xFA),
		TextMuted:      render.RGB(0xA1, 0xA1, 0xA1),
		Accent:         render.RGB(0xE5, 0xE5, 0xE5),
		OnAccent:       render.RGB(0x17, 0x17, 0x17),
		Border:         render.RGBA(0xFF, 0xFF, 0xFF, 0x1A),
		Card:           render.RGB(0x17, 0x17, 0x17),
		Popover:        render.RGB(0x17, 0x17, 0x17),
		Muted:          render.RGB(0x26, 0x26, 0x26),
		Sidebar:        render.RGB(0x17, 0x17, 0x17),
		Chart: [5]Color{
			render.RGB(0x14, 0x47, 0xE6), render.RGB(0x00, 0xBC, 0x7D), render.RGB(0xFE, 0x9A, 0x00),
			render.RGB(0xAD, 0x46, 0xFF), render.RGB(0xFF, 0x20, 0x56),
		},
		Input:       render.RGBA(0xFF, 0xFF, 0xFF, 0x26),
		Ring:        render.RGB(0x73, 0x73, 0x73),
		Destructive: render.RGB(0xFF, 0x64, 0x67),
		ShadowColor: render.RGBA(0, 0, 0, 100),
		ShadowBlur:  16,
		Radius:      8,
		Spacing:     8,
		Padding:     8,
		TextSize:    14,
		Animations:  true,
	}
}

// LightTheme returns the default light palette: shadcn/ui v4's
// neutral, a near-black primary on white.
func LightTheme() *Theme {
	return &Theme{
		Bg:             render.RGB(0xFF, 0xFF, 0xFF),
		Surface:        render.RGB(0xF5, 0xF5, 0xF5),
		SurfaceHover:   render.RGB(0xF7, 0xF7, 0xF7),
		SurfacePressed: render.RGB(0xE5, 0xE5, 0xE5),
		Text:           render.RGB(0x0A, 0x0A, 0x0A),
		TextMuted:      render.RGB(0x73, 0x73, 0x73),
		Accent:         render.RGB(0x17, 0x17, 0x17),
		OnAccent:       render.RGB(0xFA, 0xFA, 0xFA),
		Border:         render.RGB(0xE5, 0xE5, 0xE5),
		Card:           render.RGB(0xFF, 0xFF, 0xFF),
		Popover:        render.RGB(0xFF, 0xFF, 0xFF),
		Muted:          render.RGB(0xF5, 0xF5, 0xF5),
		Sidebar:        render.RGB(0xFA, 0xFA, 0xFA),
		Chart: [5]Color{
			render.RGB(0xF5, 0x49, 0x00), render.RGB(0x00, 0x96, 0x89), render.RGB(0x10, 0x4E, 0x64),
			render.RGB(0xFF, 0xB9, 0x00), render.RGB(0xFE, 0x9A, 0x00),
		},
		Input:       render.RGB(0xE5, 0xE5, 0xE5),
		Ring:        render.RGB(0xA1, 0xA1, 0xA1),
		Destructive: render.RGB(0xE7, 0x00, 0x0B),
		ShadowColor: render.RGBA(0, 0, 0, 40),
		ShadowBlur:  16,
		Radius:      8,
		Spacing:     8,
		Padding:     8,
		TextSize:    14,
		Animations:  true,
	}
}

// HighContrastTheme returns the high-contrast preset (#88): pure
// black on pure white, no muted tones, hairline chrome replaced by a
// solid border - the palette the desktop's contrast preference asks
// for. The WCAG guard in SetTheme is its acceptance test by
// construction: every pair passes AA with room to spare. Shadows and
// animations stay off; the point is legibility, not depth.
func HighContrastTheme() *Theme {
	return &Theme{
		Bg:             render.RGB(0xFF, 0xFF, 0xFF),
		Surface:        render.RGB(0xFF, 0xFF, 0xFF),
		SurfaceHover:   render.RGB(0xEE, 0xEE, 0xEE),
		SurfacePressed: render.RGB(0xDD, 0xDD, 0xDD),
		Text:           render.RGB(0x00, 0x00, 0x00),
		TextMuted:      render.RGB(0x00, 0x00, 0x00),
		Accent:         render.RGB(0x00, 0x3E, 0xB8),
		OnAccent:       render.RGB(0xFF, 0xFF, 0xFF),
		Border:         render.RGB(0x00, 0x00, 0x00),
		Radius:         2,
		Spacing:        8,
		Padding:        10,
		TextSize:       14,
		Animations:     false,
	}
}

// ParseColor reads a hex color — #rgb, #rrggbb, or #rrggbbaa, the same
// forms the markup parser accepts — into a premultiplied Color. It
// reports malformed input instead of defaulting, so a config file's
// mistake fails loudly at theme-construction time.
func ParseColor(s string) (Color, error) {
	c, ok := parseHexColor(s)
	if !ok {
		return 0, fmt.Errorf("lys-ui: invalid color %q: want #rgb, #rrggbb, or #rrggbbaa", s)
	}
	return c, nil
}

// FormatColor writes c as the hex form ParseColor reads back:
// #rrggbb when opaque, #rrggbbaa otherwise (straight alpha).
func FormatColor(c Color) string {
	col := c.Straight()
	if col[3] == 0xff {
		return fmt.Sprintf("#%02x%02x%02x", col[0], col[1], col[2])
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", col[0], col[1], col[2], col[3])
}

// with copies t and applies f to the copy. Every With* goes through
// here, so chaining is immutable by construction: the receiver keeps
// its values and each link in a chain is a distinct theme.
func (t *Theme) with(f func(*Theme)) *Theme {
	n := *t
	f(&n)
	return &n
}

// WithBg returns a copy with the window background set.
func (t *Theme) WithBg(c Color) *Theme { return t.with(func(n *Theme) { n.Bg = c }) }

// WithSurface returns a copy with the control fill set.
func (t *Theme) WithSurface(c Color) *Theme { return t.with(func(n *Theme) { n.Surface = c }) }

// WithSurfaceHover returns a copy with the hover shade set; unset, the
// copy derives one (HoverSurface).
func (t *Theme) WithSurfaceHover(c Color) *Theme {
	return t.with(func(n *Theme) { n.SurfaceHover = c })
}

// WithSurfacePressed returns a copy with the pressed shade set; unset,
// the copy derives one (PressedSurface).
func (t *Theme) WithSurfacePressed(c Color) *Theme {
	return t.with(func(n *Theme) { n.SurfacePressed = c })
}

// WithText returns a copy with the primary text color set.
func (t *Theme) WithText(c Color) *Theme { return t.with(func(n *Theme) { n.Text = c }) }

// WithTextMuted returns a copy with the secondary text color set.
func (t *Theme) WithTextMuted(c Color) *Theme {
	return t.with(func(n *Theme) { n.TextMuted = c })
}

// WithSurfaceDisabled returns a copy with the disabled-control fill
// set; unset, the copy derives one (DisabledSurface).
func (t *Theme) WithSurfaceDisabled(c Color) *Theme {
	return t.with(func(n *Theme) { n.SurfaceDisabled = c })
}

// WithAccent returns a copy with the accent color set.
func (t *Theme) WithAccent(c Color) *Theme { return t.with(func(n *Theme) { n.Accent = c }) }

// WithAccentHex returns a copy with the accent parsed from a hex
// string (#rgb, #rrggbb, #rrggbbaa). A malformed string is an error,
// never a silent default — theme construction is where a config
// mistake must surface.
func (t *Theme) WithAccentHex(s string) (*Theme, error) {
	c, err := ParseColor(s)
	if err != nil {
		return nil, err
	}
	return t.WithAccent(c), nil
}

// WithOnAccent returns a copy with the on-accent glyph color set.
func (t *Theme) WithOnAccent(c Color) *Theme { return t.with(func(n *Theme) { n.OnAccent = c }) }

// WithBorder returns a copy with the outline color set.
func (t *Theme) WithBorder(c Color) *Theme { return t.with(func(n *Theme) { n.Border = c }) }

// WithCard returns a copy with the card fill set.
func (t *Theme) WithCard(c Color) *Theme { return t.with(func(n *Theme) { n.Card = c }) }

// WithPopover returns a copy with the floating-surface fill set.
func (t *Theme) WithPopover(c Color) *Theme { return t.with(func(n *Theme) { n.Popover = c }) }

// WithMuted returns a copy with the quiet-region fill set.
func (t *Theme) WithMuted(c Color) *Theme { return t.with(func(n *Theme) { n.Muted = c }) }

// WithSecondary returns a copy with the secondary variant's fill set.
func (t *Theme) WithSecondary(c Color) *Theme { return t.with(func(n *Theme) { n.Secondary = c }) }

// WithSidebar returns a copy with the sidebar fill set.
func (t *Theme) WithSidebar(c Color) *Theme { return t.with(func(n *Theme) { n.Sidebar = c }) }

// WithInput returns a copy with the input border color set.
func (t *Theme) WithInput(c Color) *Theme { return t.with(func(n *Theme) { n.Input = c }) }

// WithRing returns a copy with the focus ring color set.
func (t *Theme) WithRing(c Color) *Theme { return t.with(func(n *Theme) { n.Ring = c }) }

// WithShadowColor returns a copy with the box-shadow color set. The
// zero color means unset (derive the default), the theme-wide
// convention.
func (t *Theme) WithShadowColor(c Color) *Theme {
	return t.with(func(n *Theme) { n.ShadowColor = c })
}

// WithShadowBlur returns a copy with the shadow falloff radius set;
// zero disables client shadows (see ShadowBlur).
func (t *Theme) WithShadowBlur(px int) *Theme {
	return t.with(func(n *Theme) { n.ShadowBlur = px })
}

// WithRadius returns a copy with the default corner rounding set.
func (t *Theme) WithRadius(px int) *Theme { return t.with(func(n *Theme) { n.Radius = px }) }

// WithSpacing returns a copy with the default layout gap set.
func (t *Theme) WithSpacing(px int) *Theme { return t.with(func(n *Theme) { n.Spacing = px }) }

// WithPadding returns a copy with the default control padding set.
func (t *Theme) WithPadding(px int) *Theme { return t.with(func(n *Theme) { n.Padding = px }) }

// WithTextSize returns a copy with the default label size set.
func (t *Theme) WithTextSize(px float64) *Theme { return t.with(func(n *Theme) { n.TextSize = px }) }

// WithMotion returns a copy with the motion tokens set; zero keeps a
// token at the toolkit's duration.
func (t *Theme) WithMotion(short, medium, long time.Duration) *Theme {
	return t.with(func(n *Theme) { n.MotionShort, n.MotionMedium, n.MotionLong = short, medium, long })
}

// MotionDurations returns the motion tokens in effect: the theme's, the
// toolkit's where unset, and zero throughout when Animations is off.
func (t *Theme) MotionDurations() (short, medium, long time.Duration) {
	if !t.Animations {
		return 0, 0, 0
	}
	return cmp.Or(t.MotionShort, anim.DurationShort), cmp.Or(t.MotionMedium, anim.DurationMedium), cmp.Or(t.MotionLong, anim.DurationLong)
}

// WithAnimations returns a copy with the animated-motion gate set;
// false is the theme-level reduced-motion switch (see Animations).
func (t *Theme) WithAnimations(on bool) *Theme {
	return t.with(func(n *Theme) { n.Animations = on })
}

// Derived state colors. The presets pin explicit shades; themes
// composed from scratch get the same states by derivation, so widgets
// read state colors through these methods instead of mixing their own.
// All derivations are pure functions of the palette: same palette in,
// same color out, on every widget.

// disabledFade is the opacity of disabled ink: the same fraction
// DisabledText fades Text by, applied by widgets to colors the theme
// cannot derive (an entry's constructor color, a button's child
// subtree through the canvas alpha stack).
const disabledFade = 0.45

// HoverSurface returns the control fill under the pointer: the
// explicit SurfaceHover when set, otherwise Surface mixed 8% toward
// Text. The text pole carries the palette's polarity, so the shift
// lightens dark palettes and darkens light ones.
func (t *Theme) HoverSurface() Color {
	if t.SurfaceHover != 0 {
		return t.SurfaceHover
	}
	return mix(t.Surface, t.Text, 0.08)
}

// CardColor returns the card fill: Card when set, otherwise Surface.
func (t *Theme) CardColor() Color { return t.or(t.Card, t.Surface) }

// PopoverColor returns the floating-surface fill: Popover when set,
// otherwise Surface.
func (t *Theme) PopoverColor() Color { return t.or(t.Popover, t.Surface) }

// MutedColor returns the quiet-region fill: Muted when set, otherwise
// Surface.
func (t *Theme) MutedColor() Color { return t.or(t.Muted, t.Surface) }

// SecondaryColor returns the secondary variant's fill: Secondary when
// set, otherwise MutedColor.
func (t *Theme) SecondaryColor() Color { return t.or(t.Secondary, t.MutedColor()) }

// ChartColor returns series i's color, cycling through the five chart
// colors; an unset one is Accent.
func (t *Theme) ChartColor(i int) Color { return t.or(t.Chart[i%len(t.Chart)], t.Accent) }

// SidebarColor returns the sidebar fill: Sidebar when set, otherwise
// Surface.
func (t *Theme) SidebarColor() Color { return t.or(t.Sidebar, t.Surface) }

// InputColor returns the input border color: Input when set,
// otherwise Border.
func (t *Theme) InputColor() Color { return t.or(t.Input, t.Border) }

// RingColor returns the focus ring color: Ring when set, otherwise
// Accent.
func (t *Theme) RingColor() Color { return t.or(t.Ring, t.Accent) }

// IsDark reports the palette's polarity: a background darker than
// mid-grey luminance.
func (t *Theme) IsDark() bool { return wcagLuminance(t.Bg) < 0.18 }

// status resolves one status color: the explicit value, else the
// Adwaita default for the palette's polarity.
func (t *Theme) status(c Color, dark, light Color) Color {
	switch {
	case c != 0:
		return c
	case t.IsDark():
		return dark
	}
	return light
}

// DestructiveColor returns the destructive fill (Destructive when set).
func (t *Theme) DestructiveColor() Color {
	return t.status(t.Destructive, render.RGB(0xc0, 0x1c, 0x28), render.RGB(0xe0, 0x1b, 0x24))
}

// SuccessColor returns the success fill (Success when set).
func (t *Theme) SuccessColor() Color {
	return t.status(t.Success, render.RGB(0x26, 0xa2, 0x69), render.RGB(0x2e, 0xc2, 0x7e))
}

// WarningColor returns the warning fill (Warning when set).
func (t *Theme) WarningColor() Color {
	return t.status(t.Warning, render.RGB(0xcd, 0x93, 0x09), render.RGB(0xe5, 0xa5, 0x0a))
}

// ErrorColor returns the error fill (Error when set).
func (t *Theme) ErrorColor() Color {
	return t.status(t.Error, render.RGB(0xc0, 0x1c, 0x28), render.RGB(0xe0, 0x1b, 0x24))
}

// PressedSurface returns the control fill while pressed: the explicit
// SurfacePressed when set, otherwise Surface mixed 20% toward black —
// the surface recedes under a press on any palette.
func (t *Theme) PressedSurface() Color {
	if t.SurfacePressed != 0 {
		return t.SurfacePressed
	}
	return mix(t.Surface, render.RGB(0, 0, 0), 0.20)
}

// HoverAccent returns the translucent accent wash rows and items paint
// under the pointer: Accent at alpha 70, the same derivation every
// highlighter uses.
func (t *Theme) HoverAccent() Color {
	return render.RGBA(t.Accent.R(), t.Accent.G(), t.Accent.B(), 70)
}

// DisabledText returns the color for rows and labels that ignore
// input: the explicit TextMuted when set, otherwise Text faded to
// disabledFade alpha.
func (t *Theme) DisabledText() Color {
	if t.TextMuted != 0 {
		return t.TextMuted
	}
	return scaleAlpha(t.Text, disabledFade)
}

// DisabledSurface returns the fill of a control that ignores input:
// the explicit SurfaceDisabled when set, otherwise Surface mixed
// halfway toward Bg — the control recedes into the window on any
// palette, the inert counterpart of PressedSurface.
func (t *Theme) DisabledSurface() Color {
	if t.SurfaceDisabled != 0 {
		return t.SurfaceDisabled
	}
	return mix(t.Surface, t.Bg, 0.5)
}

// DisabledAccent returns the accent fill (a switch's on-track, a
// slider's fill, a checkbox's tick) while the control ignores input:
// Accent faded to disabledFade, the same fade DisabledText applies to
// Text.
func (t *Theme) DisabledAccent() Color {
	return scaleAlpha(t.Accent, disabledFade)
}

// wcagAA is the WCAG 2.x AA contrast ratio for normal text.
const wcagAA = 4.5

// contrastWarnings reports one line per role whose contrast against
// the background falls below WCAG AA. Roles left unset (transparent)
// are skipped — they fall back to the presets at paint time. SetTheme
// traces the result in debug builds; it never fails the swap.
func (t *Theme) contrastWarnings() []string {
	var out []string
	pair := func(role string, fg Color) {
		if fg.A() != 255 || t.Bg.A() != 255 {
			return
		}
		if r := contrastRatio(fg, t.Bg); r < wcagAA {
			out = append(out, fmt.Sprintf(
				"theme %s/Bg contrast %.2f:1 is below WCAG AA (%.1f:1)", role, r, wcagAA))
		}
	}
	pair("Text", t.Text)
	pair("TextMuted", t.TextMuted)
	return out
}

// contrastRatio reports the WCAG 2.x contrast ratio of two opaque
// colors, (L1+0.05)/(L2+0.05) with L the relative luminance.
func contrastRatio(a, b Color) float64 {
	la, lb := wcagLuminance(a), wcagLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// wcagLuminance computes the WCAG 2.x relative luminance of an opaque
// sRGB color: the linearized channels under the perceptual weights.
func wcagLuminance(c Color) float64 {
	lin := func(p uint8) float64 {
		s := float64(p) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R()) + 0.7152*lin(c.G()) + 0.0722*lin(c.B())
}

// or picks b when a is the zero color.
func (t *Theme) or(a Color, b Color) Color {
	if a == 0 {
		return b
	}
	return a
}

// mix interpolates two premultiplied colors at p in [0, 1] — the same
// arithmetic the renderer uses for gradients: premultiplied channels
// interpolate correctly.
func mix(a, b Color, p float64) Color {
	ch := func(x, y uint8) uint32 {
		return uint32(float64(x) + (float64(y)-float64(x))*p + 0.5)
	}
	return Color(ch(a.A(), b.A())<<24 |
		ch(a.R(), b.R())<<16 |
		ch(a.G(), b.G())<<8 |
		ch(a.B(), b.B()))
}

// scaleAlpha fades a premultiplied color toward transparent by p in
// [0, 1]. Premultiplied channels scale linearly: each of alpha, red,
// green and blue is multiplied on its own (the packed value cannot be:
// its channels would carry into each other).
func scaleAlpha(c Color, p float64) Color {
	if p >= 1 {
		return c
	}
	if p <= 0 {
		return 0
	}
	ch := func(shift uint) Color {
		return Color(uint32(float64(uint8(c>>shift))*p+0.5)) << shift
	}
	return ch(24) | ch(16) | ch(8) | ch(0)
}
