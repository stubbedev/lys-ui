package headlesstest

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Trace is one line of the client's LYS_UI_DEBUG output: "<secs>s
// <category>: <message>" (see internal/debug). Lines that do not parse
// as traces (plain log.Fatal output, Go panics) keep Category empty
// and carry the whole line as Message, so the watcher can also wait
// for them.
type Trace struct {
	Secs     float64
	Category string
	Message  string
}

func (t Trace) String() string {
	if t.Category == "" {
		return t.Message
	}
	return fmt.Sprintf("%.3fs %s: %s", t.Secs, t.Category, t.Message)
}

var traceRe = regexp.MustCompile(`^\s*([0-9]+\.[0-9]+)s\s+([A-Za-z0-9_-]+): (.*)$`)

// ParseTraces extracts the trace lines from a log snapshot.
func ParseTraces(data []byte) []Trace {
	var out []Trace
	for line := range strings.SplitSeq(string(bytes.TrimSuffix(data, []byte("\n"))), "\n") {
		if m := traceRe.FindStringSubmatch(line); m != nil {
			secs, _ := strconv.ParseFloat(m[1], 64)
			out = append(out, Trace{Secs: secs, Category: m[2], Message: m[3]})
			continue
		}
		if strings.TrimSpace(line) != "" {
			out = append(out, Trace{Message: line})
		}
	}
	return out
}

// pollInterval is how often the watcher re-reads the log. Traces are
// assertions, not latency measurements; 15ms keeps suites fast without
// busy-spinning.
const pollInterval = 15 * time.Millisecond

// LogWatcher follows one client log from its current end, handing out
// only traces that arrived after the previous Wait: consecutive Waits
// assert order without re-matching older lines. Lines read but not yet
// matched stay pending, so a burst written between two polls is
// delivered one Wait at a time instead of being swallowed by whichever
// Wait ran first. It is safe for one goroutine at a time (the tests
// are sequential).
type LogWatcher struct {
	path string
	mu   sync.Mutex
	// readPos is how far the file has been read; lines read but not
	// matched yet sit in pending.
	readPos int64
	pending []pendingTrace
	seen    []Trace
}

// pendingTrace is a parsed line waiting to be matched, with the file
// offset just past its newline so consumption can advance precisely.
type pendingTrace struct {
	tr  Trace
	end int64
}

// Watch starts following the log at path from its current size, so a
// watcher only sees what happens after it was attached.
func Watch(path string) (*LogWatcher, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &LogWatcher{path: path, readPos: st.Size()}, nil
}

// Wait polls the log until a trace in category containing substr
// arrives, and returns it. category "" matches any line (including
// non-trace output). On timeout the error carries the tail of what did
// arrive, which is the failure evidence for the suite.
func (w *LogWatcher) Wait(category, substr string, timeout time.Duration) (Trace, error) {
	deadline := time.Now().Add(timeout)
	for {
		t, ok := w.step(category, substr)
		if ok {
			return t, nil
		}
		if time.Now().After(deadline) {
			return Trace{}, fmt.Errorf("timeout (%s) waiting for [%s] %q; log tail:\n%s",
				timeout, category, substr, w.Tail(25))
		}
		time.Sleep(pollInterval)
	}
}

// WaitEver waits until a trace in category containing substr has
// arrived at any point since the watcher attached - lines earlier
// Waits already passed included - and consumes nothing: for state that
// holds once reached (a window's keyboard focus) rather than an event
// in a sequence.
func (w *LogWatcher) WaitEver(category, substr string, timeout time.Duration) error {
	return w.WaitSince(0, category, substr, timeout)
}

// WaitSince waits until a trace in category containing substr has
// arrived after mark, consuming nothing: an event the caller marked
// the start of, whether or not a Wait already read past it.
func (w *LogWatcher) WaitSince(mark int, category, substr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for !w.SeenSince(mark, category, substr) {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout (%s): [%s] %q never arrived; log tail:\n%s", timeout, category, substr, w.Tail(25))
		}
		time.Sleep(pollInterval)
	}
	return nil
}

// Mark is a position in the watcher's history, for SeenSince and
// WaitSince.
func (w *LogWatcher) Mark() int {
	w.readNew()
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.seen)
}

// SeenSince reports whether a trace in category containing substr
// arrived after mark, without consuming anything.
func (w *LogWatcher) SeenSince(mark int, category, substr string) bool {
	return w.CountSince(mark, category, substr) > 0
}

// CountSince counts the traces in category containing substr that
// arrived after mark, without consuming anything.
func (w *LogWatcher) CountSince(mark int, category, substr string) int {
	w.readNew()
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, tr := range w.seen[min(mark, len(w.seen)):] {
		if (category == "" || tr.Category == category) && strings.Contains(tr.Message, substr) {
			n++
		}
	}
	return n
}

