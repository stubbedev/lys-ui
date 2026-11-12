package app

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbusx"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// An Activated and a Deactivated from the portal fire the live shortcut
// through Invoke with the timestamp's Unix seconds; an unknown id
// changes nothing.
func TestPortalShortcutsFireTheLiveShortcut(t *testing.T) {
	a := testApp(nil)
	p := &portalShortcuts{app: a}
	p.live = map[string]*GlobalShortcut{}
	routes := p.shortcutRoutes()
	const portal = ":1.7"
	trigger := func(s dbusx.Signal[shortcutTrigger], id string, us uint64) {
		t.Helper()
		sig := &dbus.Signal{Sender: portal, Path: portalPath, Name: s.Name(), Body: []any{dbus.ObjectPath("/session"), id, us, map[string]dbus.Variant{}}}
		if err := dbusx.Dispatch(sig, portal, routes); err != nil {
			t.Fatal(err)
		}
	}
	var fired []bool
	var secs []uint64
	p.live["push-to-talk"] = &GlobalShortcut{on: func(pressed bool, unixSec uint64) {
		fired = append(fired, pressed)
		secs = append(secs, unixSec)
	}}
	trigger(shortcutActivated, "push-to-talk", 1_700_000_000_123_456)
	trigger(shortcutDeactivated, "push-to-talk", 1_700_000_001_000_000)
	trigger(shortcutActivated, "ghost", 1)
	a.pump(time.Now())
	if !slices.Equal(fired, []bool{true, false}) {
		t.Errorf("triggered = %v, want press then release", fired)
	}
	if !slices.Equal(secs, []uint64{1_700_000_000, 1_700_000_001}) {
		t.Errorf("timestamps = %v, want Unix seconds", secs)
	}
}

// A Response with a non-zero code is the user's denial.
func TestPortalDenialIsErrGlobalShortcutDenied(t *testing.T) {
	a, _, m := shortcutApp(t)
	m.release()
	var (
		err      error
		answered bool
	)
	a.Invoke(func() {
		a.RegisterGlobalShortcut(deniedShortcut, "dev.stubbe.test", "", "", func(bool, uint64) {}, func(_ *GlobalShortcut, e error) { err, answered = e, true })
	})
	pumpUntil(t, a, func() bool { return answered })
	if !errors.Is(err, ErrGlobalShortcutDenied) {
		t.Errorf("a denied bind returned %v", err)
	}
}

func TestPortalTokensAreObjectPathElements(t *testing.T) {
	var c portalClient
	for _, kind := range []tokenKind{requestToken, sessionToken} {
		if token := c.token(kind); !dbus.ObjectPath("/org/freedesktop/portal/desktop/session/x/" + token).IsValid() {
			t.Errorf("token %q is not an object path element, so the portal cannot build its handle", token)
		}
	}
}

const (
	bindShortcutsSignature = "oa(sa{sv})sa{sv}"
	deniedShortcut         = "denied"
)

type mockShortcuts struct {
	d       *mockDesktop
	hold    chan struct{}
	release func()
	entered chan struct{}
	binds   atomic.Int32
	mu      sync.Mutex
	bound   []portalShortcut
}

func (m *mockShortcuts) CreateSession(sender dbus.Sender, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	m.entered <- struct{}{}
	<-m.hold
	path := m.d.handle(sender, opts)
	m.d.respond(sender, path, 0, map[string]dbus.Variant{
		"session_handle": dbus.MakeVariant(dbus.ObjectPath("/org/freedesktop/portal/desktop/session/test/s")),
	})
	return path, nil
}

func (m *mockShortcuts) BindShortcuts(msg dbus.Message, sender dbus.Sender, _ dbus.ObjectPath, shortcuts []portalShortcut, _ string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	if sig, _ := msg.Headers[dbus.FieldSignature].Value().(dbus.Signature); sig.String() != bindShortcutsSignature {
		return "", dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"BindShortcuts takes " + bindShortcutsSignature + ", not " + sig.String()})
	}
	m.binds.Add(1)
	m.mu.Lock()
	m.bound = append(m.bound, shortcuts...)
	m.mu.Unlock()
	path := m.d.handle(sender, opts)
	code := uint32(0)
	if shortcuts[0].ID == deniedShortcut {
		code = 1
	}
	m.d.respond(sender, path, code, map[string]dbus.Variant{})
	return path, nil
}

