// Package clipboard implements text copy and paste over the core
// wl_data_device protocol and, when the compositor offers it, the
// zwp_primary_selection_unstable_v1 protocol: the offer pipeline for
// reading a selection, and data sources for claiming either. The other
// end of every transfer is a foreign client, so reads and writes are
// bounded in size and time through internal/xfer — a hostile peer can
// cost an error on a paste, never unbounded memory or a hung loop.
package clipboard

import (
	"errors"
	"fmt"
	"image"
	"os"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/wlnull"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/internal/xfer"
	"github.com/stubbedev/lys-ui/transfer"
)

// ErrUnavailable reports that there is no selection or none in a
// format we can read.
var ErrUnavailable = errors.New("clipboard: no selection available")

// transferTimeout bounds one offer read or source write against a
// peer that stalls (a var so tests can shorten it): a silent peer is
// cut off with an error instead of hanging a paste, and a consumer
// that stops reading cannot freeze the dispatch loop.
var transferTimeout = xfer.DefaultTimeout

// Clipboard tracks the seat's selections and claims them on behalf of
// the app. Create one after connecting; it is not safe for concurrent
// use.
//
// The regular selection (ctrl+c/x/v) travels over wl_data_device. The
// primary selection (the X11-style PRIMARY that a middle click pastes)
// travels over zwp_primary_selection_unstable_v1 when the compositor
// advertises it; without it every primary call reports
// ErrUnavailable. The two selections stay independent — their offer
// mime sets and source payloads never mix.
//
// The data device also carries drag-and-drop (internal/dragdrop),
// whose offers advertise mime types through the same wl_data_offer
// objects. Each offer therefore tracks its own mime set (offerMimes),
// and only an offer the compositor promotes with a selection event can
// ever satisfy ReadText: a drag passing through never touches the
// selection.
type Clipboard struct {
	sess   *wlsession.Session
	offers map[*wl.DataOffer]*offerMimes
	// selection is the offer the compositor named as the selection;
	// selectionMimes its advertised set.
	selection      *wl.DataOffer
	selectionMimes map[string]bool
	// source is our claim on the selection, content what it serves and
	// serial the input serial it claimed with. Exactly one claim lives
	// at a time: the last write wins.
	source  *wl.DataSource
	content transfer.Content
	serial  uint32
	// after runs fn on the event loop once d passed (SetScheduler);
	// WriteExpiring needs it.
	after func(d time.Duration, fn func())

	// The primary selection mirrors the above over its own protocol
	// objects; separate mimes map and payload, per-selection state.
	primaryOffers map[*wlr.ZwpPrimarySelectionOfferV1]*offerMimes
	primary       *wlr.ZwpPrimarySelectionOfferV1
	primaryMimes  map[string]bool
	// primarySource is the primary claim, primaryContent its payload;
	// copy-on-select keeps it text.
	primarySource  *wlr.ZwpPrimarySelectionSourceV1
	primaryContent transfer.Content
}

// offerMimes collects the mime types one advertised offer carries.
// The bindings dispatch offer events without naming their object, so
// each offer gets its own listener.
type offerMimes struct {
	mimes map[string]bool
}

// HandleDataOfferOffer implements wl.DataOfferOfferHandler: one
// advertised mime type of this wl_data_offer.
func (o *offerMimes) HandleDataOfferOffer(ev wl.DataOfferOfferEvent) {
	o.mimes[ev.MimeType] = true
}

// HandleZwpPrimarySelectionOfferV1Offer implements
// wlr.ZwpPrimarySelectionOfferV1OfferHandler: one advertised mime type
// of this primary selection offer.
func (o *offerMimes) HandleZwpPrimarySelectionOfferV1Offer(ev wlr.ZwpPrimarySelectionOfferV1OfferEvent) {
	o.mimes[ev.MimeType] = true
}

// New wires the clipboard to the session's data device — and to the
// primary selection device when the compositor has one — and starts
// tracking selection offers.
func New(sess *wlsession.Session) *Clipboard {
	c := &Clipboard{}
	c.Reset(sess)
	return c
}

