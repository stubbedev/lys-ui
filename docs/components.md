# Components

`package component` is a relm4-style component framework. A component
is a model with typed input and output messages. `Init` builds the
view, `Update` handles each input on the loop goroutine, and a
controller owns its lifetime and forwards its outputs.

## A component

```go
type Msg int

const (
	Increment Msg = iota
	Decrement
)

type Counter struct {
	env ui.Env
	n   int
}

func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	return ui.Mount(cx, c.env, ui.Row(
		ui.Button(ui.Label("-"), 8, 6).OnClick(func() { cx.Input(Decrement) }),
		ui.Expand(ui.Label("").WatchText(func() string { return strconv.Itoa(c.n) })),
		ui.Button(ui.Label("+"), 8, 6).OnClick(func() { cx.Input(Increment) }),
	).Spacing(6))
}

func (c *Counter) Update(cx *component.Context[Msg, int], msg Msg) {
	switch msg {
	case Increment:
		c.n++
	case Decrement:
		c.n--
	}
	cx.Output(c.n)
}
```

- The struct is the model; dependencies (`ui.Env`,
  `*app.Application`) are fields.
- `Init` runs once and returns the root widget.
- `Update` runs per input, in send order, never reentrantly. Input sent
  from `Update` arrives on the next pass.
- Optional: `UpdateView(cx)` runs after each batch of updates;
  `Shutdown(cx)` runs when the component stops, and its outputs are
  still delivered.

## Views

`package ui` has a typed builder for every widget, generated from
package widget (`go generate ./ui`; a test fails when it is stale).

| relm4 `view!` | `ui` |
| --- | --- |
| `gtk::Label { set_label: "x" }` | `ui.Label("x")`, then any setter without `Set`: `.Wrap(true)` |
| `connect_clicked => Msg::Inc` | `.OnClick(func() { cx.Input(Inc) })`, for every exported hook |
| `#[watch] set_label: ...` | `.WatchText(func() string { ... })` for every setter, or `.Watch(func(*widget.Label))` |
| `#[track(cond)]` | `.Track(func(*widget.Label), deps...)` |
| `#[name = "x"]` | `.Ref(&field)` |
| child widgets | constructor and adder arguments take nodes: `ui.Button(ui.Label("+"), 8, 6)`, `.PackStart(n)`, `.AppendTab("t", n)` |
| `gtk::Box` children | `ui.Column(...)`, `ui.Row(...)`, with `ui.Expand(n)` and `ui.Aligned(n, a)` |
| `set_hexpand`, `set_halign`, `set_margin_all` | `.HExpand(true)`, `.HAlign(widget.AlignCenter)`, `.Margin(render.UniformInsets(12))` |
| `if` / `match` | `ui.If(cond, then, else)`, `ui.Match(key, ui.When(k, n)...)`, re-checked after every update |
| `#[iterate]` | `ui.Each(seq, fn)` |
| `#[local_ref]` | `ui.Use(w)` for a widget built elsewhere |
| `#[relm4::widget_template]` | a function returning a `ui.Node` |

Every builder also has the shared widget props, each with a `Watch`
form where it takes a value: `Tooltip`, `TooltipMarkup`, `AddClass`,
`ID`, `Enabled`, `Visible`, `InlineStyle`, `State`, `Attr`,
`HExpand`/`VExpand`, `HAlign`/`VAlign`, `Margin`, `SizeRequest`,
`MaximumSize`, `AccessibleName`, `DebugName`, `CursorName`,
`OnFocusChanged`, `OnPointer`, `OnScroll`, and `With(func(W))` for
anything else.

`ui.Env` holds the typeface, text size and text colour builders use
when a constructor needs them. A zero `Ink` leaves the colour to the
cascade. `Font(face, size)` and `Ink(c)` override them per widget.

- `ui.Mount(cx, env, node)` builds inside a component: watches refresh
  after updates and bindings (`BindText`, ...) unbind on shutdown.
- `ui.Build(env, node)` builds standalone and returns the root and a
  `*ui.View` with `Refresh` and `Close`.

A builder referenced from two places (a `ui.Stack` passed to
`ui.ViewSwitcher`) builds once.

## Watching and tracking

| Call | Runs |
| --- | --- |
| `cx.Watch(fn)` | now and after every batch (relm4 `#[watch]`) |
| `cx.Track(fn, deps...)` | now, and after a batch only if a dep changed (relm4 `#[track]`) |
| `cx.Tracked(v)` | creates a `*component.Tracked[T]`; `Set` and `Update` mark it, `Changed()` reads the mark, which clears after each refresh |

```go
func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	c.count = cx.Tracked(0)
	return ui.Mount(cx, c.env, ui.Label("").
		Track(func(l *widget.Label) { l.SetText(strconv.Itoa(c.count.Get())) }, c.count))
}
```

A refresh runs `UpdateView`, then watches and tracks in registration
order, then clears marks.

