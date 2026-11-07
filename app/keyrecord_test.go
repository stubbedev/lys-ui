package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/xkb"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

type recordFixture struct {
	app                       *Application
	rec                       *widget.ShortcutRecorder
	tr                        *fakeTranslator
	accelFired, captured, key int
}

const (
	recordCodeP    = 25
	recordCodeCtrl = 29
	recordCodeAlt  = 56
)

func newRecordFixture(t *testing.T) *recordFixture {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	f := &recordFixture{
		app: idleBudgetApp(t),
		rec: widget.NewShortcutRecorder(face, ""),
		tr: &fakeTranslator{
			syms: map[uint32]xkb.Keysym{recordCodeP: xkb.Keysym('P'), recordCodeCtrl: xkb.KeyControlL, recordCodeAlt: xkb.KeyAltL},
			text: map[uint32]string{recordCodeP: "p"},
		},
	}
	if err := f.app.AddAccel("ctrl+alt+p", widget.NewAction("print", func() { f.accelFired++ })); err != nil {
		t.Fatal(err)
	}
	w := f.app.windows[0]
	w.router.Root = f.rec
	w.cfg.keyCapture = func(Accel) bool { f.captured++; return false }
	w.cfg.onKey = func(*widget.Router, uint32, wlsession.Mods) { f.key++ }
	if err := w.router.SetFocus(widget.FirstFocusable(f.rec)); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *recordFixture) press(code uint32, mods wlsession.Mods) {
	f.app.deliverKey(f.tr, code, mods)
}

func (f *recordFixture) shows(text string) bool {
	for _, st := range widget.DescribeTree(f.rec) {
		if st.Role == widget.RoleLabel && st.Text == text {
			return true
		}
	}
	return false
}

func TestARecordingWidgetTakesThePressFromEveryoneElse(t *testing.T) {
	f := newRecordFixture(t)
	var changes []string
	f.rec.OnChanged = func(a string) { changes = append(changes, a) }
	f.rec.StartRecording()

	f.press(recordCodeCtrl, 0)
	f.press(recordCodeAlt, wlsession.ModCtrl)
	f.press(recordCodeP, wlsession.ModCtrl|wlsession.ModAlt|wlsession.ModCapsLock)

	if len(changes) != 1 || changes[0] != "Ctrl+Alt+P" {
		t.Fatalf("the recorder committed %v, want [Ctrl+Alt+P]", changes)
	}
	if f.accelFired != 0 || f.captured != 0 || f.key != 0 {
		t.Errorf("while recording the press reached the accelerator %d, key capture %d, OnKey %d times; want 0 each", f.accelFired, f.captured, f.key)
	}

	f.press(recordCodeP, wlsession.ModCtrl|wlsession.ModAlt)
	if f.accelFired != 1 || f.captured != 1 {
		t.Errorf("at rest the press reached the accelerator %d and key capture %d times, want 1 each", f.accelFired, f.captured)
	}
}

func TestARecordingWidgetHearsModifierReleases(t *testing.T) {
	f := newRecordFixture(t)
	fired := 0
	f.rec.OnChanged = func(string) { fired++ }
	f.rec.StartRecording()

	f.press(recordCodeCtrl, 0)
	if !f.shows("Ctrl") {
		t.Fatal("held Ctrl is not previewed")
	}
	f.app.keyUp(f.tr, recordCodeCtrl)
	if f.shows("Ctrl") || !f.shows("Press a shortcut…") {
		t.Fatal("released Ctrl is still previewed")
	}
	f.press(recordCodeP, 0)

	if fired != 1 || f.rec.Accel() != "P" {
		t.Errorf("after releasing Ctrl a plain P committed %q (%d changes), want P", f.rec.Accel(), fired)
	}
	f.app.keyUp(f.tr, recordCodeP)
	if f.rec.Recording() {
		t.Error("a release at rest restarted recording")
	}
}

type countedInhibitor struct{ destroyed *int }

func (c countedInhibitor) Destroy() { *c.destroyed++ }

func TestTheCompositorsShortcutsAreInhibitedExactlyWhileRecording(t *testing.T) {
	f := newRecordFixture(t)
	created, destroyed := 0, 0
	f.app.inhibitShortcuts = func(*hostWindow) (inhibitor, error) {
		created++
		return countedInhibitor{&destroyed}, nil
	}
	tick := func() {
		if err := f.app.tick(f.app.step, budgetNow); err != nil {
			t.Fatal(err)
		}
	}
	tick()
	if created != 0 {
		t.Fatalf("a resting recorder inhibited the compositor's shortcuts %d times", created)
	}
	f.rec.StartRecording()
	tick()
	tick()
	if created != 1 || destroyed != 0 {
		t.Fatalf("while recording: %d inhibitors made, %d destroyed; want 1 and 0", created, destroyed)
	}
	f.press(recordCodeAlt, 0)
	f.press(recordCodeP, wlsession.ModAlt)
	tick()
	if f.rec.Recording() || created != 1 || destroyed != 1 {
		t.Fatalf("after the commit: recording %v, %d made, %d destroyed; want false, 1, 1", f.rec.Recording(), created, destroyed)
	}
	f.rec.StartRecording()
	tick()
	f.app.windows[0].release()
	if created != 2 || destroyed != 2 {
		t.Fatalf("a window closed while recording: %d made, %d destroyed; want 2 and 2", created, destroyed)
	}
}