// Since returns the traces that arrived after mark, oldest first,
// without consuming anything.
func (w *LogWatcher) Since(mark int) []Trace {
	w.readNew()
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]Trace(nil), w.seen[min(mark, len(w.seen)):]...)
}

// MaxGapSince returns the largest pause in seconds between consecutive
// traces in category containing substr after mark, counting the wait
// for the first such trace from mark itself. Software renderers under
// suite load present slowly but steadily; a pacing bug stops outright,
// and this is the signal that tells them apart from a low frame count.
func (w *LogWatcher) MaxGapSince(mark int, category, substr string) float64 {
	w.readNew()
	w.mu.Lock()
	defer w.mu.Unlock()
	base := 0.0
	if mark > 0 && mark <= len(w.seen) {
		base = w.seen[mark-1].Secs
	}
	gap, last := 0.0, -1.0
	for _, tr := range w.seen[min(mark, len(w.seen)):] {
		if (category != "" && tr.Category != category) || !strings.Contains(tr.Message, substr) {
			continue
		}
		if last < 0 {
			gap = tr.Secs - base
		} else {
			gap = max(gap, tr.Secs-last)
		}
		last = tr.Secs
	}
	return gap
}

// WaitAll is Wait for a set: it waits until each substr has arrived
// (one trace per substr, in ANY order) since this call began. State
// transitions produce two racing traces - the confirmed-state line and
// the relayout draw - and a sequential Wait would consume whichever
// came first, starving the other. Unconsumed traces left over from
// earlier waits are discarded at the start, so a phase can only be
// satisfied by its own window.
func (w *LogWatcher) WaitAll(category string, timeout time.Duration, substrs ...string) error {
	w.mu.Lock()
	w.pending = nil
	// Only traces read from here on count: seen holds the whole
	// history, and an earlier step's matching line must not satisfy
	// this one (it once could - a false pass).
	scanned := len(w.seen)
	w.mu.Unlock()
	deadline := time.Now().Add(timeout)
	pending := append([]string(nil), substrs...)
	for {
		w.readNew()
		w.mu.Lock()
		for _, tr := range w.seen[scanned:] {
			scanned++
			if category != "" && tr.Category != category {
				continue
			}
			for i, p := range pending {
				if strings.Contains(tr.Message, p) {
					pending = append(pending[:i], pending[i+1:]...)
					break
				}
			}
		}
		w.mu.Unlock()
		if len(pending) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout (%s) waiting for %v; log tail:\n%s",
				timeout, pending, w.Tail(25))
		}
		time.Sleep(pollInterval)
	}
}

// step pulls newly complete lines into the pending queue, then serves
// the first match from it.
func (w *LogWatcher) step(category, substr string) (Trace, bool) {
	w.readNew()
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, pt := range w.pending {
		if category != "" && pt.tr.Category != category {
			continue
		}
		if strings.Contains(pt.tr.Message, substr) {
			w.pending = w.pending[i+1:]
			return pt.tr, true
		}
	}
	return Trace{}, false
}

// readNew reads newly complete lines of the log into seen and pending.
// Wait reads only the first match; WaitAll scans everything it has not
// examined yet, so the reading half lives here.
func (w *LogWatcher) readNew() {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(w.readPos, 0); err != nil {
		return
	}
	// Read whole lines with their exact file extents: only complete
	// lines are consumed (a writer mid-line stays for the next poll).
	var raw []byte
	buf := make([]byte, 0, 64*1024)
	for {
		n, err := f.Read(buf[:cap(buf)])
		raw = append(raw, buf[:n]...)
		if err != nil {
			break
		}
	}
	for len(raw) > 0 {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			break
		}
		line := raw[:i]
		raw = raw[i+1:]
		w.readPos += int64(i) + 1
		tr := parseLine(line)
		w.seen = append(w.seen, tr)
		w.pending = append(w.pending, pendingTrace{tr: tr, end: w.readPos})
	}
}

// parseLine turns one log line into a Trace; non-trace lines keep the
// whole line as Message with an empty Category.
func parseLine(line []byte) Trace {
	if m := traceRe.FindSubmatch(line); m != nil {
		secs, _ := strconv.ParseFloat(string(m[1]), 64)
		return Trace{Secs: secs, Category: string(m[2]), Message: string(m[3])}
	}
	return Trace{Message: string(line)}
}

// Tail returns up to n most recent traces seen so far, oldest first,
// for failure messages.
func (w *LogWatcher) Tail(n int) []Trace {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.seen) > n {
		return append([]Trace(nil), w.seen[len(w.seen)-n:]...)
	}
	return append([]Trace(nil), w.seen...)
}

// tailFile is a standalone last-n-lines helper for files the watcher
// is not attached to (sway's own log).
func tailFile(path string, n int) string {
	data, err := os.ReadFile(path) //nolint:gosec // the path is the harness's own runtime dir
	if err != nil {
		return fmt.Sprintf("(unreadable: %v)", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
