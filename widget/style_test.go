package widget

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// loadCSS installs css for one test and removes it afterwards, so no
// stylesheet leaks into a later test or golden.
func loadCSS(t *testing.T, css string) {
	t.Helper()
	LoadStylesheet(css)
	t.Cleanup(func() { LoadStylesheet("") })
}

// bareTheme empties the theme layer for one test, so a widget shows
// the fallback it paints with no stylesheet at all, and restores the
// theme afterwards.
func bareTheme(t *testing.T) {
	t.Helper()
	themeSheet.Load("")
	t.Cleanup(func() { syncThemeSheet(Current()) })
}

// arrangeTree measures and arranges root and drains any startup damage,
// so tests start from the steady state.
func arrangeTree(t *testing.T, root Widget, w, h int) {
	t.Helper()
	root.Measure(Constraints{Max: Size{W: w, H: h}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	CollectDamage(root)
}

func TestClassAPI(t *testing.T) {
	b := NewButton(NewSpacer(0, 0), 4, 4)
	if b.HasClass("a") || HasClass(b, "a") {
		t.Fatal("fresh button has classes")
	}
	b.AddClass("a", "b")
	b.AddClass("a") // duplicate is a no-op
	if !b.HasClass("a") || !HasClass(b, "b") || len(b.classes) != 2 {
		t.Fatalf("classes = %v", b.classes)
	}
	b.RemoveClass("a", "missing")
	if b.HasClass("a") || !b.HasClass("b") {
		t.Fatalf("classes after remove = %v", b.classes)
	}
	b.SetID("save-button")
	if b.ID() != "save-button" {
		t.Fatalf("id = %q", b.ID())
	}
	b.SetID("save-button") // same id is a no-op
}

func TestCSSElementNames(t *testing.T) {
	// The pinned element-name list (docs/css.md): every node-embedding
	// widget type lowercased. The app-level surfaces (dialog, popover,
	// tooltip, toast) ride SetElement and are pinned beside their
	// constructors in the app package.
	probe := func(name string, w Widget) {
		t.Helper()
		if got := typeElementName(w); got != name {
			t.Errorf("%T element = %q, want %q", w, got, name)
		}
	}
	probe("box", NewBox(Row, 0, 0))
	probe("button", NewButton(NewSpacer(0, 0), 0, 0))
	probe("calendar", &Calendar{})
	probe("checkbutton", NewCheckButton(false))
	probe("colorchooser", &ColorChooser{})
	probe("dropdown", &Dropdown{})
	probe("elevation", NewElevation(NewSpacer(0, 0)))
	probe("entry", &Entry{})
	probe("expander", &Expander{})
	probe("fader", &Fader{})
	probe("grid", &Grid{})
	probe("image", &Icon{})
	probe("image", &Image{})
	probe("label", &Label{})
	probe("listview", &List{})
	probe("gridview", &GridView{})
	probe("inscription", &Inscription{})
	probe("row", &listRow{})
	probe("menu", &Menu{})
	probe("notebook", &Notebook{})
	probe("overlay", NewOverlay())
	probe("paned", &Paned{})
	probe("progressbar", NewProgressBar(0))
	probe("label", &RichLabel{})
	probe("scrolledwindow", &Scroll{})
	probe("separator", NewSeparator(Orientation(Row)))
	probe("scale", NewSlider(0, 1, 1, 0))
	probe("spinner", &Spinner{})
	probe("stack", NewStack())
	probe("switch", NewSwitch(false))
	probe("textview", &TextArea{})
	probe("toast", &Toast{})

	// The override: an app-level surface names its card.
	el := NewElevation(NewSpacer(0, 0))
	el.SetElement("dialog")
	if el.Element() != "dialog" {
		t.Errorf("dialog override = %q", el.Element())
	}
	if got := typeElementName(el); got != "elevation" {
		t.Errorf("override leaked into the type name: %q", got)
	}
	// Calendar is listed only to pin its constructor-time name; the
	// zero value probes the switch, not the constructor.
	_ = NewCalendar(goldenFace(t), 12, Today())
	_ = NewColorChooser(goldenFace(t), 12, 0)
	_ = NewGrid(0, 0)
	_ = NewNotebook(goldenFace(t))
}

func TestStylesheetElementAndClassMatch(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `button { border-radius: 13; } button.destructive { border-width: 3; border-style: solid; } #save { border-color: #123456; }`)
	b := NewButton(NewLabel(face, 14, "Save", 0), 8, 4)
	arrangeTree(t, b, 200, 50)
	if got := b.style(b).Radius.TopLeft; got != 13 {
		t.Fatalf("radius = %d, want the stylesheet's 13", got)
	}
	if b.style(b).Has(style.PropBorderTopWidth) {
		t.Fatal("border-width matched without the class")
	}
	b.AddClass("destructive")
	b.SetID("save")
	CollectDamage(b) // the marks drain through the damage walk
	v := b.style(b)
	if !v.Has(style.PropBorderTopWidth) || v.EffBorder().Top != 3 {
		t.Errorf("border-width = %+v, want 3 via .destructive", v)
	}
	if !v.Has(style.PropBorderTopColor) || v.BorderColor[0] != render.RGB(0x12, 0x34, 0x56) {
		t.Errorf("border-color = %+v, want #123456 via #save", v)
	}
}

func TestCascadeOrigins(t *testing.T) {
	face := goldenFace(t)
	th := Current().WithSurface(render.RGB(1, 1, 1))
	SetTheme(th)
	t.Cleanup(func() { SetTheme(DarkTheme()) })

	// No stylesheet: constructor and theme values only, bit for bit.
	loadCSS(t, "")
	l := NewLabel(face, 14, "Save", th.Text)
	arrangeTree(t, l, 100, 30)
	if _, px := l.effStyle(); px != 14 {
		t.Errorf("no-stylesheet size = %v, want the constructor's", px)
	}

	// Stylesheet over theme: font-size wins over the constructor
	// default, color over the theme on an entry.
	loadCSS(t, `label { font-size: 20; } entry { color: #abcdef; background-color: #0a0b0c; }`)
	l = NewLabel(face, 14, "Save", th.Text)
	arrangeTree(t, l, 100, 30)
	if _, px := l.effStyle(); px != 20 {
		t.Errorf("font-size = %v, want the stylesheet's 20", px)
	}
	e := NewEntry(face, 14, th.Text)
	arrangeTree(t, e, 200, 32)
	v := e.style(e)
	if v.Color != render.RGB(0xab, 0xcd, 0xef) || v.Background != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("entry cascade = %v/%v", v.Color, v.Background)
	}

	// Programmatic color above the stylesheet: the constructor color
	// keeps the entry's ink.
	e2 := NewEntry(face, 14, render.RGB(9, 9, 9))
	e2.style(e2)
	if got := pickc(e2.color, e2.style(e2), style.PropColor, e2.color); got != render.RGB(9, 9, 9) {
		t.Errorf("programmatic color lost: %v", got)
	}
}