// Reset attaches the clipboard to sess, forgetting everything the
// previous session held - its offers, the selections, our claims: a
// compositor restart takes the clipboard's contents with it, so after
// a reconnect the clipboard starts empty on the new session.
func (c *Clipboard) Reset(sess *wlsession.Session) {
	*c = Clipboard{
		sess:          sess,
		offers:        make(map[*wl.DataOffer]*offerMimes),
		primaryOffers: make(map[*wlr.ZwpPrimarySelectionOfferV1]*offerMimes),
	}
	if dev := sess.DataDevice(); dev != nil {
		dev.AddDataOfferHandler(c)
		dev.AddSelectionHandler(c)
	}
	if dev := sess.PrimarySelectionDevice(); dev != nil {
		dev.AddDataOfferHandler(c)
		dev.AddSelectionHandler(c)
	}
}

// HandleDataDeviceDataOffer implements wl.DataDeviceDataOfferHandler:
// a new offer appears; start listening for its mime types. Selection
// and drag offers both pass here; only a later selection event makes
// one the clipboard's.
func (c *Clipboard) HandleDataDeviceDataOffer(ev wl.DataDeviceDataOfferEvent) {
	if ev.Id == nil {
		return
	}
	m := &offerMimes{mimes: make(map[string]bool)}
	c.offers[ev.Id] = m
	ev.Id.AddOfferHandler(m)
}

// HandleDataDeviceSelection implements wl.DataDeviceSelectionHandler:
// the offer is now the selection; a nil offer clears it.
func (c *Clipboard) HandleDataDeviceSelection(ev wl.DataDeviceSelectionEvent) {
	c.selection = ev.Id
	if m := c.offers[ev.Id]; m != nil {
		c.selectionMimes = m.mimes
	} else {
		c.selectionMimes = nil
	}
}

// HandleZwpPrimarySelectionDeviceV1DataOffer implements
// wlr.ZwpPrimarySelectionDeviceV1DataOfferHandler: a new primary
// selection offer appears; start listening for its mime types.
func (c *Clipboard) HandleZwpPrimarySelectionDeviceV1DataOffer(ev wlr.ZwpPrimarySelectionDeviceV1DataOfferEvent) {
	if ev.Offer == nil {
		return
	}
	m := &offerMimes{mimes: make(map[string]bool)}
	c.primaryOffers[ev.Offer] = m
	ev.Offer.AddOfferHandler(m)
}

// HandleZwpPrimarySelectionDeviceV1Selection implements
// wlr.ZwpPrimarySelectionDeviceV1SelectionHandler: the offer is now
// the primary selection; a nil offer clears it.
func (c *Clipboard) HandleZwpPrimarySelectionDeviceV1Selection(ev wlr.ZwpPrimarySelectionDeviceV1SelectionEvent) {
	c.primary = ev.Id
	if m := c.primaryOffers[ev.Id]; m != nil {
		c.primaryMimes = m.mimes
	} else {
		c.primaryMimes = nil
	}
}

// readOffer drains a selection offer through a pipe, as the best of
// prefs the offer carries: receive starts the transfer on the write
// end, flush is a roundtrip so the compositor dups the fd, and
// dropping our write end yields EOF at the sender's last byte. The
// transfer itself is bounded against hostile peers by xfer: capped at
// MaxPayload and cut off by the deadline, with the fd drained and
// closed either way so the peer's pipe never blocks forever.
func readOffer(receive func(string, uintptr) error, flush func() error, mimes map[string]bool, prefs []string) ([]byte, string, error) {
	mime := transfer.Pick(prefs, func(m string) bool { return mimes[m] })
	if mime == "" {
		return nil, "", ErrUnavailable
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, "", fmt.Errorf("clipboard: pipe: %w", err)
	}
	defer r.Close()
	if err := receive(mime, w.Fd()); err != nil {
		return nil, "", fmt.Errorf("clipboard: receive: %w", err)
	}
	// Flush the request so the compositor dups the fd before we drop
	// our write end; dropping it is what eventually yields EOF. The
	// close error carries no signal we could act on.
	if err := flush(); err != nil {
		return nil, "", fmt.Errorf("clipboard: flush: %w", err)
	}
	_ = w.Close()

	data, err := xfer.Read(r, xfer.MaxPayload, transferTimeout)
	if err != nil {
		// An oversize or stalled peer lands here: the payload is
		// refused whole, and the fd has been drained and closed, so
		// the peer is not left blocked on the pipe.
		return nil, "", fmt.Errorf("clipboard: transfer: %w", err)
	}
	return data, mime, nil
}

