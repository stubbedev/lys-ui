// Package datacontrol is the clipboard-manager side of the seat's
// selections, over ext_data_control_v1 or, on compositors that
// predate it, zwlr_data_control_unstable_v1. A data-control device
// needs no surface and no keyboard focus: it sees every selection
// change on the seat, can read any offer under any advertised mime
// type, and can claim either selection with its own data source.
//
// Everything here runs on the loop goroutine, like every Wayland
// event handler. The two slow parts are moved off it: reading an
// offer drains its pipe on a goroutine and delivers the bytes back
// through the application's Invoke, and serving a claim writes each
// payload on a goroutine. Both ends of every transfer are foreign
// clients, so both are bounded through internal/xfer: a read in size
// and time, a write in time.
package datacontrol

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/xfer"
)

// Selection names one of the seat's two selections.
type Selection uint8

const (
	// Clipboard is the regular selection: ctrl+c, ctrl+v.
	Clipboard Selection = iota
	// Primary is the X11-style primary selection a middle click
	// pastes.
	Primary
)

// selections is the number of Selection values; per-selection state
// is indexed by them.
const selections = 2

// String names the selection.
func (s Selection) String() string {
	switch s {
	case Clipboard:
		return "clipboard"
	case Primary:
		return "primary"
	}
	return fmt.Sprintf("Selection(%d)", uint8(s))
}

// valid reports whether s is one of the declared selections.
func (s Selection) valid() bool { return s < selections }

// Protocol names the data-control protocol a device speaks.
type Protocol uint8

const (
	// ProtocolExt is the staging ext-data-control-v1.
	ProtocolExt Protocol = iota + 1
	// ProtocolWlr is wlr-data-control-unstable-v1.
	ProtocolWlr
)

// String names the protocol as its XML does.
func (p Protocol) String() string {
	switch p {
	case ProtocolExt:
		return "ext-data-control-v1"
	case ProtocolWlr:
		return "wlr-data-control-unstable-v1"
	}
	return fmt.Sprintf("Protocol(%d)", uint8(p))
}

var (
	// ErrUnavailable reports that the compositor advertises neither
	// data-control protocol, or the seat is missing.
	ErrUnavailable = errors.New("datacontrol: compositor offers no data-control protocol")
	// ErrPrimaryUnavailable reports a primary-selection request on a
	// wlr-data-control manager older than version 2.
	ErrPrimaryUnavailable = errors.New("datacontrol: the bound protocol has no primary selection")
	// ErrFinished reports that the compositor retired the device (its
	// seat went away); nothing more can be read or claimed through it.
	ErrFinished = errors.New("datacontrol: device finished")
	// ErrStaleOffer reports a read on an offer that is no longer the
	// selection: the compositor has already replaced it.
	ErrStaleOffer = errors.New("datacontrol: offer is no longer the selection")
	// ErrMimeNotOffered reports a read under a mime type the offer did
	// not advertise.
	ErrMimeNotOffered = errors.New("datacontrol: mime type not offered")
)

// transferTimeout bounds one offer read or claim write against a
// stalled peer (a var so tests can shorten it).
var transferTimeout = xfer.DefaultTimeout

// Device is the seat's data-control device. Create it with Bind; it
// is not safe for concurrent use — call it from the loop goroutine.
type Device struct {
	proto   Protocol
	primary bool
	mgr     wireManager
	dev     wireDevice
	invoke  func(func())

	// pending holds offers the compositor introduced whose selection
	// event has not arrived yet, with the mimes gathered so far.
	pending  map[wireOffer]*Offer
	current  [selections]*Offer
	claims   [selections]*Claim
	sources  map[wireSource]*Claim
	handlers []*selectionHandler
	finished bool
}

// selectionHandler boxes one OnSelection callback so removal can find
// it by identity.
type selectionHandler struct {
	fn func(Selection, *Offer)
}

// newDevice builds the device core over a manager; the wire adapter
// creates the protocol device with the core as its event sink.
func newDevice(proto Protocol, primary bool, mgr wireManager, invoke func(func())) *Device {
	return &Device{
		proto:   proto,
		primary: primary,
		mgr:     mgr,
		invoke:  invoke,
		pending: make(map[wireOffer]*Offer),
		sources: make(map[wireSource]*Claim),
	}
}

// Protocol reports which data-control protocol the device speaks.
func (d *Device) Protocol() Protocol { return d.proto }

// PrimaryAvailable reports whether the device can watch and claim the
// primary selection: always with ext-data-control, from version 2 of
// wlr-data-control.
func (d *Device) PrimaryAvailable() bool { return d.primary && !d.finished }

