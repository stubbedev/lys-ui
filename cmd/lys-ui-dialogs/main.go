// Command lys-ui-dialogs is the dialogs showcase: one window of
// buttons opening every preset dialog constructor - MessageBox,
// PromptDialog, ColorChooserDialog, FontChooserDialog,
// CalendarDialog - plus the file dialogs, the adaptive dialog, and
// the about dialog, all parented to the window so modality and
// Escape behavior can be compared side by side. The status line
// records every response.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	face, err := app.UIFont(14)
	if err != nil {
		return err
	}
	application := app.NewApplication(sess)
	theme := widget.Current()

	status := widget.NewLabel(face, 13, "no dialog answered yet", theme.TextMuted)
	say := func(format string, args ...any) {
		status.SetText(fmt.Sprintf(format, args...))
	}

	var w *app.Window
	button := func(label string, fn func() error) *widget.Button {
		b := widget.NewButton(widget.NewLabel(face, 13, label, theme.Text), 8, 6)
		b.OnClick = func() {
			if err := fn(); err != nil {
				say("dialog error: %v", err)
			}
		}
		return b
	}
	dialogs := widget.NewFlowBox(8, 8)
	dialogs.Append(button("message", func() error {
		_, err := application.MessageBox(w, "Lunch?", "Where shall we eat?", []app.DialogButton{
			{Label: "Cancel", Response: "cancel", Role: app.ButtonRoleCancel},
			{Label: "Kitchen", Response: "kitchen", Role: app.ButtonRoleDefault},
		})
		if err == nil {
			say("message opened; answer in the dialog")
		}
		return err
	}))
	dialogs.Append(button("prompt", func() error {
		_, err := application.PromptDialog(w, "Name the pet", "Give it a name:", "", false, func(text string) {
			say("prompt answered %q", text)
		})
		return err
	}))
	dialogs.Append(button("color", func() error {
		_, err := application.ColorChooserDialog(w, theme.Accent, func(c render.Color) {
			say("color chosen %06x", uint32(c)>>8)
		})
		return err
	}))
	dialogs.Append(button("font", func() error {
		_, err := application.FontChooserDialog(w, "sans-serif", 14, func(family string, size float64) {
			say("font chosen %s %gpx", family, size)
		})
		return err
	}))
	dialogs.Append(button("calendar", func() error {
		_, err := application.CalendarDialog(w, widget.Today(), func(d widget.Date) {
			say("date chosen %s", d)
		})
		return err
	}))
	dialogs.Append(button("open file", func() error {
		_, err := application.OpenFileDialog(w, app.FileDialogConfig{
			Recents: true,
			OnOpen:  func(paths []string) { say("opened %d file(s)", len(paths)) },
		})
		return err
	}))
	dialogs.Append(button("save file", func() error {
		_, err := application.SaveFileDialog(w, app.SaveDialogConfig{
			StartName: "untitled.png",
			OnSave:    func(path string) { say("saving to %s", path) },
		})
		return err
	}))
	dialogs.Append(button("adaptive", func() error {
		content := widget.NewBox(widget.Column, 8, 16)
		content.Append(widget.NewLabel(face, 14, "narrow the window: this becomes a bottom sheet", theme.Text), false)
		_, err := application.AdaptiveDialog(w, app.AdaptiveDialogConfig{
			Title:   "Adaptive dialog",
			Content: content,
		})
		return err
	}))
	dialogs.Append(button("about", func() error {
		_, err := application.AboutDialog(w, app.AboutConfig{
			Name:     "lys-ui",
			Version:  "0.1.0",
			Comments: "a pure-Go Wayland toolkit",
			Website:  "https://github.com/stubbedev/lys-ui",
			License:  "MIT",
			Credits:  []app.AboutCredit{{Role: "Authors", Names: []string{"stubbe"}}},
		})
		return err
	}))

	hint := widget.NewLabel(face, 12, "Escape cancels the preset dialogs; Enter picks their default", theme.TextMuted)
	root := widget.NewBox(widget.Column, 14, 16)
	root.Append(hint, false)
	root.Append(dialogs, true)
	root.Append(status, false)

	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})
	w, err = application.NewWindow(app.WindowConfig{
		Title:      "lys-ui dialogs",
		AppID:      "dev.stubbe.lys.ui.dialogs",
		Width:      520,
		Height:     320,
		Root:       root,
		Background: theme.Bg,
	})
	if err != nil {
		return err
	}
	return application.Run()
}
