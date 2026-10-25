package widget

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/internal/uifont"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// The theme layer (#95): a StylePriorityTheme stylesheet generated
// from the current Theme that defines libadwaita's named colors (as
// @define-color, so @accent_bg_color and friends resolve in any
// stylesheet) and styles the css package's classes in terms of them.
// SetTheme regenerates it, so a palette switch re-derives every named
// color; an application stylesheet outranks every rule here.

// themeSheet is the installed theme layer (installed at init).
var themeSheet *Stylesheet

// syncThemeSheet (re)generates the theme layer from t.
func syncThemeSheet(t *Theme) {
	syncSystemColors(t)
	src := themeCSS(t)
	if themeSheet == nil {
		themeSheet = AddStylesheet(src, StylePriorityTheme)
		return
	}
	themeSheet.Load(src)
}

// syncSystemColors maps the CSS system color keywords onto the theme,
// the same re-derivation a palette swap gives the named colors.
func syncSystemColors(t *Theme) {
	gray := Color(0)
	if a := t.Text.Straight(); a[3] > 0 {
		gray = render.RGBA(a[0], a[1], a[2], a[3]/2)
	}
	style.SetSystemColors(style.SystemColors{
		Canvas: t.Bg, CanvasText: t.Text,
		LinkText: t.Accent, VisitedText: t.Accent, GrayText: gray,
		Highlight: t.Accent, HighlightText: t.OnAccent,
		Mark: t.Accent, MarkText: t.OnAccent,
		ButtonFace: t.Surface, ButtonText: t.Text, ButtonBorder: t.Border,
		Field: t.Surface, FieldText: t.Text,
	})
}

// namedColors maps each Adwaita named color to its value under t.
func namedColors(t *Theme) [][2]string {
	white := render.RGB(0xff, 0xff, 0xff)
	shade := t.ShadowColor
	if shade == 0 {
		shade = render.RGBA(0, 0, 0, 0x12)
	}
	pairs := []struct {
		name string
		c    Color
	}{
		{css.AccentBgColor, t.Accent},
		{css.AccentFgColor, t.OnAccent},
		{css.AccentColor, t.Accent},
		{css.DestructiveBgColor, t.DestructiveColor()},
		{css.DestructiveFgColor, white},
		{css.DestructiveColor, t.DestructiveColor()},
		{css.SuccessBgColor, t.SuccessColor()},
		{css.SuccessFgColor, white},
		{css.SuccessColor, t.SuccessColor()},
		{css.WarningBgColor, t.WarningColor()},
		{css.WarningFgColor, render.RGBA(0, 0, 0, 0xcc)},
		{css.WarningColor, t.WarningColor()},
		{css.ErrorBgColor, t.ErrorColor()},
		{css.ErrorFgColor, white},
		{css.ErrorColor, t.ErrorColor()},
		{css.WindowBgColor, t.Bg},
		{css.WindowFgColor, t.Text},
		{css.ViewBgColor, t.Surface},
		{css.ViewFgColor, t.Text},
		{css.HeaderbarBgColor, t.Surface},
		{css.HeaderbarFgColor, t.Text},
		{css.HeaderbarBorderColor, t.Border},
		{css.CardBgColor, t.CardColor()},
		{css.CardFgColor, t.Text},
		{css.DialogBgColor, t.Bg},
		{css.DialogFgColor, t.Text},
		{css.PopoverBgColor, t.PopoverColor()},
		{css.PopoverFgColor, t.Text},
		{css.SidebarBgColor, t.SidebarColor()},
		{css.SidebarFgColor, t.Text},
		{css.Chart1Color, t.ChartColor(0)},
		{css.Chart2Color, t.ChartColor(1)},
		{css.Chart3Color, t.ChartColor(2)},
		{css.Chart4Color, t.ChartColor(3)},
		{css.Chart5Color, t.ChartColor(4)},
		{css.ShadeColor, shade},
		{css.BordersColor, t.Border},
		{css.MutedBgColor, t.MutedColor()},
		{css.SecondaryBgColor, t.SecondaryColor()},
		{css.MutedFgColor, t.DisabledText()},
		{css.InputBorderColor, t.InputColor()},
		{css.RingColor, t.RingColor()},
	}
	out := make([][2]string, len(pairs))
	for i, p := range pairs {
		out[i] = [2]string{p.name, FormatColor(p.c)}
	}
	return out
}

