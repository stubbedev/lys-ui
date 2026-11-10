package component_test

import (
	"errors"
	"log"
	"strconv"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

type Msg int

const (
	Increment Msg = iota
	Decrement
)

type Counter struct {
	env ui.Env
	n   int
}

func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	return ui.Mount(cx, c.env, ui.Row(
		ui.Button(ui.Label("-"), 8, 6).OnClick(func() { cx.Input(Decrement) }),
		ui.Expand(ui.Label("").WatchText(func() string { return strconv.Itoa(c.n) })),
		ui.Button(ui.Label("+"), 8, 6).OnClick(func() { cx.Input(Increment) }),
	).Spacing(6))
}

func (c *Counter) Update(cx *component.Context[Msg, int], msg Msg) {
	switch msg {
	case Increment:
		c.n++
	case Decrement:
		c.n--
	}
	cx.Output(c.n)
}

func ExampleWindow() {
	sess, err := app.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer sess.Close()
	face, err := app.Font("sans", 15)
	if err != nil {
		log.Fatal(err)
	}
	application := app.NewApplication(sess)
	defer application.Close()
	if _, _, err := component.Window(application, app.WindowConfig{
		Title: "counter", AppID: "dev.example.counter",
	}, &Counter{env: ui.Env{Face: face, Size: 15}}); err != nil {
		log.Fatal(err)
	}
	if err := application.Run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func ExampleRun() {
	err := component.Run(app.WindowConfig{Title: "counter", AppID: "dev.example.counter"},
		func(a *app.Application) (*Counter, error) {
			face, err := app.Font("sans-serif", 15)
			if err != nil {
				return nil, err
			}
			if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
				return nil, err
			}
			return &Counter{env: ui.Env{Face: face, Size: 15}}, nil
		})
	if err != nil {
		log.Fatal(err)
	}
}
