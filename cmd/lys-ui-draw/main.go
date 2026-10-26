// Command lys-ui-draw is the custom-drawing demo, the relm4 drawing
// example's analog: a widget.DrawingArea whose OnDraw paints straight
// onto the Canvas - a polar rose sampled per point, a sweep hand, and
// axis lines - with QueueDraw on a timer animating it, damage
// tracking repainting only the area. Escape quits.
package main

import (
	"errors"
	"log"
	"math"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

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
	application := app.NewApplication(sess)
	theme := widget.Current()

	var phase float64
	area := widget.NewDrawingArea(420, 420)
	area.OnDraw = func(cv *render.Canvas, r render.Rect) {
		cv.FillRect(r, theme.Bg)
		cx := float64(r.X + r.W/2)
		cy := float64(r.Y + r.H/2)
		cv.Line(r.X, r.Y+r.H/2, r.X+r.W, r.Y+r.H/2, 1, theme.Border)
		cv.Line(r.X+r.W/2, r.Y, r.X+r.W/2, r.Y+r.H, 1, theme.Border)
		petals := 5.0
		for t := 0.0; t < 2*math.Pi; t += 0.002 {
			rr := 170 * math.Sin(petals*t+phase)
			x := cx + rr*math.Cos(t)
			y := cy + rr*math.Sin(t)
			cv.FillRect(render.Rect{X: int(x), Y: int(y), W: 2, H: 2}, theme.Accent)
		}
		sweep := phase * 3
		cv.Line(int(cx), int(cy), int(cx+150*math.Cos(sweep)), int(cy+150*math.Sin(sweep)), 3, render.RGB(0xF9, 0xE2, 0xAF))
		cv.Arc(cx, cy, 190, 2, phase, math.Pi, theme.TextMuted)
	}
	cancel := application.Every(16*time.Millisecond, func() {
		phase += 0.02
		area.QueueDraw()
	})
	defer cancel()

	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})
	if _, err := application.NewWindow(app.WindowConfig{
		Title:      "lys-ui draw",
		AppID:      "dev.stubbe.lys.ui.draw",
		Width:      460,
		Height:     460,
		Root:       area,
		Background: theme.Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}
