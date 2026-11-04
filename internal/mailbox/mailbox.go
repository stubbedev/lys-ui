// Package mailbox is the coalesced-wake queue every loop messenger
// shares: items appended from any goroutine, drained in order on the
// loop by one closure the first append schedules.
package mailbox

import "sync"

const maxSpareItems = 1024

// Box queues items for one drain closure. The zero value is ready.
type Box[T any] struct {
	mu    sync.Mutex
	items []T
	spare []T
	armed bool
	dead  bool
}

// Put appends item and reports whether the caller must schedule the
// drain: true only for the first append since the last Take. A stopped
// box drops the item and reports false.
func (b *Box[T]) Put(item T) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dead {
		return false
	}
	b.items = append(b.items, item)
	if b.armed {
		return false
	}
	b.armed = true
	return true
}

// Take returns the queued items in order and disarms the box, so the
// next Put schedules a fresh drain. Handing the batch to Release when it is
// done with spares the next batch an allocation.
func (b *Box[T]) Take() []T {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := b.items
	b.items, b.spare = b.spare, nil
	b.armed = false
	return items
}

// Release takes back a batch Take returned, dropping what it references. A
// storm's oversized batch goes to the collector instead.
func (b *Box[T]) Release(batch []T) {
	clear(batch)
	if cap(batch) > maxSpareItems {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.spare == nil {
		b.spare = batch[:0]
	}
}

// Pending counts the queued items.
func (b *Box[T]) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// Stop drops the queued items and makes every later Put a no-op.
func (b *Box[T]) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dead = true
	b.items, b.spare = nil, nil
	b.armed = false
}

// Stopped reports whether Stop ran.
func (b *Box[T]) Stopped() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dead
}
