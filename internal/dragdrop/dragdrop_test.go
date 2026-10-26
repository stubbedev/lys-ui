package dragdrop

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/transfer"
)

// fakeOffer records the requests the controller sends to one offer.
type fakeOffer struct {
	accepts    []acceptCall
	setActions [][2]uint32
	received   []string
	receiveErr error
	finished   int
}

type acceptCall struct {
	serial uint32
	mime   string
}

func (f *fakeOffer) Accept(serial uint32, mimeType string) error {
	f.accepts = append(f.accepts, acceptCall{serial, mimeType})
	return nil
}

func (f *fakeOffer) SetActions(dndActions, preferredAction uint32) error {
	f.setActions = append(f.setActions, [2]uint32{dndActions, preferredAction})
	return nil
}

func (f *fakeOffer) Receive(mimeType string, fd uintptr) error {
	f.received = append(f.received, mimeType)
	return f.receiveErr
}

func (f *fakeOffer) Finish() error { f.finished++; return nil }

// fakeSource records the requests the controller sends to one source.
type fakeSource struct {
	offers    []string
	actions   []uint32
	destroyed int
}

func (f *fakeSource) Offer(mimeType string) error {
	f.offers = append(f.offers, mimeType)
	return nil
}

func (f *fakeSource) SetActions(dndActions uint32) error {
	f.actions = append(f.actions, dndActions)
	return nil
}

func (f *fakeSource) Destroy() error { f.destroyed++; return nil }

func (f *fakeSource) AddSendHandler(wl.DataSourceSendHandler)                         {}
func (f *fakeSource) AddCancelledHandler(wl.DataSourceCancelledHandler)               {}
func (f *fakeSource) AddDndDropPerformedHandler(wl.DataSourceDndDropPerformedHandler) {}
func (f *fakeSource) AddDndFinishedHandler(wl.DataSourceDndFinishedHandler)           {}
func (f *fakeSource) AddTargetHandler(wl.DataSourceTargetHandler)                     {}
func (f *fakeSource) AddActionHandler(wl.DataSourceActionHandler)                     {}

// fakeMaker hands out one fake source; the wire proxy is an opaque
// identity the controller only names in start_drag.
type fakeMaker struct {
	src *fakeSource
	err error
}

func (m fakeMaker) CreateDataSource() (*wl.DataSource, sourceAPI, error) {
	return &wl.DataSource{}, m.src, m.err
}

// fakeDevice records start_drag requests.
type fakeDevice struct {
	drags []dragCall
	err   error
}

type dragCall struct {
	source       *wl.DataSource
	origin, icon *wl.Surface
	serial       uint32
}

func (d *fakeDevice) StartDrag(source *wl.DataSource, origin, icon *wl.Surface, serial uint32) error {
	d.drags = append(d.drags, dragCall{source, origin, icon, serial})
	return d.err
}

// recordingTarget plays a drop target and records what it received.
type recordingTarget struct {
	accept   string
	motionRe string

	mimes   []string
	enterX  float64
	enterY  float64
	enters  int
	motions int
	leaves  int
	drops   int
	dropX   float64
	dropY   float64
}

func (t *recordingTarget) DragEnter(mimes []string, x, y float64) string {
	t.enters++
	t.mimes = mimes
	t.enterX, t.enterY = x, y
	return t.accept
}

func (t *recordingTarget) DragMotion(x, y float64) string {
	t.motions++
	return t.motionRe
}

func (t *recordingTarget) DragLeave() { t.leaves++ }

func (t *recordingTarget) Drop(x, y float64) {
	t.drops++
	t.dropX, t.dropY = x, y
}

// newTestController builds a controller around a connectionless
// session; the routing state machine is pure client logic.
func newTestController() *Controller {
	return &Controller{
		sess:    &wlsession.Session{},
		version: minActionVersion,
		offers:  make(map[*wl.DataOffer]*offerMimes),
	}
}

func TestOfferMimesCollectInOrder(t *testing.T) {
	m := &offerMimes{}
	m.HandleDataOfferOffer(wl.DataOfferOfferEvent{MimeType: "application/x-lys-ui-row"})
	m.HandleDataOfferOffer(wl.DataOfferOfferEvent{MimeType: "text/plain"})
	got := m.mimes
	if len(got) != 2 || got[0] != "application/x-lys-ui-row" || got[1] != "text/plain" {
		t.Errorf("mimes = %v, want advertised order", got)
	}
}

