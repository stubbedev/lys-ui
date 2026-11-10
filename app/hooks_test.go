package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/widget"
)

type widgetHooks struct {
	invoker, stylesheetWatcher, gpuHost, videoHost, textureHost bool
}

var allWidgetHooks = widgetHooks{true, true, true, true, true}

func (s *stopSet) added() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

func hooksOf(t *testing.T, a *Application) widgetHooks {
	t.Helper()
	t.Cleanup(animclock.Reset)
	var got widgetHooks
	box := render.Rect{W: 64, H: 48}

	icon := widget.NewSVGIcon([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="#e04020"/></svg>`), 16)
	icon.Arrange(render.Rect{W: 16, H: 16})
	pixels := make([]byte, render.Stride(16)*16)
	icon.Paint(render.New(pixels, render.Stride(16), 16, 16))
	widget.WaitAssetLoads()
	got.invoker = slices.Max(pixels) == 0

	path := filepath.Join(t.TempDir(), "hooks.css")
	if err := os.WriteFile(path, []byte("label { color: red; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	watched := a.watchers.added()
	sheet, err := widget.AddStylesheetFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	got.stylesheetWatcher = a.watchers.added() == watched+1
	sheet.Remove()

	area := widget.NewGPUArea()
	area.Arrange(box)
	got.gpuHost = area.Err() != nil

	yuv, err := media.NewY4M(strings.NewReader("YUV4MPEG2 W2 H2 F25:1 C420jpeg\nFRAME\n\x10\x10\x10\x10\x80\x80"))
	if err != nil {
		t.Fatal(err)
	}
	frames := media.NewStream(yuv, "hooks")
	t.Cleanup(frames.Close)
	painted := widget.NewVideo(testFace(t), 13, frames)
	painted.Arrange(box)
	got.videoHost = painted.OffloadErr() == nil

	textures := media.NewStream(UndecodedTextures{}, "hooks")
	t.Cleanup(textures.Close)
	widget.NewVideo(testFace(t), 13, textures).Arrange(box)
	got.textureHost = textures.Err() == nil
	return got
}

func TestCloseTakesOutTheWidgetHooksOfItsApplicationAndNoOther(t *testing.T) {
	first := NewTestApplication(t, &wlsession.Session{})
	second := NewTestApplication(t, &wlsession.Session{})
	if got := hooksOf(t, second); got != allWidgetHooks {
		t.Fatalf("with two applications the widgets reach the second through %+v, want every hook", got)
	}

	first.Close()
	if got := hooksOf(t, second); got != allWidgetHooks {
		t.Errorf("closing the first application left the second's hooks as %+v, want every one in place", got)
	}

	second.Close()
	if got := hooksOf(t, second); got != (widgetHooks{}) {
		t.Errorf("closing the second application left the hooks %+v, want none", got)
	}
}