func TestInheritanceThroughTheTree(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `box { color: #00ff00; font-size: 17; }`)
	box := NewBox(Column, 0, 0)
	// A zero programmatic color inherits the box's cascade value; a set
	// one stays.
	child := NewLabel(face, 14, "inherited", 0)
	own := NewLabel(face, 14, "own", render.RGB(1, 2, 3))
	box.Append(child, false)
	box.Append(own, false)
	arrangeTree(t, box, 200, 100)

	if got := pickc(child.color, child.style(child), style.PropColor, child.color); got != render.RGB(0, 0xff, 0) {
		t.Errorf("inherited color = %v, want the box's", got)
	}
	if _, px := child.effStyle(); px != 17 {
		t.Errorf("inherited font-size = %v, want 17", px)
	}
	if got := pickc(own.color, own.style(own), style.PropColor, own.color); got != render.RGB(1, 2, 3) {
		t.Errorf("own color clobbered by inheritance: %v", got)
	}

	// background-color does not inherit: the box's fill stays local.
	if child.style(child).Has(style.PropBackgroundColor) {
		t.Error("background-color inherited")
	}
}

func TestClassToggleInvalidatesSmallestSubtree(t *testing.T) {
	face := goldenFace(t)
	// Non-inherited change: the box repaints, the label does not.
	loadCSS(t, `.x { background-color: #101010; }`)
	box := NewBox(Column, 0, 0)
	lbl := NewLabel(face, 14, "x", render.RGB(1, 1, 1))
	box.Append(lbl, false)
	arrangeTree(t, box, 100, 50)

	box.AddClass("x")
	rects, any := CollectDamage(box)
	if !any {
		t.Fatal("class toggle produced no damage")
	}
	if got := render.UnionAll(rects); got != box.Bounds() {
		t.Errorf("damage %+v, want exactly the box %+v", got, box.Bounds())
	}
	if lbl.invalid || lbl.styleDirty {
		t.Error("non-inherited restyle touched the child")
	}
	if _, any := CollectDamage(box); any {
		t.Error("damage did not drain")
	}

	// Inherited change: the label restyles and repaints too, in the
	// same drain (the collector recomputes pre-order).
	loadCSS(t, `.y { color: #202020; }`)
	box.AddClass("y")
	rects, any = CollectDamage(box)
	if !any {
		t.Fatal("inherited toggle produced no damage")
	}
	if !slices.Contains(rects, lbl.Bounds()) {
		t.Errorf("inherited restyle did not repaint the child: %v", rects)
	}
	// The cascade value inherits; the label's own programmatic color
	// would still sit above it (pinned in TestCascadeOrigins).
	if got := lbl.style(lbl).Color; got != render.RGB(0x20, 0x20, 0x20) {
		t.Errorf("label inherited color = %v, want #202020", got)
	}
}

