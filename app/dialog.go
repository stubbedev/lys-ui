package app

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

// DialogButton is one button in a dialog's button row. Role, when
// set, decides the keyboard mapping explicitly: ButtonRoleDefault
// binds Enter, ButtonRoleCancel binds Escape. Without roles the
// positional fallbacks apply - first button default, last cancel.
type DialogButton struct {
	Label    string
	Response string
	Role     DialogButtonRole
}

// DialogButtonRole is the keyboard role a dialog button opts into.
type DialogButtonRole uint8

// Button roles.
const (
	// ButtonRoleNormal is a plain button: no key bound.
	ButtonRoleNormal DialogButtonRole = iota
	// ButtonRoleDefault binds Enter.
	ButtonRoleDefault
	// ButtonRoleCancel binds Escape.
	ButtonRoleCancel
)

// DialogConfig declares a dialog: content widget plus a button row.
// Esc responds with CancelResponse, Enter with DefaultResponse (either
// may be empty to disable that key). Width and Height are the
// dialog's initial size in logical pixels, like every size in the
// toolkit; zero lets the compositor pick.
type DialogConfig struct {
	Title string
	// Width and Height size the window; a zero Height fits the dialog's
	// content at Width.
	Width, Height   uint32
	Content         widget.Widget
	Buttons         []DialogButton
	DefaultResponse string
	CancelResponse  string
	// ValidateResponse, when set, vetoes a response: returning false
	// keeps the dialog open (nothing responded, nothing closed), so a
	// dialog can refuse to close until its content is valid - the file
	// picker refusing an empty selection, say. It runs before
	// OnResponse on every Respond, including the Esc and Enter keys.
	ValidateResponse func(response string) bool
	OnResponse       func(response string)
	// OnClosed runs once the dialog window closed, after OnResponse when
	// a response closed it, and also when it closed without one (the
	// compositor's close, Close).
	OnClosed func()
	// Modal blocks every other window's input while the dialog is
	// open (and, with an xdg parent, asks the compositor to block the
	// parent itself). False - the GTK default - leaves the
	// application fully interactive under the dialog; the preset
	// dialogs set it themselves.
	Modal bool
	// Focus, when set, takes the keyboard focus as the dialog opens: a
	// widget of Content, its first field.
	Focus widget.Widget
	// Bare makes Content the whole window: no button row, no card,
	// Background as the window's clear color. The content carries its
	// own buttons (styled by the application's stylesheet) and answers
	// through Dialog.Respond; Esc and Enter still map to the cancel and
	// default responses.
	Bare       bool
	Background render.Color
}

// Dialog is a modal child window with a response callback. While it
// is open every other window's input is blocked at the application
// level, and — when the compositor offers xdg-dialog-v1 and the dialog
// has a parent — at the window level too: the compositor blocks input
// to the parent itself. Respond fires OnResponse exactly once and
// closes the dialog.
type Dialog struct {
	app       *Application
	win       *Window
	cfg       DialogConfig
	responded bool
}

// DialogParent is what a dialog can be parented to: a toplevel
// Window carries the xdg parent link and the modality hint, a
// LayerWindow parents at the application level only (layer surfaces
// have no xdg parent), and nil makes an application-level dialog.
type DialogParent interface {
	dialogWindow() *Window
}

// dialogWindow implements DialogParent.
func (w *Window) dialogWindow() *Window { return w }

// dialogWindow implements DialogParent; layer surfaces have no xdg
// window to parent to.
func (l *LayerWindow) dialogWindow() *Window { return nil }

// dialogMaxHeight caps the height a dialog sized to its content takes.
const dialogMaxHeight = 4096

