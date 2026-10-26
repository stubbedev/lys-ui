// Command lys-ui-glass shows the theme's glass surfaces over busy,
// moving content: a frosted sidebar, a clear-glass card and bottom bar
// and a liquid-glass pill over a striped backdrop
// whose bands keep sliding, so
// the backdrop blur re-samples every frame. w opens a translucent
// window that asks the compositor to blur the desktop behind it
// (ext_background_effect_v1; opaque where no compositor blur is in
// effect; -blur-window opens it at startup), t toggles light and dark,
// Escape closes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

func main() {
	blurWindow := flag.Bool("blur-window", false, "open the compositor-blurred window at startup")
	flag.Parse()
	err := run(*blurWindow)
	switch {
	case err == nil, errors.Is(err, app.ErrClosed):
	case errors.Is(err, app.ErrDisconnected):
		os.Exit(app.DisconnectExitCode)
	default:
		log.Fatal(err)
	}
}

// stripes is the busy backdrop: diagonal bands whose phase the loop
// advances, so the glass above has something moving to blur.
const stripes = `.stripes { background-image: repeating-linear-gradient(%ddeg, #e05561 0px, #e05561 18px, #3fb68b 18px, #3fb68b 36px, #4f7cff 36px, #4f7cff 54px, #f0c040 54px, #f0c040 72px); }`

func run(blurWindow bool) error {
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

	backdrop := widget.NewBox(widget.Column, 0, 0)
	backdrop.AddClass("stripes")
	widget.LoadStylesheet(fmt.Sprintf(stripes, 45))

	panel := func(min string, rows ...string) *widget.Box {
		b := widget.NewBox(widget.Column, 6, 16)
		b.AddClass(css.Glass)
		for i, text := range rows {
			size := 13.0
			if i == 0 {
				size = 16
			}
			b.Append(widget.NewLabel(face, size, text, 0), false)
		}
		b.SetInlineStyle(min)
		return b
	}
	sidebar := panel("min-width: 180px;", "Library", "Recent", "Favorites", "Shared", "Trash")
	sidebar.RemoveClass(css.Glass)
	sidebar.AddClass(css.Frosted)
	card := panel("min-width: 260px;", "Clear glass", "The backdrop blurs and saturates", "under a translucent card.")
	bar := panel("min-height: 40px;", "Playing - the bands keep moving underneath")
	pill := widget.NewBox(widget.Column, 4, 18)
	pill.AddClass(css.LiquidGlass)
	pill.Append(widget.NewLabel(face, 16, "Liquid glass", 0), false).
		Append(widget.NewLabel(face, 13, "the rim bends the bands behind it", 0), false)

	stage := widget.NewOverlay().
		Append(backdrop).
		AppendAligned(sidebar, widget.AlignStart, widget.AlignFill).
		AppendAligned(card, widget.AlignCenter, widget.AlignCenter).
		AppendAligned(bar, widget.AlignFill, widget.AlignEnd).
		AppendAligned(pill, widget.AlignEnd, widget.AlignStart)

	phase := 45
	application.Every(33*time.Millisecond, func() {
		phase = (phase + 1) % 360
		widget.LoadStylesheet(fmt.Sprintf(stripes, phase))
	})

	_, err = application.NewWindow(app.WindowConfig{
		Title:      "lys-ui glass",
		AppID:      "dev.stubbe.lys.ui.glass",
		Width:      820,
		Height:     520,
		Root:       stage,
		Background: render.RGB(0, 0, 0),
		OnKey: func(_ *widget.Router, code uint32, _ app.Mods) {
			switch application.KeySym(code) {
			case xkb.KeyEscape:
				application.Quit()
			case xkb.Keysym('w'):
				if err := openBlurred(application, face); err != nil {
					log.Printf("blurred window: %v", err)
				}
			case xkb.Keysym('t'):
				if widget.Current().IsDark() {
					widget.SetTheme(widget.LightTheme())
				} else {
					widget.SetTheme(widget.DarkTheme())
				}
			}
		},
	})
	if err != nil {
		return err
	}
	if blurWindow {
		if err := openBlurred(application, face); err != nil {
			return err
		}
	}
	return application.Run()
}

// openBlurred opens a translucent .glass window that asks the
// compositor to blur the desktop behind it, and says whether it does.
func openBlurred(application *app.Application, face render.Font) error {
	root := widget.NewBox(widget.Column, 8, 24)
	root.AddClass(css.Glass)
	status := widget.NewLabel(face, 14, "", 0)
	root.Append(widget.NewLabel(face, 18, "Compositor blur", 0), false).Append(status, false)
	stop := func() {}
	w, err := application.NewWindow(app.WindowConfig{
		Title:          "lys-ui glass - compositor blur",
		AppID:          "dev.stubbe.lys.ui.glass",
		Width:          420,
		Height:         220,
		Root:           root,
		Background:     render.RGBA(0, 0, 0, 0),
		BackgroundBlur: true,
		OnClosed:       func() { stop() },
	})
	if err != nil {
		return err
	}
	stop = application.Every(500*time.Millisecond, func() {
		if w.BackgroundBlurActive() {
			status.SetText("the compositor blurs the desktop behind this window")
		} else {
			status.SetText("no compositor blur here: the window stays opaque")
		}
	})
	return nil
}
