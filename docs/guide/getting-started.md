# Getting started

lys-ui is a pure-Go (no cgo) widget toolkit for Wayland. It runs on any
compositor with xdg-shell; sway and Hyprland are its CI gates.

## Install

```sh
go get github.com/stubbedev/lys-ui@latest
```

## A first app

A counter: a component with typed messages, its view built with the `ui`
builders. This is `component/example_test.go`, kept compiling as an
example test:

<<< @/../component/example_test.go

`component.Run` opens the window and runs the loop.
[Components](../components) walks through components, the typed
builders, factories, workers and actions. `app.NewApplication` and
`NewWindow` are the lower-level path for apps that want the widget tree
without components (`ExampleApplication` in `app/example_test.go`).

## Packages

Apps import the public packages; everything under `internal/` is
implementation.

| Package | Contents |
| --- | --- |
| `component` | the component framework: components, controllers, factories, workers, commands |
| `component/componenttest` | a loop that runs components in tests without a compositor |
| `ui` | typed view builders, one per widget |
| `app` | the session, windows and layer surfaces, the loop, dialogs, popovers, desktop services |
| `widget` | the widget tree, theme, CSS, actions, bindings |
| `widget/css` | libadwaita's style classes and named colors as constants |
| `anim` | tweens, easing, springs and timelines |
| `appearance` | the desktop's color scheme, accent and contrast preferences |
| `gtksettings` | GTK 4's settings.ini, read with GTK's file order |
| `icontheme` | freedesktop icon theme lookup for programs that draw icons without widgets |
| `i18n` | gettext catalogs (`.mo`, `.po`) |
| `highlight` | syntax highlighters for the code view |
| `media` | video and sound playback |
| `pdf` | PDF writing |
| `transfer` | the shared shape of clipboard and drag-and-drop content |

The canvas widgets paint on (`render`), the GPU renderer (`gpu`), screen
capture (`capture`), virtual input (`vinput`), the Wayland protocol
bindings (`wlr`), dmabufs, MP4/Matroska muxing and the Opus encoder live
in [lys-lib](https://github.com/stubbedev/lys-lib), which lys-ui imports.

The [demos](./demos) show the packages at work.

## The gallery

```sh
just gallery
```

opens every component in one window. Ctrl+T toggles the
palette, Ctrl+K opens the command palette, and Escape quits.
[Widgets](../widgets) shows each one.
