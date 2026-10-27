package app

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/inspect"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// chordTree builds a small arranged tree for the chord tests.
func chordTree(t *testing.T) (widget.Widget, *widget.Entry) {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	entry := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	box := widget.NewBox(widget.Column, 0, 0).Append(entry, false)
	box.Measure(widget.Constraints{Max: widget.Size{W: 200, H: 100}})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 100})
	return box, entry
}

func TestInspectKey(t *testing.T) {
	tests := []struct {
		name string
		sym  xkb.Keysym
		mods wlsession.Mods
		want inspectAction
		ok   bool
	}{
		{"ctrl+shift+i toggles", 'i', wlsession.ModCtrl | wlsession.ModShift, inspectToggle, true},
		{"ctrl+shift+I toggles", 'I', wlsession.ModCtrl | wlsession.ModShift, inspectToggle, true},
		{"ctrl+shift+d dumps", 'd', wlsession.ModCtrl | wlsession.ModShift, inspectDump, true},
		{"plain i is not a chord", 'i', 0, 0, false},
		{"ctrl+i is not a chord", 'i', wlsession.ModCtrl, 0, false},
		{"shift+i is not a chord", 'i', wlsession.ModShift, 0, false},
		{"alt never chords", 'i', wlsession.ModCtrl | wlsession.ModShift | wlsession.ModAlt, 0, false},
		{"ctrl+shift+p is not a chord", 'p', wlsession.ModCtrl | wlsession.ModShift, 0, false},
	}
	for _, c := range tests {
		got, ok := inspectKey(c.sym, c.mods)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: inspectKey = (%v, %v), want (%v, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestWindowInspectToggle builds a wire-free hostWindow around an
// overlaid tree (the same wiring newWindow performs) and pins what
// the toggle does to the overlay and the damage state.
func TestWindowInspectToggle(t *testing.T) {
	root, _ := chordTree(t)
	w := &hostWindow{
		host:   &fakeHost{w: 300, h: 200},
		scale:  1,
		router: &widget.Router{Root: root},
		tip:    &tooltipCtl{since: time.Now()},
		lastW:  300, lastH: 200,
		dirty:     true,
		inspector: inspect.NewOverlay(root),
	}
	w.inspector.SetRouter(w.router)

	if w.inspector.On() {
		t.Fatal("overlay starts on")
	}
	widget.CollectDamage(w.router.Root) // drain fixture invalidations

	w.setInspect(true)
	if !w.inspector.On() {
		t.Error("setInspect(true) left the overlay off")
	}
	if !w.dirty {
		t.Error("toggling did not mark the window dirty")
	}
	// The full-window rect must be pending, so the annotations
	// actually appear even though no widget invalidated.
	if _, any := widget.CollectDamage(w.router.Root); !any && w.damage.Empty() {
		t.Error("no full-repaint rect queued; toggle would paint nothing")
	}

	w.setInspect(false)
	if w.inspector.On() {
		t.Error("setInspect(false) left the overlay on")
	}
}

// TestOverlayWrappingIsTransparent pins that the app's overlay
// wrapping keeps hit tests landing on real widgets.
func TestOverlayWrappingIsTransparent(t *testing.T) {
	root, entry := chordTree(t)
	ov := inspect.NewOverlay(root)
	ov.SetOn(true)
	hit := ov.HitTest(widget.Point{X: 10, Y: 10})
	if hit != widget.Widget(entry) {
		t.Errorf("hit = %T, want the entry through the overlay", hit)
	}
}

func TestDoctorRequested(t *testing.T) {
	t.Setenv("LYS_UI_DOCTOR", "1")
	if !doctorRequested() {
		t.Error("LYS_UI_DOCTOR=1 not requested")
	}
	t.Setenv("LYS_UI_DOCTOR", "off")
	if doctorRequested() {
		t.Error("LYS_UI_DOCTOR=off requested the doctor")
	}
}

// TestNewApplicationPrintsDoctorOnEnv runs the startup doctor against
// a zero-value session through a pipe and pins the required lines.
func TestNewApplicationPrintsDoctorOnEnv(t *testing.T) {
	t.Setenv("LYS_UI_DOCTOR", "1")

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	app := NewApplication(&wlsession.Session{})
	os.Stdout = old
	_ = w.Close()

	out, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"lys-ui doctor\n",
		"globals: (none bound)\n",
		"outputs: (none)\n",
		"cursor theme: unavailable",
		"fonts: sans=",
		"buffer: ARGB8888 premultiplied",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("startup doctor missing %q:\n%s", want, got)
		}
	}
	if app == nil {
		t.Fatal("no application built")
	}
}
