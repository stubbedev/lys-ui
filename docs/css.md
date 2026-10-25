# lys-ui CSS

A GTK-flavored CSS layer above the typed `Theme`. The palette stays the
source of truth and works with no stylesheet loaded. Stylesheets
override individual properties where rules match. Real GTK application
stylesheets load as they are: wayle's compiled SCSS, about 1800 rules,
parses in about 6 ms with no warnings.

## API

```go
s, err := widget.AddStylesheetFile("theme.css", widget.StylePriorityTheme) // its own sheet, hot-reloaded
widget.LoadStylesheetFile("app.css")             // the application slot, hot-reloaded
widget.LoadStylesheet(`.title { color: #c00 }`) // or from a string; "" removes it
settings := widget.AddStylesheet(css, widget.StylePrioritySettings)
settings.Load(newCSS)                           // replace its rules in place
settings.Remove()

b := widget.NewButton(child, 10, 8)
b.AddClass("destructive")
b.SetID("save-button")
b.SetState(widget.StateSelected, true)          // :selected, :checked, :backdrop, ...
b.SetInlineStyle("padding: 4px")                // a widget-scoped provider
b.SetAttr("data-kind", "danger")                // [data-kind="danger"]
box.AttachStylesheet(widget.NewStylesheet(css, widget.StylePriorityApplication))
```

- **Priorities** follow GTK's provider priorities: `StylePriorityFallback`,
  `Theme` (regenerated on `SetTheme`), `Settings`, `Application` (the
  `LoadStylesheet` slot), and `User` (inline styles). An app-wide
  default that the theme's state rules should still outrank (a plain
  button's transparent fill under the theme's `:hover` and `:checked`
  fills) belongs at `StylePriorityTheme`, added after the theme with
  `AddStylesheet`: at `Application` it beats every theme rule, states
  included.
- **Scoped files.** `NewStylesheetFile(path, priority)` reads a sheet
  for `AttachStylesheet` from a file, so its `@import` rules resolve
  relative to it; it hot-reloads like the installed file sheets.
- **Hot reload.** `AddStylesheetFile`, `LoadStylesheetFile` and
  `NewStylesheetFile` stat each
  loaded file once a second on the application's timer and reload it on
  change; one poller covers every file, starts with the first file load,
  and each sheet's `Remove` (or `Load`) stops its file. A file that
  vanishes or stops parsing keeps its last good rules and is reported
  through `widget.SetParseWarn`; a load that cannot read or parse it
  fails with an error carrying the path.
- **Fonts.** `font-family` and `font-weight` resolve through
  `widget.SetFaceResolver` (for example over `app.FontWeighted`),
  with `@font-face` families consulted first ([Syntax](#syntax)). Without a
  resolver they keep the constructor face. A variable face still
  takes `font-weight` on its wght axis.
- **Warnings.** Parse warnings go to the library logger, or to
  `widget.SetParseWarn`.
- **Units.** `widget.SetRootFontSize` sets what 1rem is (16 by default).

## Syntax

- A CSS Syntax 3 tokenizer.
- `@keyframes` (and `@-gtk-keyframes`) compile into the sheet.
- `@media` evaluates against the tree's media state (below) — size
  features (`width`, `height`, `aspect-ratio`, `orientation`,
  `resolution` in `dpi`, `dpcm`, `dppx` or `x`, with `min-`/`max-`
  and Level 4 range syntax like `(400px <= width <= 800px)`),
  `prefers-color-scheme`, `prefers-contrast`,
  `prefers-reduced-motion`, the Level 4 condition grammar (`not`,
  `and`, `or`, nested parentheses), a comma list (any query
  matching), media types with `not`/`only` (`screen`/`all` true, the
  others false; `not print` is true), and nesting (the inner
  conditions intersect). Evaluation is three-valued as Level 4
  defines it: an unknown feature or a `<general-enclosed>` form is
  unknown, `not` keeps it unknown, and an unknown query is false. A
  `@media` block inside a rule scopes that rule's remaining
  declarations.
- `@supports` evaluates at parse time against the engine's actual
  property and selector support: `(prop: value)`, `selector(...)`,
  `not`, `and`, `or`, and parens. A false block's rules never
  compile; unknown properties are false.
- `@import "file.css";` (or `url(...)`, with the optional `layer`
  and `layer(name)` qualifiers — dotted names nest — the optional
  `supports(...)` qualifier evaluated at parse time like `@supports`,
  and an optional trailing media query) loads and inlines its target
  relative to the importing stylesheet's path — file loads resolve
  beside the file, string stylesheets relative to the working
  directory. Cycles are detected per import chain and reported
  precisely (`@import cycle: a.css -> b.css -> a.css`); a missing
  file, a false `supports()`, or a malformed import warns and drops
  that import only. An `@import` must precede every rule but
  `@charset` and `@layer` statements; a later one warns and is
  ignored. GTK's `@define-color` may precede an `@import` too, as GTK
  accepts it there.
- `@layer` orders cascade layers (below). Dotted names nest:
  `@layer a.b` is a layer below `a` (the parent outranks its nested
  layers), `@layer a { @layer b { ... } }` means `a.b`, and a name
  statement registers the dotted prefixes it names.
- `@container` evaluates per compute against the node's query
  containers — the widgets with `container-type` set in its ancestry
  (below).
- `@font-face` declares app fonts: `font-family`, `src: url(...)`
  with `format()` hints (truetype/opentype load; an unsupported
  format warns and falls through to the next src), the `font-weight`
  range, and `font-style`. The face loads on the next relayer and
  the font-family resolver serves it before the app's own; a src
  that cannot be resolved or read reports through the stylesheet
  error path (a file load fails with its path). `font-stretch` and
  the other descriptors parse and are unused — the loader has no
  matching knob.
- `@property` registers typed custom properties: `syntax` (the
  known type names and `*`, pipe lists with the trailing `+`),
  `inherits`, and an `initial-value` validated against the syntax.
  A registered name's initial value answers var() wherever nothing
  declares it; `inherits: false` resolves the name to its initial
  value on nodes that do not declare it, however their ancestors
  set it. A declared value that does not match the syntax is invalid
  at computed-value time and acts as `unset`: the inherited value, or
  the initial value when the registration does not inherit.
  Registered properties do not animate (see
  [Syntax and selector limits](#syntax-and-selector-limits)).
- `@scope (root) to (limit)? { ... }` scopes the block's rules
  (CSS Cascade 6): each subject must be a root element or descend
  from one without crossing a limit element, the root may be a
  selector list, and a nested `@scope`'s roots are scoped to the outer
  one. `:scope` names the root (and is `:root` outside `@scope`), `&`
  is the root with its selector's specificity, and a selector naming
  neither is relative to the root, as `:where(:scope) sel` — the root
  adds no specificity. The cascade ranks scoped rules by scoping
  proximity, below specificity: of two equally specific rules, the one
  whose root is fewer generations up wins, and a scoped rule beats an
  unscoped one.
- Native nesting: `&` in a nested block is the parent selector list
  with `:is()`'s matching and specificity (`&:hover`, `& > .child`,
  `.x &`, `& + &`); a nested selector without `&` is relative to the
  parent, a descendant (or the combinator it leads with); nesting
  nests, and a nested list with an invalid member drops that nested
  rule alone.
- `!important` wins the cascade over every normal declaration
  (below).
- A malformed declaration skips that declaration, and a malformed
  selector skips its rule — one invalid member invalidates the whole
  selector list, as CSS says. Everything around it still applies.

## Selectors

- **Simple selectors.** Element, `.class`, `#id` and `*`.
- **Attribute selectors.** `[attr]`, `[attr=v]`, `~=`, `|=`, `^=`,
  `$=`, `*=`, each with the trailing `i`/`s` case flag. The built-in
  attributes are `id`, `class` (the class list, so `[class~=x]` and
  `[class|=x]` work) and `role` (the a11y role's name, `button`,
  `entry`, `label`, ...); any other name reads an app-set data
  attribute (`SetAttr`). A widget's state is not an attribute: the
  state pseudo-classes below test it, as HTML's do for an element's
  state. An empty value never matches `^=`, `$=` or `*=`.
- **Combinators.** ` `, `>`, `+` and `~`.
- **State pseudo-classes.**
  - `:hover` matches the hovered widget and its ancestors (GTK's rule).
  - `:active`, `:focus`, `:focus-visible` and `:focus-within`.
  - `:disabled`, `:checked`, `:selected`, `:indeterminate`,
    `:backdrop` and `:target`. These are app-driven through
    `SetState`; `StateTarget` is the `:target` hook — the widget tree
    has no URL fragment, so the app marks the widget an in-app
    navigation points at.
  - `:enabled` (the negation of `:disabled`), `:read-only` and
    `:read-write` (an entry's `SetReadOnly`), and
    `:placeholder-shown` (an empty entry with a placeholder).
- **Functional pseudo-classes.** `:is()` and `:where()` with
  forgiving selector lists (an invalid member drops alone) — `:is()`
  counts its most specific argument's specificity, `:where()` counts
  nothing — and `:not()` with complex selector lists (Level 4). Each
  functional pseudo-class adds its own argument's specificity, so
  `:not(.a):not(.b)` counts two classes. `:has()` takes relative
  selectors (`:has(.a)`, `:has(> .a)`, `:has(+ .a)`, `:has(~ .a)`,
  with complex arguments too), counts its most specific argument, and
  a repeated `:has()` requires every list; one compute
  spends at most 4096 node visits on `:has()` walks, after which a
  `:has()` conservatively stops matching — the budget keeps relational
  matching linear instead of quadratic in tree size, and a sheet that
  loads `:has()` selectors also makes descendant class/state changes
  (and a child's first link into the tree) restyle the ancestors and
  siblings that read them.
- **Structural pseudo-classes.** `:first-child`, `:last-child`,
  `:only-child`, `:nth-child()`, `:nth-last-child()` (over visible
  siblings), `:nth-of-type()`, `:nth-last-of-type()`,
  `:first-of-type`, `:last-of-type`, `:only-of-type` (over visible
  siblings of the same element name), `:root`, `:empty` (no children
  and no text: a label or entry showing text is not empty), and
  `:not()` with compound lists and Level 4 specificity.
  `:nth-child(an+b of S)` counts only the siblings matching the
  selector list S and adds S's most specific member's specificity.
- **Pseudo-elements.** `::before` and `::after` cascade their own
  style (inherited from the element's) with `content:` of strings,
  `attr(name)` (the built-ins and data attributes), `counter(name)`
  and `counters(name, sep)`; `counter-reset` and
  `counter-increment` follow CSS Lists 3's counter inheritance — a
  node inherits its parent's counters and its previous sibling's, with
  the values of the element before it in tree order, a reset
  instantiates a counter (nested under an inherited one), an increment
  bumps the innermost — and a change re-counts everything after it;
  `content: url(...)` paints an image as the generated
  content, beside the text, and the quote keywords
  `open-quote`/`close-quote`/`no-open-quote`/`no-close-quote` emit
  the typographic quote pairs over the inherited quote nesting
  counter. The pseudo-element's box layers and content paint over
  the widget's background — content at the content start (`::before`)
  and end (`::after`), in the pseudo-element's own color and
  font-size — wherever the widget paints box layers (`PaintBoxLayers`)
  or shapes text (labels and entries). `widget.PseudoStyle` and
  `widget.PseudoContent` read the computed values. `::placeholder`,
  `::selection` and `::marker` desugar onto the existing part
  machinery: `entry::placeholder` styles the entry's `placeholder`
  part (as a descendant part selector) and `::selection` its
  `selection` part; `::marker` matches a `marker` part.
  `::first-letter` cascades its own pseudo style and paints on
  `Label`'s single-line draw: the first grapheme cluster in its own
  color, font-family, weight, and size, the remainder in the
  label's style. `::first-line` never matches (see
  [Syntax and selector limits](#syntax-and-selector-limits)).
- **Element names.** These are GTK's CSS node names where lys-ui has the
  GTK widget (`button`, `label`, `entry`, `scale`, `scrolledwindow`,
  `textview`, `listview`, `row`, `gridview`, `child`, `flowboxchild`, ...), and otherwise
  the Go type lowercased. Sub-nodes are named too: `entry > text`,
  `checkbutton > check`, `notebook > header > tabs > tab`,
  `progressbar > trough > progress`, `paned > separator`, and menus as
  `popover.menu` with `modelbutton` rows. The app surfaces are
  `dialog`, `popover`, `tooltip` and `toast`. `TestCSSElementNames`
  pins the list.
- **Popovers style as GTK's.** A popover's content is the `popover`
  node with GtkPopover's `.background` class, styled as a child of the
  widget it opens from (`widget.SetStyleAnchor`): it inherits that
  window's fonts and colors, `:root` is that window's root, and
  descendant selectors see the anchor's ancestry. The theme's plate
  (fill and shadow) yields to a stylesheet that declares the
  popover's background, so `popover.background { background:
  transparent; }` leaves the surface to the content's own card.

### Syntax and selector limits

- Animating a registered property is not supported: the animations and
  transitions interpolate the longhand table, which custom properties
  sit outside, so a custom property in a keyframe drops with a warning
  and a transition on one applies the new value at once.
- `::first-line` parses but never matches: line fragmentation happens
  inside the text shaper at paint time, and a widget's text is one
  shaped run with no per-line box the cascade could address.

## Values

- **Custom properties.** `--x` is inherited and computed where it is
  declared, and cycles are invalid. `var()` takes fallbacks, and an
  unresolvable `var()` acts as `unset`. `var()` works everywhere a
  value can appear, including inside shorthand expansion
  (`border: var(--w) solid var(--c)`).
- **Math.** `calc()`, `min()`, `max()` and `clamp()`, plus the CSS
  Values 4 set: `round()` (with the `nearest`/`up`/`down`/`to-zero`
  strategies; nearest takes the upper multiple on a tie), `mod()`,
  `rem()`, `abs()`, `sign()`, `sin()`, `cos()`, `tan()`, `asin()`,
  `acos()`, `atan()`, `atan2()` (number arguments are radians, the
  inverse functions return angles; atan2 takes any two values of one
  type), `pow()`, `sqrt()`, `hypot()` (of any one type), `log()` (with
  an optional base) and `exp()`, all nestable inside `calc()`, with the
  constants `e`, `pi`, `infinity`, `-infinity` and `NaN`. A division
  by zero is an infinity, not an error, `mod()`/`rem()` of a zero
  divisor are NaN, and a calculation's top level clamps an infinity to
  the largest representable value and NaN to zero, as Values 4 says.
- **Units.** px, rem, em, ch, ex, cap, ic, lh and rlh (the
  font-relative family: ch/ex/cap/ic resolve against the default
  face's metrics scaled by the node's font size — `render.Font.Metrics`
  — with the CSS fallback ratios when the face reports none; lh is the
  node's computed line-height; rem and rlh read the tree root's
  computed font-size and line-height, `SetRootFontSize` the initial
  value), the viewport family — vw, vh, vi, vb, vmin and vmax and
  their small (`sv*`), large (`lv*`) and dynamic (`dv*`) forms, which
  read the same media viewport: a desktop window has no retracting
  toolbars — cqw, cqh, cqi, cqb, cqmin and cqmax (the container-query
  units, against the nearest eligible query container per axis, else
  the viewport — see [Container queries](#container-queries)), pt, pc,
  in, cm, mm, Q, %, deg, grad, rad, turn, s, ms, and the resolution
  units dpi, dpcm, dppx and x. Units match ASCII case-insensitively.
- **env()** answers the safe-area and keyboard inset family
  (`env(safe-area-inset-top)`, `env(keyboard-inset-height)`, ...) as
  zero lengths, the desktop value; an unknown name takes the fallback
  after the comma, and is invalid without one.
- **attr()** reads the node's attributes — the built-in `id`, `class`
  and `role` and the app-set data attributes, the same store the
  attribute selectors read — as typed values wherever a number,
  length, angle, time or color is taken: untyped (the attribute parses
  as the CSS value it holds), a type keyword (`attr(n number)`,
  `length`, `percentage`, `angle`, `time`, `color`), `type(<length>)`,
  or a unit (`attr(w px)`, `attr(p %)`) applied to a bare number. A
  missing or mistyped attribute takes the fallback after the comma, and
  behaves as `unset` without one.
- **Colors.**
  - Hex (3/4/6/8 digits), named colors, `transparent` and
    `currentColor` (valid wherever a color is, including inside every
    function below).
  - `rgb[a]()`, `hsl[a]()`, `hwb()`, `lab()`, `lch()`, `oklab()`,
    `oklch()` and `color()` — the predefined spaces `srgb`,
    `srgb-linear`, `display-p3`, `a98-rgb`, `prophoto-rgb`, `rec2020`,
    `xyz`, `xyz-d65` and `xyz-d50`, with the transfer functions
    extended past [0, 1] as CSS Color 4 extends them. Colors outside
    sRGB gamut-map as CSS Color 4 §13.2 says: the OKLCH chroma reduces,
    lightness and hue kept, until clipping is under a just-noticeable
    difference.
  - Relative color syntax: `rgb(from <color> r g b / a)`, and the
    same `from` form for hsl, hwb, lab, lch, oklab and oklch, each
    channel keyword the origin's channel as the number its plain form
    takes (degrees for a hue, 0..255 for rgb, 0..100 for hsl's s and l),
    in `calc()`.
  - `color-mix(in <space>, A, B)` in every interpolation space —
    `srgb`, `srgb-linear`, `lab`, `lch`, `oklab`, `oklch`, `hsl`,
    `hwb`, the predefined RGB spaces and `xyz`/`xyz-d65`/`xyz-d50` —
    with the hue methods `shorter` (default), `longer`, `increasing`
    and `decreasing`; an achromatic color's powerless hue takes the
    other color's.
  - `light-dark(a, b)` follows the active theme's color scheme.
  - System colors — `Canvas`, `CanvasText`, `LinkText`, `VisitedText`,
    `GrayText`, `Highlight`, `HighlightText`, `Mark`, `MarkText`,
    `ButtonFace`, `ButtonText`, `ButtonBorder`, `Field`, `FieldText` —
    map onto the active theme (background, text, accent, borders) and
    re-derive on `SetTheme`.
  - GTK's `alpha()`, `shade()` and `mix()`.
  - `@define-color name value;` with `@name` references, which compile
    to `var(--name)`, so a subtree's `--name` overrides them. The
    theme layer defines all of libadwaita's named colors from the
    palette.
- **Images.** Wherever an image is taken (`background-image`,
  `mask-image`, `border-image-source`): `none`, `url("file")` (a local
  path or `file://` URL; lys-ui loads no network images), the linear,
  radial and conic gradients and their `repeating-` forms — color
  stops with up to two positions each, as lengths or percentages along
  the gradient line (angles around a conic one), color hints between
  stops, and the fixup that orders auto and backwards positions — and
  `image-set()`, whose candidate (a url, a bare string, or a gradient,
  with an `x`/`dppx`/`dpi` resolution and an optional `type()`) is the
  lowest resolution at or above the device's (`SetMediaResolution`),
  else the highest.
- **Keywords.** `inherit`, `initial`, `unset`, `all`, `revert` and
  `revert-layer` (see [Cascade](#cascade)).

## Properties

Each longhand is listed with its shorthand.

- **Text.**
  - `color`
  - `font`, `font-family`, `font-size` (lengths, percentages, the
    absolute and relative keywords), `font-weight` (numbers, `bolder`
    and `lighter` by the relative-weight table), `font-style`; the
    `font` shorthand resets `line-height` unless it names one
  - `font-feature-settings`, `font-variation-settings`
  - `letter-spacing`, `word-spacing` (both at the advance level, em
    resolving at the font size), `line-height` (normal, a number, a
    length or a percentage), `text-transform`
  - `text-align` (start, end, left, right, center, justify — justify
    spreads a wrapped row's leftover width across its spaces;
    everything but the final row; start and end follow `direction`),
    `text-indent` (a length: the first row's start edge, its wrap width
    narrowed by it), `white-space` (normal, nowrap, pre, pre-wrap,
    pre-line, break-spaces — CSS Text 3's collapsing: tabs read as
    spaces, runs collapse, a newline is a space under normal and nowrap
    and kept under pre-line, pre-wrap's trailing spaces hang,
    break-spaces' take their advance and break after any one),
    `word-break` (normal, break-all, keep-all — break-all and keep-all
    step by grapheme cluster and the UAX #14 class rules), `line-break`
    (auto, loose, normal, strict, anywhere — anywhere breaks at every
    cluster; see [Text limits](#text-limits) for the CJK strictness
    profiles), `overflow-wrap` (and
    its `word-wrap` alias: normal, break-word, anywhere — a word that
    overflows a row alone breaks between clusters, and anywhere also
    lowers the min-content width), `hyphens` (none hides U+00AD, manual
    breaks at it and renders the hyphen where the break landed, auto
    reads as manual), `text-overflow` (clip, ellipsis), `line-clamp`
    (and `-webkit-line-clamp`), `vertical-align` (baseline, sub,
    super, a length, or a percentage of the line box, on every text
    surface that reports a baseline — labels, buttons, rows)
  - `font-variant` and its longhands — caps (`small-caps`,
    `all-small-caps`, `petite-caps`, `all-petite-caps`, `unicase`,
    `titling-caps`), numeric (`lining-nums`, `oldstyle-nums`,
    `proportional-nums`, `tabular-nums`, `diagonal-fractions`,
    `stacked-fractions`, `ordinal`, `slashed-zero`), ligatures
    (`common-ligatures`, `discretionary-ligatures`,
    `historical-ligatures`, `contextual` and their `no-*` forms, which
    turn the tags off), east-asian (`jis78` … `jis2004`, `simplified`,
    `traditional`, `full-width`, `proportional-width`, `ruby`),
    position (`sub`, `super`) and alternates (`historical-forms`) — all
    mapping onto the OpenType feature tags, with
    `font-feature-settings` winning per tag
  - `font-synthesis` (none/weight/style/small-caps — the engine
    never synthesizes a face quality, so this parses as the guard it
    is; there is nothing to gate), `font-optical-sizing` (auto feeds
    the pixel size to the `opsz` axis in points, none keeps it),
    `font-palette` (normal, light, dark — see [Text limits](#text-limits))
  - `direction` (ltr/rtl — it forces a label's or entry's base
    direction and flips start and end), `unicode-bidi` (the overrides
    force the base the same way), `writing-mode` (horizontal only;
    see [Text limits](#text-limits))
  - `text-decoration` and its longhands `text-decoration-line`
    (underline, overline, line-through, blink drawing nothing),
    `-style` (solid, double, dotted, dashed, wavy) and `-color`;
    `text-shadow` (a list of offset, blur and color, painted under the
    text, inherited); `caret-color`, `caret-shape` (auto, bar, block,
    underscore — the entry and text area caret's painted form)
  - `user-select` (auto, none, text, all, contain) on the surfaces
    that select: Entry, TextArea, RichLabel. `none` keeps a press a
    caret move and a drag from extending a selection; `all` selects
    everything on a press; `auto` resolves as CSS UI 4 says — contain
    on an editable field, else the parent's none or all, else text —
    and the property does not inherit. Plain Label carries no
    selection machinery, so the value changes nothing there.
- **Box.**
  - `background` and its longhands — `background-color`,
    `background-image`, `background-size` (`auto`, `cover`,
    `contain`, one or two lengths or percentages), `background-position`
    (every `<position>` form), `background-repeat` (`repeat-x`,
    `repeat-y`, and one or two of `repeat`, `space`, `round` and
    `no-repeat`), `background-clip` and `background-origin`
    (`border-box`, `padding-box`, `content-box`) and
    `background-attachment` (`scroll`, `local`, `fixed`) — painted as
    [Backgrounds](#backgrounds) describes
  - `padding-*`, `margin-*` (`auto` too: the CSS layouts below hand
    it the free space; programmatic containers read it as 0)
  - `border[-side][-width|-style|-color]` (a width draws only with a
    visible style), `border-*-radius`
  - `border-image` and its longhands `-source`, `-slice` (with
    `fill`), `-width` (numbers multiplying the border width, lengths,
    percentages, `auto`), `-outset` and `-repeat` (`stretch`, `repeat`,
    `round`, `space`); the `border` shorthand resets them
  - `box-shadow` (lists, inset, spread and blur)
  - `outline[-width|-style|-color|-offset]`, with `outline-style: auto`
    (the focus ring: the declared width, else `widget.FocusRingWidth`)
    and `outline-color: auto` (the initial value: the accent color under
    an auto style, `currentColor` under another)
  - `border-spacing` (the Box gap)
- **Layout.** `display`, `width`, `height`, `min-width`, `min-height`,
  `max-width`, `max-height`, `box-sizing`, `aspect-ratio`, `position`,
  `top`/`right`/`bottom`/`left`/`inset`, `z-index`, the flexbox group
  (`flex`, `flex-grow`, `flex-shrink`, `flex-basis`, `flex-direction`,
  `flex-wrap`, `flex-flow`, `order`, `justify-content`,
  `align-items`, `align-self`, `align-content`, `place-content`,
  `gap`, `row-gap`, `column-gap`), the grid group
  (`grid-template-columns`, `grid-template-rows`,
  `grid-template-areas`, `grid-template`, `grid-auto-columns`,
  `grid-auto-rows`, `grid-auto-flow`, `grid`, `grid-row-start`,
  `grid-row-end`, `grid-column-start`, `grid-column-end`, `grid-row`,
  `grid-column`, `grid-area`, `justify-items`, `justify-self`,
  `place-items`, `place-self`), `overflow`/`overflow-x`/
  `overflow-y`, `visibility`, `float`, `clear`, `object-fit` and
  `object-position` — see [Layout](#layout).
- **Effects.** `opacity`, `filter`, `backdrop-filter`,
  `mix-blend-mode`, `isolation`, `clip-path`, `mask` (and the
  `mask-image`/`-position`/`-size`/`-repeat`/`-mode` longhands),
  `transform`, `transform-origin`, `perspective`, `transform-style`
  and `backface-visibility` — see [Rendering effects](#rendering-effects).
- **Motion.**
  - `transition` and its longhands `transition-property`,
    `-duration`, `-delay`, `-timing-function` and `-behavior`, each a
    comma list matched by index against the property list, a shorter
    one repeating (css-transitions-1 §2): a changed longhand
    transitions with the last entry naming it (or `all`), when its
    combined duration is positive; the longhands sharing an entry share
    one tween. A negative delay starts the transition part-way, and the
    start value holds through a positive one. Every interpolating
    longhand transitions (below); `transition-behavior: allow-discrete`
    also flips the discrete `isolation`, `mix-blend-mode`,
    `backface-visibility`, `appearance`, `caret-shape` and `visibility`
    halfway.
  - `animation` and its longhands `animation-name`, `-duration`,
    `-timing-function`, `-iteration-count`, `-direction`,
    `-play-state`, `-delay`, `-fill-mode` and `-composition`, each a
    comma list zipped against the names, a shorter one repeating; the
    shorthand resets all of them but `animation-composition`
    (css-animations-2 leaves it out). An animation runs the Web
    Animations timing model on the animation clock: its local time
    counts from the instant it applies (a negative delay starts it
    part-way, `paused` holds the time), the active interval spans the
    iterations, the fill modes hold the first or last frame outside
    it, and the direction picks each iteration's way. The timing
    function eases each keyframe interval (a `@keyframes` stop's own
    `animation-timing-function` overrides it for the interval it
    starts), and a longhand no 0% or 100% stop declares takes the
    underlying value there. Later animations in the list sit on top;
    `add` and `accumulate` compose a frame onto the value beneath it —
    numbers and lengths sum, colors add per channel, transforms append
    the effect's functions.
  - Every longhand a `@keyframes` stop declares animates: the colors,
    opacity, the brightness-only filter, the lengths and sizes
    (padding, margin, border widths and radii, outline, width, height
    and their bounds, the insets, gaps, flex-basis), the numbers
    (font-size and weight, letter- and word-spacing, line-height,
    text-indent, perspective, flex-grow/-shrink, z-index, order),
    `box-shadow` lists, `transform`, `-gtk-icon-transform` and
    `transform-origin` interpolate (colors in premultiplied sRGB, the
    space CSS Color 4 interpolates legacy colors in); every other
    longhand animates discretely, flipping at the interval's midpoint.
    The transition and animation longhands, custom properties,
    `!important` and `var()` values in a stop drop with a warning, and
    a stop's values compute against the root context (rem, the initial
    font size).
  - Every timing function slot takes the keywords (`linear`, `ease`,
    `ease-in`, `ease-out`, `ease-in-out`, `step-start`, `step-end`),
    `cubic-bezier()` (x controls within [0, 1]), `steps(n, <position>)`
    with every jump position (`jump-start`, `jump-end`, `jump-none` —
    n of at least 2 — `jump-both`, `start`, `end`) and the before flag,
    and `linear()` with up to two percentages per point, its positions
    made canonical and its ends extrapolated (CSS Easing 2).
  - `@starting-style` inside a rule names the values a newly styled
    widget's transitions start from — the entry transition.
- **Icons.** `-gtk-icon-size`, `-gtk-icon-source`, `-gtk-icon-palette`
  and `-gtk-icon-transform`.
- **Interaction.**
  - `cursor`: every CSS keyword (default, pointer, crosshair, text,
    wait, progress, help, move, the n/s/e/w/ne/nw/se/sw/ns/ew/nesw/
    nwse-resize family, zoom-in, zoom-out, grab, grabbing, none,
    context-menu, cell, vertical-text, alias, copy, no-drop,
    not-allowed, col-resize, row-resize, all-scroll) maps onto the
    cursor-shape-v1 vocabulary, with the xcursor theme as the
    fallback for compositors without the protocol (the theme path
    carries the classic core names). `cursor: url("file.cur") x y,
    <keyword>` loads the named xcursor file and pins the CSS hotspot
    over the file's own; the first frame shows, and a file that does
    not load shows the fallback keyword. The property inherits — one
    declaration covers a subtree — and outranks a widget's built-in
    shape (text fields' caret, buttons' hand); `auto` defers to it.
  - `pointer-events`: `none` takes the widget out of hit testing and
    the press falls through to whatever is behind (a Stack's lower
    child, the parent); it inherits, so a container's `none` covers
    its subtree unless a descendant sets `auto` again. Widgets with
    hand-written HitTests outside the shared HitLeaf do not honor it.
    A `clip-path`'s clipped-out region takes no hits either.
  - `accent-color`: a color or `auto` (the theme accent), inherited.
    The checked and accent surfaces follow it: the switch's on track,
    the checkbutton and radio fill, the progressbar and levelbar fill,
    the slider's highlight, and the auto outline. An explicit
    background on the part still wins.
  - `touch-action`: `pan-x`, `pan-y`, `none` (and the direction forms
    `pan-left/right/up/down`, which gate their axis) gate the touch
    scroll path — the two-finger pan and the kinetic glide after it —
    intersected with the ancestors' values. The wheel path is never
    gated (touch-action speaks to touch manipulation only) and the
    touchpad's pixel scroll shares the gated path. `manipulation`
    parses as both pan axes; the pinch half of its meaning is not
    supported ([Interaction limits](#interaction-limits)).
  - `appearance`: `none` and `auto` parse and compute; `none` does not
    strip a widget's chrome ([Interaction limits](#interaction-limits)).
- **Scrolling.** The properties style the `Scroll` viewport
  (`scrolledwindow`) and what it scrolls.
  - `scroll-behavior`: `smooth` animates programmatic scrolls —
    `RevealRect` and friends — over an eased 300 ms; `auto` (and the
    wheel, always) lands at once, as the spec scopes smooth scrolling
    to programmatic scrolls.
  - `scroll-margin` (all four sides, negatives allowed) grows a
    widget's revealed rect and its snap area; `scroll-padding` insets
    the viewport the reveal and snap math use.
  - `scroll-snap-type` (`none`, or `x`/`y`/`block`/`inline`/`both`
    with `proximity` or `mandatory`) turns snapping on for the
    container; `scroll-snap-align` (`start`/`end`/`center`, block and
    inline) on the scrolled children names the snap areas, and
    `scroll-snap-stop: always` makes one a gesture cannot skip past.
    Mandatory snapping re-settles through a relayout or resize;
    proximity takes a position when the resting offset lies within
    half a viewport of it (the UA freedom the spec leaves).
  - `overscroll-behavior` (and `-x`/`-y`; `auto`, `contain`, `none`):
    `auto` chains motion a clamp swallowed to the parent scroll — the
    root scroll, with none above, takes the edge glow — `contain`
    keeps the glow here without chaining, `none` shows nothing.
  - `scrollbar-color` (`auto` or thumb then track) and
    `scrollbar-width` (`auto`, `thin`, `none`) reach the bars —
    Scroll's own and the standalone Scrollbar; the color inherits, the
    width does not. `scrollbar-gutter` (`auto`, `stable`, `stable
    both-edges`) reserves the bar's column in measure even without
    overflow (a matching one on the near edge under `both-edges`), so
    content does not reflow when a bar appears.

The box model applies to the containers, controls and text widgets,
painted in GTK's order: outer shadows, background, image, inset
shadows, border, content, outline. Opacity and filter apply over the
subtree, and ink outside the border box joins the damage rect.

### Text limits

The CSS Text properties apply to the styled text surfaces: `Label`
(and the buttons, rows and menus that hold one), `Entry` (direction)
and the pseudo content. Within them:

- `text-align` and `justify` do not apply to `Entry` and `TextArea`:
  editing widgets own their layout for caret and selection
  correctness, so they clip and scroll rather than justify.
- `RichLabel` composes its runs itself and takes the font group only.
- CJK justify is not supported: rows without spaces stay
  start-aligned, since no inter-cluster distribution exists.
- `hyphens: auto` (no hyphenation dictionary ships; without one, auto
  would guess) reads as manual.
- The `line-break` strictness profiles for CJK punctuation are not
  distinguished: the typesetting segmenter computes one UAX #14
  profile, so auto, normal and strict read the same; loose and
  anywhere differ from them.
- `unicode-bidi`'s embed and isolate embeddings are not supported:
  internal/text resolves one paragraph per widget, so there is no
  embedding level to open.
- On a text surface, `vertical-align`'s top, middle, bottom, text-top
  and text-bottom have no effect: they need an inline formatting
  context's box, which a lone widget baseline does not have.
- `writing-mode`'s vertical forms (vertical-rl/vertical-lr parse and
  compute but paint horizontally): the whole paint stack, caret math,
  wrapping and ellipsizing are horizontal.
- `word-spacing` and `text-indent` percentages are invalid: they need
  the containing block, which the per-declaration parse cache cannot
  see; lengths resolve at paint.
- `text-overflow` ellipsizes a label whatever its `overflow`: a
  toolkit label never paints past its box, so `overflow` is no clip
  switch there and `text-overflow` decides alone.
- `font-palette` computes and inherits; color glyphs always paint
  with the font's first palette: the font loader exposes CPAL's
  palettes without their light and dark type flags, and
  `@font-palette-values` is not parsed.

### Interaction limits

- `appearance: none` parses and computes and leaves the widget's
  chrome painted: widget chrome is programmatic theme painting, not CSS
  pseudo-element chrome, so stripping it needs a per-widget unstyled
  design. A stylesheet restyles every chrome surface through the box
  model instead (`background`, `border`, the part selectors).
- `touch-action`'s pinch semantics (`manipulation`'s zoom half,
  `pinch-zoom`) are not supported: the gesture layer exposes pinch only
  as a claimed `GesturePinch` a widget opts into, not an axis the
  scroll gates can mask, so `manipulation` computes to both pan axes.

## Layout

The widget tree is the document. Every widget is one box; a Box (and
any type embedding one) is the container the CSS layouts run in, the
way a `div` is in a browser.

**The interaction rule.** Where the stylesheet sets a layout property,
CSS wins; where it does not, the programmatic layout stands. A Box
without a `display` declaration keeps its GTK box layout (axis,
spacing, expand, AppendAligned, homogeneous), and inside a CSS layout
an undeclared property falls back to the programmatic setting: the
box axis for `flex-direction` (a column box is a column flexbox), the
box spacing for `gap`, Append's expand (or SetHExpand/SetVExpand) for
`flex-grow`, AppendAligned and SetHAlign/SetVAlign across for
`align-self` when `align-items` is not declared either. The CSS-wide
keywords (`initial`, `unset`, `revert`, and `all`) return a layout
longhand to undeclared, so a GTK-style `* { all: unset; }` reset keeps
the toolkit's layout (and likewise `text-overflow` and `line-clamp`,
whose initial value is a label's own ellipsizing and line limit); `min-width`/`min-height` keep their reading of
it as a zero floor, and `visibility` inherits as CSS says.

**display.** `block`, `inline`, `inline-block`, `flow-root`, `flex`,
`inline-flex`, `contents`, `none`, `grid` and `inline-grid`, and the
two-keyword forms (`block flow`, `inline flow-root`, `inline flex`,
...). On a Box the inner display picks the layout of its children:
normal flow for the block and inline values, flexbox for the flex
values, grid for `grid` and `inline-grid`. On every widget the outer display decides how it sits in a CSS
flow parent: block-level, or inline-level for `inline`,
`inline-block`, `inline-flex` and `inline-grid`; undeclared, a widget
is block-level. `none` takes the widget out of layout, paint, hit
testing, Tab focus and the accessibility snapshot, exactly like
SetVisible(false), in any container. `contents` dissolves a Box into
its parent's CSS layout: its children become the parent's items while
staying its children (they inherit from it, and it paints them); it
draws no box of its own and takes no hits, and its bounds cover its
children.

**Normal flow.** Block-level children stack down the content box, as
wide as the band beside the floats unless a width says otherwise;
`margin: auto` centers a narrower block, a single auto side pushes it,
and an RTL box aligns it right. Adjacent block siblings' vertical
margins collapse (the largest positive plus the most negative).
Inline-level children fill line boxes, breaking when the next does not
fit, aligned by `text-align` (start, end, left, right, center) and on
the line by `vertical-align` (the baseline family — sub, super and
lengths shift the widget's own baseline — or top, middle, bottom); a
widget without a baseline sits on its bottom margin edge, as an
inline-block does. RTL lines run right to left. `float: left`, `right`,
`inline-start` and `inline-end` pull a child to that side as high as
the content before it allows — onto the current line when it fits
beside its inline content — with blocks and line boxes flowing beside
the floats and moving down when they do not fit; `clear` (`left`,
`right`, `both`, and the logical sides) moves a block or a float below
them. The box's height includes its floats.

**Flexbox.** The CSS Flexible Box Layout algorithm (§9): flex base
sizes from `flex-basis` (a length, a percentage of a definite main
size, `auto` deferring to `width`/`height`, `content` and the content
keywords), hypothetical sizes clamped by the min/max sizes with the
automatic minimum (`min-width: auto` is the content's min-content size,
capped by a definite size, and zero for an item whose overflow clips),
greedy line breaking with the gaps, the flexible-length resolution
with its freeze loop (a flex-grow or flex-shrink sum below one
distributes only that fraction; shrinking scales by the inner base
size), cross sizes at the used main sizes, single-line containers
sized to a definite cross size, baseline groups, `align-content` over
the lines (`normal` and `stretch` stretch them; the distributed values
fall back as CSS says), `align-self` per item with `stretch` honoring
max sizes and auto cross margins, then the main axis: auto margins
take the free space first, else `justify-content`. Every keyword is
honored: `flex-start`, `flex-end`, `start`, `end`, `left`, `right`,
`center`, `normal`, `stretch`, `space-between`, `space-around`,
`space-evenly`, `self-start`, `self-end`, `baseline`, `first`/`last
baseline`, with `safe` falling back to start on overflow and `unsafe`
(the default) overflowing both ways. `row-reverse`, `column-reverse`,
`wrap-reverse` and an RTL `direction` flip the axes the CSS way (start
and end stay with the writing mode, flex-start and flex-end follow the
flow). `order` reorders layout and paint alike. `visibility: collapse`
removes an item's main size but keeps its cross size as a strut on its
line. Gap percentages resolve against the content box.

**Grid.** The CSS Grid Layout Module Level 2 algorithm. The explicit
grid comes from `grid-template-columns` and `grid-template-rows` —
track sizes as lengths, percentages (auto while the container's extent
is indefinite), `fr`, `auto`, `min-content`, `max-content`,
`fit-content(<length-percentage>)` and `minmax(min, max)`; named lines
in brackets, merged where two lists meet; `repeat()` by an integer,
or `auto-fill` and `auto-fit` (as many repetitions of their fixed
tracks as fit the definite size or max size with the gaps, the fewest
that reach a definite min size, else one; `auto-fit` collapses the
repetitions no item lands in, gutters included) — and from
`grid-template-areas` (rows of names and `.` null cells, every name a
rectangle, its edges named `<area>-start` and `<area>-end`), with the
`grid-template` and `grid` shorthands (`grid` also takes `auto-flow`,
`dense` and the implicit sizes on either side of the slash).
Placement reads `grid-row-start`/`-end`, `grid-column-start`/`-end`,
`grid-row`, `grid-column` and `grid-area`: line numbers, negative ones
counting from the explicit grid's end, `span N`, named lines with an
optional count, a span to a named line, and area names (a lone name is
the area's edge, else the first line so called); past the explicit
grid every implicit line counts as having any name, and lines clamp to
±10000 as browsers clamp them. Auto-placement follows §8.5 in
order-modified document order, `grid-auto-flow` `row` or `column`,
sparse (the cursor never backtracks) or `dense` (each item takes the
first hole that fits), items locked to a row (or column) first, then
the rest; implicit tracks before and after the explicit grid take
`grid-auto-columns`/`grid-auto-rows`, the first track after it the
first size and the last before it the last, cycling. Track sizing is
§11's: base sizes and growth limits, the non-spanning items'
contributions, then the spanning items' grouped by span (minimum,
min-content and max-content contributions to the intrinsic minimums,
then to the intrinsic maximums, past the growth limits as the spec
distributes, fit-content() clamped), the items crossing flexible
tracks by fr ratio, the maximized tracks, the fr expansion (by the free
space of a definite extent, or under a max-content constraint by the
largest fr the tracks and items ask for), and `stretch` of auto tracks
under `justify-content`/`align-content` `normal` or `stretch`. A grid
item's minimum contribution is its automatic minimum size where CSS
applies one: its min-content contribution, capped by fixed spanned
maximums; zero for an item spanning several tracks one of them
flexible, or whose overflow clips. The columns size first, the rows at
the items' used widths. `justify-content` and `align-content` distribute the free
space over the tracks with every keyword and `safe`/`unsafe`;
`justify-self`/`justify-items` and `align-self`/`align-items`, with
`place-items` and `place-self`, align each item in its area — `normal`
stretches an item without an aspect ratio and starts one with it, auto
margins take the free space first, `self-start`/`self-end` follow the
item's own direction, `left`/`right` the physical sides — and the
baseline values share a baseline across a row (first baseline in its
first row, last baseline in its last; a widget without a baseline
aligns its bottom edge), growing the row by the shim; the grid's first
baseline is its first row's. Gaps are `row-gap`/`column-gap`
(percentages of a definite content box), the box spacing where
undeclared. `order` reorders placement and paint, and `z-index` orders
grid items like flex items. An RTL grid runs its columns from the
right. An absolutely positioned child of a positioned grid resolves
its containing block from its grid lines — the area they name, an
`auto` line, a lone span or a line outside the grid standing for the
padding edge, and no implicit tracks made. Measured in a bounded width
a block-level grid fills it and an `inline-grid` shrinks to its
max-content width when that fits; an unbounded measure is the
max-content size, and the grid's min-content size answers the shrink
queries (`min-content` sizes, a flex item's automatic minimum).

**Sizing.** `width`, `height`, `min-*` and `max-*` take lengths,
percentages, `calc()` sums and products mixing the two (`calc(100% -
2em)`), `auto` (`none` for max-*), `min-content`, `max-content`,
`fit-content` and `fit-content(<length-percentage>)`; `box-sizing`
picks the content or border box they measure; `aspect-ratio` (`auto`,
`<ratio>`, `auto <ratio>`) transfers a definite size, or the measured
width when both are auto, to the other axis. They apply to every
widget, in any container: a definite size or a bound holds the slot a
container offers (a column box does not stretch a 100px-wide child,
which sits at the start, or by SetHAlign), and a min size widens it.
Percentages resolve against the containing block — the content box a
CSS layout hands its items, and in a programmatic container the space
it offers in the measure constraint; a percentage height resolves only
against a definite height (a fixed or pinned one), and reads as auto
under a content-sized parent, as CSS 2 §10.5 says. min-content is the
widget's floor (what it can give up as a Shrinker or WidthShrinker,
lowered to a MinSizer's floor — a wrapping label's widest word); a box
whose lines break between its items — normal flow, a multi-line flex
row, a column flex container — has its widest item's min-content
contribution as its own, and a single-line flex row the sum of its
items'; max-content is its unbounded natural size; in the block axis
the content keywords are auto, as CSS reads them. A plain-pixel `min-width`/`min-height` stays
each widget's own floor, GTK's reading (a switch's knob, a slider's
handle).

**Positioning.** `relative` shifts a widget by its offsets (left over
right, top over bottom, percentages of the parent's content box)
without moving its siblings. `sticky` keeps a widget inside the
nearest Scroll's visible viewport, inset by its declared sides, until
its parent's content box runs out. `absolute` and `fixed` leave the
flow — the container measures nothing for them — and resolve against
their containing block: the padding box of the nearest positioned
ancestor for `absolute`, the tree root's box (the window) for `fixed`
and for an absolute without one. Each axis follows CSS: a definite
size wins; both insets set stretch an auto size between them and
center a definite one under auto margins; otherwise the widget takes
its shrink-to-fit size from the set inset, or from its static position
— where the flow reached — with neither. `z-index` (auto stacks as 0)
orders a container's children in the CSS painting layers: negative
z-index, in-flow blocks, floats, inline-level boxes, positioned boxes
at 0, positive z-index; it applies to positioned widgets and to flex
items. Paint and hit testing read the one order, so the topmost
painted child is the one the pointer hits.

**overflow and visibility.** `overflow`, `overflow-x` and `overflow-y`
(`visible`, `hidden`, `clip`, `scroll`, `auto`) clip a Box's children
to its padding box on each clipping axis, for paint and hit testing
alike. `visibility: hidden` keeps the widget's space but paints
nothing, takes no hits and no focus, and leaves the accessibility
snapshot; a `visibility: visible` child of a hidden Box still shows.

**object-fit and object-position.** On an `Image`, `object-fit`
(`fill`, `contain`, `cover`, `none`, `scale-down`) picks how the
picture fills the widget's box, and `object-position` (a `<position>`:
one or two keywords, lengths or percentages, or the three- and
four-value edge-offset forms like `right 10px bottom 20px`) places the
concrete object in it, the overflow clipped to the box. Undeclared,
the widget's `SetScale` policy stands, centered.

### Layout limits

- Inside a programmatic container a widget never outgrows the slot it
  is offered: a definite size larger than the slot clamps to it,
  because a widget's measure lies within its constraints. A CSS layout
  (a Box with `display`) lets a definite size overflow its content box,
  as CSS does.
- `subgrid`: `grid-template-columns`/`grid-template-rows: subgrid`
  with its line-name lists parse, compute and report supported, and
  lay out as `none`, CSS's reading of `subgrid` on a grid that is not a
  subgrid. A subgrid's items size its parent's tracks, so the parent's
  pass would have to place and measure a nested Box's children inside
  its own track sizing and hand its tracks to that Box's arrange; the
  widget tree measures and arranges each child Box as one widget.
- The grid sizing algorithm's second pass (§11.1 steps 3 and 4: the
  columns re-resolved when an item's min-content width changed with the
  row sizes) does not run: a horizontal widget's width does not depend
  on its row's height.
- Under a max-content constraint (an unbounded measure) the fr size and
  the maximized tracks do not redo against the container's own
  min-width/min-height or max-width/max-height (§11.6, §11.7); the
  container's sizes clamp the result, and the arrange, sizing into the
  definite box, honors them exactly.
- `justify-self` and `justify-items` `baseline` align as `start` (`last
  baseline` as `end`): no widget has a baseline across the row axis.
  The grid reports a first baseline only, widgets having one.
- `justify-items: legacy` computes to `normal` and `legacy left`,
  `right` or `center` to the bare keyword: the legacy keyword's
  inheritance into descendants' `legacy` is HTML's `<center>`
  mechanism, not modeled.
- The static position of an absolute child of a grid that is not its
  containing block is the content box's start corner, as in a flex
  container; CSS aligns it as the sole item of a content-box area.
- Inner display on containers other than Box: Grid, CenterBox, Stack,
  Paned and the other built-in containers keep their own layout
  algorithms — their placement is their API — so `display: flex`,
  `grid` or `block` there only sets how they sit in a CSS parent, and
  `display: contents` dissolves only a Box (elsewhere the widget keeps
  its box).
- Inline formatting is of whole widgets: a widget is an atomic inline
  box, so `display: inline` lays out as `inline-block` and a label's
  text never fragments across its parent's line boxes;
  `text-align: justify` on line boxes reads as start, since there are
  no spaces between widgets to stretch.
- Every CSS-laid-out box roots its own block formatting context
  (`block` reads as `flow-root`): floats never reach into a nested
  box, and margins never collapse through a parent and its first or
  last child or through an empty box.
- Stacking contexts are per container: `z-index` orders a widget
  among its siblings and never lifts it above its container's
  siblings, and a dissolved (`contents`) box's children paint with
  it. Absolute and fixed widgets paint in their parent's pass, so an
  ancestor's clip (`overflow`, a Scroll viewport) still clips them,
  and a transform does not make a containing block.
- The static position of an absolute child of a flex container is the
  content box's start corner; CSS aligns it as the sole flex item.
- `overflow: scroll` and `auto` clip without scrollbars — wrap the
  content in a Scroll for scrolling — and overflow has no effect on
  widgets other than Box, whose content already stays inside their
  box (labels ellipsize or clip, a Scroll scrolls by its own policy).
- `min()`, `max()` and `clamp()` mixing percentages with lengths are
  invalid: the comparison needs the layout-time basis, which the
  compute-time evaluator never sees; `calc()` sums and products of
  them work.
- `last baseline` aligns on a widget's single baseline (widgets report
  one), and baseline alignment in a column flex container falls back
  to start, as CSS's does.
- A `visibility: visible` descendant shows inside a hidden Box only; a
  hidden widget of another type hides its whole subtree.
- Content-box sizes count the built-in padding of a widget that keeps
  it outside the CSS box (Entry's and TextArea's text inset) as
  content.
- The window's root widget takes the window size; CSS sizes on it do
  not apply.
- Layout is horizontal-tb: vertical writing modes compute but lay out
  horizontally, as their paint does.

## Backgrounds

A box's background (css-backgrounds-3 §3) is a stack of image layers
over a color. `background-image`'s list counts the layers, the first
on top; every other background longhand is a list matched to it by
index, a shorter one repeating. Each layer's image sizes against its
positioning area — the `background-origin` box, or the window for
`background-attachment: fixed` — at its natural size where
`background-size` is `auto` (a gradient, which has none, fills the
area; one auto dimension follows the natural ratio), scaled to `cover`
or `contain`, or to explicit lengths and percentages; it sits at its
`background-position` (percentages of the area minus the image); and
it repeats per axis: `repeat` tiles both ways, `no-repeat` paints the
one tile, `round` rescales the tile to a whole count across the area
(the other axis keeping the ratio when its size is auto), and `space`
spreads whole tiles with even gaps when two or more fit. The layer
paints clipped to its `background-clip` box with that box's rounded
corners; the color paints under every layer, clipped to the bottom
layer's clip box. The padding and content boxes derive from the
borders and the CSS padding the stylesheet declares (a widget's
built-in padding is not CSS's). `local` reads as `scroll`: no box here
scrolls its own background. The `background` shorthand takes a layer
per comma group — any order of an image, a position with an optional
`/ size`, a repeat style, an attachment and one or two boxes (the
first the origin, the second the clip) — and the color in the final
layer only.

`background-blend-mode` is not supported (see
[Rendering limits](#rendering-limits)).

## Rendering effects

The effect properties apply to a widget's painted result, per frame.
A stylesheet that declares none of them takes the plain paint path:
the brightness stack inline, no offscreen buffer, no allocation (the
budget tests pin it).

- **`opacity`** below one paints the subtree into an offscreen group
  and composites it at the opacity (css-color-4 §15.1), so a child
  overlapping its parent does not show through it.
- **`filter`**: `blur()`, `brightness()`, `contrast()`, `grayscale()`,
  `hue-rotate()`, `invert()`, `opacity()`, `saturate()`, `sepia()` and
  `drop-shadow()`, in list order. The color functions apply the
  filter-effects matrices (each function's output clamps before the
  next, as the spec defines); `blur()` is a three-pass box
  approximation of the Gaussian (within a couple of counts of it);
  `drop-shadow()` blurs and tints the painted silhouette and composites
  it under the element. A `brightness()`-only list keeps the inline
  fast path (no layer). `url()` filters are not supported
  ([Rendering limits](#rendering-limits)).
- **`backdrop-filter`**: the same function list, applied to what has
  been painted behind the widget this frame — the canvas region under
  the border box is snapshotted, filtered, clipped to the rounded
  border box, and composited back over the original backdrop, before
  the widget's own transform. `url()` filters are not supported here
  either.
- **`mix-blend-mode`**: all sixteen modes — the separable set
  (`multiply`, `screen`, `overlay`, `darken`, `lighten`, `color-dodge`,
  `color-burn`, `hard-light`, `soft-light`, `difference`, `exclusion`)
  and the non-separable set (`hue`, `saturation`, `color`,
  `luminosity`) — blended per the compositing spec, then composited
  source-over. **`isolation`**: `isolate` makes the subtree its own
  compositing group (an offscreen layer), so descendants' blend modes
  blend against the group instead of everything below; `auto` blends
  against the canvas. A blending element that also carries a transform
  composites through the transform first (the projective composite is
  source-over only).
- **`clip-path`**: `inset()` (with `round` corners), `circle()`,
  `ellipse()` (keywords and percentages resolve against the border
  box), `polygon()` (the fill-rule keyword is accepted; every path
  fills nonzero) and `path()` — the SVG `d` grammar with arcs
  subdivided into cubics. The shape's coverage multiplies the
  element's painted result, and the clipped-out region takes no hits.
- **`mask`**: one image layer — `mask-image` (any image, as above),
  `mask-size`, `mask-position` and `mask-repeat` with background
  layers' grammar and geometry, sized, positioned and clipped in the
  border box — and `mask-mode` (`alpha`, `luminance`, `match-source`).
  Alpha masks read the mask's alpha, luminance masks its WCAG
  luminance; the `mask` shorthand takes one layer of the background
  shorthand's grammar with the mode in place of the color. A mask is
  a single layer ([Rendering limits](#rendering-limits)).
- Effects on a transformed box apply inside its transform layer, in
  the box's local space, before the transform maps the result.
- **3-D transforms**: `perspective()` (as a transform function),
  `rotateX()`, `rotateY()` and `translateZ()` compose as true 4x4
  matrices restricted to the element's plane — a homography — so a
  tilt foreshortens under perspective and a `translateZ` zooms. The
  `perspective` property takes the same distance and composes outside
  the transform (the engine applies every transform to the node
  itself, GTK's model, so `perspective` affects the element's own
  composed map rather than only its children). A perspective with a
  purely 2-D transform is a no-op — a flat plane at z=0 projects onto
  itself, which is CSS's own math. **`backface-visibility`**: `hidden`
  hides the element when its composed map flips the plane (the
  flat-pipeline reading: a negative determinant of the plane map,
  which is what `rotateY(180deg)` and `scaleX(-1)` produce). Shared
  3-D rendering contexts are not supported
  ([Rendering limits](#rendering-limits)).
- **`border-image`** (css-backgrounds-3 §6): the source is cut into
  nine regions by the slices (numbers are image pixels, percentages
  of the image's size, clamped to it) and drawn over the border image
  area — the border box grown by `border-image-outset`. The drawn
  widths are `border-image-width`'s (a number multiplies the side's
  border width, `auto` takes the slice's size), scaled down together
  when opposite ones overlap; the corners scale into the corners, each
  edge scales across its side and `stretch`es, `repeat`s (whole tiles
  centered), `round`s (rescaled to a whole count) or `space`s (whole
  tiles, gaps around them) along it, and the middle paints only under
  `fill`. A box whose border image loads draws it instead of its
  border styles; one whose source fails to load keeps them. A gradient
  source renders at the border image area's size, cached across
  frames.

### Rendering limits

- `url()` filters are not supported: they name SVG filter graphs,
  which need a document loader the engine does not have. A `filter` or
  `backdrop-filter` declaration carrying one is skipped with a warning.
- Multiple mask layers are not supported: a comma list rejects the
  declaration, since the engine composites one mask layer.
  `mask-origin`, `mask-clip`, `mask-composite` and `mask-border` are
  unknown properties.
- `transform-style: preserve-3d` parses and computes but paints as
  `flat`: the engine composites each subtree as one offscreen layer
  with no z-interaction between siblings, so a shared 3-D rendering
  context (interpenetrating children, per-fragment sorting) has no
  compositor to run on.
- `matrix3d()`, `rotate3d()`, `scale3d()` and `translate3d()` are
  rejected for the same reason: only the plane-restricted functions
  above compose correctly on this pipeline.
- `background-blend-mode` parses and is ignored: the background layers
  paint straight onto the canvas, and blending one layer over the next
  needs a layer group per box.

## lys-ui extensions

Beyond CSS and GTK's `-gtk-` vocabulary, lys-ui adds a few properties,
functions and at-rules of its own, prefixed `-lys-`:

- **`backdrop-filter: -lys-refract(<bezel> <displacement>)`** bends the
  backdrop through a glass bezel along the box's rounded edge: inside
  the bezel, pixels sample the backdrop displaced outward along the edge
  normal, strongest at the edge. The liquid-glass bend (`.liquid-glass`).
- **`background-image: -lys-grain(<opacity>)`** is a fine luminance
  noise anchored to the box, the etching of frosted glass (`.frosted`).
  Not valid as a border image.
- **`-lys-layout-motion: none | smooth | snappy | bouncy`** springs the
  box from its old laid-out rect to a new one instead of jumping, laid
  out once at the destination (`widget.SetLayoutMotion`).
- **`-lys-spring(<bounce>?)`** and **`-lys-bounce`** are timing
  functions: a spring filling the duration, damped 1 - bounce, landing
  exactly at its end.
- **`@-lys-define <name> <value>;`** names any value sheet-wide, read
  as `var(--<name>)`, like GTK's `@define-color` for colors.

The theme layer defines the shared motion tokens with it:
`--lys-duration-short`, `--lys-duration-medium` and
`--lys-duration-long` (the `Theme` motion durations, zero under reduced
motion), `--lys-ease-standard`, `--lys-ease-emphasized-decelerate`,
`--lys-ease-emphasized-accelerate`, and `--lys-spring-smooth`,
`--lys-spring-snappy` and `--lys-spring-bouncy`:

```css
.card { transition: transform var(--lys-duration-medium) var(--lys-spring-snappy); }
```

## Cascade

The origins, from highest to lowest precedence:

1. programmatic per-widget colors (`Button.Bg`, `Entry.SetColor`, ...)
2. stylesheet declarations, ordered as CSS Cascade 6 orders them,
   with GTK's provider priority standing in for the origins:
   `!important` first (an important declaration at any priority
   beats every normal one), then priority, then cascade layer
   (unlayered rules outrank every `@layer`; among layers, the
   later-declared wins — `@layer a, b;` fixes the order, `@layer name
   { ... }` styles in it, and a layer's own rules outrank its nested
   layers, so a dotted name's parent outranks it; for important
   declarations the layer order reverses — an earlier layer's
   important beats a later one's, and a layered important beats an
   unlayered one), then specificity `(ids, classes+states, elements)`,
   then @scope proximity (fewer generations up to the scoping root
   wins; scoped beats unscoped), then source order. Inline styles are
   element-attached: they outrank every layer at their priority.
3. the `Theme` and its derived state colors

`revert` rolls back to the next origin down: the best declaration at
a lower provider priority (an application sheet's revert yields the
theme sheets' value), walking on through a revert there, and to the
`Theme` itself — `unset` — at the bottom. `revert-layer` rolls to the
next cascade layer down within the same origin, and walks the chain:
a revert-layer beneath a revert-layer rolls another layer down, until
a concrete declaration or the bottom (then `unset`).

Constructor metrics (a button's padding, a label's size) are
defaults that a stylesheet overrides. `color`, the `font-*` group,
`cursor`, `pointer-events`, `accent-color`, `caret-color`,
`caret-shape`, `scrollbar-color`, `text-shadow`, the CSS Text
properties and `visibility` inherit. Box and layout properties, the
backgrounds, effects and motion, `user-select`, `scrollbar-width`, and
`vertical-align`, `line-clamp`, `text-overflow`, `unicode-bidi` and
`font-synthesis`, do not.

## Media state

`@media` conditions evaluate per compute against the tree's media
state:

- **Viewport.** `widget.SetMediaViewportFor(root, w, h)` — the app
  layer feeds it from each window's resize callback, and every
  window's tree computes against its own size (one stylesheet, two
  windows, two results). `widget.SetMediaViewport` is the
  process-wide fallback for trees whose root reported nothing (the
  last value set wins there); width, height, aspect-ratio and
  orientation features read the tree's own viewport. Until set, the
  viewport is 0×0, so min-conditions never match.
- **Appearance.** `prefers-color-scheme` derives from the active
  theme's polarity (`Theme.IsDark`) and `prefers-reduced-motion`
  from its animation gate, so `SetTheme` re-evaluates them;
  `prefers-contrast` comes from `widget.SetMediaContrast` (the
  desktop's contrast preference, which no Theme field carries), and
  `resolution` from `widget.SetMediaResolution` (dpi, 96 by default).

A viewport or preference change restyles the whole tree — but only
  when some loaded stylesheet carries `@media` rules; without one,
  the setters record the state and style nothing.

## Container queries

`container-type: inline-size | size | normal` and `container-name:`
(or the `container: <name> / <type>` shorthand) make a widget a
query container. A `@container` prelude is a comma list of
conditions, each an optional name and a query in the media condition
grammar (three-valued, as above) over `width`, `height`,
`inline-size`, `block-size`, `aspect-ratio` and `orientation`; one
true condition applies the block. Each condition evaluates against
its own query container (CSS Containment 3 §6.1): the nearest
ancestor whose `container-name` list holds the name and whose type
can answer every queried feature — a block-axis or two-axis feature
needs a `size` container, an `inline-size` one answers the inline
axis only — and without one the condition is unknown, so false
(negated or not). Nested `@container` blocks must all hold, each
selecting its own container. The container units resolve as the
spec says: `cqw`/`cqi` against the nearest query container, `cqh`/
`cqb` against the nearest `size` container, `cqmin`/`cqmax` over the
two, and an axis without one against the viewport. A container's
resize restyles its subtree like a viewport change does, so queries
re-evaluate on window resize (and on any relayout that moves a
container's bounds when a sheet reads them).

## Conformance

`widget/cssconformance_*_test.go` is the engine's conformance suite:
one table of cases and one of documented exceptions per
feature group, each case a stylesheet, a small widget tree and the outcomes
the spec fixes for them — computed values, arranged bounds, hit tests
and paint probes. Many adapt a named Web Platform Tests `css/` test
(its path in the case's `wpt` field), and every case cites the
spec section its expectations follow; each expectation is derived from
the spec, none from the engine's output. A failure names the case, its
source and the expected and actual values. The exception tables assert
the documented behavior of each deliberate deviation, and the
runner checks this file states each one in the wording the case
quotes. `go test ./widget -run TestCSSConformance` runs it.

| Group | Covers | docs/css.md | Cases (`TestCSSConformance…`) |
| --- | --- | --- | --- |
| Layout | `display`; flexbox; grid (incl. `subgrid`); sizing, `box-sizing`, `aspect-ratio`, the content keywords; `position`, insets, `z-index`; `overflow`, `float`/`clear`, `visibility`, `object-fit`/`object-position` | [Layout](#layout), [Layout limits](#layout-limits) | `Layout` (flexbox, grid, display, normal flow, sizing, positioning, overflow and visibility, object-fit), `LayoutExceptions` |
| Layout (text) | `overflow-wrap` | [Properties](#properties) — Text | `Text`: overflow-wrap cases |
| Selectors | attribute selectors and flags; `:is()`/`:where()`/`:has()`/`:not()`; the of-type family, `:empty`, `:enabled`, `:read-only`/`:read-write`, `:placeholder-shown`, `:focus-visible`, `:target`; `::before`/`::after` with `content`, `attr()`, counters; `::placeholder`/`::selection`/`::marker` | [Selectors](#selectors), [Syntax and selector limits](#syntax-and-selector-limits) | `Selectors`, `SelectorsExceptions` |
| At-rules and cascade | `@media` (every feature, range syntax, three-valued logic); `@container` and its units; `@supports`, `@import`, `@layer`, `@font-face`, `@property`, `@scope`; nesting; `!important`; `revert`, `revert-layer` | [Syntax](#syntax), [Syntax and selector limits](#syntax-and-selector-limits), [Cascade](#cascade), [Media state](#media-state), [Container queries](#container-queries) | `Cascade`, `CascadeExceptions` |
| Values and functions | every unit family; the math functions and constants; the color spaces, gamut mapping, relative colors, `light-dark()`, `color-mix()`, system colors, `currentColor`; `env()`, typed `attr()`, `var()` everywhere | [Values](#values) | `Values` |
| Rendering | `filter`, `backdrop-filter`; `mix-blend-mode`, `isolation`; `clip-path`; `mask`; 3-D transforms; multiple backgrounds, `url()` images, `image-set()`; `border-image`; `outline: auto` | [Backgrounds](#backgrounds), [Rendering effects](#rendering-effects), [Rendering limits](#rendering-limits), [Properties](#properties) — Box | `Rendering`, `BorderImage`, `RenderingExceptions` |
| Text | `text-align`, `text-indent`, `white-space`, `word-break`, `line-break`, `hyphens`, `word-spacing`, `vertical-align`, `line-clamp`, `text-overflow`; `font-variant-*`, `font-synthesis`, `font-optical-sizing`, `font-palette`; `direction`, `unicode-bidi`, `writing-mode`; fonts, `text-decoration`, `text-shadow` | [Properties](#properties) — Text, [Text limits](#text-limits) | `Text`, `TextExceptions` |
| Interaction and UI | `cursor`, `pointer-events`, `user-select`, `touch-action`, `caret-*`, `accent-color`, `appearance`; the scrolling group; transitions and animations, timing functions, `animation-composition`, `allow-discrete`, `@starting-style` | [Properties](#properties) — Interaction, Scrolling, Motion; [Interaction limits](#interaction-limits) | `Interaction`, `InteractionExceptions` |

The deliberate exceptions, each with its reason in the section the
table names: `subgrid` lays out as `none`; widgets are atomic inline
boxes; every CSS box roots its own formatting context (no margin
collapsing through a parent); stacking contexts are per container; an
absolute flex child's static position is the content start; `min()`,
`max()` and `clamp()` cannot mix percentages with lengths; the window
root takes the window size; a programmatic container clamps an
oversized child; `::first-line` never matches; registered custom
properties do not animate; GTK's `@define-color` may precede an
`@import`; `url()` filters, `preserve-3d`, the general 3-D matrix
functions, mask layer lists and `background-blend-mode`; `hyphens:
auto` without a dictionary, horizontal-only writing modes, `text-indent`
and `word-spacing` percentages, `text-overflow` regardless of
`overflow`, the first CPAL palette only; `appearance: none` and
`touch-action`'s pinch half.

## Performance

Style is cached per widget and invalidated, never re-matched per paint:

- Rules are bucketed by their rightmost simple selector.
- Matching and computing a widget's style (`style.Compute`) allocates
  nothing (`TestComputeVarFreeAllocatesNothing` and
  `TestSelectorFormsAllocateNothingPerMatch` in internal/style), and
  every selector form keeps that: attribute tests read the filled
  target, nested `:is()`/`:not()` reuse a fixed buffer pool per
  nesting level, `:nth-child(of S)` fills a per-depth sibling buffer,
  and `:has()` is the only downward walk — budgeted (above). The
  revert chain records declarations only when a loaded sheet
  carries a `revert` or `revert-layer` value, and the @scope
  proximity walk runs only for scoped rules that matched.
- A stylesheet load bumps a generation, and widgets recompute lazily by
  stamp compare, so nothing walks the tree. A recompute lands in one
  shared buffer and is compared with the cached values
  (`style.Values.Equal`: the comparable values in one `==`, the list
  groups beside them); an unchanged result touches nothing — no copy,
  no relayout or subtree checks — and a changed one invalidates what
  moved. The matcher clears only the longhands the previous compute
  offered.
- A class, id or state flip restyles the subtree only when a loaded
  selector tests that fact on an ancestor, and siblings only when
  sibling selectors are loaded.
- The steady-state frame reads the computed struct and makes no style
  calls. Rendering effects (a filter, a blend mode, a clip path, a
  mask, a backdrop filter) paint the widget into an offscreen layer
  cached on the widget across frames — allocated once per size — and
  composite it back through the effect chain; the no-effect frame
  never allocates (the budget tests pin it).
- A tree that declares no layout property takes the programmatic
  layout unchanged. The CSS layouts run their passes in per-depth
  buffers reused across frames, so a static flex and flow tree
  measures and arranges without allocating
  (`TestCSSLayoutSteadyFrameAllocatesNothing`); a layout property's
  change relayouts only the branch it sits in.

The `BenchmarkStyleGallery*` benchmarks (73 widgets, 25 rules)
measure it: the full-tree restyle and the single-widget class toggle
allocate nothing, and a styled frame paints in the unstyled frame's
time ([bench/baseline.md](../bench/baseline.md) has the numbers).