// Finished reports whether the compositor retired the device.
func (d *Device) Finished() bool { return d.finished }

// Offer returns sel's current offer, nil when the selection is empty
// (or sel is the primary selection and the device has none).
func (d *Device) Offer(sel Selection) *Offer {
	if !sel.valid() {
		return nil
	}
	return d.current[sel]
}

// OnSelection registers fn for every selection change: sel is the
// selection that changed and offer its new content, nil when the
// selection was cleared. The compositor announces both selections as
// soon as the device exists, so the first calls report what the seat
// already holds. The returned function unregisters fn.
func (d *Device) OnSelection(fn func(sel Selection, offer *Offer)) (remove func()) {
	h := &selectionHandler{fn: fn}
	d.handlers = append(d.handlers, h)
	return func() {
		d.handlers = slices.DeleteFunc(d.handlers, func(x *selectionHandler) bool { return x == h })
	}
}

// Source is what a claim offers: the mime types, in the order
// receivers should prefer them, and how each request is answered —
// exactly one of Data, Send and Pass.
type Source struct {
	Mimes []string
	// Data returns the payload for mime. It runs on the loop goroutine
	// once per request, and the bytes are written off the loop under
	// the transfer deadline; nil serves an empty payload.
	Data func(mime string) []byte
	// Send hands the receiver's pipe over instead, for payloads that
	// are produced later or elsewhere (a remote-desktop bridge that
	// asks its peer first). It runs on the loop goroutine; w carries
	// the transfer deadline, may be written from any goroutine, and
	// must be closed — closing is what ends the receiver's read.
	Send func(mime string, w io.WriteCloser)
	// Pass hands the receiver's pipe over untouched (blocking, no
	// deadline), for a bridge that passes the descriptor itself to
	// another process (a portal handing it to the app that owns the
	// selection). It runs on the loop goroutine and owns pipe: it must
	// close it, after passing it on or not.
	Pass func(mime string, pipe *os.File)
}

// validate rejects a source the protocol or its receivers cannot use:
// nothing offered, an empty mime name, a mime offered twice (a
// protocol error on some compositors), or not exactly one way to
// answer a request.
func (s Source) validate() error {
	if len(s.Mimes) == 0 {
		return errors.New("datacontrol: a source must offer at least one mime type")
	}
	if ways := btoi(s.Data != nil) + btoi(s.Send != nil) + btoi(s.Pass != nil); ways != 1 {
		return errors.New("datacontrol: a source needs exactly one of Data, Send and Pass")
	}
	seen := make(map[string]bool, len(s.Mimes))
	for _, m := range s.Mimes {
		if m == "" {
			return errors.New("datacontrol: empty mime type")
		}
		if seen[m] {
			return fmt.Errorf("datacontrol: mime type %q offered twice", m)
		}
		seen[m] = true
	}
	return nil
}

// SetSelection claims sel with src. The previous claim on sel, if
// any, is cancelled first-hand: its OnCancelled callbacks fire and it
// stops serving. The compositor then announces the new selection like
// any other — the offer that arrives is marked Own.
func (d *Device) SetSelection(sel Selection, src Source) (*Claim, error) {
	if err := d.usable(sel); err != nil {
		return nil, err
	}
	if err := src.validate(); err != nil {
		return nil, err
	}
	source, err := d.mgr.createSource(d)
	if err != nil {
		return nil, fmt.Errorf("datacontrol: create source: %w", err)
	}
	for _, m := range src.Mimes {
		if err := source.offer(m); err != nil {
			_ = source.destroy()
			return nil, fmt.Errorf("datacontrol: offer: %w", err)
		}
	}
	if err := d.setWire(sel, source); err != nil {
		_ = source.destroy()
		return nil, fmt.Errorf("datacontrol: set %s selection: %w", sel, err)
	}
	c := &Claim{
		dev:   d,
		sel:   sel,
		wire:  source,
		mimes: slices.Clone(src.Mimes),
		src:   src,
		live:  true,
	}
	d.sources[source] = c
	old := d.claims[sel]
	d.claims[sel] = c
	old.end()
	return c, nil
}

// ClearSelection empties sel, cancelling our claim on it if we hold
// one.
func (d *Device) ClearSelection(sel Selection) error {
	if err := d.usable(sel); err != nil {
		return err
	}
	if err := d.setWire(sel, nil); err != nil {
		return fmt.Errorf("datacontrol: clear %s selection: %w", sel, err)
	}
	old := d.claims[sel]
	d.claims[sel] = nil
	old.end()
	return nil
}

