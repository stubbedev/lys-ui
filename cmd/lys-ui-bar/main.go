// Command lys-ui-bar is a layer-shell top bar component: a logo pill, the
// clock, and a seconds gauge, ticked once a second by Application.Every.
// Damage tracking repaints only what changed each second (the gauge,
// and the clock once a minute); build with -tags lysdebug and run with
// LYS_DEBUG=frame to watch the damage rects. -dump renders one frame
// to a PNG instead.
package main

import (
	"bytes"
	"errors"
	"flag"
	"image/png"
	"log"
	"os"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

const lysUILogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <path fill="#89b4fa" d="M12 2 L14.35 8.76 L21.51 8.91 L15.80 13.24 L17.88 20.09 L12 16 L6.12 20.09 L8.20 13.24 L2.49 8.91 L9.65 8.76 Z"/>
</svg>`

const barHeight = 32

var (
	bgColor    = render.RGB(0x1E, 0x1E, 0x2E)
	textColor  = render.RGB(0xCD, 0xD6, 0xF4)
	labelColor = render.RGB(0xBA, 0xB6, 0xC8)
	pillColor  = render.RGB(0x11, 0x11, 0x1B)
)

func main() {
	dump := flag.String("dump", "", "render one bar frame to a PNG file instead of mapping on the compositor")
	flag.Parse()
	var err error
	if *dump != "" {
		err = dumpFrame(*dump)
	} else {
		err = run()
	}
	if err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

type tick time.Time

type bar struct {
	env  ui.Env
	now  time.Time
	logo *render.Icon
}

func newBar() (*bar, error) {
	face, err := app.UIFont(14)
	if err != nil {
		return nil, err
	}
	logo, err := render.LoadSVG([]byte(lysUILogoSVG), 16, 16)
	if err != nil {
		return nil, err
	}
	return &bar{env: ui.Env{Face: app.FontFallback(face), Size: 14, Ink: textColor}, now: time.Now(), logo: logo}, nil
}

func (b *bar) view() ui.Node {
	return ui.Row(
		ui.Aligned(ui.Button(ui.Row(ui.Icon(b.logo), ui.Label("lys-ui").Ink(labelColor)).Spacing(6), 4, 6).
			Bg(pillColor).BgHover(render.RGB(0x18, 0x18, 0x25)).BgPressed(render.RGB(0x0c, 0x0c, 0x14)), widget.AlignCenter),
		ui.Expand(ui.Spacer(0, 0)),
		ui.Aligned(ui.Label("").Font(nil, 17).WatchText(func() string { return b.now.Format("15:04") }), widget.AlignCenter),
		ui.Expand(ui.Spacer(0, 0)),
		ui.Aligned(ui.LevelBar(0).WatchValue(func() float64 { return float64(b.now.Second()) / 59 }), widget.AlignCenter),
	).Spacing(8).Padding(render.Insets{Left: 8, Right: 8})
}

func (b *bar) Init(cx *component.Context[tick, struct{}]) widget.Widget {
	return ui.Mount(cx, b.env, b.view())
}

func (b *bar) Update(_ *component.Context[tick, struct{}], t tick) { b.now = time.Time(t) }

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	b, err := newBar()
	if err != nil {
		return err
	}
	outputs := sess.Outputs()
	if len(outputs) == 0 {
		return errors.New("lys-ui-bar: no output to draw on")
	}
	application := app.NewApplication(sess)
	ctrl, _, err := component.Layer(application, app.LayerConfig{
		Output:        outputs[0],
		Layer:         app.LayerTop,
		Anchor:        app.AnchorTop | app.AnchorLeft | app.AnchorRight,
		Height:        barHeight,
		ExclusiveZone: barHeight,
		Keyboard:      app.KeyboardNone,
		Namespace:     "lys-ui-bar",
		Background:    bgColor,
		OnResize:      func(w, h int) { log.Printf("lys-ui-bar: mapped at %dx%d", w, h) },
	}, b)
	if err != nil {
		return err
	}
	defer application.Every(time.Second, func() { ctrl.Send(tick(time.Now())) })()
	return application.Run()
}

func dumpFrame(path string) error {
	b, err := newBar()
	if err != nil {
		return err
	}
	b.now = time.Date(2026, 1, 1, 9, 41, 17, 0, time.UTC)
	const w, h = 800, barHeight
	root, _ := ui.Build(b.env, b.view())
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	cv.Clear(cv.Rect(), bgColor)
	root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	root.Arrange(render.Rect{W: w, H: h})
	root.Paint(cv)
	img := render.NRGBA(data, render.Stride(w), w, h)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
