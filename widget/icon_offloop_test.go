package widget

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/icons"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/testbuild"
)

func settleAssets(t *testing.T, f *fakeInvoker) {
	t.Helper()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		WaitAssetLoads()
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("the asset queue never ran out of work")
	}
	f.mu.Lock()
	fns := f.fns
	f.fns = nil
	f.mu.Unlock()
	for range fns {
		<-f.got
	}
	for _, fn := range fns {
		fn()
	}
}

func onTime(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s blocked on the disk", what)
	}
}

func fifoPath(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

func holdRead(t *testing.T, path string) (feed func(data string)) {
	t.Helper()
	read := readAssetFile
	held := make(chan string)
	fed := false
	readAssetFile = func(name string, limit int64) ([]byte, error) {
		if name != path {
			return read(name, limit)
		}
		return []byte(<-held), nil
	}
	t.Cleanup(func() {
		if !fed {
			close(held)
		}
		WaitAssetLoads()
		readAssetFile = read
	})
	return func(data string) {
		fed = true
		onTime(t, "the off-loop read never started, so feeding it", func() { held <- data })
	}
}

func TestThemeIconPaintNeverWaitsForTheIndex(t *testing.T) {
	f := newFakeInvoker(t)
	root := t.TempDir()
	theme := filepath.Join(root, "wfifo")
	if err := os.MkdirAll(filepath.Join(theme, "16x16/apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(theme, "16x16/apps/flag.svg"), []byte(testGreenSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(theme, "index.theme"), []byte("[Icon Theme]\nName=W\nDirectories=16x16/apps\n\n[16x16/apps]\nSize=16\nType=Fixed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	def := icons.Default()
	def.SetSearchPaths([]string{root})
	def.SetTheme("wfifo")
	t.Cleanup(func() {
		def.SetSearchPaths(nil)
		def.SetTheme("hicolor")
	})

	w := NewThemeIcon("flag", 16)
	var data []byte
	onTime(t, "a paint", func() { data, _ = paintIcon(w, 16, 1, 1) })
	if got := centerPx(data, 16); got != render.RGB(0, 0, 0) {
		t.Errorf("first paint center = %v, want blank", got)
	}
	onTime(t, "the generation", func() { _ = def.Generation() })
	CollectDamage(w)

	settleAssets(t, f)
	if _, any := CollectDamage(w); !any {
		t.Fatal("the decode's answer repainted nothing")
	}
	data, _ = paintIcon(w, 16, 1, 1)
	if got := centerPx(data, 16); got != render.RGB(0, 0xff, 0) {
		t.Errorf("settled center = %v, want green", got)
	}
	if _, any := CollectDamage(w); any {
		t.Error("a settled paint owes another")
	}
}

func TestFileIconPaintNeverReads(t *testing.T) {
	f := newFakeInvoker(t)
	path := filepath.Join(t.TempDir(), "held.svg")
	feed := holdRead(t, path)
	w := NewFileIcon(path, 16)
	onTime(t, "a paint", func() { paintIcon(w, 16, 1, 1) })
	onTime(t, "a second paint", func() { paintIcon(w, 16, 1, 1) })
	feed(testGreenSVG)
	settleAssets(t, f)
	data, _ := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, 16); got != render.RGB(0, 0xff, 0) {
		t.Errorf("settled center = %v, want green", got)
	}
}

func TestFileIconFailureSurfacesAfterItsDecode(t *testing.T) {
	f := newFakeInvoker(t)
	w := NewFileIcon(filepath.Join(t.TempDir(), "gone.png"), 16)
	paintIcon(w, 16, 1, 1)
	if w.Err() != nil {
		t.Errorf("failure before the decode: %v", w.Err())
	}
	settleAssets(t, f)
	paintIcon(w, 16, 1, 1)
	if w.Err() == nil {
		t.Error("no failure after the decode")
	}
}

func TestPreloadPatternPaintsFirstTimeWithoutAsking(t *testing.T) {
	newFakeInvoker(t)
	iconTree(t)
	for _, name := range []string{"flag", "flag-extra-name"} {
		if ThemeIconPreloaded(name, 16, 1, 1) {
			t.Fatalf("%s: nothing has loaded it", name)
		}
		PreloadThemeIcon(name, 16, 1, 1)
		if !ThemeIconPreloaded(name, 16, 1, 1) {
			t.Fatalf("%s: preload did not take", name)
		}
		w := NewThemeIconFallback(name, 16)
		data, _ := paintIcon(w, 16, 1, 1)
		if got := centerPx(data, 16); got != render.RGB(0, 0xff, 0) {
			t.Errorf("%s: first paint center = %v, want green", name, got)
		}
		assets.mu.Lock()
		queued := len(assets.jobs)
		assets.mu.Unlock()
		if queued != 0 {
			t.Errorf("%s: the paint queued %d decodes", name, queued)
		}
	}
}

func TestIconKeepsItsRasterUntilTheNextIsDecoded(t *testing.T) {
	f := newFakeInvoker(t)
	iconTree(t)
	w := NewThemeIcon("flag", 16)
	paintIcon(w, 16, 1, 1)
	settleAssets(t, f)
	paintIcon(w, 16, 1, 1)
	icons.Default().SetTheme("wblue")
	data, _ := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, 16); got != render.RGB(0, 0xff, 0) {
		t.Errorf("mid-switch center = %v, want the old green, not a blank frame", got)
	}
	settleAssets(t, f)
	data, _ = paintIcon(w, 16, 1, 1)
	if got := centerPx(data, 16); got != render.RGB(0, 0, 0xff) {
		t.Errorf("settled center = %v, want blue", got)
	}
}

func collectingInvoker(t *testing.T) (run func()) {
	t.Helper()
	var mu sync.Mutex
	var fns []func()
	SetInvoker(func(fn func()) {
		mu.Lock()
		defer mu.Unlock()
		fns = append(fns, fn)
	})
	t.Cleanup(func() { SetInvoker(nil) })
	return func() {
		mu.Lock()
		defer mu.Unlock()
		for _, fn := range fns {
			fn()
		}
		fns = nil
	}
}

func TestAssetQueueBoundsAndAnswersWhatItDrops(t *testing.T) {
	run := collectingInvoker(t)
	hold := make(chan struct{})
	answered := map[int]bool{}
	loadAsset(-1, func() { <-hold }, func() { answered[-1] = true })
	for i := range maxAssetQueue + 3 {
		loadAsset(i, func() {}, func() { answered[i] = true })
	}
	assets.mu.Lock()
	queued := len(assets.queue)
	assets.mu.Unlock()
	if queued > maxAssetQueue {
		t.Errorf("%d queued, bound %d", queued, maxAssetQueue)
	}
	close(hold)
	onTime(t, "the queue", WaitAssetLoads)
	run()
	for i := -1; i < maxAssetQueue+3; i++ {
		if !answered[i] {
			t.Fatalf("job %d never answered: its asker would stay blank", i)
		}
	}
}

func TestStopDropsWhatHasNotStarted(t *testing.T) {
	collectingInvoker(t)
	hold := make(chan struct{})
	var ran atomic.Bool
	for i := range maxAssetWorkers {
		loadAsset(i, func() { <-hold }, func() {})
	}
	loadAsset("queued", func() { ran.Store(true) }, func() {})
	StopAssetLoads()
	close(hold)
	onTime(t, "the queue", WaitAssetLoads)
	if ran.Load() {
		t.Error("a decode ran after the stop")
	}
}

func TestRetintReadsNothingAndAllocatesLittle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glyph.svg")
	if err := os.WriteFile(path, []byte(testSymbolicSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	iconTree(t)
	for name, w := range map[string]*Icon{
		"file":  NewFileIcon(path, 16),
		"svg":   NewSVGIcon([]byte(testSymbolicSVG), 16),
		"theme": NewThemeIcon("face-symbolic", 16),
	} {
		t.Run(name, func(t *testing.T) {
			w.SetTint(render.RGB(1, 2, 3))
			data, _ := paintIcon(w, 16, 1, 1)
			if got := centerPx(data, 16); got != render.RGB(1, 2, 3) {
				t.Fatalf("center = %v", got)
			}
			if name == "file" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			pix := make([]byte, render.Stride(16)*16)
			cv := render.New(pix, render.Stride(16), 16, 16)
			step := uint32(0)
			retint := func() {
				step++
				w.SetTint(render.Color(0xff000000 | step))
				w.Paint(cv)
			}
			retint()
			want := render.Color(0xff000000 | (step + 1))
			retint()
			if w.err != nil {
				t.Fatalf("retint failed: %v", w.err)
			}
			if got := centerPx(pix, 16); got != want {
				t.Errorf("center = %v, want %v", got, want)
			}
			testbuild.SkipAllocBudget(t)
			if allocs := testing.AllocsPerRun(100, retint); allocs > 2 {
				t.Errorf("a retint step allocates %v, budget 2", allocs)
			}
		})
	}
}
