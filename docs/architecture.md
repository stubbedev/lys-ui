# lys-ui architecture

How lys-ui is put together, and the rules that keep it correct: the
layering first, then the invariants each layer must hold. Each rule
cites the code and the tests that pin it.

## Layers

    compositor
      |  one wayland connection
      v
    internal/wlsession          Session: the only reader of the
      |  routes per surface     connection (Step / Roundtrip /
      |                         WakeAfter), registry bindings, focus
      v                         and grab state, keymap, outputs
    internal/window             one Host per surface:
    internal/layersurface         xdg_toplevel | zwlr_layer_surface_v1
      |
      v
    app                         Application / Run: the parked loop,
      |  per-window state       hostWindow (pool, damage, pacing),
      |                         router wiring, dialogs, popovers,
      v                         tooltips, IME, drag and drop
    widget.Router               hover, grab, click, drag, focus,
      |                         scroll, dnd bubbling, key actions
      v
    widget.*                    the retained tree: Measure / Arrange /
      |                         Paint / HitTest, damage flags
      v
    render                      Canvas (premultiplied ARGB), shaped
      |                         text, icons; the only device-scale
      |                         boundary in the stack; a recording
      v                         Canvas captures a frame as a List
    gpu                         plays a List with Vulkan into dmabufs
                                the compositor imports

A window draws on the GPU when it can: `LYS_UI_RENDERER` unset or
`auto` opens one Vulkan device per application and, where the
compositor imports a dmabuf layout the device renders (ARGB8888 in a
shared modifier), records each frame and renders it into a dmabuf
buffer. The CPU canvas draws otherwise, into shared memory.
`LYS_UI_RENDERER=cpu` always draws on the CPU, and `=gpu` makes a GPU
that cannot draw an error from `Run`. Both paths paint the same pixels:
the GPU mirrors the canvas's integer arithmetic, and every widget
golden is checked on both (`just gpu`). The Vulkan loader is found
through the library path, which the nix packages set.

Each layer talks only to its neighbors: widgets never touch the wire,
the session never knows widgets exist, and everything above the session
speaks logical pixels. Demos in cmd/ compose these pieces; they contain
no loop plumbing of their own.

## The rules

### 1. One connection, one loop, one goroutine

`Session.Roundtrip` and `Session.Step` are the only readers of the
connection, and one goroutine drives the loop (internal/wlsession/
session.go, app/application.go). Wayland requests from one client must
stay ordered, and per-window goroutines would need a marshalling layer
to preserve that, so lys-ui has none (see Non-goals). Timer work (key
repeat, animation deadlines, tooltip dwell, animated cursors) runs on
small goroutines that only arm `Session.WakeAfter`, which kicks the
parked read with a `wl_display.sync` and returns.

### 2. The loop parks

`app.Run` and `Application.Run` block in `Session.Step` (one blocking
dispatch) instead of polling. Wakeups come from exactly three sources:
compositor events, frame callbacks, and timer deadlines armed through
`WakeAfter` — key repeat, animation frames, tooltip dwell, `Every`
poller deadlines, and `Invoke` kicks (app/invoke.go,
docs/threading.md; one armed wake covers an entire invoke storm).
`nextWake` (app/app.go) computes the earliest pending
deadline; with no dirty state and nothing scheduled the loop schedules
no kick and holds no CPU — idle is 0%, verified against headless sway.
`loopKicker.schedule` coalesces timer kicks: one outstanding kick
covers every deadline in its span, so an animation queues one sync per
frame, not one per pass.

### 3. Redraw only damage, paced by frame callbacks

Widgets record invalidation instead of apps blindly repainting:
`node.Invalidate` / `InvalidateRect` / `InvalidateLayout` walk the
parent chain into `widget.CollectDamage` (widget/damage.go,
widget/widget.go). The frame loop clips painting to the damage union,
sends per-rect `damage_buffer`, and skips buffer acquisition and
commit entirely when nothing is dirty. `Measure` is memoized per
widget against the constraints it was asked with (widget/widget.go),
so a static tree costs one cache hit at the root; edits and resizes
drop exactly the affected branch.

Partial repaint carries an obligation: it is only correct if the
acquired buffer already matches the screen outside the damaged region.
Each pooled buffer therefore tracks `Stale` — the damage painted into
other buffers since its content was last shown — and repaints Stale ∪
frame damage; fresh buffers start fully stale (internal/buffer/pool.go,
app/window.go). `Presented` records only what actually changed on
screen, so one buffer's init paint cannot poison the pool.