// usable reports why sel cannot be claimed right now, nil when it
// can.
func (d *Device) usable(sel Selection) error {
	switch {
	case !sel.valid():
		return fmt.Errorf("datacontrol: unknown selection %d", uint8(sel))
	case d.finished:
		return ErrFinished
	case sel == Primary && !d.primary:
		return ErrPrimaryUnavailable
	}
	return nil
}

// setWire sends set_selection or set_primary_selection.
func (d *Device) setWire(sel Selection, source wireSource) error {
	if sel == Primary {
		return d.dev.setPrimarySelection(source)
	}
	return d.dev.setSelection(source)
}

// dataOffer implements eventSink: a new offer object; its mimes follow.
func (d *Device) dataOffer(o wireOffer) {
	d.pending[o] = &Offer{dev: d, wire: o}
}

// offerMime implements eventSink: one advertised mime type of o.
func (d *Device) offerMime(o wireOffer, mime string) {
	if off := d.pending[o]; off != nil && !slices.Contains(off.mimes, mime) {
		off.mimes = append(off.mimes, mime)
	}
}

// selection implements eventSink: o is now sel's content, nil for an
// empty selection. The replaced offer is destroyed, and so is every
// other pending offer — each selection event follows its own offer's
// introduction, so one left behind was never going to be named.
func (d *Device) selection(sel Selection, o wireOffer) {
	if !sel.valid() {
		return
	}
	var next *Offer
	if o != nil {
		next = d.pending[o]
		delete(d.pending, o)
		if next == nil {
			// A selection naming an offer we never saw introduced:
			// nothing to read its mimes from, but it still replaces
			// the old content.
			next = &Offer{dev: d, wire: o}
		}
		next.sel = sel
		next.live = true
		// The compositor announces our own claim back to us; a
		// clipboard manager must not read its own offer (it would be
		// both ends of the pipe) nor record it twice.
		next.own = d.claims[sel].Live()
	}
	for w, orphan := range d.pending {
		orphan.destroy()
		delete(d.pending, w)
	}
	prev := d.current[sel]
	d.current[sel] = next
	if prev != nil && prev != d.current[1-sel] {
		prev.destroy()
	}
	d.emit(sel, next)
}

// finish implements eventSink: the compositor retired the device.
// Both selections read as cleared and every claim ends.
func (d *Device) finish() {
	if d.finished {
		return
	}
	d.finished = true
	for w, off := range d.pending {
		off.destroy()
		delete(d.pending, w)
	}
	for sel := range Selection(selections) {
		if prev := d.current[sel]; prev != nil {
			d.current[sel] = nil
			prev.destroy()
			d.emit(sel, nil)
		}
		old := d.claims[sel]
		d.claims[sel] = nil
		old.end()
	}
	_ = d.dev.destroy()
}

// emit runs the selection handlers over a snapshot, so a handler that
// unregisters itself does not disturb the walk.
func (d *Device) emit(sel Selection, offer *Offer) {
	for _, h := range slices.Clone(d.handlers) {
		h.fn(sel, offer)
	}
}

// send implements eventSink: a receiver asked source for mime. The
// fd is ours to close whatever happens; a request for a claim that
// ended, or for a mime it never offered, gets an immediate EOF.
func (d *Device) send(source wireSource, mime string, fd uintptr, fdErr error) {
	if fdErr != nil {
		return
	}
	c := d.sources[source]
	if c == nil || !c.live || !slices.Contains(c.mimes, mime) {
		closeFD(fd)
		return
	}
	if c.src.Pass != nil {
		c.src.Pass(mime, os.NewFile(fd, "data-control-send"))
		return
	}
	w, err := xfer.DeadlineWriter(fd, transferTimeout)
	if err != nil {
		debug.Log("input", "data-control send: %v", err)
		closeFD(fd)
		return
	}
	if c.src.Send != nil {
		c.src.Send(mime, w)
		return
	}
	payload := c.src.Data(mime)
	// Off the loop: a slow receiver of a large payload must not stall
	// input and frames; the deadline still cuts a stalled one off.
	go func() {
		if _, err := w.Write(payload); err != nil {
			debug.Log("input", "data-control send: %v", err)
		}
		_ = w.Close()
	}()
}

// cancelled implements eventSink: the compositor took the selection
// away from source (another client claimed it).
func (d *Device) cancelled(source wireSource) {
	c := d.sources[source]
	if c == nil {
		return
	}
	if d.claims[c.sel] == c {
		d.claims[c.sel] = nil
	}
	c.end()
}

// closeFD closes a raw received descriptor.
func closeFD(fd uintptr) {
	if f := os.NewFile(fd, "data-control-send"); f != nil {
		_ = f.Close()
	}
}

