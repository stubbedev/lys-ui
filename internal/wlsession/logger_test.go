package wlsession

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/logutil"
)

// capturedOutput redirects the process's stderr and stdout into pipes
// for the duration of a test, so a lifecycle run can be asserted to
// produce nothing (the silent-by-default contract) or exactly the
// records an injected logger emits. Tests run sequentially within the
// package, so nothing else writes into the pipes. Reading happens
// after the writers close, so the volumes are bounded by the lifecycle
// itself, never by a blocked reader.
type capturedOutput struct {
	oldErr, oldOut *os.File
	errR, errW     *os.File
	outR, outW     *os.File
	mu             sync.Mutex
	restored       bool
	stderr, stdout string
}

func captureOutput(t *testing.T) *capturedOutput {
	t.Helper()
	c := &capturedOutput{}
	var err error
	if c.errR, c.errW, err = os.Pipe(); err != nil {
		t.Fatal(err)
	}
	if c.outR, c.outW, err = os.Pipe(); err != nil {
		t.Fatal(err)
	}
	c.oldErr, c.oldOut = os.Stderr, os.Stdout
	os.Stderr, os.Stdout = c.errW, c.outW
	t.Cleanup(c.restore)
	return c
}

// restore puts the real streams back, closes the writers so a pending
// ReadAll completes, and drains the pipes exactly once. Idempotent:
// the test may restore early to flush before asserting.
func (c *capturedOutput) restore() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.restored {
		return
	}
	c.restored = true
	os.Stderr, os.Stdout = c.oldErr, c.oldOut
	c.errW.Close()
	c.outW.Close()
	eb, _ := io.ReadAll(c.errR)
	ob, _ := io.ReadAll(c.outR)
	c.errR.Close()
	c.outR.Close()
	c.stderr, c.stdout = string(eb), string(ob)
}

// Stderr flushes and returns everything written to the captured
// stderr.
func (c *capturedOutput) Stderr() string {
	c.restore()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stderr
}

// Stdout flushes and returns everything written to the captured
// stdout.
func (c *capturedOutput) Stdout() string {
	c.restore()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stdout
}

// driveLifecycle walks one fake session through the states that emit
// library output: an optional global bind, the post-registry scan for
// missing optional globals, the compositor raising a fatal protocol
// error, and teardown. No compositor and no wire objects: every step
// is the same code path a live connection runs (bindFailed is the
// funnel of every optional bind site; Close is nil-safe on a session
// that never fully connected). In a prod build (no lysdebug tag) the
// trace facility at these sites is compiled out, so only the injected
// logger can produce output.
func driveLifecycle(t *testing.T) {
	t.Helper()
	s := &Session{globals: make(map[string]bool), ifaceNames: make(map[uint32]string)}

	// Bind: an advertised optional global refuses the bind.
	s.bindFailed("xdg_activation_v1", errors.New("version unsupported"))

	// Optional globals missing: the scan Connect runs after the
	// registry burst (globals empty — a compositor with none of them).
	s.logOptionalGlobals()

	// Protocol warning: the compositor's fatal wl_display.error.
	s.HandleDisplayError(wl.DisplayErrorEvent{Message: "compositor object mismatch"})

	// Close: teardown of a session that never fully connected.
	s.Close()
}

func setTestLogger(t *testing.T, l *slog.Logger) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	if l != nil {
		SetLogger(slog.New(slog.NewTextHandler(buf, nil)))
	} else {
		SetLogger(nil)
	}
	t.Cleanup(func() { SetLogger(nil) })
	return buf
}

// TestLifecycleSilentByDefault is the headline: with no logger
// configured — the default every embedding app gets — a full session
// lifecycle writes zero bytes to stderr or stdout. Any direct
// fmt.Fprint in library code shows up here.
func TestLifecycleSilentByDefault(t *testing.T) {
	out := captureOutput(t)
	driveLifecycle(t)
	out.restore()

	if got := out.Stderr(); got != "" {
		t.Errorf("library wrote to stderr with the default logger:\n%s", got)
	}
	if got := out.Stdout(); got != "" {
		t.Errorf("library wrote to stdout with the default logger:\n%s", got)
	}
}

// TestLifecycleInjectedLogger pins the levels contract: the bind
// failure is Warn (degraded but running), the missing optional
// protocols are Debug chatter, the compositor's fatal protocol error
// is Error (terminal), and nothing is ever Info.
func TestLifecycleInjectedLogger(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { SetLogger(nil) })

	driveLifecycle(t)
	got := buf.String()

	for _, want := range []string{
		`level=WARN msg="wlsession: optional global bind failed; feature disabled" global=xdg_activation_v1`,
		`level=ERROR msg="wlsession: compositor fatal protocol error"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q\nfull output:\n%s", want, got)
		}
	}
	if n := strings.Count(got, `level=DEBUG msg="wlsession: optional protocol not advertised"`); n != len(optionalGlobals) {
		t.Errorf("missing-optional Debug records = %d, want %d (one per optional global)\nfull output:\n%s",
			n, len(optionalGlobals), got)
	}
	if strings.Contains(got, "level=INFO") {
		t.Errorf("library must not log at Info\nfull output:\n%s", got)
	}
}

// TestNilLoggerDiscards: SetLogger(nil) must mean discard — the same
// lifecycle that logged in the injected case goes quiet again.
func TestNilLoggerDiscards(t *testing.T) {
	out := captureOutput(t)
	buf := setTestLogger(t, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	SetLogger(nil)

	driveLifecycle(t)
	out.restore()

	if buf.Len() != 0 {
		t.Errorf("SetLogger(nil) still routed records:\n%s", buf.String())
	}
	if got := out.Stderr(); got != "" {
		t.Errorf("nil logger leaked to stderr:\n%s", got)
	}
}

// TestDebugChatterBelowWarnLevel: an application running its logger at
// the usual Warn threshold hears the warnings and the terminal error
// but none of the protocol chatter.
func TestDebugChatterBelowWarnLevel(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { SetLogger(nil) })

	driveLifecycle(t)
	got := buf.String()

	if strings.Contains(got, "level=DEBUG") {
		t.Errorf("Debug chatter passed a Warn-level handler:\n%s", got)
	}
	if !strings.Contains(got, `level=WARN`) || !strings.Contains(got, `level=ERROR`) {
		t.Errorf("Warn/Error records missing at a Warn threshold:\n%s", got)
	}
}

// TestOptionalGlobalsNotRequired: the chatter list must stay out of
// the required list — an optional global logged as missing must never
// fail Connect.
func TestOptionalGlobalsNotRequired(t *testing.T) {
	for _, g := range optionalGlobals {
		for _, req := range requiredGlobals {
			if g == req {
				t.Errorf("%s is both required and optional", g)
			}
		}
	}
}

// TestLogutilDefaultDiscard re-asserts the package-level default the
// session contract leans on: logutil starts discarding, and Set(nil)
// returns there.
func TestLogutilDefaultDiscard(t *testing.T) {
	if logutil.L() == nil {
		t.Fatal("logutil.L() = nil, want a discarding logger")
	}
	logutil.Set(nil)
	if logutil.L().Handler().Enabled(t.Context(), slog.LevelError) {
		t.Error("default logger must be enabled at no level")
	}
}