// NewDialog opens a dialog parented to a window of this application.
// A nil parent makes an application-level dialog that blocks every
// other window while open (when Modal).
func (a *Application) NewDialog(parent DialogParent, cfg DialogConfig) (*Dialog, error) {
	if cfg.Content == nil {
		return nil, errors.New("app: dialog needs content")
	}
	var d *Dialog
	respond := func(response string) {
		if d != nil {
			d.Respond(response)
		}
	}
	root, background, err := a.dialogRoot(cfg, respond)
	if err != nil {
		return nil, err
	}
	if cfg.Height == 0 {
		cfg.Height = uint32(root.Measure(widget.Constraints{Max: widget.Size{W: int(cfg.Width), H: dialogMaxHeight}}).H)
	}

	winCfg := WindowConfig{
		Title: cfg.Title, Width: cfg.Width, Height: cfg.Height,
		Root: root, Background: background,
		OnKey: func(_ *widget.Router, code uint32, _ wlsession.Mods) {
			if resp, ok := dialogResponseForKey(cfg, a.sess.KeySym(code)); ok {
				respond(resp)
			}
		},
		OnClosed: func() {
			for i, x := range a.dialogs {
				if x == d {
					a.dialogs = append(a.dialogs[:i], a.dialogs[i+1:]...)
					break
				}
			}
			// Recompute the block from what is still open: a modal closing
			// over a non-modal must unblock, not wait for the last dialog.
			modal := false
			for _, x := range a.dialogs {
				modal = modal || x.cfg.Modal
			}
			a.blockAll(modal)
			if cfg.OnClosed != nil {
				cfg.OnClosed()
			}
		},
	}
	// Dialogs animate: KindDialog gives the child window a fade-in on
	// open and an exit tween on Respond, during which the modal block
	// stays on (the unblock happens in OnClosed, once the tween's
	// destroy has landed and the loop reaped the window).
	w, err := a.newWindowWindow(winCfg, surfx.KindDialog)
	if err != nil {
		return nil, fmt.Errorf("app: dialog window: %w", err)
	}
	if hw := a.hostWindowOf(w); hw != nil {
		hw.isDialog = true
		hw.blocked = false
	}
	d = &Dialog{app: a, win: w, cfg: cfg}
	if cfg.Focus != nil {
		w.SetFocus(cfg.Focus)
	}
	a.dialogs = append(a.dialogs, d)
	if cfg.Modal {
		a.blockAll(true)
	}
	if pw := parent.dialogWindow(); pw != nil && pw.win != nil {
		w.win.SetParent(pw.win)
		if !cfg.Modal {
			return d, nil
		}
		// Window-level modality where the compositor offers it: the hint
		// rides the dialog's first commit, and the compositor blocks input
		// to the parent itself. Without the protocol this is a silent
		// no-op and the application-level block above stays the whole
		// story — the floor every compositor gets.
		if err := w.win.SetModal(a.sess.DialogManager(), true); err != nil {
			debug.Log("shell", "dialog modality hint: %v", err)
		}
	}
	return d, nil
}

// Respond fires OnResponse once and closes the dialog. Further calls
// are ignored, so a double event cannot produce two responses.
func (d *Dialog) Respond(response string) {
	if d.responded {
		return
	}
	// A vetoed response leaves the dialog exactly as it was: not
	// responded, not closed, retryable.
	if d.cfg.ValidateResponse != nil && !d.cfg.ValidateResponse(response) {
		return
	}
	d.responded = true
	if d.cfg.OnResponse != nil {
		d.cfg.OnResponse(response)
	}
	d.win.Close()
}

// Closed reports whether the dialog already produced a response or was
// closed.
func (d *Dialog) Closed() bool { return d.responded || d.win.Closed() }

// Close closes the dialog without a response; OnResponse does not run.
func (d *Dialog) Close() {
	d.responded = true
	d.win.Close()
}

// dialogResponseForKey maps Escape and Enter to the dialog's cancel
// and default responses; an empty response disables the key.
func dialogResponseForKey(cfg DialogConfig, sym xkb.Keysym) (string, bool) {
	switch sym {
	case xkb.KeyEscape:
		return cfg.CancelResponse, cfg.CancelResponse != ""
	case xkb.KeyReturn, xkb.KeyKPEnter:
		return cfg.DefaultResponse, cfg.DefaultResponse != ""
	}
	return "", false
}

// blockAll flips the input-blocked flag; dialog windows are exempt
// from their own blocking.
func (a *Application) blockAll(blocked bool) {
	dialogWins := make(map[*Window]bool, len(a.dialogs))
	for _, d := range a.dialogs {
		dialogWins[d.win] = true
	}
	for _, w := range a.windows {
		if w.win == nil || dialogWins[w.win] {
			continue
		}
		w.blocked = blocked
	}
}

// customPalette is this application's color picks: every color a
// ColorChooserDialog confirms lands here (deduped, most recent
// first, capped), and every later chooser offers them back as
// swatches. Scoped per Application (#84): two apps in one process
// never swap palettes.
type appPalette struct {
	colors []render.Color
}

