package clipboard

import (
	"errors"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/transfer"
)

// TestOwnClaimIsReadInProcess pins the self-paste path: while this
// client holds a selection, reading it never touches the wire. The
// Clipboard has no session, so a wire read would panic; a payload far
// past the pipe buffer would deadlock the loop if it went through one.
func TestOwnClaimIsReadInProcess(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	c := testClipboard(t)
	c.source, c.content = &wl.DataSource{}, transfer.Text(big)
	c.primarySource, c.primaryContent = &wlr.ZwpPrimarySelectionSourceV1{}, transfer.Text("primary payload")

	text, err := collectText(t, c.ReadText)
	if err != nil || text != big {
		t.Fatalf("ReadText of our own claim = %d bytes, %v; want the %d-byte claim", len(text), err, len(big))
	}
	if !c.Offers(transfer.MimeText) {
		t.Error("Offers does not report our own claim's text")
	}
	if c.Offers("image/png") {
		t.Error("Offers reports a mime our claim does not carry")
	}
	if r := collectRead(t, func(done func([]byte, string, error)) { c.Read([]string{"image/png"}, done) }); !errors.Is(r.err, ErrUnavailable) {
		t.Errorf("Read of a mime our claim lacks = %v, want ErrUnavailable", r.err)
	}
	if got, err := collectText(t, c.ReadPrimary); err != nil || got != "primary payload" {
		t.Errorf("ReadPrimary of our own claim = %q, %v", got, err)
	}
}
