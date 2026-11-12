package app

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbusx"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/notify"
	"github.com/stubbedev/lys-ui/internal/waittest"
)

// TestNotifyRoutesActionsOntoLoop pins the loop bridge: an activation
// arriving from the notifier's goroutine dispatches the notification's
// OnAction on the loop goroutine (inside a pump), and a close forgets
// the id - a late action after the close fires nothing.
func TestNotifyRoutesActionsOntoLoop(t *testing.T) {
	a := testApp(nil)
	fired := ""
	closed := 0
	a.Notify("title", "body", NotifyOptions{
		Timeout:  2 * time.Second,
		OnAction: func(key string) { fired = key },
		OnClosed: func(notify.CloseReason) { closed++ },
	})
	defer a.closeNotifier()
	a.notifyMu.Lock()
	var id string
	for k := range a.notifyOpts.opts {
		id = k
	}
	a.notifyMu.Unlock()
	if id == "" {
		t.Fatal("the notification was not registered")
	}

	// The dispatch is what the Invoke bridge runs inside a pump: the
	// action fires the callback, the close forgets the id.
	a.Invoke(func() { a.notifyAction(id, "open") })
	a.pump(time.Now())
	if fired != "open" {
		t.Errorf("action never dispatched: %q", fired)
	}
	a.Invoke(func() { a.notifyClosed(id, notify.ReasonExpired) })
	a.pump(time.Now())
	if closed != 1 {
		t.Errorf("close dispatched %d times", closed)
	}
	a.Invoke(func() { a.notifyAction(id, "open") })
	a.pump(time.Now())
	if fired != "open" {
		t.Error("an action after the close changed state")
	}
}

// TestNotifyInertWithoutBus pins the no-desktop failure model: with no
// session bus, Notify is silent - no panic, no error - and the id
// still registers so late callbacks (never fired) route harmlessly.
func TestNotifyInertWithoutBus(t *testing.T) {
	a := testApp(nil)
	defer a.closeNotifier()
	a.Notify("hello", "background daemon says hi", NotifyOptions{})
	a.notifyMu.Lock()
	n := len(a.notifyOpts.opts)
	a.notifyMu.Unlock()
	if n != 1 {
		t.Errorf("registered %d notifications, want 1", n)
	}
}

// The action buttons reach the transport (they were once dropped on
// the way, leaving OnAction nothing to answer); a plain notification
// carries none and no default.
func TestNotifyCarriesItsActions(t *testing.T) {
	n := notification("t", "b", NotifyOptions{
		Actions:       []NotifyAction{{Key: "reply", Label: "Reply"}, {Key: "mute", Label: "Mute"}},
		DefaultAction: NotifyAction{Key: "open", Label: "Open"},
	})
	if len(n.Actions) != 2 || n.Actions[0].Key != "reply" || n.Actions[1].Label != "Mute" {
		t.Errorf("actions = %+v, want both buttons in order", n.Actions)
	}
	if !n.HasDefault || n.DefaultAction.Key != "open" {
		t.Errorf("default = %+v (%v), want open", n.DefaultAction, n.HasDefault)
	}
	if plain := notification("t", "b", NotifyOptions{}); len(plain.Actions) != 0 || plain.HasDefault {
		t.Errorf("plain = %+v, want no actions and no default", plain)
	}
}

type fakeNotifyBus struct {
	loop    string
	release chan struct{}

	mu     sync.Mutex
	onLoop int
	sends  int
}

func (f *fakeNotifyBus) Call(ctx context.Context, _ string, _ dbus.ObjectPath, method string, _ dbus.Flags, ret any, args ...any) error {
	f.mu.Lock()
	if callerGoroutine() == f.loop {
		f.onLoop++
	}
	f.mu.Unlock()
	if method == "org.freedesktop.DBus.NameHasOwner" {
		*ret.(*bool) = args[0] == "org.freedesktop.Notifications"
		return nil
	}
	f.mu.Lock()
	f.sends++
	f.mu.Unlock()
	select {
	case <-f.release:
	case <-ctx.Done():
	}
	return nil
}

func (f *fakeNotifyBus) Watch(context.Context, dbusx.Sender, func(error), ...dbusx.Route) (func() error, error) {
	return func() error { return nil }, nil
}

func (f *fakeNotifyBus) Close() error { return nil }

func TestNotifyOnLoopSurvivesAStuckDaemon(t *testing.T) {
	f := &fakeNotifyBus{loop: callerGoroutine(), release: make(chan struct{})}
	a := testApp(nil)
	a.notifyOnce.Do(func() {
		a.adoptNotifier(notify.NewWithDial(func() (notify.Bus, error) {
			if callerGoroutine() == f.loop {
				t.Error("the dial ran on the loop")
			}
			return f, nil
		}))
	})
	defer a.closeNotifier()
	defer close(f.release)
	ran := 0
	a.Invoke(func() {
		a.Notify("stuck", "body", NotifyOptions{})
		a.Notify("stuck too", "body", NotifyOptions{})
		ran++
	})
	a.pump(time.Now())
	if ran != 1 {
		t.Fatalf("the pass ran %d times", ran)
	}
	a.notifyMu.Lock()
	registered := len(a.notifyOpts.opts)
	a.notifyMu.Unlock()
	if registered != 2 {
		t.Errorf("registered %d notifications with locally made ids, want 2", registered)
	}
	waittest.Until(t, func() bool {
		f.mu.Lock()
		sends, onLoop := f.sends, f.onLoop
		f.mu.Unlock()
		if onLoop != 0 {
			t.Fatalf("%d bus calls on the loop", onLoop)
		}
		return sends > 0
	}, "the worker to reach the daemon")
}

func TestNotifyLedgerDropsTheOldestPastItsCap(t *testing.T) {
	var l notifyLedger
	id := func(i int) string { return "n" + strconv.Itoa(i) }
	total := 3 * maxTrackedNotifications
	for i := range total {
		l.put(id(i), NotifyOptions{})
		if len(l.opts) > maxTrackedNotifications {
			t.Fatalf("%d entries after %d puts, want at most %d", len(l.opts), i+1, maxTrackedNotifications)
		}
		if len(l.order) > 2*maxTrackedNotifications {
			t.Fatalf("order holds %d after %d puts", len(l.order), i+1)
		}
	}
	if _, ok := l.opts[id(total-maxTrackedNotifications-1)]; ok {
		t.Error("the entry just past the cap survived")
	}
	if _, ok := l.opts[id(total-maxTrackedNotifications)]; !ok {
		t.Error("the oldest entry inside the cap went")
	}
	if _, ok := l.opts[id(total-1)]; !ok {
		t.Error("the newest entry went")
	}
}

func TestNotifyLedgerStaysBoundedAcrossCloses(t *testing.T) {
	var l notifyLedger
	for i := range 10 * maxTrackedNotifications {
		id := strconv.Itoa(i)
		l.put(id, NotifyOptions{})
		l.take(id)
		if len(l.order) > 2*maxTrackedNotifications {
			t.Fatalf("order holds %d closed ids after %d puts", len(l.order), i+1)
		}
	}
	if len(l.opts) != 0 {
		t.Errorf("%d entries after every one closed", len(l.opts))
	}
}
