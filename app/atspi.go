// The application side of the AT-SPI bridge (#65, #114): the
// application is the bridge's Scene - window roots, the focused widget
// of the topmost window, AT-driven focus, and the loop queue - and Run
// starts serving the semantic model on the accessibility bus when the
// desktop asks for assistive technologies (A11yAuto, the default).
package app

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/atspi"
	"github.com/stubbedev/lys-ui/internal/worker"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	a11yQueueDepth  = 16
	a11yStopTimeout = 3 * time.Second
)

type a11yRunner struct {
	once      sync.Once
	queue     *worker.Queue
	bridge    *atspi.Bridge
	stopWatch func()
}

func (a *Application) a11yQueue() *worker.Queue {
	a.a11y.once.Do(func() { a.a11y.queue = worker.New(a11yQueueDepth) })
	return a.a11y.queue
}

// A11YOptions configures the AT-SPI bridge (ServeAccessibility).
type A11YOptions = atspi.Options

// A11yMode chooses when the AT-SPI bridge serves.
type A11yMode uint8

// Accessibility modes. The LYS_UI_A11Y environment variable overrides
// the application's choice, like GTK_A11Y: "none" is A11yOff, "atspi"
// A11yOn.
const (
	// A11yAuto serves while the desktop asks for assistive
	// technologies (org.a11y.Status IsEnabled or ScreenReaderEnabled -
	// a screen reader running, the accessibility setting on), starting
	// and stopping with it.
	A11yAuto A11yMode = iota
	// A11yOn serves from Run on, whatever the desktop says.
	A11yOn
	// A11yOff never serves (unless ServeAccessibility is called).
	A11yOff
)

// SetAccessibility chooses when Run serves the bridge; call before Run.
func (a *Application) SetAccessibility(mode A11yMode) { a.a11yMode = mode }

// effectiveA11yMode is the mode after LYS_UI_A11Y.
func (a *Application) effectiveA11yMode() A11yMode {
	switch os.Getenv("LYS_UI_A11Y") {
	case "none", "off", "0":
		return A11yOff
	case "atspi", "on", "1":
		return A11yOn
	}
	return a.a11yMode
}

// startAccessibility applies the mode at Run: serve now (A11yOn), or
// follow the desktop's switch (A11yAuto). The switch's bus and the
// bridge's come up on the worker, so Run's first frame waits on neither.
// A desktop without a session bus or at-spi2 has no switch: nothing
// serves.
func (a *Application) startAccessibility() {
	switch a.effectiveA11yMode() {
	case A11yOn:
		a.ServeAccessibility(A11YOptions{}, nil)
	case A11yAuto:
		a.postA11y("watch the accessibility switch", func() error {
			stop, err := a.watchA11yStatus()
			if err != nil {
				return err
			}
			a.a11y.stopWatch = stop
			return nil
		}, nil)
	}
}

func (a *Application) postA11y(what string, work func() error, done func(error)) {
	if !a.a11yQueue().Post(func() { a.reportDone(what, done, work()) }) {
		a.reportDone(what, done, errors.New("app: the accessibility worker is stuck or stopped"))
	}
}

// followA11yStatus starts or stops the bridge with the desktop's
// switch, on any goroutine.
func (a *Application) followA11yStatus(on bool) {
	if on {
		a.ServeAccessibility(A11YOptions{}, nil)
		return
	}
	a.postA11y("stop the accessibility bridge", a.stopBridge, nil)
}

func (a *Application) stopBridge() error {
	if a.a11y.bridge != nil {
		a.a11y.bridge.Stop()
		a.a11y.bridge = nil
	}
	return nil
}

// stopAccessibility is Close's part: the switch and the bridge stop, after
// whatever the worker still had queued, and the worker ends. Work stuck
// on a bus past the bound still reaches the stop when it returns.
func (a *Application) stopAccessibility() {
	a.postA11y("stop accessibility", func() error {
		if a.a11y.stopWatch != nil {
			a.a11y.stopWatch()
			a.a11y.stopWatch = nil
		}
		return a.stopBridge()
	}, nil)
	if !a.a11yQueue().Stop(a11yStopTimeout) {
		debug.Log("a11y", "the accessibility worker outlived the stop")
	}
}

// ServeAccessibility starts the in-process AT-SPI bridge: the
// accessibility bus is discovered through org.a11y.Bus (opts
// overrides), the window trees are exported at the AT-SPI root, and
// the registry handshake runs. It returns at once and waits on no bus:
// done (nil logs a failure instead) runs on the loop once the bridge
// serves or failed to. Idempotent. Call before Run (the same window
// SetRoot et al. honor) or from any goroutine; the bridge stops with
// Run. The first sample queues through Invoke and drains once Run
// starts. Run calls it itself under A11yOn, and under A11yAuto when
// the desktop asks for assistive technologies.
func (a *Application) ServeAccessibility(opts A11YOptions, done func(error)) {
	// A negative interval means the embedder samples by hand, which an
	// application cannot: the bridge would never sample.
	if opts.Poll < 0 {
		a.reportDone("serve accessibility", done, fmt.Errorf("app: ServeAccessibility: negative poll interval %v", opts.Poll))
		return
	}
	a.postA11y("serve accessibility", func() error {
		if a.a11y.bridge != nil {
			return nil
		}
		br, err := atspi.Serve(a, opts)
		if err != nil {
			return err
		}
		a.a11y.bridge = br
		return nil
	}, done)
}

// A11yGeneration implements atspi.Scene.
func (a *Application) A11yGeneration() uint64 { return widget.A11yGeneration() }

// Roots implements atspi.Scene: the window roots in window order.
// Only ever called on the loop goroutine (from inside Invoke), where
// the window list and the routers are consistent.
func (a *Application) Roots() []widget.Widget {
	out := make([]widget.Widget, 0, len(a.windows))
	for _, w := range a.windows {
		out = append(out, w.router.Root)
	}
	return out
}

// Focused implements atspi.Scene: the focused widget of the topmost
// window (the most recently created one holds input focus), nil when
// nothing does.
func (a *Application) Focused() widget.Widget {
	for _, v := range slices.Backward(a.windows) {
		if f := v.router.Focused(); f != nil {
			return f
		}
	}
	return nil
}

// SetFocus implements atspi.Scene: focus moves to w in the window
// whose tree holds it, through that window's router (the path a click
// takes), and the window repaints.
func (a *Application) SetFocus(w widget.Widget) error {
	hw := a.windowOf(w)
	if hw == nil {
		return fmt.Errorf("app: focus %T: %w", w, widget.ErrNotInTree)
	}
	if err := hw.router.SetFocus(w); err != nil {
		return err
	}
	hw.dirty = true
	return nil
}

func (a *Application) watchA11yStatus() (stop func(), err error) {
	if a.sessionBus != nil {
		return atspi.WatchStatus(a.sessionBus, a.followA11yStatus)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("atspi: session bus: %w", err)
	}
	stopWatch, err := atspi.WatchStatus(conn, a.followA11yStatus)
	if err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return func() {
		stopWatch()
		_ = conn.Close()
	}, nil
}
