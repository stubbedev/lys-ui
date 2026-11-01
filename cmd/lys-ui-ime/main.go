// Command lys-ui-ime is the IME composition demo: an entry and a text
// area driven by whatever input method the desktop runs, with a
// status line mirroring each field's composing state and contents -
// the preedit an IME shows at the caret, and the commit that lands
// when composition finishes. Type through an IME (fcitx5, ibus) and
// watch the preedit arrive; Escape quits.
package main

import (
	"errors"
	"log"
	"time"

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
	theme := widget.Current()

	entry := widget.NewEntry(face, 14, theme.Text)
	entry.SetPlaceholder("compose here through your IME")
	area := widget.NewTextArea(face, 14, theme.Text)
	status := widget.NewLabel(face, 12, "idle", theme.TextMuted)
	hint := widget.NewLabel(face, 12, "the status line mirrors preedit (composing) and committed text; Escape quits", theme.TextMuted)
	root := widget.NewBox(widget.Column, 12, 16)
	root.Append(hint, false)
	root.Append(entry, false)
	root.Append(area, true)
	root.Append(status, false)

	last := ""
	application.Every(100*time.Millisecond, func() {
		text := "entry: "
		if entry.Composing() {
			text += "composing "
		}
		text += quote(entry.Text()) + "  area: "
		if area.Composing() {
			text += "composing "
		}
		text += quote(area.Text())
		if text != last {
			last = text
			status.SetText(text)
		}
	})

	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})
	if _, err := application.NewWindow(app.WindowConfig{
		Title:      "lys-ui ime",
		AppID:      "dev.stubbe.lys.ui.ime",
		Width:      480,
		Height:     320,
		Root:       root,
		Background: theme.Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}

func quote(s string) string {
	if len(s) > 40 {
		s = s[:40] + "..."
	}
	return `"` + s + `"`
}
