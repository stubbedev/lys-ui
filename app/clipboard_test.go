package app

import (
	"errors"
	"image"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/clipboard"
	"github.com/stubbedev/lys-ui/internal/dragdrop"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/internal/xfer"
	"github.com/stubbedev/lys-ui/widget"
)

// NewClipboard is the public way in for consumers outside the module:
// the app hands it back from Clipboard once installed, and a session
// without a data device reports the clipboard unavailable instead of
// crashing on a programmatic copy.
func TestClipboardInstalledAndUnavailable(t *testing.T) {
	a := NewApplication(&wlsession.Session{})
	if a.Clipboard() != nil {
		t.Fatal("a fresh application has a clipboard")
	}
	c := NewClipboard(&wlsession.Session{})
	a.SetClipboard(c)
	if a.Clipboard() != c {
		t.Fatal("Clipboard does not return the installed clipboard")
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := a.Clipboard().WriteImage(img); !errors.Is(err, ErrClipboardUnavailable) {
		t.Fatalf("WriteImage without a data device = %v, want ErrUnavailable", err)
	}
}

// A failed transfer reaches the handler; nothing to paste does not,
// and an empty clipboard pastes nothing quietly on either paste path.
func TestTransferErrorHandler(t *testing.T) {
	a := accelApp()
	var got []error
	a.SetTransferErrorHandler(func(err error) { got = append(got, err) })
	a.reportTransfer(nil)
	a.reportTransfer(ErrClipboardUnavailable)
	a.reportTransfer(dragdrop.ErrNoPayload)
	a.reportTransfer(xfer.ErrTooLarge)
	if len(got) != 1 || !errors.Is(got[0], xfer.ErrTooLarge) {
		t.Fatalf("reported %v, want the oversize transfer alone", got)
	}

	clip := &clipboard.Clipboard{}
	face := testFace(t)
	entry := widget.NewEntry(face, 14, render.RGB(0, 0, 0))
	r := &widget.Router{Root: entry}
	r.SetFocus(entry)
	if err := pasteSelection(r, clip); err != nil {
		t.Errorf("text paste from an empty clipboard: %v", err)
	}
	img := widget.NewImage(nil)
	img.OnPasteImage = func(image.Image) {}
	r = &widget.Router{Root: img}
	r.SetFocus(img)
	if err := pasteSelection(r, clip); err != nil {
		t.Errorf("image paste from an empty clipboard: %v", err)
	}

	in := &surfaceInput{transferError: a.reportTransfer}
	in.reportTransfer(xfer.ErrTimeout)
	if len(got) != 2 {
		t.Errorf("surface input did not report: %v", got)
	}
}
