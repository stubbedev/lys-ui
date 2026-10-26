// Compositor disconnect policy. When the connection dies (compositor
// crash, reload, upgrade), the session surfaces one typed
// DisconnectError (internal/wlsession); this file turns that into the
// application's story: fire OnDisconnect exactly once, tear every child
// down — window pools, then the session's arena and display — and exit
// the loop with an error callers can match. The sanctioned default is a
// clean exit with the distinct DisconnectExitCode so a supervisor
// (systemd Restart=on-failure, a wayle supervisor) respawns on the
// restarted session; SetReconnect (reconnect.go) rebuilds on the new
// session instead.
package app

import (
	"errors"

	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// ErrDisconnected reports the compositor ended the connection. Run
// returns it (matching under errors.Is) after running OnDisconnect and
// tearing the loop down. Ordinary closes are ErrClosed; only a dead
// connection is this.
var ErrDisconnected = wlsession.ErrDisconnected

// DisconnectReason classifies why the connection ended: the compositor
// went away (crash/reload/upgrade), or it raised a fatal protocol error
// against this client first.
type DisconnectReason = wlsession.DisconnectReason

const (
	// DisconnectConnectionLost means the compositor is gone without a
	// verdict — the restart case.
	DisconnectConnectionLost = wlsession.DisconnectConnectionLost
	// DisconnectProtocol means the compositor sent a fatal
	// wl_display.error before the connection ended.
	DisconnectProtocol = wlsession.DisconnectProtocol
)

// DisconnectExitCode is the exit status of the clean-exit policy: a
// distinct, nonzero code (EX_TEMPFAIL) a supervisor's Restart=on-failure
// (or a wayle supervisor) watches to respawn the app on the new session,
// while keeping genuine crashes (which exit by panic) distinguishable.
const DisconnectExitCode = 75

// DisconnectedEvent is what OnDisconnect receives: the classified
// reason and the underlying wire error.
type DisconnectedEvent struct {
	Reason DisconnectReason
	// Err is the *wlsession.DisconnectError the loop failed with.
	Err error
	// Reconnecting reports that Run will dial the compositor again and
	// rebuild the windows (SetReconnect) rather than return.
	Reconnecting bool
}

// OnDisconnect installs the disconnect policy hook (also available as
// Config.OnDisconnect for the single-window Run). It fires at most once
// per Run, on the loop goroutine, before any teardown: the callback may
// flush state, but must not block indefinitely and must not route work
// back through the loop (Invoke already drops once Run is unwinding).
// After it returns, Run releases every window's pool and closes the
// session (shared arena: memfd, mapping, fd — and the display), so the
// exiting process leaves nothing mapped. Without a hook the same
// teardown runs; the hook is optional observation, not permission.
func (a *Application) OnDisconnect(fn func(DisconnectedEvent)) { a.onDisconnect = fn }

// loopError classifies a loop failure before Run returns it. A
// disconnect — already classified by the session's dispatch, or a raw
// wire failure from the frame path, run through the same classifier —
// runs the policy once and comes back as the typed error; anything
// else is returned untouched.
func (a *Application) loopError(err error) error {
	if de, ok := errors.AsType[*wlsession.DisconnectError](err); ok {
		a.handleDisconnect(de)
		return err
	}
	if a.sess != nil {
		if de, ok := errors.AsType[*wlsession.DisconnectError](a.sess.Classify(err)); ok {
			a.handleDisconnect(de)
			return de
		}
	}
	return err
}

// handleDisconnect runs the disconnect policy exactly once per
// Application (a nested dispatch loop — a popup grab — can surface the
// same dead socket twice before Run unwinds): fire the hook, then the
// children-first teardown — every window's surface registration and
// buffer pool, then the session's shared arena and display. Run's
// deferred queues.shutdown stops the timers and drains Invokes right
// after, so nothing survives the exit path to touch a dead connection.
func (a *Application) handleDisconnect(de *wlsession.DisconnectError) {
	if !a.disconnectOnce.CompareAndSwap(false, true) {
		return
	}
	debug.Log("wire", "app: compositor disconnected (%s): %v", de.Reason, de.Err)
	reconnecting := a.reconnect != nil && de.Reason == DisconnectConnectionLost && a.rebuildable()
	if a.onDisconnect != nil {
		a.onDisconnect(DisconnectedEvent{Reason: de.Reason, Err: de, Reconnecting: reconnecting})
	}
	if reconnecting {
		a.rebuildPlan = a.planRebuild()
	}
	for _, w := range a.windows {
		a.toasts.closeHost(w.host)
		w.release()
	}
	a.windows = nil
	if a.sess != nil {
		a.sess.Close()
	}
}
