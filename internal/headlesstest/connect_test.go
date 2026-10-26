package headlesstest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-ui/app"
)

// A program started with an absolute WAYLAND_DISPLAY, as lys-wm hands
// the programs it starts, connects to that socket whatever
// XDG_RUNTIME_DIR names (#369).
func TestHeadlessConnectsToAnAbsoluteDisplay(t *testing.T) {
	requireEnv(t)
	sock := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("WAYLAND_DISPLAY"))
	t.Setenv("WAYLAND_DISPLAY", sock)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sess, err := app.Connect()
	if err != nil {
		t.Fatalf("connect with WAYLAND_DISPLAY=%s: %v", sock, err)
	}
	sess.Close()
}
