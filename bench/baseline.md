# Allocation and syscall baseline

What the hot paths cost, how the numbers are measured, and the
allocation budgets the test suite enforces. The budgets are hard
ceilings: a regression past one fails `just check`.

## Method

Machine: Intel i5-10400F (6 cores, 12 threads), Go 1.27, Linux, measured
2026-11-01 at load 3-20. Each benchmark runs six times; the tables give
the median. The canvas and text benchmarks live with their code in
lys-lib.

    go test -run '^$' -bench . -benchmem -count 6 ./widget ./app ./internal/icons
    go test -run TestAllocBudget -v ./app ./widget

`just bench` runs every benchmark once; CI archives that output for
trend watching and does not gate on it, since shared runners are too
noisy for timing. Allocation counts are deterministic, so the budget
tests gate those instead.

A change to a hot path is measured with benchstat over six runs
before and after, and leaves the goldens byte-identical
(`git diff -- '*testdata/golden*'` empty).

## Widget gallery

`buildShowcase` is the lys-ui-hello window shape: 75 widgets. The style
benchmarks load a 25-rule stylesheet over the same tree.
`settingsTree` is a settings window at 1605x1391.

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| ShowcaseMeasure (settled tree) | 31ns | 0 | 0 |
| StaticTreeMeasure (warm, cache hits) | 30ns | 0 | 0 |
| ShowcaseArrange (settled tree) | 6.6ns | 0 | 0 |
| ShowcasePaint (full frame: measure+arrange+damage+paint) | 353µs | 162 | 1 |
| ProgressOnlyFrame (one bar animating, damage-clipped) | 33.7µs | 32 | 1 |
| AutosizedListFrame (settled 200-row list, nothing changed) | 69ns | 0 | 0 |
| EntryPaint (steady entry repaint) | 49.6µs | 3 | 0 |
| SettingsFrame/layout (settled tree) | 66ns | 0 | 0 |
| SettingsFrame/paint (whole window) | 1.71ms | 0 | 0 |
| SettingsFrame/sweep (a new width each frame) | 2.04ms | 101Ki | 12 |
| SettingsFrame/resize (two widths in turn) | 2.13ms | 101Ki | 12 |
| StyleGalleryFullRestyle (match+compute, whole tree) | 271µs | 0 | 0 |
| StyleGalleryClassToggle (single restyle) | 2.4µs | 0 | 0 |
| StyleGalleryPaint (styled steady frame) | 260µs | 129 | 1 |

A frame costs what it changes. A box no invalidation reached since its
last arrange returns from Arrange at once, the damage walk skips a branch
with no mark, and a label outside the damage skips its paint. SettingsFrame's
101Ki is the harness collecting damage into a fresh rect list; a window
reuses its buffer.

## Input events

Per event, over the gallery. A gesture delivers tens of events per
frame, so each allocation here would multiply.

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| PointerMove (hover hit walk) | 274ns | 0 | 0 |
| WheelOverScroll (one wheel tick into a scrolled list) | 250ns | 0 | 0 |
| DragHover (drag-hover walk) | 250ns | 0 | 0 |

A hit walks the ancestors' `clip-path`s, since a clipped-out region
takes no pointer events.

## Icon lookup

72 names over a desktop-sized search path, as a settings page asks.

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| Lookup72/cold/scale1 (index built) | 1.67ms | 899Ki | 6300 |
| Lookup72/warm/scale1 | 7.6µs | 1344 | 36 |
| FirstNamespacedLookup/chain (one name, index built) | 12.6ms | 865Ki | 5844 |
| FirstNamespacedLookup/namespace (one name, `SetIconNamespace`) | 318µs | 9.6Ki | 122 |

## App loop

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| ProgressOnlyFrames (real `hostWindow` draw, fake wire) | 12.0µs | 342 | 0 |

The idle tick over a wired window allocates nothing
(`app.TestAllocBudgetIdleLoopTick`), and an idle loop schedules no
wakeup at all (`app.TestIdleLoopDoesNotWake` pins zero wakeups across
a 200ms idle window).

## Startup

From exec to the first committed frame, on private headless sway at load
4, median of 12. lys-ui-columns draws no themed icon; lys-ui-hello does.

| client | time |
|---|---|
| lys-ui-columns | 22ms |
| lys-ui-hello | 59ms |
| lys-ui-settings | 31ms |

The default look draws in the embedded Geist, so it scans no font
directory. The first system family or uncovered rune builds the font
store: 12-14ms with about 470 fonts and a warm index. The first themed
icon lists the theme's directories: 11.3ms, 6028 syscalls, on a desktop
whose theme chain declares 1323 of them.

## Steady-state syscalls

