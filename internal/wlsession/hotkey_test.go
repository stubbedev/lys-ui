package wlsession

import (
	"errors"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/wlr"
	"github.com/stubbedev/lys-lib/xkb"
)

type hotkeyTrigger struct{ keysym, modifiers uint32 }

type fakeHotkey struct {
	description string
	triggers    []hotkeyTrigger
	commits     int
	destroyed   int
	bound       wlr.HotkeyV1BoundHandler
	denied      wlr.HotkeyV1DeniedHandler
	revoked     wlr.HotkeyV1RevokedHandler
	triggered   wlr.HotkeyV1TriggeredHandler
	released    wlr.HotkeyV1ReleasedHandler
}

func (f *fakeHotkey) SetDescription(d string) error { f.description = d; return nil }
func (f *fakeHotkey) SetKeyTrigger(k, m uint32) error {
	f.triggers = append(f.triggers, hotkeyTrigger{k, m})
	return nil
}
func (f *fakeHotkey) Commit() error                                  { f.commits++; return nil }
func (f *fakeHotkey) Destroy() error                                 { f.destroyed++; return nil }
func (f *fakeHotkey) AddBoundHandler(h wlr.HotkeyV1BoundHandler)     { f.bound = h }
func (f *fakeHotkey) AddDeniedHandler(h wlr.HotkeyV1DeniedHandler)   { f.denied = h }
func (f *fakeHotkey) AddRevokedHandler(h wlr.HotkeyV1RevokedHandler) { f.revoked = h }
func (f *fakeHotkey) AddTriggeredHandler(h wlr.HotkeyV1TriggeredHandler) {
	f.triggered = h
}
func (f *fakeHotkey) AddReleasedHandler(h wlr.HotkeyV1ReleasedHandler) { f.released = h }

type fakeHotkeyManager struct {
	appIDs    []string
	hotkeys   []*fakeHotkey
	destroyed int
}

func (m *fakeHotkeyManager) SetAppId(id string) error { m.appIDs = append(m.appIDs, id); return nil }

func (m *fakeHotkeyManager) CreateHotkey() (hotkeyAPI, error) {
	k := &fakeHotkey{}
	m.hotkeys = append(m.hotkeys, k)
	return k, nil
}
func (m *fakeHotkeyManager) Destroy() error { m.destroyed++; return nil }

type fakeHotkeyGlobal struct{ managers []*fakeHotkeyManager }

func (g *fakeHotkeyGlobal) bind() (hotkeyManagerAPI, error) {
	m := &fakeHotkeyManager{}
	g.managers = append(g.managers, m)
	return m, nil
}

func TestHotkeysAreOptional(t *testing.T) {
	if slices.Contains(requiredGlobals, "xx_hotkey_manager_v1") || !slices.Contains(optionalGlobals, "xx_hotkey_manager_v1") {
		t.Error("xx_hotkey_manager_v1 must be an optional, reported global")
	}
	s := &Session{}
	if s.HotkeysAvailable() {
		t.Error("hotkeys available without the global")
	}
	if h, err := s.NewHotkeys("org.example.App"); !errors.Is(err, ErrHotkeysUnavailable) || h != nil {
		t.Errorf("NewHotkeys without the global = %v, %v, want ErrHotkeysUnavailable", h, err)
	}
}

func TestHotkeysBindAManagerPerAppID(t *testing.T) {
	g := &fakeHotkeyGlobal{}
	s := &Session{hotkeyGlobal: g}
	if !s.HotkeysAvailable() {
		t.Fatal("hotkeys unavailable with the global")
	}
	for _, id := range []string{"org.example.A", "org.example.B", ""} {
		if _, err := s.NewHotkeys(id); err != nil {
			t.Fatal(err)
		}
	}
	if len(g.managers) != 3 {
		t.Fatalf("bound %d managers, want one per NewHotkeys", len(g.managers))
	}
	if !slices.Equal(g.managers[0].appIDs, []string{"org.example.A"}) || !slices.Equal(g.managers[1].appIDs, []string{"org.example.B"}) || len(g.managers[2].appIDs) != 0 {
		t.Errorf("app ids sent %v %v %v; an unidentified client sends none", g.managers[0].appIDs, g.managers[1].appIDs, g.managers[2].appIDs)
	}
}