func TestEnterAsksTargetAndStoresSerial(t *testing.T) {
	c := newTestController()
	target := &recordingTarget{accept: "a/b"}
	c.offers[nil] = &offerMimes{mimes: []string{"a/b", "c/d"}}

	c.enter(target, 3, 4, 42, nil)

	if target.enters != 1 {
		t.Fatalf("target entered %d times, want 1", target.enters)
	}
	if len(target.mimes) != 2 || target.mimes[0] != "a/b" {
		t.Errorf("target saw mimes %v, want the advertised ones best first", target.mimes)
	}
	if target.enterX != 3 || target.enterY != 4 {
		t.Errorf("enter at (%.0f,%.0f), want (3,4)", target.enterX, target.enterY)
	}
	if c.accepted != "a/b" {
		t.Errorf("accepted = %q, want a/b", c.accepted)
	}
	if c.serial != 42 {
		t.Errorf("serial = %d, want the enter serial 42", c.serial)
	}
}

func TestAcceptUsesEnterSerialAndVersion(t *testing.T) {
	t.Run("version 3 accepts with the enter serial, copy and move, copy preferred", func(t *testing.T) {
		c := newTestController()
		offer := &fakeOffer{}
		c.dragOffer = offer
		c.serial = 77

		c.accept("a/b")

		if len(offer.accepts) != 1 || offer.accepts[0] != (acceptCall{77, "a/b"}) {
			t.Errorf("accepts = %v, want one accept with serial 77 and a/b", offer.accepts)
		}
		if len(offer.setActions) != 1 || offer.setActions[0] != [2]uint32{3, 1} {
			t.Errorf("setActions = %v, want copy|move preferring copy", offer.setActions)
		}
	})

	t.Run("rejecting accepts the empty mime", func(t *testing.T) {
		c := newTestController()
		offer := &fakeOffer{}
		c.dragOffer = offer
		c.serial = 5

		c.accept("")

		if len(offer.accepts) != 1 || offer.accepts[0].mime != "" {
			t.Errorf("accepts = %v, want one empty-mime reject", offer.accepts)
		}
	})

	t.Run("version 2 has no action requests", func(t *testing.T) {
		c := newTestController()
		c.version = 2
		offer := &fakeOffer{}
		c.dragOffer = offer
		c.serial = 5

		c.accept("a/b")

		if len(offer.setActions) != 0 {
			t.Errorf("v2 controller sent %d set_actions, want none", len(offer.setActions))
		}
	})

	t.Run("no offer accepts nothing", func(t *testing.T) {
		c := newTestController()
		c.accept("a/b") // must not panic
	})
}

func TestMotionReasksTargetAndUpdatesAccept(t *testing.T) {
	c := newTestController()
	target := &recordingTarget{accept: "a/b", motionRe: "c/d"}
	c.offers[nil] = &offerMimes{mimes: []string{"a/b", "c/d"}}
	c.enter(target, 1, 1, 9, nil)
	// The enter's offer is the one accept replies on.
	offer := &fakeOffer{}
	c.dragOffer = offer

	c.motion(2, 2)

	if target.motions != 1 {
		t.Errorf("target got %d motions, want 1", target.motions)
	}
	if c.accepted != "c/d" {
		t.Errorf("accepted = %q, want the motion's c/d", c.accepted)
	}
	// The decision change re-accepts with the enter serial.
	if len(offer.accepts) != 1 || offer.accepts[0] != (acceptCall{9, "c/d"}) {
		t.Errorf("accepts = %v, want the changed mime accepted with serial 9", offer.accepts)
	}
}

func TestLeaveNotifiesAndResets(t *testing.T) {
	c := newTestController()
	target := &recordingTarget{accept: "a/b"}
	c.offers[nil] = &offerMimes{mimes: []string{"a/b"}}
	c.enter(target, 1, 1, 9, nil)

	c.leave()

	if target.leaves != 1 {
		t.Errorf("target got %d leaves, want 1", target.leaves)
	}
	if c.target != nil || c.dragOffer != nil || c.accepted != "" {
		t.Errorf("drag state survived leave: target=%v offer=%v accepted=%q",
			c.target, c.dragOffer, c.accepted)
	}
}

