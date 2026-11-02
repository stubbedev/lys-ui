package headlesstest

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestHeadlessClickAwayPopoverLeavesTheKeyboard drives a dropdown beside
// a focused window of another client through a real compositor: keys
// stay with the window until a click into the dropdown, which then has
// them, Esc closes it, a click away closes it and goes no further, and
// each close plays the exit before the surface goes.
func TestHeadlessClickAwayPopoverLeavesTheKeyboard(t *testing.T) {
	requireEnv(t)
	_, window, _ := startShowcase(t)
	in := newInput(t)
	waitKeyboard(t, window)
	bin, err := BuildClient(testEnv.Dir, "./cmd/lys-ui-popover", "lys-ui-popover")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "dropdown-"+t.Name(), "input,demo")
	if err != nil {
		t.Fatal(err)
	}
	stopAndReport(t, c)
	drop, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	tr, err := drop.Wait("demo", "control drop center ", traceTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var x, y, entryY, moreY int
	if _, err := fmt.Sscanf(tr.Message, "control drop center (%d,%d)", &x, &y); err != nil {
		t.Fatalf("bad trace %q: %v", tr.Message, err)
	}

	requireExit := func(how string) {
		t.Helper()
		for _, want := range []string{"dropdown exit", "dropdown exit done", "dropdown closed"} {
			if _, err := drop.Wait("demo", want, traceTimeout); err != nil {
				t.Fatalf("%s: never saw %q: %v\n%s", how, want, err, tailTraces(drop, 25))
			}
		}
	}

	// Opening leaves the keyboard where it was: a key reaches the window.
	if err := clickUntil(drop, in, "demo", x, y, BTNLeft, "dropdown open"); err != nil {
		t.Fatalf("the dropdown never opened: %v\n%s", err, tailTraces(drop, 25))
	}
	tr, err = drop.Wait("demo", "dropdown controls ", traceTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Sscanf(tr.Message, "dropdown controls entry y %d more y %d", &entryY, &moreY); err != nil {
		t.Fatalf("bad trace %q: %v", tr.Message, err)
	}
	keys, dropMark := window.Mark(), drop.Mark()
	if err := in.Tap(KeyTab); err != nil {
		t.Fatal(err)
	}
	if err := window.WaitSince(keys, "input", "wire key", traceTimeout); err != nil {
		t.Errorf("a key with the dropdown open never reached the focused window: %v\n%s", err, tailTraces(window, 15))
	}
	for _, tr := range drop.Since(dropMark) {
		if strings.Contains(tr.Message, "popover key") {
			t.Errorf("the dropdown took a key it was not focused for: %s", tr.Message)
		}
	}

	// A click into it gives it the keys: typing reaches its entry.
	if err := in.ClickAt(x, entryY, BTNLeft); err != nil {
		t.Fatal(err)
	}
	if err := in.TypeText("hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := drop.Wait("demo", "dropdown text hi", traceTimeout); err != nil {
		t.Fatalf("typing after a click into the dropdown never reached it: %v\n%s", err, tailTraces(drop, 25))
	}

	// Esc closes it, through the exit.
	if err := in.Tap(KeyEscape); err != nil {
		t.Fatal(err)
	}
	requireExit("Esc")

	// A click away closes it, through the exit, and goes no further: the
	// window beneath hears no press. Far from the layer and the dropdown.
	if err := clickUntil(drop, in, "demo", x, y, BTNLeft, "dropdown open"); err != nil {
		t.Fatalf("the dropdown never reopened: %v\n%s", err, tailTraces(drop, 25))
	}
	time.Sleep(100 * time.Millisecond)
	mark := window.Mark()
	if err := in.ClickAt(showcaseW-20, showcaseH-20, BTNLeft); err != nil {
		t.Fatal(err)
	}
	requireExit("a click away")
	for _, tr := range window.Since(mark) {
		if strings.Contains(tr.Message, "route button") && strings.Contains(tr.Message, " press ") {
			t.Errorf("the press that closed the dropdown reached the window: %s", tr.Message)
		}
	}

	// A menu nests under the dropdown without a grab of its own, which a
	// compositor would refuse under a parent holding none, and a click
	// away closes the whole chain.
	if err := clickUntil(drop, in, "demo", x, y, BTNLeft, "dropdown open"); err != nil {
		t.Fatalf("the dropdown never reopened for the menu: %v\n%s", err, tailTraces(drop, 25))
	}
	menu := drop.Mark()
	if err := clickUntil(drop, in, "demo", x, moreY, BTNLeft, "dropdown menu open"); err != nil {
		t.Fatalf("the nested menu never opened: %v\n%s", err, tailTraces(drop, 25))
	}
	if err := in.ClickAt(showcaseW-20, showcaseH-20, BTNLeft); err != nil {
		t.Fatal(err)
	}
	requireExit("a click away from the nested menu")
	if err := drop.WaitSince(menu, "demo", "dropdown menu closed", traceTimeout); err != nil {
		t.Fatalf("a click away left the nested menu open: %v\n%s", err, tailTraces(drop, 25))
	}

	// The catchers are gone: the next click reaches the window.
	mark = window.Mark()
	if err := in.ClickAt(showcaseW-20, showcaseH-20, BTNLeft); err != nil {
		t.Fatal(err)
	}
	if err := window.WaitSince(mark, "input", "route button", traceTimeout); err != nil {
		t.Errorf("a click after the close never reached the window: %v\n%s", err, tailTraces(window, 15))
	}
}
