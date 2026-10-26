package wlsession

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/dmabufwl"
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
		Modifiers: map[uint32][]uint64{dmabuf.FormatARGB8888: {7, 0}},
	})
	if dev, ok := s.DmabufDevice(); !ok || dev != 0xe280 {
		t.Errorf("main device %#x, %v; want 0xe280", dev, ok)
	}
	if got := s.DmabufModifiers(dmabuf.FormatARGB8888); !slices.Equal(got, []uint64{0, 7}) {
		t.Errorf("ARGB8888 modifiers %v, want [0 7]", got)
	}
}
