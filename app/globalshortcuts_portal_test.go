package app

import (
	"testing"
	"time"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

// TestPortalShortcutsDispatch pins the portal state machine, fed the
// signals directly: a Response resolves the pending call by handle
// token carrying code and results; Activated and Deactivated fire the
// live shortcut through Invoke with the timestamp's Unix seconds; an
// unknown id or empty token changes nothing.
func TestPortalShortcutsDispatch(t *testing.T) {
	a := testApp(nil)
	p := &portalShortcuts{app: a}
	p.pending = map[string]chan portalResponseData{}
	p.live = map[string]*GlobalShortcut{}
	p.onSignal = p.shortcutSignal

	// Response: results ride along, the pending entry is consumed.
	ch := make(chan portalResponseData, 1)
	p.pending["bind1"] = ch
	p.dispatch(&dbus.Signal{
		Name: portalResponse,
		Path: "/org/freedesktop/portal/desktop/request/:1.42/bind1",
		Body: []any{uint32(0), map[string]dbus.Variant{
			"session_handle": dbus.MakeVariant(dbus.ObjectPath("/org/freedesktop/portal/desktop/session/:1.42/lysui_s1")),
		}},
	})
	select {
	case resp := <-ch:
		if resp.code != 0 {
			t.Errorf("response code = %d", resp.code)
		}
		if got, _ := resp.results["session_handle"].Value().(dbus.ObjectPath); got == "" {
			t.Error("session handle lost from the response")
		}
	default:
		t.Fatal("the Response signal never reached the pending call")
	}
	if _, still := p.pending["bind1"]; still {
		t.Error("the pending entry was not consumed")
	}

	// An unrelated handle token resolves nothing and drops nothing.
	p.dispatch(&dbus.Signal{Name: portalResponse, Path: "/x/nope"})
	p.dispatch(&dbus.Signal{Name: portalResponse})

	// Activated fires the live shortcut on the loop, with the µs
	// timestamp folded to Unix seconds.
	var fired []bool
	var secs []uint64
	s := &GlobalShortcut{on: func(pressed bool, unixSec uint64) {
		fired = append(fired, pressed)
		secs = append(secs, unixSec)
	}}
	p.live["push-to-talk"] = s
	p.dispatch(&dbus.Signal{
		Name: portalActivated,
		Body: []any{dbus.ObjectPath("/session"), "push-to-talk", uint64(1_700_000_000_123_456)},
	})
	p.dispatch(&dbus.Signal{
		Name: portalDeactivated,
		Body: []any{dbus.ObjectPath("/session"), "push-to-talk", uint64(1_700_000_001_000_000)},
	})
	a.pump(time.Now())
	if len(fired) != 2 || !fired[0] || fired[1] {
		t.Errorf("triggered = %v, want press then release", fired)
	}
	if len(secs) != 2 || secs[0] != 1_700_000_000 {
		t.Errorf("timestamps = %v, want Unix seconds", secs)
	}

	// Unknown ids and short bodies change nothing.
	p.dispatch(&dbus.Signal{Name: portalActivated, Body: []any{dbus.ObjectPath("/session"), "ghost", uint64(1)}})
	p.dispatch(&dbus.Signal{Name: portalActivated})
	a.pump(time.Now())
	if len(fired) != 2 {
		t.Errorf("an unknown id fired: %v", fired)
	}
}

// TestPortalDeniedMapsResponseCode pins the verdict mapping: any
// non-zero Response code is the user's denial
// (ErrGlobalShortcutDenied), zero is success.
func TestPortalDeniedMapsResponseCode(t *testing.T) {
	a := testApp(nil)
	p := &portalShortcuts{app: a}
	p.pending = map[string]chan portalResponseData{}
	p.live = map[string]*GlobalShortcut{}
	p.onSignal = p.shortcutSignal
	p.session = "/org/freedesktop/portal/desktop/session/x"
	// bindOne waits on call(); without a connection call fails with
	// ErrGlobalShortcutsUnavailable before any denial, so pin the
	// mapping through dispatch feeding the code bindOne turns into the
	// error.
	ch := make(chan portalResponseData, 1)
	p.pending["bind"] = ch
	p.dispatch(&dbus.Signal{
		Name: portalResponse,
		Path: "/org/freedesktop/portal/desktop/request/:1.42/bind",
		Body: []any{uint32(1)},
	})
	select {
	case resp := <-ch:
		if resp.code == 0 {
			t.Error("a denied bind mapped to success")
		}
	default:
		t.Fatal("denial never dispatched")
	}
}

// TestPathTail pins the handle-token extraction: the segment after the
// last slash of the request path.
func TestPathTail(t *testing.T) {
	cases := map[string]string{
		"/org/freedesktop/portal/desktop/request/:1.42/lysui1": "lysui1",
		"/org/freedesktop/portal/desktop/request/_1_42/lysui2": "lysui2",
		"nopath": "",
	}
	for path, want := range cases {
		if got := pathTail(dbus.ObjectPath(path)); got != want {
			t.Errorf("pathTail(%q) = %q, want %q", path, got, want)
		}
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