func shortcutApp(t *testing.T) (*Application, *mockDesktop, *mockShortcuts) {
	t.Helper()
	d := startMockDesktop(t)
	hold := make(chan struct{})
	m := &mockShortcuts{d: d, hold: hold, release: sync.OnceFunc(func() { close(hold) }), entered: make(chan struct{}, 8)}
	if err := d.conn.Export(m, "/org/freedesktop/portal/desktop", portalShortcutsIface); err != nil {
		t.Fatal(err)
	}
	a := testApp(nil)
	a.sess = &wlsession.Session{}
	t.Cleanup(a.shortcuts.shutdown)
	t.Cleanup(m.release)
	return a, d, m
}

func TestRegisterGlobalShortcutReturnsWhileThePortalIsStuck(t *testing.T) {
	a, d, m := shortcutApp(t)
	var (
		got      *GlobalShortcut
		err      error
		answered bool
		pressed  []bool
	)
	a.Invoke(func() {
		a.RegisterGlobalShortcut("ptt", "dev.stubbe.test", "talk", "CTRL+T", func(on bool, _ uint64) {
			pressed = append(pressed, on)
		}, func(s *GlobalShortcut, e error) { got, err, answered = s, e, true })
	})
	a.pump(time.Now())
	await(t, m.entered, "the worker reaching the portal")
	a.pump(time.Now())
	if answered || m.binds.Load() != 0 {
		t.Fatalf("answered %v with %d binds while the portal held the session", answered, m.binds.Load())
	}
	m.release()
	pumpUntil(t, a, func() bool { return answered })
	if err != nil || got == nil {
		t.Fatalf("registration = %v, %v", got, err)
	}
	if m.binds.Load() != 1 {
		t.Errorf("%d binds, want one", m.binds.Load())
	}
	m.mu.Lock()
	bound := m.bound
	m.mu.Unlock()
	if len(bound) != 1 || bound[0].ID != "ptt" ||
		bound[0].Properties["description"].Value() != "talk" ||
		bound[0].Properties["preferred_trigger"].Value() != "CTRL+T" {
		t.Errorf("the portal bound %+v, want ptt with its description and trigger", bound)
	}
	d.unicast(a.shortcuts.conn.Names()[0], portalPath, portalShortcutsIface+"|Activated", dbus.ObjectPath("/s"), "ptt", uint64(1_000_000), map[string]dbus.Variant{})
	pumpUntil(t, a, func() bool { return len(pressed) == 1 })
	got.Destroy()
}

func TestGlobalShortcutsAvailableAsksOffTheLoop(t *testing.T) {
	a, _, _ := shortcutApp(t)
	release := holdWorker(t, func(job func()) bool { return a.shortcuts.post(a, job) })
	var up, answered bool
	a.GlobalShortcutsAvailable(func(v bool) { up, answered = v, true })
	a.pump(time.Now())
	if answered {
		t.Fatal("answered while the worker was held")
	}
	release()
	pumpUntil(t, a, func() bool { return answered })
	if !up {
		t.Error("the mock portal owns its name, want available")
	}
}

func TestRegisterGlobalShortcutBusyBehindAStuckPortal(t *testing.T) {
	a, _, _ := shortcutApp(t)
	holdWorker(t, func(job func()) bool { return a.shortcuts.post(a, job) })
	for a.shortcuts.post(a, func() {}) {
	}
	var got error
	a.RegisterGlobalShortcut("x", "app", "", "", func(bool, uint64) {}, func(_ *GlobalShortcut, err error) { got = err })
	a.pump(time.Now())
	if !errors.Is(got, ErrPortalBusy) {
		t.Errorf("registration over a full queue = %v", got)
	}
}