### 4. Resize before Acquire; a resize commit bypasses pacing

The draw path checks size and scale before acquiring a buffer:
`Pool.Resize` destroys buffers, and resizing after Acquire would
destroy the buffer about to be attached, a protocol error that ends
the connection. Configure relayout runs `syncSize` between events
(app/window.go) and marks the frame dirty.

The resize commit bypasses frame pacing (`shouldDraw`, app/app.go):
compositors withhold a surface's frame callback until it commits at
the configured size, so a paced repaint deadlocks the resize. That is
a liveness requirement, not politeness. Min/max limits are clamped
client-side in the same pass, so a compositor configuring outside the
limits still lays out inside them.

### 5. Occlusion degrades to parking, never spinning

A fully occluded surface stops receiving frame callbacks. The loop
must not spin waiting for one: dirty input still forces the next
redraw, and a running animation takes over pacing itself — the
animation clock's timer pushes a frame only once the callback has gone
unheard for a frame and a half (`frameOwed`, app/window.go). Animation
is timer-paced for the same reason. Apps declare timelines with the
public `anim` package (Animate, Easing, Sequence, Parallel, Delay,
Play), and the loop drives the scheduler in internal/animclock:
`Tick` advances the launched pieces on wakes, `Next` reports the next
frame deadline, and nothing running means no deadline and a full park.
Only ticks that ran callbacks mark windows dirty. The split keeps the
loop-driving calls out of the public API.

### 6. Logical coordinates end to end; the device scale exists only at the paint boundary

The widget tree — input routing, hit tests, caret rects, popup
anchoring — lives in logical surface pixels. The device scale appears
where the hardware needs it and nowhere else (internal/scale,
render/canvas.go):

- scales are rational and 120-based (`scale.Denom` = 120; 1.25 is
  150/120), matching `wp_fractional_scale_v1`'s wire unit;
- `render.Canvas` carries the rational scale; primitives take logical
  rects and land on device pixels rounded outward, so a logical rect
  covers every device pixel it spans;
- text is shaped once at its logical size and rasterized at the device
  scale — crisp at any factor, no glyph cache to invalidate;
