package widget

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type hookCase struct {
	name string
	run  func(t *testing.T)
}

func hookOf[F any](name string, slot *atomic.Pointer[F], set func(F) func(), first, second, none F) hookCase {
	return hookCase{name, func(t *testing.T) {
		t.Cleanup(func() { slot.Store(nil) })
		if slot.Load() != nil {
			t.Fatal("a hook is installed before the test installs one")
		}
		remove := set(first)
		if slot.Load() == nil {
			t.Fatal("installing left no hook")
		}
		remove()
		remove()
		if slot.Load() != nil {
			t.Fatal("removing the hook left it installed")
		}

		removeFirst := set(first)
		removeSecond := set(second)
		later := slot.Load()
		removeFirst()
		if slot.Load() != later {
			t.Fatal("removing an earlier hook took out the one installed after it")
		}
		removeSecond()
		if slot.Load() != nil {
			t.Fatal("removing the later hook left it installed")
		}

		set(first)
		set(none)()
		if slot.Load() != nil {
			t.Fatal("installing none left a hook")
		}
	}}
}

func TestAHookIsRemovedByWhoInstalledItAndSurvivesAnEarlierRemoval(t *testing.T) {
	resetWatches(t)
	watch := func() StylesheetWatcher {
		return func([]string, func()) (func(), error) { return func() {}, nil }
	}
	for _, c := range []hookCase{
		hookOf("invoker", &invoker, SetInvoker, func(func()) {}, func(func()) {}, nil),
		hookOf("stylesheet watcher", &watcherHook, SetStylesheetWatcher, watch(), watch(), nil),
		hookOf("error bell", &errorBell, SetErrorBell, func(Widget) {}, func(Widget) {}, nil),
		hookOf("surface fade", &surfaceFade, SetSurfaceFade, func(Widget, float64) bool { return true }, func(Widget, float64) bool { return false }, nil),
		hookOf("GPU host", &gpuHost, SetGPUHost, func(*GPUArea) (GPUSurface, error) { return nil, nil }, func(*GPUArea) (GPUSurface, error) { return nil, os.ErrInvalid }, nil),
		hookOf("video host", &videoHost, SetVideoHost,
			func(*Video, VideoFormat, func()) (VideoSurface, error) { return nil, nil },
			func(*Video, VideoFormat, func()) (VideoSurface, error) { return nil, os.ErrInvalid }, nil),
		hookOf("texture host", &textureHost, SetTextureHost,
			func(*Video, int, int, func(), func()) (TextureSurface, error) { return nil, nil },
			func(*Video, int, int, func(), func()) (TextureSurface, error) { return nil, os.ErrInvalid }, nil),
		hookOf("surface origin resolver", &originResolver, SetSurfaceOriginResolver, func(Widget) (Point, bool) { return Point{}, true }, func(Widget) (Point, bool) { return Point{}, false }, nil),
		hookOf("removed hook", &removedHook, SetRemovedHook, func(Widget) {}, func(Widget) {}, nil),
	} {
		t.Run(c.name, c.run)
	}
}

func TestRemovingTheStylesheetWatcherEndsItsWatchAndNoOther(t *testing.T) {
	resetWatches(t)
	type watcher struct{ armed, stopped int }
	install := func(w *watcher) (remove func()) {
		return SetStylesheetWatcher(func([]string, func()) (func(), error) {
			w.armed++
			return func() { w.stopped++ }, nil
		})
	}
	path := filepath.Join(t.TempDir(), "a.css")
	if err := os.WriteFile(path, []byte("label { color: red; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	var first, second watcher
	removeFirst := install(&first)
	t.Cleanup(func() { SetStylesheetWatcher(nil) })
	sheet, err := AddStylesheetFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sheet.Remove)
	if first != (watcher{armed: 1}) {
		t.Fatalf("with a file loaded the first watcher is %+v, want its watch armed", first)
	}
	removeSecond := install(&second)
	if first != (watcher{armed: 1, stopped: 1}) || second != (watcher{armed: 1}) {
		t.Fatalf("after a second install: first %+v, second %+v; want the watch moved to the second", first, second)
	}
	removeFirst()
	if second != (watcher{armed: 1}) {
		t.Fatalf("removing the first watcher left the second %+v, want its watch running", second)
	}
	removeSecond()
	if second != (watcher{armed: 1, stopped: 1}) || first != (watcher{armed: 1, stopped: 1}) {
		t.Errorf("removing the second watcher: first %+v, second %+v; want its watch ended, once", first, second)
	}
}
