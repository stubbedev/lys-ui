# CSS

A GTK-flavoured CSS engine above the typed `Theme`. The palette works
with no stylesheet loaded; stylesheets override properties where rules
match. GTK application stylesheets load unchanged.

## API

```go
s, err := widget.AddStylesheetFile("theme.css", widget.StylePriorityTheme) // own sheet, hot-reloaded
widget.LoadStylesheetFile("app.css")             // the application slot, hot-reloaded
widget.LoadStylesheet(`.title { color: #c00 }`) // from a string; "" removes it
settings := widget.AddStylesheet(css, widget.StylePrioritySettings)
settings.Load(newCSS)                           // replace its rules
parsed := widget.ParseStylesheet(newCSS)        // any goroutine
settings.LoadParsed(parsed)                     // on the loop, no parse
settings.Remove()

b := widget.NewButton(child, 10, 8)
b.AddClass("destructive")
b.SetID("save-button")
b.SetState(widget.StateSelected, true)          // :selected, :checked, :backdrop, ...
b.SetInlineStyle("padding: 4px")
b.SetAttr("data-kind", "danger")                // [data-kind="danger"]
box.AttachStylesheet(widget.NewStylesheet(css, widget.StylePriorityApplication))
```

- **Priorities** (GTK's provider priorities, low to high):
  `StylePriorityFallback`, `Theme` (regenerated on `SetTheme`),
  `Settings`, `Application` (the `LoadStylesheet` slot), `User` (inline
  styles). An app default that the theme's state rules should still
  beat (a transparent button fill under the theme's `:hover`) belongs
  at `StylePriorityTheme`, added after the theme; at `Application` it
  beats every theme rule.
- **Scoped sheets.** `AttachStylesheet` applies a sheet to a subtree.
  `NewStylesheetFile(path, priority)` loads one from a file.
- **Hot reload.** `AddStylesheetFile`, `LoadStylesheetFile` and
  `NewStylesheetFile` reload when the file changes, through the
  `Application`'s inotify watch (no polling; without an `Application`
  there is no hot reload). A file that vanishes or stops parsing keeps
  its last good rules and reports through `widget.SetParseWarn`. An
  initial load that cannot read or parse fails with an error naming the
  path. `Remove` or `Load` stops watching.
- **Fonts.** `font-family` and `font-weight` resolve through
  `widget.SetFaceResolver` (for example over `app.FontWeighted`), after
  `@font-face` families. Without a resolver the constructor face stays.
  Variable faces take `font-weight` on their wght axis.
- **Warnings** go to the library logger, or to `widget.SetParseWarn`.
- **rem.** `widget.SetRootFontSize` sets the initial root font size
  (16).

## Syntax

- CSS Syntax 3 tokenizer. A malformed declaration drops that
  declaration; a malformed selector drops its rule (one bad member
  invalidates the list).
- `!important`.
- Native nesting: `&` is the parent selector list with `:is()`
  matching and specificity (`&:hover`, `& > .child`, `.x &`, `& + &`).
  A nested selector without `&` is a descendant of the parent (or uses
  its leading combinator). A nested list with an invalid member drops
  that nested rule only.
- `@keyframes` (and `@-gtk-keyframes`).
- `@media`: see [Media state](#media-state). Features `width`,
  `height`, `aspect-ratio`, `orientation`, `resolution` (`dpi`, `dpcm`,
  `dppx`, `x`) with `min-`/`max-` and range syntax (`(400px <= width <=
  800px)`), `prefers-color-scheme`, `prefers-contrast`,
  `prefers-reduced-motion`; `not`, `and`, `or`, nesting, comma lists;
  media types with `not`/`only` (`screen` and `all` match). Evaluation is
  three-valued (Level 4): unknown features and `<general-enclosed>` are
  unknown, and an unknown query is false. A `@media` inside a rule scopes
  that rule's remaining declarations.
- `@supports`: `(prop: value)`, `selector(...)`, `not`, `and`, `or`,
  evaluated at parse time against what the engine supports.
- `@import "file.css";` or `url(...)`, with optional `layer` /
  `layer(name)`, `supports(...)` and a media query. Paths resolve
  beside the importing file, or the working directory for string
  sheets. Cycles report the chain (`@import cycle: a.css -> b.css ->
  a.css`). A missing file, false `supports()` or malformed import drops
  that import with a warning. An `@import` must come before every rule
  except `@charset` and `@layer` statements; GTK's `@define-color` may
  precede an `@import` too.
- `@layer`: cascade layers ([Cascade](#cascade)). Dotted names nest
  (`@layer a.b`; `@layer a { @layer b { } }` is `a.b`).
- `@container`: [Container queries](#container-queries).
- `@font-face`: `font-family`, `src: url(...)` with `format()` hints
  (TrueType and OpenType load; others fall through to the next src), a
  `font-weight` range and `font-style`. Other descriptors parse and are
  unused. A src that cannot be read fails the load with its path.
- `@property`: `syntax` (type names, `*`, `|` lists, trailing `+`),
  `inherits`, `initial-value`. The initial value answers `var()` where
  nothing declares the name; `inherits: false` resets it per node. A
  value that fails the syntax acts as `unset`.
- `@scope (root) to (limit)? { }` (CSS Cascade 6): subjects must be
  inside a root and not past a limit. Roots may be lists; nested scopes
  nest. `:scope` is the root (`:root` outside `@scope`), `&` is the root
  with its specificity, and other selectors are relative to the root
  without adding specificity.
- `@starting-style` inside a rule: the values a newly styled widget's
  transitions start from.

### Syntax and selector limits

- Animating a registered property is not supported: a custom property
  in a keyframe drops with a warning, and a transition on one applies
  at once.
- `::first-line` parses but never matches: a widget's text has no
  per-line box.

## Selectors

| Kind | Supported |
| --- | --- |
| Simple | element, `.class`, `#id`, `*` |
| Attribute | `[attr]`, `=`, `~=`, `\|=`, `^=`, `$=`, `*=`, with `i`/`s` flags. Built-in attributes: `id`, `class`, `role` (the accessible role name); any other name reads `SetAttr` data. An empty value never matches `^=`, `$=`, `*=`. |
| Combinators | descendant, `>`, `+`, `~` |
| State | `:hover` (the hovered widget and its ancestors, as in GTK), `:active`, `:focus`, `:focus-visible`, `:focus-within`, `:enabled`, `:disabled`, `:read-only`, `:read-write`, `:placeholder-shown`; set with `SetState`: `:checked`, `:selected`, `:indeterminate`, `:backdrop`, `:target` (`StateTarget`, for in-app navigation) |
| Functional | `:is()` and `:where()` (forgiving lists; `:where()` adds no specificity), `:not()` (complex lists), `:has()` (relative selectors) |
| Structural | `:first-child`, `:last-child`, `:only-child`, `:nth-child()`, `:nth-last-child()`, `:nth-child(an+b of S)` (over visible siblings); `:nth-of-type()`, `:nth-last-of-type()`, `:first-of-type`, `:last-of-type`, `:only-of-type`; `:root`; `:empty` (no children and no text) |
| Pseudo-elements | `::before`, `::after`, `::first-letter`, `::placeholder`, `::selection`, `::marker` |

- Each functional pseudo-class adds its most specific argument's
  specificity.
- `:has()` spends at most 4096 node visits per style compute; past
  that it stops matching. Loading a `:has()` sheet makes descendant
  changes restyle the ancestors and siblings that read them.
- `::before`/`::after` take `content:` strings, `attr(name)`,
  `counter()`, `counters()`, `url(...)` images and the quote keywords
  (`open-quote`, `close-quote`, `no-open-quote`, `no-close-quote`).
  `counter-reset` and `counter-increment` follow CSS Lists 3
  inheritance. Generated content paints over the background at the
  content start or end, in its own colour and size, on widgets that
  paint box layers or text. `widget.PseudoStyle` and
  `widget.PseudoContent` read the computed values.
- `::placeholder`, `::selection` and `::marker` select the
  `placeholder`, `selection` and `marker` parts.
- `::first-letter` styles the first grapheme of a single-line `Label`
  (colour, family, weight, size).
- **Element names** are GTK's CSS node names where the GTK widget exists
  (`button`, `label`, `entry`, `scale`, `scrolledwindow`, `textview`,
  `listview`, `row`, `gridview`, `child`, `flowboxchild`, ...), else the
  lowercase Go type. Sub-nodes are named: `entry > text`,
  `checkbutton > check`, `notebook > header > tabs > tab`,
  `progressbar > trough > progress`, `paned > separator`, menus as
  `popover.menu` with `modelbutton` rows. App surfaces are `dialog`,
  `popover`, `tooltip` and `toast`.
- **Popovers** are `popover.background` nodes styled as children of the
  widget they open from (`widget.SetStyleAnchor`): they inherit that
  window's fonts and colours, and descendant selectors see the anchor's
  ancestors. A sheet that sets the popover's background replaces the
  theme's fill and shadow.

## Values

- **Custom properties.** `--x` inherits and computes where declared;
  cycles are invalid. `var()` takes fallbacks, works in every value
  including shorthands, and an unresolvable one acts as `unset`.
- **Math.** `calc()`, `min()`, `max()`, `clamp()`, `round()` (`nearest`,
  `up`, `down`, `to-zero`), `mod()`, `rem()`, `abs()`, `sign()`,
  `sin()`, `cos()`, `tan()`, `asin()`, `acos()`, `atan()`, `atan2()`,
  `pow()`, `sqrt()`, `hypot()`, `log()`, `exp()`, and the constants
  `e`, `pi`, `infinity`, `-infinity`, `NaN`, following CSS Values 4
  (division by zero is infinite; the top level clamps infinities and
  maps NaN to zero).
- **Units.** `px`, `rem`, `em`, `ch`, `ex`, `cap`, `ic`, `lh`, `rlh`
  (font-relative units use the face's metrics, with CSS fallback
  ratios); `vw`, `vh`, `vi`, `vb`, `vmin`, `vmax` and their `sv*`,
  `lv*`, `dv*` forms (all the same viewport); `cqw`, `cqh`, `cqi`,
  `cqb`, `cqmin`, `cqmax`; `pt`, `pc`, `in`, `cm`, `mm`, `Q`, `%`;
  `deg`, `grad`, `rad`, `turn`; `s`, `ms`; `dpi`, `dpcm`, `dppx`, `x`.
  Case-insensitive.
- **`env()`.** `safe-area-inset-*` and `keyboard-inset-*` are zero.
  Unknown names use the fallback, else are invalid.
- **`attr()`.** Reads built-in and `SetAttr` attributes as typed values:
  untyped, a type keyword (`number`, `length`, `percentage`, `angle`,
  `time`, `color`), `type(<length>)`, or a unit (`attr(w px)`). Missing
  or mistyped values use the fallback, else act as `unset`.
- **Colours.**
  - Hex (3, 4, 6, 8 digits), named colours, `transparent`,
    `currentColor`.
  - `rgb[a]()`, `hsl[a]()`, `hwb()`, `lab()`, `lch()`, `oklab()`,
    `oklch()`, `color()` with `srgb`, `srgb-linear`, `display-p3`,
    `a98-rgb`, `prophoto-rgb`, `rec2020`, `xyz`, `xyz-d65`, `xyz-d50`.
    Out-of-gamut colours map to sRGB by reducing OKLCH chroma (CSS
    Color 4).
  - Relative colours: `rgb(from <color> r g b / a)` and the same form
    for hsl, hwb, lab, lch, oklab, oklch, usable in `calc()`.
  - `color-mix(in <space>, A, B)` in every interpolation space, with
    `shorter` (default), `longer`, `increasing`, `decreasing` hue.
  - `light-dark(a, b)`, following the theme.
  - System colours (`Canvas`, `CanvasText`, `LinkText`, `VisitedText`,
    `GrayText`, `Highlight`, `HighlightText`, `Mark`, `MarkText`,
    `ButtonFace`, `ButtonText`, `ButtonBorder`, `Field`, `FieldText`),
    mapped from the theme.
  - GTK's `alpha()`, `shade()`, `mix()`.
  - `@define-color name value;` and `@name`, compiled to `var(--name)`.
    The theme layer defines all of libadwaita's named colours.
- **Images** (`background-image`, `mask-image`,
  `border-image-source`): `none`; `url("file")` (quoted; a local path
  or `file://`, no network); linear, radial and conic gradients and
  their `repeating-` forms, with two-position stops and colour hints;
  `image-set()`, choosing the lowest resolution at or above the
  device's (`widget.SetMediaResolution`), else the highest.
- **Keywords.** `inherit`, `initial`, `unset`, `all`, `revert`,
  `revert-layer`.

## Properties

Longhands are listed with their shorthands.

### Text

| Property | Values |
| --- | --- |
| `color` | |
| `font`, `font-family`, `font-size`, `font-weight`, `font-style` | `font` resets `line-height` unless it sets one; `bolder`/`lighter` |
| `font-feature-settings`, `font-variation-settings` | |
| `font-variant` and longhands | caps, numeric, ligatures (`no-*` turns tags off), east-asian, position, alternates; mapped to OpenType features; `font-feature-settings` wins per tag |
| `font-synthesis` | computes only; a family without italic paints a synthetic oblique |
| `font-optical-sizing` | `auto` feeds the size to `opsz` |
| `font-palette` | `normal`, `light`, `dark` (see limits) |
| `letter-spacing`, `word-spacing` | |
| `line-height` | `normal`, number, length, percentage |
| `text-transform` | |
| `text-align` | `start`, `end`, `left`, `right`, `center`, `justify` (spreads wrapped rows across spaces, except the last) |
| `text-indent` | a length |
| `white-space` | `normal`, `nowrap`, `pre`, `pre-wrap`, `pre-line`, `break-spaces` (CSS Text 3) |
| `word-break` | `normal`, `break-all`, `keep-all` |
| `line-break` | `auto`, `loose`, `normal`, `strict`, `anywhere` |
| `overflow-wrap` (`word-wrap`) | `normal`, `break-word`, `anywhere` (also lowers min-content) |
| `hyphens` | `none` hides U+00AD; `manual` breaks there and shows a hyphen |
| `text-overflow` | `clip`, `ellipsis` |
| `line-clamp`, `-webkit-line-clamp` | |
| `vertical-align` | `baseline`, `sub`, `super`, length, percentage, on text that reports a baseline |
| `direction`, `unicode-bidi` | force a label's or entry's base direction and flip start/end |
| `writing-mode` | horizontal only (see limits) |
| `text-decoration` and longhands | `underline`, `overline`, `line-through`; `solid`, `double`, `dotted`, `dashed`, `wavy`; colour |
| `text-shadow` | list of offset, blur, colour |
| `caret-color`, `caret-shape` | `auto`, `bar`, `block`, `underscore` |
| `user-select` | `auto`, `none`, `text`, `all`, `contain`, on `Entry`, `TextArea`, `RichLabel` |

Text properties apply to `Label` (and buttons, rows and menus holding
one), `Entry` (direction, `text-decoration`) and generated content.
`RichLabel` takes the font group, `line-height` and `text-decoration`.

#### Text limits

- `text-align` and `justify` do not apply to `Entry` and `TextArea`.
- Rows without spaces (CJK) are not justified.
- `hyphens: auto` (no hyphenation dictionary ships; without one, auto
  would guess) reads as manual.
- `line-break` strictness levels are not distinguished: `auto`,
  `loose`, `normal` and `strict` behave the same.
- `unicode-bidi` embed and isolate values have no effect; each widget
  is one bidi paragraph.
- On text, `vertical-align` `top`, `middle`, `bottom`, `text-top` and
  `text-bottom` have no effect.
- `writing-mode`'s vertical forms (vertical-rl/vertical-lr parse and
  compute but paint horizontally).
