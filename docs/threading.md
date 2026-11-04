# lys-ui threading model

Who may touch a widget, when, and how everything else gets in.

## The contract

1. **One loop goroutine owns the UI.** `Application.Run` runs on one
   goroutine. Every widget mutation and every event callback
   (`OnClick`, `OnKey`, `OnSelect`, dnd hooks, IME delivery, `Invoke`
   closures, `Every` ticks) runs there. lys-ui's widgets take no locks;
   the single-thread rule is the lock.
2. **Other goroutines reach the tree only through
   `Application.Invoke(fn)`** (app/invoke.go). `Invoke` queues the
   closure and wakes the parked loop, which runs `fn` at the top of its
   next pass with the guarantees of an event callback. Reading
   loop-owned state from another goroutine is off-limits too: hand the
   value into the closure instead.
3. **The compositor connection is loop-only.** Requests from one client
   must stay ordered ([architecture.md](architecture.md) rule 1), so
   nothing outside the loop goroutine issues protocol requests, except
   animated-cursor frames, under their own lock.
4. **Timer callbacks are loop callbacks.** `Application.Every(d, fn)`
   runs `fn` on the loop at a fixed cadence. No timer thread exists
   whose callback could race the tree.

Library code follows the same rule. `media.Stream` decodes on its own
goroutine a few frames ahead and hands frames over through channels
that the Stream drains on the loop's frame clock. Its sound plays on
the audio client's goroutine, which shares only atomics: frames played, closed and drained. Every Stream method, and every subscriber it calls,
runs on the loop. A `widget.GPUArea`'s `OnFrame` runs on the loop too.
An application that renders on its own thread hands the finished
buffer back through `Invoke` and presents it there.

A tree may be built before `Run` starts, on the goroutine that will
call `Run`. After `Run` returns, the guard disarms and teardown may
touch widgets from any goroutine.

## How `Invoke` wakes the park

The loop parks in `Session.Step`. It wakes for compositor events, frame
callbacks, and the timer deadlines `nextWake` computes (app/app.go).
Enqueueing an `Invoke` arms a single pending wake with one
`Session.WakeAfter(0)`. The armed flag lives under the queue mutex, so
a storm of Invokes arms one wake, and the loop drains the whole queue
in one pass.

`Every` deadlines join the `nextWake` set. A poller wakes the park
exactly at its deadline and costs nothing in between. Ticks anchor at
fire time, so a slow frame delays the next tick instead of bursting to
catch up. Timers stop when `Run` returns, and queued Invokes are
dropped, so a worker outliving the windows cannot pile closures onto a
dead application.

## Detection: the off-loop hook

`Application.Run` marks its goroutine (`widget.MarkLoop`). The sampled
mutation entry points (`Invalidate`, `InvalidateRect`, `SetTooltip`,
and through `Invalidate` every `SetText`-style mutation) check it. A
mutation from any other goroutine fires the hook installed by
`widget.SetOffLoopHook`. Disarmed, the check costs one atomic load; armed with a hook, it also
parses the goroutine id. It is a
debugging aid that samples the common choke points, not a lock.

## The typed messaging layer

`app/message.go` builds typed messengers on the same coalesced-wake
rule as `Invoke`: a mailbox whose first append routes one drain
closure through `Invoke`, so a hundred `Send`s cost one loop wake and
deliver as one ordered batch.

- **`app.Stream[Msg]`**: typed publish/subscribe. `Send` works from any
  goroutine, and `Subscribe(fn)` delivers on the loop. A `Stream` held
  in a package variable is relm4's `MessageBroker`.
- **`app.SharedState[T]`**: a loop-notified value. `Get` snapshots from
  any goroutine. `Update` mutates the value and notifies subscribers on
  the loop.
- **`app.Reducer[S, M]`**: relm4's `Reducer`. `Emit(msg)` from any
  goroutine, and the `reduce` function applies it on the loop in emit
  order. Subscribers hear the state only when `reduce` reports a
  change, so the message type is the complete set of transitions.