Window properties follow the model with `cx.WindowTitle(get)`,
`cx.WindowMaximized(get)`, `cx.WindowFullscreen(get)`,
`cx.WindowMinSize(get)` and the general `cx.WatchWindow(fn)`; each
requests a change only when the value moves. `cx.Window()` and
`cx.Layer()` return the surface the component or an ancestor opened in.
The window does not exist during `Init`, so window watches first run
after it opens.

## Launching

```go
ctrl, win, err := component.Window(application, app.WindowConfig{
	Title: "counter", AppID: "dev.example.counter",
}, &Counter{env: ui.Env{Face: face, Size: 15}})
```

| Function | Does |
| --- | --- |
| `component.Run(cfg, setup)` | connects, builds the root with `setup`, opens a window, runs the loop |
| `component.Window(a, cfg, c)`, `component.Layer(a, cfg, c)` | makes `c` the root of a toplevel or layer surface; it shuts down when the surface closes |
| `component.Dialog(a, parent, cfg, c)`, `component.Popover(a, host, cfg, c)` | runs `c` as dialog or popover content |
| `component.Launch(loop, c)` | starts `c` with no surface |
| `cx.Launch(c)` | starts a child of the current component |

Closing a dialog or popover shuts the component down, and shutting the
component down (`ctrl.Shutdown()`, `cx.Shutdown()`) closes it. A dialog
component implementing `DialogResponder` gets `OnResponse(cx,
response)` before shutdown.

`*Controller[In, Out]`:

| Method | Does |
| --- | --- |
| `Widget()` | the root widget |
| `Send(msg)`, `Sender()` | input, from any goroutine |
| `Forward(fn)` | outputs to `fn`, on the loop |
| `ForwardTo(sender, f)` | outputs mapped by `f` into another component |
| `Detach()` | outlive the owner, until the loop stops |
| `Shutdown()`, `Alive()` | stop; check whether running |
| `State()` | the `*StateWatcher` for the model |

## Children

```go
func (p *Parent) Init(cx *component.Context[ParentMsg, struct{}]) widget.Widget {
	child := cx.Launch(&Counter{env: p.env}).
		ForwardTo(cx.Sender(), func(n int) ParentMsg { return CounterMoved{n} })
	return ui.Mount(cx, p.env, ui.Column(ui.Label("counter"), ui.Use(child.Widget())))
}
```

`Context.Launch` and `ForwardTo` are generic methods (Go 1.27); the
child's message types are inferred.

A parent's shutdown stops its children (last launched first), then
runs its own `Shutdown`, then its `OnShutdown` hooks. Top-level
components stop with the loop. `Detach` hands a child to the loop.

### Observing a child's model

```go
unwatch := child.State().Subscribe(func(c *Counter) {
	p.total.Set(c.n)
})
```

- The callback runs on the loop once per batch of the child's updates,
  with the live model. It must only read it.
- A `Send` from the callback runs on the next pass.
- There is no initial call.
- The unsubscribe function is idempotent and safe from any goroutine;
  the child's shutdown releases all subscriptions.
- `State().Model[T]()` returns the current model; read it on the loop.

## Commands and async

| Call | Does |
| --- | --- |
| `cx.Oneshot(func(ctx) In)` | runs on a goroutine and delivers the result to `Update` |
| `cx.Spawn(func(ctx, emit))` | runs on a goroutine; each `emit` delivers a message |
| `cx.Context()` | the context both receive, cancelled at shutdown; later results are dropped |
| `cx.Await(func(ctx) func())` | runs work on a goroutine, then applies the returned function on the loop |
| `cx.Busy()` | true while an `Await` is pending |

```go
case Save:
	cx.Await(func(ctx context.Context) func() {
		err := store.Save(ctx, c.doc)
		return func() { c.saveErr = err }
	})
```

While an `Await` is pending, later input waits, including the rest of
the current batch. Shutdown cancels it and the result never applies.

A component implementing `Loader` initializes asynchronously:

1. `Loading()` returns the widget shown first.
2. `Load(ctx)` runs on a goroutine and may fill in the model.
3. `Init` runs on the loop afterwards; its view replaces the loading
   one.

Input sent while loading is held until `Init`. Shutting down mid-load
cancels `Load` and skips `Init`.

## Factories

`Factory[C, In, Out]` renders a collection of item components into a
container (relm4 `FactoryVecDeque`). Each item is a full component.
`cx.Index()` returns its `*component.Index`; `Current()` follows edits.

```go
func (l *List) Init(cx *component.Context[ListMsg, struct{}]) widget.Widget {
	var box *widget.Box
	root := ui.Mount(cx, l.env, ui.Scroll(ui.Column().Ref(&box)))
	l.rows = cx.NewFactory(component.BoxView[*Row](box, false)).
		ForwardTo(cx.Sender(), func(x *component.Index, o RowOut) ListMsg { return Removed{x.Current()} })
	for _, item := range l.items {
		l.rows.PushBack(&Row{env: l.env, item: item})
	}
	return root
}
```

