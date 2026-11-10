# Architecture

The layers, and the invariants each one holds.

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
      |  per-window state       per-window pool, damage and pacing,
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
      |                         boundary; a recording Canvas captures
      v                         a frame as a List
    gpu                         plays a List with Vulkan into dmabufs
                                the compositor imports

Each layer talks only to its neighbours. Widgets never touch the wire,
the session knows nothing about widgets, and everything above the
session uses logical pixels.

## Renderer

`LYS_UI_RENDERER` picks the renderer:

| Value | Behaviour |
| --- | --- |
| unset, `auto` | One Vulkan device per application, opened on a worker. A window draws on the GPU once the device is ready and the compositor imports an ARGB8888 dmabuf layout it renders; otherwise on the CPU into shared memory. |
| `cpu` | Always the CPU. |
| `gpu` | The GPU or an error from `Run`. A failed GPU frame is fatal. |

Any other value is an error. Under `auto`, a failed GPU frame or a
refused dmabuf import is logged and moves every window to the CPU. A
reconnect chooses the renderer again.

A lost device is reopened under `auto`. The windows draw on the CPU
until the new device is there. A video decoding on the GPU
(`media.OpenFile`) follows them: its `widget.Video` gives up the
surface and the decoder of the lost device, the stream plays on by its
clock with its sound uninterrupted, and decoding resumes on the new
device from the keyframe before the playback position. The loss is
answered the same whether a window's frame, the video's decoding or the
drawing of a video frame finds it. A device lost more than three times
in a minute stays closed: the windows stay on the CPU and such a video
fails with `app.ErrNoGPURenderer`.

Both paths produce identical pixels. The GPU runs the canvas's
arithmetic step for step, and `just gpu` checks every widget golden on
both, byte for byte, under the Vulkan validation layer. The Vulkan
loader is found through the library path.

Image upload on the GPU:

| Image | Uploaded |
| --- | --- |
| `render.StillImage` (an `*image.RGBA` never changed) | once; icons, decoded images and cached rasters use it |
| `render.VersionedImage` | when its version changes, as for video frames |
| any other image | at every draw |

## Invariants

### 1. One connection, one loop, one goroutine

`Session.Step` and `Session.Roundtrip` are the only readers of the
connection, and one goroutine drives the loop. There are no per-window
goroutines. Key repeat, animation deadlines and tooltip dwell run on the
loop. `Session.WakeAfter` only moves the socket's read deadline; it
sends nothing to the compositor.

### 2. The loop parks

`Application.Run` blocks in `Session.Step`. It wakes for compositor
events, frame callbacks and one timer deadline: the earliest of key
repeat, animation frames, tooltip dwell, `Every` deadlines and `Invoke`
kicks. With nothing dirty and nothing scheduled the loop uses no CPU.

### 3. Redraw only damage, paced by frame callbacks

- `Invalidate`, `InvalidateRect` and `InvalidateLayout` record damage
  up the parent chain (`widget.CollectDamage`). A frame clips painting to
  the damage union, sends per-rect `damage_buffer`, and skips acquire
  and commit when nothing is dirty.
- Layout is GTK's measure protocol: `Measure(axis, forSize)` returns
  minimum and natural extent for one axis given the other, never clamped
  to the host. Each widget memoizes its recent requests, so a static
  tree measures nothing per frame.
- Each pooled buffer tracks the damage painted into other buffers since
  it was last shown and repaints that plus the frame's damage. On the
  GPU the stale rects are copied inside the frame's submission
  (`gpu.Carry`). A frame the device refuses (`gpu.ErrBusy`) stays owed.
- Arrange records a paint bound for each audited kit widget
  (widget/paintink.go), and `PaintChild` skips children outside the
  clip. Unknown types, including every `widget.Base` widget, are always
  visited.
- A scroll step that provably yields the same picture moves pixels
  instead of repainting (widget/scrollshift.go): the viewport is copied
  and only the exposed strip paints. A non-flat backdrop, an
  overlapping sibling, an effect on an ancestor or a step between
  device pixels repaints the whole viewport.

### 4. Resize before acquire; resize commits bypass pacing

Size and scale are checked before a buffer is acquired, because
resizing the pool destroys buffers. The commit for a configured size
ignores frame pacing, because compositors withhold the frame callback
until the surface commits at that size. Min/max limits are clamped
client-side in the same pass.

### 5. Occlusion parks, never spins

An occluded surface gets no frame callbacks. Dirty input still forces
the next redraw, and a running animation paces itself by timer once a
callback is a frame and a half late. Apps declare animations with the
`anim` package (`Animate`, `Easing`, `Sequence`, `Parallel`, `Delay`,
`Play`); nothing running means no deadline.

### 6. Logical coordinates; device scale only at paint

- Input routing, hit tests, caret rects and popup anchoring use logical
  surface pixels.
- Scales are rational in 120ths (`scale.Denom`; 1.25 is 150/120),
  matching `wp_fractional_scale_v1`.
- `render.Canvas` takes logical rects and covers every device pixel
  they touch.
- Text is shaped at its logical size and rasterized at the device scale;
  the glyph cache keys on scale.
- `preferred_scale` rescales a window in place: same router, focus and
  pool, rebuilt buffers, full repaint. Compositors without the
  fractional protocols use integer `set_buffer_scale`.

### 7. Premultiplied alpha

`render.Color` is premultiplied ARGB; blending is source-over. Straight
alpha appears only at colour construction, the PNG boundary and CSS
blend modes. Anti-aliased primitives multiply coverage into all four
channels. Every primitive has a row in the source-over conformance table
in lys-lib's `render/blend_test.go`; a new primitive needs one too.

### 8. Measure and Paint agree

