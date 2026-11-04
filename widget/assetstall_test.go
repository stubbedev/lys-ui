package widget

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/assetfile"
)

func TestAStuckReadAndAPipeHoldUpNoOtherIcon(t *testing.T) {
	f := newFakeInvoker(t)
	iconTree(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.svg")
	if err := os.WriteFile(good, []byte(testGreenSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	pipe := filepath.Join(dir, "pipe.svg")
	fifoPath(t, pipe)

	stuck := make(chan struct{})
	t.Cleanup(func() {
		close(stuck)
		onTime(t, "the stuck load's release", WaitAssetLoads)
	})
	loadAsset("dead mount", func() { <-stuck }, func() {})

	pipeFile := NewFileIcon(pipe, 16)
	pipeTheme := NewThemeIconFallback(pipe, 16)
	others := map[string]*Icon{
		"file":  NewFileIcon(good, 16),
		"svg":   NewSVGIcon([]byte(testGreenSVG), 16),
		"theme": NewThemeIcon("flag", 16),
	}
	all := []*Icon{pipeFile, pipeTheme, others["file"], others["svg"], others["theme"]}
	for _, w := range all {
		onTime(t, "a paint", func() { paintIcon(w, 16, 1, 1) })
	}

	completed := 0
	deadline := time.After(10 * time.Second)
	for completed < len(all) {
		select {
		case <-f.got:
			completed++
		case <-deadline:
			t.Fatalf("%d of %d decodes completed while the stuck read held", completed, len(all))
		}
	}
	f.mu.Lock()
	fns := f.fns
	f.fns = nil
	f.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
	if completed != len(all) {
		t.Errorf("%d decodes completed while the stuck read held, want %d", completed, len(all))
	}
	for name, w := range others {
		data, _ := paintIcon(w, 16, 1, 1)
		if got := centerPx(data, 16); got != render.RGB(0, 0xff, 0) {
			t.Errorf("%s: center %v, want green", name, got)
		}
	}
	paintIcon(pipeFile, 16, 1, 1)
	if err := pipeFile.Err(); !errors.Is(err, assetfile.ErrNotRegular) {
		t.Errorf("pipe file icon error %v, want it refused as no regular file", err)
	}
	data, _ := paintIcon(pipeTheme, 16, 1, 1)
	if !errors.Is(pipeTheme.Err(), assetfile.ErrNotRegular) {
		t.Errorf("pipe theme icon error %v, want it refused", pipeTheme.Err())
	}
	if got := centerPx(data, 16); got != render.RGB(0, 0, 0) {
		t.Errorf("pipe theme icon center %v, want nothing painted", got)
	}
}

func TestAPipeIsRefusedWhereverAStylesheetNamesOne(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, "pipe")
	fifoPath(t, pipe)
	onTime(t, "a css image", func() {
		if _, err := decodeCSSImage(pipe); !errors.Is(err, assetfile.ErrNotRegular) {
			t.Errorf("css image: %v", err)
		}
	})
	onTime(t, "a stylesheet file", func() {
		if _, err := AddStylesheetFile(pipe, StylePriorityApplication); !errors.Is(err, assetfile.ErrNotRegular) {
			t.Errorf("stylesheet file: %v", err)
		}
	})
	onTime(t, "an image file", func() {
		if _, err := fetchSource(imageSource{kind: imageFile, path: pipe}); !errors.Is(err, assetfile.ErrNotRegular) {
			t.Errorf("image file: %v", err)
		}
	})
}