// paletteCap bounds the palette.
const paletteCap = 16

// remember records a confirmed color, most recent first.
func (p *appPalette) remember(col render.Color) {
	for i, c := range p.colors {
		if c == col {
			p.colors = append(p.colors[:i], p.colors[i+1:]...)
			break
		}
	}
	p.colors = append([]render.Color{col}, p.colors...)
	if len(p.colors) > paletteCap {
		p.colors = p.colors[:paletteCap]
	}
}

// ColorChooserDialog opens a modal color picker (#72): an SV square,
// hue and alpha strips, a hex entry, theme-derived presets, and this
// application's custom palette. Confirming responds "ok" and fires
// onColor with the exact picked color (and the pick joins the
// palette); Esc responds "cancel".
func (a *Application) ColorChooserDialog(parent *Window, initial render.Color, onColor func(render.Color)) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	chooser := widget.NewColorChooser(face, 13, initial)
	chooser.SetPaletteSource(func() []render.Color { return a.palette.colors }, a.palette.remember)
	if a.eyedropperAvailable() {
		chooser.SetEyedropper(func() { a.startEyedrop(chooser) })
	}
	return a.NewDialog(parent, DialogConfig{
		Title:   widget.Tr("Pick a color"),
		Width:   300,
		Height:  310,
		Content: chooser,
		Modal:   true,
		Buttons: []DialogButton{
			{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel},
			{Label: widget.Tr("Select"), Response: "ok", Role: ButtonRoleDefault},
		},
		DefaultResponse: "ok",
		CancelResponse:  "cancel",
		OnResponse: func(resp string) {
			if resp == "ok" {
				a.palette.remember(chooser.Color())
				if onColor != nil {
					onColor(chooser.Color())
				}
			}
		},
	})
}

// fontFilter narrows a family list by the chooser's controls: query
// is a case-insensitive substring over the family name, monoOnly keeps
// families whose regular face is fixed-width (isMono classifies,
// cached by the caller). Deterministic: the result keeps the input
// order.
func fontFilter(families []string, query string, monoOnly bool, isMono func(string) bool) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	out := families[:0:0]
	for _, fam := range families {
		if query != "" && !strings.Contains(fam, query) {
			continue
		}
		if monoOnly && !isMono(fam) {
			continue
		}
		out = append(out, fam)
	}
	return out
}

// fontRowModel serves the chooser's family list: rows lazily render in
// their own family's face (the virtualized list only builds the rows
// it shows, so only the visible faces ever parse).
type fontRowModel struct {
	face render.Font
	fams []string
}

func (m *fontRowModel) Len() int { return len(m.fams) }

func (m *fontRowModel) Row(i int) widget.Widget {
	return widget.NewLabel(m.face, 13, sysfont.FamilyDisplay(m.fams[i]), 0)
}

