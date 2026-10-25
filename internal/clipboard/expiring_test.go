package clipboard

import (
	"strings"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"

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

// Without a scheduler WriteExpiring fails loudly instead of leaving a
// secret on the clipboard for good.
func TestWriteExpiringNeedsAScheduler(t *testing.T) {
	err := (&Clipboard{}).WriteExpiring(transfer.Text("hunter2"), time.Second)
	if err == nil || !strings.Contains(err.Error(), "scheduler") {
		t.Errorf("WriteExpiring without a scheduler = %v, want the scheduler error", err)
	}
}
