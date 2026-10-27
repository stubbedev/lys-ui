package headlesstest

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/wltest"
)

// The compositor-in-the-loop suite: attaches to the private headless
// sway session the just headless recipe booted (TestMain, only when
// LYS_TEST_HEADLESS is set), runs the widget showcase inside it, and drives
// a synthetic seat against it. Every
// assertion lands on the client's own trace log - the input must have
// traversed compositor, wire, session and widget tree for a trace to
// appear, which is what makes this a regression net for live-input
// bugs (#1) that widget-tree unit tests cannot see.

// run gates the suite: without LYS_TEST_HEADLESS the integration tests
// skip (local dev has no compositor), with it set the env is required
// and a missing compositor fails the package - a skip behind
// LYS_TEST_HEADLESS would be a silent hole in CI.
func run(m *testing.M) int {
	if os.Getenv(wltest.HeadlessEnv) == "" {
		return m.Run()
	}
	env, err := Attach()
	if err != nil {
		fmt.Fprintf(os.Stderr, "headlesstest: %v\n", err)
		return 1
	}
	testEnv = env
	bin, err := BuildShowcase(env.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "headlesstest: %v\n", err)
		return 1
	}
	testBin = bin
	return m.Run()
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

var (
	testEnv *Env
	testBin string
)

// traceTimeout bounds every single-trace wait. Compositor scheduling
// adds jitter; nothing here should ever need this long, so exceeding
// it is a real failure.
const traceTimeout = 8 * time.Second

// showcaseControls is how many "control ... center" traces the
// showcase logs after mapping: the seven core controls plus the
// sixth-pass section's (listbox, its three toggles, the fixed
// button, the typed list, the draw surface, and the open button's two
// parts).
const showcaseControls = 16

// dwellTime is how long the pointer rests on a widget in the tooltip
// test: past app's 500ms tooltip delay, short of test patience.
const dwellTime = 900 * time.Millisecond

// attemptTimeout bounds one tap's effect in tests that retry taps:
// synthetic input on a loaded compositor can be lost, so a few short
// attempts beat one long wait.
const attemptTimeout = 3 * time.Second

func requireEnv(t *testing.T) {
	t.Helper()
	if os.Getenv(wltest.HeadlessEnv) == "" {
		t.Skip("LYS_TEST_HEADLESS is not set; compositor input tests run under just check-headless")
	}
}

// startShowcase launches a fresh showcase client (its own log, its own
// window) and returns it with a trace watcher and the control centers
// the client itself traced after mapping.
func startShowcase(t *testing.T) (*Client, *LogWatcher, map[string][2]int) {
	t.Helper()
	return startShowcaseEnv(t)
}

// startShowcaseEnv is startShowcase with extra client environment.
func startShowcaseEnv(t *testing.T, env ...string) (*Client, *LogWatcher, map[string][2]int) {
	t.Helper()
	c, err := testEnv.StartClientEnv(testBin, "client-"+strings.ReplaceAll(t.Name(), "/", "-"), "input,frame,demo,seat,wire,shell", env)
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", fmt.Sprintf("mapped %dx%d", showcaseW, showcaseH), traceTimeout); err != nil {
		t.Fatalf("showcase never mapped at the pinned %dx%d: %v\ncompositor log tail:\n\t%s",
			showcaseW, showcaseH, err, strings.ReplaceAll(tailFile(testEnv.Dir+"/sway.log", 15), "\n", "\n\t"))
	}
	centers := make(map[string][2]int)
	for range showcaseControls {
		tr, err := w.Wait("demo", "control ", traceTimeout)
		if err != nil {
			t.Fatalf("control center trace missing: %v", err)
		}
		name, xy, err := parseCenter(tr.Message)
		if err != nil {
			t.Fatalf("bad control trace %q: %v", tr.Message, err)
		}
		centers[name] = xy
	}
	waitPresented(t, w)
	return c, w, centers
}

func parseCenter(msg string) (string, [2]int, error) {
	var name string
	var x, y int
	if _, err := fmt.Sscanf(msg, "control %s center (%d,%d)", &name, &x, &y); err != nil {
		return "", [2]int{}, err
	}
	return name, [2]int{x, y}, nil
}

