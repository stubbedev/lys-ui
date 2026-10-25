package inspect

import (
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// inspectTree builds the test tree: a column with a named button
// (wrapping a label), a plain label, and an entry, arranged at a
// fixed size, plus a router with hover on the button and focus on the
// entry.
func inspectTree(t *testing.T) (widget.Widget, *widget.Router, *widget.Button) {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	inner := widget.NewLabel(face, 13, "save", render.RGB(255, 255, 255))
	button := widget.NewButton(inner, 10, 4)
	button.SetDebugName("toolbar:save")
	button.SetTooltip("saves the thing")
	hint := widget.NewLabel(face, 13, "hint", render.RGB(255, 255, 255))
	entry := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	entry.SetDebugName("search")
	column := widget.NewBox(widget.Column, 8, 4).
		Append(button, false).
		Append(hint, false).
		Append(entry, false)
	column.Measure(widget.Constraints{Max: widget.Size{W: 300, H: 200}})
	column.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})
	r := &widget.Router{Root: column}
	bs := button.Bounds()
	p := widget.Point{X: bs.X + bs.W/2, Y: bs.Y + bs.H/2}
	r.Move(p)
	r.Press(widget.BTNLeft, p, 0)
	r.Release(widget.BTNLeft, p)
	r.FocusNext() // first focusable: the button
	return column, r, button
}

func newCanvas(t *testing.T, w, h int) *render.Canvas {
	t.Helper()
	return render.New(make([]byte, render.Stride(w)*h), render.Stride(w), w, h)
}

func TestOverlayPassthrough(t *testing.T) {
	root, _, button := inspectTree(t)
	ov := NewOverlay(root)
	ov.SetOn(false)

	t.Run("measure and arrange match the root", func(t *testing.T) {
		con := widget.Constraints{Max: widget.Size{W: 300, H: 200}}
		if got, want := ov.Measure(con), root.Measure(con); got != want {
			t.Errorf("overlay measure = %v, want %v", got, want)
		}
		ov.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})
		if button.Bounds() == (render.Rect{}) {
			t.Error("arrange did not reach the wrapped tree")
		}
	})

	t.Run("hit test lands on real widgets", func(t *testing.T) {
		bs := button.Bounds()
		hit := ov.HitTest(widget.Point{X: bs.X + bs.W/2, Y: bs.Y + bs.H/2})
		if hit != widget.Widget(button) {
			t.Errorf("hit = %T, want the button", hit)
		}
	})

	t.Run("children expose the root for tree walks", func(t *testing.T) {
		kids := ov.Children()
		if len(kids) != 1 || kids[0] != root {
			t.Errorf("children = %v, want the wrapped root", kids)
		}
	})
}

func TestOverlayPaintDrawsAnnotationsWithoutMutating(t *testing.T) {
	root, r, _ := inspectTree(t)
	ov := NewOverlay(root)
	ov.SetRouter(r)
	ov.SetOn(true)
	cv := newCanvas(t, 300, 200)

	widget.CollectDamage(root) // drain fixture invalidations
	root.Measure(widget.Constraints{Max: widget.Size{W: 300, H: 200}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})
	before := widget.DumpTree(root, r)
	widget.CollectDamage(root)

	ov.Paint(cv)

	if after := widget.DumpTree(root, r); after != before {
		t.Errorf("overlay paint mutated widget state.\nbefore:\n%safter:\n%s", before, after)
	}
	if rects, any := widget.CollectDamage(root); any {
		t.Errorf("overlay paint invalidated widgets: %v", rects)
	}
	if cv.Touched() == 0 {
		t.Error("overlay painted nothing")
	}
}

func TestOverlayPaintOffDrawsRootOnly(t *testing.T) {
	root, r, _ := inspectTree(t)
	ov := NewOverlay(root)
	ov.SetRouter(r)
	ov.SetOn(false)
	cvOff := newCanvas(t, 300, 200)
	ov.Paint(cvOff)
	offTouched := cvOff.Touched()

	ov.SetOn(true)
	cvOn := newCanvas(t, 300, 200)
	ov.Paint(cvOn)
	if cvOn.Touched() <= offTouched {
		t.Errorf("annotations off touched %d px, on %d; on must draw strictly more",
			offTouched, cvOn.Touched())
	}
}

func TestAnnotationsCarryNamesAndStates(t *testing.T) {
	root, r, button := inspectTree(t)
	nodes := widget.InspectTree(root, r)
	var found, entryFocused bool
	for _, ni := range nodes {
		switch ni.DebugName {
		case "toolbar:save":
			found = true
			if ni.Bounds != button.Bounds() {
				t.Errorf("named node bounds = %v, want %v", ni.Bounds, button.Bounds())
			}
			if !ni.Hovered {
				t.Error("named button not reported as the hover target")
			}
			if ni.Focused {
				t.Error("button holds focus; FocusNext moved it to the next focusable")
			}
		case "search":
			entryFocused = ni.Focused
		}
	}
	if !found {
		t.Errorf("debug name missing from annotations:\n%s", widget.DumpTree(root, r))
	}
	if !entryFocused {
		t.Errorf("entry not focused after FocusNext:\n%s", widget.DumpTree(root, r))
	}
}

func TestPaintAnnotationsLabelFontFailureIsSafe(t *testing.T) {
	root, r, _ := inspectTree(t)
	SetLabelFont(nil) // no face: labels skip, bounds still draw
	t.Cleanup(func() {
		labelFontMu.Lock()
		labelOnce = sync.Once{}
		labelFace = nil
		labelFontMu.Unlock()
	})
	nodes := widget.InspectTree(root, r)
	cv := newCanvas(t, 300, 200)
	PaintAnnotations(cv, nodes)
	if cv.Touched() == 0 {
		t.Error("annotations without a label face painted nothing")
	}
}