// classRules styles the css package's classes in terms of the named
// colors; Activatable and Numeric are behavior and face hints with no
// paint of their own.
var classRules = [][2]string{
	{css.Accent, "color: @accent_color;"},
	{css.Success, "color: @success_color;"},
	{css.Warning, "color: @warning_color;"},
	{css.Error, "color: @error_color;"},
	{css.DimLabel, "color: @muted_fg_color;"},
	{css.Heading, "font-weight: 700;"},
	{css.Caption, "font-size: 12px; line-height: 16px;"},
	{css.CaptionHeading, "font-size: 12px; line-height: 16px; font-weight: 700;"},
	{css.Title1, "font-size: 28px; font-weight: 800;"},
	{css.Title2, "font-size: 22px; font-weight: 800;"},
	{css.Title3, "font-size: 20px; font-weight: 700;"},
	{css.Title4, "font-size: 17px; font-weight: 700;"},
	{css.Monospace, "font-family: \"" + uifont.MonoFamily + "\";"},
	{css.Flat, "background-color: transparent;"},
	{css.SuggestedAction, "background-color: @accent_bg_color; color: @accent_fg_color;"},
	{css.DestructiveAction, "background-color: @destructive_bg_color; color: @destructive_fg_color;"},
	{css.Pill, "border-radius: 9999px; padding: 10px 32px;"},
	{css.Circular, "border-radius: 9999px;"},
	{css.Glass, "background-color: alpha(@card_bg_color, 0.55); backdrop-filter: blur(16px) saturate(1.5); border: 1px solid alpha(@window_fg_color, 0.12); border-radius: {r+4}; box-shadow: 0px 8px 24px alpha(black, 0.18);"},
	{css.LiquidGlass, "background-color: alpha(@card_bg_color, 0.18); backdrop-filter: -lys-refract(18px 10px) blur(2px) saturate(1.8); border: 1px solid alpha(white, 0.3); border-radius: {r+16}; box-shadow: inset 0px 1px 0px alpha(white, 0.5), inset 0px -1px 0px alpha(black, 0.12), 0px 10px 30px alpha(black, 0.22);"},
	{css.Frosted, "background-color: alpha(mix(@card_bg_color, white, 0.08), 0.62); background-image: -lys-grain(0.05); backdrop-filter: blur(24px) saturate(1.4); border: 1px solid alpha(@window_fg_color, 0.12); border-radius: {r+4}; box-shadow: 0px 8px 24px alpha(black, 0.18);"},
	{css.Glass + "." + css.BlurUnavailable + ", ." + css.Frosted + "." + css.BlurUnavailable, "background-color: @card_bg_color;"},
	{css.Card, "background-color: @card_bg_color; color: @card_fg_color; border: 1px solid @borders; border-radius: {r+6}; box-shadow: 0px 1px 2px alpha(black, 0.05);"},
	{css.BoxedList, "background-color: @card_bg_color; border-radius: {r+4};"},
	{css.OSD, "background-color: #000000b3; color: #ffffff;"},
	{css.View, "background-color: @view_bg_color; color: @view_fg_color;"},
	{css.Toolbar, "padding: 6px;"},
	{css.Keycap, "font-size: 12px; line-height: 16px; font-weight: 500; padding: 0px 4px; min-height: 20px; min-width: 20px; border-radius: {r-2}; background-color: @muted_bg_color; color: @muted_fg_color;"},
	{css.H1, "font-size: 36px; line-height: 40px; font-weight: 800;"},
	{css.H2, "font-size: 30px; line-height: 36px; font-weight: 600; padding-bottom: 8px; border-bottom: 1px solid @borders;"},
	{css.H3, "font-size: 24px; line-height: 32px; font-weight: 600;"},
	{css.H4, "font-size: 20px; line-height: 28px; font-weight: 600;"},
	{css.Lead, "font-size: 20px; line-height: 28px; color: @muted_fg_color;"},
	{css.LargeText, "font-size: 18px; line-height: 28px; font-weight: 600;"},
	{css.SmallText, "font-size: 14px; font-weight: 500;"},
	{css.Muted, "font-size: 14px; color: @muted_fg_color;"},
	{css.Blockquote, "font-style: italic; padding-left: 24px; border-left: 2px solid @borders;"},
	{css.InlineCode, "font-family: \"" + uifont.MonoFamily + "\"; font-size: 14px; font-weight: 600; padding: 2px 5px; border-radius: {r-4}; background-color: @muted_bg_color;"},
}

