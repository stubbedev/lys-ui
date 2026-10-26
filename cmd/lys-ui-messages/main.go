// Command lys-ui-messages demonstrates the typed messaging layer
// (app/message.go) and single-instance activation (app/instance.go):
// one SharedState counter shared across two windows - either window's
// buttons move both labels - and one Stream broker that any depth of
// code can reach without threading senders: both windows subscribe, a
// button publishes, every window toasts. The click path is a component
// (package component): clicks cross onto the loop as typed input. A second
// run of the binary is the GApplication remote: it forwards its argv
// and --open paths to the primary, which toasts them, and exits.
package main

import (
	"cmp"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/widget"
)

// clickMsg is the click component's input: which window was clicked.
type clickMsg struct{ window string }

// lastClick is the click path as a component: clicks cross onto the
// loop as typed input, bump the shared counter, and name the window.
type lastClick struct {
	face    *render.Typeface
	counter *app.SharedState[int]
	window  string
}

func (c *lastClick) Init(cx *component.Context[clickMsg, struct{}]) widget.Widget {
	label := widget.NewLabel(c.face, 12, "", widget.Current().TextMuted)
	cx.Watch(func() { label.SetText("last click: " + cmp.Or(c.window, "--")) })
	return label
}

func (c *lastClick) Update(_ *component.Context[clickMsg, struct{}], m clickMsg) {
	c.counter.Update(func(n *int) { *n++ })
	c.window = m.window
}

// announceMsg is the broker message: a toast line for every window.
type announceMsg struct{ text string }

func main() {
	if err := run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func run() error {
	// The single-instance guard runs before anything else: a second
	// invocation forwards its argv and --open paths to the primary and
	// exits, never touching the compositor (GApplication's remote
	// activation). The primary toasts whatever arrives.
	var broker *app.Stream[announceMsg]
	ins, primary, err := app.ClaimInstance(app.InstanceConfig{
		AppID: "dev.stubbe.lys.ui.messages",
		OnCommandLine: func(args []string, _ string) {
			broker.Send(announceMsg{text: "remote invocation: " + strings.Join(args, " ")})
		},
		OnOpen: func(paths []string, _ string) {
			broker.Send(announceMsg{text: "open: " + strings.Join(paths, ", ")})
		},
	}, app.OSInvocation(os.Args[1:]))
	if err != nil {
		return err
	}
	if !primary {
		return nil
	}
	defer ins.Close()

	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	tf, err := app.UIFont(14)
	if err != nil {
		return err
	}

	application := app.NewApplication(sess)
	ins.Bind(application)
	theme := widget.Current()

	broker = app.NewStream[announceMsg](application)
	broker.Subscribe(func(m announceMsg) {
		_, _ = application.ShowToast(m.text, 3*time.Second, nil)
	})

	// The shared state: one counter, every window's label subscribes, so
	// no per-window copy can fall out of sync.
	counter := app.NewSharedState(application, 0)

	clicks := component.Launch(application, &lastClick{face: tf, counter: counter})

	window := func(title string, pingText string) *widget.Box {
		count := widget.NewLabel(tf, 28, "0", theme.Text)
		counter.Subscribe(func(n int) {
			count.SetText(strconv.Itoa(n))
		})
		plus := widget.NewButton(widget.NewLabel(tf, 14, "increment", theme.OnAccent), 10, 6)
		plus.OnClick = func() { clicks.Send(clickMsg{window: title}) }
		ping := widget.NewButton(widget.NewLabel(tf, 14, "ping both", theme.OnAccent), 10, 6)
		ping.OnClick = func() { broker.Send(announceMsg{text: pingText}) }
		hint := widget.NewLabel(tf, 12, "increments move both counters; a second run of this demo forwards here; Escape quits", theme.TextMuted)
		box := widget.NewBox(widget.Column, 10, 24)
		box.Append(count, false)
		box.Append(plus, false)
		box.Append(ping, false)
		box.Append(hint, false)
		return box
	}

	application.OnKey(func(_ *widget.Router, code uint32, mods app.Mods) {
		if mods&app.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})

	rootA := window("lys-ui messages a", "ping from the left window")
	rootA.Append(clicks.Widget(), false)
	rootB := window("lys-ui messages b", "ping from the right window")
	for _, cfg := range []app.WindowConfig{
		{Title: "lys-ui messages a", AppID: "dev.stubbe.lys.ui.messages", Width: 380, Height: 300, Root: rootA, Background: theme.Bg},
		{Title: "lys-ui messages b", AppID: "dev.stubbe.lys.ui.messages", Width: 380, Height: 260, Root: rootB, Background: theme.Bg},
	} {
		if _, err := application.NewWindow(cfg); err != nil {
			return err
		}
	}
	return application.Run()
}
