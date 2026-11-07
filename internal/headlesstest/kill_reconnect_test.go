// The reconnect-with-rebuild test (#117): kill the compositor under a
// client that opted into reconnecting, boot it again on the same
// socket, and assert the client lived through it - the policy traced
// once with a rebuild planned, the window rebuilt on the new session,
// and keyboard input reaching it there. It leaves the restarted
// compositor serving the rest of the suite.
package headlesstest

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestHeadlessReconnectRebuildsWindows(t *testing.T) {
	requireEnv(t)
	if !testEnv.CanRestartCompositor() {
		t.Skip("the compositor driver cannot boot the compositor again, which a reconnect needs")
	}
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-states", "lys-ui-states")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClientEnv(bin, "client-"+t.Name(), "demo,wire", []string{"LYS_UI_DEMO_RECONNECT=1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", "state maximized=false", 10*time.Second); err != nil {
		t.Fatalf("the window never configured: %v; %s", err, tailTraces(w, 15))
	}

	fdsBefore := openFDs(t, c.cmd.Process.Pid)

	testEnv.KillCompositor()
	if err := testEnv.RestartCompositor(); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Wait("wire", "app: compositor disconnected", 10*time.Second); err != nil {
		t.Fatalf("no disconnect policy trace: %v; %s", err, tailTraces(w, 15))
	}
	if _, err := w.Wait("wire", "app: reconnected, 1 windows rebuilt", 20*time.Second); err != nil {
		t.Fatalf("the window was not rebuilt: %v; %s", err, tailTraces(w, 20))
	}
	if _, err := w.Wait("demo", "reconnected", 5*time.Second); err != nil {
		t.Fatalf("OnReconnected never ran: %v", err)
	}
	select {
	case <-c.Exited():
		t.Fatalf("the client exited instead of reconnecting: %s", tailTraces(w, 20))
	default:
	}
	// Nothing of the old session leaks: its socket, shm arena, and
	// buffers closed, the new session's replaced them one for one.
	if after := openFDs(t, c.cmd.Process.Pid); after > fdsBefore {
		t.Errorf("open fds %d after the rebuild, %d before the kill: the old session leaked", after, fdsBefore)
	}

	// Keyboard input reaches the rebuilt window on the new session: a
	// fresh seat (the old one died with the compositor) taps p, the
	// handler reads the new window's state through the live session.
	code, _, err := KeyFor('p')
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; ; attempt++ {
		if attempt == 4 {
			t.Fatalf("p never reached the rebuilt window: %s", tailTraces(w, 15))
		}
		in := newInput(t)
		if err := in.Tap(code); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Wait("demo", "polled maximized=false", attemptTimeout); err == nil {
			break
		}
	}
}

// openFDs counts a process's open file descriptors.
func openFDs(t *testing.T, pid int) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/" + strconv.Itoa(pid) + "/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