- `word-spacing` and `text-indent` percentages are invalid.
- `text-overflow` ellipsizes a label whatever its `overflow`.
- `font-palette` computes and inherits; color glyphs always paint with
  the font's first palette. `@font-palette-values` is not parsed.

### Box

- `background` and longhands: `background-color`, `-image`, `-size`
  (`auto`, `cover`, `contain`, lengths, percentages), `-position`,
  `-repeat` (`repeat-x`, `repeat-y`, `repeat`, `space`, `round`,
  `no-repeat`), `-clip` and `-origin` (`border-box`, `padding-box`,
  `content-box`), `-attachment` (`scroll`, `local`, `fixed`). See
  [Backgrounds](#backgrounds).
- `padding-*`; `margin-*` including `auto` (CSS layouts use it;
  programmatic containers read 0).
- `border[-side][-width|-style|-color]` (width needs a visible style),
  `border-*-radius`.
- `border-image` and `-source`, `-slice` (with `fill`), `-width`,
  `-outset`, `-repeat` (`stretch`, `repeat`, `round`, `space`); the
  `border` shorthand resets them.
- `box-shadow`: lists, `inset`, spread, blur.
- `outline[-width|-style|-color|-offset]`. `outline-style: auto` is the
  focus ring (the width, else `widget.FocusRingWidth`);
  `outline-color: auto` is the accent under `auto` style, else
  `currentColor`.
- `border-spacing`: the Box gap.

Painting order follows GTK: outer shadows, background, image, inset
shadows, border, content, outline. Opacity and filters apply to the
subtree; ink outside the border box is added to damage.

### Layout properties

`display`, `width`, `height`, `min-*`, `max-*`, `box-sizing`,
`aspect-ratio`, `position`, `top`/`right`/`bottom`/`left`/`inset`,
`z-index`; flexbox (`flex`, `flex-grow`, `flex-shrink`, `flex-basis`,
`flex-direction`, `flex-wrap`, `flex-flow`, `order`,
`justify-content`, `align-items`, `align-self`, `align-content`,
`place-content`, `gap`, `row-gap`, `column-gap`); grid
(`grid-template-columns`, `-rows`, `-areas`, `grid-template`,
`grid-auto-columns`, `-rows`, `-flow`, `grid`, `grid-row-start`,
`-end`, `grid-column-start`, `-end`, `grid-row`, `grid-column`,
`grid-area`, `justify-items`, `justify-self`, `place-items`,
`place-self`); `overflow`, `overflow-x`, `overflow-y`, `visibility`,
`float`, `clear`, `object-fit`, `object-position`. See
[Layout](#layout).

### Effects

`opacity`, `filter`, `backdrop-filter`, `mix-blend-mode`, `isolation`,
`clip-path`, `mask` (`mask-image`, `-position`, `-size`, `-repeat`,
`-mode`), `transform`, `transform-origin`, `perspective`,
`transform-style`, `backface-visibility`. See
[Rendering effects](#rendering-effects).

### Motion

- `transition` and `-property`, `-duration`, `-delay`,
  `-timing-function`, `-behavior`: comma lists matched by index, shorter
  ones repeating. A negative delay starts part-way. Every interpolable
  longhand transitions. `transition-behavior: allow-discrete` also flips
  `isolation`, `mix-blend-mode`, `backface-visibility`, `appearance`,
  `caret-shape`, `-lys-layout-motion` and `visibility` halfway.
- `animation` and `-name`, `-duration`, `-timing-function`,
  `-iteration-count`, `-direction`, `-play-state`, `-delay`,
  `-fill-mode`, `-composition` (the shorthand does not reset
  `-composition`). Timing follows Web Animations: delay, iterations,
  fill modes, direction, `paused`. A keyframe's own
  `animation-timing-function` eases its interval; missing 0%/100% values
  use the underlying value. Later animations stack on top; `add` and
  `accumulate` compose numbers, lengths, colours and transforms.
- Interpolated: colours (premultiplied sRGB), opacity, brightness-only
  filters, lengths and sizes (padding, margin, borders, radii, outline,
  sizes and bounds, insets, gaps, flex-basis), numbers (font size and
  weight, spacing, line-height, text-indent, perspective, flex factors,
  z-index, order), `box-shadow`, `transform`, `-gtk-icon-transform`,
  `transform-origin`. Other longhands flip at the midpoint. Keyframes
  drop transition and animation longhands, custom properties,
  `!important` and `var()` with a warning, and compute against the root.
- Timing functions: `linear`, `ease`, `ease-in`, `ease-out`,
  `ease-in-out`, `step-start`, `step-end`, `cubic-bezier()`,
  `steps(n, <position>)` with every jump position, and `linear()` (CSS
  Easing 2).

### Icons

`-gtk-icon-size`, `-gtk-icon-source`, `-gtk-icon-palette`,
`-gtk-icon-transform`.

### Interaction

| Property | Behaviour |
| --- | --- |
| `cursor` | every CSS keyword, mapped to cursor-shape-v1 with xcursor fallback; `url("file.cur") x y, <keyword>` loads an xcursor file with a hotspot, falling back to the keyword. Inherits and beats a widget's built-in cursor; `auto` defers to it. |
| `pointer-events` | `none` removes the widget from hit testing so presses fall through; inherits. Widgets with custom `HitTest`s outside `HitLeaf` ignore it. A `clip-path`'s clipped area takes no hits. |
| `accent-color` | a colour or `auto`; inherits. Drives switch tracks, check and radio fills, progress and level bars, slider highlight and the auto outline, unless the part sets a background. |
| `touch-action` | `pan-x`, `pan-y`, `pan-left/right/up/down`, `none`, `manipulation` (both pan axes), intersected with ancestors. Gates touch and touchpad pixel scrolling, not the wheel. |
| `appearance` | `none` and `auto` parse and compute (see limits) |

#### Interaction limits

- `appearance: none` parses and computes and leaves the widget's chrome
  painted. Restyle chrome with `background`, `border` and part
  selectors.
- `touch-action`'s pinch semantics (`manipulation`'s zoom half,
  `pinch-zoom`) are not supported.

### Scrolling

These style a `Scroll` (`scrolledwindow`) and its content.

| Property | Behaviour |
| --- | --- |
| `scroll-behavior` | `smooth` animates programmatic scrolls (`RevealRect`, ...) over 300 ms; the wheel is always immediate |
| `scroll-margin`, `scroll-padding` | grow the revealed rect and snap area; inset the viewport for reveal and snap |
| `scroll-snap-type`, `scroll-snap-align`, `scroll-snap-stop` | `x`/`y`/`block`/`inline`/`both` with `proximity` or `mandatory`; `start`/`end`/`center`; `always`. Mandatory snapping re-settles after relayout; proximity snaps within half a viewport. |
| `overscroll-behavior` (`-x`, `-y`) | `auto` chains to the parent scroll (the root shows the edge glow), `contain` glows without chaining, `none` shows nothing |
| `scrollbar-color`, `scrollbar-width` | `auto` or thumb and track; `auto`, `thin`, `none`; apply to `Scroll` and `Scrollbar` |
| `scrollbar-gutter` | `auto`, `stable`, `stable both-edges`: reserve the bar's space |

## Layout

The widget tree is the document. A `Box` (or a type embedding one) is
the container CSS layouts run in.

**The interaction rule.** Where a stylesheet sets a layout property,
CSS wins; elsewhere the programmatic layout stays. A Box with no
`display` keeps its GTK box layout (axis, spacing, expand,
`AppendAligned`, homogeneous), though a child's `align-self` still
applies. Inside a CSS layout, undeclared properties fall back to the
programmatic ones: box axis for `flex-direction`, spacing for `gap`,
expand (`Append`, `SetHExpand`/`SetVExpand`) for `flex-grow`,
`AppendAligned` and `SetHAlign`/`SetVAlign` for `align-self`. The
CSS-wide keywords (`initial`, `unset`, `revert`, `all`) return layout
longhands, `text-overflow` and `line-clamp` to undeclared, so
`* { all: unset; }` keeps the toolkit's layout.

**display.** `block`, `inline`, `inline-block`, `flow-root`, `flex`,
`inline-flex`, `grid`, `inline-grid`, `contents`, `none`, and
two-keyword forms. On a Box the inner display picks normal flow,
flexbox or grid. The outer display decides how any widget sits in a CSS
parent (block-level by default). `none` acts like `SetVisible(false)`:
no layout, paint, hits, focus or accessibility. `contents` dissolves a
Box: its children join the parent's layout but still inherit from and
paint with it.

**Normal flow.** Blocks stack, filling the width beside floats;
`margin: auto` centres; vertical margins of adjacent blocks collapse.
Inline-level children fill line boxes, aligned by `text-align` and
`vertical-align` (baseline family, `top`, `middle`, `bottom`). RTL runs
right to left. `float` (`left`, `right`, `inline-start`, `inline-end`)
and `clear` work for blocks and floats; a box's height includes its
floats.

**Flexbox.** The full CSS Flexbox algorithm: `flex-basis` (including
`content`), min/max clamping with automatic minimums, line breaking with
gaps, the flexible-length freeze loop, baseline groups,
`align-content`, `align-self`, auto margins, every `justify-content`
and alignment keyword with `safe`/`unsafe`, reversed directions and
RTL, `order` (layout and paint), and `visibility: collapse` struts.

**Grid.** CSS Grid Level 2:

- tracks: lengths, percentages, `fr`, `auto`, `min-content`,
  `max-content`, `fit-content()`, `minmax()`; named lines; `repeat()`
  with counts, `auto-fill` and `auto-fit`;
- `grid-template-areas`, `grid-template` and `grid` (with `auto-flow`
  and `dense`);
- placement by line numbers (negative too), `span`, named lines and
  areas; lines clamp to ±10000;
- auto-placement, sparse or `dense`, by row or column, with implicit
  tracks from `grid-auto-columns`/`-rows`;
- track sizing per §11, intrinsic contributions, `fr` expansion and
  `stretch`;
- `justify-content`/`align-content`, `justify-self`/`align-self`
  (and `-items`, `place-*`) with every keyword, baselines across rows;
- gaps, `order`, `z-index`, RTL columns, and absolutely positioned
  children placed by grid lines.

**Sizing.** `width`, `height`, `min-*`, `max-*` take lengths,
percentages, `calc()` mixes, `auto`/`none`, `min-content`,
`max-content`, `fit-content` and `fit-content()`. `box-sizing` and
`aspect-ratio` apply. They work on every widget in any container.
Percentages resolve against the containing block; a measure reads them
as auto, and a percentage height needs a definite parent height. A
pixel `min-width`/`min-height` is the widget's own floor.

**Positioning.** `relative` offsets without moving siblings. `sticky`
stays inside the nearest `Scroll` viewport within its parent.
`absolute` and `fixed` leave the flow and resolve against the nearest
positioned ancestor's padding box, or the window. `z-index` orders
children in CSS painting layers; paint and hit testing share the order.

**overflow and visibility.** `overflow` (`visible`, `hidden`, `clip`,
`scroll`, `auto`) clips a Box's children to its padding box per axis.
`visibility: hidden` keeps the space and hides paint, hits, focus and
accessibility; a `visible` child of a hidden Box still shows.

**object-fit and object-position** apply to `Image`. Undeclared, the
widget's `SetScale` policy applies, centred.

### Layout limits

- `subgrid`: `grid-template-columns`/`grid-template-rows: subgrid`
  parses and lays out as `none`.
- The grid sizing second pass (§11.1 steps 3–4) does not run.
- Under a max-content constraint, `fr` and maximized tracks do not
  re-resolve against the container's min/max size (§11.6, §11.7); the
  arranged result honours them.
- `justify-self`/`justify-items: baseline` align as `start` (`last
  baseline` as `end`). Grids report a first baseline only.
- `justify-items: legacy` computes to `normal`, and `legacy left`,
  `right`, `center` to the bare keyword.
- Display on containers other than Box (Grid, CenterBox, Stack, Paned,
  ...) only sets how they sit in a CSS parent; `display: contents`
  dissolves only a Box.
- Widgets are atomic inline boxes, so `display: inline` lays out as
  `inline-block`; `text-align: justify` on line boxes reads as start.
- Every CSS box is its own block formatting context: floats never reach
  into a nested box, and margins never collapse through a parent and its
  first or last child or through an empty box.
- Stacking contexts are per container: `z-index` orders a widget
  among its siblings only. Absolute and fixed widgets paint with their
  parent, inside its clip; a transform does not create a containing
  block.
- The static position of an absolute child of a flex container is the
  content box's start corner; the same applies in a grid that is not
  its containing block.
- `overflow: scroll` and `auto` clip without scrollbars (use a
  `Scroll`), and `overflow` affects only Box.
- `min()`, `max()` and `clamp()` mixing percentages with lengths are
  invalid; `calc()` mixes work.
- `last baseline` uses the widget's single baseline; baselines in a
  column flex container fall back to start.
- A `visibility: visible` descendant shows only inside a hidden Box.
- `Entry` and `TextArea` text insets count as content.
- The window's root widget takes the window size; CSS sizes on it do
  not apply.
- Layout is horizontal-tb.

## Backgrounds

A background is a stack of image layers over a colour
(css-backgrounds-3). `background-image` sets the layer count, first on
top; the other longhands are lists matched by index. Each layer sizes
against its `background-origin` box (or the window for `fixed`),
positions by `background-position`, repeats per axis (`repeat`,
`no-repeat`, `round`, `space`), and clips to its `background-clip` box
with rounded corners. The colour paints under all layers, clipped to the
bottom layer's box. `local` reads as `scroll`. The `background`
shorthand takes one layer per comma group and the colour in the last.

## Rendering effects

Effects apply to the painted result each frame. Without effects, the
plain paint path allocates nothing.

- **`opacity`** below 1 composites the subtree as a group.
- **`filter`**: `blur()` (three-pass box approximation),
  `brightness()`, `contrast()`, `grayscale()`, `hue-rotate()`,
  `invert()`, `opacity()`, `saturate()`, `sepia()`, `drop-shadow()`, in
  order. A `brightness()`-only list needs no layer.
- **`backdrop-filter`**: the same functions over what is painted behind
  the border box, clipped to its rounded shape.
- **`mix-blend-mode`**: all sixteen modes. **`isolation: isolate`**
  makes the subtree its own group.
- **`clip-path`**: `inset()` (with `round`), `circle()`, `ellipse()`,
  `polygon()` (always nonzero fill), `path()` (SVG path data). Clipped
  areas take no hits.
- **`mask`**: one layer of `mask-image`, `-size`, `-position`,
  `-repeat` (background geometry in the border box) and `mask-mode`
  (`alpha`, `luminance`, `match-source`).
- **3-D transforms**: `perspective()`, `rotateX()`, `rotateY()`,
  `translateZ()` as plane-restricted 4x4 matrices. The `perspective`
  property composes on the element itself (GTK's model). A perspective
  with only 2-D transforms changes nothing. **`backface-visibility:
  hidden`** hides a flipped plane.
- **`border-image`**: nine-slice over the border image area (grown by
  `-outset`); edges stretch, repeat, round or space; the middle paints
  with `fill`. A border image that loads replaces the border styles.

Effects on a transformed box apply in its local space before the
transform.

### Rendering limits

- `url()` filters are not supported: they name SVG filter graphs. A
  declaration containing one is skipped with a warning.
- Multiple mask layers are not supported: a comma list rejects the
  declaration. `mask-origin`, `mask-clip`, `mask-composite` and
  `mask-border` are unknown.
- `transform-style: preserve-3d` parses and computes but paints as
  `flat`.
- `matrix3d()`, `rotate3d()`, `scale3d()` and `translate3d()` are
  rejected.
- `background-blend-mode` parses and is ignored.

## lys-ui extensions

| Extension | Does |
| --- | --- |
| `backdrop-filter: -lys-refract(<bezel> <displacement>)` | bends the backdrop through a glass bezel along the rounded edge (`.liquid-glass`) |
| `background-image: -lys-grain(<opacity>)` | fine luminance noise anchored to the box (`.frosted`); not valid as a border image |
| `-lys-layout-motion: none \| smooth \| snappy \| bouncy` | springs the box from its old rect to its new one (`SetLayoutMotion`) |
| `-lys-spring(<bounce>?)`, `-lys-bounce` | spring timing functions that land exactly at the end |
| `@-lys-define <name> <value>;` | a sheet-wide value, read as `var(--<name>)` |

The theme defines motion tokens: `--lys-duration-short`, `-medium`,
`-long` (zero under reduced motion), `--lys-ease-standard`,
`--lys-ease-emphasized-decelerate`, `--lys-ease-emphasized-accelerate`,
`--lys-spring-smooth`, `-snappy`, `-bouncy`.

```css
.card { transition: transform var(--lys-duration-medium) var(--lys-spring-snappy); }
```

## Cascade

From highest precedence:

1. Programmatic per-widget colours (`Button.Bg`, `Entry.SetColor`, ...).
2. Stylesheet declarations, ordered:
   1. `!important` over normal;
   2. provider priority;
   3. cascade layer: unlayered beats layered, later layers beat earlier,
      a parent layer beats its nested layers; reversed for
      `!important`;
   4. specificity (ids, classes and states, elements);
   5. `@scope` proximity (scoped beats unscoped);
   6. source order.

   Inline styles beat every layer at their priority.
3. The `Theme`.

`revert` rolls back to the next lower priority, down to the `Theme`.
`revert-layer` rolls back to the next lower layer.

Constructor metrics (a button's padding) are defaults stylesheets
override. Inherited: `color`, the font group, the text properties,
`cursor`, `pointer-events`, `accent-color`, `caret-color`,
`caret-shape`, `scrollbar-color`, `text-shadow`, `font-synthesis`,
`visibility`. Not inherited: box and layout properties, backgrounds,
effects, motion, `user-select`, `scrollbar-width`, `vertical-align`,
`line-clamp`, `text-overflow`, `unicode-bidi`.

## Media state

`@media` evaluates against each tree's media state:

| Feature | Source |
| --- | --- |
| `width`, `height`, `aspect-ratio`, `orientation` | `widget.SetMediaViewportFor(root, w, h)`, set by each window on resize; `widget.SetMediaViewport` is the fallback for other trees; 0×0 until set |
| `prefers-color-scheme` | `Theme.IsDark` |
| `prefers-reduced-motion` | the theme's animation setting |
| `prefers-contrast` | `widget.SetMediaContrast` |
| `resolution` | `widget.SetMediaResolution` (96 dpi by default) |

A change restyles the tree only when a loaded sheet uses `@media`,
`@container` or container units.

## Container queries

`container-type: inline-size | size | normal` and `container-name` (or
`container: <name> / <type>`) make a query container. A `@container`
prelude is a comma list of optional names and conditions over `width`,
`height`, `inline-size`, `block-size`, `aspect-ratio` and
`orientation`, using the media grammar. Each condition uses the nearest
ancestor with a matching name and a type that can answer it (block-axis
features need `size`); with none it is false. Nested blocks must all
match. Container units resolve against the nearest suitable container,
else the viewport. A container's resize restyles its subtree.

## Conformance

`widget/cssconformance_*_test.go` holds the conformance suite: per
feature group, cases of a stylesheet, a widget tree and expected
computed values, bounds, hits and paint probes, derived from the specs
(many adapted from Web Platform Tests). Exception tables assert each
documented limit, and the runner checks this page states it.

```sh
go test ./widget -run TestCSSConformance
```

| Group | Tests | Limits |
| --- | --- | --- |
| Layout | `Layout`, `LayoutExceptions` | [Layout limits](#layout-limits) |
| Selectors | `Selectors`, `SelectorsExceptions` | [Syntax and selector limits](#syntax-and-selector-limits) |
| At-rules and cascade | `Cascade`, `CascadeExceptions` | [Syntax](#syntax) |
| Values | `Values` | |
| Rendering | `Rendering`, `BorderImage`, `RenderingExceptions` | [Rendering limits](#rendering-limits) |
| Text | `Text`, `TextExceptions` | [Text limits](#text-limits) |
| Interaction and UI | `Interaction`, `InteractionExceptions` | [Interaction limits](#interaction-limits) |

## Performance

- Rules are bucketed by their rightmost simple selector.
- Matching and computing style allocates nothing for every selector
  form.
- A stylesheet load bumps a generation; widgets recompute lazily, and
  an unchanged result touches nothing.
- A class, id or state change restyles descendants or siblings only
  when a loaded selector depends on it.
- Steady-state frames make no style calls. Effects paint into an
  offscreen layer cached per widget.
- Static flex and flow trees lay out without allocating.

The `BenchmarkStyleGallery*` benchmarks measure this
([bench/baseline.md](https://github.com/stubbedev/lys-ui/blob/main/bench/baseline.md)).
