# lys-ui completeness

The capability map against relm4, GTK4 and libadwaita: what lys-ui ships
per area and where it lives. Known gaps are listed at the end; anything
not listed there is shipped. The standing non-goals and their reasons
live in [architecture.md](architecture.md#non-goals).

## Framework

| relm4 / GTK | lys-ui | Where |
| --- | --- | --- |
| `relm4::Application`, `ApplicationWindow` | `app.Application` (one loop for every window), `app.Window`, `app.LayerWindow` | app/application.go, [application-model.md](application-model.md) |
| main-thread contract, `glib::timeout_add` | one loop goroutine; `Application.Invoke`, `Every`; the off-loop mutation guard | app/invoke.go, widget/thread.go, [threading.md](threading.md) |
| `Component`, `Controller`, `RelmApp::run`, dialog components | `component.Component[In, Out]`, `Context`, `Controller` (`Forward`, `ForwardTo`, `Detach`, `State()` watcher: typed state observation after each update batch, no copy per notification), `component.Run`/`Window`/`Layer`/`Dialog`/`Popover`; headless tests with `componenttest.Loop` | component/, [components.md](components.md) |
| `view!`, `#[name]`, `#[watch]` setters, `#[relm4::widget_template]` | `ui`: a generated typed builder per widget (setters, `Watch*`, hooks, `Bind*`, adders), `Ref`, `Column`/`Row`, `If`, `Match`, `Each`, `Use`; templates are functions; `ui.Mount`/`ui.Build` | ui/, internal/uigen, [components.md](components.md#views-typed-builders) |
| `Command`, `oneshot_command`, `spawn_command`, `AsyncComponent` | `Context.Oneshot`, `Spawn`, `Context()` (cancelled at shutdown); `Loader` (loading view, `Load` off the loop, held input); `Context.Await`/`Busy` for awaiting updates | component/command.go |
| `FactoryVecDeque`, `FactoryHashMap`, `FactoryComponent`, `AsyncFactoryComponent`, `DynamicIndex`, `FactoryView`, `FixedPosition` | `component.Factory[C, In, Out]` of item components with in-place edits (async items through `Loader`: loading view first, input held, mapped to relm4's AsyncFactoryComponent), `KeyedFactory[K, ...]`, `Index`, `BoxView`/`FlowBoxView`/`StackView`/`NotebookView`/`GridView`/`FixedView`/`ListBoxView` | component/factory.go, component/views.go |
| `Worker`, `detach_worker` | `component.Worker[In, Out]` on its own goroutine, `LaunchWorker` (top-level or owned by a component), `WorkerController` | component/worker.go |
| `tracker::track`, `#[watch]`, `#[track]` | `Context.Tracked` (`*Tracked[T]`, cleared after each refresh), `Context.Watch`, `Context.Track` | component/tracked.go |
| `MessageBroker`, `SharedState`, `Reducer` | `app.Stream[Msg]`, `app.SharedState[T]`, `app.Reducer[S, M]` | app/message.go |
| relm4 `binding` module | `widget.Binding[T]` + the widget `Bind*` connectors | widget/binding.go |
| GApplication single instance, `command-line`, `open` | `app.ClaimInstance` + `InstanceConfig` hooks | app/instance.go |
| GSettings | `app.NewSettings` + typed `Key[T]`, persisted atomically | app/settings.go |
| gettext (`gettext`, `ngettext`, `pgettext`, catalogs, locales) | `i18n`: pure-Go `.mo`/`.po` parsing, Plural-Forms, contexts, `Locales()`, `Load`/`LoadSystem`; `widget.SetMessageCatalog`, `Tr`, `TrN`, `TrCtx` for built-in and app strings | i18n/, widget/message.go |
| `RelmAction`, `RelmActionGroup`, `menu!`, `set_accels_for_action` | typed `widget.Action`, `ParamAction[P]` and `StateAction[S]` (`Toggle`, `Target`); `ActionItem`/`CheckItem`/`RadioItem` menu rows; `BindAction` on buttons and toggles; `AddAccel` (chords) and subtree-scoped `AddScopedAccel` | widget/action.go, app/accel.go |
| library logging | injectable `*slog.Logger` (`app.SetLogger`), silent by default | internal/logutil |
| GTK inspector | `LYS_UI_INSPECT=1`, `Application.SetInspect`, `widget.DumpTree` | app/inspect.go, [inspector.md](inspector.md) |

## Widgets

| relm4 / GTK / adw | lys-ui | Where |
| --- | --- | --- |
| Label, styled text | `Label` (UAX #14 wrap, start/middle/end ellipsize), `RichLabel` (markup, links) | widget/label.go, widget/richlabel.go |
| Button, ToggleButton, CheckButton, Switch, radio groups | `Button` (any child), `ToggleButton`, `CheckButton`, `Switch`; toggles and checks join radio groups | widget/button.go, widget/toggle.go, widget/radio.go |
| Scale, ProgressBar, LevelBar, SpinButton | `Slider`, `ProgressBar`, `LevelBar`, `SpinButton` | widget/slider.go, widget/progressbar.go, widget/levelbar.go, widget/spin.go |
| Entry, PasswordEntry, SearchEntry, EditableLabel | `Entry` (selection, clipboard, IME, `EchoPassword`/`EchoNone`), `SearchEntry`, `SearchBar`, `ComboEntry` + completion, `EditableLabel` | widget/entry.go, widget/search.go, widget/completion.go, widget/editablelabel.go |
| undo/redo | bounded coalescing stack shared by `Entry` and `TextArea` | widget/undo.go |
| TextView, SourceView | `TextArea` (soft wrap, Tab trap), code view (line numbers, highlighters, auto-indent); `highlight`: TOML, Go, JSON, YAML, Markdown, Adwaita schemes | widget/textarea.go, widget/codeview.go, highlight/ |
| DropDown, ComboBox | `Dropdown`, `DropdownOf[T]`, `NewDropdownRows` (icons, headers); type-ahead | widget/dropdown.go |
| widget layout properties (`hexpand`, `vexpand`, `halign`, `valign`, `margin-*`, `set_size_request`, `max-width`/`max-height`) | `SetHExpand`/`SetVExpand` (propagated like GTK's computed expand), `SetHAlign`/`SetVAlign`, `SetMargin`, `SetSizeRequest` and `SetMaximumSize` (clipping what overflows) on every widget, honored by the containers | widget/layoutprops.go |
| Box, CenterBox, Stack, Overlay, ScrolledWindow | `Box`, `CenterBox`, `Stack` (transitions), `Overlay`, `Scroll` (kinetic, `RevealRect`, smooth scrolling, snap, overscroll chaining), `Scrollbar` | widget/box.go, widget/centerbox.go, widget/containers.go |
| Grid, FlowBox, adw WrapBox, Fixed | `Grid` (spans, squeeze to MinSizer floors), `FlowBox`, `NewWrapBox`, `Fixed` (children at explicit (x, y) logical pixels, natural size, union measurement) | widget/grid.go, widget/flowbox.go, widget/fixed.go |
| Paned, Frame, AspectFrame, Revealer, Expander, SizeGroup | the same names | widget/paned.go, widget/frame.go, widget/revealer.go, widget/expander.go, widget/shrink.go |
| Notebook | `Notebook`: close hook, reorder by drag, overflow scrolling | widget/notebook.go |
| adw ToolbarView | `ToolbarView`: top and bottom bar stacks, sliding reveal, flat/raised/raised-border styles, content extending under the bars | widget/toolbarview.go |
| adw TabView, TabBar, TabOverview | `TabView` (pages with title, icon, loading, indicator, attention, pinned; close veto; transfer between views), `TabBar` (select, close, drag to reorder or move to another window's bar), `TabOverview` (thumbnail grid); `Application.AttachTabView` binds ctrl+Tab, ctrl+PageUp/Down, alt+1..9 | widget/tabview.go, widget/tabbar.go, app/chrome.go |
| ListView, GridView, ColumnView, TreeExpander, `TypedListView`/`TypedGridView`, ListBox | virtualized `List` (none/single/browse/multiple selection, rubber band), virtualized `GridView` (min/max columns, 2D keyboard motion, `gridview` > `child`), `ColumnView[T]` (sort, filter stack), `FlatTree[T]`; typed `TypedList[T]`/`TypedGrid[T]` over `TypedItems[T]` (widget recycling: Setup once, Bind per item, Unbind; `InsertSorted`; filter stack; typed selection), non-virtual `ListBox` (selection modes, activation, keyboard navigation, filter/sort/header functions, placeholder) | widget/list.go, widget/gridview.go, widget/columnview.go, widget/tree.go, widget/typedview.go, widget/listbox.go |
| PopoverMenu, PopoverMenuBar | `Menu` (check/radio, submenus, mnemonics, accels), `MenuBar` | widget/menu.go, widget/menubar.go |
| adw WindowTitle, adaptive AdwDialog | `WindowTitle` (title and subtitle pair); `Application.AdaptiveDialog`: a parented dialog over a wide parent, a bottom sheet inside a narrow one (or forced either way), with the dialog card, responses and cancel-on-dismiss | widget/windowtitle.go, app/adaptivedialog.go |
| HeaderBar, WindowControls, ActionBar | the same names; `Application.AttachHeader` | widget/chrome.go, app/chrome.go |
| Popover | `app.Popover`, anchored to any widget, on toplevels and layer surfaces | app/popover.go |
| Dialog, MessageDialog, AlertDialog | `app.Dialog`, `MessageBox`, `PromptDialog`, `ProgressDialog`; modality through xdg-dialog-v1 | app/dialog.go, app/prompt.go |
| FileDialog | `Application.OpenFileDialog`, `OpenFilesDialog`, `OpenFolderDialog`, `SaveFileDialog` (pure-Go picker, XDG recents); relm4-components' `OpenButton` is `Application.NewOpenButton` (label, filters, folder mode, recent-files dropdown over the shared store) | app/filedialog.go, app/openbutton.go, widget/filechooser.go |
| ColorDialogButton, FontDialogButton, Inscription | `ColorButton` and `FontButton` (`Application.AttachColorButton`/`AttachFontButton` open the choosers; `BindColor`/`BindFont`), `Inscription` (sized by character and line counts) | widget/dialogbuttons.go, widget/inscription.go, app/chrome.go |
| ColorDialog, FontDialog, Calendar, EmojiChooser | `ColorChooserDialog`, `FontChooserDialog`, `CalendarDialog`, `EmojiChooser` | app/dialog.go, app/emojichooser.go |
| adw AboutDialog | `Application.AboutDialog` | app/about.go |
| ShortcutsWindow | `ShortcutsView` + `Application.ShortcutsDialog` from the accelerator registry | widget/shortcuts.go, app/accel.go |
| adw Toast | `Toast` + `Application.ShowToast` (stacking, action, hover pause) | widget/toast.go, app/toast.go |
| Tooltip | `SetTooltip`, `SetTooltipMarkup`, `SetTooltipOptions` on any widget | widget/widget.go, app/tooltip.go |
| GLArea | `widget.GPUArea`: application-rendered dmabufs (zero copy, async import checked against the advertised formats) or CPU images, on a subsurface below the window, paced by `OnFrame` | widget/gpuarea.go, app/gpuarea.go, dmabuf/ |
| Video, MediaControls, MediaStream, MediaFile | `widget.Video` (fit policies, controls on hover or pause, autoplay), `widget.MediaControls`, `media.Stream` (play, pause, seek, loop, volume, mute, audio-clocked sync); sources: animated images, YUV4MPEG2 in pure Go, any file through an ffmpeg subprocess; sound through the PulseAudio protocol | media/, widget/video.go, widget/mediacontrols.go |
| Spinner, Separator, Image, Icon | `Spinner`, `NewSeparator`, `Image` (fit/cover, async decode, PNG/JPEG/GIF/WebP, animated GIF/APNG), `Icon` (theme, file, SVG, symbolic tint, bundled Lucide fallback) | widget/spinner.go, widget/image.go, widget/icon.go |
| DrawingArea, GestureStylus | `DrawingArea` (`OnDraw`, `OnStylus`); relm4 DrawHandler as `DrawSurface`: a retained backing canvas (incremental `Draw`, damage limited to the touched rect, resize keeps content, rescale reallocates and calls `OnResize`) | widget/drawingarea.go, widget/drawsurface.go |
| adw PreferencesDialog | `widget.PreferencesView` (page switcher, search over every searchable row's title and subtitle with jump-to-row results, subpages) in `Application.PreferencesDialog` with `AddToast` | widget/prefsview.go, app/dialog.go |
| adw PreferencesPage/Group, ActionRow family | `PreferencesPage`, `PreferencesGroup`, `ActionRow`, `SwitchRow`, `SpinRow`, `ComboRow`, `EntryRow`, `ButtonRow`, `ExpanderRow` | widget/prefs.go, widget/actionrow.go, widget/controlrows.go |
| adw NavigationView, NavigationSplitView, OverlaySplitView, Carousel, ViewSwitcher, StackSwitcher, Clamp, BreakpointBin | the same names; one breakpoint engine | widget/navigation.go, widget/adaptive.go |
| adw Banner, BottomSheet, StatusPage, Avatar, SplitButton, ButtonContent, ToggleGroup | the same names | widget/adw.go, widget/bottomsheet.go |
| drag and drop | `DragSource`/`DragEnterer` per widget, action negotiation, cross-window | widget/input.go, app/dragdrop.go |
| RTL/bidi | UAX #9 in every text path; `Direction` on text widgets, menus, dropdowns and Box | internal/text, widget/direction.go |

## Text, rendering and theming

| Area | lys-ui | Where |
| --- | --- | --- |
| rendering | premultiplied ARGB8888, signed-distance AA, damage-clipped repaint, gradients (linear, radial, conic, repeating) | render/ |
| fonts | system lookup (`app.Font`, `FontWeighted`, `FontFamilies`), per-rune fallback chains, color emoji (CBDT/sbix, COLRv0/v1, OT-SVG), variable fonts | app/fonts.go, render/, internal/sysfont |
| theming | typed `Theme` palette with copying `With*`, dark/light/high-contrast presets, WCAG guard | widget/theme.go |
| CSS | GTK-flavored stylesheets above the palette: selectors, `@define-color`, transitions, `@keyframes`, hot reload; stylesheet files loadable at chosen priorities (`AddStylesheetFile`) with one shared poller | internal/style, widget/style.go, [css.md](css.md) |
| icon themes | freedesktop spec in pure Go, live icon-theme following | internal/icons, [icons.md](icons.md) |
| accessibility | semantic roles, keyboard-first guarantee, in-process AT-SPI bridge | widget/a11y.go, internal/atspi, [a11y.md](a11y.md) |
| animation (`adw::TimedAnimation`, `SpringAnimation`) | `anim`: tweens, easing curves, springs, Sequence/Parallel/Delay timelines, reduced motion | anim/, internal/animclock |

## Protocols and platform

| Area | lys-ui | Where |
| --- | --- | --- |
| windows | xdg-shell toplevels (close veto, resize edges, min/max, maximize, fullscreen per output, transient parents, move/resize grabs), wlr-layer-shell surfaces | internal/window, internal/layersurface, app/windowctl.go |
| decorations | xdg-decoration when the compositor decorates; `HeaderBar` is the explicit client-side opt-in | internal/window, [application-model.md](application-model.md#window-chrome) |
| window icons | xdg-toplevel-icon-v1 | app/windowicon.go |
| surface hints | content type, live opacity (alpha-modifier), error bell, `wm_capabilities` | app/surfacehints.go |
| keyboard | the compositor's xkb keymap, synthesized repeat | internal/wlsession |
| IME | zwp_text_input_v3 into Entry and TextArea, popovers included | internal/wlsession/textinput.go |
| pointer, cursors | cursor-shape-v1 with xcursor fallback, pointer constraints, relative motion | internal/wlsession, app/constraints.go |
| touch, gestures, tablets | wl_touch with pointer emulation, pointer-gestures, tablet-v2 stylus samples | internal/wlsession, widget/touch.go |
| fractional scale | wp_viewporter + wp_fractional_scale_v1, live rescale; integer fallback | internal/scale |
| clipboard | wl_data_device + primary selection; text, images, URIs, HTML | app/clipboard.go, transfer/ |
| clipboard managers | ext-data-control-v1, wlr-data-control fallback | app/datacontrol.go |
| taskbars, launchers | wlr-foreign-toplevel-management, xdg-activation, `OpenURL`/`OpenPath` (portal, xdg-open fallback); xdg-foreign v2 exports every toplevel so portal dialogs parent to it | internal/wlsession, app/launcher.go |
| idle | idle inhibit, ext-idle-notify-v1 (`OnIdle`) | app/application.go |
| shortcuts | keyboard-shortcuts-inhibit, GlobalShortcuts portal | internal/wlsession, app/globalshortcuts.go |
| printing (GtkPrintDialog, GtkPrintOperation) | `Application.Print` through the Print portal, `ExportPDF`; pages painted as rasters into a PDF by `pdf` | app/print.go, pdf/ |
| session, background | Inhibit portal (`Inhibit`, `OnSessionStateChange` with query-end), Background portal (`RequestBackground`, `SetBackgroundStatus`) | app/portals.go |
| lock screens | ext-session-lock-v1 (`LockSession`, every output, hotplug) | app/sessionlock.go |
| notifications | `Application.Notify` with actions | app/notification.go |
| screen capture | `capture`: wlr-screencopy, ext-image-copy-capture, Hyprland toplevel export, dmabuf import | capture/ |
| file and fd watching | `WatchFiles`, `WatchFD` on the loop | app/watch.go |
| compositor restarts | typed disconnect, clean exit code 75, optional reconnect with rebuild | app/disconnect.go, app/reconnect.go |
| system appearance (`adw::StyleManager`) | `Application.ColorScheme`, `Accent`, `Contrast` with loop-delivered `On*Change` hooks; types in `appearance` | app/appearance.go, appearance/, [appearance.md](appearance.md) |

## Known gaps

None. Sway and Hyprland both run the full compositor-in-the-loop
suite as required CI gates.
