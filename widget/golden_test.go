package widget

// The golden-image harness and the widget snapshot set. NewGolden
// Measures, Arranges, and Paints a widget into a canonical buffer and
// compares the pixels byte-exactly against a committed PNG under
// testdata/golden (see internal/golden for the tolerance and update
// policy, render/testdata/README.md for the fixture font). Every shot
// shapes text with the bundled Cantarell face and pins its theme, so
// the same pixels come out on any machine and there is no skip path.
//
// Each snapshot doubles as the paint race guard: the harness paints
// twice (cold caches, then warm) and fails unless both buffers match
// and the widget's state — the #41 debug dump, hashed — is unchanged
// across both paints. Paint must be a pure function of widget state.

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/golden"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// goldenPad is the margin between the widget and its shot's edge, so
// focus rings and hover washes have room and nothing clips.
const goldenPad = 12

// goldenShot carries the per-snapshot options.
type goldenShot struct {
	// theme is pinned for the shot; nil means DarkTheme.
	theme *Theme
	// frame is the rect the widget is arranged into; zero means the
	// widget's measured natural size.
	frame Size
	// focus, when set, gets the keyboard focus ring drawn around it,
	// exactly as the app window draws it.
	focus Widget
	// afterArrange runs between Arrange and the first paint: the hook
	// for state that needs final geometry (hover coordinates, scroll
	// offsets, pans).
	afterArrange func()
}

// paletteShot is one palette's shot of a golden: its name and theme.
type paletteShot struct {
	name string
	th   *Theme
}

// bothPalettes is name's shots in the two palettes: name in the dark
// one, name-light in the light one.
func bothPalettes(name string) [2]paletteShot {
	return [2]paletteShot{{name, DarkTheme()}, {name + "-light", LightTheme()}}
}

// goldenOption mutates one shot's configuration.
type goldenOption func(*goldenShot)

// goldenTheme pins the palette the shot paints with.
func goldenTheme(t *Theme) goldenOption { return func(s *goldenShot) { s.theme = t } }

// goldenFrame arranges the widget into a fixed rect instead of its
// natural size — the viewport, field-width, and wrap-width cases.
func goldenFrame(w, h int) goldenOption {
	return func(s *goldenShot) { s.frame = Size{W: w, H: h} }
}

// goldenFocus draws the keyboard focus ring around w.
func goldenFocus(w Widget) goldenOption { return func(s *goldenShot) { s.focus = w } }

// goldenAfterArrange registers a hook that runs after Arrange, before
// the first paint.
func goldenAfterArrange(f func()) goldenOption { return func(s *goldenShot) { s.afterArrange = f } }

// NewGolden renders w into a canonical buffer and compares it against
// testdata/golden/name.png. It fails when the golden is missing, when
// the pixels differ, when the second (warm-cache) paint produces
// different pixels, or when Paint mutated any widget state.
func NewGolden(t *testing.T, w Widget, name string, opts ...goldenOption) {
	t.Helper()
	cfg := goldenShot{}
	for _, o := range opts {
		o(&cfg)
	}

	// Deterministic motion: every tween collapses to its end state, so
	// scroll-bar fades and enter/exit animations paint their settled
	// frame regardless of wall-clock timing.
	t.Cleanup(anim.SetInstant(true))

	// Pin the theme per shot: a golden must never depend on what a
	// previous test left in the global.
	prevTheme := Current()
	SetTheme(cfg.theme) // nil restores DarkTheme
	defer SetTheme(prevTheme)
	theme := Current()

	frame := cfg.frame
	if frame == (Size{}) {
		frame = w.Measure(Constraints{Max: Size{W: 4096, H: 4096}})
	} else {
		w.Measure(Constraints{Max: Size{W: frame.W, H: frame.H}})
	}
	w.Arrange(render.Rect{X: goldenPad, Y: goldenPad, W: frame.W, H: frame.H})
	if cfg.afterArrange != nil {
		cfg.afterArrange()
	}

	bg := theme.Bg
	bw, bh := frame.W+2*goldenPad, frame.H+2*goldenPad
	stride := render.Stride(bw)
	if cfg.focus != nil {
		if _, ok := cfg.focus.(Boundser); !ok {
			t.Fatalf("golden focus target %T does not report Bounds", cfg.focus)
		}
	}
	paint := func() []byte {
		buf := make([]byte, stride*bh)
		cv := render.New(buf, stride, bw, bh)
		cv.Clear(cv.Rect(), bg)
		if cfg.focus != nil {
			cv.MarkFocus(FocusRingOwner(cfg.focus), func(cv *render.Canvas) { PaintFocusRing(cv, cfg.focus) })
		}
		w.Paint(cv)
		cv.FinishFocus()
		return buf
	}

	// Text fields paint their caret only while focused: a shot shows the
	// caret, as the goldens always have.
	defer caretsForShot(w)()
	before := goldenStateHash(w)
	first := paint()
	warmHash := goldenStateHash(w)
	second := paint()

	if before != warmHash {
		t.Fatalf("%s: Paint mutated widget state (state hash changed after the first paint):\n%s", name, DumpTree(w, nil))
	}
	if warmHash != goldenStateHash(w) {
		t.Fatalf("%s: Paint mutated widget state (state hash changed after the second paint):\n%s", name, DumpTree(w, nil))
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("%s: Paint is not idempotent: the second (warm-cache) paint differs from the first", name)
	}
	golden.Check(t, filepath.Join("testdata", "golden"), name, render.NRGBA(first, stride, bw, bh), golden.Tolerance{})
}