func TestDropDeliversOnlyWhenAccepted(t *testing.T) {
	t.Run("accepted drop reaches the target", func(t *testing.T) {
		c := newTestController()
		target := &recordingTarget{accept: "a/b"}
		c.offers[nil] = &offerMimes{mimes: []string{"a/b"}}
		c.enter(target, 10, 20, 9, nil)

		c.drop()

		if target.drops != 1 || target.dropX != 10 || target.dropY != 20 {
			t.Errorf("drop = %d at (%.0f,%.0f), want once at (10,20)", target.drops, target.dropX, target.dropY)
		}
		if c.target != nil || c.accepted != "" {
			t.Errorf("drag state survived drop: target=%v accepted=%q", c.target, c.accepted)
		}
	})

	t.Run("rejected drag never drops", func(t *testing.T) {
		c := newTestController()
		target := &recordingTarget{accept: ""}
		c.offers[nil] = &offerMimes{mimes: []string{"a/b"}}
		c.enter(target, 1, 1, 9, nil)

		c.drop()

		if target.drops != 0 {
			t.Errorf("rejected drag delivered %d drops", target.drops)
		}
	})

	t.Run("motion without a target is ignored", func(t *testing.T) {
		c := newTestController()
		c.motion(5, 5) // must not panic
	})
}

// newSourceController builds a controller wired to the fake device and
// its source factory at the given data-device version.
func newSourceController(dev *fakeDevice, src *fakeSource, version uint32) *Controller {
	return &Controller{
		dev:     dev,
		maker:   fakeMaker{src: src},
		version: version,
		offers:  make(map[*wl.DataOffer]*offerMimes),
	}
}

func TestStartDragGuards(t *testing.T) {
	t.Run("no device fails with ErrUnavailable", func(t *testing.T) {
		c := newTestController()
		err := c.StartDrag(StartConfig{Content: transfer.Drag{Content: transfer.Content{Mimes: []string{"a/b"}, Write: fakeWriter}}})
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("StartDrag = %v, want ErrUnavailable", err)
		}
	})

	t.Run("a running drag rejects a second one", func(t *testing.T) {
		c := newSourceController(&fakeDevice{}, &fakeSource{}, minActionVersion)
		c.src = &wl.DataSource{}
		err := c.StartDrag(StartConfig{Content: transfer.Drag{Content: transfer.Content{Mimes: []string{"a/b"}, Write: fakeWriter}}})
		if !errors.Is(err, ErrDragActive) {
			t.Errorf("StartDrag = %v, want ErrDragActive", err)
		}
	})

	t.Run("content without mimes or writer fails", func(t *testing.T) {
		c := newSourceController(&fakeDevice{}, &fakeSource{}, minActionVersion)
		if err := c.StartDrag(StartConfig{Origin: &wl.Surface{}}); err == nil {
			t.Error("content without mimes started a drag")
		}
		if err := c.StartDrag(StartConfig{Content: transfer.Drag{Content: transfer.Content{Mimes: []string{"a/b"}}}}); err == nil {
			t.Error("content without a writer started a drag")
		}
	})
}

// fakeWriter satisfies Content.Write without producing anything.
func fakeWriter(mime string, w io.Writer) error {
	_, err := io.WriteString(w, "payload:"+mime)
	return err
}

