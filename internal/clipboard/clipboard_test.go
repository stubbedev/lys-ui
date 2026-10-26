package clipboard

import (
	"errors"
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/transfer"
)

func TestPickTextMime(t *testing.T) {
	t.Run("prefers utf-8 plain text", func(t *testing.T) {
		got := pickTextMime(func(m string) bool {
			return m == "text/plain" || m == "text/plain;charset=utf-8"
		})
		if got != "text/plain;charset=utf-8" {
			t.Errorf("mime = %q, want the utf-8 variant", got)
		}
	})

	t.Run("falls back to plain text", func(t *testing.T) {
		got := pickTextMime(func(m string) bool { return m == "text/plain" })
		if got != "text/plain" {
			t.Errorf("mime = %q, want text/plain", got)
		}
	})

	t.Run("none of the text mimes gives empty", func(t *testing.T) {
		if got := pickTextMime(func(string) bool { return false }); got != "" {
			t.Errorf("mime = %q, want empty", got)
		}
	})
}

func TestClipboardUnavailableWithoutOffer(t *testing.T) {
	c := &Clipboard{sess: &wlsession.Session{}}
	if _, err := c.ReadText(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ReadText = %v, want ErrUnavailable", err)
	}
}

// Regression: offers from an unrelated transfer (a drag passing over
// the window, internal/dragdrop) used to clobber the selection's mime
// set, because one shared tracker collected every offer's mimes. Each
// offer now carries its own tracker and only a selection event
// promotes one, so the selection must survive a drag's offer
// advertising a non-text mime.
func TestSelectionSurvivesDragOffers(t *testing.T) {
	c := &Clipboard{sess: &wlsession.Session{}, offers: make(map[*wl.DataOffer]*offerMimes)}
	sel := &wl.DataOffer{} // opaque identity; never driven on the wire here
	selectionMimes := &offerMimes{mimes: make(map[string]bool)}
	selectionMimes.HandleDataOfferOffer(wl.DataOfferOfferEvent{MimeType: "text/plain;charset=utf-8"})
	c.offers[sel] = selectionMimes
	c.HandleDataDeviceSelection(wl.DataDeviceSelectionEvent{Id: sel})

	// A drag offer appears and advertises a custom type.
	drag := &wl.DataOffer{}
	dragMimes := &offerMimes{mimes: make(map[string]bool)}
	c.offers[drag] = dragMimes
	dragMimes.HandleDataOfferOffer(wl.DataOfferOfferEvent{MimeType: "application/x-lys-ui-tile"})

	// The selection is still the text one, with its own mimes intact.
	if c.selection != sel {
		t.Errorf("selection = %v, want the selection offer", c.selection)
	}
	if got := pickTextMime(func(m string) bool { return c.selectionMimes[m] }); got != "text/plain;charset=utf-8" {
		t.Errorf("selection mime = %q, want the utf-8 text type", got)
	}
	if c.selectionMimes["application/x-lys-ui-tile"] {
		t.Error("the drag offer's mime leaked into the selection")
	}

	// A nil selection clears the state.
	c.HandleDataDeviceSelection(wl.DataDeviceSelectionEvent{})
	if c.selection != nil || c.selectionMimes != nil {
		t.Errorf("nil selection left state: %v %v", c.selection, c.selectionMimes)
	}
	if _, err := c.ReadText(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ReadText = %v, want ErrUnavailable after clearing", err)
	}
}

func TestSendHandlerWritesAndCloses(t *testing.T) {
	c := &Clipboard{content: transfer.Text("clipboard payload")}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: "text/plain;charset=utf-8", Fd: w.Fd(), FdError: nil})
	w.Close()

	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "clipboard payload" {
		t.Errorf("received %q, want the clipboard payload", string(buf[:n]))
	}
	// The handler must have closed its fd: a second read gives EOF.
	if _, err := r.Read(buf); err == nil {
		t.Error("fd still open after Send")
	}
}
