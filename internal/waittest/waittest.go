// Package waittest bounds what a test waits for from another goroutine,
// so that a hang fails the test, naming what it waited for, instead of
// stalling the run until the test binary's own timeout.
package waittest

import (
	"testing"
	"time"
)

var limit = 10 * time.Second

const pollInterval = 2 * time.Millisecond

// For returns the next value received from ch, failing the test when
// none arrives within the limit. A closed ch answers at once, which makes
// a channel closed as a signal one to wait for too.
func For[T any](t testing.TB, ch <-chan T, what string) T {
	t.Helper()
	timeout := time.NewTimer(limit)
	defer timeout.Stop()
	select {
	case v := <-ch:
		return v
	case <-timeout.C:
		t.Fatalf("waited %v for %s", limit, what)
		panic("unreachable")
	}
}

// Until polls cond until it holds, failing the test when it still does
// not after the limit.
func Until(t testing.TB, cond func() bool, what string) {
	t.Helper()
	if cond() {
		return
	}
	poll := time.NewTicker(pollInterval)
	defer poll.Stop()
	timeout := time.NewTimer(limit)
	defer timeout.Stop()
	for !cond() {
		select {
		case <-poll.C:
		case <-timeout.C:
			t.Fatalf("waited %v for %s", limit, what)
			return
		}
	}
}