func TestStartDragHappyPath(t *testing.T) {
	dev := &fakeDevice{}
	src := &fakeSource{}
	c := newSourceController(dev, src, minActionVersion)
	origin, icon := &wl.Surface{}, &wl.Surface{}
	done := 0

	err := c.StartDrag(StartConfig{
		Origin: origin, Icon: icon, GrabSerial: 1234,
		Content: transfer.Drag{
			Content: transfer.Content{Mimes: []string{"application/x-lys-ui-tile", "text/plain"}, Write: fakeWriter},
			OnDone:  func(transfer.Action) { done++ },
		},
	})
	if err != nil {
		t.Fatalf("StartDrag: %v", err)
	}

	// The offered mimes, best first.
	if len(src.offers) != 2 || src.offers[0] != "application/x-lys-ui-tile" || src.offers[1] != "text/plain" {
		t.Errorf("offered %v, want the content's mimes in order", src.offers)
	}
	// Version 3 declares the copy action on the source.
	if len(src.actions) != 1 || src.actions[0] != wl.DataDeviceManagerDndActionCopy {
		t.Errorf("source actions = %v, want copy", src.actions)
	}
	// start_drag carries the origin, the icon, and the grab serial.
	if len(dev.drags) != 1 {
		t.Fatalf("start_drag sent %d times, want once", len(dev.drags))
	}
	call := dev.drags[0]
	if call.origin != origin || call.icon != icon {
		t.Errorf("start_drag origin=%v icon=%v, want the configured surfaces", call.origin, call.icon)
	}
	if call.serial != 1234 {
		t.Errorf("start_drag serial = %d, want the grab serial 1234", call.serial)
	}
	if !c.Dragging() {
		t.Error("controller does not report the running drag")
	}
	if done != 0 {
		t.Errorf("OnDone fired %d times before the drag concluded", done)
	}

	// The destination acknowledging the transfer concludes the source.
	// Swap the icon for a recorder first: the wire surface's Destroy
	// needs a live connection, its teardown is what we pin here.
	iconRec := &destroyRecorder{}
	c.icon = iconRec
	c.HandleDataSourceDndFinished(wl.DataSourceDndFinishedEvent{})
	if c.Dragging() {
		t.Error("drag still running after dnd_finished")
	}
	if src.destroyed != 1 {
		t.Errorf("source destroyed %d times, want once", src.destroyed)
	}
	if iconRec.destroyed != 1 {
		t.Errorf("icon destroyed %d times, want once", iconRec.destroyed)
	}
	if done != 1 {
		t.Errorf("OnDone fired %d times, want once with dropped=true", done)
	}
}

// destroyRecorder stands in for the icon surface.
type destroyRecorder struct{ destroyed int }

func (d *destroyRecorder) Destroy() error { d.destroyed++; return nil }

