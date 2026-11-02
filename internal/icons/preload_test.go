package icons

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO holds the preload inside its read for as long as the test wants:
// the read must not hold the lock, and a theme switch during it must keep
// the stale raster out.
func TestPreloadReadsOutsideTheLockAndDropsAStaleDecode(t *testing.T) {
	c := newTestCache(t, "hicolor")
	path := filepath.Join(t.TempDir(), "held.svg")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Preload(path, 16, 120)
	}()
	// Opening for write returns once the preload has opened for read.
	w, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	if !c.mu.TryLock() {
		t.Fatal("the lock is held across the read: a painting UI loop would wait")
	}
	c.mu.Unlock()
	if c.Cached(path, 16, 120) {
		t.Error("cached before its decode finished")
	}
	c.InvalidateTheme()
	if _, err := w.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"/>`); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the preload never finished")
	}
	if c.Cached(path, 16, 120) {
		t.Error("a raster decoded across a theme switch must not be kept")
	}
}

func TestCachedNeverWaits(t *testing.T) {
	c := newTestCache(t, "hicolor")
	path := filepath.Join(t.TempDir(), "a.svg")
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	c.Preload(path, 16, 120)
	if !c.Cached(path, 16, 120) {
		t.Fatal("not cached after its preload")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Cached(path, 16, 120) {
		t.Error("with the lock held elsewhere the answer is no")
	}
}
