# Threading

## The contract

1. **One loop goroutine owns the UI.** `Application.Run` runs on one
   goroutine. Every widget mutation and every callback (`OnClick`,
   `OnKey`, `OnSelect`, drag and drop hooks, IME delivery, `Invoke`
   closures, `Every` ticks) runs there. Widgets take no locks.
2. **Other goroutines use `Application.Invoke(fn)`.** It queues `fn`
   and wakes the loop, which runs it at the start of the next pass.
   Do not read loop-owned state from other goroutines either; pass the
   value into the closure.
3. **Only the loop talks to the compositor.** The one exception is
   animated-cursor frames, under their own lock.
4. **Timers are loop callbacks.** `Application.Every(d, fn)` runs `fn`
   on the loop at a fixed cadence and returns a cancel function.

A tree may be built before `Run`, on the goroutine that will call
`Run`. After `Run` returns, teardown may touch widgets from any
goroutine.

```go
go func() {
	data := fetch()
	application.Invoke(func() { label.SetText(data) })
}()
```

## Wakes

The loop parks in `Session.Step` and wakes for compositor events, frame
callbacks and one deadline (`Session.SetDeadline`), the earliest pending
timer.

- **`Invoke`.** The first queued closure arms one immediate wake; a
  burst of `Invoke`s costs one wake and drains in one pass.
- **`Every`.** A deadline joins the timer set; nothing runs between
  ticks. Ticks anchor at fire time, so a slow frame delays the next tick
  instead of bursting.
- **Animations.** They tick on frame callbacks at the compositor's
  timestamp. Without one, they tick every 1/60 s while no surface awaits
  a callback and every 1/4 s while one does or none can show them, so an
  occluded tween still finishes. Each application has its own animation
  clock: what is launched from `NewApplication` until it closes, and
  what was launched while no application existed, runs on that
  application's loop alone and is dropped when it closes, so a later
  application in the process never steps it.
- **Requests.** Outgoing requests are buffered and flushed as one
  `sendmsg` before `Step` parks. When the socket is full, frames are
  skipped (not blocked) until it drains (`Session.Stalled`).

When `Run` returns, timers stop and queued `Invoke`s are dropped.

## Off-loop detection

`Run` marks its goroutine (`widget.MarkLoop`). `Invalidate`,
`InvalidateRect`, `SetTooltip`, and every setter that invalidates, check
it. A call from another goroutine fires the hook set with
`widget.SetOffLoopHook`. This samples common entry points; it is not a
lock. Disarmed, the check is one atomic load.

## Typed messaging

`app/message.go` builds on `Invoke`: the first send in a burst schedules
one drain, so many sends cost one wake and arrive as one ordered batch.

| Type | Use |
| --- | --- |
| `app.Stream[Msg]` (`NewStream`) | publish/subscribe; `Send` from any goroutine, `Subscribe` delivers on the loop |
| `app.SharedState[T]` (`NewSharedState`) | a value; `Get` from any goroutine, `Update` mutates and notifies on the loop |
| `app.Reducer[S, M]` (`NewReducer`) | `Emit(msg)` from any goroutine; `reduce` applies messages on the loop in order and subscribers hear only reported changes |

A send from inside a subscriber is delivered on the next pass.
Everything stops when the application closes, which `Run` does when it
returns and `Application.Close` does for an application that never
ran; `Application.OnStop` hooks run in reverse order. Component input and output queues use the same mailbox.
`cmd/lys-ui-messages` is the example.

## Off-loop work

Nothing the loop would wait on (a file, a bus, a subprocess, a peer)
runs on it. Each of these runs on a worker and returns through
`Invoke`:

| Area | Behaviour |
| --- | --- |
| Assets | Icons, stylesheet images, `@font-face` files and stylesheet reloads decode on one bounded queue. A paint draws what is decoded and never reads the disk. Without an `Application` loads run inline; tests wait with `widget.WaitAssetLoads`. Icons decode for the device scale passed by `widget.SetDeviceScale` before the first frame. |
| Animations | Not off-loop work, but what a test waits on the same way: `Application.Animating` reports whether a tween, delay, spring, spinner or surface fade of that application still runs. A test polls it on the loop until it is false before reading a settled frame, instead of sleeping. A spinner on screen keeps it true. |
| Font fallback | `app.FontFallback` resolves a missing rune on a goroutine and shapes with the primary face meanwhile; text reshapes once the answer lands. |
| Clipboard, drag and drop | The request leaves on the loop, the pipe is read or written on a transfer goroutine, and the result returns on the loop. `transfer.Content` writers run there. |
| Keymaps, cursors | A keymap compiles on the session's worker; keyboard events behind it wait, in order. Cursor files load there; common shapes are read ahead. |
| Desktop settings | The portal monitor runs on its own D-Bus goroutine ([Appearance](appearance.md)). |
| `app.Settings` | `Key.Set` updates memory; the file is written 250 ms later, coalescing bursts. `Run`'s exit and `Settings.Flush` flush. |
| `Application.Notify` | The bus call runs on a worker; when the bounded queue is full the notification is dropped. |
| `Inhibit`, `OnSessionStateChange`, `RequestBackground`, `SetBackgroundStatus` | Return at once and answer through `done` on the loop, in order on one worker. |
| `RegisterGlobalShortcut`, `GlobalShortcutsAvailable` | Answer through `done` on the loop. |
| AT-SPI bridge | Start, stop, publishing and diffing run on workers; the loop only builds the snapshot ([Accessibility](a11y.md)). |
| File picker | Listing, recents and the overwrite check run on goroutines. |
| `media.Stream` | Decodes ahead on its own goroutine; frames are taken on the loop's frame clock. Sound plays on the audio client's goroutine. `Close` returns at once; `Wait` joins the goroutines. Every method and subscriber runs on the loop. |
| GPU | Under `LYS_UI_RENDERER=auto` the device opens on a worker; windows draw on the CPU until it is ready and switch at a frame boundary. Buffer targets allocate on renderer workers. With linux-drm-syncobj, one worker waits on release points. |

`widget.GPUArea`'s `OnFrame` runs on the loop. An application that
renders on its own thread hands the finished buffer back through
`Invoke`.