func TestStateToggleRestyles(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `button:hover { background-color: #334455; } button:disabled { background-color: #101010; }`)
	b := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	arrangeTree(t, b, 100, 40)

	b.SetHovered(true)
	if _, any := CollectDamage(b); !any {
		t.Fatal("hover flip produced no damage")
	}
	if got := b.style(b).Background; got != render.RGB(0x33, 0x44, 0x55) {
		t.Errorf("hover background = %v, want the :hover rule", got)
	}

	// A state the stylesheet does not name still repaints: the theme's
	// pressed shade differs from the rest shade — but the cascade does
	// not move.
	b.SetHovered(false)
	CollectDamage(b)
	before := *b.style(b)
	b.SetPressed(true)
	CollectDamage(b)
	after := *b.style(b)
	if after.Background != before.Background || after.Color != before.Color ||
		after.Padding != before.Padding || after.Set != before.Set ||
		after.Opacity != before.Opacity || after.Transform.M != before.Transform.M {
		t.Errorf("unstyled :active flip moved the cascade: %+v vs %+v", after, before)
	}

	// Disabled is inherited: a disabled box marks the whole subtree.
	box := NewBox(Column, 0, 0)
	box.Append(b, false)
	arrangeTree(t, box, 100, 40)
	box.SetEnabled(false)
	if _, any := CollectDamage(box); !any {
		t.Fatal("disable flip produced no damage")
	}
	if got := b.style(b).Background; got != render.RGB(0x10, 0x10, 0x10) {
		t.Errorf("disabled background = %v, want the :disabled rule", got)
	}
}

func TestLayoutAffectingChangeRelayouts(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `button { padding: 4; }`)
	b := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	box := NewBox(Column, 0, 0)
	box.Append(b, false)
	arrangeTree(t, box, 200, 60)
	warm := b.Bounds()

	loadCSS(t, `button { padding: 20; }`)
	// The measure cache drops by stamp; the next frame relayouts and
	// arranges with the new padding.
	box.Measure(Constraints{Max: Size{W: 200, H: 60}})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60})
	if got, want := b.Bounds().H, warm.H+32; got != want {
		t.Errorf("arranged height %d, want %d (padding 4 → 20)", got, want)
	}
}