// tabLists and tabTriggers are the three tab strips' lists and tabs -
// StackSwitcher and ViewSwitcher (a togglegroup), TabBar, Notebook -
// styled as tabs: a muted list of muted tabs, the active one
// raised on the background fill.
const (
	tabLists    = "togglegroup, viewswitcher, tabbar, notebook > header > tabs"
	tabTriggers = "togglegroup button, viewswitcher button, tabbar tab, notebook tab"
	activeTabs  = "togglegroup button:checked, viewswitcher button:checked, tabbar tab:selected, notebook tab:checked"
)

// variants are the button classes that take the full button shape; a
// plain button keeps the padding and radius its constructor was given,
// so internal buttons (calendar navigation, column headers, window
// controls) keep theirs.
var variants = []string{css.SuggestedAction, css.DestructiveAction, css.Secondary, css.Outline, css.Ghost, css.Link}

// controlRules give the controls the toolkit's look: hairline input
// borders, the button variants and sizes, and the floating surfaces.
// Every color is a named color, so a palette switch restyles them.
func controlRules() [][2]string {
	var all strings.Builder
	for i, v := range variants {
		if i > 0 {
			all.WriteString(", ")
		}
		all.WriteString("button." + v)
	}
	return [][2]string{
		{":root", "line-height: 1.428571;"},
		{":root." + css.Background, "background-color: @window_bg_color; color: @window_fg_color;"},
		{"button", "font-weight: 500;"},
		{all.String(), "min-height: 36px; padding: 0px 16px; border-radius: {r}; box-sizing: border-box;"},
		{"button." + css.Small, "min-height: 32px; padding: 0px 12px; border-radius: {r}; box-sizing: border-box;"},
		{"button." + css.Large, "min-height: 40px; padding: 0px 24px; border-radius: {r}; box-sizing: border-box;"},
		{"button." + css.Icon, "min-width: 36px; min-height: 36px; padding: 0px; box-sizing: border-box;"},
		{"button." + css.Icon + "." + css.Small, "min-width: 32px; min-height: 32px; padding: 0px; box-sizing: border-box;"},
		{"button." + css.Icon + "." + css.Large, "min-width: 40px; min-height: 40px; padding: 0px; box-sizing: border-box;"},
		{"button.toggle", "min-width: 36px; padding: 0px 8px;"},
		{"button." + css.SuggestedAction + ":hover", "background-color: mix(@accent_bg_color, @window_bg_color, 0.1);"},
		{"button." + css.SuggestedAction + ":active", "background-color: mix(@accent_bg_color, @window_bg_color, 0.2);"},
		{"button." + css.SuggestedAction + ":disabled", "background-color: alpha(@accent_bg_color, 0.5);"},
		{"button." + css.DestructiveAction + ":hover", "background-color: mix(@destructive_bg_color, @window_bg_color, 0.1);"},
		{"button." + css.DestructiveAction + ":active", "background-color: mix(@destructive_bg_color, @window_bg_color, 0.2);"},
		{"button." + css.DestructiveAction + ":disabled", "background-color: alpha(@destructive_bg_color, 0.5);"},
		{"button." + css.Secondary, "background-color: @secondary_bg_color; color: @window_fg_color;"},
		{"button." + css.Secondary + ":hover", "background-color: alpha(@secondary_bg_color, 0.8);"},
		{"button." + css.Outline, "background-color: @window_bg_color; border: 1px solid @borders; box-shadow: 0px 1px 2px alpha(black, 0.05);"},
		{"button." + css.Outline + ":hover, button." + css.Ghost + ":hover", "background-color: @muted_bg_color;"},
		{"button." + css.Ghost + ", button." + css.Link, "background-color: transparent;"},
		{"button." + css.Ghost + ":checked, button." + css.Outline + ":checked", "background-color: @muted_bg_color;"},
		{"button." + css.Ghost + ":disabled, button." + css.Link + ":disabled, button." + css.Outline + ":disabled", "background-color: transparent;"},
		{"button." + css.Link, "color: @accent_color;"},
		{"button." + css.Link + ":hover label", "text-decoration-line: underline;"},
		{"dropdown > button", "background-color: transparent; box-shadow: inset 0px 0px 0px 1px @input_border_color; border-radius: {r}; padding: 0px 12px;"},
		{"dropdown > button:hover", "background-color: @muted_bg_color;"},
		{"checkbutton check, checkbutton radio", "border: 1px solid @input_border_color; background-color: transparent;"},
		{"progressbar trough", "border-radius: 9999px;"},
		{"checkbutton:checked check, checkbutton:indeterminate check", "border-color: @accent_bg_color; background-color: @accent_bg_color; color: @accent_fg_color;"},
		{"checkbutton:checked radio", "color: @accent_bg_color;"},
		{"checkbutton > :not(check)", "margin-left: 8px;"},
		{"scale slider", "background-color: white; box-shadow: inset 0px 0px 0px 1px @accent_bg_color, 0px 1px 2px alpha(black, 0.05);"},
		{"entry, spinbutton, textview", "box-shadow: inset 0px 0px 0px 1px @input_border_color; border-radius: {r}; background-color: transparent;"},
		{"textview", "min-height: 64px;"},
		{"entry:focus, spinbutton:focus, textview:focus", "box-shadow: inset 0px 0px 0px 1px @ring_color;"},
		{"badge", "padding: 2px 8px; border: 1px solid transparent; border-radius: 9999px; font-size: 12px; line-height: 16px; font-weight: 500; background-color: @accent_bg_color; color: @accent_fg_color;"},
		{"badge." + css.Secondary, "background-color: @secondary_bg_color; color: @window_fg_color;"},
		{"badge." + css.Destructive, "background-color: @destructive_bg_color; color: @destructive_fg_color;"},
		{"badge." + css.Outline, "background-color: transparent; color: @window_fg_color; border-color: @borders;"},
		{"alert", "padding: 12px 16px; border: 1px solid @borders; border-radius: {r+2}; background-color: @card_bg_color; color: @card_fg_color;"},
		{"alert." + css.Destructive, "color: @destructive_color;"},
		{"alert .title", "font-weight: 500;"},
		{"alert .description", "color: @muted_fg_color;"},
		{"alert." + css.Destructive + " .description", "color: alpha(@destructive_color, 0.9);"},
		{"statuspage .icon", "border-radius: {r}; background-color: @muted_bg_color; color: @window_fg_color;"},
		{"statuspage .title", "font-size: 1.125rem; font-weight: 500;"},
		{"statuspage .description", "font-size: 0.875rem; color: @muted_fg_color;"},
		{"accordion .item", "border-bottom: 1px solid @borders;"},
		{"accordion .item:last-child", "border-bottom: none;"},
		{"accordion button.trigger", "background-color: transparent; padding: 16px 0px; border-radius: {r-2}; color: @window_fg_color;"},
		{"accordion button.trigger:hover label", "text-decoration-line: underline;"},
		{"accordion .chevron", "color: @muted_fg_color; transition: transform 200ms;"},
		{"accordion .item:checked .chevron", "transform: rotate(180deg);"},
		{"accordion .content", "padding-bottom: 16px;"},
		{"breadcrumb", "color: @muted_fg_color;"},
		{"breadcrumb button.crumb", "background-color: transparent; padding: 0px; min-height: 0px; border-radius: {r-4}; color: @muted_fg_color; font-weight: 400; transition: color 150ms;"},
		{"breadcrumb button.crumb:hover", "color: @window_fg_color;"},
		{"breadcrumb .page", "color: @window_fg_color;"},
		{"breadcrumb button.ellipsis", "background-color: transparent; padding: 0px; min-width: 36px; min-height: 36px; border-radius: {r-2}; color: @window_fg_color;"},
		{"pagination button", "background-color: transparent; min-height: 36px; border-radius: {r}; color: @window_fg_color;"},
		{"pagination button.page-link", "min-width: 36px; padding: 0px;"},
		{"pagination button.previous", "padding: 0px 10px 0px 8px;"},
		{"pagination button.next", "padding: 0px 8px 0px 10px;"},
		{"pagination button:hover", "background-color: @muted_bg_color;"},
		{"pagination button.page-link:checked", "background-color: @window_bg_color; box-shadow: inset 0px 0px 0px 1px @input_border_color, 0px 1px 2px alpha(black, 0.05);"},
		{"pagination button:disabled", "background-color: transparent; color: @muted_fg_color;"},
		{"field .title, fieldset .legend", "font-weight: 500;"},
		{"field .description, fieldset .description", "color: @muted_fg_color;"},
		{"field.invalid .title, field ." + css.Error, "color: @destructive_color;"},
		{"entry." + css.Error + ", spinbutton." + css.Error + ", textview." + css.Error, "color: @window_fg_color; box-shadow: inset 0px 0px 0px 1px @destructive_color;"},
		{"inputgroup", "box-shadow: inset 0px 0px 0px 1px @input_border_color; border-radius: {r}; color: @muted_fg_color; min-height: 36px;"},
		{"inputgroup:focus-within", "box-shadow: inset 0px 0px 0px 1px @ring_color;"},
		{"inputgroup." + css.Error, "color: @muted_fg_color; box-shadow: inset 0px 0px 0px 1px @destructive_color;"},
		{"inputgroup entry, inputgroup entry:focus", "box-shadow: none; padding-left: 0px; padding-right: 0px; color: @window_fg_color;"},
		{"inputotp .slot", "border-top: 1px solid @input_border_color; border-bottom: 1px solid @input_border_color; border-right: 1px solid @input_border_color;"},
		{"inputotp .slot:first-child", "border-left: 1px solid @input_border_color; border-radius: {r} 0px 0px {r};"},
		{"inputotp .slot:last-child", "border-radius: 0px {r} {r} 0px;"},
		{"inputotp .slot:only-child", "border-radius: {r};"},
		{"inputotp:focus .slot:checked", "box-shadow: inset 0px 0px 0px 1px @ring_color;"},
		{"command", "background-color: @popover_bg_color; color: @popover_fg_color; border-radius: {r};"},
		{"command .input", "min-height: 36px; border-bottom: 1px solid @borders; color: @muted_fg_color;"},
		{"command .input entry, command .input entry:focus", "box-shadow: none; padding-left: 0px; padding-right: 0px; background-color: transparent; color: @popover_fg_color;"},
		{"command .heading", "font-weight: 500; line-height: 16px; color: @muted_fg_color; padding: 6px 8px;"},
		{"command .item", "border-radius: {r-4};"},
		{"command .item:selected", "background-color: @muted_bg_color;"},
		{"command .item:disabled", "color: @muted_fg_color;"},
		{"command .shortcut", "color: @muted_fg_color; letter-spacing: 1px;"},
		{"command .separator", "margin: 4px 0px;"},
		{"command .empty", "padding: 24px 0px; color: @muted_fg_color;"},
		{"combobox > button." + css.Outline + ", datepicker > button." + css.Outline, "padding: 0px 12px;"},
		{"datepicker > button." + css.Outline, "font-weight: 400;"},
		{"datepicker .placeholder", "color: @muted_fg_color;"},
		{"calendar button.nav", "background-color: transparent; border-radius: {r}; color: @window_fg_color;"},
		{"calendar button.nav:hover", "background-color: @muted_bg_color; color: @window_fg_color;"},
		{"sheet .title", "font-weight: 600;"},
		{"sheet .description", "color: @muted_fg_color;"},
		{"sheet button.close", "background-color: transparent; border-radius: {r-4}; color: @muted_fg_color;"},
		{"sheet button.close:hover", "color: @window_fg_color;"},
		{"sidebar .panel", "background-color: @sidebar_bg_color; color: @sidebar_fg_color; border-right: 1px solid @borders; overflow: hidden;"},
		{"sidebar .sheet-panel", "padding: 0px;"},
		{"sidebar .group-label", "font-size: 0.75rem; font-weight: 500; color: @muted_fg_color; padding: 8px 8px 4px 8px;"},
		{"sidebar button.item label", "font-size: 0.875rem;"},
		{"sidebar button.group-label", "background-color: transparent; min-height: 0px; border-radius: {r-2}; font-size: 0.75rem;"},
		{"sidebar button.group-label:hover", "background-color: @muted_bg_color;"},
		{"sidebar button.item", "background-color: transparent; min-height: 32px; padding: 0px 8px; border-radius: {r-2}; font-weight: 400; color: @sidebar_fg_color;"},
		{"sidebar button.item:hover", "background-color: @muted_bg_color;"},
		{"sidebar button.item:checked", "background-color: @muted_bg_color; font-weight: 500;"},
		{"sidebar button.item .badge", "font-size: 0.75rem; font-weight: 500;"},
		{"sidebar .sub", "border-left: 1px solid @borders; margin-left: 14px; padding-left: 8px;"},
		{"sidebar button.trigger", "background-color: transparent; border-radius: {r-2};"},
		{"sidebar button.trigger:hover", "background-color: @muted_bg_color;"},
		{"navigationmenu button.trigger", "background-color: transparent; min-height: 36px; padding: 0px 16px; border-radius: {r-2}; font-weight: 500;"},
		{"navigationmenu button.trigger:hover, navigationmenu button.trigger:checked", "background-color: @muted_bg_color;"},
		{"navigationmenu .chevron", "transition: transform 300ms;"},
		{"navigationmenu button.trigger:checked .chevron", "transform: rotate(180deg);"},
		{"navigationmenu .viewport", "padding: 8px;"},
		{"navigationmenu button.link", "background-color: transparent; padding: 12px; border-radius: {r-2}; font-weight: 400;"},
		{"navigationmenu button.link:hover", "background-color: @muted_bg_color;"},
		{"datatable .table", "border: 1px solid @borders; border-radius: {r};"},
		{"columnview listview", "background-color: transparent;"},
		{"columnview .header", "min-height: 40px; border-bottom: 1px solid @borders;"},
		{"columnview .header button", "background-color: transparent; font-weight: 500;"},
		{"columnview .header button:hover", "background-color: @muted_bg_color;"},
		{"columnview row", "border-bottom: 1px solid @borders;"},
		{"columnview row:hover", "background-color: alpha(@muted_bg_color, 0.5);"},
		{"columnview row:selected", "background-color: @muted_bg_color;"},
		{"item", "padding: 16px; border: 1px solid transparent; border-radius: {r};"},
		{"item." + css.Small, "padding: 12px 16px;"},
		{"item." + css.Outline, "border-color: @borders;"},
		{"item." + css.Muted, "background-color: alpha(@muted_bg_color, 0.5); font-size: inherit; color: inherit;"},
		{"item .title", "font-size: 0.875rem; font-weight: 500;"},
		{"item .description", "font-size: 0.875rem; color: @muted_fg_color;"},
		{"item .media", "background-color: @muted_bg_color; border-radius: {r-4};"},
		{"item." + css.Activatable + ":hover", "background-color: alpha(@muted_bg_color, 0.5);"},
		{"buttongroup > box > *:nth-child(n)", "border-radius: 0px;"},
		{"buttongroup > box > *:first-child", "border-top-left-radius: {r}; border-bottom-left-radius: {r};"},
		{"buttongroup > box > *:last-child", "border-top-right-radius: {r}; border-bottom-right-radius: {r};"},
		{"buttongroup > box > *:not(:first-child)", "border-left-width: 0px;"},
		{"buttongroup.vertical > box > *:first-child", "border-radius: {r} {r} 0px 0px;"},
		{"buttongroup.vertical > box > *:last-child", "border-radius: 0px 0px {r} {r};"},
		{"buttongroup.vertical > box > *:not(:first-child)", "border-left-width: 1px; border-top-width: 0px;"},
		{"buttongroup > box > *:only-child", "border-radius: {r};"},
		{tabLists, "background-color: @muted_bg_color; border-radius: {r+2}; padding: 3px;"},
		{tabTriggers, "background-color: transparent; border-radius: {r}; min-height: 28px; padding: 0px 12px; font-weight: 500; color: @muted_fg_color;"},
		{activeTabs, "background-color: @window_bg_color; color: @window_fg_color; box-shadow: 0px 1px 3px alpha(black, 0.1);"},
		{"carousel button.nav", "min-width: 32px; min-height: 32px; padding: 0px; border-radius: 9999px;"},
		{"notebook", "background-color: transparent;"},
		{"notebook > header", "margin-bottom: 8px;"},
		{"tabbar .tab-close-button", "background-color: transparent; border-radius: {r-4};"},
		{"tabbar .tab-close-button:hover", "background-color: @muted_bg_color;"},
		{"skeleton", "background-color: @muted_bg_color; border-radius: {r};"},
		{"bullets", "margin-left: 8px;"},
		{"rows", "border: 1px solid @borders;"},
		{"dialog", "border: 1px solid @borders; border-radius: {r+2};"},
		{"dialog .title", "font-size: 1.125rem; font-weight: 600;"},
		{"dialog .description, dialog .footer label", "font-size: 0.875rem;"},
		{"toast", "background-color: @popover_bg_color; color: @popover_fg_color; border: 1px solid @borders; border-radius: {r};"},
		{"menubar", "background-color: @window_bg_color; border: 1px solid @borders; border-radius: {r}; padding: 4px; box-shadow: 0px 1px 2px alpha(black, 0.05);"},
		{"menubar > box > button", "background-color: transparent; border-radius: {r-4}; padding: 4px 8px; min-height: 0px; font-weight: 500;"},
		{"menubar > box > button:hover, menubar > box > button:checked", "background-color: @muted_bg_color;"},
		{"tooltip", "background-color: @window_fg_color; color: @window_bg_color; border-radius: {r}; padding: 6px 12px; font-size: 12px; line-height: 16px;"},
		{"popover, menu", "background-color: @popover_bg_color; color: @popover_fg_color; border: 1px solid @borders; border-radius: {r};"},
	}
}

