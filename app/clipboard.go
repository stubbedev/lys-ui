package app

import (
	"github.com/stubbedev/lys-ui/internal/clipboard"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// Clipboard is the seat's regular and primary selections: ReadText and
// WriteText for ctrl+c/v content, ReadPrimary and WritePrimary for the
// middle-click selection, ReadImageBytes and WriteImage for pictures,
// and Write and Read for any transfer.Content (uri lists, HTML).
// It is the alias external consumers name the clipboard by; hand it to
// Application.SetClipboard (or Config.Clipboard) to wire the built-in
// copy and paste keys, and keep it to put content on the clipboard from
// application code - a launcher copying a calculator result, say. Reads
// take a callback: a peer may never answer, so the payload arrives on
// the loop goroutine when the transfer ends.
type Clipboard = clipboard.Clipboard

// ErrClipboardUnavailable reports that the clipboard holds no content
// in a usable type, or that the compositor offers no data device.
var ErrClipboardUnavailable = clipboard.ErrUnavailable

// NewClipboard wires a clipboard to the session's data device (and its
// primary-selection device where the compositor has one). A compositor
// without either still yields a Clipboard; its reads and writes then
// fail with ErrClipboardUnavailable instead of panicking.
func NewClipboard(sess *wlsession.Session) *Clipboard {
	return clipboard.New(sess)
}
