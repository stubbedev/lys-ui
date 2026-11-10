package headlesstest

import (
	"errors"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

// An application mints an activation token on the test compositor and
// it raises another window: the first window, unfocused once the
// second opened, takes the focus back with the second one's token.
func TestHeadlessActivationToken(t *testing.T) {
	requireEnv(t)
	sess, err := app.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	a := app.NewApplication(sess)
	defer a.Close()
	cfg := func(title string) app.WindowConfig {
		return app.WindowConfig{Title: title, AppID: "dev.stubbe.lys.ui.activation", Width: 200, Height: 120, Root: widget.NewBox(widget.Column, 0, 0)}
	}
	first, err := a.NewWindow(cfg("activation first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.NewWindow(cfg("activation second"))
	if err != nil {
		t.Fatal(err)
	}
	in := newInput(t)
	var token string
	var tokenErr error
	const keyShift = 42
	deadline := time.Now().Add(10 * time.Second)
	phase := 0
	a.Every(50*time.Millisecond, func() {
		switch {
		case time.Now().After(deadline):
			a.Quit()
		case phase == 0 && second.State().Activated && !first.State().Activated:
			// A key on the focused window: the token's anchoring
			// interaction, without which the compositor only marks
			// the target urgent.
			if err := in.Tap(keyShift); err != nil {
				t.Errorf("tap: %v", err)
				a.Quit()
				return
			}
			phase = 1
		case phase == 1:
			phase = 3
			second.RequestActivationToken(func(tok string, err error) {
				token, tokenErr = tok, err
				phase = 2
				first.Activate(tok)
			})
		case phase == 2 && first.State().Activated:
			a.Quit()
		}
	})
	if err := a.Run(); err != nil && !errors.Is(err, app.ErrClosed) {
		t.Fatal(err)
	}
	if tokenErr != nil || token == "" {
		t.Fatalf("token %q, err %v, phase %d (first %v, second %v); want a token", token, tokenErr, phase, first.State().Activated, second.State().Activated)
	}
	if !first.State().Activated {
		t.Errorf("the token did not activate the first window (phase %d)", phase)
	}
}
