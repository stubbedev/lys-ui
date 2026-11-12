package app

import (
	"errors"
	"sync"
	"testing"

	"github.com/stubbedev/lys-ui/internal/waittest"
)

// launchRecorder records transport calls race-safely.
type launchRecorder struct {
	mu        sync.Mutex
	calls     []launchCall
	portalErr error
}

type launchCall struct {
	uri    string
	parent string
	token  string
}

func (r *launchRecorder) portal() func(uri, parent, token string) error {
	return func(uri, parent, token string) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, launchCall{uri, parent, token})
		return r.portalErr
	}
}

func (r *launchRecorder) external() func(uri string) error {
	return func(uri string) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, launchCall{uri: uri, token: "external"})
		return nil
	}
}

func (r *launchRecorder) snapshot() []launchCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]launchCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// TestOpenURLPortalFirst pins the transport order: without activation
// support the open runs immediately through the portal; a failing
// portal falls back to the external opener exactly once.
func TestOpenURLPortalFirst(t *testing.T) {
	a := testApp(nil) // no session: no activation protocol
	rec := &launchRecorder{}
	a.launchState.openPortal = rec.portal()
	a.launchState.openExternal = rec.external()

	a.OpenURL("https://example.com")
	waittest.Until(t, func() bool { return len(rec.snapshot()) == 1 }, "the portal call")
	if got := rec.snapshot()[0]; got.uri != "https://example.com" || got.token != "" {
		t.Errorf("portal call = %+v", got)
	}

	// A failing portal falls back.
	rec.calls, rec.portalErr = nil, errors.New("portal down")
	a.OpenURL("https://fallback.example")
	waittest.Until(t, func() bool {
		for _, c := range rec.snapshot() {
			if c.token == "external" {
				return true
			}
		}
		return false
	}, "the fallback")
	if got := rec.snapshot(); len(got) != 2 || got[1].uri != "https://fallback.example" {
		t.Errorf("fallback calls = %+v", got)
	}
}

// TestOpenPathEncodesFileURI pins the FileLauncher shape: a path
// launches as its file:// URI.
func TestOpenPathEncodesFileURI(t *testing.T) {
	a := testApp(nil)
	rec := &launchRecorder{}
	a.launchState.openPortal = rec.portal()
	a.OpenPath("/tmp/report.pdf")
	waittest.Until(t, func() bool { return len(rec.snapshot()) == 1 }, "the portal call")
	if got := rec.snapshot()[0].uri; got != "file:///tmp/report.pdf" {
		t.Errorf("opened %q, want the file URI", got)
	}
}
