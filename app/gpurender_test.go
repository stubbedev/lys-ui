package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// LYS_UI_RENDERER picks how windows draw: cpu never tries the GPU,
// auto falls back to the CPU where the GPU cannot draw for the
// compositor, gpu makes that an error, and anything else is one.
func TestRendererModes(t *testing.T) {
	for _, tc := range []struct {
		mode, err string
	}{
		{mode: "cpu"},
		{mode: ""},
		{mode: "auto"},
		{mode: "gpu", err: "LYS_UI_RENDERER=gpu: the compositor has no linux-dmabuf"},
		{mode: "vulkan", err: `LYS_UI_RENDERER="vulkan": want cpu, gpu or auto`},
	} {
		t.Setenv(rendererEnv, tc.mode)
		a := &Application{sess: &wlsession.Session{}}
		r, err := a.renderer()
		switch {
		case r != nil:
			t.Errorf("%q: a GPU renderer on a compositor without linux-dmabuf", tc.mode)
		case tc.err == "" && err != nil:
			t.Errorf("%q: %v, want the CPU", tc.mode, err)
		case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
			t.Errorf("%q: error %v, want %q", tc.mode, err, tc.err)
		}
		if r2, err2 := a.renderer(); r2 != r || !errors.Is(err2, err) {
			t.Errorf("%q: a second call gave %v, %v", tc.mode, r2, err2)
		}
	}
}