- Edits apply in place: `PushBack`, `PushFront`, `Insert`, `Remove`,
  `Move`, `Swap`, `Clear`. Kept items keep their widgets; moved items
  keep focus and selection.
- Read with `Len`, `Get(i)`, `All()`; send with `Send(i, msg)` and
  `Broadcast(msg)`.
- `Forward`/`ForwardTo` receive the emitting item's index.
- Removed items shut down. `cx.NewFactory` ties the factory to the
  component; `component.NewFactory(loop, view)` makes a top-level one
  (`Detach`, `Shutdown`).
- Items implementing `Loader` show their loading widget in place, hold
  input until `Init`, and keep their slot through `Move` and `Swap`.

`KeyedFactory[K, C, In, Out]` (relm4 `FactoryHashMap`) addresses items
by key: `Insert(k, c)` replaces in place; `Get`, `Has`, `Index`,
`Remove` and `Send` take the key; `All` yields keys in order;
`SortFunc` (or `component.Sort` for ordered keys) reorders in place;
outputs forward with the key.

```go
users := cx.NewKeyedFactory[UserID](component.BoxView[*UserRow](box, false))
users.Insert(u.ID, &UserRow{user: u})
```

Views: `BoxView`, `FlowBoxView`, `StackView` (named pages),
`NotebookView` (titled tabs), `GridView` (cells placed by index),
`FixedView` and `ListBoxView`. The container holds only the factory's
items. Implement `FactoryView[C]` for anything else. For large
virtualized data use `widget.TypedList[T]`, `widget.List` or
`ColumnView[T]` with a model.

## Workers

`Worker[In, Out]` is a background actor. `Update(msg, emit)` runs on the
worker's goroutine in send order; each `emit` reaches the forward route
on the loop.

```go
type Indexer struct{ db *Index }

func (x *Indexer) Update(path string, emit func(Indexed)) {
	emit(Indexed{path, x.db.Add(path)})
}

func (a *App) Init(cx *component.Context[Msg, struct{}]) widget.Widget {
	a.indexer = cx.LaunchWorker(&Indexer{db: a.db}).
		ForwardTo(cx.Sender(), func(r Indexed) Msg { return IndexDone{r} })
	...
}
```

`component.LaunchWorker(loop, w)` starts a top-level worker;
`cx.LaunchWorker(w)` one owned by the component. `WorkerController`
has `Send`, `Sender`, `Forward`, `ForwardTo`, `Detach` and `Shutdown`.
On shutdown, queued input is dropped, the current update finishes, and
an optional `Shutdown()` runs on the worker goroutine.

## Actions

Actions are typed values that menu items, buttons, toggles and
accelerators bind to.

```go
save := widget.NewAction("save", func() { cx.Input(Save) })
wrap := widget.NewStateAction("wrap", true, func(on bool) { cx.Input(WrapChanged(on)) })
theme := widget.NewStateAction("theme", "dark", func(s string) { cx.Input(ThemeChanged(s)) })

application.AddAccel("ctrl+s", save)
application.DescribeAction(save, "Files", "Save the document")

menu := []widget.MenuItem{
	widget.ActionItem("Save", save),
	widget.CheckItem("Wrap lines", widget.Toggle(wrap)),
	widget.RadioItem("Dark", theme.Target("dark")),
	widget.RadioItem("Light", theme.Target("light")),
}
ui.Button(ui.Label("Save"), 8, 6).BindAction(save)
```

- `SetEnabled(false)` on an action disables every proxy, and its
  accelerator lets the key through.
- `ParamAction[P]` (`NewParamAction`) takes a value:
  `open.Target(path)` is a bindable activation.
- `StateAction[S]` shows its state on every proxy; `widget.Toggle`
  (bool) and `Target(v)` (radio) are its checkable views.
- Menus opened through the application show each action's accelerator.
- `AddAccel(keys, action)` binds app-wide; `AddScopedAccel(scope, keys,
  action)` binds within a subtree, and the innermost scope wins.

## Messaging

- `cx.Input(msg)` and `Sender.Send(msg)` work from any goroutine. A
  burst costs one loop wake and runs as one batch.
- `cx.Output(msg)` goes to the controller's forward route on the loop;
  without a route it is dropped.
- `app.Stream`, `app.SharedState` and `app.Reducer` work alongside
  components ([Threading](threading.md#typed-messaging)).

## Testing

`component/componenttest.Loop` runs components without a compositor.
`Pass` runs one loop pass, `Settle` runs until idle, `Stop` ends it like
`Run` returning.

```go
var loop componenttest.Loop
ctrl := component.Launch(&loop, &Counter{env: env})
ctrl.Send(Increment)
loop.Settle()
```