- `preferred_scale` rescales the window in place (same router, focus,
  and pool; buffers rebuilt, whole window queued for repaint — the
  fresh buffers' full staleness is what guarantees the full repaint);
  compositors without the fractional protocols keep the integer
  `set_buffer_scale` path.

### 7. Premultiplied alpha, and anti-aliasing must respect it

`render.Color` is premultiplied ARGB end to end; blending is
source-over, and straight alpha exists only at the PNG boundary.
Anti-aliased primitives (signed-distance RoundedRect and Line)
multiply the *coverage into all premultiplied channels*, not just
alpha — scaling alpha alone leaves full-strength source color on edge
pixels and halos around every rounded control (render/canvas.go, pinned
by TestRoundedRectEdgeBlending and TestLine). Every primitive is held
to a shared source-over conformance table in render/blend_test.go:
when you add a primitive, add a row there — paint, probe points, and a
tolerance rationale — so its blending stays pinned against the suite's
independent reference compositor.

### 8. A widget's Measure and Paint must agree

Layout and painting derive from the same sources, or the widget
measures boxes its own painter refuses to fill into: a measure that
rounds a line height while the painter's fit check compares the exact
float lays out a label that paints nothing. `Label.Measure` and
`DrawAligned` share `ShapedText.LineHeight` (render/text.go), pinned by
TestLabelNaturalHeightPaints. When adding a widget, compute shared
geometry once and measure the thing you paint.

### 9. Buffers outlive the frames that read them; objects die with their done events

The shell never draws into a buffer the compositor holds: `Pool.Resize`
drops busy buffers from rotation but keeps the wl_buffer object and its
slot alive until the release event lands, retiring the oldest frame
beyond pool capacity instead of stacking unbounded pending buffers
(internal/buffer/pool.go). `ErrBusy` survives: no pending buffer is
ever handed out.

Storage is one arena per session: a single anonymous file backs one
`wl_shm_pool`, buffers are sub-allocations (offset + byte range), and
growth goes through truncate + `pool.resize` + remap — a session holds
exactly one fd and one mapping no matter how many windows resize how
often (internal/buffer/arena.go). `Session.Close` tears the arena down.

Done events are destructor events: frame callbacks, sync callbacks, and
loop kicks unregister their proxies after done, or a per-frame loop
leaks an object id per frame (app/app.go `frameDone`).

### 10. Input routes by surface, under an implicit grab

The session owns a focus/grab state machine and routes every pointer
event to the handler registered for its surface; surfaces never swap
global hooks (internal/wlsession, docs/input-model.md). A button press
opens a client-side implicit grab on the focus surface — drags keep
feeding the widget they started on — and a press on a resize edge is
intercepted before the router sees it, so the frame and the widget
model never fight over the same press. On the keyboard side,
`widget.KeyActionForSym` is the single keysym-to-action map; app-level
routing and popup hosts both delegate to it (widget/input.go), so an
editing key means the same thing everywhere.

### 11. A layer surface's zero axis is automatic, as in gtk4-layer-shell

zwlr_layer_surface_v1 leaves an axis to the compositor only when both
edges of that axis are anchored; requesting a zero width or height
otherwise is a protocol error. `LayerConfig` keeps gtk4-layer-shell's
meaning for a zero axis instead: anchored on both edges it stretches
(the size the wire carries is zero and the buffer falls back to the
output mode), and anchored on one edge or none it takes the root's
natural size, re-measured before every frame so the surface follows its
content (app/layerfit.go). An OSD or a notification stack anchored to a
corner therefore needs no size at all. The layer surface still validates
the wire rule for every concrete request (internal/layersurface), and
`Reshape` changes anchor and size as one validated state.

### 12. Optional globals are feature-detected and never gate Connect

Every protocol beyond the required core — xdg-decoration,
fractional-scale, text-input, the data device (version-capped) — binds
optionally, reports its presence or negotiated version, and the
callers gate on it. A compositor missing any of them gets the
pre-feature behavior byte for byte; none of them fail `Connect`
(internal/wlsession). Feature work adds capabilities without adding
requirements.

### 13. A surface declares its own opacity

Every shm buffer is ARGB8888 premultiplied — `buffer.Format`
(internal/buffer), never XRGB, because a translucent surface loses its
alpha channel to XRGB and composites as garbage. The whole alpha path
is pinned end to end in app/alpha_test.go: `Config.Background` through
`ClearDevice` (an overwrite, so the background is premultiplied
exactly once), widget blends on top, wl_shm bytes read back as B, G,
R, A.

Opacity is declared once per window and acted on by the frame
pipeline (app/window.go `syncOpaque`):

- **Background alpha < 255 — translucent, the panel shape.** The
  compositor blends the surface over whatever is behind it. Hyprland
  applies its blur to translucent layer surfaces automatically, so a
  wayle panel gets the blur look for free: keep the background in the
  200–235 alpha range (`render.RGBA(r, g, b, 216)` is a good default)
  — solid enough for text contrast, transparent enough for the blur to
  read. Translucent windows never set an opaque region: promising
  opacity there would make the compositor skip the very blend the
  panel exists for.
- **Background alpha == 255 — opaque.** The pipeline promises that to
  the compositor once per size change: `wl_surface.set_opaque_region`
  over the full device rect (region created from the compositor,
  destroyed right after the set), re-sent on resize and rescale,
  never per frame. The compositor can then skip blending behind the
  window entirely. An explicit `Config.Opaque` forces the promise —
  only sound if the tree really paints every pixel opaquely.

## Theming

The theme is a single palette value (`widget.Theme`): public color
roles and three metrics (radius, spacing, padding), constructed from
the `DarkTheme`/`LightTheme` presets and customized through `With*`
methods. Every `With*` returns a fresh copy and never mutates the
receiver, so a config-driven palette composes and branches without
aliasing:

	theme, err := widget.DarkTheme().WithAccentHex("#a6e3a1")
	if err != nil {
		return err // malformed config colors fail at construction, never silently default
	}
	widget.SetTheme(theme.WithPadding(8))

`SetTheme` swaps the palette globally and bumps the theme generation,
so the next frame repaints every widget (rule 3's damage walk).
Explicit per-widget colors (`button.Bg`) always win over the palette.

Stylesheets ([css.md](css.md)) restyle widgets per class and state
above the palette. Programmatic per-widget colors stay above the
cascade, and with no stylesheet loaded the palette alone decides.

**Derived state colors live on the palette, not in widgets.** Hover,
pressed, and disabled appearances are methods on `Theme`
(`widget/theme.go`), so a custom palette derives the same states as
the presets and every widget paints identically:

- `HoverSurface`: the explicit `SurfaceHover` when set, otherwise
  `Surface` mixed 8% toward `Text` — the text pole carries the
  palette's polarity, so dark palettes lighten and light palettes
  darken.
- `PressedSurface`: the explicit `SurfacePressed` when set, otherwise
  `Surface` mixed 20% toward black — the surface recedes under a
  press on any palette.
- `HoverAccent`: `Accent` at alpha 70 — the translucent wash rows and
  menu items paint under the pointer.
- `DisabledText`: the explicit `TextMuted` when set, otherwise `Text`
  faded to 45% alpha.
- `DisabledSurface`: the explicit `SurfaceDisabled` when set, otherwise
  `Surface` mixed halfway toward `Bg` — a disabled control recedes
  into the window, the inert counterpart of `PressedSurface`.
- `DisabledAccent`: `Accent` faded to 45% alpha — a switch's on-track,
  a slider's fill, a checkbox's tick while the control ignores input.

Widgets never mix these themselves; the presets pin explicit shades
that pass through untouched. Widgets also fade colors the palette
cannot know (an entry's constructor color, a button's child subtree
through the canvas alpha stack) by the same 45% fraction, so every
muted surface shares one derivation family.

**Enabled and read-only states.** `SetEnabled(bool)` exists on every
widget (it rides the shared `node`); disabled controls paint muted,
ignore presses, keys, drags, and wheel, and are skipped by focus
traversal. Propagation is a per-query ancestor walk
(`widget.IsEnabled`), not a rewrite: disabling a `Box` makes the whole
subtree read disabled while every widget keeps its own flag, so
re-enabling the box never resurrects a child the app disabled on
purpose. Disabled is not chrome: `widget.IsInteractive` still counts a
disabled control as interactive, because it still consumes — and
swallows — the press. Read-only (`Entry.SetReadOnly`,
`TextArea.SetReadOnly`) freezes user edits — typing, paste, cut,
composition, undo/redo, and new undo entries — while selection, copy,
caret motion, and pan keep working; `SetText` stays programmatic.
Visually the states are mirrors: disabled fades the text, read-only
fades only the caret.

**Contrast guard.** `SetTheme` checks `Text`/`Bg` and
`TextMuted`/`Bg` against WCAG AA (4.5:1) and reports a warning at
Warn on the injected library logger (`app.SetLogger`; nil keeps the
silent default). It warns and
applies; a theme is never rejected for its colors.

## Non-goals

- **No per-window goroutines.** There is one loop and one goroutine,
  so requests stay ordered (rule 1). Background work runs on its own
  goroutines and reaches the loop through `Invoke`.
- **No window manager.** lys-ui does no cascading, tiling or focus
  stealing; on Wayland that is the compositor's job.
- **No automatic theme switching.** The toolkit never changes its own
  palette, not even on a system dark/light change. The preference is
  offered as a signal (`Application.OnColorSchemeChange`), and the app
  decides whether to call `SetTheme` ([appearance.md](appearance.md)).
- **One kind of subsurface: GPUArea's. No single-pixel buffers.** lys-ui
  draws each window into one buffer per surface. Overlays (popover
  shadows, drag icons, the fader) are painted into that buffer, and
  spacers are rectangle fills inside the damage, so damage and pacing
  stay one problem per window. The one exception is `widget.GPUArea`,
  whose pixels the application renders outside lys-ui. Each area gets one
  desynchronized subsurface placed *below* its window, so everything
  lys-ui draws over the area stays on top. The surface shows through a
  transparent hole the area paints, which is also cut out of the opaque
  region. It has an empty input region, so the window keeps routing
  input, and a viewport sized to the area's bounds. Its frames pace
  themselves by the subsurface's own frame callbacks and leave the
  window's damage and pacing alone. No other widget gets a subsurface.
- **No commit timing** (wp_commit_timing_v1, wp_fifo_v1). Frame
  callbacks pace every commit (rule 3), and video presents on the
  frame clock like any other repaint; commit timing matters only to a
  presentation path that targets exact display times.
- **No security context and no toplevel drag.** Security context is
  for sandbox launchers, which lys-ui does not ship. Toplevel drag only
  matters for detachable tabs.
- **No color management or HDR.** A surface without an image
  description is handled as sRGB, which is what lys-ui paints, so its
  colors map correctly onto wide-gamut and HDR outputs. Showing content
  beyond sRGB would need per-surface image descriptions, a color-state
  canvas, 10-bit or half-float formats and blending in the right
  transfer function.
