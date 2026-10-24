// Package css names libadwaita's style classes and named colors - the
// relm4-css analog (#95) - so application code spells them as
// constants and stylesheets written for GTK port over verbatim:
//
//	button.AddClass(css.DestructiveAction)
//	widget.LoadStylesheet(`.sidebar { background-color: @window_bg_color; }`)
//
// The widget package's theme layer styles every class listed here and
// defines every named color from the current theme (widget.SetTheme),
// at StylePriorityTheme, so an application stylesheet overrides any of
// them and a theme switch re-derives them all.
package css

// Style classes, the libadwaita names.
const (
	// Accent paints text in the accent color.
	Accent = "accent"
	// Activatable marks a row as clickable as a whole.
	Activatable = "activatable"
	// Background is the window surface on a window's root: the
	// window's background and text colors. Popover content carries the
	// class too, as GTK's does, and keeps the popover's colors.
	Background = "background"
	// BoxedList is a list drawn as a rounded card with row separators.
	BoxedList = "boxed-list"
	// Caption is small secondary text.
	Caption = "caption"
	// CaptionHeading is small bold text.
	CaptionHeading = "caption-heading"
	// Card is a raised rounded surface.
	Card = "card"
	// Circular makes a button round.
	Circular = "circular"
	// DestructiveAction marks a button whose action loses data.
	DestructiveAction = "destructive-action"
	// Destructive is the destructive variant of a non-button widget
	// (a badge, an alert); buttons take DestructiveAction.
	Destructive = "destructive"
	// DimLabel fades text to secondary emphasis.
	DimLabel = "dim-label"
	// Error paints text in the error color.
	Error = "error"
	// Flat removes a button's resting background.
	Flat = "flat"
	// Glass is clear glass: a translucent surface over a blurred,
	// saturated backdrop.
	Glass = "glass"
	// Frosted is frosted glass: a heavier blur under a whiter tint and
	// a fine grain, so the surface reads as etched.
	Frosted = "frosted"
	// BlurUnavailable marks a window's root that asked the compositor
	// to blur behind it while no blur is in effect, so its style can
	// turn opaque instead of showing the unblurred desktop.
	BlurUnavailable = "blur-unavailable"
	// LiquidGlass is liquid glass: a nearly clear surface whose rim
	// bends the backdrop like a lens, under a specular edge.
	LiquidGlass = "liquid-glass"
	// Heading is bold body text.
	Heading = "heading"
	// Keycap draws a label as a key on a keyboard (a shortcut's keys).
	Keycap = "keycap"
	// Monospace sets the monospace family.
	Monospace = "monospace"
	// Numeric uses tabular figures (a hint; the face decides).
	Numeric = "numeric"
	// OSD is the dark translucent on-screen-display surface.
	OSD = "osd"
	// Pill makes a button a wide rounded pill.
	Pill = "pill"
	// Success paints text in the success color.
	Success = "success"
	// SuggestedAction marks the affirmative button.
	SuggestedAction = "suggested-action"
	// Title1 through Title4 are the display heading sizes.
	Title1 = "title-1"
	Title2 = "title-2"
	Title3 = "title-3"
	Title4 = "title-4"
	// The typography classes, shadcn/ui's: display headings H1 to H4,
	// Lead (a large muted intro), LargeText, SmallText, Muted
	// (secondary text), Blockquote and InlineCode, for labels.
	H1         = "h1"
	H2         = "h2"
	H3         = "h3"
	H4         = "h4"
	Lead       = "lead"
	LargeText  = "large-text"
	SmallText  = "small-text"
	Muted      = "muted"
	Blockquote = "blockquote"
	InlineCode = "inline-code"
	// Toolbar spaces a row of buttons as a toolbar.
	Toolbar = "toolbar"
	// View is the content-area surface.
	View = "view"
	// Warning paints text in the warning color.
	Warning = "warning"
)

// Button variants and sizes, the shadcn/ui names: a plain button is
// the secondary variant, SuggestedAction the default (primary) one.
const (
	// Secondary is the quiet filled button, the plain button's look.
	Secondary = "secondary"
	// Outline is a bordered button on the window background.
	Outline = "outline"
	// Ghost is a borderless button that fills only under the pointer.
	Ghost = "ghost"
	// Link is a button drawn as a text link.
	Link = "link"
	// Small and Large step the control height down and up.
	Small = "small"
	Large = "large"
	// Icon makes a button a square for an icon alone, 36px, or 32px and
	// 40px with Small and Large (shadcn's size="icon", icon-sm,
	// icon-lg); it composes with every variant.
	Icon = "icon"
)

// Named colors, referenced in stylesheets as @name.
const (
	AccentBgColor        = "accent_bg_color"
	AccentFgColor        = "accent_fg_color"
	AccentColor          = "accent_color"
	DestructiveBgColor   = "destructive_bg_color"
	DestructiveFgColor   = "destructive_fg_color"
	DestructiveColor     = "destructive_color"
	SuccessBgColor       = "success_bg_color"
	SuccessFgColor       = "success_fg_color"
	SuccessColor         = "success_color"
	WarningBgColor       = "warning_bg_color"
	WarningFgColor       = "warning_fg_color"
	WarningColor         = "warning_color"
	ErrorBgColor         = "error_bg_color"
	ErrorFgColor         = "error_fg_color"
	ErrorColor           = "error_color"
	WindowBgColor        = "window_bg_color"
	WindowFgColor        = "window_fg_color"
	ViewBgColor          = "view_bg_color"
	ViewFgColor          = "view_fg_color"
	HeaderbarBgColor     = "headerbar_bg_color"
	HeaderbarFgColor     = "headerbar_fg_color"
	HeaderbarBorderColor = "headerbar_border_color"
	CardBgColor          = "card_bg_color"
	CardFgColor          = "card_fg_color"
	DialogBgColor        = "dialog_bg_color"
	DialogFgColor        = "dialog_fg_color"
	PopoverBgColor       = "popover_bg_color"
	PopoverFgColor       = "popover_fg_color"
	SidebarBgColor       = "sidebar_bg_color"
	SidebarFgColor       = "sidebar_fg_color"
	ShadeColor           = "shade_color"
	BordersColor         = "borders"

	// The shadcn tokens libadwaita has no name for: the muted fill,
	// input borders, the focus ring and the five chart colors
	// (--chart-1 to --chart-5).
	MutedBgColor     = "muted_bg_color"
	MutedFgColor     = "muted_fg_color"
	InputBorderColor = "input_border_color"
	RingColor        = "ring_color"
	Chart1Color      = "chart_1"
	Chart2Color      = "chart_2"
	Chart3Color      = "chart_3"
	Chart4Color      = "chart_4"
	Chart5Color      = "chart_5"
)
