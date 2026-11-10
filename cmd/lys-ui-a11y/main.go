// Command lys-ui-a11y is the accessibility demo target: a small form
// whose every control carries a semantic role and label, served to
// screen readers through the in-process AT-SPI bridge. Built with
// -tags atspi it serves the bridge from Run on (A11yOn) whatever the
// desktop's bus says, so Orca or accercierge can be pointed at it
// deterministically; without the tag it follows the desktop's switch
// (A11yAuto). Tab walks the controls; Escape quits.
package main

import (
	"errors"
	"log"
	"strconv"

	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/app"
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
	defer application.Close()
	initA11y(application)
	theme := widget.Current()

	name := widget.NewEntry(face, 14, theme.Text)
	name.SetPlaceholder("your name")
	size := widget.NewBinding(14.0)
	preview := widget.NewLabel(face, 13, "hello, stranger", theme.Text)
	greet := func() {
		who := name.Text()
		if who == "" {
			who = "stranger"
		}
		preview.SetText("hello, " + who + " at " + strconv.Itoa(int(size.Get())) + "px")
	}
	size.Subscribe(func(float64) { greet() })
	name.OnChanged = func(string) { greet() }

	rows := widget.NewPreferencesGroup(face, 14, "Greeting")
	nameRow := widget.NewActionRow(face, 14, "Name", "what the greeting says")
	nameRow.PackEnd(name)
	rows.Add(nameRow)
	slider := widget.NewSlider(10, 30, 1, 14)
	slider.BindValue(size)
	sizeRow := widget.NewActionRow(face, 14, "Size", "greeting size in pixels")
	sizeRow.PackEnd(slider)
	rows.Add(sizeRow)
	hi := widget.NewButtonRow(face, 14, "Say it", greet)
	rows.Add(hi)

	root := widget.NewBox(widget.Column, 12, 16)
	root.Append(rows, false)
	root.Append(preview, false)

	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})
	if _, err := application.NewWindow(app.WindowConfig{
		Title:      "lys-ui a11y",
		AppID:      "dev.stubbe.lys.ui.a11y",
		Width:      420,
		Height:     280,
		Root:       root,
		Background: theme.Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}
