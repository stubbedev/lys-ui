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
without components.

## Packages

Apps import the public packages; everything under `internal/` is
implementation.

| Package | Contents |
| --- | --- |
| `component` | the component framework: components, controllers, factories, workers, commands |
| `ui` | typed view builders, one per widget |
| `app` | the session, windows and layer surfaces, the loop, dialogs, popovers, desktop services |
| `widget` | the widget tree, theme, CSS, actions, bindings |
| `widget/css` | libadwaita's style classes and named colors as constants |
| `render` | the premultiplied canvas, shaped text, icons |
| `anim` | tweens, easing, springs and timelines |

The [README](https://github.com/stubbedev/lys-ui#packages) lists every
package and demo.

## The gallery

```sh
just gallery
```

opens every component in one window. Ctrl+T toggles the
palette, Ctrl+K opens the command palette, and Escape quits.
[Widgets](../widgets) shows each one.