// Read returns the selection's payload as the best of prefs it offers,
// with that mime, blocking until the compositor delivers it. Errors
// with ErrUnavailable when there is no selection or none of prefs.
// The transfer is bounded against hostile peers: a selection past
// xfer.MaxPayload is refused (the fd drained so the offering peer
// never blocks) and a peer that stalls is cut off after the transfer
// deadline - both surface here as errors, so paste callers react to a
// rejected paste instead of an OOM or a hung window.
//
// While this client holds the selection, the payload comes from the
// claim directly: the wire path would make the loop goroutine both the
// pipe's reader and, in its send handler, its writer, which stalls
// until the transfer deadline wherever the compositor delivers the
// send after the flush roundtrip (Hyprland), and on any payload larger
// than the pipe buffer.
func (c *Clipboard) Read(prefs ...string) ([]byte, string, error) {
	if c.source != nil {
		return readClaim(c.content, prefs)
	}
	if c.selection == nil {
		return nil, "", ErrUnavailable
	}
	return readOffer(c.selection.Receive, c.sess.Roundtrip, c.selectionMimes, prefs)
}

// readClaim serves a read from this client's own claim: the best of
// prefs the content offers, written in process.
func readClaim(content transfer.Content, prefs []string) ([]byte, string, error) {
	mime := transfer.Pick(prefs, content.Has)
	if mime == "" {
		return nil, "", ErrUnavailable
	}
	data, err := content.Bytes(mime)
	if err != nil {
		return nil, "", fmt.Errorf("clipboard: read own %s claim: %w", mime, err)
	}
	return data, mime, nil
}

// Offers reports whether the selection offers mime - what a paste
// target checks before choosing how to read.
func (c *Clipboard) Offers(mime string) bool {
	if c.source != nil {
		return c.content.Has(mime)
	}
	return c.selection != nil && c.selectionMimes[mime]
}

// ReadText returns the selection as text.
func (c *Clipboard) ReadText() (string, error) {
	data, _, err := c.Read(transfer.TextMimes...)
	return string(data), err
}

// ReadImageBytes returns the selection's image payload - the best of
// transfer.ImageMimes the offer carries - as encoded bytes with its
// mime type. Decoding is the caller's: the widget pipeline decodes off
// the loop goroutine.
func (c *Clipboard) ReadImageBytes() ([]byte, string, error) {
	return c.Read(transfer.ImageMimes...)
}

// ReadURIs returns the selection's uri list - files a file manager
// copied, say; transfer.FilePaths keeps the local ones.
func (c *Clipboard) ReadURIs() ([]string, error) {
	data, _, err := c.Read(transfer.MimeURIList)
	return transfer.ParseURIList(data), err
}

// ReadHTML returns the selection's formatted text as HTML.
func (c *Clipboard) ReadHTML() (string, error) {
	data, _, err := c.Read(transfer.MimeHTML)
	return transfer.DecodeHTML(data), err
}

// ReadPrimary returns the current primary selection as text. Errors
// with ErrUnavailable when the protocol is missing or there is nothing
// to read; oversize and stalled peers surface as errors exactly as in
// Read - the two selections share one hardened transfer - and, as in
// Read, this client's own claim is served in process.
func (c *Clipboard) ReadPrimary() (string, error) {
	if c.primarySource != nil {
		data, _, err := readClaim(c.primaryContent, transfer.TextMimes)
		return string(data), err
	}
	if c.primary == nil {
		return "", ErrUnavailable
	}
	data, _, err := readOffer(c.primary.Receive, c.sess.Roundtrip, c.primaryMimes, transfer.TextMimes)
	return string(data), err
}

