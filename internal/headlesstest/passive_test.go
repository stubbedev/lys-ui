package headlesstest

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestHeadlessPassivePopoverLetsClicksAndKeysThrough drives a passive
// popover, a hover preview's kind, through a real compositor: a key
// with it open reaches the focused window, the first click on another
// control of its own layer acts there, and a click on another client's
// window reaches that window, while the popover never takes a key and
// stays open until its owner dismisses it.
func TestHeadlessPassivePopoverLetsClicksAndKeysThrough(t *testing.T) {
	window, in, drop, dropX, dropY := startDropdownBesideWindow(t)
	tr, err := drop.Wait("demo", "control peek center ", traceTimeout)
	if err != nil {
		t.Fatal(err)
	}
	var x, y int
	if _, err := fmt.Sscanf(tr.Message, "control peek center (%d,%d)", &x, &y); err != nil {
		t.Fatalf("bad trace %q: %v", tr.Message, err)
	}
	if err := clickUntil(drop, in, "demo", x, y, BTNLeft, "peek open"); err != nil {
		t.Fatalf("the peek never opened: %v\n%s", err, tailTraces(drop, 25))
	}
	opened := drop.Mark()

	keys := window.Mark()
	if err := in.Tap(KeyTab); err != nil {
		t.Fatal(err)
	}
	if err := window.WaitSince(keys, "input", "wire key", traceTimeout); err != nil {
		t.Errorf("a key with the peek open never reached the focused window: %v\n%s", err, tailTraces(window, 15))
	}

	mark := window.Mark()
	if err := in.ClickAt(showcaseW-20, showcaseH-20, BTNLeft); err != nil {
		t.Fatal(err)
	}
	if err := window.WaitSince(mark, "input", "route button", traceTimeout); err != nil {
		t.Errorf("a click beside the peek never reached the window beneath: %v\n%s", err, tailTraces(window, 15))
	}

	for _, tr := range drop.Since(opened) {
		if strings.Contains(tr.Message, "peek closed") {
			t.Errorf("a key or a click beside the peek closed it: %s", tr.Message)
		}
	}

	dropMark := drop.Mark()
	if err := in.ClickAt(dropX, dropY, BTNLeft); err != nil {
		t.Fatal(err)
	}
	if err := drop.WaitSince(dropMark, "demo", "dropdown open", traceTimeout); err != nil {
		t.Fatalf("the first click on the layer's dropdown button beside the peek did not open it: %v\n%s", err, tailTraces(drop, 25))
	}
	if err := drop.WaitSince(dropMark, "demo", "peek closed", traceTimeout); err != nil {
		t.Errorf("the dropdown opened over the layer did not replace the peek: %v\n%s", err, tailTraces(drop, 25))
	}

	time.Sleep(100 * time.Millisecond)
	for _, tr := range drop.Since(opened) {
		if strings.Contains(tr.Message, "popover key") {
			t.Errorf("the peek took a key: %s", tr.Message)
		}
	}
}
