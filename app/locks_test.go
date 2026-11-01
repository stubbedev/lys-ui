package app

import (
	"testing"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

func TestOnLocksReportsNowAndEachChangeUntilStopped(t *testing.T) {
	a := NewApplication(&wlsession.Session{})
	var seen []Locks
	stop := a.OnLocks(func(l Locks) { seen = append(seen, l) })
	if len(seen) != 1 || seen[0] != (Locks{}) {
		t.Fatalf("registering reported %v, want the current state once", seen)
	}
	a.wireSession()
	a.sess.OnLocks(Locks{Caps: true})
	if len(seen) != 2 || !seen[1].Caps {
		t.Errorf("a change reported %v", seen)
	}
	stop()
	a.sess.OnLocks(Locks{})
	if len(seen) != 2 {
		t.Errorf("a stopped watcher saw %v", seen)
	}
}