func TestStartDragDeviceErrorConcludes(t *testing.T) {
	dev := &fakeDevice{err: errors.New("boom")}
	src := &fakeSource{}
	c := newSourceController(dev, src, minActionVersion)
	concluded := -1
	err := c.StartDrag(StartConfig{
		GrabSerial: 7,
		Content: transfer.Drag{
			Content: transfer.Content{Mimes: []string{"a/b"}, Write: fakeWriter},
			OnDone:  func(a transfer.Action) { concluded = boolInt(a != transfer.ActionNone) },
		},
	})
	if err == nil {
		t.Fatal("StartDrag succeeded despite the device error")
	}
	if concluded != 0 {
		t.Errorf("OnDone(dropped=%d), want 0 for a drag that never started", concluded)
	}
	if c.Dragging() || src.destroyed != 1 {
		t.Errorf("failed start left the source alive (destroyed=%d)", src.destroyed)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestCancelAndDropPerformedConclude(t *testing.T) {
	t.Run("cancelled reports dropped=false", func(t *testing.T) {
		c := newTestController()
		src := &fakeSource{}
		c.src, c.srcReq = &wl.DataSource{}, src
		c.srcData = transfer.Drag{Mimes: []string{"a/b"}, Write: fakeWriter, OnDone: func(a transfer.Action) {
			if a != transfer.ActionNone {
				t.Error("cancel reported as dropped")
			}
		}}

		c.HandleDataSourceCancelled(wl.DataSourceCancelledEvent{})

		if c.Dragging() || src.destroyed != 1 {
			t.Error("cancelled drag left the source alive")
		}
	})

	t.Run("version 3 waits for finish after drop_performed", func(t *testing.T) {
		c := newTestController()
		c.src, c.srcReq = &wl.DataSource{}, &fakeSource{}
		c.srcData = transfer.Drag{Mimes: []string{"a/b"}, Write: fakeWriter}

		c.HandleDataSourceDndDropPerformed(wl.DataSourceDndDropPerformedEvent{})

		if !c.Dragging() {
			t.Error("v3 source concluded before dnd_finished")
		}
	})

	t.Run("version 2 concludes on drop_performed", func(t *testing.T) {
		c := newTestController()
		c.version = 2
		src := &fakeSource{}
		c.src, c.srcReq = &wl.DataSource{}, src
		c.srcData = transfer.Drag{Mimes: []string{"a/b"}, Write: fakeWriter}

		c.HandleDataSourceDndDropPerformed(wl.DataSourceDndDropPerformedEvent{})

		if c.Dragging() || src.destroyed != 1 {
			t.Error("v2 source survived drop_performed with no finish request")
		}
	})
}

func TestReadPayloadShortCircuitsSelfDrops(t *testing.T) {
	c := newTestController()
	offer := &fakeOffer{}
	c.dragOffer = offer
	c.accepted = "application/x-lys-ui-tile"
	src := &fakeSource{}
	c.src, c.srcReq = &wl.DataSource{}, src
	done := -1
	c.srcData = transfer.Drag{
		Mimes: []string{"application/x-lys-ui-tile"}, Write: fakeWriter,
		OnDone: func(a transfer.Action) {
			done = boolInt(a == transfer.ActionCopy)
		},
	}

	data, err := c.ReadPayload("application/x-lys-ui-tile")
	if err != nil {
		t.Fatalf("ReadPayload: %v", err)
	}
	if string(data) != "payload:application/x-lys-ui-tile" {
		t.Errorf("payload = %q, want the provider's bytes", data)
	}
	// The wire transfer never starts for a self-drop…
	if len(offer.received) != 0 || offer.finished != 0 {
		t.Errorf("self-drop used the pipe (receive=%v finish=%d)", offer.received, offer.finished)
	}
	// …and the source concludes immediately, dropped=true.
	if done != 1 {
		t.Errorf("OnDone(dropped=%d), want 1", done)
	}
	if c.Dragging() {
		t.Error("source survived its own drop")
	}
}

func TestReadPayloadRequiresADrag(t *testing.T) {
	c := newTestController()
	if _, err := c.ReadPayload(""); !errors.Is(err, ErrNoPayload) {
		t.Errorf("empty mime = %v, want ErrNoPayload", err)
	}
	if _, err := c.ReadPayload("a/b"); !errors.Is(err, ErrNoPayload) {
		t.Errorf("no drag = %v, want ErrNoPayload", err)
	}
	// An accepted drag whose receive fails surfaces the error.
	c.dragOffer = &fakeOffer{receiveErr: errors.New("no fd")}
	c.accepted = "a/b"
	if _, err := c.ReadPayload("a/b"); err == nil {
		t.Error("failed receive returned no error")
	}
}

func TestBindRoutesThroughSession(t *testing.T) {
	sess := &wlsession.Session{}
	c := New(sess)
	target := &recordingTarget{accept: "a/b", motionRe: "a/b"}
	surf := &wl.Surface{}
	c.Bind(surf, target)

	// The compositor's enter names the surface; the controller must
	// route it to that surface's target with the advertised mimes.
	c.offers[nil] = &offerMimes{mimes: []string{"a/b"}}
	sess.HandleDataDeviceEnter(wl.DataDeviceEnterEvent{Surface: surf, X: 2, Y: 3, Serial: 11})

	if target.enters != 1 || target.enterX != 2 || target.enterY != 3 {
		t.Errorf("target got %d enters at (%.0f,%.0f), want one at (2,3)", target.enters, target.enterX, target.enterY)
	}
	if c.accepted != "a/b" || c.serial != 11 {
		t.Errorf("accepted=%q serial=%d, want a/b and 11", c.accepted, c.serial)
	}

	// Motion and drop arrive without a surface: they reach the target
	// holding the drag.
	sess.HandleDataDeviceMotion(wl.DataDeviceMotionEvent{X: 4, Y: 5})
	if target.motions != 1 {
		t.Errorf("target got %d motions, want 1", target.motions)
	}
	sess.HandleDataDeviceDrop(wl.DataDeviceDropEvent{})
	if target.drops != 1 {
		t.Errorf("target got %d drops, want 1", target.drops)
	}

	// Unbinding stops delivery.
	c.Bind(surf, nil)
	sess.HandleDataDeviceEnter(wl.DataDeviceEnterEvent{Surface: surf})
	if target.enters != 1 {
		t.Errorf("unbound target got another enter (%d)", target.enters)
	}
}

func TestSourceSendWritesAndCloses(t *testing.T) {
	c := newTestController()
	c.srcData = transfer.Drag{Mimes: []string{"text/plain"}, Write: fakeWriter}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: "text/plain", Fd: w.Fd()})
	w.Close()

	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "payload:text/plain" {
		t.Errorf("received %q, want the provider payload", buf[:n])
	}
	if _, err := r.Read(buf); err == nil {
		t.Error("fd still open after Send")
	}

	// A send for a drag without content is ignored.
	c2 := newTestController()
	c2.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: "text/plain", Fd: w.Fd()})
}

