package app_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

const cageClientEnv = "LYS_UI_CAGE_CLIENT_DONE"

func TestHeadlessToplevelRunsOnCage(t *testing.T) {
	wltest.Require(t)
	if done := os.Getenv(cageClientEnv); done != "" {
		runCageClient(t, done)
		return
	}
	done := filepath.Join(t.TempDir(), "done")
	t.Setenv(cageClientEnv, done)
	wltest.RuntimeDir(t)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("WLR_BACKENDS", "headless")
	t.Setenv("WLR_RENDERER", "pixman")
	t.Setenv("WLR_LIBINPUT_NO_DEVICES", "1")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "cage", "--", os.Args[0], "-test.v", "-test.run=^"+t.Name()+"$")
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	if _, statErr := os.Stat(done); err != nil || statErr != nil {
		t.Fatalf("the client under cage did not finish: %v, %v\n%s", err, statErr, out)
	}
}

func runCageClient(t *testing.T, done string) {
	t.Helper()
	sess, err := app.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()
	a := app.NewApplication(sess)
	if _, err := a.NewLayer(layerConfig()); !errors.Is(err, app.ErrLayerShellUnavailable) {
		t.Fatalf("NewLayer under cage: err = %v, want ErrLayerShellUnavailable", err)
	}
	presented := false
	if _, err := a.NewWindow(app.WindowConfig{
		Title: "cage", Width: fakeSurfaceW, Height: fakeSurfaceH, Root: widget.NewDrawingArea(fakeSurfaceW, fakeSurfaceH),
		OnPresented: func() {
			presented = true
			a.Quit()
		},
	}); err != nil {
		t.Fatalf("NewWindow under cage: %v", err)
	}
	watchdog := time.AfterFunc(30*time.Second, func() { a.Invoke(a.Quit) })
	defer watchdog.Stop()
	if err := a.Run(); err != nil && !errors.Is(err, app.ErrClosed) {
		t.Fatalf("run: %v", err)
	}
	if !presented {
		t.Fatal("the toplevel was never presented")
	}
	if err := os.WriteFile(done, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}
