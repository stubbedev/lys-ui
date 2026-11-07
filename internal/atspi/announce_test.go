package atspi

import (
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/widget"
)

// announcementRecorder keeps the Announcement events the bridge sends.
type announcementRecorder struct {
	mu   sync.Mutex
	seen []event
}

func (r *announcementRecorder) emit(ev event) error {
	if ev.name == eventObject("Announcement") {
		r.mu.Lock()
		r.seen = append(r.seen, ev)
		r.mu.Unlock()
	}
	return nil
}

func (r *announcementRecorder) take() []event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.seen
	r.seen = nil
	return out
}

func (r *announcementRecorder) pass(br *Bridge) []event {
	br.check()
	br.settle()
	return r.take()
}

func announcedText(ev event) (string, int32) {
	text, _ := ev.args[3].(dbus.Variant).Value().(string)
	politeness, _ := ev.args[1].(int32)
	return text, politeness
}

func TestAnnounceSpeaksOnceOnTheWidgetsObject(t *testing.T) {
	rec := &announcementRecorder{}
	scene := &countingScene{}
	br, lbl := serveLabel(t, scene, rec.emit)
	widget.Announce(lbl, "3 new messages", widget.Assertive)
	got := rec.pass(br)
	if len(got) != 1 {
		t.Fatalf("%d announcements, want one", len(got))
	}
	text, politeness := announcedText(got[0])
	if got[0].path != widgetPath(idOf(t, br, lbl)) || text != "3 new messages" || politeness != int32(widget.Assertive) {
		t.Errorf("announcement %+v, want the text, assertive, on the label's object", got[0])
	}
	if again := rec.pass(br); len(again) != 0 {
		t.Errorf("the next pass spoke %d announcements again", len(again))
	}
}

func TestALiveRegionChangingThriceInAPassSpeaksItsLastText(t *testing.T) {
	rec := &announcementRecorder{}
	scene := &countingScene{}
	br, lbl := serveLabel(t, scene, rec.emit)
	region := lbl.Parent()
	widget.SetLiveRegion(region, widget.Polite)
	for _, v := range []string{"Volume 40%", "Volume 45%", "Volume 50%"} {
		lbl.SetText(v)
	}
	got := rec.pass(br)
	if len(got) != 1 {
		t.Fatalf("%d announcements for three changes in a pass, want one", len(got))
	}
	text, politeness := announcedText(got[0])
	if text != "Volume 50%" || politeness != int32(widget.Polite) || got[0].path != widgetPath(idOf(t, br, region)) {
		t.Errorf("announcement %+v, want the last text, polite, on the region", got[0])
	}
	widget.SetLiveRegion(region, 0)
	lbl.SetText("Volume 55%")
	if got := rec.pass(br); len(got) != 0 {
		t.Errorf("a region turned off spoke %d announcements", len(got))
	}
}

func TestAnAnnouncementFromAWidgetNoWindowShowsIsNotSpoken(t *testing.T) {
	rec := &announcementRecorder{}
	scene := &countingScene{}
	br, _ := serveLabel(t, scene, rec.emit)
	hidden := widget.NewToast(faceOf(t), "Saved", time.Second)
	widget.Announce(hidden, hidden.Text(), widget.LiveRegion(hidden))
	hidden.SetText("Saved again")
	if got := rec.pass(br); len(got) != 0 {
		t.Errorf("a toast no window shows spoke %d announcements", len(got))
	}
	shown := widget.NewToast(faceOf(t), "Copied", time.Second)
	root := scene.roots[0].(*widget.Box)
	root.Append(shown, false)
	arrange(t, root, 300, 100)
	widget.Announce(shown, shown.Text(), widget.LiveRegion(shown))
	got := rec.pass(br)
	if len(got) != 1 {
		t.Fatalf("%d announcements for a shown toast, want one", len(got))
	}
	if text, politeness := announcedText(got[0]); text != "Copied" || politeness != int32(widget.Polite) {
		t.Errorf("toast announcement %q at %d, want it polite", text, politeness)
	}
}

func TestAnnounceWithoutAListenerDoesNothing(t *testing.T) {
	lbl := widget.NewLabel(faceOf(t), 13, "x", render.RGB(0, 0, 0))
	widget.SetLiveRegion(lbl, widget.Polite)
	allocs := testing.AllocsPerRun(100, func() { widget.Announce(lbl, "unheard", widget.Assertive) })
	lbl.SetText("changed")
	if allocs != 0 {
		t.Errorf("announcing with no listener allocates %v times", allocs)
	}
	if got := widget.TakeAnnouncements(nil); len(got) != 0 {
		t.Errorf("%d announcements queued with no listener", len(got))
	}
}