// Write claims the selection with content, served to every consumer
// that asks until another client or app replaces the selection. The
// transfer builders (transfer.Text, Image, Files, HTML, Merge) encode
// up front, so serving never encodes under a peer's deadline.
// The claim carries the serial of the latest input event (the key or
// click that copied); before any it fails with
// wlsession.ErrNoInputEvent.
func (c *Clipboard) Write(content transfer.Content) error {
	_, err := c.claim(content)
	return err
}

// SetScheduler installs how the clipboard defers work to its event
// loop: after runs fn on the loop goroutine once d passed. An
// Application installs its own (Application.SetClipboard); WriteExpiring
// needs one.
func (c *Clipboard) SetScheduler(after func(d time.Duration, fn func())) { c.after = after }

// WriteExpiring is Write with a lifetime, for a secret a password
// manager copies: after ttl, if this claim still holds the selection -
// no other client and no later write replaced it - the selection
// clears (set_selection with no source, on the claim's serial). A
// replaced claim is left alone, so a later, unrelated copy survives.
func (c *Clipboard) WriteExpiring(content transfer.Content, ttl time.Duration) error {
	if c.after == nil {
		return errors.New("clipboard: WriteExpiring needs a scheduler (Application.SetClipboard)")
	}
	source, err := c.claim(content)
	if err != nil {
		return err
	}
	c.after(ttl, func() { c.expire(source) })
	return nil
}

// expire clears the selection if source is still our claim.
func (c *Clipboard) expire(source *wl.DataSource) {
	if c.source != source || source == nil {
		return
	}
	serial := c.serial
	c.dropClaim()
	if c.sess == nil {
		return
	}
	if dev := c.sess.DataDevice(); dev != nil {
		if err := clearSelection(dev, serial); err != nil {
			debug.Log("input", "clipboard expire: %v", err)
		}
	}
}

// dataDeviceSetSelection is the wl_data_device.set_selection opcode.
const dataDeviceSetSelection = 1

// clearSelection is wl_data_device.set_selection with the null source,
// which the generated SetSelection cannot send: a nil *wl.DataSource
// panics in the wire encoder with the connection's send lock held.
func clearSelection(dev *wl.DataDevice, serial uint32) error {
	return dev.Context().SendRequest(dev, dataDeviceSetSelection, wlnull.Null, serial)
}

// claim is Write: it claims the selection with content and returns the
// claim.
func (c *Clipboard) claim(content transfer.Content) (*wl.DataSource, error) {
	if content.Empty() {
		return nil, errors.New("clipboard: empty content")
	}
	if c.sess == nil {
		return nil, ErrUnavailable
	}
	mgr := c.sess.DataDeviceManager()
	dev := c.sess.DataDevice()
	if mgr == nil || dev == nil {
		return nil, ErrUnavailable
	}
	serial := c.sess.InputSerial()
	if serial == 0 {
		return nil, fmt.Errorf("clipboard: set selection: %w", wlsession.ErrNoInputEvent)
	}
	source, err := mgr.CreateDataSource()
	if err != nil {
		return nil, fmt.Errorf("clipboard: create source: %w", err)
	}
	for _, m := range content.Mimes {
		if err := source.Offer(m); err != nil {
			return nil, fmt.Errorf("clipboard: offer: %w", err)
		}
	}
	c.dropClaim()
	c.source, c.content, c.serial = source, content, serial
	source.AddSendHandler(c)
	source.AddCancelledHandler(c)
	if err := dev.SetSelection(source, serial); err != nil {
		return nil, fmt.Errorf("clipboard: set selection: %w", err)
	}
	return source, nil
}

// WriteText claims the selection with s as text.
func (c *Clipboard) WriteText(s string) error { return c.Write(transfer.Text(s)) }

