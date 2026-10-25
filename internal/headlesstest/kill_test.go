// The compositor-restart kill test (#42): the headless suite's honest
// disconnect. The showcase client runs against the live private
// session; this test SIGKILLs the compositor mid-run and asserts the
// client's defined story — the disconnect policy fires exactly once
// (one wire trace), the loop unwinds, and the process exits by itself
// with the distinct DisconnectExitCode a supervisor respawns on. No
// panic, no zombie.
//
// This file sorts after integration_test.go on purpose: killing the
// compositor ends the shared session, so the kill must be the suite's
// last act. The env recipe's trap-based teardown still runs afterwards
// and is happy to tear down an already-dead compositor.
package headlesstest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/app"
)

// killTimeout bounds how long the client may take to notice the dead
// socket and unwind after the compositor is SIGKILLed. The read fails
// in milliseconds; the budget is for a loaded CI machine, not for the
// detection itself.
const killTimeout = 30 * time.Second

// killCompositor SIGKILLs every process serving this test env's
// compositor config: on some environments the recorded pid
// daemonizes - a parent that forks the real compositor and exits - so
// by kill time the pid file is stale and the socket holder's only
// reliable trace is the config path in its cmdline. A tree walk from a
// dead parent finds nothing; a cmdline scan finds whatever shape the
// fork took.
func killCompositor() {
	testEnv.KillCompositor()
}

// liveCompositorProcs lists the surviving compositor-tree processes
// (the compositor, its helpers, any leftover holding the display),
// read from procfs cmdlines and parent pids so the failure names its
// suspects and how they relate.
func liveCompositorProcs() string {
	name := testEnv.Compositor().Name()
	var names []string
	forEachProcPID(func(pid int, cmdline string) {
		line := strings.ReplaceAll(cmdline, "\x00", " ")
		if strings.Contains(line, name) {
			stat, _ := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
			fields := strings.Fields(string(stat))
			parent := "?"
			if len(fields) > 3 {
				parent = fields[3]
			}
			names = append(names, fmt.Sprintf("%d(ppid %s)=%s", pid, parent, strings.TrimSpace(line)))
		}
	})
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "; ")
}

