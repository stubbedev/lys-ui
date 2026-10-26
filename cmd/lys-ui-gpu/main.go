// Command lys-ui-gpu shows a GPUArea: the area's surface sits below the
// window and the application paces its own frames through OnFrame.
// This demo renders an animated pattern on the CPU and presents it as
// images; an application with a GPU stack imports dmabufs with
// GPUArea.Import and presents those instead, with no copy. A label
// drawn over the area shows lys-ui painting on top of it. Escape quits.
package main

import (
	"image"
	"log"
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

type demo struct {
	env   ui.Env
	area  *widget.GPUArea
	phase float64
	frame *image.RGBA
}

func (d *demo) Init(cx *component.Context[struct{}, struct{}]) widget.Widget {
	return ui.Mount(cx, d.env, ui.Overlay().
		Append(ui.GPUArea().OnFrame(d.render).Ref(&d.area)).
		AppendAligned(ui.Label("lys-ui draws over the GPU surface").Font(nil, 15).Margin(render.UniformInsets(10)),
			widget.AlignCenter, widget.AlignStart))
}

func (d *demo) Update(*component.Context[struct{}, struct{}], struct{}) {}

func (d *demo) render(f widget.GPUFrame) {
	w, h := max(f.Size.W/4, 1), max(f.Size.H/4, 1)
	if d.frame == nil || d.frame.Rect.Dx() != w || d.frame.Rect.Dy() != h {
		d.frame = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	d.phase += 0.05
	for y := range h {
		for x := range w {
			v := math.Sin(float64(x)/9+d.phase) + math.Sin(float64(y)/7-d.phase) + math.Sin(float64(x+y)/13+d.phase/2)
			i := d.frame.PixOffset(x, y)
			d.frame.Pix[i] = uint8(128 + 120*math.Sin(v))
			d.frame.Pix[i+1] = uint8(128 + 120*math.Sin(v+2))
			d.frame.Pix[i+2] = uint8(128 + 120*math.Sin(v+4))
			d.frame.Pix[i+3] = 255
		}
	}
	if err := d.area.PresentImage(d.frame); err != nil {
		log.Print(err)
	}
}

func main() {
	err := component.Run(app.WindowConfig{
		Title: "lys-ui GPUArea", AppID: "dev.stubbe.lys.ui.gpu",
		Width: 640, Height: 400, Background: widget.Current().Bg,
	}, func(a *app.Application) (*demo, error) {
		if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
			return nil, err
		}
		face, err := app.UIFont(15)
		if err != nil {
			return nil, err
		}
		return &demo{env: ui.Env{Face: app.FontFallback(face), Size: 15}}, nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
