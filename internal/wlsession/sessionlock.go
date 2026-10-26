// Session lock: optional ext_session_lock_manager_v1 support — the
// secure lock-screen protocol (internal/sessionlock drives it). The
// manager global is feature-detected: without it SessionLockManager
// stays nil and locking is unavailable.
//
// Output watchers live here too: a lock must cover every output,
// hotplugged ones included, without claiming the single-slot
// OnOutputAdded/OnOutputRemoved hooks the application owns.
package wlsession

import (
	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxSessionLockVersion is the ext_session_lock_manager_v1 version we
// bind: the protocol is at version 1.
const maxSessionLockVersion = 1

// SessionLockManager returns the bound ext_session_lock_manager_v1,
// nil when the compositor does not offer it.
func (s *Session) SessionLockManager() *wlr.SessionLockManagerV1 { return s.sessionLockMgr }

// bindSessionLockManager binds the optional session-lock manager.
func (s *Session) bindSessionLockManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewSessionLockManagerV1(ctx)
	if !s.bindOptional(ev, maxSessionLockVersion, mgr) {
		return
	}
	s.sessionLockMgr = mgr
	debug.Log("shell", "ext-session-lock-v1 bound")
}

// outputWatcher is one WatchOutputs or WatchOutputIdentity
// subscription.
type outputWatcher struct {
	added, removed, identity func(*Output)
}

// WatchOutputs subscribes to output hotplug alongside the
// OnOutputAdded/OnOutputRemoved hooks: added runs when a wl_output
// global appears, removed when it goes away (either may be nil). The
// returned stop unsubscribes; it is idempotent. Watchers run on the
// event-loop goroutine, inside dispatch, after the hooks.
func (s *Session) WatchOutputs(added, removed func(*Output)) (stop func()) {
	return s.watch(&outputWatcher{added: added, removed: removed})
}

// WatchOutputIdentity subscribes to output identities alongside the
// OnOutputIdentity hook: fn runs when an output's xdg-output name
// arrives or changes. The returned stop unsubscribes; it is
// idempotent. Watchers run on the event-loop goroutine, after the hook.
func (s *Session) WatchOutputIdentity(fn func(*Output)) (stop func()) {
	return s.watch(&outputWatcher{identity: fn})
}

func (s *Session) watch(w *outputWatcher) (stop func()) {
	s.outputWatchers = append(s.outputWatchers, w)
	return func() {
		for i, cur := range s.outputWatchers {
			if cur == w {
				s.outputWatchers = append(s.outputWatchers[:i], s.outputWatchers[i+1:]...)
				return
			}
		}
	}
}

// notifyOutputIdentity fans a named output out to the hook and every
// identity watcher.
func (s *Session) notifyOutputIdentity(out *Output) {
	if s.OnOutputIdentity != nil {
		s.OnOutputIdentity(out)
	}
	for _, w := range append([]*outputWatcher(nil), s.outputWatchers...) {
		if w.identity != nil {
			w.identity(out)
		}
	}
}

// notifyOutputAdded fans an added output out to the hook and every
// watcher. The watcher list is copied first, so a watcher that
// unsubscribes (or subscribes) mid-notify does not skip or repeat
// anyone.
func (s *Session) notifyOutputAdded(out *Output) {
	if s.OnOutputAdded != nil {
		s.OnOutputAdded(out)
	}
	for _, w := range append([]*outputWatcher(nil), s.outputWatchers...) {
		if w.added != nil {
			w.added(out)
		}
	}
}

// notifyOutputRemoved is notifyOutputAdded for removals.
func (s *Session) notifyOutputRemoved(out *Output) {
	if s.OnOutputRemoved != nil {
		s.OnOutputRemoved(out)
	}
	for _, w := range append([]*outputWatcher(nil), s.outputWatchers...) {
		if w.removed != nil {
			w.removed(out)
		}
	}
}
