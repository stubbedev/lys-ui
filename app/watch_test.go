package app

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/waittest"
)

// loopApp is a testApp with a fake loop goroutine pumping each wake,
// as the parked loop would; stop ends it the way Run's exit does.
func loopApp(t *testing.T) *Application {
	t.Helper()
	kick := make(chan struct{}, 64)
	a := testApp(kick)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-kick:
				a.pump(time.Now())
			case <-done:
				return
			}
		}
	}()
	t.Cleanup(func() {
		a.queues.shutdown()
		a.watchers.shutdown()
		a.endLoop()
		close(done)
	})
	return a
}

// quiet asserts cond stays false for a while.
func quiet(t *testing.T, what string, cond func() bool) {
	t.Helper()
	time.Sleep(60 * time.Millisecond)
	if cond() {
		t.Fatal(what)
	}
}

func TestWatchFDRunsOnTheLoopOncePerReadiness(t *testing.T) {
	a := loopApp(t)
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(fds[0]); _ = unix.Close(fds[1]) }()
	var calls atomic.Int32
	stop, err := a.WatchFD(fds[0], unix.POLLIN, func(revents int16) {
		if revents&unix.POLLIN == 0 {
			t.Errorf("revents = %#x, want POLLIN", revents)
		}
		buf := make([]byte, 64)
		_, _ = unix.Read(fds[0], buf)
		calls.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	quiet(t, "an idle fd called back", func() bool { return calls.Load() != 0 })
	_, _ = unix.Write(fds[1], []byte("x"))
	waittest.Until(t, func() bool { return calls.Load() == 1 }, "the first readiness")
	quiet(t, "a drained fd called back again", func() bool { return calls.Load() != 1 })
	_, _ = unix.Write(fds[1], []byte("y"))
	waittest.Until(t, func() bool { return calls.Load() == 2 }, "the second readiness")

	stop()
	stop() // twice is harmless
	_, _ = unix.Write(fds[1], []byte("z"))
	quiet(t, "a stopped watch called back", func() bool { return calls.Load() != 2 })
}

func TestWatchFDEndsWithTheLoop(t *testing.T) {
	a := loopApp(t)
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(fds[0]); _ = unix.Close(fds[1]) }()
	var calls atomic.Int32
	if _, err := a.WatchFD(fds[0], unix.POLLIN, func(int16) { calls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	a.queues.shutdown()
	a.watchers.shutdown()
	a.endLoop()
	_, _ = unix.Write(fds[1], []byte("x"))
	quiet(t, "a watch outlived its loop", func() bool { return calls.Load() != 0 })
	if _, err := a.WatchFD(fds[0], unix.POLLIN, func(int16) {}); !errors.Is(err, ErrLoopEnded) {
		t.Errorf("a watch after the loop = %v, want ErrLoopEnded", err)
	}
}

func TestWatchFilesCoalescesAndRecurses(t *testing.T) {
	a := loopApp(t)
	dir := t.TempDir()
	var flat, deep atomic.Int32
	stopFlat, err := a.WatchFiles([]string{dir, filepath.Join(dir, "missing")}, false, func() { flat.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer stopFlat()
	stopDeep, err := a.WatchFiles([]string{dir}, true, func() { deep.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer stopDeep()

	if err := os.WriteFile(filepath.Join(dir, "a.scss"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	waittest.Until(t, func() bool { return flat.Load() >= 1 && deep.Load() >= 1 }, "a top-level change")
	time.Sleep(30 * time.Millisecond)
	flatBefore := flat.Load()

	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	waittest.Until(t, func() bool { return flat.Load() > flatBefore }, "the new directory")
	time.Sleep(30 * time.Millisecond)
	flatBefore, deepBefore := flat.Load(), deep.Load()
	if err := os.WriteFile(filepath.Join(sub, "b.scss"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	waittest.Until(t, func() bool { return deep.Load() > deepBefore }, "a change in a directory created later")
	quiet(t, "a flat watch saw a nested change", func() bool { return flat.Load() != flatBefore })
}

// A readiness already queued for the loop when the watch stops is
// dropped: after stop, fn never runs.
func TestWatchFDStopDropsAQueuedCallback(t *testing.T) {
	a := testApp(nil)
	t.Cleanup(func() { a.queues.shutdown(); a.watchers.shutdown(); a.endLoop() })
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(fds[0]); _ = unix.Close(fds[1]) }()
	var calls atomic.Int32
	stop, err := a.WatchFD(fds[0], unix.POLLIN, func(int16) { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	_, _ = unix.Write(fds[1], []byte("x"))
	waittest.Until(t, func() bool { return a.queues.pending() == 1 }, "the queued callback")
	stop()
	a.pump(time.Now())
	if calls.Load() != 0 {
		t.Error("a callback queued before stop ran after it")
	}
}