func newTestHotkey(t *testing.T, events HotkeyEvents) (*Hotkey, *fakeHotkey) {
	t.Helper()
	g := &fakeHotkeyGlobal{}
	s := &Session{hotkeyGlobal: g}
	h, err := s.NewHotkeys("org.example.App")
	if err != nil {
		t.Fatal(err)
	}
	k, err := h.NewHotkey("Toggle", events)
	if err != nil {
		t.Fatal(err)
	}
	return k, g.managers[0].hotkeys[0]
}

func TestHotkeyBindAnswersEachCommitInOrder(t *testing.T) {
	var presses []bool
	k, wire := newTestHotkey(t, HotkeyEvents{Pressed: func(p bool) { presses = append(presses, p) }})
	if wire.description != "Toggle" {
		t.Errorf("description %q", wire.description)
	}
	var answers []error
	done := func(err error) { answers = append(answers, err) }
	if err := k.Bind(xkb.Keysym('p'), ModCtrl|ModAlt, done); err != nil {
		t.Fatal(err)
	}
	if err := k.Bind(xkb.KeyF9, ModSuper, done); err != nil {
		t.Fatal(err)
	}
	want := []hotkeyTrigger{{'p', wlr.HotkeyManagerV1ModifiersCtrl | wlr.HotkeyManagerV1ModifiersAlt}, {uint32(xkb.KeyF9), wlr.HotkeyManagerV1ModifiersSuper}}
	if !slices.Equal(wire.triggers, want) || wire.commits != 2 {
		t.Fatalf("sent %v in %d commits, want %v in 2", wire.triggers, wire.commits, want)
	}
	if len(answers) != 0 {
		t.Fatal("answered before the compositor did")
	}
	wire.bound.HandleHotkeyV1Bound(wlr.HotkeyV1BoundEvent{})
	wire.denied.HandleHotkeyV1Denied(wlr.HotkeyV1DeniedEvent{Reason: wlr.HotkeyV1DenyReasonAlreadyBound, Message: "Super+F9 is bound"})
	var denied *HotkeyDeniedError
	if len(answers) != 2 || answers[0] != nil || !errors.As(answers[1], &denied) || denied.Reason != HotkeyAlreadyBound || denied.Message != "Super+F9 is bound" {
		t.Fatalf("answers %v, want [nil, already bound]", answers)
	}
	wire.triggered.HandleHotkeyV1Triggered(wlr.HotkeyV1TriggeredEvent{})
	wire.released.HandleHotkeyV1Released(wlr.HotkeyV1ReleasedEvent{})
	if !slices.Equal(presses, []bool{true, false}) {
		t.Errorf("presses %v, want [true false]", presses)
	}
}

func TestHotkeyDestroyAnswersCommitsInFlightAndSilencesEvents(t *testing.T) {
	presses, revoked := 0, 0
	k, wire := newTestHotkey(t, HotkeyEvents{Pressed: func(bool) { presses++ }, Revoked: func(string) { revoked++ }})
	var answer error
	if err := k.Bind(xkb.Keysym('p'), ModCtrl, func(err error) { answer = err }); err != nil {
		t.Fatal(err)
	}
	k.Destroy()
	k.Destroy()
	if wire.destroyed != 1 || !errors.Is(answer, ErrHotkeyDestroyed) {
		t.Fatalf("destroyed %d times, answer %v; want 1 and ErrHotkeyDestroyed", wire.destroyed, answer)
	}
	wire.triggered.HandleHotkeyV1Triggered(wlr.HotkeyV1TriggeredEvent{})
	wire.revoked.HandleHotkeyV1Revoked(wlr.HotkeyV1RevokedEvent{})
	if presses != 0 || revoked != 0 {
		t.Errorf("a destroyed hotkey reported %d presses, %d revocations", presses, revoked)
	}
	if err := k.Bind(xkb.Keysym('p'), ModCtrl, func(error) {}); !errors.Is(err, ErrHotkeyDestroyed) {
		t.Errorf("Bind after Destroy = %v", err)
	}
}

func TestHotkeyRefusesANullKey(t *testing.T) {
	k, wire := newTestHotkey(t, HotkeyEvents{})
	if err := k.Bind(xkb.KeyNoSymbol, ModCtrl, func(error) {}); err == nil || wire.commits != 0 {
		t.Errorf("a null keysym was committed (err %v, %d commits); the protocol makes it fatal", err, wire.commits)
	}
}
