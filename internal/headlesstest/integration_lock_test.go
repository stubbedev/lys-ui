package headlesstest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

// lockTimeout bounds each step of the session-lock scenario.
const lockTimeout = 8 * time.Second

// lockRun is one lock client: its application loop runs on its own
// goroutine, and the lock callbacks surface as channels so the test
// steps through the compositor's verdicts in order.
type lockRun struct {
	app      *app.Application
	locked   chan struct{}
	finished chan struct{}
	closed   chan *app.Output
	done     chan struct{} // closed when Run returned
	// runErr is what Run returned, set before done closes.
	runErr error
}

// startLock connects, requests the session lock, and starts the loop.
// LockSession runs before Run, on the test goroutine: a loop with no
// window and no lock would return before pumping an Invoke.
func startLock(t *testing.T) *lockRun {
	t.Helper()
	sess, err := app.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	r := &lockRun{
		app:      app.NewApplication(sess),
		locked:   make(chan struct{}, 1),
		finished: make(chan struct{}, 1),
		closed:   make(chan *app.Output, 16),
		done:     make(chan struct{}),
	}
	if _, err := r.app.LockSession(app.SessionLockConfig{
		Surface: func(out *app.Output) app.LockSurface {
			return app.LockSurface{
				Root:       widget.NewBox(widget.Row, 0, 0),
				Background: render.RGBA(0, 0, 0, 255),
				OnClosed:   func() { r.closed <- out },
			}
		},
		OnLocked:   func() { r.locked <- struct{}{} },
		OnFinished: func() { r.finished <- struct{}{} },
	}); err != nil {
		sess.Close()
		t.Fatalf("LockSession: %v", err)
	}
	go func() { r.runErr = r.app.Run(); close(r.done) }()
	t.Cleanup(func() {
		r.app.Invoke(r.app.Quit)
		select {
		case <-r.done:
		case <-time.After(lockTimeout):
			t.Error("application loop did not stop")
		}
		sess.Close()
	})
	return r
}

// onLoop runs fn on the loop goroutine and waits for it; false when
// the loop is gone (it ended with no window and no lock left).
func (r *lockRun) onLoop(fn func()) bool {
	done := make(chan struct{})
	r.app.Invoke(func() { fn(); close(done) })
	select {
	case <-done:
		return true
	case <-r.done:
		return false
	case <-time.After(lockTimeout):
		return false
	}
}

func waitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(lockTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// TestSessionLockLifecycle locks the headless session for real: the
// compositor confirms the lock, a second client's lock is refused
// while it holds, a hotplugged output gets a surface and an unplugged
// one loses it (sway), and Unlock releases the session - and ends the
// loop, which has nothing left to run.
func TestSessionLockLifecycle(t *testing.T) {
	requireEnv(t)
	holder := startLock(t)
	unlocked := false
	unlock := func() {
		if unlocked {
			return
		}
		unlocked = true
		ran := holder.onLoop(func() {
			l := holder.app.SessionLock()
			if l == nil {
				return
			}
			if err := l.Unlock(); err != nil {
				t.Errorf("Unlock: %v", err)
			}
			if holder.app.SessionLock() != nil {
				t.Error("the lock is still recorded as held after Unlock")
			}
			if err := l.Unlock(); !errors.Is(err, app.ErrLockEnded) {
				t.Errorf("second Unlock = %v, want ErrLockEnded", err)
			}
		})
		// A lock left behind breaks every later test on the shared
		// compositor (no keyboard focus while locked): an unlock that
		// never ran is a failure here, not a silent pass.
		if !ran {
			t.Error("the unlock never ran: the holder's loop was gone, the session may stay locked")
		}
	}
	// Whatever fails below, never leave the shared compositor locked.
	t.Cleanup(unlock)

	waitSignal(t, holder.locked, "locked")
	holder.onLoop(func() {
		l := holder.app.SessionLock()
		if l == nil || !l.Locked() {
			t.Error("the handle does not report the confirmed lock")
			return
		}
		if len(l.Outputs()) == 0 {
			t.Error("locked with no output covered")
		}
		if err := l.Cancel(); !errors.Is(err, app.ErrLocked) {
			t.Errorf("Cancel on a locked session = %v, want ErrLocked", err)
		}
	})

	t.Run("a second lock is refused while one holds", func(t *testing.T) {
		// The rival runs in its own process, as a rival lock client
		// does.
		out := runLockHelper(t, "refused")
		if !strings.Contains(out, "rival: finished") {
			t.Fatalf("the rival lock was not refused:\n%s", out)
		}
		t.Logf("rival helper:\n%s", out)
	})

	if testEnv.Compositor().Name() == "sway" {
		t.Run("hotplugged outputs are covered and uncovered", func(t *testing.T) {
			var before int
			holder.onLoop(func() { before = len(holder.app.SessionLock().Outputs()) })
			if err := swayIPCCommand(testEnv.Dir, "create_output"); err != nil {
				t.Fatal(err)
			}
			var added *app.Output
			var name string
			deadline := time.Now().Add(lockTimeout)
			for name == "" && time.Now().Before(deadline) {
				holder.onLoop(func() {
					outs := holder.app.SessionLock().Outputs()
					if len(outs) > before {
						added = outs[len(outs)-1]
						name = added.Name // lands via xdg-output after the global
					}
				})
				time.Sleep(50 * time.Millisecond)
			}
			if name == "" {
				t.Fatal("the hotplugged output never got a (named) lock surface")
			}
			if err := swayIPCCommand(testEnv.Dir, "output "+name+" unplug"); err != nil {
				t.Fatal(err)
			}
			select {
			case out := <-holder.closed:
				if out != added {
					t.Errorf("unplug closed the lock surface of %q, want %q", out.Name, name)
				}
			case <-time.After(lockTimeout):
				t.Fatal("the unplugged output's lock surface never closed")
			}
			holder.onLoop(func() {
				if !holder.app.SessionLock().Locked() {
					t.Error("unplugging an output ended the lock")
				}
			})
		})
	}

	unlock()
	select {
	case <-holder.finished:
		t.Error("a client unlock reported finished")
	default:
	}
	select {
	case <-holder.done:
	case <-time.After(lockTimeout):
		t.Error("the loop outlived its unlocked lock with no window left")
	}

	// The compositor's side: a session it really unlocked grants the
	// next lock (one it still holds refuses it). A lock left behind
	// would take keyboard focus from every later test on the shared
	// compositor, so this is checked, not assumed.
	if out := runLockHelper(t, "granted"); !strings.Contains(out, "probe: locked") {
		t.Errorf("the session was still locked after Unlock - a fresh lock was refused:\n%s", out)
	}
}

// runLockHelper runs TestSessionLockRivalHelper in a child process in
// mode ("refused": a rival expecting the compositor to refuse it;
// "granted": a probe expecting the lock, then unlocking) and returns
// its output. Its own process: the wayland binding keys proxy user
// data process-wide, so one process holds one connection.
func runLockHelper(t *testing.T, mode string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*lockTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionLockRivalHelper$", "-test.v")
	cmd.Env = append(privateEnv("LYS_TEST_HEADLESS=", "WAYLAND_DISPLAY="+testEnv.Display, "XDG_RUNTIME_DIR="+testEnv.Dir), rivalEnv+"="+mode,
		// The helper's lock lifecycle, when the suite is built with the
		// lysdebug tag (a no-op otherwise).
		"LYS_DEBUG=shell,wire")
	out, err := cmd.CombinedOutput()
	t.Logf("lock helper (%s):\n%s", mode, out)
	if err != nil {
		t.Fatalf("lock helper (%s): %v\n%s", mode, err, out)
	}
	return string(out)
}

// rivalEnv switches TestSessionLockRivalHelper on in a child process.
const rivalEnv = "LYS_UI_LOCK_RIVAL"

// TestSessionLockRivalHelper is TestSessionLockLifecycle's child
// lock client (runLockHelper): as the rival it requests a lock while
// another holds one and reports the refusal; as the probe it takes the
// lock a released session grants and unlocks again. Either way its
// loop must end on its own - no lock and no window are left.
func TestSessionLockRivalHelper(t *testing.T) {
	mode := os.Getenv(rivalEnv)
	if mode == "" {
		t.Skip("helper process for TestSessionLockLifecycle")
	}
	r := startLock(t)
	if mode == "granted" {
		select {
		case <-r.locked:
			fmt.Println("probe: locked")
		case <-r.finished:
			t.Fatal("probe: the compositor refused the lock - the session is still locked")
		case <-time.After(lockTimeout):
			t.Fatal("probe: no verdict")
		}
		var unlockErr error
		if !r.onLoop(func() { unlockErr = r.app.SessionLock().Unlock() }) {
			t.Fatal("probe: the unlock never ran")
		}
		fmt.Println("probe: unlock returned", unlockErr)
		select {
		case <-r.done:
			fmt.Println("probe: loop ended:", r.runErr)
		case <-time.After(lockTimeout):
			t.Error("probe: the loop outlived its unlocked lock")
		}
		return
	}
	select {
	case <-r.finished:
		fmt.Println("rival: finished")
	case <-r.locked:
		t.Fatal("rival: the compositor granted a second session lock")
	case <-time.After(lockTimeout):
		t.Fatal("rival: no verdict")
	}
	select {
	case <-r.done:
	case <-time.After(lockTimeout):
		t.Error("rival: a refused lock kept its loop alive")
	}
}
