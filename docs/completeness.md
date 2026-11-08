# Completeness

What lys-ui supports, mapped to relm4, GTK4 and libadwaita. Gaps are
listed at the end; [Non-goals](architecture.md#non-goals) are out of
scope.

## Framework

Details: [Application model](application-model.md),
[Components](components.md), [Threading](threading.md),
[Inspector](inspector.md).

| relm4 / GTK | lys-ui |
| --- | --- |
| `relm4::Application`, `ApplicationWindow` | `app.Application` (one loop for every window), `app.Window`, `app.LayerWindow` |
| main-thread contract, `glib::timeout_add` | one loop goroutine; `Application.Invoke`, `Every`; the off-loop mutation guard |
| `Component`, `Controller`, `RelmApp::run`, dialog components | `component.Component[In, Out]`, `Context`, `Controller` (`Forward`, `ForwardTo`, `Detach`, `State()`), `component.Run`/`Window`/`Layer`/`Dialog`/`Popover`; headless tests with `componenttest.Loop` |
| `view!`, `#[name]`, `#[watch]` setters, `#[relm4::widget_template]` | `ui`: a generated typed builder per widget (setters, `Watch*`, hooks, `Bind*`, adders), `Ref`, `Column`/`Row`, `If`, `Match`, `Each`, `Use`; templates are functions; `ui.Mount`/`ui.Build` |
| `Command`, `oneshot_command`, `spawn_command`, `AsyncComponent` | `Context.Oneshot`, `Spawn`, `Context()` (cancelled at shutdown); `Loader` (loading view, `Load` off the loop, held input); `Context.Await`/`Busy` for awaiting updates |
| `FactoryVecDeque`, `FactoryHashMap`, `FactoryComponent`, `AsyncFactoryComponent`, `DynamicIndex`, `FactoryView`, `FixedPosition` | `component.Factory[C, In, Out]` of item components with in-place edits (async items through `Loader`), `KeyedFactory[K, ...]`, `Index`, `BoxView`/`FlowBoxView`/`StackView`/`NotebookView`/`GridView`/`FixedView`/`ListBoxView` |
| `Worker`, `detach_worker` | `component.Worker[In, Out]` on its own goroutine, `LaunchWorker` (top-level or owned by a component), `WorkerController` |
| `tracker::track`, `#[watch]`, `#[track]` | `Context.Tracked` (`*Tracked[T]`, cleared after each refresh), `Context.Watch`, `Context.Track` |
| `MessageBroker`, `SharedState`, `Reducer` | `app.Stream[Msg]`, `app.SharedState[T]`, `app.Reducer[S, M]` |
| relm4 `binding` module | `widget.Binding[T]` + the widget `Bind*` connectors |
| GApplication single instance, `command-line`, `open` | `app.ClaimInstance` + `InstanceConfig` hooks |
| GSettings | `app.NewSettings` + typed `Key[T]`, persisted atomically |
| gettext (`gettext`, `ngettext`, `pgettext`, catalogs, locales) | `i18n`: pure-Go `.mo`/`.po` parsing, Plural-Forms, contexts, `Locales()`, `Load`/`LoadSystem`; `widget.SetMessageCatalog`, `Tr`, `TrN`, `TrCtx` for built-in and app strings |
| `RelmAction`, `RelmActionGroup`, `menu!`, `set_accels_for_action` | typed `widget.Action`, `ParamAction[P]` and `StateAction[S]` (`Toggle`, `Target`); `ActionItem`/`CheckItem`/`RadioItem` menu rows; `BindAction` on buttons and toggles; `AddAccel` (chords) and subtree-scoped `AddScopedAccel` |
| library logging | injectable `*slog.Logger` (`app.SetLogger`), silent by default |
| GTK inspector | `LYS_UI_INSPECT=1`, `Application.SetInspect`, `widget.DumpTree` |

## Widgets

Screenshots: [Widgets](widgets.md).

| relm4 / GTK / adw | lys-ui |
| --- | --- |
| Label, styled text | `Label` (UAX #14 wrap, start/middle/end ellipsize), `RichLabel` (markup, links) |
| Button, ToggleButton, CheckButton, Switch, radio groups | `Button` (any child), `ToggleButton`, `CheckButton`, `Switch`; toggles and checks join radio groups |
| Scale, ProgressBar, LevelBar, SpinButton | `Slider`, `ProgressBar`, `ProgressRing` (circular, with a centred child), `LevelBar`, `SpinButton` |
| Entry, PasswordEntry, SearchEntry, EditableLabel | `Entry` (selection, clipboard, IME, `EchoPassword`/`EchoNone`), `SearchEntry`, `SearchBar`, `ComboEntry` + completion, `EditableLabel` |
| undo/redo | bounded coalescing stack shared by `Entry` and `TextArea` |
| TextView, SourceView | `TextArea` (soft wrap, Tab trap), code view (line numbers, highlighters, auto-indent); `highlight`: TOML, Go, JSON, YAML, Markdown, Adwaita schemes |
| DropDown, ComboBox | `Dropdown`, `DropdownOf[T]`, `NewDropdownRows` (icons, headers); type-ahead |
| widget layout properties (`hexpand`, `vexpand`, `halign`, `valign`, `margin-*`, `set_size_request`, `max-width`/`max-height`) | `SetHExpand`/`SetVExpand` (propagated like GTK's computed expand), `SetHAlign`/`SetVAlign`, `SetMargin`, `SetSizeRequest` and `SetMaximumSize` (clipping what overflows) on every widget, honored by the containers |
| Box, CenterBox, Stack, Overlay, ScrolledWindow | `Box`, `CenterBox`, `Stack` (transitions), `Overlay`, `Scroll` (kinetic, `RevealRect`, smooth scrolling, snap, overscroll chaining), `Scrollbar` |
| Grid, FlowBox, adw WrapBox, Fixed | `Grid` (spans, squeeze to the children's minimums), `FlowBox`, `NewWrapBox`, `Fixed` (children at explicit (x, y) logical pixels, natural size, union measurement) |
| Paned, Frame, AspectFrame, Revealer, Expander, SizeGroup | the same names |
| Notebook | `Notebook`: close hook, reorder by drag, overflow scrolling |
| adw ToolbarView | `ToolbarView`: top and bottom bar stacks, sliding reveal, flat/raised/raised-border styles, content extending under the bars |
| adw TabView, TabBar, TabOverview | `TabView` (pages with title, icon, loading, indicator, attention, pinned; close veto; transfer between views), `TabBar` (select, close, drag to reorder or move to another window's bar), `TabOverview` (thumbnail grid); `Application.AttachTabView` binds ctrl+Tab, ctrl+PageUp/Down, alt+1..9 |
| ListView, GridView, ColumnView, TreeExpander, `TypedListView`/`TypedGridView`, ListBox | virtualized `List` (none/single/browse/multiple selection, rubber band), virtualized `GridView` (min/max columns, 2D keyboard motion, `gridview` > `child`), `ColumnView[T]` (sort, filter stack), `FlatTree[T]`; typed `TypedList[T]`/`TypedGrid[T]` over `TypedItems[T]` (widget recycling: Setup once, Bind per item, Unbind; `InsertSorted`; filter stack; typed selection), non-virtual `ListBox` (selection modes, activation, keyboard navigation, filter/sort/header functions, placeholder) |
| PopoverMenu, PopoverMenuBar | `Menu` (check/radio, submenus, mnemonics, accels), `MenuBar` |
| adw WindowTitle, adaptive AdwDialog | `WindowTitle` (title and subtitle pair); `Application.AdaptiveDialog`: a parented dialog over a wide parent, a bottom sheet inside a narrow one (or forced either way), with the dialog card, responses and cancel-on-dismiss |
| HeaderBar, WindowControls, ActionBar | the same names; `Application.AttachHeader` |
| Popover | `app.Popover`, anchored to any widget, on toplevels and layer surfaces |
| Dialog, MessageDialog, AlertDialog | `app.Dialog`, `MessageBox`, `PromptDialog`, `ProgressDialog`; modality through xdg-dialog-v1 |
| FileDialog | `Application.OpenFileDialog`, `OpenFilesDialog`, `OpenFolderDialog`, `SaveFileDialog` (pure-Go picker, XDG recents); relm4-components' `OpenButton` is `Application.NewOpenButton` (label, filters, folder mode, recent-files dropdown over the shared store) |
| ColorDialogButton, FontDialogButton, Inscription | `ColorButton` (opens a `SwatchPicker` popover of `ColorSwatch` presets, a custom row and the full `ColorChooser` in place) and `FontButton` (`Application.AttachFontButton` opens the chooser); `BindColor`/`BindFont`; `Inscription` (sized by character and line counts) |
| ColorDialog, FontDialog, Calendar, EmojiChooser | `ColorChooserDialog`, `FontChooserDialog`, `CalendarDialog`, `EmojiChooser` |
| adw AboutDialog | `Application.AboutDialog` |
| ShortcutsWindow | `ShortcutsView` + `Application.ShortcutsDialog` from the accelerator registry |
| ShortcutRecorder (GNOME Settings "Set Shortcut", KKeySequenceWidget) | `ShortcutRecorder`: keycaps at rest, records through `KeyRecorder` ahead of key capture and accelerators, live modifier preview, `Validate` |
| adw Toast | `Toast` + `Application.ShowToast` (stacking, action, hover pause) |
| Tooltip | `SetTooltip` and `SetTooltipMarkup` on any widget; `Application.SetTooltipOptions` for every window |
| GLArea | `widget.GPUArea`: application-rendered dmabufs (zero copy, async import checked against the advertised formats) or CPU images, on a subsurface below the window, paced by `OnFrame` |
| Video, MediaControls, MediaStream, MediaFile | `widget.Video` (fit policies, controls on hover or pause, autoplay, YUV frames on a subsurface of their own where the compositor imports YUV dmabufs), `widget.MediaControls`, `media.Stream` (play, pause, seek, loop, volume, mute, audio-clocked sync); sources: animated images, YUV4MPEG2 in pure Go, any file through an ffmpeg subprocess; sound through the PulseAudio protocol |
| Spinner, Separator, Image, Icon | `Spinner`, `NewSeparator`, `Image` (fit/cover, async decode, PNG/JPEG/GIF/WebP, animated GIF/APNG), `Icon` (theme, file, SVG, symbolic tint, bundled Lucide fallback) |
| DrawingArea, GestureStylus | `DrawingArea` (`OnDraw`, `OnStylus`); relm4 DrawHandler as `DrawSurface`: a retained backing canvas (incremental `Draw`, damage limited to the touched rect, resize keeps content, rescale reallocates and calls `OnResize`) |
| adw PreferencesDialog | `widget.PreferencesView` (page switcher, search over every searchable row's title and subtitle with jump-to-row results, subpages) in `Application.PreferencesDialog` with `AddToast` |
| adw PreferencesPage/Group, ActionRow family | `PreferencesPage`, `PreferencesGroup`, `ActionRow`, `SwitchRow`, `SpinRow`, `ComboRow`, `EntryRow`, `ButtonRow`, `ExpanderRow` |
| adw NavigationView, NavigationSplitView, OverlaySplitView, Carousel, ViewSwitcher, StackSwitcher, Clamp, BreakpointBin | the same names; one breakpoint engine |
| adw Banner, BottomSheet, StatusPage, Avatar, SplitButton, ButtonContent, ToggleGroup | the same names |
| drag and drop | `DragSource`/`DragEnterer` per widget, action negotiation, cross-window |
| GestureClick, EventControllerScroll | `SetOnPointer` (any button, click count, modifiers) and `SetOnScroll` (steps, modifiers) on every widget; `List.OnPointer`/`OnScroll` with the row |
| modern components (no GTK counterpart) | `Accordion`, `Alert`, `Badge`, `Breadcrumb`, `Chart`, `Combobox`, `DataTable`, `DatePicker`, `Form`, `HoverCard`, `InputOTP`, `Pagination`, `Sidebar` |
| shell parts (no GTK counterpart) | `Bar` (start, center and end zones along an edge, turning with `SetEdge`), `BarButton`, `BarSeparator`, `SegmentIndicator`, `app.NewBar`; `FloatingPanel` (drag, resize, confinement, input-region pass-through via `InputRegioner`); `Card`; `Chip` and the drag-reorderable `ChipBox` |
| RTL/bidi | UAX #9 in every text path; `Direction` on text widgets, menus, dropdowns and Box |

## Text, rendering and theming

Details: [Architecture](architecture.md), [CSS](css.md), [Appearance](appearance.md), [Icons](icons.md), [Accessibility](a11y.md).

| Area | lys-ui |
| --- | --- |
| rendering | premultiplied ARGB8888, signed-distance AA, damage-clipped repaint, gradients (linear, radial, conic, repeating) |
| GPU rendering | a Vulkan renderer into dmabufs through lys-lib's `gpu`, chosen by `LYS_UI_RENDERER` (`auto`, `gpu`, `cpu`), on the CPU where it cannot open |
| fonts | system lookup (`app.Font`, `FontWeighted`, `FontFamilies`), per-rune fallback chains, color emoji (CBDT/sbix, COLRv0/v1, OT-SVG), variable fonts |
| theming | typed `Theme` palette with copying `With*`, dark/light/high-contrast presets, WCAG guard |
| CSS | GTK-flavoured stylesheets above the palette: selectors, `@define-color`, transitions, `@keyframes`, flexbox, grid, effects; files at chosen priorities (`AddStylesheetFile`), hot-reloaded through inotify |
| glass surfaces | the `css.Glass`, `css.LiquidGlass` and `css.Frosted` style classes; behind-window and behind-popover blur through ext_background_effect_v1 (`WindowConfig`, `LayerConfig` and `PopoverConfig.BackgroundBlur`) |
| GTK settings | `gtksettings`: GTK 4's settings.ini |
| desktop settings | `portalsettings`: the appearance, icon theme and font xdg-desktop-portal publishes, followed live, for programs that draw without an `app.Application` |
| icon themes | freedesktop spec in pure Go, live icon-theme following; `icontheme` for programs that draw icons without widgets |
| accessibility | semantic roles, keyboard-first guarantee, in-process AT-SPI bridge |
| animation (`adw::TimedAnimation`, `SpringAnimation`) | `anim`: tweens, easing curves, springs, Sequence/Parallel/Delay timelines, reduced motion |

## Protocols and platform

Details: [Application model](application-model.md), [Input model](input-model.md).

| Area | lys-ui |
| --- | --- |
| windows | xdg-shell toplevels (close veto, resize edges, min/max, maximize, fullscreen per output, transient parents, move/resize grabs), wlr-layer-shell surfaces |
| decorations | xdg-decoration when the compositor decorates; `HeaderBar` is the explicit client-side opt-in |
| window icons | xdg-toplevel-icon-v1 |
| surface hints | content type, live opacity (alpha-modifier), error bell, `wm_capabilities` |
| keyboard | the compositor's xkb keymap, synthesized repeat |
| IME | zwp_text_input_v3 into Entry and TextArea, popovers included |
| pointer, cursors | cursor-shape-v1 with xcursor fallback, pointer constraints, relative motion |
| touch, gestures, tablets | wl_touch with pointer emulation, pointer-gestures, tablet-v2 stylus samples |
| fractional scale | wp_viewporter + wp_fractional_scale_v1, live rescale; integer fallback |
| clipboard | wl_data_device + primary selection; text, images, URIs, HTML |
| clipboard managers | ext-data-control-v1, wlr-data-control fallback |
| taskbars, launchers | wlr-foreign-toplevel-management, xdg-activation, `OpenURL`/`OpenPath` (portal, xdg-open fallback); xdg-foreign v2 exports every toplevel so portal dialogs parent to it |
| idle | idle inhibit, ext-idle-notify-v1 (`OnIdle`) |
| shortcuts | keyboard-shortcuts-inhibit, GlobalShortcuts portal, xx_hotkey_v1 client-chosen hotkeys |
| printing (GtkPrintDialog, GtkPrintOperation) | `Application.Print` through the Print portal, `ExportPDF`; pages painted as rasters into a PDF by `pdf` |
| session, background | Inhibit portal (`Inhibit`, `OnSessionStateChange` with query-end), Background portal (`RequestBackground`, `SetBackgroundStatus`) |
| lock screens | ext-session-lock-v1 (`LockSession`, every output, hotplug) |
| notifications | `Application.Notify` with actions |
| screen capture | `capture`: wlr-screencopy, ext-image-copy-capture, Hyprland toplevel export, dmabuf import |
| file and fd watching | `WatchFiles`, `WatchFD` on the loop |
| compositor restarts | typed disconnect, clean exit code 75, optional reconnect with rebuild |
| system appearance (`adw::StyleManager`) | `Application.ColorScheme`, `Accent`, `Contrast` with loop-delivered `On*Change` hooks; types in `appearance` |

## Known gaps

- The AT-SPI bridge has no Selection or Table interfaces and reports no
  text attributes ([Accessibility](a11y.md#limits)).
- Tablet pads are not bound.
- CSS limits are listed per section in [CSS](css.md).

## Tested compositors

Sway and Hyprland are required CI gates. Sway runs the full
compositor-in-the-loop suite (`internal/headlesstest`). Hyprland runs
the same suite except the reconnect test, which needs a compositor
restart; the compositor-kill test runs last, on its own.