// clientSocketPeers reports, for every unix socket the client holds
// open, which other processes share that same socket (its peer end).
// A read parked forever means no EOF arrived, which means some process
// still holds the far end: this names it.
func clientSocketPeers(clientPid int) string {
	descriptors, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", clientPid))
	if err != nil {
		return fmt.Sprintf("client fds: %v", err)
	}
	sockets := map[string]bool{}
	for _, d := range descriptors {
		target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", clientPid, d.Name()))
		if err == nil && strings.HasPrefix(target, "socket:[") {
			sockets[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}
	if len(sockets) == 0 {
		return "client holds no unix sockets"
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Sprintf("procfs: %v", err)
	}
	var holders []string
	for _, e := range entries {
		pid := e.Name()
		if !e.IsDir() || !isDigits(pid) {
			continue
		}
		fds, err := os.ReadDir("/proc/" + pid + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink("/proc/" + pid + "/fd/" + fd.Name())
			if err != nil {
				continue
			}
			for inode := range sockets {
				if target == "socket:["+inode+"]" {
					holders = append(holders, "socket "+inode+" held by pid "+pid)
				}
			}
		}
	}
	if len(holders) == 0 {
		return "no process holds the client's sockets (peer gone, EOF owed)"
	}
	return strings.Join(holders, "; ")
}

// mainGoroutineStack extracts goroutine 1's frames from a runtime
// SIGQUIT dump: the park point of the client's loop.
func mainGoroutineStack(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Sprintf("client log: %v", err)
	}
	text := string(data)
	start := strings.Index(text, "goroutine 1 ")
	if start < 0 {
		return "no goroutine 1 in dump; last 40 lines:\n" + tailFile(logPath, 40)
	}
	rest := text[start:]
	if end := strings.Index(rest, "\ngoroutine "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// TestHeadlessCompositorKillExitsCleanly drives the clean-exit policy
// end to end: real compositor, real client, real death.
func TestHeadlessCompositorKillExitsCleanly(t *testing.T) {
	requireEnv(t)
	c, w, _ := startShowcase(t)

	// The client must be alive right up to the kill; an early exit
	// would make every later assertion vacuous.
	select {
	case <-c.Exited():
		t.Fatal("showcase exited before the compositor was killed")
	case <-time.After(250 * time.Millisecond):
	}

	pidFile := testEnv.PIDPath()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("compositor pid file %s: %v", pidFile, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("bad compositor pid %q: %v", data, err)
	}
	// The compositor is whatever serves this env's config: the recorded
	// pid may be a daemonizing parent that exited long before the kill,
	// with the real socket holder forked off and reparented to init.
	// Kill by the config path every candidate carries in its cmdline.
	killCompositor()
	// What runs after this test (the next package on the same env)
	// needs a compositor: boot it again where the driver can.
	if testEnv.CanRestartCompositor() {
		t.Cleanup(func() {
			if err := testEnv.RestartCompositor(); err != nil {
				t.Errorf("restarting the compositor after the kill: %v", err)
			}
		})
	}

	// The client must exit on its own — the zombie scenario this issue
	// kills is a client that parks forever on a dead fd.
	select {
	case <-c.Exited():
	case <-time.After(killTimeout):
		// Evidence before failing: did the SIGKILL actually take sway
		// down (kill(2) with signal 0 only probes), where is the client
		// parked (SIGQUIT makes the Go runtime dump every goroutine's
		// stack into the client log), and who still holds the socket
		// ends (a blocked read means no EOF arrived, i.e. the peer fd
		// is still open in some process). The dump's tail is runtime
		// workers, so pull goroutine 1's frames out explicitly.
		swayAlive := syscall.Kill(pid, 0) == nil
		peers := clientSocketPeers(c.cmd.Process.Pid)
		_ = c.cmd.Process.Signal(syscall.SIGQUIT)
		time.Sleep(2 * time.Second)
		t.Fatalf("showcase outlived the compositor: no disconnect policy ran; compositor pid %d still alive: %v; procs: %s; client fd peers: %s; compositor log tail:\n\t%s; goroutine 1:\n\t%s",
			pid, swayAlive, liveCompositorProcs(), peers,
			strings.ReplaceAll(tailFile(testEnv.LogPath(), 40), "\n", "\n\t"),
			strings.ReplaceAll(mainGoroutineStack(c.LogPath), "\n", "\n\t"))
	}
	waitErr := c.Wait()
	var ee *exec.ExitError
	if !errors.As(waitErr, &ee) {
		t.Fatalf("client wait = %v, want an exit status", waitErr)
	}
	if code := ee.ExitCode(); code != app.DisconnectExitCode {
		tail := w.Tail(25)
		lines := make([]string, len(tail))
		for i, tr := range tail {
			lines[i] = tr.String()
		}
		t.Fatalf("client exit code = %d, want %d (signal death or crash, not the clean-exit policy); log tail:\n\t%s",
			code, app.DisconnectExitCode, strings.Join(lines, "\n\t"))
	}

	// The policy ran exactly once, and nothing panicked on the way out.
	if _, err := w.Wait("wire", "app: compositor disconnected", traceTimeout); err != nil {
		t.Errorf("disconnect trace missing: %v", err)
	}
	raw, err := os.ReadFile(c.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "app: compositor disconnected"); got != 1 {
		t.Errorf("disconnect policy traced %d times, want exactly once", got)
	}
	if strings.Contains(string(raw), "panic") {
		tail := w.Tail(25)
		lines := make([]string, len(tail))
		for i, tr := range tail {
			lines[i] = tr.String()
		}
		t.Errorf("client log contains a panic:\n\t%s", strings.Join(lines, "\n\t"))
	}
}