func TestEnabled(t *testing.T) {
	for _, c := range []struct {
		env  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"YES", true},
		{"on", true},
		{"", false},
		{"0", false},
		{"off", false},
		{"false", false},
		{"garbage", false},
	} {
		t.Setenv("LYS_UI_INSPECT", c.env)
		if got := Enabled(); got != c.want {
			t.Errorf("LYS_UI_INSPECT=%q: Enabled = %v, want %v", c.env, got, c.want)
		}
	}
}

func TestDumpBanner(t *testing.T) {
	root, r, _ := inspectTree(t)
	dump := Dump(root, r)
	if !strings.HasPrefix(dump, "lys-ui tree dump: ") {
		t.Errorf("dump missing banner:\n%s", dump)
	}
	if !strings.Contains(dump, widget.DumpTree(root, r)) {
		t.Errorf("dump does not carry the tree:\n%s", dump)
	}
}

// doctorInfo assembles the fake-session harness: real values a fake
// session would report, pinned into the doctor block below.
func doctorInfo() wlsession.InspectInfo {
	return wlsession.InspectInfo{
		Globals: map[string]uint32{
			"wl_compositor":       4,
			"wl_output":           2,
			"wl_seat":             7,
			"wl_shm":              1,
			"wp_viewporter":       1,
			"zwlr_layer_shell_v1": 1,
		},
		Outputs: []*wlsession.Output{
			{
				Name: "DP-1", Scale: 2, ModeW: 2560, ModeH: 1440,
				LogicalX: 0, LogicalY: 0, LogicalW: 1280, LogicalH: 720,
			},
		},
		CursorTheme:     "Bibata-Modern-Ice",
		CursorSize:      24,
		FractionalScale: true,
	}
}

func TestDoctorLines(t *testing.T) {
	got := DoctorFrom(doctorInfo(), DoctorOptions{Sans: "DejaVu Sans", Mono: "JetBrains Mono"})

	for _, want := range []string{
		"lys-ui doctor\n",
		"go: ",
		"globals: wl_compositor v4, wl_output v2, wl_seat v7, wl_shm v1, wp_viewporter v1, zwlr_layer_shell_v1 v1",
		"outputs: DP-1 mode 2560x1440 scale 2 logical 0,0+1280x720",
		"cursor theme: Bibata-Modern-Ice 24px",
		"fonts: sans=\"DejaVu Sans\" mono=\"JetBrains Mono\"",
		"buffer: ARGB8888 premultiplied",
		"fractional scale: yes",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("doctor block missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "fallback=[") && !strings.Contains(got, "Noto") {
		t.Errorf("fallback families garbled:\n%s", got)
	}
}

func TestDoctorEmptySession(t *testing.T) {
	got := DoctorFrom(wlsession.InspectInfo{Globals: map[string]uint32{}}, DoctorOptions{Sans: "X", Mono: "Y"})
	for _, want := range []string{
		"globals: (none bound)\n",
		"outputs: (none)\n",
		"cursor theme: unavailable",
		"fractional scale: no",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty-session doctor missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorNilSession(t *testing.T) {
	got := Doctor(nil, DoctorOptions{})
	if !strings.Contains(got, "globals: (no session)\n") {
		t.Errorf("nil-session doctor missing the no-session line:\n%s", got)
	}
}

func TestShortType(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"*widget.Button", "Button"},
		{"*widget.Box", "Box"},
		{"*foo.Bar", "foo.Bar"},
		{"Bare", "Bare"},
	} {
		if got := shortType(c.in); got != c.want {
			t.Errorf("shortType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestOverlayCarriesInvalidationsToTheWindow pins the overlay as a link
// in the parent chain: a window is Fader -> Overlay -> root, and a
// change deep in the root must leave the fader's cache stale, or the
// window keeps the old layout.
func TestOverlayCarriesInvalidationsToTheWindow(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	label := widget.NewLabel(face, 12, "a", render.RGBA(0, 0, 0, 255))
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(label, false)
	ov := NewOverlay(root)
	fader := widget.NewFader(ov)
	con := widget.Constraints{Max: widget.Size{W: 400, H: 400}}
	for range 3 {
		fader.Measure(con)
		fader.Arrange(render.Rect{W: 400, H: 400})
	}
	if root.Parent() != ov {
		t.Fatalf("root parent = %T, want the overlay", root.Parent())
	}
	if widget.LayoutPending(fader) {
		t.Fatal("a settled window is pending")
	}
	label.SetText("a much longer label")
	if !widget.LayoutPending(fader) {
		t.Error("a label change inside the overlay never reached the window")
	}
}

// TestStyleRootKeepsScopedRootVariables pins the style root: below the
// fader and the overlay, a sheet scoped to the user root still sees
// that root as :root, so its variables resolve.
func TestStyleRootKeepsScopedRootVariables(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	label := widget.NewLabel(face, 10, "Big", render.RGBA(0, 0, 0, 255))
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(label, false)
	root.AttachStylesheet(widget.NewStylesheet(`:root { --big: 40px; } label { font-size: var(--big); }`, widget.StylePriorityUser))
	widget.SetStyleRoot(root)
	fader := widget.NewFader(NewOverlay(root))
	con := widget.Constraints{Max: widget.Size{W: 400, H: 400}}
	fader.Measure(con)
	fader.Arrange(render.Rect{W: 400, H: 400})
	for widget.LayoutPending(fader) {
		fader.Measure(con)
		fader.Arrange(render.Rect{W: 400, H: 400})
	}
	if h := label.Bounds().H; h < 40 {
		t.Errorf("label height %d: the scoped :root variable did not resolve", h)
	}
}
