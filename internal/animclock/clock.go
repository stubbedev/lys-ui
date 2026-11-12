// Package animclock is the instant animations read: the local clock, or
// the one a test pins with SetClock, and the conversion of a frame
// callback's timestamp onto it. The clocks that schedule animations live
// in package anim.
package animclock

import (
	"sync/atomic"
	"time"
)

var source atomic.Pointer[func() time.Time]

// Now is the animations' current instant: the pinned one under SetClock.
func Now() time.Time {
	if fn := source.Load(); fn != nil {
		return (*fn)()
	}
	return time.Now()
}

// SetClock replaces the time source launches and deadlines read and
// returns the restore function; nil restores time.Now.
func SetClock(fn func() time.Time) (restore func()) {
	var next *func() time.Time
	if fn != nil {
		next = &fn
	}
	old := source.Swap(next)
	return func() { source.Store(old) }
}
