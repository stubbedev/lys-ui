package datacontrol

import (
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// The two data-control protocols are the same protocol under two
// names: ext-data-control-v1 is wlr-data-control-unstable-v1 promoted
// to staging, request for request and event for event. The Device
// core is written once against the narrow interfaces below; one
// adapter per protocol translates the generated bindings, and tests
// substitute recorders.

// wireManager is the request side of a data-control manager.
type wireManager interface {
	// createSource makes a data source whose events reach sink.
	createSource(sink eventSink) (wireSource, error)
}

// wireDevice is the request side of a data-control device.
type wireDevice interface {
	setSelection(source wireSource) error
	setPrimarySelection(source wireSource) error
	destroy() error
}

// wireSource is the request side of a data-control source. Adapters
// are comparable values so the core can key its claims by them.
type wireSource interface {
	offer(mime string) error
	destroy() error
}

// wireOffer is the request side of a data-control offer, comparable
// like wireSource.
type wireOffer interface {
	receive(mime string, fd uintptr) error
	destroy() error
}

// eventSink is the event side, implemented by *Device.
type eventSink interface {
	dataOffer(o wireOffer)
	offerMime(o wireOffer, mime string)
	selection(sel Selection, o wireOffer)
	finish()
	send(source wireSource, mime string, fd uintptr, fdErr error)
	cancelled(source wireSource)
}

// Bind creates the seat's data-control device, preferring
// ext-data-control-v1 over wlr-data-control-unstable-v1. invoke must
// run a function on the loop goroutine (the application's Invoke):
// finished reads deliver through it. Fails with ErrUnavailable when
// the compositor offers neither protocol or has no seat.
func Bind(sess *wlsession.Session, invoke func(func())) (*Device, error) {
	seat := sess.Seat()
	if seat == nil {
		return nil, ErrUnavailable
	}
	if mgr := sess.ExtDataControlManager(); mgr != nil {
		d := newDevice(ProtocolExt, true, extManager{mgr}, invoke)
		dev, err := mgr.GetDataDevice(seat)
		if err != nil {
			return nil, err
		}
		d.dev = newExtDevice(dev, d)
		return d, nil
	}
	if mgr, version := sess.WlrDataControlManager(); mgr != nil {
		d := newDevice(ProtocolWlr, version >= 2, wlrManager{mgr}, invoke)
		dev, err := mgr.GetDataDevice(seat)
		if err != nil {
			return nil, err
		}
		d.dev = newWlrDevice(dev, d)
		return d, nil
	}
	return nil, ErrUnavailable
}

// ext-data-control-v1 adapters.

type (
	extManager struct{ p *wlr.DataControlManagerV1 }
	extDevice  struct {
		p    *wlr.DataControlDeviceV1
		sink eventSink
	}
	extSource struct {
		p    *wlr.DataControlSourceV1
		sink eventSink
	}
	extOffer struct {
		p    *wlr.DataControlOfferV1
		sink eventSink
	}
)

func (m extManager) createSource(sink eventSink) (wireSource, error) {
	p, err := m.p.CreateDataSource()
	if err != nil {
		return nil, err
	}
	s := extSource{p: p, sink: sink}
	p.AddSendHandler(s)
	p.AddCancelledHandler(s)
	return s, nil
}

func newExtDevice(p *wlr.DataControlDeviceV1, sink eventSink) extDevice {
	d := extDevice{p: p, sink: sink}
	p.AddDataOfferHandler(d)
	p.AddSelectionHandler(d)
	p.AddPrimarySelectionHandler(d)
	p.AddFinishedHandler(d)
	return d
}

func (d extDevice) setSelection(s wireSource) error {
	return d.p.SetSelection(extSourceProxy(s))
}

func (d extDevice) setPrimarySelection(s wireSource) error {
	return d.p.SetPrimarySelection(extSourceProxy(s))
}

func (d extDevice) destroy() error { return d.p.Destroy() }

// extSourceProxy unwraps a source for a request; nil clears.
func extSourceProxy(s wireSource) *wlr.DataControlSourceV1 {
	if src, ok := s.(extSource); ok {
		return src.p
	}
	return nil
}

func (d extDevice) HandleDataControlDeviceV1DataOffer(ev wlr.DataControlDeviceV1DataOfferEvent) {
	if ev.Id == nil {
		return
	}
	o := extOffer{p: ev.Id, sink: d.sink}
	ev.Id.AddOfferHandler(o)
	d.sink.dataOffer(o)
}

func (d extDevice) HandleDataControlDeviceV1Selection(ev wlr.DataControlDeviceV1SelectionEvent) {
	d.sink.selection(Clipboard, d.offer(ev.Id))
}

func (d extDevice) HandleDataControlDeviceV1PrimarySelection(ev wlr.DataControlDeviceV1PrimarySelectionEvent) {
	d.sink.selection(Primary, d.offer(ev.Id))
}

func (d extDevice) HandleDataControlDeviceV1Finished(wlr.DataControlDeviceV1FinishedEvent) {
	d.sink.finish()
}

// offer wraps an event's offer, keeping a nil proxy a nil interface.
func (d extDevice) offer(p *wlr.DataControlOfferV1) wireOffer {
	if p == nil {
		return nil
	}
	return extOffer{p: p, sink: d.sink}
}

func (s extSource) offer(mime string) error { return s.p.Offer(mime) }
func (s extSource) destroy() error          { return s.p.Destroy() }

func (s extSource) HandleDataControlSourceV1Send(ev wlr.DataControlSourceV1SendEvent) {
	s.sink.send(s, ev.MimeType, ev.Fd, ev.FdError)
}

func (s extSource) HandleDataControlSourceV1Cancelled(wlr.DataControlSourceV1CancelledEvent) {
	s.sink.cancelled(s)
}

func (o extOffer) receive(mime string, fd uintptr) error { return o.p.Receive(mime, fd) }
func (o extOffer) destroy() error                        { return o.p.Destroy() }

func (o extOffer) HandleDataControlOfferV1Offer(ev wlr.DataControlOfferV1OfferEvent) {
	o.sink.offerMime(o, ev.MimeType)
}

// wlr-data-control-unstable-v1 adapters.

type (
	wlrManager struct{ p *wlr.ZwlrDataControlManagerV1 }
	wlrDevice  struct {
		p    *wlr.ZwlrDataControlDeviceV1
		sink eventSink
	}
	wlrSource struct {
		p    *wlr.ZwlrDataControlSourceV1
		sink eventSink
	}
	wlrOffer struct {
		p    *wlr.ZwlrDataControlOfferV1
		sink eventSink
	}
)

func (m wlrManager) createSource(sink eventSink) (wireSource, error) {
	p, err := m.p.CreateDataSource()
	if err != nil {
		return nil, err
	}
	s := wlrSource{p: p, sink: sink}
	p.AddSendHandler(s)
	p.AddCancelledHandler(s)
	return s, nil
}

func newWlrDevice(p *wlr.ZwlrDataControlDeviceV1, sink eventSink) wlrDevice {
	d := wlrDevice{p: p, sink: sink}
	p.AddDataOfferHandler(d)
	p.AddSelectionHandler(d)
	p.AddPrimarySelectionHandler(d)
	p.AddFinishedHandler(d)
	return d
}

func (d wlrDevice) setSelection(s wireSource) error {
	return d.p.SetSelection(wlrSourceProxy(s))
}

func (d wlrDevice) setPrimarySelection(s wireSource) error {
	return d.p.SetPrimarySelection(wlrSourceProxy(s))
}

func (d wlrDevice) destroy() error { return d.p.Destroy() }

// wlrSourceProxy unwraps a source for a request; nil clears.
func wlrSourceProxy(s wireSource) *wlr.ZwlrDataControlSourceV1 {
	if src, ok := s.(wlrSource); ok {
		return src.p
	}
	return nil
}

func (d wlrDevice) HandleZwlrDataControlDeviceV1DataOffer(ev wlr.ZwlrDataControlDeviceV1DataOfferEvent) {
	if ev.Id == nil {
		return
	}
	o := wlrOffer{p: ev.Id, sink: d.sink}
	ev.Id.AddOfferHandler(o)
	d.sink.dataOffer(o)
}

func (d wlrDevice) HandleZwlrDataControlDeviceV1Selection(ev wlr.ZwlrDataControlDeviceV1SelectionEvent) {
	d.sink.selection(Clipboard, d.offer(ev.Id))
}

func (d wlrDevice) HandleZwlrDataControlDeviceV1PrimarySelection(ev wlr.ZwlrDataControlDeviceV1PrimarySelectionEvent) {
	d.sink.selection(Primary, d.offer(ev.Id))
}

func (d wlrDevice) HandleZwlrDataControlDeviceV1Finished(wlr.ZwlrDataControlDeviceV1FinishedEvent) {
	d.sink.finish()
}

// offer wraps an event's offer, keeping a nil proxy a nil interface.
func (d wlrDevice) offer(p *wlr.ZwlrDataControlOfferV1) wireOffer {
	if p == nil {
		return nil
	}
	return wlrOffer{p: p, sink: d.sink}
}

func (s wlrSource) offer(mime string) error { return s.p.Offer(mime) }
func (s wlrSource) destroy() error          { return s.p.Destroy() }

func (s wlrSource) HandleZwlrDataControlSourceV1Send(ev wlr.ZwlrDataControlSourceV1SendEvent) {
	s.sink.send(s, ev.MimeType, ev.Fd, ev.FdError)
}

func (s wlrSource) HandleZwlrDataControlSourceV1Cancelled(wlr.ZwlrDataControlSourceV1CancelledEvent) {
	s.sink.cancelled(s)
}

func (o wlrOffer) receive(mime string, fd uintptr) error { return o.p.Receive(mime, fd) }
func (o wlrOffer) destroy() error                        { return o.p.Destroy() }

func (o wlrOffer) HandleZwlrDataControlOfferV1Offer(ev wlr.ZwlrDataControlOfferV1OfferEvent) {
	o.sink.offerMime(o, ev.MimeType)
}