// themeOnlyButtonSet is the properties a button has under the theme
// layer alone, the baseline an emptied application stylesheet returns
// to.
func themeOnlyButtonSet(t *testing.T) style.PropSet {
	t.Helper()
	b := NewButton(NewLabel(goldenFace(t), 14, "x", 0), 4, 4)
	return b.style(b).Set
}

func TestLoadStylesheetFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.css")
	if err := os.WriteFile(path, []byte(`button { border-radius: 9; }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadStylesheetFile(path); err != nil {
		t.Fatalf("load: %v", err)
	}
	t.Cleanup(func() { LoadStylesheet("") })
	face := goldenFace(t)
	b := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	arrangeTree(t, b, 100, 40)
	if got := b.style(b).Radius.TopLeft; got != 9 {
		t.Fatalf("radius = %d, want the file's 9", got)
	}

	// Reload replaces, not merges; empty removes.
	if err := os.WriteFile(path, []byte(`button { border-width: 2; border-style: solid; }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadStylesheetFile(path); err != nil {
		t.Fatal(err)
	}
	CollectDamage(b)
	v := b.style(b)
	if v.Radius.TopLeft != 0 || v.EffBorder().Top != 2 {
		t.Errorf("reload did not replace: radius %d border %d", v.Radius.TopLeft, v.EffBorder().Top)
	}
	LoadStylesheet("")
	CollectDamage(b)
	if v := b.style(b); v.Set != themeOnlyButtonSet(t) {
		t.Errorf("empty stylesheet left %b set, want the theme layer's %b", v.Set, themeOnlyButtonSet(t))
	}

	// A missing file errors instead of half-loading.
	if err := LoadStylesheetFile(filepath.Join(dir, "missing.css")); err == nil {
		t.Error("missing file loaded without error")
	}
}

func TestStylesheetReloadRepaintsEverything(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, "")
	root := NewBox(Column, 0, 0)
	lbl := NewLabel(face, 14, "x", 0)
	root.Append(lbl, false)
	arrangeTree(t, root, 100, 50)

	LoadStylesheet(`label { color: #123123; }`)
	rects, any := CollectDamage(root)
	if !any {
		t.Fatal("a stylesheet load produced no damage")
	}
	if got := render.UnionAll(rects); got.W < root.Bounds().W {
		t.Errorf("damage %+v does not cover the tree %+v", got, root.Bounds())
	}
	// One generation, one repaint: the second pass is clean.
	if _, any := CollectDamage(root); any {
		t.Error("reload damage did not drain")
	}
}

