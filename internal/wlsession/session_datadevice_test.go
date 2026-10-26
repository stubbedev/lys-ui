package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// dropRecorder captures the drag-and-drop callbacks one surface got.
type dropRecorder struct {
	enters, motions, leaves, drops int
	lastX, lastY                   float64
	lastSerial                     uint32
	lastOffer                      *wl.DataOffer
}

func (h *dropRecorder) HandleDragEnter(x, y float64, serial uint32, offer *wl.DataOffer) {
	h.enters++
	h.lastX, h.lastY = x, y
	h.lastSerial = serial
	h.lastOffer = offer
}

func (h *dropRecorder) HandleDragMotion(x, y float64) {
	h.motions++
	h.lastX, h.lastY = x, y
}

func (h *dropRecorder) HandleDragLeave() { h.leaves++ }

func (h *dropRecorder) HandleDrop() { h.drops++ }

func TestSurfaceDropRouting(t *testing.T) {
	s := &Session{} // lazy handler map: no live connection needed
	s1, s2 := &wl.Surface{}, &wl.Surface{}
	h1, h2 := &dropRecorder{}, &dropRecorder{}
	s.SetSurfaceDrop(s1, h1)
	s.SetSurfaceDrop(s2, h2)

	t.Run("enter follows the named surface", func(t *testing.T) {
		s.HandleDataDeviceEnter(wl.DataDeviceEnterEvent{Surface: s1, X: 5, Y: 7, Serial: 9})
		if h1.enters != 1 || h1.lastX != 5 || h1.lastY != 7 || h1.lastSerial != 9 {
			t.Errorf("s1 got enter=%d at (%.0f,%.0f) serial=%d",
				h1.enters, h1.lastX, h1.lastY, h1.lastSerial)
		}
		if h2.enters != 0 {
			t.Errorf("s2 received %d enters while s1 was targeted", h2.enters)
		}
	})

	t.Run("motion and drop reach every surface's handler", func(t *testing.T) {
		s.HandleDataDeviceMotion(wl.DataDeviceMotionEvent{X: 6, Y: 8})
		s.HandleDataDeviceDrop(wl.DataDeviceDropEvent{})
		for _, h := range []*dropRecorder{h1, h2} {
			if h.motions != 1 || h.drops != 1 {
				t.Errorf("handler got motions=%d drops=%d, want 1/1", h.motions, h.drops)
			}
		}
	})

	t.Run("leave reaches every handler once", func(t *testing.T) {
		s.HandleDataDeviceLeave(wl.DataDeviceLeaveEvent{})
		for _, h := range []*dropRecorder{h1, h2} {
			if h.leaves != 1 {
				t.Errorf("handler got %d leaves, want 1", h.leaves)
			}
		}
	})

	t.Run("unregistered surfaces drop their events", func(t *testing.T) {
		s.HandleDataDeviceEnter(wl.DataDeviceEnterEvent{Surface: &wl.Surface{}})
		s.HandleDataDeviceEnter(wl.DataDeviceEnterEvent{})
		if h1.enters+h2.enters != 1 {
			t.Errorf("enter total = %d, want no new ones", h1.enters+h2.enters)
		}
	})

	t.Run("unregistering stops delivery", func(t *testing.T) {
		s.SetSurfaceDrop(s1, nil)
		s.HandleDataDeviceEnter(wl.DataDeviceEnterEvent{Surface: s1})
		if h1.enters != 1 {
			t.Errorf("s1 got %d enters after unregistering, want 1", h1.enters)
		}
	})
}

func TestDataDeviceVersionCap(t *testing.T) {
	if got := bindVersion(5, minDataDeviceVersion); got != minDataDeviceVersion {
		t.Errorf("bindVersion(5) = %d, want the cap %d", got, minDataDeviceVersion)
	}
	if got := bindVersion(2, minDataDeviceVersion); got != 2 {
		t.Errorf("bindVersion(2) = %d, want the advertised 2", got)
	}
	s := &Session{dataDeviceVersion: minDataDeviceVersion}
	if s.DataDeviceVersion() != minDataDeviceVersion {
		t.Errorf("DataDeviceVersion = %d, want %d", s.DataDeviceVersion(), minDataDeviceVersion)
	}
}
