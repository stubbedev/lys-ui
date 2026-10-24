package app

import (
	"github.com/stubbedev/lys-ui/internal/emoji"
	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

// The emoji chooser (#92): a popover over the internal/emoji table -
// category sections in a scrollable grid, a search field over the
// names and keywords, and a click picking the emoji. GTK binds it on
// ctrl+period; lys-ui leaves the binding to the app (an accel or OnKey)
// and wires the insertion target with EmojiChooserForEntry.

// emojiFace resolves an emoji-capable face: an emoji family from the
// system font configuration, else the UI face (the picker still
// inserts real emoji; the display falls back to whatever the system
// can shape).
func emojiFace(sizePx float64) render.Font {
	for _, family := range []string{"Noto Color Emoji", "emoji", "EmojiOne"} {
		if tf, err := sysfont.Best(family, sizePx); err == nil {
			return tf
		}
	}
	tf, err := sysfont.UI()
	if err != nil {
		return nil
	}
	return tf
}

// EmojiChooser opens the emoji picker over host: categories in a
// scrollable grid, a search field, and OnPick receiving the chosen
// emoji string exactly once (the popover closes with the pick).
func (a *Application) EmojiChooser(host Host, onPick func(emoji string)) (*Popover, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	glyph := emojiFace(18)
	if glyph == nil {
		glyph = face
	}

	var pop *Popover
	pick := func(e string) {
		if pop != nil {
			pop.Dismiss()
		}
		if onPick != nil {
			onPick(e)
		}
	}
	results := widget.NewBox(widget.Column, 6, 0)
	rebuild := func(query string) {
		results.Clear()
		add := func(items []emoji.Emoji) {
			grid := widget.NewFlowBox(4, 4)
			grid.SetSelectionMode(widget.SelectionNone)
			grid.SetMaxChildrenPerLine(9)
			for _, e := range items {
				item := e
				btn := widget.NewButton(widget.NewLabel(glyph, 18, item.Glyph, 0), 6, 2)
				btn.OnClick = func() { pick(item.Glyph) }
				grid.Append(btn)
			}
			results.Append(grid, false)
		}
		if query != "" {
			add(emoji.Matches(query, 200))
			return
		}
		for _, cat := range emoji.Categories {
			if len(cat.Items) == 0 {
				continue
			}
			name := widget.NewLabel(face, 12, cat.Name, 0)
			name.AddClass(css.DimLabel)
			results.Append(name, false)
			add(cat.Items)
		}
		results.InvalidateLayout()
	}
	rebuild("")

	search := widget.NewSearchEntry(face, 13, "search emoji")
	search.SetDelay(0)
	search.OnSearchChanged = func(q string) { rebuild(q) }

	column := widget.NewBox(widget.Column, 8, 10)
	column.Append(search.Entry, false)
	scroll := widget.NewScroll(results)
	scroll.SetMaxContentHeight(260)
	column.Append(scroll, true)

	var err error
	pop, err = a.OpenPopover(host, PopoverConfig{
		Content:   column,
		MaxHeight: 320,
	})
	return pop, err
}

// EmojiInserter is the insertion half an emoji target offers: Entry
// and TextArea both implement it (Insert at the caret).
type EmojiInserter interface {
	Insert(s string)
}

// EmojiChooserForEntry opens the picker over host and inserts the
// chosen emoji into target at its caret - the ctrl+period flow. The
// binding itself stays the app's (an accel); this is the handler.
func (a *Application) EmojiChooserForEntry(host Host, target EmojiInserter) (*Popover, error) {
	return a.EmojiChooser(host, func(e string) { target.Insert(e) })
}