// FontChooserDialog opens a modal font picker (#74): a searchable,
// virtualized family list (each row lazily rendered in its own
// family's face), a size entry with a synced slider, a monospace
// filter toggle, and a live preview line shaped through the same
// fallback chain production text uses. Confirming responds "ok" and
// fires onFont with the family name sysfont.Best accepts and the
// chosen pixel size; Esc responds "cancel". The monospace toggle
// classifies families by their fixed-width flag on first use (cached;
// face parses ride the shared face cache).
func (a *Application) FontChooserDialog(parent *Window, initialFamily string, initialSize float64, onFont func(family string, size float64)) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	families, err := sysfont.Families()
	if err != nil {
		return nil, fmt.Errorf("app: font chooser: %w", err)
	}
	if initialSize <= 0 {
		initialSize = 13
	}

	th := widget.Current()
	monoCache := map[string]bool{}
	isMono := func(fam string) bool {
		if v, ok := monoCache[fam]; ok {
			return v
		}
		v := false
		if tf, err := sysfont.Best(fam, 13); err == nil {
			v = tf.IsMonospace()
		}
		monoCache[fam] = v
		return v
	}

	model := &fontRowModel{face: face, fams: slices.Clone(families)}
	list := widget.NewList(model, 26)
	if idx := slices.Index(model.fams, strings.ToLower(strings.TrimSpace(initialFamily))); idx >= 0 {
		list.Select(idx)
	}

	search := widget.NewEntry(face, 13, th.Text)
	search.SetPlaceholder("search families")
	mono := widget.NewCheckButton(false)
	sizeEntry := widget.NewEntry(face, 13, th.Text)
	sizeEntry.SetText(strconv.FormatFloat(initialSize, 'f', -1, 64))
	size := widget.NewSlider(6, 72, 1, initialSize)

	// The preview rebuilds its face through the production chain on
	// every applied family or size change: what you see is what ships.
	preview := widget.NewLabel(face, initialSize, "The quick brown fox jumps over the lazy dog 0123", 0)
	chosenSize := func() float64 {
		if v, err := strconv.ParseFloat(strings.TrimSpace(sizeEntry.Text()), 64); err == nil && v > 0 && v <= 512 {
			return v
		}
		return size.Value()
	}
	refreshPreview := func() {
		fam := ""
		if i := list.Selected(); i >= 0 && i < len(model.fams) {
			fam = model.fams[i]
		}
		if tf, err := sysfont.Best(fam, 13); err == nil {
			preview.SetFace(sysfont.Fallback(tf))
		}
		preview.SetSizePx(chosenSize())
	}

	refilter := func() {
		model.fams = fontFilter(families, search.Text(), mono.Checked(), isMono)
		list.Changed()
		refreshPreview()
	}
	search.OnChanged = func(string) { refilter() }
	mono.OnChanged = func(bool) { refilter() }
	size.OnChanged = func(v float64) {
		sizeEntry.SetText(strconv.FormatFloat(v, 'f', -1, 64))
		refreshPreview()
	}
	sizeEntry.OnChanged = func(string) { refreshPreview() }
	list.OnSelect = func(int) { refreshPreview() }

	controls := widget.NewBox(widget.Row, 8, 0)
	controls.Append(search, true)
	controls.Append(mono, false)
	sizeRow := widget.NewBox(widget.Row, 8, 0)
	sizeLabel := widget.NewLabel(face, 12, "size", 0)
	sizeLabel.AddClass(css.DimLabel)
	sizeRow.Append(sizeLabel, false)
	sizeRow.Append(size, true)
	sizeRow.Append(sizeEntry, false)
	root := widget.NewBox(widget.Column, 8, 8)
	root.Append(controls, false)
	root.Append(list, true)
	root.Append(sizeRow, false)
	root.Append(preview, false)
	refreshPreview()

	return a.NewDialog(parent, DialogConfig{
		Title:   widget.Tr("Pick a font"),
		Width:   380,
		Height:  420,
		Content: root,
		Modal:   true,
		Buttons: []DialogButton{
			{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel},
			{Label: widget.Tr("Select"), Response: "ok", Role: ButtonRoleDefault},
		},
		DefaultResponse: "ok",
		CancelResponse:  "cancel",
		OnResponse: func(resp string) {
			if resp != "ok" || onFont == nil {
				return
			}
			fam := ""
			if i := list.Selected(); i >= 0 && i < len(model.fams) {
				fam = model.fams[i]
			}
			onFont(fam, chosenSize())
		},
	})
}

// CalendarDialog opens a modal calendar picker: a widget.Calendar
// starting at initial, responding "select" with the chosen date
// through OnResponse when a day is picked (the dialog closes) and
// "cancel" on Esc. onSelect is the Calendar's own hook for apps that
// want every applied selection, not just the closing one.
func (a *Application) CalendarDialog(parent *Window, initial widget.Date, onSelect func(widget.Date)) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	cal := widget.NewCalendar(face, 14, initial)
	var d *Dialog
	cal.OnSelect = func(day widget.Date) {
		if onSelect != nil {
			onSelect(day)
		}
		if d != nil {
			d.Respond("select")
		}
	}
	var err error
	d, err = a.NewDialog(parent, DialogConfig{
		Title:          widget.Tr("Select date"),
		Width:          300,
		Height:         380,
		Content:        cal,
		Modal:          true,
		Buttons:        []DialogButton{{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel}},
		CancelResponse: "cancel",
	})
	return d, err
}

// MessageBox opens the standard preset, shadcn/ui's AlertDialog: the
// title over the muted text, wrapped, above the buttons, the dialog as
// tall as they need.
func (a *Application) MessageBox(parent *Window, title, text string, buttons []DialogButton) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	body := widget.NewLabel(face, 14, text, 0)
	body.SetWrap(true)
	body.AddClass(css.DimLabel)
	return a.NewDialog(parent, DialogConfig{
		Title:           title,
		Width:           440,
		Content:         body,
		Buttons:         buttons,
		Modal:           true,
		DefaultResponse: defaultResponse(buttons),
		CancelResponse:  cancelResponse(buttons),
	})
}