Measured with the compositor in the loop, in a private test environment
only (`just test-env <dir>`, its own `XDG_RUNTIME_DIR` and socket,
never the desktop session). Each demo runs under `strace -f -ttt`, and
its entries are counted over 5s after 3s of settling, resume lines
excluded. Per demo: `bench/data/syscalls-by-window.txt`.

A client with no timer armed is at zero: no syscalls and no context
switches, lys-ui-multi over a 10s window once its spare buffers are
trimmed, 5s after the last frame. Every syscall count above zero in that
file is the demo's own timer or animation.

A timer wake sends nothing: `Session.WakeAfter` sets the socket's read
deadline, so the compositor does not hear of it. lys-ui-gallery's spinner
costs 1.2-1.6% of a core at 60 frames a second, about 13 syscalls a frame
under strace, 8 allocations.

## Budgets

Each budget is the ceiling of 1.2x the measured count, and a change
that lowers a count tightens its budget in the same commit. They run
in `just check` through `go test`.

| budget | measured | ceiling | gate |
|---|---|---|---|
| gallery frame: measure+arrange+damage+paint | 1 (the harness's rect append) | 2 | `widget.TestAllocBudgetGalleryFrame` |
| whole-tree cascade rebuild | 0 | 0 | `widget.TestAllocBudgetStyleFullRestyle` |
| sibling-selector restyle of a wide sibling set | 0 | 0 | `widget.TestAllocBudgetSiblingRestyle` |
| input-method composing display reads | 0 | 0 | `widget.TestAllocBudgetComposingDisplay` |
| Shape of a stable line (warm cache) | 0 | 0 | `render.TestAllocBudgetShape` |
| ShapeRunes of a stable rune slice | 0 | 0 | `render.TestAllocBudgetShapeRunes` |
| DrawAligned over a fixed box (warm) | 0 | 0 | `render.TestAllocBudgetDrawAligned` |
| canvas primitive mix repaint | 0 | 0 | `render.TestAllocBudgetCanvasPaint` |
| idle loop tick | 0 | 0 | `app.TestAllocBudgetIdleLoopTick` |
| key delivery through window and app hooks | 0 | 0 | `app.TestAllocBudgetKeyDelivery` |
| icon reap over many windows with no icons | 0 | 0 | `app.TestAllocBudgetIconReap` |
| file watch drain | 0 | 0 | `app.TestAllocBudgetFileWatchDrain` |
| damage walk under a visible toast | 0 | 0 | `app.TestAllocBudgetToastDamageWalk` |
| damage-restricted window frame | 0 | 0 | `app.TestAllocBudgetWindowFrame` |
| widgets arranged in the first steady frame | 172 | 207 | `widget.TestSteadyLayoutArrangesEachWidgetOnce` |
| widgets arranged once settled | 0 | 0 | `widget.TestASettledTreeArrangesNothing` |

Two app gates count work instead of allocations:
`app.TestAllocBudgetPumpScopedDamageWalk` (a drained `Invoke` that
touched no tree runs no damage walk) and
`app.TestAllocBudgetTooltipPainterPass` (a settled tooltip runs no
paint pass, an owed frame exactly one).

The CSS engine has its own allocation-free gates:
`internal/style`'s `TestComputeVarFreeAllocatesNothing` and
`TestSelectorFormsAllocateNothingPerMatch` (the matcher and cascade),
`widget.TestEffectsFastPathAllocationFree` (a frame with no rendering
effect), and `widget.TestCSSLayoutSteadyFrameAllocatesNothing` and
`widget.TestCSSGridSteadyFrameAllocatesNothing` (static flex, flow and
grid trees).

## Known costs

Allocation and syscall sites the profiles show, each with its cost and
why it stays:

- **`widget.CollectDamage`'s rect list**: about one allocation per
  dirty frame when the caller keeps no buffer, as the benchmarks do. A
  window passes its own to `CollectDamageInto`.
- **`render.NewScaled` per non-window canvas** (`app/dragdrop.go`,
  `app/print.go`): one Canvas per dropped-image paint and per printed
  page. That is event-time work; the frame path reuses the window's
  canvas (`Canvas.Reset`).
- **The Go runtime's background monitor**: nanosleep calls during tweens,
  inflated by strace, none at idle. It is not lys-ui code.
- **Walk snapshot buffers** (`widget.walkStack`, `styleStack`,
  `walkTreeStack`, the per-parent sibling lists) and **the rune-keyed
  shaping table**: the buffers are retained at the deepest tree the
  process has walked, plus one filtered child list per parent with
  sibling selectors (a few hundred bytes to a few KB for a 48-row
  list); the rune table is fixed-size (512 sets × 4 ways). They are
  never freed; the bound is the tree's shape.
