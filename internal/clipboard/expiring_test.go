package clipboard

import (
	"strings"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/headlesstest"
	"github.com/stubbedev/lys-ui/transfer"
)

// fakeSource is a data source whose requests go nowhere: a context
// without a connection fails them instead of writing.
func fakeSource() *wl.DataSource {
	s := &wl.DataSource{}
	s.SetContext(&wl.Context{})
	return s
}

// An expiring claim clears when its time comes if it still holds the
// selection, and leaves a claim that replaced it alone (#276).
func TestExpiringClaim(t *testing.T) {
	secret := transfer.Merge(transfer.Text("hunter2"), transfer.Payloads([]string{"x-kde-passwordManagerHint"}, map[string][]byte{"x-kde-passwordManagerHint": []byte("secret")}))
	c := &Clipboard{}
	held := fakeSource()
	c.source, c.content, c.serial = held, secret, 7
	c.expire(held)
	if c.source != nil || !c.content.Empty() {
		t.Error("the expired claim still holds the selection")
	}

	stale := fakeSource()
	later := fakeSource()
	c.source, c.content = later, transfer.Text("an unrelated later copy")
	c.expire(stale)
	if c.source != later {
		t.Error("expiring a replaced claim dropped the claim that replaced it")
	}
}

// The expiry's set_selection reaches the wire as the null source and
// the claim's serial. The generated SetSelection(nil, serial) panicked
// in the encoder over a real connection, with its send lock held, and
// froze the client (#277).
func TestClearSelectionSendsTheNullSource(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-clipboard")
	disp, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disp.Context().Close() })
	dev := &wl.DataDevice{}
	disp.Context().Register(dev)
	// No compositor answers here, so drop the id from the unsent
	// bookkeeping a later new-id request would wait on.
	dev.Unregister()
	if err := clearSelection(dev, 42); err != nil {
		t.Fatal(err)
	}
	f := headlesstest.WaitForFrame(t, frames, "set_selection", func(f headlesstest.WireFrame) bool {
		return f.Obj == uint32(dev.Id()) && f.Opcode == dataDeviceSetSelection
	})
	if len(f.Body) != 8 || f.Word(0) != 0 || f.Word(1) != 42 {
		t.Errorf("set_selection payload %v, want the null source and serial 42", f.Body)
	}
}

// Without a scheduler WriteExpiring fails loudly instead of leaving a
// secret on the clipboard for good.
func TestWriteExpiringNeedsAScheduler(t *testing.T) {
	err := (&Clipboard{}).WriteExpiring(transfer.Text("hunter2"), time.Second)
	if err == nil || !strings.Contains(err.Error(), "scheduler") {
		t.Errorf("WriteExpiring without a scheduler = %v, want the scheduler error", err)
	}
}