A widget measures and paints from the same values. `Label.Measure` and
`DrawAligned` share `ShapedText.LineHeight`. A new widget computes
shared geometry once.

### 9. Buffers outlive the frames that read them

- The client never draws into a buffer the compositor holds. A resize
  drops busy buffers from rotation and keeps them alive until their
  release event; beyond pool capacity the oldest frame is retired.
- One anonymous file backs one `wl_shm_pool` per session. Buffers are
  sub-allocations, and growth truncates, resizes the pool and remaps:
  one fd and one mapping per session.
- Frame callbacks, sync callbacks and loop kicks unregister their
  proxies on `done`.

### 10. Input routes by surface, under an implicit grab

Every pointer event goes to the handler registered for its surface. A
press opens an implicit grab, so a drag keeps feeding the widget it
started on. A press on a resize edge is handled before the router.
`widget.KeyActionForSym` is the single keysym-to-action map. See
[Input model](input-model.md).

### 11. Layer surfaces size a zero axis automatically

As in gtk4-layer-shell, a zero width or height in `LayerConfig`:

- anchored on both edges of that axis, stretches;
- otherwise takes the root's natural size, re-measured before every
  frame (app/layerfit.go).

Corner-anchored OSDs and notification stacks need no size. Concrete
requests are validated against the protocol, and `Reshape` changes
anchor and size as one validated state.

### 12. Optional globals never gate Connect

Only `wl_compositor` and `wl_shm` are required. Every other global
binds optionally and callers check its presence or version. `NewWindow`
fails without `xdg_wm_base`, `NewLayer` without
`zwlr_layer_shell_v1`; a session with no `wl_output` has no outputs yet.

### 13. A surface declares its own opacity

Every shm buffer is ARGB8888 premultiplied (`buffer.Format`), never
XRGB. Opacity follows `WindowConfig.Background`:

| Background alpha | Behaviour |
| --- | --- |
| below 255 | Translucent. The compositor blends the surface; no opaque region is set. Hyprland blurs translucent layer surfaces; an alpha of 200–235 keeps text readable over the blur. |
| 255 | Opaque. `set_opaque_region` covers the surface minus any `GPUArea` or `Video` holes, re-sent only when the size or holes change. |

`WindowConfig.Opaque` forces the opaque region. Use it only when the
tree paints every pixel opaque.

## Theming

`widget.Theme` is one palette value: colour roles, metrics and motion.
Start from `DarkTheme`, `LightTheme` or `HighContrastTheme` and adjust
with `With*` methods, which return a copy:

```go
theme, err := widget.DarkTheme().WithAccentHex("#a6e3a1")
if err != nil {
	return err
}
widget.SetTheme(theme.WithPadding(8))
```

`SetTheme` replaces the palette globally and repaints every widget
(nil restores `DarkTheme`). Explicit per-widget colours (`Button.Bg`)
win over the palette, and [stylesheets](css.md) sit between the two.

Derived state colours are `Theme` methods, so every widget derives them
the same way:

| Method | Value |
| --- | --- |
| `HoverSurface` | `SurfaceHover`, else `Surface` mixed 8% toward `Text` |
| `PressedSurface` | `SurfacePressed`, else `Surface` mixed 20% toward black |
| `HoverAccent` | `Accent` at alpha 70 |
| `DisabledText` | `TextMuted`, else `Text` at 45% alpha |
| `DisabledSurface` | `SurfaceDisabled`, else `Surface` mixed halfway toward `Bg` |
| `DisabledAccent` | `Accent` at 45% alpha |

`SetTheme` checks opaque `Text`/`Bg` and `TextMuted`/`Bg` against WCAG
AA (4.5:1) and logs a Warn through `app.SetLogger` when they fail. The
theme is still applied.

### Enabled and read-only

- `SetEnabled(false)` works on every widget. Disabled controls paint
  muted, ignore pointer, keys and wheel, and are skipped by focus
  traversal. `widget.IsEnabled` walks ancestors per query, so
  re-enabling a container never re-enables a child disabled on its own.
  `widget.IsInteractive` still counts a disabled control: it swallows
  the press.
- `Entry.SetReadOnly` and `TextArea.SetReadOnly` block typing, paste,
  cut, composition and undo/redo. Selection, copy, caret motion and
  panning still work, and `SetText` still applies.
- Disabled fades the text; read-only fades only the caret.

## Non-goals

- **Per-window goroutines.** One loop, one goroutine. Background work
  reaches the loop through `Invoke`.
- **Window management.** No cascading, tiling or focus stealing.
- **Automatic theme switching.** The toolkit never changes its palette
  on its own. Apps follow the desktop through
  `Application.OnColorSchemeChange` ([Appearance](appearance.md)).
- **Subsurfaces, except for `GPUArea` and `Video`.** Each window draws
  into one buffer. Popover shadows and the fader paint into it; a drag
  icon is its own `start_drag` surface. No single-pixel buffers.
  - `widget.GPUArea` gets one desynchronized subsurface below the
    window, with an empty input region, a viewport sized to the area,
    and its own frame callbacks. The window paints a transparent hole
    over it, also cut from the opaque region.
  - `widget.Video` gets the same for a 4:2:0 stream when the compositor
    imports linear YUV420 or NV12 dmabufs and offers wp_viewporter and
    wp_color_representation_v1. Frames are copied into a ring of three
    udmabufs, tagged with the stream's matrix, range and the video
    content type; a frame with no free buffer is dropped. Otherwise the
    frames paint into the window, and `Video.OffloadErr` says why.
- **Commit timing** (wp_commit_timing_v1, wp_fifo_v1). Frame callbacks
  pace every commit.
- **Security context and toplevel drag.**
- **Colour management and HDR.** Surfaces carry no image description,
  so the compositor treats them as sRGB.
