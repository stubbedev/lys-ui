package app

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

func TestKeyRepeatHoldsOffWhileAKeymapCompiles(t *testing.T) {
	a := idleBudgetApp(t)
	repeats := 0
	a.OnKey(func(*widget.Router, uint32, wlsession.Mods) { repeats++ })
	a.rep.press(30, 0)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	a.rep.next = time.Now().Add(-time.Millisecond)
	if err := a.tick(a.step, time.Now()); err != nil {
		t.Fatal(err)
	}
	if repeats != 1 {
		t.Fatalf("%d repeats with no keymap compiling, want 1", repeats)
	}

	fd, err := syscall.Dup(int(r.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	a.sess.HandleKeyboardKeymap(wl.KeyboardKeymapEvent{Format: 1, Fd: uintptr(fd), Size: 0})
	if !a.sess.KeymapPending() {
		t.Fatal("the keymap is not pending")
	}
	a.rep.next = time.Now().Add(-time.Millisecond)
	if err := a.tick(a.step, time.Now()); err != nil {
		t.Fatal(err)
	}
	if repeats != 1 {
		t.Errorf("%d repeats behind a compiling keymap, want none past the first", repeats)
	}
}
