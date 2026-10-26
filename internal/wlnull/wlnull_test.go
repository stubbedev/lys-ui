package wlnull

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// TestNullIsASafeProxy pins what the wire encoder relies on: Null is a
// wl.Proxy whose id is 0 and whose methods are safe on the nil pointer.
func TestNullIsASafeProxy(t *testing.T) {
	var p wl.Proxy = Null
	if p.Id() != 0 || p.Context() != nil {
		t.Errorf("null proxy: id %d", p.Id())
	}
	p.SetId(7)
	p.SetContext(nil)
	p.Unregister()
	if p.Id() != 0 {
		t.Error("the null proxy took an id")
	}
}