// actionTarget is a drop target that prefers one action.
type actionTarget struct {
	recordingTarget
	want transfer.Action
}

func (a *actionTarget) DragAction(transfer.Action) transfer.Action { return a.want }

// An ActionTarget's pick becomes the preferred action; a rejection
// declares none; an ask drop settles on the target's non-ask pick
// before the finish.
func TestDropActions(t *testing.T) {
	c := newTestController()
	offer := &fakeOffer{}
	c.dragOffer = offer
	c.dragActions = transfer.ActionCopy | transfer.ActionMove
	c.target = &actionTarget{want: transfer.ActionMove}
	c.accept("a/b")
	if got := offer.setActions[len(offer.setActions)-1]; got != [2]uint32{3, 2} {
		t.Errorf("move target set %v, want copy|move preferring move", got)
	}
	c.accept("")
	if got := offer.setActions[len(offer.setActions)-1]; got != [2]uint32{0, 0} {
		t.Errorf("rejection set %v, want none", got)
	}

	// The fake keeps no copy of the fd: the read ends empty at once.
	resolveOffer := &fakeOffer{}
	if _, err := receivePayload(resolveOffer, "a/b", func() error { return nil }, minActionVersion, transfer.ActionMove); err != nil {
		t.Fatal(err)
	}
	if len(resolveOffer.setActions) != 1 || resolveOffer.setActions[0] != [2]uint32{2, 2} || resolveOffer.finished != 1 {
		t.Errorf("ask resolve set %v finish %d, want move alone then finish", resolveOffer.setActions, resolveOffer.finished)
	}
}

// The source hears acceptance and the negotiated action as they
// change, the finish reports the negotiated action, and CancelDrag
// ends the drag with ActionNone.
func TestSourceFeedbackAndCancel(t *testing.T) {
	c := newTestController()
	src := &fakeSource{}
	c.src, c.srcReq = &wl.DataSource{}, src
	var heard []transfer.Feedback
	var done []transfer.Action
	c.srcData = transfer.Drag{
		Mimes: []string{"a/b"}, Write: fakeWriter,
		Actions:    transfer.ActionCopy | transfer.ActionMove,
		OnFeedback: func(f transfer.Feedback) { heard = append(heard, f) },
		OnDone:     func(a transfer.Action) { done = append(done, a) },
	}
	c.HandleDataSourceTarget(wl.DataSourceTargetEvent{MimeType: "a/b"})
	c.HandleDataSourceAction(wl.DataSourceActionEvent{DndAction: uint32(transfer.ActionMove)})
	c.HandleDataSourceAction(wl.DataSourceActionEvent{DndAction: uint32(transfer.ActionMove)}) // unchanged: not repeated
	if len(heard) != 2 || !heard[1].Accepted() || heard[1].Action != transfer.ActionMove {
		t.Fatalf("feedback = %+v", heard)
	}
	c.HandleDataSourceDndFinished(wl.DataSourceDndFinishedEvent{})
	if len(done) != 1 || done[0] != transfer.ActionMove {
		t.Fatalf("done = %v, want move", done)
	}

	if c.CancelDrag() {
		t.Error("CancelDrag with no drag reported one")
	}
	c.src, c.srcReq = &wl.DataSource{}, src
	c.srcData.OnDone = func(a transfer.Action) { done = append(done, a) }
	if !c.CancelDrag() || c.Dragging() || done[len(done)-1] != transfer.ActionNone {
		t.Errorf("cancel: dragging %v, done %v", c.Dragging(), done)
	}
}
