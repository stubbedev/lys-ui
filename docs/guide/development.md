# Development

```sh
nix develop        # pinned Go, gopls, golangci-lint, just, sway; CGO_ENABLED=0
just demo          # the lys-ui-hello showcase
```

## Gates

CI runs all of these:

```sh
just fmt-check     # gofumpt and gci formatting
just check         # go vet, golangci-lint, go test, build
just race          # the suite under the race detector
just gpu           # the GPU renderer's tests on lavapipe
just headless      # the compositor-in-the-loop suite on a private headless sway
just hyprland-vm   # the same suite against Hyprland in a private NixOS VM (needs KVM)
just atspi         # the AT-SPI bridge on a private accessibility bus
just atspi-verify  # the bridge against real at-spi2-core
just package       # the Nix package with its sandboxed tests
```

`just docker <recipe>` runs a recipe in a container that sees only the
dev shell's store paths, the checkout, its own Go caches, `/dev/kvm` and
the GPU render and driver nodes. It has no DRM card nodes, host runtime
dir, session bus or display socket. `just docker race 16g` raises its
memory cap.

| Recipe | Does |
| --- | --- |
| `just bench` | runs the benchmarks; CI archives the numbers without gating ([bench/baseline.md](https://github.com/stubbedev/lys-ui/blob/main/bench/baseline.md) has the method and the allocation budgets the tests enforce) |
| `just goldens` | regenerates the golden-image snapshots |
| `just gallery` | opens the gallery |
| `just docshots` | screenshots every widget for this site |

## Builds and releases

`nix build .#lys-ui` builds the module and every demo. A `v*` tag
releases `lys-ui-hello`, `lys-ui-bar` and `lys-ui-panel` for
linux/amd64 and arm64 through goreleaser.

## Security

`.github/workflows/security.yml` runs CodeQL (Go and GitHub Actions),
Grype and govulncheck. Dependency review gates pull requests.
Dependabot updates Go modules, workflow actions and the docs site's npm
packages.

## Dependencies

Pure Go, no cgo. Direct dependencies:
[lys-lib](https://github.com/stubbedev/lys-lib),
[stubbedev/memory-go](https://github.com/stubbedev/memory-go),
[golang.org/x/image](https://pkg.go.dev/golang.org/x/image) and
[golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys).

lys-lib adds
[ebitengine/purego](https://github.com/ebitengine/purego),
[kettek/apng](https://github.com/kettek/apng),
[rivo/uniseg](https://github.com/rivo/uniseg),
[golang.org/x/net](https://pkg.go.dev/golang.org/x/net) and
[golang.org/x/text](https://pkg.go.dev/golang.org/x/text). Its
`third_party` directory holds patched copies of
[neurlang/wayland](https://github.com/neurlang/wayland),
[godbus/dbus](https://github.com/godbus/dbus),
[go-text/typesetting](https://github.com/go-text/typesetting),
[srwiley/oksvg](https://github.com/srwiley/oksvg),
[srwiley/rasterx](https://github.com/srwiley/rasterx),
`golang.org/x/image/vector`, the portable parts of Go's `math`, and the
Vulkan bindings of [gogpu/wgpu](https://github.com/gogpu/wgpu).

Found at runtime:

- `libvulkan`, opened through purego by the GPU renderer. Without it,
  windows draw on the CPU, and `media.OpenFile`'s video, which decodes
  on the renderer's Vulkan video queue, does not play.
