<p align="center">
  <img src="docs/public/lys-ui.svg" width="160" alt="lys-ui">
</p>

<h1 align="center">lys-ui</h1>

<p align="center">
  A pure-Go (no cgo) widget toolkit for Wayland, built as a
  feature-complete counterpart of <a href="https://relm4.org">relm4</a>: a component
  framework with typed messages, a retained widget tree with
  GTK4/libadwaita-class widgets and every shadcn/ui component,
  xdg-shell and layer-shell windows, a GTK-flavored CSS engine over a
  typed palette, the desktop's portals and protocols, and an event loop
  that idles at 0% CPU.
</p>

<p align="center">
  <a href="https://github.com/stubbedev/lys-ui/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/stubbedev/lys-ui/ci.yml?branch=main&style=for-the-badge&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=for-the-badge" alt="MIT license"></a>
  <a href="https://stubbedev.github.io/lys-ui/"><img src="https://img.shields.io/badge/docs-stubbedev.github.io-informational?style=for-the-badge" alt="Documentation"></a>
</p>

<p align="center">
  <img src="https://stubbedev.github.io/lys-ui/components/card.png" width="380" alt="A shadcn/ui card in lys-ui">
</p>

The [documentation site](https://stubbedev.github.io/lys-ui/) shows a
screenshot of [every widget](https://stubbedev.github.io/lys-ui/widgets),
taken from the current code on every push.

## Status

lys-ui runs on sway and Hyprland, and both are required CI gates: the
compositor-in-the-loop suite runs against a private headless sway
(`just headless`) and against Hyprland in a private NixOS VM
(`just hyprland-vm`). [docs/completeness.md](docs/completeness.md) maps
every capability against relm4, GTK4 and libadwaita.

## Install

```sh
go get github.com/stubbedev/lys-ui@latest
```

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
| `render` | the premultiplied canvas, shaped text, icons |
| `anim` | tweens, easing, springs and timelines |
| `appearance` | the desktop's color scheme, accent and contrast preferences |
| `i18n` | gettext catalogs (`.mo`, `.po`) |
| `highlight` | syntax highlighters for the code view |
| `media` | video and sound playback |
| `pdf` | PDF writing |
| `transfer` | the shared shape of clipboard and drag-and-drop content |
| `dmabuf` | dma-buf frame descriptions |
| `capture` | screen and window capture |
| `vinput` | virtual pointer and keyboard injection |
| `wlr` | generated Wayland protocol bindings |

## A minimal app

```go
package main

import (
	"log"
	"strconv"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

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

func main() {
	err := component.Run(app.WindowConfig{Title: "counter", AppID: "dev.example.counter"},
		func(a *app.Application) (*Counter, error) {
			face, err := app.Font("sans-serif", 15)
			if err != nil {
				return nil, err
			}
			if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
				return nil, err
			}
			return &Counter{env: ui.Env{Face: face, Size: 15}}, nil
		})
	if err != nil {
		log.Fatal(err)
	}
}
```

This program is kept compiling as `ExampleRun` in
`component/example_test.go`. [docs/components.md](docs/components.md)
walks through components, the typed builders, factories, workers and
actions. `app.NewApplication` and `NewWindow` are the lower-level
path for apps that want the widget tree without components
(`ExampleApplication` in `app/example_test.go`).

## Development

```sh
nix develop        # pinned Go, gopls, golangci-lint, just, sway; CGO_ENABLED=0
just demo          # the lys-ui-hello showcase
```

The gates, all required in CI:

```sh
just check         # go vet, golangci-lint, go test, build
just fmt-check     # gofumpt and gci formatting
just headless      # the compositor-in-the-loop suite on a private headless sway
just hyprland-vm   # the same suite against Hyprland in a private NixOS VM (needs KVM)
just atspi         # the AT-SPI bridge on a private accessibility bus
just atspi-verify  # the bridge against real at-spi2-core
```

`just bench` runs the benchmarks; CI archives the numbers without
gating on them ([bench/baseline.md](bench/baseline.md) holds the
method and the allocation budgets the tests enforce). `just goldens`
regenerates the golden-image snapshots after a deliberate visual
change.

`nix build .#lys-ui` builds the module and every demo with the pinned
toolchain. Pushing a `v*` tag releases `lys-ui-hello`, `lys-ui-bar` and
`lys-ui-panel` (linux/amd64 and arm64) through goreleaser.

## Documentation

| Document | Contents |
| --- | --- |
| [docs/completeness.md](docs/completeness.md) | the capability map against relm4/GTK/libadwaita, and the known gaps |
| [docs/architecture.md](docs/architecture.md) | the layers, the invariants that keep them correct, theming, non-goals |
| [docs/application-model.md](docs/application-model.md) | windows, layer surfaces, disconnects, single instance, file dialogs, data transfer, chrome |
| [docs/components.md](docs/components.md) | the component framework: components, typed view builders, tracking, controllers, children, testing |
| [docs/threading.md](docs/threading.md) | the loop goroutine, `Invoke`/`Every`, the off-loop guard, the typed messaging layer |
| [docs/input-model.md](docs/input-model.md) | routing, implicit grab, click/drag, drag and drop, keyboard, IME, touch, tablets |
| [docs/css.md](docs/css.md) | the CSS layer: selectors, values, properties, cascade, lifecycle |
| [docs/appearance.md](docs/appearance.md) | following the desktop's color scheme, accent and contrast |
| [docs/icons.md](docs/icons.md) | icon-theme lookup and symbolic recoloring |
| [docs/a11y.md](docs/a11y.md) | semantic roles, the keyboard-first guarantee, the AT-SPI bridge |
| [docs/inspector.md](docs/inspector.md) | the live widget inspector, tree dump, and doctor block |

## Demos

| Command | What it shows |
| --- | --- |
| `cmd/lys-ui-hello` | the showcase as a component: core widgets, a context menu on typed actions, tooltips, Tab focus, drag-to-move |
| `cmd/lys-ui-bar` | a layer-shell top bar repainting only what changed each second |
| `cmd/lys-ui-panel` | a right-anchored layer-shell panel with live widgets |
| `cmd/lys-ui-launcher` | a launcher: a keyboard-exclusive overlay, a filtered app grid, live toplevel thumbnails through `capture` |
| `cmd/lys-ui-osd` | an OSD: a transient layer-shell volume/brightness popup over the overlay layer |
| `cmd/lys-ui-lock` | a lock screen: one ext-session-lock surface per output with an auth entry |
| `cmd/lys-ui-multi` | one loop: per-output layer bars, on-demand windows, close-request veto |
| `cmd/lys-ui-invoke` | a lifetime-bound spawned command and `Every` feeding a component |
| `cmd/lys-ui-messages` | the typed messaging layer and single-instance forwarding |
| `cmd/lys-ui-settings` | a settings app: PreferencesView rows, the adaptive shell, typed persisted settings |
| `cmd/lys-ui-columns` | ten thousand sorted, filtered rows; the tree view; drag-to-reorder |
| `cmd/lys-ui-dialogs` | every dialog constructor: message, prompt, color, font, calendar, file, adaptive, about |
| `cmd/lys-ui-dnd` | cross-window drag-and-drop: swatches that drag and drop between two toplevels |
| `cmd/lys-ui-draw` | custom painting: a DrawingArea animating straight onto the Canvas |
| `cmd/lys-ui-ime` | IME composition: preedit and commit mirrored from an entry and a text area |
| `cmd/lys-ui-a11y` | a labeled form served over AT-SPI; builds with `-tags atspi` to force the bridge on |
| `cmd/lys-ui-capture` | the capture consumer: list, screenshot, and monitor outputs and windows to PNG |
| `cmd/lys-ui-gpu` | a GPUArea paced by its own frame callbacks, with lys-ui drawing over it |
| `cmd/lys-ui-video` | a video with its controls: a file through ffmpeg, or a generated clip decoded in pure Go |
| `cmd/lys-ui-i18n` | a gettext .po catalog (plurals included) localizing built-in and app strings |
| `cmd/lys-ui-states` | the window-state client the headless suite drives: xdg state chords, mirrored from what the compositor confirms |
| `cmd/lys-ui-multilist` | the multi-select client the headless suite drives: a multiple-selection list with keyboard and rubber-band selection |
| `cmd/lys-ui-popover` | the popover client the headless suite drives: popup grabs, live repaints, nested menus |
| `cmd/wlpointer`, `cmd/zz-vpclick` | synthetic input for the headless suite |

## Dependencies

All pure Go:
[neurlang/wayland](https://github.com/neurlang/wayland),
[godbus/dbus](https://github.com/godbus/dbus),
[go-text/typesetting](https://github.com/go-text/typesetting),
[rivo/uniseg](https://github.com/rivo/uniseg),
[golang.org/x/image](https://pkg.go.dev/golang.org/x/image),
[golang.org/x/text](https://pkg.go.dev/golang.org/x/text),
[golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys),
[srwiley/oksvg](https://github.com/srwiley/oksvg) +
[rasterx](https://github.com/srwiley/rasterx),
[kettek/apng](https://github.com/kettek/apng),
[yalue/native_endian](https://github.com/yalue/native_endian),
[unxed/xkb-go](https://github.com/unxed/xkb-go), and
[jfreymuth/pulse](https://github.com/jfreymuth/pulse) (the PulseAudio
protocol, which pipewire-pulse also serves). `media.OpenFile` runs
`ffmpeg` and `ffprobe` from PATH when an app plays a file; nothing else
needs them.

## License

MIT
