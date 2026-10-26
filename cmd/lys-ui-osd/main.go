// Command lys-ui-osd is the M5 OSD app: a transient layer-shell popup
// anchored top-center over the overlay layer - the volume or
// brightness notification media keys produce. It shows the themed
// icon and a level bar, follows +/- while it is up, and dismisses
// itself after a timeout. The compositor never reserves space for it.
package main

import (
	"errors"
	"flag"
	"log"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

var osdIcons = map[string]string{
	"volume":     "audio-volume-high",
	"brightness": "display-brightness",
}

func main() {
	mode := flag.String("mode", "volume", "volume or brightness")
	value := flag.Float64("value", 0.4, "initial level, 0..1")
	timeout := flag.Duration("timeout", 2*time.Second, "how long the popup stays up")
	flag.Parse()
	if *mode != "volume" && *mode != "brightness" {
		log.Fatalf("lys-ui-osd: unknown mode %q", *mode)
	}
	if err := run(*mode, *value, *timeout); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func run(mode string, level float64, timeout time.Duration) error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	application := app.NewApplication(sess)

	bar := widget.NewLevelBar(level)
	body := widget.NewBox(widget.Row, 12, 12)
	body.Append(widget.NewThemeIcon(osdIcons[mode], 24), false)
	body.Append(bar, false)
	body.SetHomogeneous(false)

	var w *app.LayerWindow
	w, err = application.NewLayer(app.LayerConfig{
		Layer:      app.LayerOverlay,
		Anchor:     app.AnchorTop,
		Width:      220,
		Height:     48,
		Keyboard:   app.KeyboardOnDemand,
		Namespace:  "lys-ui-osd",
		Root:       body,
		Background: render.RGBA(0x1E, 0x1E, 0x2E, 235),
	})
	if err != nil {
		return err
	}
	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt != 0 {
			return
		}
		switch sess.KeySym(code) {
		case xkb.KeyEscape:
			application.Quit()
		case '+', '=':
			bar.SetValue(min(bar.Value()+0.05, 1))
		case '-':
			bar.SetValue(max(bar.Value()-0.05, 0))
		}
	})
	time.AfterFunc(timeout, func() { application.Invoke(w.Close) })
	return application.Run()
}