// darkRules are the dark variants, added on a dark palette: the inputs
// and outline controls take a translucent input fill, and the
// destructive fills soften to 60%.
var darkRules = [][2]string{
	{"entry, spinbutton, textview, inputgroup, inputotp .slot, dropdown > button, combobox > button, datepicker > button, checkbutton check, checkbutton radio", "background-color: alpha(@input_border_color, 0.3);"},
	{"inputgroup entry", "background-color: transparent;"},
	{"button." + css.Outline, "background-color: alpha(@input_border_color, 0.3); border-color: @input_border_color;"},
	{"button." + css.Outline + ":hover, dropdown > button:hover", "background-color: alpha(@input_border_color, 0.5);"},
	{"button." + css.DestructiveAction + ", badge." + css.Destructive, "background-color: alpha(@destructive_bg_color, 0.6);"},
	{"button." + css.DestructiveAction + ":hover, button." + css.DestructiveAction + ":active", "background-color: alpha(@destructive_bg_color, 0.5);"},
	{activeTabs, "background-color: alpha(@input_border_color, 0.3); box-shadow: inset 0px 0px 0px 1px @input_border_color;"},
}

// partRules style widget parts: the level bar's trough and offset
// classes, the progress ring's status fills, and the media controls'
// volume slider length.
var partRules = [][2]string{
	{"levelbar trough", "background-color: @view_bg_color;"},
	{"levelbar block." + LevelBarOffsetLow, "background-color: @warning_bg_color;"},
	{"levelbar block." + LevelBarOffsetHigh, "background-color: @accent_bg_color;"},
	{"levelbar block." + LevelBarOffsetFull, "background-color: @success_bg_color;"},
	{"progressring." + css.Success + " progress", "background-color: @success_bg_color;"},
	{"progressring." + css.Warning + " progress", "background-color: @warning_bg_color;"},
	{"progressring." + css.Error + " progress", "background-color: @error_bg_color;"},
	{"controls .volume trough", "min-width: 80px;"},
}

