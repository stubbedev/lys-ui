package clipboard

import (
	"errors"
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/transfer"
)

func TestPrimaryUnavailableWithoutOffer(t *testing.T) {
	// No protocol device, no offer: reading the primary selection is
	// ErrUnavailable, the same contract ReadText has.
	c := &Clipboard{sess: &wlsession.Session{}}
	if _, err := c.ReadPrimary(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ReadPrimary = %v, want ErrUnavailable", err)
	}
	// Mimes without an offer read as unavailable.
	c.primaryMimes = map[string]bool{"text/plain": true}
	if _, err := c.ReadPrimary(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ReadPrimary without an offer = %v, want ErrUnavailable", err)
	}
	// An offer with no text mime reads as unavailable.
	prim := &wlr.ZwpPrimarySelectionOfferV1{} // opaque identity; never driven on the wire here
	c.primary = prim
	c.primaryMimes = map[string]bool{"application/x-lys-ui-tile": true}
	if _, err := c.ReadPrimary(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ReadPrimary without a text mime = %v, want ErrUnavailable", err)
	}
}

// Regression: the primary selection's offers and the wl_data_device
// selection's offers must never promote each other or mix mime sets —
// they are independent selections over independent protocol objects,
// exactly like the drag offers in TestSelectionSurvivesDragOffers.
func TestPrimaryOfferTrackingIsolatesSelections(t *testing.T) {
	c := &Clipboard{
		sess:          &wlsession.Session{},
		offers:        make(map[*wl.DataOffer]*offerMimes),
		primaryOffers: make(map[*wlr.ZwpPrimarySelectionOfferV1]*offerMimes),
	}

	// The primary offer appears and advertises text, then the device
	// names it the primary selection.
	prim := &wlr.ZwpPrimarySelectionOfferV1{}
	primMimes := &offerMimes{mimes: make(map[string]bool)}
	c.primaryOffers[prim] = primMimes
	primMimes.HandleZwpPrimarySelectionOfferV1Offer(wlr.ZwpPrimarySelectionOfferV1OfferEvent{MimeType: "text/plain;charset=utf-8"})
	c.HandleZwpPrimarySelectionDeviceV1Selection(wlr.ZwpPrimarySelectionDeviceV1SelectionEvent{Id: prim})

	if c.primary != prim {
		t.Errorf("primary = %v, want the primary offer", c.primary)
	}
	if got := pickTextMime(func(m string) bool { return c.primaryMimes[m] }); got != "text/plain;charset=utf-8" {
		t.Errorf("primary mime = %q, want the utf-8 text type", got)
	}

	// An unrelated wl_data_device offer (a drag passing over the
	// window) advertises a custom type and gets promoted as the
	// regular selection. The primary must not hear about it.
	sel := &wl.DataOffer{}
	selMimes := &offerMimes{mimes: make(map[string]bool)}
	c.offers[sel] = selMimes
	selMimes.HandleDataOfferOffer(wl.DataOfferOfferEvent{MimeType: "application/x-lys-ui-tile"})
	c.HandleDataDeviceSelection(wl.DataDeviceSelectionEvent{Id: sel})
	if len(c.primaryMimes) != 1 || !c.primaryMimes["text/plain;charset=utf-8"] {
		t.Errorf("primary mimes = %v, want only the utf-8 text type", c.primaryMimes)
	}
	if c.selectionMimes["text/plain;charset=utf-8"] {
		t.Error("the primary offer's mime leaked into the regular selection")
	}

	// A nil primary clears only the primary state.
	c.HandleZwpPrimarySelectionDeviceV1Selection(wlr.ZwpPrimarySelectionDeviceV1SelectionEvent{})
	if c.primary != nil || c.primaryMimes != nil {
		t.Errorf("nil primary left state: %v %v", c.primary, c.primaryMimes)
	}
	if c.selection != sel {
		t.Errorf("nil primary cleared the regular selection: %v", c.selection)
	}
	if _, err := c.ReadPrimary(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ReadPrimary = %v, want ErrUnavailable after clearing", err)
	}
}

func TestPrimarySendHandlerWritesAndCloses(t *testing.T) {
	// The primary source's send event carries the primary payload,
	// and only it: the regular selection's payload stays untouched.
	c := &Clipboard{content: transfer.Text("clipboard payload"), primaryContent: transfer.Text("primary payload")}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{MimeType: "text/plain;charset=utf-8", Fd: w.Fd()})
	w.Close()

	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "primary payload" {
		t.Errorf("received %q, want the primary payload", string(buf[:n]))
	}
	// The handler must have closed its fd: a second read gives EOF.
	if _, err := r.Read(buf); err == nil {
		t.Error("fd still open after primary Send")
	}
}

func TestPrimarySendHandlerIgnoresBrokenFds(t *testing.T) {
	c := &Clipboard{primaryContent: transfer.Text("primary payload")}
	// A failed descriptor dup arrives as FdError or a zero fd; the
	// handler must drop both without writing anywhere.
	c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{FdError: errors.New("dup failed")})
	c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{})
}

func TestSelectionPayloadsStayIndependent(t *testing.T) {
	// The regular and primary sources are alive at once whenever both
	// selections are claimed; each consumer must read the payload of
	// the selection it asked for.
	c := &Clipboard{content: transfer.Text("clipboard payload"), primaryContent: transfer.Text("primary payload")}

	cr, cw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: transfer.MimeText, Fd: cw.Fd()})
	cw.Close()

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c.HandleZwpPrimarySelectionSourceV1Send(wlr.ZwpPrimarySelectionSourceV1SendEvent{MimeType: transfer.MimeText, Fd: pw.Fd()})
	pw.Close()

	want := map[string]string{"clipboard": "clipboard payload", "primary": "primary payload"}
	for name, rd := range map[string]*os.File{"clipboard": cr, "primary": pr} {
		buf := make([]byte, 64)
		n, _ := rd.Read(buf)
		if string(buf[:n]) != want[name] {
			t.Errorf("%s consumer received %q, want %q", name, string(buf[:n]), want[name])
		}
		rd.Close()
	}
}

func TestWritePrimaryWithoutProtocol(t *testing.T) {
	// A compositor without the primary selection manager: claiming
	// the primary is ErrUnavailable, the same contract as WriteText
	// without a data device.
	c := &Clipboard{sess: &wlsession.Session{}}
	if err := c.WritePrimary("text", 1); !errors.Is(err, ErrUnavailable) {
		t.Errorf("WritePrimary = %v, want ErrUnavailable", err)
	}
}