// WriteImage claims the selection with img, as PNG and JPEG encoded
// here on the caller's goroutine. The primary selection is untouched:
// copy-on-select keeps its text semantics.
func (c *Clipboard) WriteImage(img image.Image) error {
	content, err := transfer.Image(img)
	if err != nil {
		return err
	}
	return c.Write(content)
}

// WritePrimary claims the primary selection with s as its text
// content. serial is the serial of the input event that triggered the
// claim - the protocol asks for the triggering press, so callers pass
// the pointer (or keyboard) serial they received. The source stays
// alive until another client or app replaces the selection.
func (c *Clipboard) WritePrimary(s string, serial uint32) error {
	mgr := c.sess.PrimarySelectionManager()
	dev := c.sess.PrimarySelectionDevice()
	if mgr == nil || dev == nil {
		return ErrUnavailable
	}
	content := transfer.Text(s)
	source, err := mgr.CreateSource()
	if err != nil {
		return fmt.Errorf("clipboard: create primary source: %w", err)
	}
	for _, m := range content.Mimes {
		if err := source.Offer(m); err != nil {
			return fmt.Errorf("clipboard: offer: %w", err)
		}
	}
	c.primarySource, c.primaryContent = source, content
	source.AddSendHandler(c)
	source.AddCancelledHandler(c)
	if err := dev.SetSelection(source, serial); err != nil {
		return fmt.Errorf("clipboard: set primary selection: %w", err)
	}
	return nil
}

// dropClaim tears down our claim on the regular selection, if any.
func (c *Clipboard) dropClaim() {
	if c.source != nil {
		_ = c.source.Destroy()
	}
	c.source, c.content, c.serial = nil, transfer.Content{}, 0
}

// sendPayload writes content's payload for mime to a consumer's fd and
// closes it, so the reader sees EOF. The write is bounded by the
// deadline - a consumer that never reads would otherwise stall the
// dispatch loop on a full pipe - and EPIPE from one that closed early
// ends it at once. A failed write leaves the consumer a short payload
// plus EOF: the transfer visibly broke instead of hanging. A mime the
// content does not offer writes nothing.
func sendPayload(content transfer.Content, mime string, fd uintptr) {
	w, err := xfer.DeadlineWriter(fd, transferTimeout)
	if err != nil {
		// A zero fd (the binding failed to dup the descriptor) lands
		// here too; both are routine peer-side breakage.
		debug.Log("input", "clipboard send: %v", err)
		return
	}
	if content.Has(mime) {
		if err := content.Write(mime, w); err != nil {
			debug.Log("input", "clipboard send: %v", err)
		}
	}
	_ = w.Close()
}

// HandleDataSourceSend implements wl.DataSourceSendHandler: a consumer
// asked for the regular selection's data.
func (c *Clipboard) HandleDataSourceSend(ev wl.DataSourceSendEvent) {
	if ev.FdError != nil {
		return
	}
	sendPayload(c.content, ev.MimeType, ev.Fd)
}

// HandleDataSourceCancelled implements wl.DataSourceCancelledHandler:
// another claim replaced ours.
func (c *Clipboard) HandleDataSourceCancelled(wl.DataSourceCancelledEvent) { c.dropClaim() }

// HandleZwpPrimarySelectionSourceV1Send implements
// wlr.ZwpPrimarySelectionSourceV1SendHandler: a consumer asked for the
// primary selection's data.
func (c *Clipboard) HandleZwpPrimarySelectionSourceV1Send(ev wlr.ZwpPrimarySelectionSourceV1SendEvent) {
	if ev.FdError != nil {
		return
	}
	sendPayload(c.primaryContent, ev.MimeType, ev.Fd)
}

// HandleZwpPrimarySelectionSourceV1Cancelled implements
// wlr.ZwpPrimarySelectionSourceV1CancelledHandler.
func (c *Clipboard) HandleZwpPrimarySelectionSourceV1Cancelled(wlr.ZwpPrimarySelectionSourceV1CancelledEvent) {
	if c.primarySource != nil {
		_ = c.primarySource.Destroy()
		c.primarySource = nil
	}
	c.primaryContent = transfer.Content{}
}
