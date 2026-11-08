# Icons

## Lookup

`internal/icons` implements the freedesktop icon theme spec in pure Go:

- `index.theme` parsing: `Inherits`, `Directories`/`ScaledDirectories`,
  and per-directory `Size`, `Type`, `MinSize`, `MaxSize`, `Threshold`
  and `Scale`;
- the spec's lookup: exact size, then nearest size, along the
  inheritance chain with hicolor last, then the unthemed base
  directories;
- `-symbolic` name fallback;
- a raster cache keyed by name, logical size and scale;
- a last tier of bundled [Lucide](https://lucide.dev) icons (ISC) that
  common freedesktop names such as `document-save-symbolic` map onto, so
  stock chrome draws without an installed theme. An installed theme
  always wins.

Programs that draw icons without widgets use the public `icontheme`
package.

## `widget.Icon`

| Function | Does |
| --- | --- |
| `NewIcon(ic)` | an icon from a caller-made `*render.Icon` |
| `NewThemeIcon(name, size)` | a theme icon |
| `NewThemeIconFallback(name, size)` | a theme icon that falls back to dash-shortened names (`a-b-c`, `a-b`, `a`) |
| `NewFileIcon(path, size)` | a PNG, SVG or SVGZ file |
| `NewSVGIcon(svg, size)` | embedded SVG bytes |
| `ThemeIconExists(name)` | whether a theme name resolves |
| `SetIconSearchPaths(paths)` | replaces the base directories; nil restores the XDG default |
| `SetIconNamespace(dir, prefixes)` | serves names with these prefixes from `dir` without walking the theme chain |
| `RefreshIcons()` | forgets resolved icons, picking up installs and removals |
| `PreloadThemeIcon`, `ThemeIconPreloaded` | decode ahead of the first paint |

Icons rasterize at the device scale, fractional included. A paint never
reads or decodes: it draws the decoded raster (or the nearest decoded
scale) and requests a decode off the loop ([Threading](threading.md)).

## Symbolic recolouring

Symbolic sources are `-symbolic` theme files and SVGs that paint with
`currentColor`. Recolouring is a mask pass: each pixel keeps its alpha
and takes the tint colour, so anti-aliasing and `fill-opacity` carry
over. Ink comes out in one flat colour; multi-colour art becomes
monochrome.

A symbolic icon takes the stylesheet `color`, else the theme accent.
`SetTint` pins a colour (zero unpins). Non-symbolic icons keep their
colours unless a tint is pinned. Icons with the same tint share one
raster.

## Theme changes

An `Application` starts with `gtk-icon-theme-name` from GTK's
`settings.ini` and follows the portal's
`org.gnome.desktop.interface icon-theme` setting live. A change swaps
the theme, drops cached rasters and repaints icons on the next frame.
An empty setting keeps the current theme and logs a Warn. The palette
is not affected.
