package headlesstest

import (
	"bytes"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// buildTimeout bounds one `go build` of the client under test; a warm
// cache compiles it in seconds, a cold one needs the slack.
const buildTimeout = 3 * time.Minute

// ModuleDir locates the lys-ui module root by walking up from the
// current directory (the tests run inside the module tree), so the
// harness can build cmd targets without assuming a working directory.
func ModuleDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("headlesstest: no go.mod above %s", dir)
		}
		dir = parent
	}
}

// BuildShowcase compiles the widget showcase with the lysdebug tag
// (its trace calls are compiled out otherwise) into dir/lys-ui-hello and
// returns the binary path.
func BuildShowcase(dir string) (string, error) {
	return BuildClient(dir, "./cmd/lys-ui-hello", "lys-ui-hello")
}

// BuildClient compiles one lys-ui command package with the lysdebug tag
// (trace calls are compiled out otherwise) into dir/bin and returns the
// binary path.
func BuildClient(dir, pkg, bin string) (string, error) {
	root, err := ModuleDir()
	if err != nil {
		return "", err
	}
	// goTool and dir come from the module tree this test binary runs in,
	// not from any untrusted source.
	cmd := exec.Command(goTool(), "build", "-tags", "lysdebug", "-o", filepath.Join(dir, bin), pkg) //nolint:gosec // fixed subcommand, module-local paths
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := runWithTimeout(cmd, buildTimeout)
	if err != nil {
		return "", fmt.Errorf("build %s: %w\n%s", pkg, err, out)
	}
	return filepath.Join(dir, bin), nil
}

// goTool prefers the go in PATH; GOROOT-built test binaries (go test
// run by the toolchain itself) still find it there.
func goTool() string {
	if _, err := exec.LookPath("go"); err == nil {
		return "go"
	}
	return filepath.Join(build.Default.GOROOT, "bin", "go")
}

// Client is one run of the client under test inside an Env, with its
// stderr and stdout going to a log file a LogWatcher can follow.
type Client struct {
	Name    string
	LogPath string

	cmd     *exec.Cmd
	logFile *os.File

	exitOnce sync.Once
	exited   chan struct{}
	reaped   bool
	waitErr  error
}

// StartShowcase runs the showcase binary inside the env with input,
// frame, seat, wire and demo tracing on, and returns the handle. The
// caller must Stop it (or Wait it after a close request).
func (e *Env) StartShowcase(bin, name string) (*Client, error) {
	return e.StartClient(bin, name, "input,frame,demo,seat,wire")
}

// StartClient runs a built client binary inside the env with the given
// LYS_DEBUG categories and extra arguments, its stdout and stderr
// going to a log file a LogWatcher can follow. The caller must Stop it
// (or Wait it after a close request).
func (e *Env) StartClient(bin, name, categories string, args ...string) (*Client, error) {
	return e.StartClientEnv(bin, name, categories, nil, args...)
}

// StartClientEnv is StartClient with extra environment entries
// (KEY=value), for knobs such as LYS_UI_NO_CURSOR_SHAPE.
func (e *Env) StartClientEnv(bin, name, categories string, env []string, args ...string) (*Client, error) {
	logPath := filepath.Join(e.Dir, name+".log")
	// The path is the harness's own runtime dir, built from the test's
	// name; nothing user-controlled reaches it.
	log, err := os.Create(logPath) //nolint:gosec // module-local path, see above
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // module-local binary, see above
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Env = append(privateEnv(
		"WAYLAND_DISPLAY="+e.Display,
		"XDG_RUNTIME_DIR="+e.Dir,
		"LYS_DEBUG="+categories,
	), env...)
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		return nil, err
	}
	return &Client{Name: name, LogPath: logPath, cmd: cmd, logFile: log}, nil
}

// Watch attaches a LogWatcher to this client's log.
func (c *Client) Watch() (*LogWatcher, error) {
	return Watch(c.LogPath)
}

// Exited returns a channel that closes once the client process has
// exited and been reaped. Tests arm it to assert the client SURVIVED
// an event (a tooltip, a grab) - a dead client passes every trace wait
// that already matched, so survival needs its own assertion.
func (c *Client) Exited() <-chan struct{} {
	c.exitOnce.Do(func() {
		c.exited = make(chan struct{})
		go func() {
			c.waitErr = c.cmd.Wait()
			c.reaped = true
			close(c.exited)
		}()
	})
	return c.exited
}

// Wait blocks until the client process exits (e.g. after the Escape
// key closed the window) and reports whether it exited cleanly. A
// nonzero exit or signal death comes back as the error.
func (c *Client) Wait() error {
	<-c.Exited()
	return c.waitErr
}

// Stop kills the client if it is still running and reaps it. Safe in
// any order relative to Wait and any number of times.
func (c *Client) Stop() {
	if !c.reaped && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	<-c.Exited()
	_ = c.logFile.Close()
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return out.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return out.String(), fmt.Errorf("timed out after %s: %w", timeout, <-done)
	}
}
