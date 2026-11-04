package app

import (
	"slices"
	"sync"

	"github.com/stubbedev/lys-ui/internal/mailbox"
)

// subscriberSet is the fan-out half of a messenger: callbacks added from
// any goroutine, snapshotted per message for delivery on the loop in
// subscription order.
type subscriberSet[Msg any] struct {
	mu   sync.Mutex
	next int
	subs []subscriber[Msg]
}

type subscriber[Msg any] struct {
	id int
	fn func(Msg)
}

func (s *subscriberSet[Msg]) add(fn func(Msg)) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	s.subs = append(s.subs, subscriber[Msg]{id: s.next, fn: fn})
	return s.next
}

func (s *subscriberSet[Msg]) remove(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = slices.DeleteFunc(s.subs, func(sub subscriber[Msg]) bool { return sub.id == id })
}

func (s *subscriberSet[Msg]) snapshot() []subscriber[Msg] {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.subs)
}

// Stream is a typed publish/subscribe messenger onto the loop goroutine:
// Send publishes from any goroutine, Subscribe registers a callback that
// receives every message in Send order. It is the one shape behind the
// relm4 concepts that are all fan-out under different names: a message
// broker (a Stream held in a package variable, so any nesting depth
// reaches it without threading senders) and SharedState change
// notifications.
//
// Delivery is deferred and non-reentrant: a Send from inside a subscriber
// (or from the loop itself) lands at the top of the loop's next pass.
type Stream[Msg any] struct {
	app  *Application
	subs subscriberSet[Msg]
	box  mailbox.Box[Msg]
	stop func()
}

// NewStream creates a Stream on the application's loop. After the loop
// ends it comes back stopped: Sends are dropped, subscriptions never
// fire.
func NewStream[Msg any](a *Application) *Stream[Msg] {
	s := &Stream[Msg]{app: a}
	s.stop = a.OnStop(s.box.Stop)
	return s
}

// Send publishes msg to every subscriber on the loop goroutine. Safe from
// any goroutine; a burst of Sends drains as one batch per loop pass.
func (s *Stream[Msg]) Send(msg Msg) {
	if s.box.Put(msg) {
		s.app.Invoke(s.drain)
	}
}

// Subscribe registers fn to receive every message on the loop goroutine.
// The returned cancel removes the subscription (idempotent, safe from any
// goroutine); it takes effect at the next message, so a cancel racing a
// pending batch never rewinds deliveries already made.
func (s *Stream[Msg]) Subscribe(fn func(Msg)) (cancel func()) {
	if fn == nil {
		return func() {}
	}
	id := s.subs.add(fn)
	return sync.OnceFunc(func() { s.subs.remove(id) })
}

// Close stops the stream: queued messages are dropped, later Sends are
// no-ops, and subscriptions stop firing. The loop's end closes it too.
func (s *Stream[Msg]) Close() {
	s.box.Stop()
	s.stop()
}

// drain is the closure Send routes through Invoke: it takes the batch and
// hands each message to the subscribers live at that message.
func (s *Stream[Msg]) drain() {
	batch := s.box.Take()
	for _, msg := range batch {
		for _, sub := range s.subs.snapshot() {
			sub.fn(msg)
		}
	}
	s.box.Release(batch)
}

// SharedState is a loop-owned observable value: Update mutates it from any
// goroutine and delivers the new value to every subscriber on the loop
// goroutine - relm4's SharedState, with the change stream being the same
// Stream shape every other messenger uses. One state shared across
// windows updates every window's widgets from the one subscription point,
// which is what makes cross-window sharing correct by construction: there
// is no per-window copy to fall out of sync.
type SharedState[T any] struct {
	mu      sync.Mutex
	val     T
	changes *Stream[T]
}

// NewSharedState creates a SharedState holding initial, notifying on the
// application's loop.
func NewSharedState[T any](a *Application, initial T) *SharedState[T] {
	return &SharedState[T]{val: initial, changes: NewStream[T](a)}
}

// Get returns a snapshot of the value, safe from any goroutine: the read
// guard. Copying is the guard - a snapshot cannot be raced by a later
// Update.
func (s *SharedState[T]) Get() T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.val
}

// Update applies f to the value and notifies subscribers with the new
// value on the loop goroutine. Notification is unconditional, like
// relm4's set: f already knows whether it changed anything, and callers
// that need change detection compare against Get inside f.
func (s *SharedState[T]) Update(f func(*T)) {
	s.mu.Lock()
	f(&s.val)
	next := s.val
	s.mu.Unlock()
	s.changes.Send(next)
}

// Subscribe registers fn to receive the value after every Update, on the
// loop goroutine. The returned cancel removes the subscription.
func (s *SharedState[T]) Subscribe(fn func(T)) (cancel func()) {
	return s.changes.Subscribe(fn)
}

// Close stops change notifications; later Updates still mutate the value.
func (s *SharedState[T]) Close() {
	s.changes.Close()
}

// Reducer is shared state changed only through typed messages, relm4's
// Reducer: Emit sends a message from any goroutine, reduce applies it
// to the state on the loop in emit order, and subscribers hear the new
// state only when reduce reports a change. The set of transitions is
// the message type, and nothing else writes the state.
type Reducer[S, M any] struct {
	app    *Application
	mu     sync.Mutex
	state  S
	reduce func(*S, M) bool
	box    mailbox.Box[M]
	subs   subscriberSet[S]
	stop   func()
}

// NewReducer creates a Reducer at initial, reducing with reduce on the
// application's loop. After the loop ends it comes back stopped.
func NewReducer[S, M any](a *Application, initial S, reduce func(state *S, msg M) (changed bool)) *Reducer[S, M] {
	r := &Reducer[S, M]{app: a, state: initial, reduce: reduce}
	r.stop = a.OnStop(r.box.Stop)
	return r
}

// Emit queues msg for reduction on the loop. Safe from any goroutine.
func (r *Reducer[S, M]) Emit(msg M) {
	if r.box.Put(msg) {
		r.app.Invoke(r.drain)
	}
}

// Get returns a snapshot of the state, safe from any goroutine.
func (r *Reducer[S, M]) Get() S {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Subscribe registers fn to receive the state after every reduction
// that changed it, on the loop goroutine.
func (r *Reducer[S, M]) Subscribe(fn func(S)) (cancel func()) {
	if fn == nil {
		return func() {}
	}
	id := r.subs.add(fn)
	return sync.OnceFunc(func() { r.subs.remove(id) })
}

// Close stops the reducer: queued messages are dropped and later Emits
// do nothing.
func (r *Reducer[S, M]) Close() {
	r.box.Stop()
	r.stop()
}

func (r *Reducer[S, M]) drain() {
	batch := r.box.Take()
	defer r.box.Release(batch)
	for _, msg := range batch {
		r.mu.Lock()
		changed := r.reduce(&r.state, msg)
		state := r.state
		r.mu.Unlock()
		if !changed {
			continue
		}
		for _, sub := range r.subs.snapshot() {
			sub.fn(state)
		}
	}
}
