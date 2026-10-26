// Package inproc runs lys-ui applications inside a test process against
// the headless compositor and inspects the output through screencopy:
// the in-process half of the compositor-in-the-loop suite, for tests
// that drive the app API directly instead of a demo client.
package inproc

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
)

// Show runs a lys-ui application on its own goroutine with build adding
// its surfaces, and stops it at test cleanup.
func Show(t *testing.T, build func(a *app.Application, out *app.Output) error) {
	t.Helper()
	sess, err := app.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	a := app.NewApplication(sess)
	outs := sess.Outputs()
	if len(outs) == 0 {
		sess.Close()
		t.Fatal("no outputs")
	}
	if err := build(a, outs[0]); err != nil {
		sess.Close()
		t.Fatalf("build: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = a.Run()
	}()
	t.Cleanup(func() {
		a.Invoke(a.Quit)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("application did not quit")
		}
		sess.Close()
	})
}

// Capture connects a screencopy client, closed at test cleanup.
func Capture(t *testing.T) *capture.Client {
	t.Helper()
	c, err := capture.Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if len(c.Outputs()) == 0 {
		t.Fatal("no outputs")
	}
	return c
}

// WantColor checks one pixel of a captured frame against a render
// color, opaque.
func WantColor(img *image.RGBA, x, y int, want render.Color) error {
	return wltest.WantColor(img, x, y, color.RGBA{R: want.R(), G: want.G(), B: want.B(), A: 0xff})
}
