// ServeAccessibility's consumer contract: the options type is nameable
// outside lys-ui (A11YOptions), and a broken address is the defined
// failure, not a panic.
package app

import (
	"strings"
	"testing"
	"time"
)

// serveResult is ServeAccessibility's answer, pumped onto the test's loop.
func serveResult(t *testing.T, app *Application, opts A11YOptions) error {
	t.Helper()
	var (
		got      error
		answered bool
	)
	app.ServeAccessibility(opts, func(err error) { got, answered = err, true })
	pumpUntil(t, app, func() bool { return answered })
	return got
}

func TestServeAccessibilityTakesExportedOptions(t *testing.T) {
	app := testApp(nil)
	defer app.stopAccessibility()
	err := serveResult(t, app, A11YOptions{Address: "unix:path=/nonexistent/lys-ui-a11y-test"})
	if err == nil {
		t.Fatal("bogus address: want the dial failure")
	}
	if !strings.Contains(err.Error(), "a11y") && !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "dial") {
		t.Fatalf("error %v does not name the accessibility bus", err)
	}
	// Idempotence holds on failure too: a second call is attempted, not
	// wedged by a half-started bridge.
	if err := serveResult(t, app, A11YOptions{Address: "unix:path=/nonexistent/lys-ui-a11y-test"}); err == nil {
		t.Fatal("second ServeAccessibility on the same broken bus should also fail")
	}
}

// Serving, and following the desktop's switch, runs on the worker: a bus
// that never answers leaves ServeAccessibility and the switch's callback
// returning at once on the loop.
func TestServeAccessibilityWaitsOnNoBus(t *testing.T) {
	app := testApp(nil)
	release := make(chan struct{})
	entered := make(chan struct{})
	if !app.a11yQueue().Post(func() {
		close(entered)
		<-release
	}) {
		t.Fatal("the worker refused the holding job")
	}
	<-entered
	answered := false
	app.Invoke(func() {
		app.ServeAccessibility(A11YOptions{Address: "unix:path=/nonexistent/lys-ui-a11y-test"}, func(error) { answered = true })
		app.followA11yStatus(false)
		app.followA11yStatus(true)
	})
	app.pump(time.Now())
	if answered {
		t.Error("the serve answered while the bus held the worker")
	}
	close(release)
	pumpUntil(t, app, func() bool { return answered })
	app.stopAccessibility()
}

// An application cannot sample the bridge by hand, so the negative
// interval that means "the embedder drives sampling" is refused instead
// of starting a bridge that never samples.
func TestServeAccessibilityRefusesNegativePoll(t *testing.T) {
	app := testApp(nil)
	err := serveResult(t, app, A11YOptions{Address: "unix:path=/nonexistent/lys-ui-a11y-test", Poll: -time.Second})
	if err == nil || !strings.Contains(err.Error(), "negative poll interval") {
		t.Fatalf("ServeAccessibility with a negative poll = %v, want the refusal", err)
	}
}

// LYS_UI_A11Y overrides the application's accessibility mode.
func TestA11yModeEnv(t *testing.T) {
	a := &Application{}
	a.SetAccessibility(A11yOn)
	t.Setenv("LYS_UI_A11Y", "")
	if a.effectiveA11yMode() != A11yOn {
		t.Error("the application's mode lost without LYS_UI_A11Y")
	}
	t.Setenv("LYS_UI_A11Y", "none")
	if a.effectiveA11yMode() != A11yOff {
		t.Error("LYS_UI_A11Y=none did not turn the bridge off")
	}
	a.SetAccessibility(A11yOff)
	t.Setenv("LYS_UI_A11Y", "atspi")
	if a.effectiveA11yMode() != A11yOn {
		t.Error("LYS_UI_A11Y=atspi did not turn the bridge on")
	}
	a.stopAccessibility() // nothing serving: a no-op
}
