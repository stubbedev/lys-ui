// XDG activation: optional xdg_activation_v1 support — the
// compositor-consented focus request. A client asks for a token
// anchored to a real user interaction (the serial of a pointer press
// or keyboard event); once the compositor issues it, the token may be
// spent to raise and focus a surface, or handed to a spawned app
// through XDG_ACTIVATION_TOKEN so the child can focus itself. The
// manager global is feature-detected: without it a token request
// reports ErrActivationUnavailable and Activate does nothing.
package wlsession

import (
	"errors"
	"fmt"
	"slices"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxActivationVersion is the xdg_activation_v1 version we bind: the
// protocol stopped at version 1.
const maxActivationVersion = 1

// activationAPI is the request side of xdg_activation_v1, seen
// through the narrow interface below so tests can record the
// requests. The wire proxy satisfies it through wireActivation.
type activationAPI interface {
	GetActivationToken() (activationTokenAPI, error)
	Activate(token string, surface *wl.Surface) error
}

// activationTokenAPI is the request and listener side of
// xdg_activation_token_v1. *wlr.ActivationTokenV1 satisfies it;
// tests substitute a recorder.
type activationTokenAPI interface {
	SetSerial(serial uint32, seat *wl.Seat) error
	SetAppId(appID string) error
	SetSurface(surface *wl.Surface) error
	Commit() error
	Destroy() error
	AddDoneHandler(h wlr.ActivationTokenV1DoneHandler)
}

// wireActivation adapts the generated xdg-activation proxies to the
// narrow interfaces above; tests substitute recorders.
type wireActivation struct{ mgr *wlr.ActivationV1 }

// GetActivationToken implements activationAPI.
func (a wireActivation) GetActivationToken() (activationTokenAPI, error) {
	t, err := a.mgr.GetActivationToken()
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Activate implements activationAPI.
func (a wireActivation) Activate(token string, surface *wl.Surface) error {
	return a.mgr.Activate(token, surface)
}

// ActivationAvailable reports whether the compositor advertised
// xdg_activation_v1. When false, every activation call on the session
// is a no-op.
func (s *Session) ActivationAvailable() bool { return s.activation != nil }

// bindActivation binds the optional activation global.
func (s *Session) bindActivation(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewActivationV1(ctx)
	if !s.bindOptional(ev, maxActivationVersion, mgr) {
		return
	}
	s.activation = wireActivation{mgr: mgr}
	debug.Log("shell", "xdg-activation-v1 bound")
}

// ErrActivationUnavailable is a token request on a compositor without
// xdg_activation_v1.
var ErrActivationUnavailable = errors.New("wlsession: the compositor offers no xdg_activation_v1")

// RequestActivationToken begins a token request anchored to the
// user-interaction serial of a pointer press or keyboard event (0
// means unanchored, which compositors may refuse to honor with
// focus). done always runs once: with the issued token - each request
// its own, so concurrent requests (a launch, an attention request)
// never trade tokens - or with ErrActivationUnavailable without the
// protocol, or the request's failure.
func (s *Session) RequestActivationToken(surface *wl.Surface, serial uint32, done func(token string, err error)) {
	req := s.activation
	if req == nil {
		done("", ErrActivationUnavailable)
		return
	}
	tokReq, err := req.GetActivationToken()
	if err != nil {
		done("", fmt.Errorf("wlsession: request an activation token: %w", err))
		return
	}
	tok := &activationToken{sess: s, req: tokReq, done: done}
	s.activationTokens = append(s.activationTokens, tok)
	tokReq.AddDoneHandler(tok)
	if serial != 0 && s.seat != nil {
		_ = tokReq.SetSerial(serial, s.seat)
	}
	if surface != nil {
		_ = tokReq.SetSurface(surface)
	}
	if err := tokReq.Commit(); err != nil {
		s.removeActivationToken(tok)
		_ = tokReq.Destroy()
		done("", fmt.Errorf("wlsession: commit an activation token request: %w", err))
	}
}

// Activate raises and focuses surface with a token the compositor
// issued through RequestActivationToken (the one the launcher handed
// to the spawned app). No-op without the protocol.
func (s *Session) Activate(surface *wl.Surface, token string) {
	if s.activation == nil || token == "" || surface == nil {
		return
	}
	_ = s.activation.Activate(token, surface)
}

// activationToken tracks one in-flight token request.
type activationToken struct {
	sess *Session
	req  activationTokenAPI
	done func(token string, err error)
}

// HandleActivationTokenV1Done implements
// wlr.ActivationTokenV1DoneHandler: the compositor issued the token;
// hand it to the host and tear the object down.
func (t *activationToken) HandleActivationTokenV1Done(ev wlr.ActivationTokenV1DoneEvent) {
	t.sess.removeActivationToken(t)
	_ = t.req.Destroy()
	t.done(ev.Token, nil)
}

// removeActivationToken drops a finished or failed request from the
// in-flight list.
func (s *Session) removeActivationToken(tok *activationToken) {
	for i, t := range s.activationTokens {
		if t == tok {
			s.activationTokens = slices.Delete(s.activationTokens, i, i+1)
			return
		}
	}
}
