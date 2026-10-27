// Command lys-ui-panel is the layer-shell demo: a right-anchored panel
// component with a slider, progress bar, switch, checkbox, text entry,
// notes area and a scrollable list, live through the pointer and
// keyboard input stack. -dump renders one frame to a PNG instead.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

const panelWidth = 260

var (
	bgColor   = render.RGB(0x1E, 0x1E, 0x2E)
	accent    = render.RGB(0x89, 0xB4, 0xFA)
	textColor = render.RGB(0xCD, 0xD6, 0xF4)
	muted     = render.RGB(0xA6, 0xAD, 0xC3)
)

func main() {
	dump := flag.String("dump", "", "render one panel frame to a PNG file instead of mapping on the compositor")
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

type brightness float64

type panel struct {
	env   ui.Env
	level float64
}

func (p *panel) Init(cx *component.Context[brightness, struct{}]) widget.Widget {
	return ui.Mount(cx, p.env, p.view(cx.Sender()))
}

func (p *panel) view(send component.Sender[brightness]) ui.Node {
	heading := func(text string) ui.Node { return ui.Label(text).Font(nil, 12).Ink(muted) }
	servers := make([]string, 14)
	for i := range servers {
		servers[i] = fmt.Sprintf("server-%02d.example", i+1)
	}
	return ui.Column(
		ui.Label("lys-ui panel").Font(nil, 17).Ink(accent),
		heading("Brightness"),
		ui.Slider(0, 100, 1, p.level*100).OnChanged(func(v float64) { send.Send(brightness(v / 100)) }),
		ui.ProgressBar(p.level).WatchValue(func() float64 { return p.level }),
		ui.Label("").Font(nil, 12).Ink(muted).WatchText(func() string { return fmt.Sprintf("%d%%", int(p.level*100)) }),
		heading("Preferences"),
		ui.Row(ui.CheckButton(true), ui.Label("Enable notifications")).Spacing(8),
		ui.Row(ui.Switch(false), ui.Label("Night light")).Spacing(8),
		heading("Quick note"),
		ui.Entry().Placeholder("type here"),
		heading("Notes"),
		ui.TextArea().Placeholder("multi-line..."),
		heading("Servers (scroll me)"),
		ui.Expand(ui.Scroll(ui.Column(ui.Each(slices.Values(servers), func(s string) ui.Node {
			return ui.Label(s)
		})...).Spacing(2).Padding(render.UniformInsets(4))).ShowBars(true)),
	).Spacing(12).Padding(render.UniformInsets(12))
}

func (p *panel) Update(_ *component.Context[brightness, struct{}], b brightness) {
	p.level = float64(b)
}

func newPanel() (*panel, error) {
	face, err := app.UIFont(13)
	if err != nil {
		return nil, err
	}
	return &panel{env: ui.Env{Face: app.FontFallback(face), Size: 13, Ink: textColor}, level: 0.5}, nil
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	p, err := newPanel()
	if err != nil {
		return err
	}
	outputs := sess.Outputs()
	if len(outputs) == 0 {
		return errors.New("lys-ui-panel: no output to draw on")
	}
	application := app.NewApplication(sess)
	application.SetClipboard(app.NewClipboard(sess))
	if _, _, err := component.Layer(application, app.LayerConfig{
		Output:        outputs[0],
		Layer:         app.LayerTop,
		Anchor:        app.AnchorTop | app.AnchorBottom | app.AnchorRight,
		Width:         panelWidth,
		ExclusiveZone: panelWidth,
		Keyboard:      app.KeyboardOnDemand,
		Namespace:     "lys-ui-panel",
		Background:    bgColor,
		OnResize:      func(w, h int) { log.Printf("lys-ui-panel: mapped at %dx%d", w, h) },
	}, p); err != nil {
		return err
	}
	return application.Run()
}

func dumpFrame(path string) error {
	p, err := newPanel()
	if err != nil {
		return err
	}
	const dumpH = 480
	root, _ := ui.Build(p.env, p.view(component.Sender[brightness]{}))
	data := make([]byte, render.Stride(panelWidth)*dumpH)
	cv := render.New(data, render.Stride(panelWidth), panelWidth, dumpH)
	cv.Clear(cv.Rect(), bgColor)
	root.Arrange(render.Rect{W: panelWidth, H: dumpH})
	root.Paint(cv)
	return writePNG(data, panelWidth, dumpH, path)
}

func writePNG(data []byte, w, h int, path string) error {
	img := render.NRGBA(data, render.Stride(w), w, h)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
