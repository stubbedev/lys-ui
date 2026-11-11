// The reconnect-with-rebuild test (#117): kill the compositor under a
// client that opted into reconnecting, boot it again on the same
// socket, and assert the client lived through it - the policy traced
// once with a rebuild planned, the window rebuilt on the new session,
// and keyboard input reaching it there. It leaves the restarted
// compositor serving the rest of the suite.
package headlesstest

import (
	"errors"
	"io/fs"
	"os"
	"regexp"
	"slices"
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
	c, err := testEnv.StartClientEnv(bin, "client-"+t.Name(), "demo,wire,frame", []string{"LYS_UI_DEMO_RECONNECT=1"})
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
	// A committed frame is the session whole: its socket, its shm arena
	// and the pool made of it.
	if err := w.WaitEver("frame", frameCommitted, 10*time.Second); err != nil {
		t.Fatalf("the window never drew: %v", err)
	}
	before := restingDescriptors(t, c.cmd.Process.Pid)

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
	// buffers closed, the new session's replaced them one for one. The
	// new session is whole once the rebuilt window drew; what it opens
	// on the way there, a compose table it reads, a descriptor on its
	// way to the compositor, closes by itself, and a leak never does.
	if _, err := w.Wait("frame", frameCommitted, 10*time.Second); err != nil {
		t.Fatalf("the rebuilt window never drew: %v", err)
	}
	if leaked := descriptorsLeftBeyond(t, c.cmd.Process.Pid, before); len(leaked) > 0 {
		t.Errorf("the client holds descriptors after the rebuild that it did not before the kill, the old session leaked: %q\nbefore the kill: %q", leaked, before)
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

// frameCommitted is the client's trace of a frame it committed.
const frameCommitted = "frame committed"

// descriptorSettle is how long a client has to close what it opened on
// the way to a session at rest, and restingReadings how many readings
// in a row must agree for its descriptors to count as at rest.
const (
	descriptorSettle = 10 * time.Second
	restingReadings  = 4
)

var numberedInode = regexp.MustCompile(`^(socket|pipe):\[\d+\]$`)

// descriptors is what a process's open file descriptors refer to, in
// order: the path each /proc/<pid>/fd entry links to, a socket and a
// pipe without their inode numbers, so that two readings compare by
// what is open rather than by which descriptor numbers hold it.
func descriptors(t *testing.T, pid int) []string {
	t.Helper()
	dir := "/proc/" + strconv.Itoa(pid) + "/fd"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	held := make([]string, 0, len(entries))
	for _, entry := range entries {
		target, err := os.Readlink(dir + "/" + entry.Name())
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, numberedInode.ReplaceAllString(target, "$1"))
	}
	slices.Sort(held)
	return held
}

// beyond lists what held has more of than baseline, both in order.
func beyond(held, baseline []string) []string {
	var extra []string
	for _, target := range held {
		for len(baseline) > 0 && baseline[0] < target {
			baseline = baseline[1:]
		}
		if len(baseline) > 0 && baseline[0] == target {
			baseline = baseline[1:]
			continue
		}
		extra = append(extra, target)
	}
	return extra
}

// restingDescriptors reads a process's descriptors until
// restingReadings readings in a row agree: one taken while the process
// reads a file or hands a descriptor to its compositor is not what it
// holds.
func restingDescriptors(t *testing.T, pid int) []string {
	t.Helper()
	deadline := time.Now().Add(descriptorSettle)
	held, agreed := descriptors(t, pid), 1
	for agreed < restingReadings {
		if time.Now().After(deadline) {
			t.Fatalf("the descriptors of process %d never came to rest; the last reading: %q", pid, held)
		}
		time.Sleep(pollInterval)
		again := descriptors(t, pid)
		if slices.Equal(again, held) {
			agreed++
			continue
		}
		held, agreed = again, 1
	}
	return held
}

// descriptorsLeftBeyond waits for a process to hold nothing beyond
// baseline and returns what it still holds beyond it when the wait
// ends: nothing, unless it leaked.
func descriptorsLeftBeyond(t *testing.T, pid int, baseline []string) []string {
	t.Helper()
	deadline := time.Now().Add(descriptorSettle)
	for {
		extra := beyond(descriptors(t, pid), baseline)
		if len(extra) == 0 || time.Now().After(deadline) {
			return extra
		}
		time.Sleep(pollInterval)
	}
}