// defaultResponse is the response Enter fires: the button marked
// ButtonRoleDefault, else the first button (the positional fallback).
func defaultResponse(buttons []DialogButton) string {
	for _, b := range buttons {
		if b.Role == ButtonRoleDefault {
			return b.Response
		}
	}
	if len(buttons) > 0 {
		return buttons[0].Response
	}
	return ""
}

// cancelResponse is the response Escape fires: the button marked
// ButtonRoleCancel, else the last button (the positional fallback).
func cancelResponse(buttons []DialogButton) string {
	for _, b := range buttons {
		if b.Role == ButtonRoleCancel {
			return b.Response
		}
	}
	if len(buttons) > 0 {
		return buttons[len(buttons)-1].Response
	}
	return ""
}

// ErrNoDialogFace reports a system with no usable text face - no
// configured tooltip face and no system sans font. Every dialog
// builder (message boxes, file dialogs, about) returns it.
var ErrNoDialogFace = errors.New("app: dialog text face unavailable: no configured tooltip face and the system has no sans font")

// dialogRoot builds the dialog window's root: the content in the
// toolkit's card, shadcn/ui's Dialog: the title, the content and the
// footer of buttons, the default response's on the primary fill and the
// others outlined; or, Bare, the content alone.
func (a *Application) dialogRoot(cfg DialogConfig, respond func(string)) (widget.Widget, render.Color, error) {
	if cfg.Bare {
		return cfg.Content, cfg.Background, nil
	}
	if len(cfg.Buttons) == 0 {
		cfg.Buttons = []DialogButton{{Label: widget.Tr("OK"), Response: "ok"}}
	}
	// The title and the button row need a face; under the toolkit-wide
	// nil-face contract a nil face into a constructor panics, so a
	// system without any usable font surfaces as an error here.
	face := a.resolveFace(nil)
	if face == nil {
		return nil, 0, ErrNoDialogFace
	}
	primary := cfg.DefaultResponse
	if primary == "" {
		primary = defaultResponse(cfg.Buttons)
	}
	footer := widget.NewBox(widget.Row, 8, 0)
	footer.Append(widget.NewSpacer(0, 0), true)
	for _, b := range cfg.Buttons {
		resp := b.Response
		btn := widget.NewButton(widget.NewLabel(face, 14, b.Label, 0), 0, 0)
		variant := css.Outline
		if resp == primary {
			variant = css.SuggestedAction
		}
		btn.AddClass(variant)
		btn.OnClick = func() { respond(resp) }
		footer.Append(btn, false)
	}
	content := widget.NewBox(widget.Column, 16, 24)
	if cfg.Title != "" {
		title := widget.NewLabel(face, 18, cfg.Title, 0)
		title.AddClass("title")
		content.Append(title, false)
	}
	content.Append(cfg.Content, true)
	content.Append(footer, false)
	root, background := dialogCard(content)
	return root, background, nil
}

// Window returns the dialog's own window, for toasts and popovers
// inside the dialog.
func (d *Dialog) Window() *Window { return d.win }

// PreferencesDialog is libadwaita's AdwPreferencesDialog: a dialog
// around a widget.PreferencesView whose toasts show inside it.
type PreferencesDialog struct {
	*Dialog
	View *widget.PreferencesView
}

// PreferencesDialog opens view in a dialog over parent titled title,
// closed by its Close button or Esc.
func (a *Application) PreferencesDialog(parent *Window, title string, view *widget.PreferencesView) (*PreferencesDialog, error) {
	d, err := a.NewDialog(parent, DialogConfig{
		Title:          title,
		Width:          640,
		Height:         560,
		Content:        view,
		Buttons:        []DialogButton{{Label: widget.Tr("Close"), Response: "close", Role: ButtonRoleCancel}},
		CancelResponse: "close",
	})
	if err != nil {
		return nil, err
	}
	return &PreferencesDialog{Dialog: d, View: view}, nil
}

// AddToast shows text as a toast inside the dialog.
func (p *PreferencesDialog) AddToast(text string, timeout time.Duration) (*widget.Toast, error) {
	return p.app.ShowToast(text, timeout, &ToastConfig{Host: p.win})
}