// goldenStateHash hashes the widget tree's live state — type, name,
// bounds, role, value, text, caret, selection, checked — as the #41
// debug dump reports it. Two hashes differing means Paint wrote state.
func goldenStateHash(w Widget) string {
	h := fnv.New64a()
	for _, ni := range InspectTree(w, nil) {
		fmt.Fprintf(h, "%+v\n", ni)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// goldenFace loads the bundled fixture face; goldens never consult the
// host's fonts.
func goldenFace(tb testing.TB) *render.Typeface {
	tb.Helper()
	tf, err := render.NewFixtureTypeface()
	if err != nil {
		tb.Fatalf("load fixture font: %v", err)
	}
	return tf
}

func TestGoldenLabel(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	NewGolden(t, NewLabel(face, 14, "Save", th.Text), "label-short", goldenTheme(th))
	NewGolden(t, NewLabel(face, 14, "The quick brown fox jumps over the lazy dog while the compositor waits for a frame", th.Text), "label-long", goldenTheme(th))
}

// TestGoldenLabelWrapped covers wrapped text presentation through the
// toolkit's wrapping text widget; Label's own wrap mode is pinned by
// TestGoldenLabelModes.
func TestGoldenLabelWrapped(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	ta := NewTextArea(face, 14, th.Text)
	ta.SetWrap(true)
	ta.SetText("Wrapped text breaks on spaces to fit the width it is given, " +
		"so the same string lands on different rows as the field resizes.")
	NewGolden(t, ta, "label-wrapped", goldenTheme(th), goldenFrame(200, 76))
}

// TestGoldenLabelModes pins the label layout modes: the ellipsis at
// each cut point (end for status lines, middle for paths), wrapped
// rows, and the wrap+ellipsize interaction - only the final row
// truncates. The light theme rides along on the status-line cases,
// which are the ones paired with tooltips.
func TestGoldenLabelModes(t *testing.T) {
	face := goldenFace(t)
	dark, light := DarkTheme(), LightTheme()
	const px = 14
	lineH := face.Shape("x", px).LineHeight()

	ellipsize := func(mode EllipsizeMode, text, name string, th *Theme) {
		l := NewLabel(face, px, text, th.Text)
		l.SetEllipsize(mode)
		NewGolden(t, l, name, goldenTheme(th), goldenFrame(150, lineH+3))
	}
	const status = "server-02.example.com: syncing"
	ellipsize(EllipsizeEnd, status, "label-ellipsize-end", dark)
	ellipsize(EllipsizeEnd, status, "label-ellipsize-end-light", light)
	const path = "/var/lib/lys-ui/cache/sessions.bin"
	ellipsize(EllipsizeMiddle, path, "label-ellipsize-middle", dark)
	ellipsize(EllipsizeMiddle, path, "label-ellipsize-middle-light", light)

	wrapShot := func(l *Label, name string, width int) {
		NewGolden(t, l, name, goldenTheme(dark),
			goldenFrame(width, len(l.wrapped(float64(width)))*lineH))
	}
	wrapped := NewLabel(face,
		px, "Settings panes and toast bodies wrap their description "+
			"text to the width the panel offers", dark.Text)

	wrapped.SetWrap(true)
	wrapShot(wrapped, "label-wrap-rows", 180)

	both := NewLabel(face,
		px, "wrap fills rows and the ellipsis lands only on the "+
			"final row: unbreakabletokenthatcannotfitanywhere", dark.Text)

	both.SetWrap(true)
	both.SetEllipsize(EllipsizeEnd)
	wrapShot(both, "label-wrap-ellipsize", 180)
}

func TestGoldenButtonStates(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	newButton := func() *Button {
		return NewButton(NewLabel(face, 14, "Open", th.Text), 10, 8)
	}
	NewGolden(t, newButton(), "button-idle", goldenTheme(th))
	hover := newButton()
	hover.Hovered = true
	NewGolden(t, hover, "button-hover", goldenTheme(th))
	pressed := newButton()
	pressed.Pressed = true
	NewGolden(t, pressed, "button-pressed", goldenTheme(th))
}

func TestGoldenEntry(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()

	empty := NewEntry(face, 14, th.Text)
	empty.SetPlaceholder("Type here…")
	NewGolden(t, empty, "entry-empty", goldenTheme(th), goldenFrame(140, 30))

	text := NewEntry(face, 14, th.Text)
	text.SetText("deterministic metrics")
	NewGolden(t, text, "entry-text", goldenTheme(th), goldenFrame(200, 30))

	sel := NewEntry(face, 14, th.Text)
	sel.SetText("deterministic metrics")
	sel.SelectAll()
	NewGolden(t, sel, "entry-selection", goldenTheme(th), goldenFrame(200, 30))

	pan := NewEntry(face, 14, th.Text)
	pan.SetText("a field too narrow for its text pans to follow the caret")
	NewGolden(t, pan, "entry-pan", goldenTheme(th), goldenFrame(96, 30), goldenAfterArrange(pan.MoveEnd))
}

func TestGoldenTextAreaSelection(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	ta := NewTextArea(face, 14, th.Text)
	ta.SetText("Multi-line text areas wrap, select across rows,\n" +
		"keep a caret on the visual row, and paint the\n" +
		"accent band under the selection.")
	ta.SetCursor(0, 20)
	for range 24 {
		ta.KeyAction(KeyRight, ModShift) // drag the selection across the wrap boundary
	}
	NewGolden(t, ta, "textarea-selection", goldenTheme(th), goldenFrame(220, 92))
}

func TestGoldenSliderDrag(t *testing.T) {
	s := NewSlider(0, 100, 0, 47)
	s.SetPressed(true) // mid-drag
	NewGolden(t, s, "slider-drag")
}

func TestGoldenSwitch(t *testing.T) {
	NewGolden(t, NewSwitch(false), "switch-off")
	NewGolden(t, NewSwitch(true), "switch-on")
}

func TestGoldenCheckbox(t *testing.T) {
	NewGolden(t, NewCheckButton(false), "checkbox-unchecked")
	NewGolden(t, NewCheckButton(true), "checkbox-checked")
}

func TestGoldenScrollBars(t *testing.T) {
	face := goldenFace(t)
	for _, shot := range bothPalettes("scroll-bars") {
		th := shot.th
		content := NewBox(Column, 6, 12)
		for line := range strings.SplitSeq(
			"Scrolling content taller and wider than its viewport draws the auto-hiding bars",
			" ") {
			content.Append(NewLabel(face, 14, line, th.Text), false)
		}
		content.Append(NewLabel(face, 14, "a-very-long-unbreakable-token-that-overflows-the-viewport", th.Text), false)
		sc := NewScroll(content)
		sc.ShowBars = true
		NewGolden(t, sc, shot.name, goldenTheme(th), goldenFrame(180, 120), goldenAfterArrange(func() {
			sc.SetHovered(true) // hold the bars visible: the dwell would fade them mid-shot
			sc.SetOffset(0, 24)
		}))
	}
}

func TestGoldenMenuHover(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	m := NewMenu(face, 14,
		MenuItem{Label: "New file", Accel: "Ctrl+N"},
		MenuItem{Label: "Save copy…", OnClick: func() {}},
		MenuSeparator(),
		MenuItem{Label: "Read-only action"}, // no action: painted muted
		MenuItem{Kind: ItemCheck, Label: "Word wrap", Checked: true},
		MenuItem{Label: "Sort by", Items: []MenuItem{{Label: "Name"}}},
	)
	NewGolden(t, m, "menu-hover", goldenTheme(th),
		goldenAfterArrange(func() { hoverMenuRow(m, 1) }))
}

// TestGoldenMenuMnemonics pins the mnemonic underlines (#63): explicit
// letters underline where they were resolved to, auto-resolved rows
// underline their first free letter, and a conflicting explicit letter
// leaves its row without one.
func TestGoldenMenuMnemonics(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	m := NewMenu(face, 14,
		MenuItem{Label: "New file", Accel: "Ctrl+N"},                     // auto: n
		MenuItem{Label: "Save copy…", Mnemonic: 'c', OnClick: func() {}}, // explicit c
		MenuItem{Label: "Save as", OnClick: func() {}},                   // auto: skips taken letters to a
		MenuItem{Label: "Sort by", Mnemonic: 's', OnClick: func() {}},    // explicit s
		MenuItem{Label: "Second", Mnemonic: 's'},                         // conflict: no underline
	)
	m.SetMnemonicsVisible(true)
	NewGolden(t, m, "menu-mnemonics", goldenTheme(th),
		goldenAfterArrange(func() { hoverMenuRow(m, 1) }))
}

// hoverMenuRow moves the menu's hover to row i, the pointer resting on
// a row mid-popup.
func hoverMenuRow(m *Menu, i int) { m.HoverMove(rowCenter(m, i)) }

// TestGoldenPaned pins both orientations of the splitter (#71): the
// themed handle bar at the arranged position and the panes laid around
// it.
// TestGoldenCalendar pins the month grid (#73): title and weekday
// header, the theme-ruled grid, the accent selection, and the today
// marker's border.
func TestGoldenCalendar(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	cal := NewCalendar(face, 13, Date{2026, time.September, 14})
	cal.SetToday(Date{2026, time.September, 3})
	NewGolden(t, cal, "calendar", goldenTheme(th))
}

// TestGoldenColorChooser pins the picker's look (#72): the SV square's
// stacked gradients, the hue and alpha strips, the cursor ring and
// bars, the hex entry, and both palette rows.
func TestGoldenColorChooser(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	chooser := NewColorChooser(face, 12, render.RGB(0x89, 0xB4, 0xFA))
	chooser.SetPaletteSource(func() []render.Color {
		return []render.Color{render.RGB(0xff, 0x55, 0x55), render.RGB(0x55, 0xff, 0x55)}
	}, nil)
	NewGolden(t, chooser, "color-chooser", goldenTheme(th))
}

func TestGoldenPaned(t *testing.T) {
	face := goldenFace(t)
	for i, shot := range bothPalettes("paned-row") {
		th := shot.th
		row := NewPaned(Row,
			NewLabel(face, 13, "start pane", th.Text),
			NewLabel(face, 13, "end pane", th.Text))
		NewGolden(t, row, shot.name, goldenTheme(th), goldenFrame(220, 60),
			goldenAfterArrange(func() { row.SetPosition(120) }))

		col := NewPaned(Column,
			NewLabel(face, 13, "top pane", th.Text),
			NewLabel(face, 13, "bottom pane", th.Text))
		NewGolden(t, col, bothPalettes("paned-column")[i].name, goldenTheme(th), goldenFrame(220, 90),
			goldenAfterArrange(func() { col.SetPosition(40) }))
	}
}

// TestGoldenGridSqueeze pins the deficit negotiation (#67): tracks
// shrink proportionally into an under-sized rect, wrapping labels
// rest at their token floors while floorless tracks collapse, and a
// spanning child rides the shrunken run.
func TestGoldenGridSqueeze(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	wrap := NewLabel(face, 12, "wide column words wrap here", th.Text)
	wrap.SetWrap(true)
	grid := NewGrid(8, 8)
	grid.Attach(wrap, 0, 0, 1, 1)
	grid.Attach(NewLabel(face, 12, "shrinks freely", th.Text), 1, 0, 1, 1)
	grid.Attach(NewLabel(face, 12, "spans the squeezed run", th.Text), 0, 1, 2, 1)
	NewGolden(t, grid, "grid-squeeze", goldenTheme(th), goldenFrame(150, 70))
}

func TestGoldenProgressBar(t *testing.T) {
	NewGolden(t, NewProgressBar(0.62), "progressbar")
}

func TestGoldenFocusRing(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	button := NewButton(NewLabel(face, 14, "Focused", th.Text), 10, 8)
	column := NewBox(Column, 10, 4).
		Append(button, false).
		Append(NewLabel(face, 14, "unfocused label", th.TextMuted), false)
	NewGolden(t, column, "focus-ring", goldenTheme(th), goldenFocus(button))
}

// TestGoldenTooltipCard paints the card app's openTooltip maps into a
// plateless popup, over the window it floats on.
func TestGoldenTooltipCard(t *testing.T) {
	face := goldenFace(t)
	for _, shot := range bothPalettes("tooltip-card") {
		NewGolden(t, NewTooltipCard(face, "Tooltips appear after a short dwell", false), shot.name, goldenTheme(shot.th))
	}
}

// TestGoldenLightTheme re-renders the core set under the light preset:
// the dark pass is the default above, this is the light one.
func TestGoldenLightTheme(t *testing.T) {
	face := goldenFace(t)
	th := LightTheme()
	light := goldenTheme(th)

	NewGolden(t, NewLabel(face, 14, "Save", th.Text), "label-short-light", light)

	newButton := func() *Button {
		return NewButton(NewLabel(face, 14, "Open", th.Text), 10, 8)
	}
	NewGolden(t, newButton(), "button-idle-light", light)
	hover := newButton()
	hover.Hovered = true
	NewGolden(t, hover, "button-hover-light", light)
	pressed := newButton()
	pressed.Pressed = true
	NewGolden(t, pressed, "button-pressed-light", light)

	text := NewEntry(face, 14, th.Text)
	text.SetText("deterministic metrics")
	NewGolden(t, text, "entry-text-light", light, goldenFrame(200, 30))

	sel := NewEntry(face, 14, th.Text)
	sel.SetText("deterministic metrics")
	sel.SelectAll()
	NewGolden(t, sel, "entry-selection-light", light, goldenFrame(200, 30))

	NewGolden(t, NewSwitch(true), "switch-on-light", light)
	NewGolden(t, NewCheckButton(true), "checkbox-checked-light", light)
	NewGolden(t, NewProgressBar(0.62), "progressbar-light", light)

	slider := NewSlider(0, 100, 0, 47)
	slider.SetPressed(true)
	NewGolden(t, slider, "slider-drag-light", light)

	menu := NewMenu(face, 14,
		MenuItem{Label: "New file", Accel: "Ctrl+N"},
		MenuItem{Label: "Save copy…", OnClick: func() {}},
		MenuSeparator(),
		MenuItem{Label: "Read-only action"},
	)
	NewGolden(t, menu, "menu-hover-light", light,
		goldenAfterArrange(func() { hoverMenuRow(menu, 1) }))

	button := NewButton(NewLabel(face, 14, "Focused", th.Text), 10, 8)
	column := NewBox(Column, 10, 4).
		Append(button, false).
		Append(NewLabel(face, 14, "unfocused label", th.TextMuted), false)
	NewGolden(t, column, "focus-ring-light", light, goldenFocus(button))
}

// caretsForShot marks every entry and text area under w focused, so
// the shot paints their carets; the returned func restores them.
func caretsForShot(w Widget) (restore func()) {
	var marked []*node
	walkTree(w, 0, func(k Widget, _ int) {
		switch k.(type) {
		case *Entry, *TextArea:
			if n := nodeOf(k); n != nil && !n.focused {
				n.focused = true
				n.invalidateState(style.Focus)
				marked = append(marked, n)
			}
		}
	})
	return func() {
		for _, n := range marked {
			n.focused = false
			n.invalidateState(style.Focus)
		}
	}
}
