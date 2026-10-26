// Command lys-ui-invoke demonstrates the threading model. A spawned
// command "polls a sensor" twice a second on its own goroutine and
// sends each reading to the component's Update, while Application.Every
// drives a second label on the loop's own timer wakes. Between ticks
// the loop parks: build with -tags lysdebug, run with LYS_DEBUG=wake,
// and watch it go quiet - two wakeups per second, nothing in between,
// 0% idle CPU.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

type msg interface{ msg() }

type reading struct{ millivolts int }

type tick struct{}

func (reading) msg() {}
func (tick) msg()    {}

type sensor struct {
	app     *app.Application
	env     ui.Env
	reading int
	ticks   int
}

func (s *sensor) Init(cx *component.Context[msg, struct{}]) widget.Widget {
	cx.Spawn(func(ctx context.Context, emit func(msg)) {
		poll := time.NewTicker(500 * time.Millisecond)
		defer poll.Stop()
		for n := 0; ; n++ {
			select {
			case <-ctx.Done():
				return
			case <-poll.C:
				emit(reading{millivolts: 1800 + n%400})
			}
		}
	})
	cx.OnShutdown(s.app.Every(time.Second, func() { cx.Input(tick{}) }))
	cx.WindowTitle(func() string {
		if s.reading == 0 {
			return "lys-ui invoke"
		}
		return fmt.Sprintf("lys-ui invoke: %d mV", s.reading)
	})

	return ui.Mount(cx, s.env, ui.Column(
		ui.Label("").Font(nil, 18).WatchText(func() string {
			if s.reading == 0 {
				return "sensor: -- mV"
			}
			return fmt.Sprintf("sensor: %d mV", s.reading)
		}),
		ui.Label("").WatchText(func() string {
			if s.ticks == 0 {
				return "every: --"
			}
			return fmt.Sprintf("every: %d s on the loop goroutine", s.ticks)
		}),
		ui.Label("a spawned command feeds the top label; Escape quits").Font(nil, 12).Ink(widget.Current().TextMuted),
	).Spacing(10).Padding(render.UniformInsets(16)))
}

func (s *sensor) Update(_ *component.Context[msg, struct{}], m msg) {
	switch m := m.(type) {
	case reading:
		s.reading = m.millivolts
	case tick:
		s.ticks++
	}
}

func main() {
	err := component.Run(app.WindowConfig{
		Title: "lys-ui invoke", AppID: "dev.stubbe.lys.ui.invoke",
		Width: 460, Height: 150, Background: widget.Current().Bg,
	}, func(a *app.Application) (*sensor, error) {
		face, err := app.UIFont(14)
		if err != nil {
			return nil, err
		}
		if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
			return nil, err
		}
		return &sensor{app: a, env: ui.Env{Face: face, Size: 14}}, nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