// newInput connects a synthetic seat to the test compositor.
func newInput(t *testing.T) *VirtualInput {
	t.Helper()
	v, err := Dial(os.Getenv("WAYLAND_DISPLAY"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}

// newPointerOnlyInput connects a synthetic seat with no keyboard.
func newPointerOnlyInput(t *testing.T) *VirtualInput {
	t.Helper()
	v, err := DialPointerOnly(os.Getenv("WAYLAND_DISPLAY"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}

// clickUntil clicks (x, y) until the expected trace lands: a freshly
// booted compositor eats the first synthetic presses (the socket is
// pollable long before the seat reliably delivers buttons), so one
// click there flakes the gate on lost input instead of a real
// regression. A few short attempts beat one long wait - the same trade
// tapUntil makes for keys. A lost press leaves no trace, so a retry is
// honest: the effect shows up exactly once, on the attempt that lands.
func clickUntil(w *LogWatcher, in *VirtualInput, category string, x, y int, button uint32, want string) error {
	for range 3 {
		mark := w.Mark()
		if err := in.ClickAt(x, y, button); err != nil {
			return fmt.Errorf("click: %w", err)
		}
		if err := w.WaitAll(category, attemptTimeout, want); err == nil {
			return nil
		}
		// Re-click only a press the client never saw: once it saw the
		// press, the action is merely slow, and clicking again would
		// repeat it - toggling a row back out, closing what it opened.
		if w.SeenSince(mark, "input", "press at") {
			return w.WaitSince(mark, category, want, traceTimeout)
		}
	}
	return fmt.Errorf("no trace [%s] %q after 3 attempts; last tail:\n%s", category, want, w.Tail(25))
}

// stopAndReport stops c when the test ends and, on failure, logs its
// log tail: the whole client log is the evidence, the trace tails a
// Wait error carries cover only what was read in time.
func stopAndReport(t *testing.T, c *Client) {
	t.Helper()
	t.Cleanup(func() {
		c.Stop()
		if !t.Failed() {
			return
		}
		if data, err := os.ReadFile(c.LogPath); err == nil {
			// The idle frame trace (every pass, nothing to paint) would
			// push everything else out of the tail.
			var lines []string
			for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
				if !strings.Contains(line, "draw skipped: nothing damaged") {
					lines = append(lines, line)
				}
			}
			if len(lines) > 160 {
				lines = lines[len(lines)-160:]
			}
			t.Logf("client log tail:\n\t%s", strings.Join(lines, "\n\t"))
		}
	})
}

// waitPresented waits until the compositor presented the client's
// window - its first frame callback fired. The client's own "mapped"
// trace comes before its first frame even lands, and pointer input
// aimed at a window not yet on screen hits whatever is beneath it.
func waitPresented(t *testing.T, w *LogWatcher) {
	t.Helper()
	if err := w.WaitEver("frame", "frame callback done", traceTimeout); err != nil {
		t.Fatalf("the window was never presented: %v", err)
	}
}

// pollSize asks the states client for its confirmed state (the p
// chord) and returns the size it reports.
func pollSize(t *testing.T, tap func(...rune), w *LogWatcher) string {
	t.Helper()
	for range 4 {
		tap('p')
		if tr, err := w.Wait("demo", "polled ", attemptTimeout); err == nil {
			if i := strings.LastIndexByte(tr.Message, ' '); i >= 0 {
				return tr.Message[i+1:]
			}
		}
	}
	t.Fatalf("the poll never answered: %s", tailTraces(w, 15))
	return ""
}

// waitKeyboard waits until the client holds keyboard focus: keys sent
// before the compositor focused the window go nowhere (Hyprland
// activates a new window a beat after its first frame; sway's headless
// seat has no keyboard at all until the test's virtual one exists).
func waitKeyboard(t *testing.T, w *LogWatcher) {
	t.Helper()
	if err := w.WaitEver("input", "keyboard enter", traceTimeout); err != nil {
		t.Fatalf("the client never got keyboard focus: %v", err)
	}
}

// hoverUntil moves the pointer onto (x, y) (MoveOnto) until want
// traces.
func hoverUntil(w *LogWatcher, in *VirtualInput, category string, x, y int, want string) error {
	for range 3 {
		if err := in.MoveOnto(x, y); err != nil {
			return fmt.Errorf("move: %w", err)
		}
		if err := w.WaitAll(category, attemptTimeout, want); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no trace [%s] %q after 3 attempts; last tail:\n%s", category, want, w.Tail(25))
}

// TestHeadlessClicksOnShowcaseControls is the regression net for #1:
// synthetic clicks must land on real widgets and the showcase's own
// handlers must run. A broken click path dies here before it can be
// dead-on-hardware.
func TestHeadlessClicksOnShowcaseControls(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	in := newInput(t)

	click := func(name string) [2]int {
		t.Helper()
		c, ok := centers[name]
		if !ok {
			t.Fatalf("no traced center for %s", name)
		}
		return c
	}

	// The button: two clicks, two activations.
	btn := click("button")
	if err := clickUntil(w, in, "demo", btn[0], btn[1], BTNLeft, "button clicked 1 times"); err != nil {
		t.Errorf("first click never reached the button: %v", err)
	}
	if err := clickUntil(w, in, "demo", btn[0], btn[1], BTNLeft, "button clicked 2 times"); err != nil {
		t.Errorf("second click never reached the button: %v", err)
	}

	// The switch starts on; one click turns it off.
	sw := click("switch")
	if err := clickUntil(w, in, "demo", sw[0], sw[1], BTNLeft, "switch false"); err != nil {
		t.Errorf("click never toggled the switch: %v", err)
	}

	// The checkbox starts off; one click turns it on.
	check := click("checkbox")
	if err := clickUntil(w, in, "demo", check[0], check[1], BTNLeft, "checkbox true"); err != nil {
		t.Errorf("click never checked the checkbox: %v", err)
	}

	// A drag mid-track moves the slider: this slider changes value on
	// drag and arrows only (a plain click on the trough is a no-op).
	s := click("slider")
	if err := in.DragTo(s[0], s[1], s[0]+80, s[1], 4); err != nil {
		t.Fatalf("drag slider: %v", err)
	}
	if _, err := w.Wait("demo", "slider at ", traceTimeout); err != nil {
		t.Errorf("drag never set the slider: %v", err)
	}

	// Clicking the entry focuses it; keys then land in it.
	in.ClickAt(click("entry")[0], click("entry")[1], BTNLeft)
	if err := in.TypeText("hi"); err != nil {
		t.Fatalf("type hi: %v", err)
	}
	if _, err := w.Wait("demo", `entry: "hi"`, traceTimeout); err != nil {
		t.Errorf("typed keys never reached the focused entry: %v", err)
	}

	// The text area receives presses (no demo hook fires for it, so
	// assert the routed press itself).
	in.ClickAt(click("textarea")[0], click("textarea")[1], BTNLeft)
	if _, err := w.Wait("input", "over *widget.TextArea", traceTimeout); err != nil {
		t.Errorf("press never routed to the text area: %v", err)
	}

	// A wheel tick over the list must scroll it: assert the offset the
	// showcase reports, not just the routed hover (which names the leaf
	// label inside the viewport and would pass on an ignored wheel). A
	// fresh compositor re-evaluates pointer focus around the text area's
	// click, and axis events sent while it has none are dropped, so the
	// move-and-wheel pair is retried like every other click-and-wait.
	for attempt := range 3 {
		if err := in.MoveOnto(click("scroll")[0], click("scroll")[1]); err != nil {
			t.Fatalf("move onto the list: %v", err)
		}
		if err := in.ScrollAt(click("scroll")[0], click("scroll")[1], 60); err != nil {
			t.Fatalf("wheel the list: %v", err)
		}
		if _, err := w.Wait("demo", "list scrolled to 0,", attemptTimeout); err == nil {
			return
		} else if attempt == 2 {
			t.Errorf("wheel event never scrolled the list: %v", err)
		}
	}
}

// TestHeadlessFingerScroll sends a touchpad scroll through the virtual
// pointer - the finger source with the motion, then the stop - and
// asserts the client saw a precise scroll and its end.
func TestHeadlessFingerScroll(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	in := newInput(t)
	at, ok := centers["scroll"]
	if !ok {
		t.Fatal("no traced center for the list")
	}
	for attempt := range 3 {
		if err := in.MoveOnto(at[0], at[1]); err != nil {
			t.Fatalf("move onto the list: %v", err)
		}
		if err := in.FingerScrollAt(at[0], at[1], 30); err != nil {
			t.Fatalf("finger scroll: %v", err)
		}
		if _, err := w.Wait("input", "route precise scroll", attemptTimeout); err != nil {
			if attempt == 2 {
				t.Fatalf("a finger scroll never reached the client as a precise scroll: %v", err)
			}
			continue
		}
		if _, err := w.Wait("input", "route scroll end", attemptTimeout); err != nil {
			t.Fatalf("the finger lift never reached the client as a scroll end: %v", err)
		}
		return
	}
}

// TestHeadlessPopupGrabOpenAndDismiss covers the context menu: open on
// right-click, dismiss by outside click (the popup grab), dismiss by
// Escape, and keyboard activation through the grab.
func TestHeadlessPopupGrabOpenAndDismiss(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	mark := w.Mark()
	in := newInput(t)
	btn := centers["button"]

	openMenu := func(when string) {
		t.Helper()
		if err := clickUntil(w, in, "demo", btn[0], btn[1], BTNRight, "menu open "); err != nil {
			t.Fatalf("%s: menu never opened: %v", when, err)
		}
		if _, err := w.Wait("input", "popup surface ", traceTimeout); err != nil {
			t.Fatalf("%s: popup surface never created: %v", when, err)
		}
		// A popup whose grab protocol state is wrong (grab after commit,
		// say) gets killed by strict compositors; a rejected grab must
		// never pass for success.
		for _, tr := range w.Tail(50) {
			if strings.Contains(tr.Message, "compositor fatal") {
				t.Fatalf("%s: compositor rejected the popup: %s", when, tr)
			}
		}
	}

	// Outside click while the grab is live dismisses the menu.
	openMenu("first")
	if err := in.ClickAt(10, showcaseH-10, BTNLeft); err != nil {
		t.Fatalf("outside click: %v", err)
	}
	if _, err := w.Wait("input", " dismissed", traceTimeout); err != nil {
		t.Errorf("popup grab never dismissed on outside click: %v", err)
	}
	if _, err := w.Wait("input", " destroyed", traceTimeout); err != nil {
		t.Errorf("the dismissed popup surface was never torn down: %v", err)
	}

	// Escape dismisses too.
	openMenu("second")
	if err := in.Tap(KeyEscape); err != nil {
		t.Fatalf("escape: %v", err)
	}
	if _, err := w.Wait("input", " dismissed", traceTimeout); err != nil {
		t.Errorf("popup never dismissed on escape: %v", err)
	}
	if _, err := w.Wait("input", " destroyed", traceTimeout); err != nil {
		t.Errorf("the escaped popup surface was never torn down: %v", err)
	}

	// Keyboard through the grab: Home selects the first row - not the
	// pointer, whose hover depends on where the compositor enters the
	// popup (sway: under the resting cursor; Hyprland: its corner) -
	// Down moves to the second row and Enter activates it.
	openMenu("third")
	if err := in.Tap(KeyHome); err != nil {
		t.Fatalf("home: %v", err)
	}
	if err := in.Tap(KeyDown); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := in.Tap(KeyEnter); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := w.Wait("demo", "theme light", traceTimeout); err != nil {
		t.Errorf("menu item never activated through the grab: %v", err)
	}
	requireGrabsCarryTheTriggeringSerial(t, w, mark, 3)
}

// TestHeadlessTooltipLifecycle: dwell opens the tooltip, moving on
// closes it, and a new dwell opens the next widget's tooltip. The
// client must survive the whole cycle - a tooltip popup that takes the
// connection down (a second wp_viewport on the popup surface once did
// exactly that) shows up as a dead client and a click that never
// lands, not just as a missing trace.
func TestHeadlessTooltipLifecycle(t *testing.T) {
	requireEnv(t)
	c, w, centers := startShowcase(t)
	in := newInput(t)
	exited := c.Exited()
	btn, entry := centers["button"], centers["entry"]

	alive := func(when string) {
		t.Helper()
		select {
		case <-exited:
			t.Fatalf("client died %s; log tail:\n%s", when, tailTraces(w, 15))
		default:
		}
	}
	assertNoFatal := func(when string) {
		t.Helper()
		for _, tr := range w.Tail(500) {
			if strings.Contains(tr.Message, "compositor fatal") {
				t.Fatalf("compositor rejected the client %s: %s", when, tr)
			}
		}
	}

	if err := in.MoveOnto(btn[0], btn[1]); err != nil {
		t.Fatalf("move to button: %v", err)
	}
	time.Sleep(dwellTime)
	if _, err := w.Wait("input", `tooltip open "increments the counter`, traceTimeout); err != nil {
		t.Errorf("dwell never opened the button tooltip: %v", err)
	}
	alive("after the button tooltip opened")
	assertNoFatal("after the button tooltip opened")

	// Hovering something else closes the old tooltip.
	if err := in.MoveTo(entry[0], entry[1]); err != nil {
		t.Fatalf("move to entry: %v", err)
	}
	if _, err := w.Wait("input", "tooltip closed", traceTimeout); err != nil {
		t.Errorf("tooltip never closed on hover change: %v", err)
	}

	// And the entry earns its own after its dwell.
	time.Sleep(dwellTime)
	if _, err := w.Wait("input", `tooltip open "single-line entry`, traceTimeout); err != nil {
		t.Errorf("dwell never opened the entry tooltip: %v", err)
	}
	alive("after the entry tooltip opened")

	// Input still flows after the tooltip lifecycle: clicking the
	// button through the closing tooltip must activate it.
	mark := w.Mark()
	in.ClickAt(btn[0], btn[1], BTNLeft)
	if _, err := w.Wait("demo", "button clicked 1 times", traceTimeout); err != nil {
		t.Errorf("click after the tooltip lifecycle never reached the button: %v", err)
	}
	alive("after the post-tooltip click")
	assertNoFatal("after the post-tooltip click")
	time.Sleep(2 * dwellTime)
	if n := w.CountSince(mark, "input", "tooltip open"); n != 0 {
		t.Errorf("the pressed button's tooltip came back %d times while the pointer rested on it\n%s", n, tailTraces(w, 25))
	}
}

// tailTraces renders up to n recent traces for failure messages.
func tailTraces(w *LogWatcher, n int) string {
	var b strings.Builder
	for _, tr := range w.Tail(n) {
		b.WriteString("  ")
		b.WriteString(tr.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// TestHeadlessKeyboardTraversal: Tab walks the controls in order and
// Enter/Space/arrows drive them, all through the compositor's real
// keyboard path; Escape closes the window and the client exits 0.
func TestHeadlessKeyboardTraversalAndEscape(t *testing.T) {
	requireEnv(t)
	c, w, _ := startShowcase(t)
	in := newInput(t)
	waitKeyboard(t, w)

	// Tab lands on the button; Enter clicks it.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeyEnter); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := w.Wait("demo", "button clicked 1 times", traceTimeout); err != nil {
		t.Errorf("Tab+Enter never clicked the button: %v", err)
	}

	// Tab reaches the slider; the right arrow raises it.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeyRight); err != nil {
		t.Fatalf("right: %v", err)
	}
	if _, err := w.Wait("demo", "slider at ", traceTimeout); err != nil {
		t.Errorf("arrow key never moved the focused slider: %v", err)
	}

	// Tab reaches the switch; Enter toggles it off.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeyEnter); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := w.Wait("demo", "switch false", traceTimeout); err != nil {
		t.Errorf("Enter never toggled the focused switch: %v", err)
	}

	// Tab reaches the checkbox; Space toggles it on.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeySpace); err != nil {
		t.Fatalf("space: %v", err)
	}
	if _, err := w.Wait("demo", "checkbox true", traceTimeout); err != nil {
		t.Errorf("Space never toggled the focused checkbox: %v", err)
	}

	// Escape closes the window and the showcase exits cleanly.
	if err := in.Tap(KeyEscape); err != nil {
		t.Fatalf("escape: %v", err)
	}
	select {
	case <-c.Exited():
		if err := c.Wait(); err != nil {
			t.Errorf("escape did not close the showcase cleanly: %v\nlog tail:\n%s", err, tailTraces(w, 15))
		}
	case <-time.After(traceTimeout):
		t.Errorf("escape did not close the showcase within %s; log tail:\n%s", traceTimeout, tailTraces(w, 25))
	}
}

// TestHeadlessPointerOnlySeat runs the tooltip lifecycle and a click
// on a keyboard-less seat: the shape that once killed the client
// outright. Hover surfaces must open, the client must live through
// them, and the pointer must keep working afterwards.
func TestHeadlessPointerOnlySeat(t *testing.T) {
	requireEnv(t)
	c, w, centers := startShowcase(t)
	in := newPointerOnlyInput(t)
	exited := c.Exited()
	btn := centers["button"]

	if err := in.MoveOnto(btn[0], btn[1]); err != nil {
		t.Fatalf("move to button: %v", err)
	}
	time.Sleep(dwellTime)
	if _, err := w.Wait("input", `tooltip open "increments the counter`, traceTimeout); err != nil {
		t.Errorf("tooltip never opened on a pointer-only seat: %v", err)
	}
	select {
	case <-exited:
		t.Fatalf("client died when the tooltip opened; log tail:\n%s", tailTraces(w, 15))
	default:
	}

	// The pointer still works after the tooltip cycle.
	if err := in.ClickAt(btn[0], btn[1], BTNLeft); err != nil {
		t.Fatalf("click: %v", err)
	}
	if _, err := w.Wait("demo", "button clicked 1 times", traceTimeout); err != nil {
		t.Errorf("click never reached the button on a pointer-only seat: %v", err)
	}
	select {
	case <-exited:
		t.Fatalf("client died before the test ended; log tail:\n%s", tailTraces(w, 15))
	default:
	}
}

// TestHeadlessClipboardRoundtrip: type into the entry, copy the
// selection to the compositor clipboard, paste it back - the text only
// doubles if the client's set_selection and the compositor's offer
// pipeline both work.
func TestHeadlessClipboardRoundtrip(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	in := newInput(t)
	waitKeyboard(t, w)
	entry := centers["entry"]

	// Focusing the entry is a press, and a fresh compositor can eat
	// it; the typed text is the proof of focus, so retry the pair and
	// let the assert name the failure if focus never took.
	for attempt := range 3 {
		if err := in.ClickAt(entry[0], entry[1], BTNLeft); err != nil {
			t.Fatalf("focus entry: %v", err)
		}
		if err := in.TypeText("abc"); err != nil {
			t.Fatalf("type abc: %v", err)
		}
		if _, err := w.Wait("demo", `entry: "abc"`, attemptTimeout); err == nil {
			break
		} else if attempt == 2 {
			t.Fatalf("typing never reached the entry: %v", err)
		}
	}

	// The copy->offer->paste pipeline crosses the compositor's
	// clipboard and can land late under load. Each attempt starts from
	// a fresh "abc" (select all, retype), so a late paste of an earlier
	// attempt cannot leave text the doubled match never sees again.
	for attempt := range 3 {
		if attempt > 0 {
			if err := in.Combo(KeyA); err != nil {
				t.Fatalf("ctrl+a: %v", err)
			}
			if err := in.TypeText("abc"); err != nil {
				t.Fatalf("retype abc: %v", err)
			}
			if _, err := w.Wait("demo", `entry: "abc"`, attemptTimeout); err != nil {
				t.Fatalf("the entry never reset to abc: %v", err)
			}
		}
		if err := in.Combo(KeyA); err != nil { // select all
			t.Fatalf("ctrl+a: %v", err)
		}
		if err := in.Combo(KeyC); err != nil { // copy
			t.Fatalf("ctrl+c: %v", err)
		}
		if err := in.Tap(KeyEnd); err != nil { // collapse to end-of-text
			t.Fatalf("end: %v", err)
		}
		if err := in.Combo(KeyV); err != nil { // paste
			t.Fatalf("ctrl+v: %v", err)
		}
		if _, err := w.Wait("demo", `entry: "abcabc"`, attemptTimeout); err == nil {
			return
		} else if attempt == 2 {
			t.Errorf("clipboard roundtrip failed: text never came back doubled: %v", err)
		}
	}
}

// startStatesClient launches the window-state client (cmd/lys-ui-states)
// with demo (the client's confirmed-state traces) and shell (the
// window's own state-change traces) on, and attaches a watcher.
func startStatesClient(t *testing.T) (*Client, *LogWatcher) {
	t.Helper()
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-states", "lys-ui-states")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "input,frame,demo,shell")
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	waitPresented(t, w)
	return c, w
}

// TestHeadlessWindowStates is the acceptance proof for the window
// state model (#47) against a live compositor: each chord's request is
// judged only by the state the compositor CONFIRMS in a configure -
// including a refusal (sway, like i3, ignores set_maximized on a
// floating window, and the reported state must stay what was confirmed
// before) - the maximize/fullscreen confirmations arrive as
// output-sized configures whose frames draw at the new size through the
// one relayout path, and a compositor-initiated close while fullscreen
// tears the client down cleanly.
func TestHeadlessWindowStates(t *testing.T) {
	requireEnv(t)
	c, w := startStatesClient(t)
	exited := c.Exited()
	var in *VirtualInput
	tap := func(rs ...rune) {
		t.Helper()
		for _, r := range rs {
			code, _, err := KeyFor(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := in.Tap(code); err != nil {
				t.Fatalf("tap %q: %v", r, err)
			}
		}
	}
	// tapUntil taps r until the expected effect traces appear (each
	// substr matched by one line, in any order), retrying a few times:
	// synthetic input on a loaded compositor can be lost, and the
	// assertions here judge confirmed state, not first-attempt delivery.
	// Halfway through, the virtual seat is re-dialed: a fresh keyboard
	// re-arms focus delivery when the old device stops producing keys.
	tapUntil := func(r rune, want ...string) {
		t.Helper()
		for attempt := 0; ; attempt++ {
			if attempt == 4 {
				t.Fatalf("tap %q never produced %v: %s", r, want, tailTraces(w, 15))
			}
			if attempt == 2 {
				in = newInput(t)
				waitKeyboard(t, w)
			}
			tap(r)
			if err := w.WaitAll("", attemptTimeout, want...); err == nil {
				return
			}
		}
	}
	// Confirmed-state traces read "state maximized=<bool> fullscreen=<bool>
	// <w>x<h>" (flag-form, so one bit matches regardless of the tiled_*/
	// activated bits sway carries alongside); the poll chord reads
	// "polled ...". The waits match the bare flag pair, since maximized=
	// always precedes fullscreen= in the print order.

	// The recipe pins the client to a floating 420x280 window; that size
	// in the first confirmed-state trace is the pin assertion. The
	// virtual seat is dialed only after the map, so keyboard delivery
	// starts from a mapped, focused surface.
	if _, err := w.Wait("demo", "420x280", traceTimeout); err != nil {
		t.Fatalf("states client never mapped at the pinned %dx%d: %v", statesW, statesH, err)
	}
	in = newInput(t)
	// The maximize request is judged by what the compositor confirms,
	// whichever way it goes: the on-demand poll must read the confirmed
	// state. sway 1.11 refuses set_maximized on a floating window - its
	// handler schedules a configure carrying the state UNCHANGED, the
	// issue's rule that such a configure reports the request
	// unconfirmed - and that refusal stays pinned (update this test if
	// sway changes). Hyprland honors it; the window then unmaximizes
	// before the fullscreen steps below.
	tapUntil('m', "requested maximize")
	_, err := w.Wait("demo", "state maximized=true", 2*time.Second)
	honored := err == nil
	if honored && testEnv.Compositor().Name() == "sway" {
		t.Errorf("the maximize was confirmed; the suite expected sway to refuse it")
	}
	tapUntil('p', fmt.Sprintf("polled maximized=%v", honored))
	if honored {
		// Whether the compositor confirms the unmaximize is its call
		// (requests are hints); the poll below reads whatever holds.
		tapUntil('n', "requested unmaximize")
	}
	// Fullscreen from floating: the output-sized configure is a REAL
	// relayout (420x280 -> 1280x800), so the first frame drawn at the
	// fullscreen size is the live proof of the syncSize path.
	tapUntil('f', "fullscreen=true", fmt.Sprintf("draw %dx%d", outputW, outputH))
	select {
	case <-exited:
		t.Fatalf("client died during the fullscreen transition; log tail:\n%s", tailTraces(w, 15))
	default:
	}

	// Unfullscreen: the size the window comes back to is compositor
	// policy (sway keeps the floating container at the size the
	// fullscreen surface committed and configures it explicitly;
	// Hyprland restores its float geometry, keeping a confirmed
	// maximize flag) - what the client owes is laying out and drawing
	// at whatever is confirmed.
	tapUntil('g', "fullscreen=false")
	size := pollSize(t, tap, w)
	if err := w.WaitEver("frame", "draw "+size, traceTimeout); err != nil {
		t.Errorf("never drew at the confirmed %s after unfullscreen: %v", size, err)
	}

	// Close while fullscreen: fullscreen again, then the COMPOSITOR
	// closes the window (sway kill, the real xdg_toplevel.close event).
	// The client must tear down cleanly - no panic, exit 0.
	tapUntil('f', "fullscreen=true")
	if err := testEnv.CloseWindow(StatesAppID); err != nil {
		t.Fatalf("compositor close: %v", err)
	}
	select {
	case <-exited:
		if err := c.Wait(); err != nil {
			t.Errorf("close while fullscreen did not exit cleanly: %v\nlog tail:\n%s", err, tailTraces(w, 15))
		}
	case <-time.After(traceTimeout):
		t.Errorf("client outlived the compositor close while fullscreen; log tail:\n%s", tailTraces(w, 25))
	}
}

// startMultilistClient launches the multi-select list client
// (cmd/lys-ui-multilist) and returns it with a trace watcher and the row
// centers the client traced after mapping at the pinned size.
func startMultilistClient(t *testing.T) (*Client, *LogWatcher, [][2]int) {
	t.Helper()
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-multilist", "lys-ui-multilist")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "input,frame,demo,shell")
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", fmt.Sprintf("mapped %dx%d", multilistW, multilistH), traceTimeout); err != nil {
		t.Fatalf("multilist never mapped at the pinned %dx%d: %v", multilistW, multilistH, err)
	}
	rows := make([][2]int, multilistRows)
	for i := range rows {
		tr, err := w.Wait("demo", fmt.Sprintf("row %d center ", i), traceTimeout)
		if err != nil {
			t.Fatalf("row %d center trace missing: %v", i, err)
		}
		var x, y int
		if _, err := fmt.Sscanf(tr.Message, fmt.Sprintf("row %d center (%%d,%%d)", i), &x, &y); err != nil {
			t.Fatalf("bad row trace %q: %v", tr.Message, err)
		}
		rows[i] = [2]int{x, y}
	}
	waitPresented(t, w)
	return c, w, rows
}

// TestHeadlessListMultiSelect drives the multiple-selection list model
// (#60) through the compositor: clicks toggle, ctrl+space toggles the
// cursor row, shift+arrows extend from the anchor, ctrl+a selects
// everything, a pointer drag rubber-bands a range, and a rapid second
// click activates the row instead of toggling it back out.
func TestHeadlessListMultiSelect(t *testing.T) {
	requireEnv(t)
	_, w, rows := startMultilistClient(t)
	in := newInput(t)
	waitKeyboard(t, w)

	// Click row 2: the toggle lands through the compositor's pointer
	// path and focuses the list for the keyboard half of the test.
	if err := clickUntil(w, in, "demo", rows[2][0], rows[2][1], BTNLeft, "selection [2]"); err != nil {
		t.Fatalf("click on row 2 never toggled it in: %v", err)
	}

	// ctrl+space toggles the cursor row back out, then in.
	for pass := range 2 {
		expect := "selection [none]"
		if pass == 1 {
			expect = "selection [2]"
		}
		if err := in.Combo(KeySpace); err != nil {
			t.Fatalf("ctrl+space: %v", err)
		}
		if _, err := w.Wait("demo", expect, traceTimeout); err != nil {
			t.Errorf("ctrl+space pass %d never produced %q: %v", pass+1, expect, err)
		}
	}

	// shift+Down extends the anchor range.
	if err := in.mods(ModShift); err != nil {
		t.Fatalf("shift: %v", err)
	}
	if err := in.Tap(KeyDown); err != nil {
		t.Fatalf("shift+down: %v", err)
	}
	if err := in.mods(0); err != nil {
		t.Fatalf("shift release: %v", err)
	}
	if _, err := w.Wait("demo", "selection [2,3]", traceTimeout); err != nil {
		t.Errorf("shift+down never extended the anchor range: %v", err)
	}

	// ctrl+a takes everything through the select-all route.
	if err := in.Combo(KeyA); err != nil {
		t.Fatalf("ctrl+a: %v", err)
	}
	if _, err := w.Wait("demo", "selection [0,1,2,3,4,5,6,7,8,9]", traceTimeout); err != nil {
		t.Errorf("ctrl+a never selected everything: %v", err)
	}

	// A pointer drag rubber-bands a range and reports it once.
	if err := in.DragTo(rows[5][0], rows[5][1], rows[7][0], rows[7][1], 4); err != nil {
		t.Fatalf("drag rows 5..7: %v", err)
	}
	if _, err := w.Wait("demo", "selection [5,6,7]", traceTimeout); err != nil {
		t.Errorf("the drag never rubber-banded [5,6,7]: %v", err)
	}

	// A rapid second click on a row activates it (double-click opens)
	// without toggling its membership back out: the first click removes
	// row 6 from the set, the pair then activates it.
	if err := in.DoubleClickAt(rows[6][0], rows[6][1], BTNLeft); err != nil {
		t.Fatalf("double click on row 6: %v", err)
	}
	if err := w.WaitAll("demo", traceTimeout, "selection [5,7]", "activated 6"); err != nil {
		t.Errorf("the rapid pair did not toggle row 6 out and activate it: %v", err)
	}
}

// dialogX is the recipe's floating pin for the multilist client's
// modal dialog: beside the parent window at x=340.
const dialogX = 340

// TestHeadlessDialogModality drives the dialog-modality lifecycle
// (#61) through the headless compositor: a parented modal dialog
// blocks input to the parent window while it is open, and responding
// releases the block. sway (1.12) does not implement xdg-dialog-v1, so
// the gate exercises the fallback path — application-level modality,
// the floor every compositor gets — and pins that the session degrades
// silently without the global. The protocol's own wire contract
// (get_xdg_dialog, set_modal, unset on close, silence without a
// manager) is pinned by TestDialogModalityRequests in internal/window.
// When sway grows the global, flip the negative wait below and assert
// the live binding alongside the lifecycle.
func TestHeadlessDialogModality(t *testing.T) {
	requireEnv(t)
	c, w, rows := startMultilistClient(t)
	in := newInput(t)
	waitKeyboard(t, w)
	exited := c.Exited()

	// sway does not advertise xdg_wm_dialog_v1: the session must
	// degrade silently — no binding trace, no error, modality falls
	// back to the application-level block.
	if _, err := w.Wait("shell", "xdg-dialog-v1 bound", time.Second); err == nil {
		t.Log("sway now advertises xdg-dialog-v1; flip this test to assert the live binding")
	} else {
		for _, tr := range w.Tail(80) {
			if strings.Contains(tr.Message, "compositor fatal") {
				t.Fatalf("the session treated the missing dialog manager as fatal: %s", tr)
			}
		}
	}

	// Before any dialog, row clicks land.
	if err := clickUntil(w, in, "demo", rows[1][0], rows[1][1], BTNLeft, "selection [1]"); err != nil {
		t.Fatalf("click on row 1 never toggled it in: %v", err)
	}

	// d opens the modal dialog; it maps and takes focus.
	if err := in.Tap(KeyD); err != nil {
		t.Fatalf("tap d: %v", err)
	}
	if _, err := w.Wait("demo", "dialog open 1", traceTimeout); err != nil {
		t.Fatalf("d never opened the dialog: %v", err)
	}

	// While the dialog is modal, the parent's input is blocked: a
	// click on a row changes nothing, and the parent's own key (d)
	// stays silent because focus sits on the dialog.
	if err := in.ClickAt(rows[2][0], rows[2][1], BTNLeft); err != nil {
		t.Fatalf("blocked click: %v", err)
	}
	if err := in.Tap(KeyD); err != nil {
		t.Fatalf("blocked d: %v", err)
	}
	if _, err := w.Wait("demo", "selection", 700*time.Millisecond); err == nil {
		t.Error("a selection change traced while the dialog was modal")
	}
	for _, tr := range w.Tail(30) {
		if strings.Contains(tr.Message, "dialog open 2") {
			t.Error("the parent saw keyboard input while the dialog was modal")
		}
	}
	select {
	case <-exited:
		t.Fatal("client died with a modal dialog open")
	default:
	}

	// Focus the dialog by clicking its chrome (sway does not move
	// focus to a mapping dialog on its own), then Enter responds with
	// the default and closes it. The dialog floats at the recipe's
	// (340,0) pin beside the parent, so the parent stays clickable.
	if err := in.ClickAt(dialogX+10, 8, BTNLeft); err != nil {
		t.Fatalf("focus click on the dialog: %v", err)
	}
	if err := in.Tap(KeyEnter); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := w.Wait("demo", "dialog response ok", traceTimeout); err != nil {
		t.Fatalf("Enter never responded to the dialog: %v", err)
	}
	if _, err := w.Wait("demo", "dialog component shut down", traceTimeout); err != nil {
		t.Errorf("the dialog's component outlived the dialog: %v", err)
	}

	// The block is gone: the same row click that died while modal now
	// lands, and d opens a second dialog.
	if err := clickUntil(w, in, "demo", rows[2][0], rows[2][1], BTNLeft, "selection [1,2]"); err != nil {
		t.Errorf("input to the parent never recovered after the close: %v", err)
	}
	if err := in.Tap(KeyD); err != nil {
		t.Fatalf("tap d again: %v", err)
	}
	if _, err := w.Wait("demo", "dialog open 2", traceTimeout); err != nil {
		t.Errorf("d never reached the parent after the close: %v", err)
	}
}

// TestHeadlessPopoverTakesTypingAndRepaints drives a popover the
// application loop owns: its focused Entry takes typing through the
// popup grab, its content repaints while the app changes it from a
// timer (no input arriving), and Esc dismisses it with focus inside.
func TestHeadlessPopoverTakesTypingAndRepaints(t *testing.T) {
	requireEnv(t)
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-popover", "lys-ui-popover")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "input,frame,demo")
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	mark := w.Mark()
	if _, err := w.Wait("demo", fmt.Sprintf("mapped %dx%d", popoverW, popoverH), traceTimeout); err != nil {
		t.Fatalf("popover client never mapped: %v", err)
	}
	tr, err := w.Wait("demo", "control open center ", traceTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var x, y int
	if _, err := fmt.Sscanf(tr.Message, "control open center (%d,%d)", &x, &y); err != nil {
		t.Fatalf("bad trace %q: %v", tr.Message, err)
	}
	in := newInput(t)
	if err := clickUntil(w, in, "demo", x, y, BTNLeft, "popover open"); err != nil {
		t.Fatalf("popover never opened: %v", err)
	}
	requireGrabsCarryTheTriggeringSerial(t, w, mark, 1)
	// The layer takes the keyboard as the popover opens.
	waitKeyboard(t, w)
	// The open popover's first frame is the size to grow past.
	first, err := w.Wait("frame", " committed ", traceTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var id, fw, fh int
	if _, err := fmt.Sscanf(first.Message, "popup %d committed %dx%d", &id, &fw, &fh); err != nil {
		t.Fatalf("bad frame trace %q: %v", first.Message, err)
	}
	if err := in.TypeText("hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", "popover text hi", traceTimeout); err != nil {
		t.Errorf("typing never reached the focused entry: %v\n%s", err, tailTraces(w, 25))
	}
	// The typing adds a row, growing the popover
	// (xdg_popup.reposition): a later frame is taller.
	if _, err := w.Wait("demo", "popover grew", traceTimeout); err != nil {
		t.Fatal(err)
	}
	grew := false
	for range 20 {
		tr, err := w.Wait("frame", " committed ", traceTimeout)
		if err != nil {
			break
		}
		var gid, gw, gh int
		if _, err := fmt.Sscanf(tr.Message, "popup %d committed %dx%d", &gid, &gw, &gh); err == nil && gid == id && gh > fh {
			grew = true
			break
		}
	}
	if !grew {
		t.Errorf("the popover never grew past %dx%d with its content\n%s", fw, fh, tailTraces(w, 25))
	}
	// The ticker changes the label every 100ms with no input arriving:
	// an app-loop popover paints each change.
	for i := range 3 {
		if _, err := w.Wait("frame", " committed ", traceTimeout); err != nil {
			t.Fatalf("popover frame %d after the timer change never painted: %v\n%s", i, err, tailTraces(w, 25))
		}
	}
	if err := in.Tap(KeyEscape); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", "popover closed", traceTimeout); err != nil {
		t.Errorf("Esc with the entry focused never dismissed: %v", err)
	}
}

// TestHeadlessNestedMenuPopover drives a menu whose submenu is a
// popover nested under the root popover (an xdg_popup child of a
// popup) through a real compositor: Right opens the submenu with the
// keyboard, the nested popover takes the keys, Enter on its leaf runs
// it and closes the whole chain, child before parent, and the client
// lives on to open the menu again.
func TestHeadlessNestedMenuPopover(t *testing.T) {
	requireEnv(t)
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-popover", "lys-ui-popover")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "input,demo")
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	mark := w.Mark()
	tr, err := w.Wait("demo", "control menu center ", traceTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var x, y int
	if _, err := fmt.Sscanf(tr.Message, "control menu center (%d,%d)", &x, &y); err != nil {
		t.Fatalf("bad trace %q: %v", tr.Message, err)
	}
	in := newInput(t)
	for round := range 2 {
		if err := clickUntil(w, in, "demo", x, y, BTNLeft, "menu open"); err != nil {
			t.Fatalf("round %d: the menu never opened: %v", round, err)
		}
		if _, err := w.Wait("demo", "menu depth 1", traceTimeout); err != nil {
			t.Fatal(err)
		}
		waitKeyboard(t, w) // the layer takes the keyboard as the menu opens
		for _, key := range []uint32{KeyDown, KeyRight} {
			if err := in.Tap(key); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := w.Wait("demo", "menu depth 2", traceTimeout); err != nil {
			t.Fatalf("round %d: Right never opened the nested popover: %v\n%s", round, err, tailTraces(w, 25))
		}
		for _, key := range []uint32{KeyDown, KeyEnter} {
			if err := in.Tap(key); err != nil {
				t.Fatal(err)
			}
		}
		// The chain closes first, then the leaf acts.
		if _, err := w.Wait("demo", "menu closed", traceTimeout); err != nil {
			t.Fatalf("round %d: the leaf did not close the chain: %v\n%s", round, err, tailTraces(w, 25))
		}
		if _, err := w.Wait("demo", "menu leaf a", traceTimeout); err != nil {
			t.Fatalf("round %d: the nested popover never took the keys: %v", round, err)
		}
		if _, err := w.Wait("demo", "menu depth 0", traceTimeout); err != nil {
			t.Fatal(err)
		}
	}
	requireGrabsCarryTheTriggeringSerial(t, w, mark, 4)
}

// revealShowcase wheels the showcase's root scroll until every traced
// control named in names is on screen at once, returning the vertical
// offset to subtract from a traced center to get window coordinates
// (centers are traced at map time, offset zero, in content coordinates).
// The wheel rests over the left column, where nothing consumes the
// axis before the root scroll. A lost tick is retried; each tick is one
// wheel step, so the offset moves in small increments both ways.
func revealShowcase(t *testing.T, w *LogWatcher, in *VirtualInput, centers map[string][2]int, names ...string) int {
	t.Helper()
	deep, shallow := 0, 1<<30
	for _, name := range names {
		c, ok := centers[name]
		if !ok {
			t.Fatalf("no traced center for %s", name)
		}
		deep, shallow = max(deep, c[1]), min(shallow, c[1])
	}
	off := 0
	for range 40 {
		if shallow-off >= 16 && deep-off <= showcaseH-16 {
			return off
		}
		dy := 10.0
		if shallow-off < 16 {
			dy = -10
		}
		if err := in.ScrollAt(150, 200, dy); err != nil {
			t.Fatalf("wheel the root scroll: %v", err)
		}
		tr, err := w.Wait("demo", "root scrolled to 0,", attemptTimeout)
		if err != nil {
			continue
		}
		var x, parsed int
		if _, err := fmt.Sscanf(tr.Message, "root scrolled to %d,%d", &x, &parsed); err == nil {
			off = parsed
		}
	}
	t.Fatalf("the root scroll never brought %v on screen (offset %d):\n%s", names, off, tailTraces(w, 15))
	return 0
}

const showcaseLastListRow = "charlie"

// TestHeadlessSixthPassListBoxAndFixed drives the sixth-pass widgets
// through the compositor: the ListBox selects and activates a row on a
// plain click, its filter and sort toggles re-filter and re-sort the
// rows, the keyboard moves the selection, and the Fixed's button moves
// its positioned child. The stylesheet's @media rule and ::before/
// ::after content are asserted from the client's own trace, so the CSS
// at-rule and pseudo-element paths run on a live session too.
func TestHeadlessSixthPassListBoxAndFixed(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	in := newInput(t)
	waitKeyboard(t, w)

	// The 640-wide window satisfied the min-width branch, and both
	// pseudo-elements of the sixth-pass heading resolved their content.
	if err := w.WaitEver("demo", "media viewport "+fmt.Sprintf("%dx%d", showcaseW, showcaseH), traceTimeout); err != nil {
		t.Errorf("the window size never reached the stylesheet engine: %v", err)
	}
	if err := w.WaitEver("demo", `css before "sixth " after " pass"`, traceTimeout); err != nil {
		t.Errorf("the @media ::before or the plain ::after never resolved content: %v", err)
	}

	off := revealShowcase(t, w, in, centers, "listbox", "listfilter", "listsort", "fixedbtn")
	at := func(name string) (int, int) {
		t.Helper()
		c, ok := centers[name]
		if !ok {
			t.Fatalf("no traced center for %s", name)
		}
		return c[0], c[1] - off
	}

	// One click on a row selects it and - single-click activate is on
	// by default - activates it too.
	x, y := at("listbox")
	if err := clickUntil(w, in, "demo", x, y, BTNLeft, "listbox select "); err != nil {
		t.Errorf("a click never selected a listbox row: %v", err)
	}
	if _, err := w.Wait("demo", "listbox activate ", traceTimeout); err != nil {
		t.Errorf("the clicked listbox row never activated: %v", err)
	}

	// The click left the row holding focus: an arrow moves the selection
	// through the compositor's keyboard path - away from whichever end
	// the traced center happened to land on. This must run before the
	// toggles below, whose clicks take the focus back.
	tr, err := w.Wait("demo", "listbox select ", traceTimeout)
	if err != nil {
		t.Fatalf("the selected row never traced: %v", err)
	}
	arrow := uint32(KeyDown)
	if strings.HasSuffix(tr.Message, showcaseLastListRow) {
		arrow = KeyUp
	}
	if err := in.Tap(arrow); err != nil {
		t.Fatalf("arrow: %v", err)
	}
	if err := w.WaitAll("demo", traceTimeout, "listbox select "); err != nil {
		t.Errorf("the arrow key never moved the listbox selection: %v", err)
	}

	// The filter toggle hides the long rows, the sort toggle orders the
	// rest, and a second filter toggle brings them back.
	x, y = at("listfilter")
	if err := clickUntil(w, in, "demo", x, y, BTNLeft, "listbox filter true"); err != nil {
		t.Errorf("the filter toggle never filtered the listbox: %v", err)
	}
	x, y = at("listsort")
	if err := clickUntil(w, in, "demo", x, y, BTNLeft, "listbox sort true"); err != nil {
		t.Errorf("the sort toggle never sorted the listbox: %v", err)
	}
	x, y = at("listfilter")
	if err := clickUntil(w, in, "demo", x, y, BTNLeft, "listbox filter false"); err != nil {
		t.Errorf("the filter toggle never lifted the filter: %v", err)
	}

	// The Fixed's button moves its positioned child 20px left, in
	// place, without detaching it.
	x, y = at("fixedbtn")
	if err := clickUntil(w, in, "demo", x, y, BTNLeft, "fixed moved to (76,6)"); err != nil {
		t.Errorf("the fixed child never moved: %v", err)
	}
}

// typedSelect clicks the typed list until a selection trace lands and
// returns the index the selected item's name carries, so a test can
// tell recycled rows apart by what they were rebound to.
func typedSelect(t *testing.T, w *LogWatcher, in *VirtualInput, x, y int) int {
	t.Helper()
	for attempt := range 3 {
		mark := w.Mark()
		if err := in.ClickAt(x, y, BTNLeft); err != nil {
			t.Fatalf("click the typed list: %v", err)
		}
		tr, err := w.Wait("demo", "typed select item-", attemptTimeout)
		if err != nil && w.SeenSince(mark, "input", "press at") {
			tr, err = w.Wait("demo", "typed select item-", traceTimeout)
		}
		if err != nil {
			if attempt == 2 {
				t.Fatalf("a click on the typed list never selected a row: %v", err)
			}
			continue
		}
		var i int
		if _, err := fmt.Sscanf(tr.Message, "typed select item-%d", &i); err != nil {
			t.Fatalf("bad typed selection trace %q: %v", tr.Message, err)
		}
		return i
	}
	return -1
}

// TestHeadlessSixthPassTypedListSurfaceAndOpenButton covers the rest
// of the sixth pass under the compositor: the DrawSurface keeps
// painting incremental patches off its timer (a cadence floor, never
// an absolute rate), the TypedList recycles rows as the wheel scrolls
// its viewport (a row far past the first viewport still selects, and
// reports an item the scroll brought in), the open button's recents
// dropdown takes its press, the button opens the file dialog (a real
// second toplevel), and after Escape answers it the parent's input
// works again.
func TestHeadlessSixthPassTypedListSurfaceAndOpenButton(t *testing.T) {
	requireEnv(t)
	c, w, centers := startShowcase(t)
	in := newInput(t)
	waitKeyboard(t, w)

	off := revealShowcase(t, w, in, centers, "surface", "typedlist", "typedsort", "openfile", "openrecents")
	at := func(name string) (int, int) {
		t.Helper()
		cc, ok := centers[name]
		if !ok {
			t.Fatalf("no traced center for %s", name)
		}
		return cc[0], cc[1] - off
	}

	// Hovering the draw surface routes to the surface leaf, and the
	// timer keeps drawing: one more patch within a few ticks' budget is
	// the floor, not a frame-rate assertion.
	sx, sy := at("surface")
	if err := hoverUntil(w, in, "input", sx, sy, "*widget.DrawSurface"); err != nil {
		t.Errorf("hovering the draw surface never routed to it: %v", err)
	}
	if err := w.WaitAll("demo", 4*time.Second, "surface drew "); err != nil {
		t.Errorf("the draw surface stopped painting on its timer: %v", err)
	}

	// Select a row, scroll the viewport a few wheel ticks, select again:
	// the recycled row reports an item from deep in the model.
	tx, ty := at("typedlist")
	first := typedSelect(t, w, in, tx, ty)
	second := first
	for range 3 {
		for range 4 {
			if err := in.ScrollAt(tx, ty, 30); err != nil {
				t.Fatalf("wheel the typed list: %v", err)
			}
		}
		if second = typedSelect(t, w, in, tx, ty); second > first+4 {
			break
		}
	}
	if second <= first+4 {
		t.Errorf("after scrolling, the clicked row still reported item %d, want one past the first viewport (first %d)", second, first)
	}

	// The typed list's own sort toggle orders the model.
	x, y := at("typedsort")
	if err := clickUntil(w, in, "demo", x, y, BTNLeft, "typed sort true"); err != nil {
		t.Errorf("the typed sort toggle never sorted the list: %v", err)
	}

	// The recents dropdown half of the open button takes the press.
	rx, ry := at("openrecents")
	beforeRecents := w.Mark()
	var dropdownErr error
	for range 3 {
		if err := in.ClickAt(rx, ry, BTNLeft); err != nil {
			t.Fatalf("click the recents dropdown: %v", err)
		}
		dropdownErr = w.WaitAll("input", attemptTimeout, "over *widget.Dropdown")
		if dropdownErr == nil {
			break
		}
	}
	if dropdownErr != nil {
		t.Errorf("the recents dropdown never took its press: %v", dropdownErr)
	}
	// With recent files to list, the press opens them in a popover
	// (#280), which a click elsewhere would only dismiss: Escape closes
	// it before the button half is clicked. Without any, the dropdown
	// stays shut.
	if w.WaitSince(beforeRecents, "input", "popup grab", attemptTimeout) == nil {
		if err := in.Tap(KeyEscape); err != nil {
			t.Fatalf("escape the recents list: %v", err)
		}
		if err := w.WaitSince(beforeRecents, "input", "destroyed", traceTimeout); err != nil {
			t.Errorf("Escape never closed the recents list: %v", err)
		}
	}

	// The button half opens the file picker: a second toplevel maps, so
	// the compositor confirms a configure for it.
	x, y = at("openfile")
	if err := clickUntil(w, in, "shell", x, y, BTNLeft, "toplevel state"); err != nil {
		t.Fatalf("the open button never opened the file dialog: %v", err)
	}
	for _, tr := range w.Tail(80) {
		if strings.Contains(tr.Message, "compositor fatal") {
			t.Fatalf("the compositor rejected the file dialog: %s", tr)
		}
	}
	select {
	case <-c.Exited():
		t.Fatalf("the client died with its file dialog open; log tail:\n%s", tailTraces(w, 15))
	default:
	}

	// Escape answers the dialog with cancel, closing it, and the modal
	// block lifts with it: the typed list click that follows must land.
	if err := in.Tap(KeyEscape); err != nil {
		t.Fatalf("escape: %v", err)
	}
	tx, ty = at("typedlist")
	if err := clickUntil(w, in, "demo", tx, ty, BTNLeft, "typed select item-"); err != nil {
		t.Errorf("the parent never took input again after the dialog closed: %v", err)
	}
}

// Both cursor paths: with cursor-shape-v1 (the compositor draws) a
// text field's caret cursor is one set_shape; with the protocol turned
// off it comes from the client theme.
func TestHeadlessCursorShapePaths(t *testing.T) {
	requireEnv(t)
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"protocol", nil, `cursor shape "xterm"`},
		{"theme", []string{"LYS_UI_NO_CURSOR_SHAPE=1"}, `cursor "xterm"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, w, centers := startShowcaseEnv(t, tc.env...)
			in := newInput(t)
			entry := centers["entry"]
			if err := hoverUntil(w, in, "input", entry[0], entry[1], tc.want); err != nil {
				t.Errorf("hovering the entry never traced %s: %v", tc.want, err)
			}
		})
	}
}

// Idle notification: the compositor reports the seat idle after the
// requested quiet time, and input resumes it.
func TestHeadlessIdleNotify(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcaseEnv(t, "LYS_UI_DEMO_IDLE_MS=400")
	if _, err := w.Wait("demo", "idle", traceTimeout); err != nil {
		t.Fatalf("the seat never went idle: %v", err)
	}
	in := newInput(t)
	btn := centers["button"]
	if err := in.MoveTo(btn[0], btn[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", "resumed", traceTimeout); err != nil {
		t.Errorf("input did not resume the seat: %v", err)
	}
}

// TestHeadlessToplevelExport pins #160 on a real compositor: the
// session binds xdg-foreign v2 and every toplevel receives the handle
// portals take as its parent_window.
func TestHeadlessToplevelExport(t *testing.T) {
	requireEnv(t)
	_, w, _ := startMultilistClient(t)
	if err := w.WaitEver("shell", "xdg-foreign exporter bound", traceTimeout); err != nil {
		t.Fatalf("the session never bound zxdg_exporter_v2: %v", err)
	}
	if err := w.WaitEver("shell", "toplevel exported as", traceTimeout); err != nil {
		t.Fatalf("the toplevel never received an xdg-foreign handle: %v", err)
	}
	if w.SeenSince(0, "shell", `toplevel exported as ""`) {
		t.Error("the compositor sent an empty handle")
	}
}

func commitsWithin(w *LogWatcher, d time.Duration) int {
	mark := w.Mark()
	time.Sleep(d)
	return w.CountSince(mark, "frame", "frame committed")
}

// TestHeadlessVideoPlays pins #149 on a real compositor: the video
// demo's generated clip autoplays, presenting frames continuously off
// the media frame clock, and Space pauses it until presentation goes
// quiet.
func TestHeadlessVideoPlays(t *testing.T) {
	requireEnv(t)
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-video", "lys-ui-video")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "input,frame,shell")
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	in := newInput(t)
	waitKeyboard(t, w)
	if err := w.WaitEver("frame", "frame committed", traceTimeout); err != nil {
		t.Fatalf("the video never presented: %v", err)
	}
	mark := w.Mark()
	time.Sleep(time.Second)
	n := w.CountSince(mark, "frame", "frame committed")
	gap := w.MaxGapSince(mark, "frame", "frame committed")
	if n < 8 || gap > 0.5 {
		t.Fatalf("%d frames presented in a second of playback with a %.2fs longest pause, want continuous presentation of a 25 fps clip (pace follows the compositor on a software renderer under load)", n, gap)
	}
	if err := in.Tap(KeySpace); err != nil {
		t.Fatalf("tap space: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if n := commitsWithin(w, time.Second); n > 3 {
		t.Errorf("%d frames presented in a second after pausing", n)
	}
}

// TestHeadlessGPUAreaPaces pins #150's pacing on a real compositor: the
// GPU demo's area keeps asking for frames through its subsurface's own
// frame callbacks and presents each one.
func TestHeadlessGPUAreaPaces(t *testing.T) {
	requireEnv(t)
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-gpu", "lys-ui-gpu")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "frame,shell")
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WaitEver("frame", "GPUArea frame committed", traceTimeout); err != nil {
		t.Fatalf("the GPUArea never presented: %v", err)
	}
	mark := w.Mark()
	time.Sleep(time.Second)
	n := w.CountSince(mark, "frame", "GPUArea frame committed")
	gap := w.MaxGapSince(mark, "frame", "GPUArea frame committed")
	if n < 10 || gap > 0.5 {
		t.Errorf("%d GPUArea frames in a second with a %.2fs longest pause, want sustained pacing at the compositor's frame rate (absolute rate is renderer-bound)", n, gap)
	}
}
