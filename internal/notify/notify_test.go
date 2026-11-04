package notify

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

const patience = 10 * time.Second

func goroutine() string {
	buf := make([]byte, 64)
	return strings.Fields(string(buf[:runtime.Stack(buf, false)]))[1]
}

// fakeBus is a daemon that answers NameHasOwner for owned, and holds every
// other call until released.
type fakeBus struct {
	owned   string
	caller  string
	release chan struct{}
	classic uint32

	mu      sync.Mutex
	onCall  int
	methods []string
	closed  bool
	ch      chan<- *dbus.Signal
}

func (f *fakeBus) Call(ctx context.Context, _ string, _ dbus.ObjectPath, method string, _ dbus.Flags, ret any, args ...any) error {
	f.mu.Lock()
	if goroutine() == f.caller {
		f.onCall++
	}
	f.methods = append(f.methods, method)
	f.mu.Unlock()
	if method == "org.freedesktop.DBus.NameHasOwner" {
		*ret.(*bool) = args[0] == f.owned
		return nil
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if p, ok := ret.(*uint32); ok {
		*p = f.classic
	}
	return nil
}

func (f *fakeBus) Signal(ch chan<- *dbus.Signal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = ch
}

func (f *fakeBus) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeBus) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.methods...)
}

func awaitMethod(t *testing.T, f *fakeBus, method string) {
	t.Helper()
	deadline := time.Now().Add(patience)
	for time.Now().Before(deadline) {
		if slices.Contains(f.seen(), method) {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("%s never reached the bus; saw %v", method, f.seen())
}

// A daemon that never answers holds up nothing but the worker: Notify and
// Close return on the caller with no bus call there, the dial included.
func TestNotifyMakesNoBusCallOnTheCaller(t *testing.T) {
	f := &fakeBus{owned: classicName, caller: goroutine(), release: make(chan struct{}), classic: 7}
	dials := 0
	n := NewWithDial(func() (Bus, error) {
		dials++
		return f, nil
	})
	defer n.Shutdown()
	id := n.Notify(Notification{Title: "t", Body: "b"})
	if id == "" {
		t.Fatal("no id")
	}
	second := n.Notify(Notification{Title: "t2"})
	if second == id {
		t.Fatal("two notifications shared an id")
	}
	n.Close(id)
	awaitMethod(t, f, classicNotify)
	close(f.release)
	awaitMethod(t, f, classicCloseCall)
	f.mu.Lock()
	onCall := f.onCall
	f.mu.Unlock()
	if onCall != 0 {
		t.Errorf("%d bus calls on the caller, want none", onCall)
	}
	if dials != 1 {
		t.Errorf("%d dials, want one", dials)
	}
}

// The classic daemon's numeric id maps back to the id Notify returned, and
// a Close queued right behind its Notify finds it.
func TestCloseFollowsItsNotify(t *testing.T) {
	f := &fakeBus{owned: classicName, classic: 42}
	n := NewWithDial(func() (Bus, error) { return f, nil })
	defer n.Shutdown()
	id := n.Notify(Notification{Title: "t"})
	n.Close(id)
	awaitMethod(t, f, classicCloseCall)
	if got := n.classicID(42); got != id {
		t.Errorf("classic 42 maps to %q, want %q", got, id)
	}
}

// The worker's queue is bounded: a stuck daemon drops what does not fit.
func TestStuckDaemonDropsPastTheQueue(t *testing.T) {
	f := &fakeBus{owned: portalDesktopName, release: make(chan struct{})}
	n := NewWithDial(func() (Bus, error) { return f, nil })
	for range 2 * queueDepth {
		n.Notify(Notification{Title: "t"})
	}
	close(f.release)
	n.Shutdown()
	sends := 0
	for _, m := range f.seen() {
		if m == portalAdd {
			sends++
		}
	}
	if sends == 0 || sends > queueDepth+1 {
		t.Errorf("%d sends, want at most the queue and the one running (%d)", sends, queueDepth+1)
	}
}

// Shutdown ends the worker and closes the connection; queued notifications
// went out first, and later ones are refused.
func TestShutdownSendsWhatIsQueued(t *testing.T) {
	f := &fakeBus{owned: portalDesktopName}
	n := NewWithDial(func() (Bus, error) { return f, nil })
	n.Notify(Notification{Title: "t"})
	n.Shutdown()
	if got := f.seen(); len(got) == 0 || got[len(got)-1] != portalAdd {
		t.Errorf("calls = %v, want the queued send last", got)
	}
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if !closed {
		t.Error("the connection stayed open")
	}
	before := len(f.seen())
	n.Notify(Notification{Title: "late"})
	if len(f.seen()) != before {
		t.Error("a notification after Shutdown reached the bus")
	}
}

// No session bus leaves an inert notifier: ids still come back.
func TestNoBusIsInert(t *testing.T) {
	dials := 0
	n := NewWithDial(func() (Bus, error) {
		dials++
		return nil, errors.New("no bus")
	})
	if n.Notify(Notification{Title: "t"}) == "" {
		t.Error("no id without a bus")
	}
	n.Notify(Notification{Title: "t2"})
	n.Shutdown()
	if dials != 1 {
		t.Errorf("%d dials, want one verdict", dials)
	}
}

// A bus owning neither name closes again and leaves the notifier inert.
func TestNeitherNameOwnedIsInert(t *testing.T) {
	f := &fakeBus{}
	n := NewWithDial(func() (Bus, error) { return f, nil })
	n.Notify(Notification{Title: "t"})
	n.Shutdown()
	if got := f.seen(); len(got) != 2 {
		t.Errorf("calls = %v, want the two probes alone", got)
	}
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if !closed {
		t.Error("the unused connection stayed open")
	}
}
