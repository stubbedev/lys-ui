package wlsession

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
	"github.com/stubbedev/lys-lib/wltest"
)

// A linux-dmabuf v4 compositor's default feedback names the device it
// imports from and the layouts it imports there (#365).
func TestDmabufFeedbackNamesDeviceAndLayouts(t *testing.T) {
	s := &Session{dmabufLayouts: map[DmabufLayout]bool{}}
	if _, ok := s.DmabufDevice(); ok {
		t.Fatal("a session without feedback named a device")
	}
	s.applyDmabufFeedback(dmabufwl.Feedback{
		Device:    0xe280,
		Modifiers: map[fourcc.Format][]fourcc.Modifier{fourcc.ARGB8888: {7, 0}},
	})
	if dev, ok := s.DmabufDevice(); !ok || dev != 0xe280 {
		t.Errorf("main device %#x, %v; want 0xe280", dev, ok)
	}
	if got := s.DmabufModifiers(fourcc.ARGB8888); !slices.Equal(got, []fourcc.Modifier{0, 7}) {
		t.Errorf("ARGB8888 modifiers %v, want [0 7]", got)
	}
}

func TestASurfacesDmabufFeedbackIsFollowed(t *testing.T) {
	const device = uint64(226<<8 | 128)
	tranche := wltest.FeedbackTranche{Device: device, Flags: wlr.ZwpDmabufFeedbackV1TrancheFlagsScanout, Formats: []wltest.FormatModifier{{Format: fourcc.ARGB8888, Modifier: fourcc.Linear}}}
	renderTranche := wltest.FeedbackTranche{Device: device, Formats: tranche.Formats}
	f := wltest.NewDmabufFeedback(t, device, renderTranche)
	display, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = display.Context().Close() })
	reg, err := display.GetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{}
	if fb, err := s.WatchDmabufFeedback(nil, nil); fb != nil || err != nil {
		t.Fatalf("a session without linux-dmabuf followed feedback: %v, %v", fb, err)
	}
	b := &feedbackBinder{display: display, reg: reg}
	reg.AddGlobalHandler(b)
	roundtrip := func() {
		t.Helper()
		cb, err := display.Sync()
		if err != nil {
			t.Fatal(err)
		}
		if err := display.Context().RunTill(cb); err != nil {
			t.Fatal(err)
		}
	}
	roundtrip()
	s.dmabuf, s.dmabufVersion = b.dmabuf, 4
	surface, err := b.comp.CreateSurface()
	if err != nil {
		t.Fatal(err)
	}
	var got []dmabufwl.Feedback
	watch, err := s.WatchDmabufFeedback(surface, func(fb dmabufwl.Feedback, err error) {
		if err != nil {
			t.Error(err)
		}
		got = append(got, fb)
	})
	if err != nil || watch == nil {
		t.Fatalf("watch = %v, %v", watch, err)
	}
	roundtrip()
	f.SetFeedback(device, tranche, renderTranche)
	roundtrip()
	if len(got) != 2 || len(got[0].Tranches) != 1 || got[0].Tranches[0].Scanout() || len(got[1].Tranches) != 2 || !got[1].Tranches[0].Scanout() {
		t.Errorf("followed %+v, want the first feedback and then the scanout tranche", got)
	}
	if err := watch.Destroy(); err != nil {
		t.Fatal(err)
	}
}

type feedbackBinder struct {
	display *wl.Display
	reg     *wl.Registry
	comp    *wl.Compositor
	dmabuf  *wlr.ZwpDmabufV1
}

func (b *feedbackBinder) HandleRegistryGlobal(ev wl.RegistryGlobalEvent) {
	switch ev.Interface {
	case "wl_compositor":
		b.comp = wl.NewCompositor(b.display.Context())
		_ = b.reg.Bind(ev.Name, ev.Interface, 4, b.comp)
	case "zwp_linux_dmabuf_v1":
		b.dmabuf = wlr.NewZwpDmabufV1(b.display.Context())
		_ = b.reg.Bind(ev.Name, ev.Interface, 4, b.dmabuf)
	}
}

func (b *feedbackBinder) HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent) {}
