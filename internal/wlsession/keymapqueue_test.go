package wlsession

import (
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/xkb"
)

func goroutineID() string {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	id, _, _ := strings.Cut(strings.TrimPrefix(string(buf[:n]), "goroutine "), " ")
	return id
}

type blockedCompile struct {
	release chan compiledKeymap
	ran     chan string
}

func holdCompiles(s *Session) *blockedCompile {
	b := &blockedCompile{release: make(chan compiledKeymap), ran: make(chan string, 8)}
	s.compileKeymap = func(uintptr, uint32, uint32) compiledKeymap {
		b.ran <- goroutineID()
		return <-b.release
	}
	return b
}

func keymapEvent(t *testing.T) wl.KeyboardKeymapEvent {
	t.Helper()
	r, _, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return wl.KeyboardKeymapEvent{Format: 1, Fd: r.Fd(), Size: 1}
}

func compiledLayout(t *testing.T, layout string) compiledKeymap {
	t.Helper()
	t.Setenv("XKB_CONFIG_ROOT", findXKBData(t))
	km, err := xkb.NewContext(os.Getenv).NewKeymapFromNames(xkb.Names{Layout: layout})
	if err != nil {
		t.Skipf("%s keymap unavailable: %v", layout, err)
	}
	return compiledKeymap{keymap: km, state: km.NewState()}
}

const (
	evdevY = 21
	evdevZ = 44
)

func TestKeymapCompilesOffTheLoop(t *testing.T) {
	s := usKeymap(t)
	held := holdCompiles(s)
	s.HandleKeyboardKeymap(keymapEvent(t))
	if ran := <-held.ran; ran == goroutineID() {
		t.Error("the keymap compiled on the loop goroutine")
	}
	if !s.KeymapPending() {
		t.Error("a keymap compiling is not reported pending")
	}
	held.release <- compiledLayout(t, "de")
	settleBackground(t, s)
	if s.KeymapPending() {
		t.Error("a settled keymap still reports pending")
	}
}

func TestKeysBehindACompilingKeymapTranslateThroughIt(t *testing.T) {
	s := usKeymap(t)
	var typed []string
	s.OnKey = func(code uint32, _ Mods) { typed = append(typed, s.KeyUTF8(code)) }
	held := holdCompiles(s)
	german := compiledLayout(t, "de")

	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1, Serial: 1})
	s.HandleKeyboardKeymap(keymapEvent(t))
	<-held.ran
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1, Serial: 2})
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 1})
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevZ, State: 1, Serial: 3})
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{})
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevZ, State: 1, Serial: 4})
	if !slices.Equal(typed, []string{"y"}) {
		t.Fatalf("delivered %q with the keymap compiling, want only the key ahead of it", typed)
	}
	if s.InputSerial() != 1 {
		t.Errorf("input serial %d moved ahead of its event", s.InputSerial())
	}

	held.release <- german
	settleBackground(t, s)
	if want := []string{"y", "z", "Y", "y"}; !slices.Equal(typed, want) {
		t.Errorf("typed %q, want %q: the old keymap ahead of the event, the new one behind it", typed, want)
	}
	if s.InputSerial() != 4 {
		t.Errorf("input serial %d, want the last key's", s.InputSerial())
	}
}

func TestKeymapsInARowEachKeepTheirKeys(t *testing.T) {
	s := usKeymap(t)
	var typed []string
	s.OnKey = func(code uint32, _ Mods) { typed = append(typed, s.KeyUTF8(code)) }
	held := holdCompiles(s)
	german, american := compiledLayout(t, "de"), compiledLayout(t, "us")

	s.HandleKeyboardKeymap(keymapEvent(t))
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1, Serial: 1})
	s.HandleKeyboardKeymap(keymapEvent(t))
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1, Serial: 2})

	<-held.ran
	held.release <- german
	<-held.ran
	held.release <- american
	settleBackground(t, s)
	if want := []string{"z", "y"}; !slices.Equal(typed, want) {
		t.Errorf("typed %q, want %q", typed, want)
	}
}

func TestFocusWaitsWithTheKeys(t *testing.T) {
	s := usKeymap(t)
	held := holdCompiles(s)
	first, second := &wl.Surface{}, &wl.Surface{}
	var focusAtKey []*wl.Surface
	s.OnKey = func(uint32, Mods) { focusAtKey = append(focusAtKey, s.KeyboardFocus()) }

	s.HandleKeyboardKeymap(keymapEvent(t))
	<-held.ran
	s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: first})
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1})
	s.HandleKeyboardLeave(wl.KeyboardLeaveEvent{})
	s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: second})
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1})
	if s.KeyboardFocus() != nil {
		t.Error("focus moved ahead of the keymap")
	}
	held.release <- compiledLayout(t, "us")
	settleBackground(t, s)
	if len(focusAtKey) != 2 || focusAtKey[0] != first || focusAtKey[1] != second {
		t.Errorf("keys saw focus %v, want first then second", focusAtKey)
	}
}

func TestKeysBehindAFailedKeymapStayUntranslated(t *testing.T) {
	s := usKeymap(t)
	var typed []string
	s.OnKey = func(code uint32, _ Mods) { typed = append(typed, s.KeyUTF8(code)) }
	held := holdCompiles(s)
	s.HandleKeyboardKeymap(keymapEvent(t))
	s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1})
	<-held.ran
	held.release <- compiledKeymap{err: os.ErrInvalid}
	settleBackground(t, s)
	if !slices.Equal(typed, []string{""}) {
		t.Errorf("typed %q, want one untranslated key", typed)
	}
}

func TestKeyQueueIsBounded(t *testing.T) {
	s := usKeymap(t)
	delivered := 0
	s.OnKey = func(uint32, Mods) { delivered++ }
	held := holdCompiles(s)
	s.HandleKeyboardKeymap(keymapEvent(t))
	<-held.ran
	for range maxQueuedKeyEvents + 10 {
		s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1})
	}
	if got := len(s.keyQueue); got != maxQueuedKeyEvents {
		t.Errorf("queue holds %d, want the bound %d", got, maxQueuedKeyEvents)
	}
	held.release <- compiledLayout(t, "us")
	settleBackground(t, s)
	if delivered != maxQueuedKeyEvents-1 {
		t.Errorf("delivered %d, want the %d queued behind the keymap", delivered, maxQueuedKeyEvents-1)
	}
}

const isolatedTestEnv = "LYS_UI_ISOLATED_TEST"

func runIsolated(t *testing.T) bool {
	t.Helper()
	if os.Getenv(isolatedTestEnv) == t.Name() {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), isolatedTestEnv+"="+t.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running %s in its own process: %v\n%s", t.Name(), err, out)
	}
	return true
}

func TestKeyWithNoKeymapPendingAllocatesNothing(t *testing.T) {
	if runIsolated(t) {
		return
	}
	s := &Session{}
	s.OnKey = func(uint32, Mods) {}
	if got := testing.AllocsPerRun(100, func() {
		s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: evdevY, State: 1})
	}); got != 0 {
		t.Errorf("a key allocates %.1f times", got)
	}
}
