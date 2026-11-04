# lys-ui icons

How lys-ui resolves and paints themed icons.

## Pieces

- `internal/icons` — the freedesktop icon theme spec in pure Go:
  `index.theme` parsing (sections, `Inherits`, `Directories` /
  `ScaledDirectories`, per-directory `Size` / `Type` / `MinSize` /
  `MaxSize` / `Threshold` / `Scale`), the spec's lookup algorithm
  (exact-size pass, nearest-size pass, inheritance chain with hicolor
  last, unthemed base-directory fallback), `-symbolic` name fallback,
  and a raster cache keyed by (name, logical size, 120-based scale).
  The last tier is a bundled set of Lucide icons (lucide-static, ISC,
  `internal/icons/lucide`): freedesktop names such as
  `document-save-symbolic` map onto them, so stock chrome draws on a
  host with no icon theme, and an installed theme always wins.
- `widget.Icon` — five constructors: `NewIcon` (a raster the caller
  made), `NewThemeIcon` (theme name), `NewThemeIconFallback` (theme
  name, with fallbacks), `NewFileIcon` (png / svg / svgz
  path), `NewSVGIcon` (embedded bytes, for apps that ship their own
  glyphs). Icons rasterize at the canvas device scale, so a fractional
  rescale re-rasterizes crisply. `widget.ThemeIconExists` asks whether
  a theme name resolves, `widget.SetIconSearchPaths` replaces the base
  directories (GTK's `IconTheme.set_search_path`; nil restores the XDG
  default), and `widget.RefreshIcons` forgets every resolved icon so
  icons installed or removed since are picked up. `widget.SetIconNamespace`
  names an application's own icons by prefix and directory, so drawing
  them never lists the theme chain. A paint never reads or decodes: it
  draws what is decoded, and otherwise asks for the decode off the loop
  and keeps the raster it has until the answer repaints it
  ([threading.md](threading.md)). `widget.PreloadThemeIcon` decodes ahead
  of the first paint, so a view that shows an icon once
  `widget.ThemeIconPreloaded` says so never paints it blank.
- `render.Icon.Tint` — the recoloring primitive.

## Symbolic recoloring

Symbolic sources are `-symbolic` theme files and SVGs painting with
`currentColor` (oksvg rejects `currentColor` in strict mode, so
`internal/icons` rewrites it to opaque white before parsing). Recoloring
is a **mask pass**: the source is decoded once, and the tinted variant
keeps every pixel's alpha while taking the tint's color. This is what
GTK does for symbolic icons, and it survives opacity exactly
(anti-aliasing and `fill-opacity` carry into the result).

The honest limits of the mask approach: ink comes out one flat color.
Gradient hue variation collapses to its alpha ramp, and multi-color art
goes monochrome — which is the symbolic-icon contract, not a bug.
Gradients that encode their shape in *alpha* (the common symbolic idiom,
currentColor → transparent) survive unchanged.

A symbolic `widget.Icon` takes the stylesheet's `color`, else
`widget.Current().Accent`;
`SetTint` pins a different color (zero un-pins). Non-symbolic sources
keep their colors unless a tint is pinned. Widgets showing a theme icon in
one color share one raster; a widget whose tint then moves recolors a
raster of its own in place, reading no file.

## Icon-theme changes are followed live

The portal's `org.gnome.desktop.interface icon-theme` setting is
watched by the same monitor that carries the color-scheme preference
(internal/portalsettings). The application wires the two together:
`Monitor.OnIconThemeChange` feeds `icons.Default().ApplyIconTheme`, and
an `Application` does that wiring for you — a desktop theme switch
swaps the resolution theme, drops every cached raster, and bumps
`Generation`:

- `widget.Icon` compares its resolved generation against the cache in
  its damage drain, so the next frame after a switch repaints the new
  theme's icons; the application requests that repaint through the
  loop queue.
- An emptied setting keeps the previous theme (Warn, the
  degraded-but-running convention) — a broken portal never blanks the
  UI.
- Inside the toolkit, `icons.Cache.OnIconThemeChanged` hears about
  every applied switch, on the monitor's goroutine.

This is a lookup refresh, never a palette swap: the "toolkit never
flips its own theme" rule stands, and the color-scheme preference
stays the app's to wire ([appearance.md](appearance.md)).