func TestAddStylesheetFilePriority(t *testing.T) {
	resetWatches(t)
	dir := t.TempDir()
	write := func(name, css string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(css), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	theme, err := AddStylesheetFile(write("theme.css", `button { border-radius: 9; }`), StylePriorityTheme)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(theme.Remove)
	face := goldenFace(t)
	b := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	arrangeTree(t, b, 100, 40)
	if got := b.style(b).Radius.TopLeft; got != 9 {
		t.Fatalf("theme file radius = %d, want 9", got)
	}

	// The application file beats the theme at equal specificity; and a
	// file loaded below it loses, whatever the load order.
	app, err := AddStylesheetFile(write("app.css", `button { border-radius: 5; }`), StylePriorityApplication)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Remove)
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 5 {
		t.Fatalf("app file radius = %d, want 5", got)
	}
	under, err := AddStylesheetFile(write("under.css", `button { border-radius: 20; }`), StylePriorityFallback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(under.Remove)
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 5 {
		t.Fatalf("fallback file radius = %d, still want 5", got)
	}
	app.Remove()
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 9 {
		t.Fatalf("after removing the app file radius = %d, want the theme's 9", got)
	}
}

// A scoped sheet read from a file styles only the subtree it is
// attached to, resolves its @import relative to the file, reloads when
// the file or an import changes, and stops reloading on Remove (#249).
func TestNewStylesheetFile(t *testing.T) {
	resetWatches(t)
	dir := t.TempDir()
	write := func(name, css string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(css), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("base.css", `button { border-radius: 7; }`)
	index := write("index.css", `@import "base.css";`)
	sheet, err := NewStylesheetFile(index, StylePriorityApplication)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sheet.Remove)
	face := goldenFace(t)
	inside := NewButton(NewLabel(face, 14, "in", 0), 4, 4)
	outside := NewButton(NewLabel(face, 14, "out", 0), 4, 4)
	scope := NewBox(Row, 0, 0).Append(inside, false)
	scope.AttachStylesheet(sheet)
	root := NewBox(Row, 0, 0).Append(scope, false).Append(outside, false)
	arrangeTree(t, root, 200, 40)
	radius := func(b *Button) int { return b.style(b).Radius.TopLeft }
	if radius(inside) != 7 || radius(outside) != 0 {
		t.Fatalf("radius inside %d outside %d, want the imported 7 inside only", radius(inside), radius(outside))
	}

	later := time.Now().Add(time.Hour)
	write("index.css", `@import "base.css"; button { border-radius: 3; }`)
	if err := os.Chtimes(index, later, later); err != nil {
		t.Fatal(err)
	}
	pollStylesheets()
	CollectDamage(root)
	if radius(inside) != 3 || radius(outside) != 0 {
		t.Fatalf("after the reload: inside %d outside %d, want 3 inside only", radius(inside), radius(outside))
	}

	write("base.css", `button { border-radius: 9; }`)
	write("index.css", `@import "base.css";`)
	if err := os.Chtimes(index, later.Add(time.Minute), later.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	pollStylesheets()
	CollectDamage(root)
	write("base.css", `button { border-radius: 5; }`)
	if err := os.Chtimes(filepath.Join(dir, "base.css"), later.Add(2*time.Minute), later.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	pollStylesheets()
	CollectDamage(root)
	if radius(inside) != 5 {
		t.Fatalf("after editing the import: inside %d, want its 5 (#250)", radius(inside))
	}

	sheet.Remove()
	write("index.css", `button { border-radius: 11; }`)
	if err := os.Chtimes(index, later.Add(time.Hour), later.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	pollStylesheets()
	CollectDamage(root)
	if radius(inside) != 5 {
		t.Errorf("a removed sheet reloaded: inside %d, want 5", radius(inside))
	}
}

func TestAddStylesheetFileParseError(t *testing.T) {
	resetWatches(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.css")
	if err := os.WriteFile(path, []byte("button { border-radius: 4; }\nbutton {"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := AddStylesheetFile(path, StylePriorityApplication)
	if err == nil {
		t.Fatal("broken file loaded without error")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "stylesheet: parse") {
		t.Errorf("error %q names neither the path nor the parse", err)
	}
	face := goldenFace(t)
	b := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	arrangeTree(t, b, 100, 40)
	if got := b.style(b).Radius.TopLeft; got != 0 {
		t.Errorf("failed file left radius %d, want nothing applied", got)
	}
}

// resetWatches owns the poll state for one test: earlier file tests
// leave their watches and the scheduled poller behind.
func resetWatches(t *testing.T) {
	t.Helper()
	watchMu.Lock()
	watches = nil
	pollStarted = false
	watchMu.Unlock()
}

func TestStylesheetFileHotReloadPoll(t *testing.T) {
	resetWatches(t)
	// The poll hook: file loads schedule the stat tick through the
	// app-installed scheduler, exactly once for every file.
	var scheduled []func()
	SetStylesheetPoller(func(fn func()) { scheduled = append(scheduled, fn) })
	t.Cleanup(func() { SetStylesheetPoller(nil) })

	dir := t.TempDir()
	write := func(name, css string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(css), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	load := func(name, css string, priority int) *Stylesheet {
		t.Helper()
		s, err := AddStylesheetFile(write(name, css), priority)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Remove)
		return s
	}
	app := load("app.css", `button { border-radius: 5; }`, StylePriorityApplication)
	if len(scheduled) != 1 {
		t.Fatalf("poll scheduled %d times, want 1", len(scheduled))
	}
	// A second file joins the one poller without re-scheduling.
	theme := load("theme.css", `button { border-radius: 9; }`, StylePriorityTheme)
	if len(scheduled) != 1 {
		t.Fatalf("poll re-scheduled: %d", len(scheduled))
	}

	face := goldenFace(t)
	b := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	arrangeTree(t, b, 100, 40)
	if got := b.style(b).Radius.TopLeft; got != 5 {
		t.Fatalf("radius = %d, want 5", got)
	}

	// One tick stats both files; an unchanged stat is free, a changed
	// one reloads and the next read sees it.
	scheduled[0]()
	if got := b.style(b).Radius.TopLeft; got != 5 {
		t.Fatalf("unchanged stat restyled: radius %d", got)
	}
	write("app.css", `button { border-radius: 11; }`)
	write("theme.css", `button { border-radius: 3; }`)
	scheduled[0]()
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 11 {
		t.Fatalf("changed files did not reload: radius %d", got)
	}
	app.Remove()
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 3 {
		t.Fatalf("theme under a removed app file: radius %d, want 3", got)
	}

	// A removed sheet stops its poll: no reload, and no error for a
	// file that vanished after the removal.
	var warns []string
	style.SetParseWarn(func(msg string) { warns = append(warns, msg) })
	t.Cleanup(func() { style.SetParseWarn(nil) })
	if err := os.Remove(filepath.Join(dir, "app.css")); err != nil {
		t.Fatal(err)
	}
	scheduled[0]()
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 3 {
		t.Fatalf("removed sheet reloaded: radius %d", got)
	}
	if len(warns) != 0 {
		t.Fatalf("removed sheet still reported: %v", warns)
	}

	// A deleted watched file keeps its last good rules and reports the
	// path; so does one that stops parsing.
	if err := os.Remove(filepath.Join(dir, "theme.css")); err != nil {
		t.Fatal(err)
	}
	scheduled[0]()
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 3 {
		t.Fatalf("deleted file changed the sheet: radius %d", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "theme.css") {
		t.Fatalf("vanish warnings = %v, want one naming the path", warns)
	}
	theme.Remove()
	warns = warns[:0]
	load("later.css", `button { border-radius: 7; }`, StylePriorityUser)
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 7 {
		t.Fatalf("later file radius = %d, want 7", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "later.css"), []byte("button { border-radius: 8; } button {"), 0o600); err != nil {
		t.Fatal(err)
	}
	scheduled[0]()
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 7 {
		t.Fatalf("unparseable reload changed the sheet: radius %d", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "later.css") {
		t.Fatalf("parse warnings = %v, want one naming the path", warns)
	}
}

func TestLoadStylesheetFileUsesTheFilePath(t *testing.T) {
	resetWatches(t)
	var scheduled []func()
	SetStylesheetPoller(func(fn func()) { scheduled = append(scheduled, fn) })
	t.Cleanup(func() { SetStylesheetPoller(nil) })

	dir := t.TempDir()
	path := filepath.Join(dir, "app.css")
	write := func(css string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(css), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { LoadStylesheet("") })
	write(`button { border-radius: 9; }`)
	if err := LoadStylesheetFile(path); err != nil {
		t.Fatal(err)
	}
	face := goldenFace(t)
	b := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	arrangeTree(t, b, 100, 40)
	if got := b.style(b).Radius.TopLeft; got != 9 {
		t.Fatalf("radius = %d, want the file's 9", got)
	}

	// The application slot rides AddStylesheetFile: the poll reloads
	// it in place, a second load replaces it, and "" removes it (and
	// its poll).
	write(`button { border-radius: 11; }`)
	scheduled[0]()
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 11 {
		t.Fatalf("hot reload through the application slot: radius %d", got)
	}
	other := filepath.Join(dir, "other.css")
	if err := os.WriteFile(other, []byte(`button { border-radius: 2; }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadStylesheetFile(other); err != nil {
		t.Fatal(err)
	}
	CollectDamage(b)
	if got := b.style(b).Radius.TopLeft; got != 2 {
		t.Fatalf("second file did not replace the first: radius %d", got)
	}
	LoadStylesheet("")
	CollectDamage(b)
	if v := b.style(b); v.Set != themeOnlyButtonSet(t) {
		t.Errorf("empty stylesheet left %b set, want the theme layer's %b", v.Set, themeOnlyButtonSet(t))
	}
	watchMu.Lock()
	left := len(watches)
	watchMu.Unlock()
	if left != 0 {
		t.Fatalf("%d files still watched after the removal", left)
	}
}

func TestFocusBitFollowsTheRouter(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `entry:focus { border-width: 2; border-style: solid; border-color: #55aaff; }`)
	r := &Router{}
	entry := NewEntry(face, 14, 0)
	other := NewButton(NewLabel(face, 14, "x", 0), 4, 4)
	root := NewBox(Column, 4, 4)
	root.Append(entry, false)
	root.Append(other, false)
	r.Root = root
	arrangeTree(t, root, 300, 120)

	r.Press(BTNLeft, Point{X: 20, Y: 20}, 0)
	if !entry.focused {
		t.Fatal("press did not focus the entry")
	}
	if got := entry.style(entry).EffBorder().Top; got != 2 {
		t.Errorf("entry:focus border = %d, want 2", got)
	}
	r.FocusNext()
	if entry.focused {
		t.Error("focus moved but the entry kept the bit")
	}
	if other.focused && other.style(other).Has(style.PropBorderTopWidth) {
		t.Error("button matched entry:focus")
	}
}

// A state change restyles a widget's siblings only under a selector
// that reads a sibling's facts: a structural one (:last-child) depends
// on position alone, so the siblings stay clean and repaint nothing.
func TestStateChangeRestylesSiblingsOnlyForSiblingFacts(t *testing.T) {
	for _, tc := range []struct {
		css     string
		restyle bool
	}{
		{".x:last-child { color: #ff0000; }", false},
		{".x:hover + .x { color: #ff0000; }", true},
	} {
		LoadStylesheet(tc.css)
		a, b := NewBox(Row, 0, 0), NewBox(Row, 0, 0)
		a.AddClass("x")
		b.AddClass("x")
		root := NewBox(Column, 0, 0).Append(a, false).Append(b, false)
		arrangeTree(t, root, 100, 100)
		a.style(a)
		b.style(b)
		a.SetState(StateSelected, true)
		if b.styleDirty != tc.restyle {
			t.Errorf("%s: sibling restyled %v, want %v", tc.css, b.styleDirty, tc.restyle)
		}
	}
	LoadStylesheet("")
}

// Load reinstalls a removed AddStylesheet sheet, but a NewStylesheet
// sheet stays scoped: it styles nothing until attached, and then only
// its subtree.
func TestStylesheetLoadKeepsScopedSheetsScoped(t *testing.T) {
	red := render.RGB(0xff, 0, 0)
	global := AddStylesheet(".x { color: #ff0000; }", StylePriorityApplication)
	global.Remove()
	global.Load(".x { color: #ff0000; }")
	t.Cleanup(global.Remove)
	a := NewBox(Column, 0, 0)
	a.AddClass("x")
	arrangeTree(t, a, 10, 10)
	if v := a.style(a); v.Color != red {
		t.Error("Load did not reinstall a removed installed sheet")
	}
	global.Remove()

	scoped := NewStylesheet("", StylePriorityApplication)
	scoped.Load(".y { color: #ff0000; }")
	inside, outside := NewBox(Column, 0, 0), NewBox(Column, 0, 0)
	inside.AddClass("y")
	outside.AddClass("y")
	host := NewBox(Column, 0, 0).Append(inside, false)
	root := NewBox(Column, 0, 0).Append(host, false).Append(outside, false)
	arrangeTree(t, root, 10, 10)
	if v := outside.style(outside); v.Color == red {
		t.Fatal("a loaded NewStylesheet sheet styled an unattached widget")
	}
	host.AttachStylesheet(scoped)
	t.Cleanup(func() { host.DetachStylesheet(scoped) })
	if v := inside.style(inside); v.Color != red {
		t.Error("the scoped sheet does not style its attached subtree")
	}
	if v := outside.style(outside); v.Color == red {
		t.Error("the scoped sheet leaked past its subtree once attached")
	}
}
