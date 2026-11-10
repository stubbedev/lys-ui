// Package app_test holds the godoc examples for the app package. The
// examples need a live Wayland compositor to actually run, so they
// carry no Output comment: go test compiles them against the real API
// without executing.
package app_test

import (
	"errors"
	"fmt"
	"log"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

// ExampleApplication is a minimal app on the lower-level path, public API
// only: one toplevel window with a button that counts clicks.
func ExampleApplication() {
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
	face, err := app.Font("sans", 15)
	if err != nil {
		return err
	}

	clicks := 0
	count := widget.NewLabel(face, 15, "clicked 0 times", widget.Current().Text)
	button := widget.NewButton(count, 10, 8)
	button.OnClick = func() {
		clicks++
		count.SetText(fmt.Sprintf("clicked %d times", clicks))
	}

	application := app.NewApplication(sess)
	defer application.Close()
	if _, err := application.NewWindow(app.WindowConfig{
		Title: "hello", AppID: "dev.stubbe.lys.ui.hello",
		Width: 320, Height: 120,
		Root: button, Background: widget.Current().Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}
