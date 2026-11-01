# Demos

Each demo is a command under `cmd/`. Run one from the dev shell with
`go run ./cmd/<name>`.

| Command | What it shows |
| --- | --- |
| `cmd/lys-ui-hello` | the showcase as a component: core widgets, a context menu on typed actions, tooltips, Tab focus, drag-to-move |
| `cmd/lys-ui-gallery` | every component in one window, both palettes; `-shots` screenshots each for this site |
| `cmd/lys-ui-glass` | the theme's glass surfaces over a moving backdrop, and a window asking the compositor to blur the desktop behind it |
| `cmd/lys-ui-bar` | a layer-shell top bar repainting only what changed each second |
| `cmd/lys-ui-panel` | a right-anchored layer-shell panel with live widgets |
| `cmd/lys-ui-launcher` | a launcher: a keyboard-exclusive overlay, a filtered app grid, live toplevel thumbnails through `capture` |
| `cmd/lys-ui-osd` | an OSD: a transient layer-shell volume or brightness popup over the overlay layer |
| `cmd/lys-ui-lock` | a lock screen: one ext-session-lock surface per output with an auth entry |
| `cmd/lys-ui-multi` | one loop: per-output layer bars, on-demand windows, close-request veto |
| `cmd/lys-ui-invoke` | a spawned command and `Every` feeding a component |
| `cmd/lys-ui-messages` | the typed messaging layer and single-instance forwarding |
| `cmd/lys-ui-settings` | a settings app: PreferencesView rows, the adaptive shell, typed persisted settings |
| `cmd/lys-ui-columns` | ten thousand sorted, filtered rows; the tree view; drag-to-reorder |
| `cmd/lys-ui-dialogs` | every dialog constructor: message, prompt, color, font, calendar, file, adaptive, about |
| `cmd/lys-ui-dnd` | cross-window drag-and-drop: swatches that drag and drop between two toplevels |
| `cmd/lys-ui-draw` | custom painting: a DrawingArea animating straight onto the Canvas |
| `cmd/lys-ui-ime` | IME composition: preedit and commit mirrored from an entry and a text area |
| `cmd/lys-ui-a11y` | a labeled form served over AT-SPI; builds with `-tags atspi` to force the bridge on |
| `cmd/lys-ui-capture` | the capture consumer: list outputs and toplevels, screenshot any of them to PNG, follow one output frame by frame |
| `cmd/lys-ui-gpu` | a GPUArea paced by its own frame callbacks, with lys-ui drawing over it |
| `cmd/lys-ui-video` | a video with its controls: a file through ffmpeg, or a generated clip decoded in pure Go |
| `cmd/lys-ui-i18n` | a gettext .po catalog (plurals included) localizing built-in and app strings |
| `cmd/lys-ui-states` | the window-state client the headless suite drives: xdg state chords, mirrored from what the compositor confirms |
| `cmd/lys-ui-multilist` | the multi-select client the headless suite drives: a multiple-selection list with keyboard and rubber-band selection |
| `cmd/lys-ui-popover` | the popover client the headless suite drives: popup grabs, live repaints, nested menus |
| `cmd/wlpointer`, `cmd/zz-vpclick` | synthetic input for the headless suite |
