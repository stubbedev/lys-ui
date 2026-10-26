package app

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// TestLockSessionPreconditions pins the refusals that never reach the
// wire: a config without a surface builder, a compositor without the
// protocol, and a second lock while one is held.
func TestLockSessionPreconditions(t *testing.T) {
	surface := func(*Output) LockSurface { return LockSurface{} }

	a := &Application{sess: &wlsession.Session{}}
	if a.SessionLockAvailable() {
		t.Error("SessionLockAvailable = true without the manager global")
	}
	if _, err := a.LockSession(SessionLockConfig{}); err == nil {
		t.Error("a lock without a Surface builder must be refused")
	}
	if _, err := a.LockSession(SessionLockConfig{Surface: surface}); !errors.Is(err, ErrSessionLockUnavailable) {
		t.Errorf("LockSession without the protocol = %v, want ErrSessionLockUnavailable", err)
	}
	if a.SessionLock() != nil {
		t.Error("a refused lock must not be recorded as held")
	}

	held := &SessionLock{app: a}
	a.sessionLock = held
	if _, err := a.LockSession(SessionLockConfig{Surface: surface}); !errors.Is(err, ErrSessionLockActive) {
		t.Errorf("second LockSession = %v, want ErrSessionLockActive", err)
	}
	if a.SessionLock() != held {
		t.Error("the refused second lock replaced the held one")
	}
}

// TestHeldLockKeepsLoopAlive pins the loop-exit rule: with no window
// the loop ends, unless a session lock is held - the process that
// must unlock the session cannot exit because its last output went
// away. Quit still wins.
func TestHeldLockKeepsLoopAlive(t *testing.T) {
	a := &Application{}
	if !a.done() {
		t.Fatal("no windows and no lock: the loop must end")
	}
	a.sessionLock = &SessionLock{app: a}
	if a.done() {
		t.Fatal("a held lock with no surface mapped must keep the loop alive")
	}
	a.quit = true
	if !a.done() {
		t.Error("Quit must end the loop even while a lock is held")
	}
}

// TestSessionLockSetFocus: focus lands in the lock surface whose tree
// holds the widget and nowhere else.
func TestSessionLockSetFocus(t *testing.T) {
	face, err := Font("sans", 14)
	if err != nil {
		t.Skipf("no system font: %v", err)
	}
	entryA := widget.NewEntry(face, 14, render.RGB(255, 255, 255))
	entryB := widget.NewEntry(face, 14, render.RGB(255, 255, 255))
	a := &hostWindow{router: &widget.Router{Root: entryA}}
	b := &hostWindow{router: &widget.Router{Root: entryB}}
	l := &SessionLock{surfaces: map[*Output]*hostWindow{{}: a, {}: b}}
	l.SetFocus(entryB)
	if a.router.Focused() != nil || b.router.Focused() != entryB {
		t.Errorf("focus: a=%v b=%v, want only b's entry", a.router.Focused(), b.router.Focused())
	}
}

// TestLockHostCloser: the raw teardown of a lock host destroys its
// lock surface, like the layer and toplevel hosts' closers.
func TestLockHostCloser(t *testing.T) {
	if hostCloser(&lockHost{}) == nil {
		t.Error("lock hosts need a closer for the window teardown path")
	}
}

// TestHoldKeepsLoopAlive pins Hold: a window-less loop runs while a
// hold is unreleased, a release counts once, and Quit still wins.
func TestHoldKeepsLoopAlive(t *testing.T) {
	a := &Application{}
	first, second := a.Hold(), a.Hold()
	if a.done() {
		t.Fatal("a held application with no window must keep the loop alive")
	}
	first()
	first()
	if a.done() {
		t.Fatal("a repeated release dropped the other hold")
	}
	second()
	if !a.done() {
		t.Fatal("every hold released and no window: the loop must end")
	}
	release := a.Hold()
	a.quit = true
	if !a.done() {
		t.Error("Quit must end the loop even while held")
	}
	release()
}
