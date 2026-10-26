package highlight

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// Adwaita and AdwaitaDark are the style schemes for the def: ids every
// language here emits, in the libadwaita palette for a light and a
// dark view (GtkSourceView's Adwaita schemes' roles). They leave the
// text and background to the theme; SchemeFor picks the one matching a
// theme.
var (
	Adwaita = widget.TextScheme{
		styleComment:      {Color: render.RGB(0x77, 0x76, 0x7b), Italic: true},
		styleKeyword:      {Color: render.RGB(0x1a, 0x5f, 0xb4), Bold: true},
		"def:statement":   {Color: render.RGB(0x1a, 0x5f, 0xb4)},
		styleType:         {Color: render.RGB(0x26, 0xa2, 0x69)},
		styleString:       {Color: render.RGB(0xc6, 0x46, 0x00)},
		styleEscape:       {Color: render.RGB(0xe6, 0x61, 0x00), Bold: true},
		styleConstant:     {Color: render.RGB(0x81, 0x3d, 0x9c)},
		styleBuiltin:      {Color: render.RGB(0x1c, 0x71, 0xd8)},
		stylePreproc:      {Color: render.RGB(0xa5, 0x1d, 0x2d)},
		styleError:        {Color: render.RGB(0xe0, 0x1b, 0x24)},
		styleHeading:      {Color: render.RGB(0x1a, 0x5f, 0xb4), Bold: true},
		styleEmphasis:     {Italic: true},
		styleStrong:       {Bold: true},
		stylePreformatted: {Color: render.RGB(0x5e, 0x5c, 0x64)},
		styleInlineCode:   {Color: render.RGB(0xc6, 0x46, 0x00)},
		styleLinkText:     {Color: render.RGB(0x1c, 0x71, 0xd8)},
		"def:net-address": {Color: render.RGB(0x77, 0x76, 0x7b)},
		styleListMarker:   {Color: render.RGB(0xe6, 0x61, 0x00), Bold: true},
		styleQuoteMarker:  {Color: render.RGB(0x77, 0x76, 0x7b)},
		styleBreak:        {Color: render.RGB(0x77, 0x76, 0x7b)},
	}
	AdwaitaDark = widget.TextScheme{
		styleComment:      {Color: render.RGB(0x9a, 0x99, 0x96), Italic: true},
		styleKeyword:      {Color: render.RGB(0x62, 0xa0, 0xea), Bold: true},
		"def:statement":   {Color: render.RGB(0x62, 0xa0, 0xea)},
		styleType:         {Color: render.RGB(0x57, 0xe3, 0x89)},
		styleString:       {Color: render.RGB(0xff, 0xa3, 0x48)},
		styleEscape:       {Color: render.RGB(0xff, 0xbe, 0x6f), Bold: true},
		styleConstant:     {Color: render.RGB(0xdc, 0x8a, 0xdd)},
		styleBuiltin:      {Color: render.RGB(0x99, 0xc1, 0xf1)},
		stylePreproc:      {Color: render.RGB(0xf6, 0x61, 0x51)},
		styleError:        {Color: render.RGB(0xff, 0x7b, 0x63)},
		styleHeading:      {Color: render.RGB(0x99, 0xc1, 0xf1), Bold: true},
		styleEmphasis:     {Italic: true},
		styleStrong:       {Bold: true},
		stylePreformatted: {Color: render.RGB(0xc0, 0xbf, 0xbc)},
		styleInlineCode:   {Color: render.RGB(0xff, 0xa3, 0x48)},
		styleLinkText:     {Color: render.RGB(0x62, 0xa0, 0xea)},
		"def:net-address": {Color: render.RGB(0x9a, 0x99, 0x96)},
		styleListMarker:   {Color: render.RGB(0xff, 0xbe, 0x6f), Bold: true},
		styleQuoteMarker:  {Color: render.RGB(0x9a, 0x99, 0x96)},
		styleBreak:        {Color: render.RGB(0x9a, 0x99, 0x96)},
	}
)

// SchemeFor is the Adwaita scheme matching th's lightness.
func SchemeFor(th *widget.Theme) widget.TextScheme {
	if th.IsDark() {
		return AdwaitaDark
	}
	return Adwaita
}
