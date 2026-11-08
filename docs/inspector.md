# Inspector

A widget-tree overlay over the live UI, a text dump of the tree, and a
doctor block describing the session. All three are compiled into every
build and read-only. While off, the inspector draws, traces and
allocates nothing.

## Turning it on

- `LYS_UI_INSPECT=1` (or `true`, `yes`, `on`) arms the inspector at
  startup with the overlay on.
- `Application.SetInspect(on)` and `ToggleInspect()` do it from code.

While armed, `ctrl+shift+i` toggles the overlay and `ctrl+shift+d`
dumps the tree to stdout. The chords are handled before widgets and
accelerators, and only while armed.

## The overlay

- each widget's bounds outlined, tinted blue or orange by depth;
- a label per widget: its debug name (`SetDebugName`) or type, with
  `[F]`, `[H]`, `[P]` for the focus, hover and press targets;
- the hovered subtree washed stronger;
- the hover target outlined in magenta, the focus target in green.

Layout, hit testing and damage are the same with the overlay on or off.

## The tree dump

`widget.DumpTree(root, router)` returns one indented line per widget in
paint order; `widget.DumpTreeTo(w, root, router)` writes it. The
`ctrl+shift+d` dump adds a banner:

    lys-ui tree dump: 5 widgets
    *widget.Box bounds=0,0+400x300
      *widget.Label bounds=4,4+392x16 role=label tooltip="the page title"
      *widget.Button name="toolbar:save" bounds=4,28+392x36 role=button tooltip="saves the thing" focusable hover pressed
      *widget.Entry bounds=4,72+392x27 role=entry focusable
      *widget.Slider name="zoom" bounds=4,107+392x18 role=slider focusable focused

| Field | Meaning |
| --- | --- |
| type | the Go type |
| `name=` | debug name, when set |
| `bounds=` | `X,Y+WxH` in surface pixels |
| `role=` | accessible role, when not none |
| `tooltip=` | accessible name, when any |
| `focusable`, `focused`, `hover`, `pressed` | flags; `hover` and `pressed` mark the router's targets; no flags without a router |

The format is stable (`widget/testdata/dump.golden`). Debug names are
plain data that survive text and state changes.

## The doctor block

`LYS_UI_DOCTOR=1` (or `true`, `yes`, `on`) prints it at startup:

    lys-ui doctor
    go: go1.27.1 (linux/amd64)
    globals: wl_compositor v4, wl_output v2, wl_seat v7, wl_shm v1
    outputs: DP-1 mode 2560x1440 scale 2 logical 0,0+1280x720
    cursor theme: Bibata-Modern-Ice 24px
    fonts: sans="DejaVu Sans" mono="JetBrains Mono" fallback=[Noto Sans CJK JP, Noto Color Emoji]
    buffer: ARGB8888 premultiplied (wl_shm format 0x0)
    fractional scale: yes (wp_viewporter + wp_fractional_scale_v1)

| Line | Shows |
| --- | --- |
| `globals` | every advertised interface and version (`Session.Globals`) |
| `outputs` | xdg-output name, mode, integer scale, logical geometry |
| `cursor theme` | the loaded xcursor theme, or why it failed |
| `fonts` | the resolved sans and monospace faces and installed fallbacks |
| `buffer` | the shm pixel format |
| `fractional scale` | fractional or integer `set_buffer_scale` |

`Application.Session().Inspect()` returns the same values as a struct.
