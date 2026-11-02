# Changelog

## v0.2.0

- `PopoverConfig.BackgroundBlur` asks the compositor to blur behind a popover through `ext_background_effect_v1`, from its first frame.
- A `PopoverClickAway` popover also closes on a bare Esc while another window has the keyboard, on a compositor offering `lys_escape_v1` (today only lys-wm). That Esc doesn't reach the window. Elsewhere nothing changes.
- Closing a popup no longer leaks its input handler and per-surface hint objects for the life of the connection.

## v0.1.2

- `PopoverConfig.Dismissal` replaces `NoAutohide`. The new
  `PopoverClickAway` closes on a click elsewhere without a seat grab, so
  keys stay with the focused window until a click into the popover.
- `PopoverConfig.Exit` plays a popover's way out whatever closes it, and
  `Popover.Close` skips it.
- A menu nested in a popover without a grab no longer kills the client with a
  protocol error.

## v0.1.1

Dependencies only: lys-lib at the commit of its v0.2.1 and the latest modules.

## v0.1.0

The first release of lys-ui, a pure-Go Wayland widget toolkit and relm4
counterpart. It contains:

- **The framework.** Components with typed input and output messages,
  controllers, child components, factories (positional and keyed),
  workers, commands and async components (`component`); typed view
  builders for every widget with watches, tracking and references
  (`ui`); one parked event loop for every window, `Invoke`/`Every`,
  typed messaging (`Stream`, `SharedState`, `Reducer`), typed actions
  and accelerators, single instance, typed persisted settings, and
  gettext localization (`i18n`).
- **Widgets.** The GTK4 and libadwaita widget set: text, buttons and
  toggles, entries and text areas with undo and IME, a code view with
  syntax highlighting, containers and adaptive layouts, virtualized
  lists, grids, column and tree views, menus, notebooks and tab views,
  preferences pages and rows, dialogs and choosers, popovers, toasts,
  tooltips, images, icons, drawing surfaces, GPU-rendered areas, and
  video with media controls.
- **The CSS engine.** GTK-flavored stylesheets above the typed theme
  palette: the selector, value and cascade levels of current CSS,
  flexbox, grid and normal-flow layout, backgrounds, rendering effects,
  transitions and animations, hot reload, and a conformance suite.
- **Platform protocols.** xdg-shell windows and layer-shell surfaces,
  fractional scale, xdg-decoration, text input, cursor shape, pointer
  constraints, touch, gestures and tablets, the clipboard, the primary
  selection and data-control, drag and drop, session lock, idle
  inhibit and notify, foreign toplevels, activation, screen capture,
  and compositor disconnect handling with an optional reconnect.
- **Desktop services.** The desktop's color scheme, accent and
  contrast; icon themes followed live; notifications; printing to the
  Print portal and to PDF; global shortcuts; the Inhibit and
  Background portals; URI and file launching; and an in-process AT-SPI
  accessibility bridge.
- **Tooling.** The live widget inspector, tree dump and doctor block;
  headless component tests; golden-image and allocation-budget tests;
  and the compositor-in-the-loop suite that gates CI on sway and
  Hyprland.

[docs/completeness.md](docs/completeness.md) maps every capability
against relm4, GTK4 and libadwaita, and [docs/css.md](docs/css.md) is
the CSS reference.
