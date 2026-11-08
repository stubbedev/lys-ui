# Getting started

lys-ui is a pure-Go (no cgo) widget toolkit for Wayland. It runs on any
compositor with xdg-shell; sway and Hyprland are its CI gates.

## Install

```sh
go get github.com/stubbedev/lys-ui@latest
```

## A first app

A counter component with typed messages and a view built with the `ui`
builders (`component/example_test.go`):

<<< @/../component/example_test.go

`component.Run` opens the window and runs the loop.
[Components](../components) covers components, builders, factories,
workers and actions. Without components, use `app.NewApplication` and
`NewWindow` directly (`ExampleApplication` in `app/example_test.go`).

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
| `gtksettings` | GTK 4's settings.ini, read with GTK's file order |
| `atspitest` | real at-spi2-core for tests that check an app's accessibility tree |
| `highlight` | syntax highlighters for the code view |
| `media` | video and sound playback |
| `pdf` | PDF writing |
| `transfer` | the shared shape of clipboard and drag-and-drop content |

[lys-lib](https://github.com/stubbedev/lys-lib) provides the canvas
(`render`), the GPU renderer (`gpu`), screen capture (`capture`),
virtual input (`vinput`), the Wayland protocol bindings (`wlr`),
dmabufs, MP4/Matroska muxing and the Opus encoder.

The [demos](./demos) show the packages in use.

## The gallery

`just gallery` opens every component in one window. Ctrl+T toggles the
palette, Ctrl+K opens the command palette, Escape quits.