// motionTokens are the motion values the theme layer defines
// (@-lys-define), which stylesheets read as var(--lys-duration-short)
// and friends to share one motion language: the durations, the
// standard and emphasized curves, and the springs the surfaces use as
// duration-filling -lys-spring() easings.
func motionTokens(t *Theme) [][2]string {
	short, medium, long := t.MotionDurations()
	ms := func(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) + "ms" }
	bez := func(b anim.Bezier) string {
		f := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
		return "cubic-bezier(" + f(b[0]) + ", " + f(b[1]) + ", " + f(b[2]) + ", " + f(b[3]) + ")"
	}
	spring := func(p anim.SpringParams) string {
		return "-lys-spring(" + strconv.FormatFloat(math.Max(0, 1-p.Damping), 'g', 3, 64) + ")"
	}
	return [][2]string{
		{"lys-duration-short", ms(short)},
		{"lys-duration-medium", ms(medium)},
		{"lys-duration-long", ms(long)},
		{"lys-ease-standard", bez(anim.BezierStandard)},
		{"lys-ease-emphasized-decelerate", bez(anim.BezierEmphasizedDecelerate)},
		{"lys-ease-emphasized-accelerate", bez(anim.BezierEmphasizedAccelerate)},
		{"lys-spring-smooth", spring(anim.SpringSmooth)},
		{"lys-spring-snappy", spring(anim.SpringSnappy)},
		{"lys-spring-bouncy", spring(anim.SpringBouncy)},
	}
}

