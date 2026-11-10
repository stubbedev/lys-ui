package widget

import "sync/atomic"

func installHook[F any](slot *atomic.Pointer[F], fn F, none bool) (remove func()) {
	if none {
		slot.Store(nil)
		return func() {}
	}
	slot.Store(&fn)
	return func() { slot.CompareAndSwap(&fn, nil) }
}
