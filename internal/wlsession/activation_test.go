package wlsession

import (
	"errors"
	"testing"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// fakeActivationToken records one token object's requests and keeps
// the done listener the session registered.
type fakeActivationToken struct {
	serial    uint32
	seat      *wl.Seat
	surface   *wl.Surface
	committed int
	destroyed int
	commitErr error
	doneH     wlr.ActivationTokenV1DoneHandler
}

func (f *fakeActivationToken) SetSerial(serial uint32, seat *wl.Seat) error {
	f.serial, f.seat = serial, seat
	return nil
}

func (f *fakeActivationToken) SetAppId(string) error { return nil }

func (f *fakeActivationToken) SetSurface(surface *wl.Surface) error {
	f.surface = surface
	return nil
}

func (f *fakeActivationToken) Commit() error {
	f.committed++
	return f.commitErr
}

func (f *fakeActivationToken) Destroy() error { f.destroyed++; return nil }

func (f *fakeActivationToken) AddDoneHandler(h wlr.ActivationTokenV1DoneHandler) { f.doneH = h }

// fakeActivation records the manager's requests and hands out token
// recorders.
type fakeActivation struct {
	tokens      []*fakeActivationToken
	activations []activationCall
	tokenErr    error
	commitErr   error // handed to every new token
}

type activationCall struct {
	token   string
	surface *wl.Surface
}

func (f *fakeActivation) GetActivationToken() (activationTokenAPI, error) {
	if f.tokenErr != nil {
		return nil, f.tokenErr
	}
	t := &fakeActivationToken{commitErr: f.commitErr}
	f.tokens = append(f.tokens, t)
	return t, nil
}

func (f *fakeActivation) Activate(token string, surface *wl.Surface) error {
	f.activations = append(f.activations, activationCall{token, surface})
	return nil
}

// TestActivationIsOptional pins the bind-or-skip contract: the
// activation global must never gate Connect, and a session without it
// survives every activation call.
func TestActivationIsOptional(t *testing.T) {
	for _, g := range requiredGlobals {
		if g == "xdg_activation_v1" {
			t.Errorf("%q is required; activation must stay feature-detected", g)
		}
	}
	s := &Session{}
	if s.ActivationAvailable() {
		t.Error("ActivationAvailable = true without the manager global")
	}
	var got error
	s.RequestActivationToken(&wl.Surface{}, 7, func(_ string, err error) { got = err })
	if !errors.Is(got, ErrActivationUnavailable) {
		t.Errorf("a token request without the global reported %v, want ErrActivationUnavailable", got)
	}
	s.Activate(&wl.Surface{}, "tok") // no-op
}

// TestActivationTokenFlow drives the consent flow against fakes: the
// token request anchors the user-interaction serial and surface,
// commits, and the done event lands the issued token on the host
// before the object is torn down.
func TestActivationTokenFlow(t *testing.T) {
	s := &Session{seat: &wl.Seat{}}
	fake := &fakeActivation{}
	s.activation = fake

	var got []string
	collect := func(token string, err error) {
		if err != nil {
			t.Errorf("token request failed: %v", err)
		}
		got = append(got, token)
	}

	surf := &wl.Surface{}
	s.RequestActivationToken(surf, 42, collect)
	if len(fake.tokens) != 1 {
		t.Fatalf("token objects created = %d, want 1", len(fake.tokens))
	}
	tok := fake.tokens[0]
	if tok.serial != 42 || tok.seat != s.seat {
		t.Errorf("set_serial = %d/%v, want 42/session seat", tok.serial, tok.seat)
	}
	if tok.surface != surf {
		t.Error("set_surface did not carry the anchor surface")
	}
	if tok.committed != 1 {
		t.Errorf("commits = %d, want 1", tok.committed)
	}

	tok.doneH.HandleActivationTokenV1Done(wlr.ActivationTokenV1DoneEvent{Token: "tok-1"})
	if len(got) != 1 || got[0] != "tok-1" {
		t.Fatalf("tokens delivered = %v, want [tok-1]", got)
	}
	if tok.destroyed != 1 {
		t.Errorf("token destroyed %d times, want once", tok.destroyed)
	}
	if len(s.activationTokens) != 0 {
		t.Errorf("finished request still in flight: %d", len(s.activationTokens))
	}

	// An unanchored request (serial 0) skips set_serial; a nil surface
	// skips set_surface. Both are optional per protocol.
	s.RequestActivationToken(nil, 0, collect)
	tok2 := fake.tokens[1]
	if tok2.serial != 0 || tok2.seat != nil || tok2.surface != nil {
		t.Errorf("unanchored request sent anchors: serial=%d surface=%v", tok2.serial, tok2.surface)
	}
	if tok2.committed != 1 {
		t.Errorf("second request commits = %d, want 1", tok2.committed)
	}
}

// TestActivationTokenCommitFailure rolls a failed commit back: the
// token object is destroyed, the request leaves the in-flight list,
// and done hears the failure.
func TestActivationTokenCommitFailure(t *testing.T) {
	s := &Session{seat: &wl.Seat{}}
	good := &fakeActivation{}
	s.activation = good
	s.RequestActivationToken(nil, 5, func(string, error) {})
	if len(s.activationTokens) != 1 || good.tokens[0].destroyed != 0 {
		t.Fatalf("healthy request not in flight: %+v", s.activationTokens)
	}

	broken := &fakeActivation{commitErr: errors.New("commit failed")}
	s.activation = broken
	var failed error
	s.RequestActivationToken(nil, 6, func(_ string, err error) { failed = err })
	if failed == nil {
		t.Error("a failed commit was not reported")
	}
	if len(broken.tokens) != 1 || broken.tokens[0].destroyed != 1 {
		t.Fatal("failed commit left the token object alive")
	}
	if len(s.activationTokens) != 1 {
		t.Errorf("in-flight requests = %d, want only the healthy one", len(s.activationTokens))
	}
}

// TestActivateSpendsToken records the activate request; an empty
// token or missing surface is a silent no-op.
func TestActivateSpendsToken(t *testing.T) {
	s := &Session{}
	fake := &fakeActivation{}
	s.activation = fake
	surf := &wl.Surface{}

	s.Activate(surf, "tok")
	if len(fake.activations) != 1 || fake.activations[0].token != "tok" || fake.activations[0].surface != surf {
		t.Fatalf("activate calls = %+v, want one for tok", fake.activations)
	}

	s.Activate(surf, "")
	if len(fake.activations) != 1 {
		t.Error("empty token must not reach the wire")
	}
	s.Activate(nil, "tok")
	if len(fake.activations) != 1 {
		t.Error("nil surface must not reach the wire")
	}
}

// Each request's token goes to its own done, in whatever order the
// compositor answers.
func TestActivationTokenPerRequest(t *testing.T) {
	s := &Session{seat: &wl.Seat{}}
	fake := &fakeActivation{}
	s.activation = fake
	var first, second []string
	s.RequestActivationToken(nil, 1, func(tok string, _ error) { first = append(first, tok) })
	s.RequestActivationToken(nil, 2, func(tok string, _ error) { second = append(second, tok) })
	fake.tokens[1].doneH.HandleActivationTokenV1Done(wlr.ActivationTokenV1DoneEvent{Token: "b"})
	fake.tokens[0].doneH.HandleActivationTokenV1Done(wlr.ActivationTokenV1DoneEvent{Token: "a"})
	if len(first) != 1 || first[0] != "a" || len(second) != 1 || second[0] != "b" {
		t.Errorf("first %v second %v", first, second)
	}
}
