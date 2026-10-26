// The application side of the AT-SPI bridge (#65, #114): the
// application is the bridge's Scene - window roots, the focused widget
// of the topmost window, AT-driven focus, and the loop queue - and Run
// starts serving the semantic model on the accessibility bus when the
// desktop asks for assistive technologies (A11yAuto, the default).
package app

import (
	"fmt"
	"os"
	"slices"

	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/atspi"
	"github.com/stubbedev/lys-ui/widget"
)

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
// follow the desktop's switch (A11yAuto), hopping onto the loop to
// start and stop the bridge. A desktop without a session bus or
// at-spi2 has no switch: nothing serves.
func (a *Application) startAccessibility() {
	switch a.effectiveA11yMode() {
	case A11yOn:
		if err := a.ServeAccessibility(A11YOptions{}); err != nil {
			debug.Log("a11y", "serve: %v", err)
		}
	case A11yAuto:
		stop, err := atspi.WatchStatus("", func(on bool) {
			a.Invoke(func() { a.followA11yStatus(on) })
		})
		if err != nil {
			debug.Log("a11y", "no accessibility switch: %v", err)
			return
		}
		a.stopA11yWatch = stop
	}
}

// followA11yStatus starts or stops the bridge with the desktop's
// switch.
func (a *Application) followA11yStatus(on bool) {
	if !on {
		a.stopAccessibility()
		return
	}
	if err := a.ServeAccessibility(A11YOptions{}); err != nil {
		debug.Log("a11y", "serve: %v", err)
	}
}

// stopAccessibility stops the bridge if it serves.
func (a *Application) stopAccessibility() {
	if a.stopA11y != nil {
		a.stopA11y()
		a.stopA11y = nil
	}
}

// ServeAccessibility starts the in-process AT-SPI bridge: the
// accessibility bus is discovered through org.a11y.Bus (opts
// overrides), the window trees are exported at the AT-SPI root, and
// the registry handshake runs. Idempotent. Call before Run (the same
// window SetRoot et al. honor) or from the loop; the bridge stops with
// Run. The first sample queues through Invoke and drains once Run
// starts. Run calls it itself under A11yOn, and under A11yAuto when
// the desktop asks for assistive technologies.
func (a *Application) ServeAccessibility(opts A11YOptions) error {
	if a.stopA11y != nil {
		return nil
	}
	// A negative interval means the embedder samples by hand, which an
	// application cannot: the bridge would never sample.
	if opts.Poll < 0 {
		return fmt.Errorf("app: ServeAccessibility: negative poll interval %v", opts.Poll)
	}
	br, err := atspi.Serve(a, opts)
	if err != nil {
		return err
	}
	a.stopA11y = br.Stop
	return nil
}

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
