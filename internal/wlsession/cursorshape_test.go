package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// fakeShapeDevice records set_shape requests.
type fakeShapeDevice struct {
	shapes   []uint32
	serials  []uint32
	destroys int
}

func (f *fakeShapeDevice) SetShape(serial, shape uint32) error {
	f.serials = append(f.serials, serial)
	f.shapes = append(f.shapes, shape)
	return nil
}
func (f *fakeShapeDevice) Destroy() error { f.destroys++; return nil }

// With the shape device, a known name is one set_shape with the enter
// serial and no buffer; an unmapped name and the hidden cursor fall
// back to the theme path; losing the pointer destroys the device.
func TestCursorShapeProtocol(t *testing.T) {
	f := newSeatFixture()
	surf := &wl.Surface{}
	f.s.SetSurfaceInput(surf, &recordingHandler{})
	f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer})
	dev := &fakeShapeDevice{}
	f.s.shapeDevice = dev
	f.s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf, Serial: 42})
	if len(dev.shapes) != 1 || dev.shapes[0] != wlr.WpCursorShapeDeviceV1ShapeDefault || dev.serials[0] != 42 || f.pushes != 0 {
		t.Fatalf("enter: shapes %v serials %v pushes %d", dev.shapes, dev.serials, f.pushes)
	}
	_ = f.s.SetCursor("resize_e")
	_ = f.s.SetCursor("xterm")
	if want := []uint32{wlr.WpCursorShapeDeviceV1ShapeDefault, wlr.WpCursorShapeDeviceV1ShapeEResize, wlr.WpCursorShapeDeviceV1ShapeText}; len(dev.shapes) != 3 || dev.shapes[1] != want[1] || dev.shapes[2] != want[2] {
		t.Errorf("shapes = %v, want %v", dev.shapes, want)
	}
	_ = f.s.SetCursor("my_theme_only_shape")
	_ = f.s.SetCursor(CursorHidden)
	if len(dev.shapes) != 3 || f.pushes != 2 {
		t.Errorf("fallbacks: shapes %v pushes %d, want the theme path twice", dev.shapes, f.pushes)
	}
	f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{})
	if dev.destroys != 1 || f.s.shapeDevice != nil {
		t.Errorf("pointer loss: destroys %d", dev.destroys)
	}
}

// Every toolkit cursor name has a protocol shape, so window edges and
// the stock shapes never fall back to the theme where the compositor
// draws cursors.
func TestCursorShapeCoversToolkit(t *testing.T) {
	for name := range cursorAliases {
		if _, ok := cursorShapes[name]; !ok {
			t.Errorf("toolkit cursor %q has no protocol shape", name)
		}
	}
	if _, ok := cursorShapes[effectiveCursor("")]; !ok {
		t.Error("the default cursor has no protocol shape")
	}
}