// radiusToken is a rule's corner radius as an offset from the theme's
// Radius, {r} itself or {r-2} and {r+6}: the steps below and above it,
// so a palette with another Radius rounds every rule in proportion.
var radiusToken = regexp.MustCompile(`\{r([+-]\d+)?\}`)

// radii resolves css's radius tokens against radius, a step never
// below zero.
func radii(css string, radius int) string {
	return radiusToken.ReplaceAllStringFunc(css, func(tok string) string {
		d := 0
		if off := radiusToken.FindStringSubmatch(tok)[1]; off != "" {
			// The pattern admits only a signed integer here.
			d, _ = strconv.Atoi(off)
		}
		return strconv.Itoa(max(0, radius+d)) + "px"
	})
}

// themeCSS renders the theme layer for t.
func themeCSS(t *Theme) string {
	var b strings.Builder
	for _, tok := range motionTokens(t) {
		fmt.Fprintf(&b, "@-lys-define %s %s;\n", tok[0], tok[1])
	}
	for _, nc := range namedColors(t) {
		fmt.Fprintf(&b, "@define-color %s %s;\n", nc[0], nc[1])
	}
	for _, r := range classRules {
		fmt.Fprintf(&b, ".%s { %s }\n", r[0], r[1])
	}
	for _, r := range controlRules() {
		fmt.Fprintf(&b, "%s { %s }\n", r[0], r[1])
	}
	if t.IsDark() {
		for _, r := range darkRules {
			fmt.Fprintf(&b, "%s { %s }\n", r[0], r[1])
		}
	}
	if t.Animations {
		// The skeleton's pulse; reduced motion regenerates the layer
		// without it.
		b.WriteString("@keyframes lys-pulse { 50% { opacity: 0.5; } }\nskeleton { animation: lys-pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite; }\n")
	}
	for _, r := range partRules {
		fmt.Fprintf(&b, "%s { %s }\n", r[0], r[1])
	}
	return radii(b.String(), t.Radius)
}

// init installs the theme layer for the default palette, so the named
// colors resolve before any SetTheme.
func init() { syncThemeSheet(current) }
