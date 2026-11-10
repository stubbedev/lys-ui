package component

import (
	"errors"

	"github.com/stubbedev/lys-ui/app"
)

var _ Loop = (*app.Application)(nil)

var (
	errRootSet    = errors.New("component: the config's Root is set; the component provides the root")
	errContentSet = errors.New("component: the config's Content is set; the component provides the content")
)

// Window launches c as the root of a new toplevel window: the
// counterpart of relm4's RelmApp::run. The component shuts down when the
// window closes; a window that fails to open shuts it down at once.
func Window[In, Out any](a *app.Application, cfg app.WindowConfig, c Component[In, Out]) (*Controller[In, Out], *app.Window, error) {
	if cfg.Root != nil {
		return nil, nil, errRootSet
	}
	ctrl := Launch(a, c)
	cfg.Root = ctrl.Widget()
	cfg.OnClosed = chain(ctrl.Shutdown, cfg.OnClosed)
	w, err := a.NewWindow(cfg)
	if err != nil {
		ctrl.Shutdown()
		return nil, nil, err
	}
	ctrl.cx.surface.attach(w, nil)
	return ctrl, w, nil
}

// Layer launches c as the root of a new layer surface, like Window.
func Layer[In, Out any](a *app.Application, cfg app.LayerConfig, c Component[In, Out]) (*Controller[In, Out], *app.LayerWindow, error) {
	if cfg.Root != nil {
		return nil, nil, errRootSet
	}
	ctrl := Launch(a, c)
	cfg.Root = ctrl.Widget()
	cfg.OnClosed = chain(ctrl.Shutdown, cfg.OnClosed)
	l, err := a.NewLayer(cfg)
	if err != nil {
		ctrl.Shutdown()
		return nil, nil, err
	}
	ctrl.cx.surface.attach(nil, l)
	return ctrl, l, nil
}

func chain(first, then func()) func() {
	if then == nil {
		return first
	}
	return func() {
		first()
		then()
	}
}

// Run is relm4's RelmApp::run for one window: it connects to the
// compositor, calls setup to build the root component (setup gets the
// Application, to load fonts, register accelerators or open more
// windows), opens it as the root of a window from cfg, and runs the loop
// until the last window closes. A normal close returns nil. The
// application is closed on every way out, a failed setup or window
// included.
func Run[C Component[In, Out], In, Out any](cfg app.WindowConfig, setup func(*app.Application) (C, error)) error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	return runOn(app.NewApplication(sess), cfg, setup)
}

func runOn[C Component[In, Out], In, Out any](a *app.Application, cfg app.WindowConfig, setup func(*app.Application) (C, error)) error {
	defer a.Close()
	c, err := setup(a)
	if err != nil {
		return err
	}
	if _, _, err := Window(a, cfg, Component[In, Out](c)); err != nil {
		return err
	}
	if err := a.Run(); !errors.Is(err, app.ErrClosed) {
		return err
	}
	return nil
}
