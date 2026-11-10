package component

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

func TestRunClosesItsApplicationWhenSetupOrTheWindowFails(t *testing.T) {
	failed := errors.New("no such font")
	for _, c := range []struct {
		name     string
		cfg      app.WindowConfig
		setupErr error
		want     error
	}{
		{"setup fails", app.WindowConfig{}, failed, failed},
		{"the window fails", app.WindowConfig{Root: widget.NewSpacer(1, 1)}, nil, errRootSet},
	} {
		t.Run(c.name, func(t *testing.T) {
			closed := 0
			a := app.NewApplication(&wlsession.Session{})
			err := runOn(a, c.cfg, func(a *app.Application) (*titled, error) {
				a.OnStop(func() { closed++ })
				return &titled{applied: new([]string)}, c.setupErr
			})
			if !errors.Is(err, c.want) {
				t.Errorf("Run returned %v, want %v", err, c.want)
			}
			if closed != 1 {
				t.Errorf("the application was closed %d times, want once", closed)
			}
		})
	}
}
