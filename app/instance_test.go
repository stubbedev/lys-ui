package app

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/wltest"
)

// feedForward hands the primary a forwarded invocation the way a
// secondary process would: dial, write, wait for the ack.
func feedForward(t *testing.T, ins *Instance, inv Invocation) {
	t.Helper()
	conn, err := net.Dial("unix", ins.path)
	if err != nil {
		t.Fatalf("dial instance socket: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := writeInvocation(conn, inv); err != nil {
		t.Fatalf("write invocation: %v", err)
	}
	ack := make([]byte, 1)
	if _, err := readFullDeadline(conn, ack); err != nil {
		t.Fatalf("no ack from primary: %v", err)
	}
}

func readFullDeadline(conn net.Conn, buf []byte) (int, error) {
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// TestClaimTwoProcessesOneWins is the single-instance acceptance with
// real processes: the parent claims first, a child process claims the
// same AppID, forwards its invocation, and exits as the secondary; the
// parent's hooks receive exactly what the child sent.
func TestClaimTwoProcessesOneWins(t *testing.T) {
	wltest.RuntimeDir(t)
	appID := "dev.stubbe.test.claim"
	var lines []string
	var opens []string
	var cwd string
	ins, primary, err := ClaimInstance(InstanceConfig{
		AppID: appID,
		OnCommandLine: func(args []string, dir string) {
			lines = append(lines, fmt.Sprint(args))
		},
		OnOpen: func(paths []string, dir string) {
			opens = append(opens, paths...)
			cwd = dir
		},
	}, Invocation{})
	if err != nil || !primary {
		t.Fatalf("first claim: primary=%v err=%v", primary, err)
	}
	t.Cleanup(ins.Close)

	cmd := exec.Command(os.Args[0], "-test.run=TestInstanceChildProcess")
	cmd.Env = append(os.Environ(), "LYS_UI_INSTANCE_CHILD=1", "LYS_UI_INSTANCE_APPID="+appID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child claim run: %v\n%s", err, out)
	}
	if want := "child-secondary\n"; string(out) != want {
		t.Fatalf("child reported %q, want %q", out, want)
	}

	a := testApp(nil)
	ins.Bind(a)
	a.pump(time.Now())
	if len(opens) != 1 || opens[0] != "/tmp/one" {
		t.Errorf("OnOpen saw %v, want the child's --open path", opens)
	}
	if len(lines) != 0 {
		t.Errorf("OnCommandLine saw %v, want nothing: open replaces command-line", lines)
	}
	if cwd != "/wd" {
		t.Errorf("cwd = %q, want the child's working directory", cwd)
	}
}

// TestInstanceChildProcess is the secondary side of
// TestClaimTwoProcessesOneWins, run as its own process.
func TestInstanceChildProcess(t *testing.T) {
	if os.Getenv("LYS_UI_INSTANCE_CHILD") != "1" {
		return
	}
	inv := Invocation{
		Args:  []string{"from", "child"},
		Open:  []string{"/tmp/one"},
		Cwd:   "/wd",
		Token: "tok",
	}
	ins, primary, err := ClaimInstance(InstanceConfig{AppID: os.Getenv("LYS_UI_INSTANCE_APPID")}, inv)
	switch {
	case err != nil:
		fmt.Printf("child-error %v\n", err)
		os.Exit(2)
	case primary:
		ins.Close()
		fmt.Printf("child-primary\n")
		os.Exit(3)
	default:
		fmt.Printf("child-secondary\n")
		os.Exit(0)
	}
}

// TestClaimReclaimsStaleSocket covers the crashed-primary case: a
// socket left behind by a process that never unlinked it must not lock
// the AppID forever - the next claim dials, gets no answer, and takes
// over.
func TestClaimReclaimsStaleSocket(t *testing.T) {
	wltest.RuntimeDir(t)
	appID := "dev.stubbe.test.stale"
	path, err := instanceSocketPath(appID)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close() // the file stays; nobody answers on it

	ins, primary, err := ClaimInstance(InstanceConfig{AppID: appID}, Invocation{})
	if err != nil || !primary {
		t.Fatalf("claim over stale socket: primary=%v err=%v", primary, err)
	}
	ins.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Close left the instance socket behind")
	}
}

// TestInstanceBuffersUntilBind pins the pre-Run contract: invocations
// forwarded between the claim and Bind wait in the mailbox and flush
// through the first pump afterwards, so no secondary is ever dropped
// for wiring slowly.
func TestInstanceBuffersUntilBind(t *testing.T) {
	wltest.RuntimeDir(t)
	var got []string
	ins, primary, err := ClaimInstance(InstanceConfig{
		AppID:         "dev.stubbe.test.buffer",
		OnCommandLine: func(args []string, _ string) { got = append(got, args...) },
	}, Invocation{})
	if err != nil || !primary {
		t.Fatalf("claim: primary=%v err=%v", primary, err)
	}
	defer ins.Close()

	feedForward(t, ins, Invocation{Args: []string{"early"}})
	feedForward(t, ins, Invocation{Args: []string{"again"}})

	a := testApp(nil)
	ins.Bind(a)
	if len(got) != 0 {
		t.Fatalf("hook ran before the loop pumped: %v", got)
	}
	a.pump(time.Now())
	if len(got) != 2 || got[0] != "early" || got[1] != "again" {
		t.Errorf("buffered invocations delivered %v, want [early again] in order", got)
	}
}

// TestOpenReplacesCommandLine pins the GApplication routing: an
// invocation carrying open paths fires OnOpen alone; the plain argv
// path and the empty-activate path fire OnCommandLine.
func TestOpenReplacesCommandLine(t *testing.T) {
	wltest.RuntimeDir(t)
	var lines, opens int
	ins, primary, err := ClaimInstance(InstanceConfig{
		AppID:         "dev.stubbe.test.open",
		OnCommandLine: func([]string, string) { lines++ },
		OnOpen:        func([]string, string) { opens++ },
	}, Invocation{})
	if err != nil || !primary {
		t.Fatalf("claim: primary=%v err=%v", primary, err)
	}
	defer ins.Close()

	a := testApp(nil)
	ins.Bind(a)
	feedForward(t, ins, Invocation{Args: []string{"x"}, Open: []string{"/f"}})
	feedForward(t, ins, Invocation{Args: []string{"y"}})
	feedForward(t, ins, Invocation{})
	a.pump(time.Now())
	if opens != 1 || lines != 2 {
		t.Errorf("routing fired OnOpen %d and OnCommandLine %d times, want 1 and 2", opens, lines)
	}
}

// TestMultipleInstancesAllowed checks the opt-out: with
// AllowMultipleInstances every process is its own primary and no socket
// guards the AppID.
func TestMultipleInstancesAllowed(t *testing.T) {
	wltest.RuntimeDir(t)
	cfg := InstanceConfig{AppID: "dev.stubbe.test.multi", AllowMultipleInstances: true}
	first, primary, err := ClaimInstance(cfg, Invocation{})
	if err != nil || !primary {
		t.Fatalf("first claim: primary=%v err=%v", primary, err)
	}
	first.Close()
	second, primary, err := ClaimInstance(cfg, Invocation{})
	if err != nil || !primary {
		t.Fatalf("second claim with the guard off: primary=%v err=%v", primary, err)
	}
	second.Close()
}

// TestInstanceDiesWithLoop pins the loop-owned lifecycle: what Run's
// exit does unlinks the socket, so a killed application leaves the
// AppID claimable again.
func TestInstanceDiesWithLoop(t *testing.T) {
	wltest.RuntimeDir(t)
	ins, primary, err := ClaimInstance(InstanceConfig{AppID: "dev.stubbe.test.dies"}, Invocation{})
	if err != nil || !primary {
		t.Fatalf("claim: primary=%v err=%v", primary, err)
	}
	path, err := instanceSocketPath("dev.stubbe.test.dies")
	if err != nil {
		t.Fatal(err)
	}
	a := testApp(nil)
	ins.Bind(a)
	a.watchers.shutdown()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the loop's end left the instance socket behind")
	}
}

// TestOSInvocation pins the launch convention: plain argv passes
// through, --open pairs move into Open, and the activation token rides
// along from the environment.
func TestOSInvocation(t *testing.T) {
	t.Setenv("XDG_ACTIVATION_TOKEN", "tok-1")
	inv := OSInvocation([]string{"--verbose", "--open", "/a", "positional", "--open", "/b"})
	if len(inv.Args) != 2 || inv.Args[0] != "--verbose" || inv.Args[1] != "positional" {
		t.Errorf("args = %v, want --open pairs stripped", inv.Args)
	}
	if len(inv.Open) != 2 || inv.Open[0] != "/a" || inv.Open[1] != "/b" {
		t.Errorf("open = %v, want [/a /b]", inv.Open)
	}
	if inv.Token != "tok-1" {
		t.Errorf("token = %q, want the environment's", inv.Token)
	}
}