Everything stops when `Run` returns (`Application.OnStop` hooks run in
reverse registration order). Delivery is deferred: a `Send`
from inside a subscriber or update runs on the next pass.
`cmd/lys-ui-messages` shows both, with a component on the click path.

Clipboard and drag-and-drop transfers (internal/xfer) run the same way.
The request leaves on the loop, the pipe is read or written on a
transfer goroutine, and the result comes back through `Invoke`, so a
peer that never answers costs a paste that stays pending, never a
frame. A `transfer.Content` writer runs on such a goroutine. Transfers
are bounded, and `Run` returning ends and joins them.

The desktop-settings monitor is the background-worker shape: it
watches the portal on its own dbus goroutine, and `Application`
bridges each change onto the loop through `Invoke`
([appearance.md](appearance.md)).

Components ([components.md](components.md)) ride the same mailbox: a
component's input and output queues coalesce their wakes exactly like
`Invoke`.

## What never runs on the loop

No file, bus or subprocess the loop would wait on runs on the loop, so
no slow service can drop a frame:

- **Settings** (`app.Settings`): `Key.Set` changes memory and encodes
  the one value. The file follows 250 ms later on a timer goroutine, a
  burst of Sets coalesced into one write. Run's exit flushes, and
  `Settings.Flush` does it on demand.
- **Desktop notifications** (`Application.Notify`): the id is made
  locally, and the dial and the bus call run on the notifier's worker.
  A daemon that stopped answering costs a dropped notification once the
  bounded queue is full.
- **Session portals** (`Inhibit`, `OnSessionStateChange`,
  `SetBackgroundStatus`): each returns at once and answers through a
  `done` callback on the loop. They run in order on one worker, so a
  query-end answer follows any `Inhibit` the callback made.

## The relm4 mapping

| relm4 | lys-ui |
| --- | --- |
| `Component`, `SimpleComponent` | `component.Component[In, Out]`, launched with `component.Launch`/`Window`/`Layer` or `Context.Launch` |
| `ComponentSender::input`/`output` | `Context.Input`/`Output`, `Sender[M]` |
| `Controller`, `Connector::forward` | `component.Controller` with `Forward`/`ForwardTo`/`Detach` |
| `tracker`, `#[watch]`, `#[track]` | `Context.Tracked`, `Watch`, `Track` |
| `MessageBroker` | a `Stream` in a package variable |
| `SharedState`, `Reducer` | `app.SharedState[T]`, `app.Reducer[S, M]` |
| `Worker`, `detach_worker` | `component.Worker` with `LaunchWorker` and `WorkerController` |
| `Command`, `oneshot_command`, `spawn_command` | `Context.Oneshot`, `Context.Spawn`, cancelled at shutdown |
| `AsyncComponent` | a component implementing `component.Loader` |
| `FactoryVecDeque` | `component.Factory` of item components, edited in place |
| `TypedListView` (virtualized) | `widget.TypedList[T]`, `widget.List` + `ListModel`, `ColumnView[T]` |
| `RelmAction`, `RelmActionGroup` | `widget.Action`, `StateAction[S]`, `AddAccel`/`AddScopedAccel` |
| `glib::timeout_add` | `Application.Every` |

## Tests

- app/invoke_test.go pins the wake budget on a fake clock: a 100-deep
  invoke storm arms one wake, and one `Every` deadline costs one wake
  per tick and none after cancel.
- app/message_test.go pins the messengers: 100 `Send`s route one
  invoke, order is kept, fan-out reaches every subscriber, cancels take
  effect at the next message, and nothing outlives `Run`.
- `TestInvokeRunsOnLoopGoroutine` (app) and
  `TestOffLoopMutationTripsHook` (widget) pin the goroutine identity.
- `go test -race ./...` is clean, including a four-producer invoke
  storm.