// Offer is one selection's content as its owner advertised it. It
// stays readable until the compositor replaces the selection.
type Offer struct {
	dev   *Device
	wire  wireOffer
	sel   Selection
	mimes []string
	own   bool
	live  bool
}

// Selection reports which selection the offer is.
func (o *Offer) Selection() Selection { return o.sel }

// Mimes returns the advertised mime types, in the order the owner
// offered them.
func (o *Offer) Mimes() []string { return slices.Clone(o.mimes) }

// Has reports whether the owner advertised mime.
func (o *Offer) Has(mime string) bool { return slices.Contains(o.mimes, mime) }

// Own reports whether the offer is our own claim announced back to
// us. Reading it would put this process on both ends of the pipe, and
// a clipboard history would record its own restore as a new copy.
func (o *Offer) Own() bool { return o.own }

// Live reports whether the offer is still the selection.
func (o *Offer) Live() bool { return o.live }

// Receive asks the owner for the content under mime and returns the
// read end of the pipe it streams into; the caller closes it. The
// owner writes at its own pace, so drain the pipe off the loop
// goroutine — or use Read, which does. Only advertised mimes are
// accepted.
func (o *Offer) Receive(mime string) (*os.File, error) {
	if !o.live {
		return nil, ErrStaleOffer
	}
	if !o.Has(mime) {
		return nil, fmt.Errorf("%w: %q", ErrMimeNotOffered, mime)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("datacontrol: pipe: %w", err)
	}
	// The request carries the write end over SCM_RIGHTS as it is sent,
	// so the compositor holds its own copy once receive returns;
	// dropping ours is what lets the reader see EOF at the owner's
	// last byte.
	err = o.wire.receive(mime, w.Fd())
	_ = w.Close()
	if err != nil {
		_ = r.Close()
		return nil, fmt.Errorf("datacontrol: receive: %w", err)
	}
	return r, nil
}

// Read fetches the content under mime and calls done on the loop
// goroutine with it. The pipe drains on a goroutine, bounded by limit
// bytes and the transfer deadline: an oversized payload is refused
// whole with an error wrapping xfer.ErrTooLarge, a stalled owner with
// one wrapping xfer.ErrTimeout, and done then gets no bytes. The
// error return covers only what fails before the transfer starts.
func (o *Offer) Read(mime string, limit int64, done func([]byte, error)) error {
	if limit <= 0 {
		return fmt.Errorf("datacontrol: read limit %d must be positive", limit)
	}
	if done == nil {
		return errors.New("datacontrol: Read needs a done callback")
	}
	r, err := o.Receive(mime)
	if err != nil {
		return err
	}
	invoke := o.dev.invoke
	go func() {
		defer r.Close()
		data, err := xfer.Read(r, limit, transferTimeout)
		if err != nil {
			data = nil
		}
		invoke(func() { done(data, err) })
	}()
	return nil
}

// destroy retires the offer object.
func (o *Offer) destroy() {
	o.live = false
	_ = o.wire.destroy()
}

// Claim is our ownership of one selection, live until another client
// claims it, we replace or release it, or the device finishes.
type Claim struct {
	dev         *Device
	sel         Selection
	wire        wireSource
	mimes       []string
	src         Source
	live        bool
	onCancelled []func()
}

// Selection reports which selection the claim holds.
func (c *Claim) Selection() Selection { return c.sel }

// Live reports whether the claim still holds its selection. Nil-safe.
func (c *Claim) Live() bool { return c != nil && c.live }

// OnCancelled registers fn to run once the claim ends for any reason
// other than Release; it runs at once when the claim already ended.
func (c *Claim) OnCancelled(fn func()) {
	if !c.live {
		fn()
		return
	}
	c.onCancelled = append(c.onCancelled, fn)
}

// Release gives the selection up: the source is destroyed, which
// empties the selection unless another client claimed it since. The
// OnCancelled callbacks do not run. Idempotent.
func (c *Claim) Release() {
	if !c.live {
		return
	}
	if c.dev.claims[c.sel] == c {
		c.dev.claims[c.sel] = nil
	}
	c.onCancelled = nil
	c.end()
}

// end retires the claim: no more serving, source destroyed, the
// cancellation callbacks run once. Nil-safe and idempotent.
func (c *Claim) end() {
	if c == nil || !c.live {
		return
	}
	c.live = false
	_ = c.wire.destroy()
	delete(c.dev.sources, c.wire)
	// The object stays registered with the connection until the
	// compositor acknowledges the destroy, so a late send can still
	// reach send(), which answers it with EOF.
	fns := c.onCancelled
	c.onCancelled = nil
	for _, fn := range fns {
		fn()
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
