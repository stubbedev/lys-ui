package app

import (
	"time"

	"github.com/stubbedev/lys-ui/internal/notify"
)

// Desktop notifications (#85): app.Notify sends a GNotification-class
// desktop notification through internal/notify - the
// xdg-desktop-portal transport when the desktop owns it,
// org.freedesktop.Notifications otherwise, inert without a session bus
// (the appearance.md failure model: no error, no crash, a Debug log).
// This is how a background lys-ui daemon reports to the user; the
// in-window toast (ShowToast) is a different, compositor-free surface.

// NotifyAction is one desktop-notification action.
type NotifyAction struct {
	Key   string
	Label string
}

// NotifyOptions carries the optional halves of a notification. The
// callbacks - like every app callback - run on the loop goroutine,
// bridged from the notifier's signal goroutine through Invoke.
type NotifyOptions struct {
	// Icon is an icon-theme name or a file path; whoever shows the
	// notification resolves it, the same themed pipeline as SetIcon.
	Icon string
	// Timeout is the expiry hint: zero means the server default,
	// negative means never expire.
	Timeout time.Duration
	// Urgency raises the priority; see notify.Priority.
	Urgency notify.Priority
	// Actions become the notification's buttons; activating one fires
	// OnAction with its key, on the loop.
	Actions []NotifyAction
	// DefaultAction makes the notification body itself activatable.
	DefaultAction NotifyAction
	// OnAction fires when an action (or the default) activated.
	OnAction func(key string)
	// OnClosed fires when the notification closed, with the reason
	// when the transport reports one.
	OnClosed func(reason notify.CloseReason)
}

// Notify sends a desktop notification and forgets it: the dial and the
// bus call run on the notifier's worker, action and close callbacks
// come back onto the loop, and a desktop without a notification surface
// drops the notification silently. The lys-ui-side id the transport
// works with is generated here and stays internal.
func (a *Application) Notify(title, body string, opts NotifyOptions) {
	n := a.notifier()
	id := n.Notify(notification(title, body, opts))
	a.notifyMu.Lock()
	if a.notifyOpts == nil {
		a.notifyOpts = map[string]NotifyOptions{}
	}
	a.notifyOpts[id] = opts
	a.notifyMu.Unlock()
}

// notification is the transport's form of a Notify call, the action
// buttons included.
func notification(title, body string, opts NotifyOptions) notify.Notification {
	actions := make([]notify.Action, 0, len(opts.Actions))
	for _, act := range opts.Actions {
		actions = append(actions, notify.Action(act))
	}
	return notify.Notification{
		Title:         title,
		Body:          body,
		Icon:          opts.Icon,
		Priority:      opts.Urgency,
		Timeout:       opts.Timeout,
		Actions:       actions,
		DefaultAction: notify.Action(opts.DefaultAction),
		HasDefault:    opts.DefaultAction.Key != "",
	}
}

// notifier creates (once) the desktop notifier, which touches no bus
// until its worker runs, and wires its signals onto the loop.
func (a *Application) notifier() *notify.Notifier {
	a.notifyOnce.Do(func() { a.adoptNotifier(notify.New()) })
	a.notifyMu.Lock()
	defer a.notifyMu.Unlock()
	return a.notifyStore
}

func (a *Application) adoptNotifier(n *notify.Notifier) {
	a.notifyMu.Lock()
	a.notifyStore = n
	a.notifyMu.Unlock()
	n.OnAction(func(id, action string) {
		a.Invoke(func() { a.notifyAction(id, action) })
	})
	n.OnClosed(func(id string, reason notify.CloseReason) {
		a.Invoke(func() { a.notifyClosed(id, reason) })
	})
}

// notifyAction dispatches an activation on the loop; unknown ids
// (post-Reset notifications, portal string drift) are dropped.
func (a *Application) notifyAction(id, action string) {
	a.notifyMu.Lock()
	opts, ok := a.notifyOpts[id]
	a.notifyMu.Unlock()
	if ok && opts.OnAction != nil {
		opts.OnAction(action)
	}
}

// notifyClosed dispatches a close on the loop and forgets the id.
func (a *Application) notifyClosed(id string, reason notify.CloseReason) {
	a.notifyMu.Lock()
	opts, ok := a.notifyOpts[id]
	delete(a.notifyOpts, id)
	a.notifyMu.Unlock()
	if ok && opts.OnClosed != nil {
		opts.OnClosed(reason)
	}
}

// closeNotifier is Run's exit hook: the notifier's connection dies
// with the loop like every other loop-owned resource.
func (a *Application) closeNotifier() {
	a.notifyMu.Lock()
	n := a.notifyStore
	a.notifyStore = nil
	a.notifyMu.Unlock()
	if n != nil {
		n.Shutdown()
	}
}
