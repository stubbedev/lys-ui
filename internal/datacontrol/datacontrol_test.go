package datacontrol

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/xfer"
)

// fakeManager records the sources the core creates.
type fakeManager struct {
	sources []*fakeSource
	fail    error
}

func (m *fakeManager) createSource(eventSink) (wireSource, error) {
	if m.fail != nil {
		return nil, m.fail
	}
	s := &fakeSource{}
	m.sources = append(m.sources, s)
	return s, nil
}

// fakeDevice records set_selection / set_primary_selection requests;
// a nil entry is a clear.
type fakeDevice struct {
	selections []wireSource
	primaries  []wireSource
	destroyed  bool
}

func (d *fakeDevice) setSelection(s wireSource) error {
	d.selections = append(d.selections, s)
	return nil
}

func (d *fakeDevice) setPrimarySelection(s wireSource) error {
	d.primaries = append(d.primaries, s)
	return nil
}

func (d *fakeDevice) destroy() error {
	d.destroyed = true
	return nil
}

type fakeSource struct {
	offered   []string
	destroyed bool
}

func (s *fakeSource) offer(m string) error {
	s.offered = append(s.offered, m)
	return nil
}

func (s *fakeSource) destroy() error {
	s.destroyed = true
	return nil
}

// fakeOffer plays the selection owner: receive hands the dup'ed write
// end to write, which streams the payload on its own goroutine.
type fakeOffer struct {
	received  []string
	destroyed bool
	write     func(w *os.File)
}

func (o *fakeOffer) receive(mime string, fd uintptr) error {
	o.received = append(o.received, mime)
	dup, err := syscall.Dup(int(fd))
	if err != nil {
		return err
	}
	w := os.NewFile(uintptr(dup), "fake-owner")
	if o.write == nil {
		return w.Close()
	}
	go o.write(w)
	return nil
}

func (o *fakeOffer) destroy() error {
	o.destroyed = true
	return nil
}

// newTestDevice builds a core over fakes; invoke runs inline on the
// reading goroutine, so tests synchronize through channels.
func newTestDevice(primary bool) (*Device, *fakeManager, *fakeDevice) {
	mgr := &fakeManager{}
	dev := &fakeDevice{}
	d := newDevice(ProtocolExt, primary, mgr, func(fn func()) { fn() })
	d.dev = dev
	return d, mgr, dev
}

// announce plays the compositor introducing an offer with mimes and
// naming it sel's content.
func announce(d *Device, sel Selection, o *fakeOffer, mimes ...string) {
	d.dataOffer(o)
	for _, m := range mimes {
		d.offerMime(o, m)
	}
	d.selection(sel, o)
}

func textSource(payload string, mimes ...string) Source {
	return Source{Mimes: mimes, Data: func(string) []byte { return []byte(payload) }}
}

func TestSelectionCarriesTheOwnersMimesInOrder(t *testing.T) {
	d, _, _ := newTestDevice(true)
	var got []*Offer
	var sels []Selection
	d.OnSelection(func(sel Selection, o *Offer) {
		sels = append(sels, sel)
		got = append(got, o)
	})

	announce(d, Clipboard, &fakeOffer{}, "text/uri-list", "text/plain", "text/uri-list", "image/png")

	if len(got) != 1 || sels[0] != Clipboard {
		t.Fatalf("handler calls = %v", sels)
	}
	want := []string{"text/uri-list", "text/plain", "image/png"}
	if m := got[0].Mimes(); !slices.Equal(m, want) {
		t.Fatalf("mimes = %v, want %v (owner order, deduplicated)", m, want)
	}
	if d.Offer(Clipboard) != got[0] || d.Offer(Primary) != nil {
		t.Fatal("current offers do not track the selection event")
	}
	if got[0].Own() || !got[0].Live() || got[0].Selection() != Clipboard {
		t.Fatal("a foreign offer must be live, not own, and name its selection")
	}
}

func TestTheTwoSelectionsStayIndependent(t *testing.T) {
	d, _, _ := newTestDevice(true)
	clip, prim := &fakeOffer{}, &fakeOffer{}

	announce(d, Clipboard, clip, "text/plain")
	announce(d, Primary, prim, "UTF8_STRING")

	if !d.Offer(Clipboard).Has("text/plain") || d.Offer(Clipboard).Has("UTF8_STRING") {
		t.Fatal("clipboard offer picked up the primary's mimes")
	}
	if !d.Offer(Primary).Has("UTF8_STRING") {
		t.Fatal("primary offer lost its mime")
	}
	if clip.destroyed {
		t.Fatal("a primary change destroyed the clipboard offer")
	}
}

func TestReplacedAndOrphanedOffersAreDestroyed(t *testing.T) {
	d, _, _ := newTestDevice(true)
	first, orphan, second := &fakeOffer{}, &fakeOffer{}, &fakeOffer{}

	announce(d, Clipboard, first, "text/plain")
	old := d.Offer(Clipboard)
	d.dataOffer(orphan)
	announce(d, Clipboard, second, "text/plain")

	if !first.destroyed || !orphan.destroyed {
		t.Fatalf("replaced destroyed=%v orphan destroyed=%v", first.destroyed, orphan.destroyed)
	}
	if second.destroyed {
		t.Fatal("the current offer was destroyed")
	}
	if old.Live() {
		t.Fatal("a replaced offer still reads as live")
	}
	if _, err := old.Receive("text/plain"); !errors.Is(err, ErrStaleOffer) {
		t.Fatalf("stale receive err = %v, want ErrStaleOffer", err)
	}
}

func TestAClearedSelectionReportsNil(t *testing.T) {
	d, _, _ := newTestDevice(true)
	announce(d, Clipboard, &fakeOffer{}, "text/plain")
	var cleared bool
	d.OnSelection(func(sel Selection, o *Offer) { cleared = sel == Clipboard && o == nil })

	d.selection(Clipboard, nil)

	if !cleared || d.Offer(Clipboard) != nil {
		t.Fatal("clearing the selection was not reported as a nil offer")
	}
}

func TestRemovedHandlersStopFiring(t *testing.T) {
	d, _, _ := newTestDevice(true)
	calls := 0
	remove := d.OnSelection(func(Selection, *Offer) { calls++ })

	announce(d, Clipboard, &fakeOffer{}, "text/plain")
	remove()
	announce(d, Clipboard, &fakeOffer{}, "text/plain")

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

// readSync runs Offer.Read and waits for its done.
func readSync(t *testing.T, o *Offer, mime string, limit int64) ([]byte, error) {
	t.Helper()
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	if err := o.Read(mime, limit, func(b []byte, err error) { ch <- result{b, err} }); err != nil {
		t.Fatalf("Read: %v", err)
	}
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(10 * time.Second):
		t.Fatal("Read never delivered")
		return nil, nil
	}
}

func writer(payload []byte) func(*os.File) {
	return func(w *os.File) {
		_, _ = w.Write(payload)
		_ = w.Close()
	}
}

func TestReadDeliversTheOwnersBytesUnderTheAskedMime(t *testing.T) {
	d, _, _ := newTestDevice(true)
	owner := &fakeOffer{write: writer([]byte("\x89PNG data"))}
	announce(d, Clipboard, owner, "text/plain", "image/png")

	data, err := readSync(t, d.Offer(Clipboard), "image/png", 1<<20)
	if err != nil || string(data) != "\x89PNG data" {
		t.Fatalf("read = %q, %v", data, err)
	}
	if !slices.Equal(owner.received, []string{"image/png"}) {
		t.Fatalf("receive requests = %v", owner.received)
	}
}

func TestReadRefusesAnOversizedPayloadWhole(t *testing.T) {
	d, _, _ := newTestDevice(true)
	announce(d, Clipboard, &fakeOffer{write: writer(make([]byte, 64))}, "text/plain")

	data, err := readSync(t, d.Offer(Clipboard), "text/plain", 16)
	if !errors.Is(err, xfer.ErrTooLarge) || data != nil {
		t.Fatalf("read = %d bytes, %v; want nothing and ErrTooLarge", len(data), err)
	}

	// Exactly at the limit is fine.
	announce(d, Clipboard, &fakeOffer{write: writer(make([]byte, 16))}, "text/plain")
	data, err = readSync(t, d.Offer(Clipboard), "text/plain", 16)
	if err != nil || len(data) != 16 {
		t.Fatalf("read at the limit = %d bytes, %v", len(data), err)
	}
}

func TestReadCutsOffAStalledOwner(t *testing.T) {
	old := transferTimeout
	transferTimeout = 50 * time.Millisecond
	t.Cleanup(func() { transferTimeout = old })
	d, _, _ := newTestDevice(true)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	announce(d, Clipboard, &fakeOffer{write: func(w *os.File) {
		<-release
		_ = w.Close()
	}}, "text/plain")

	_, err := readSync(t, d.Offer(Clipboard), "text/plain", 1024)
	if !errors.Is(err, xfer.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestReadRejectsWhatCannotBeNegotiated(t *testing.T) {
	d, _, _ := newTestDevice(true)
	owner := &fakeOffer{}
	announce(d, Clipboard, owner, "text/plain")
	o := d.Offer(Clipboard)
	done := func([]byte, error) { t.Error("done must not run for a rejected read") }

	if err := o.Read("image/png", 1024, done); !errors.Is(err, ErrMimeNotOffered) {
		t.Fatalf("unoffered mime err = %v", err)
	}
	if err := o.Read("text/plain", 0, done); err == nil {
		t.Fatal("a zero limit was accepted")
	}
	if err := o.Read("text/plain", 1024, nil); err == nil {
		t.Fatal("a nil done was accepted")
	}
	if len(owner.received) != 0 {
		t.Fatalf("rejected reads still asked the owner: %v", owner.received)
	}
}

func TestSetSelectionOffersTheSourceInOrder(t *testing.T) {
	d, mgr, dev := newTestDevice(true)

	c, err := d.SetSelection(Clipboard, textSource("hi", "text/plain;charset=utf-8", "STRING"))
	if err != nil {
		t.Fatal(err)
	}
	if len(mgr.sources) != 1 || !slices.Equal(mgr.sources[0].offered, []string{"text/plain;charset=utf-8", "STRING"}) {
		t.Fatalf("offered = %+v", mgr.sources)
	}
	if len(dev.selections) != 1 || dev.selections[0] != mgr.sources[0] || len(dev.primaries) != 0 {
		t.Fatal("the source was not set as the regular selection")
	}
	if !c.Live() || c.Selection() != Clipboard {
		t.Fatal("a fresh claim is not live")
	}

	if _, err := d.SetSelection(Primary, textSource("hi", "text/plain")); err != nil {
		t.Fatal(err)
	}
	if len(dev.primaries) != 1 {
		t.Fatal("a primary claim did not use set_primary_selection")
	}
}

func TestSetSelectionRejectsInvalidSources(t *testing.T) {
	d, mgr, _ := newTestDevice(true)
	cases := map[string]Source{
		"no mimes":   textSource("x"),
		"empty mime": textSource("x", "text/plain", ""),
		"duplicate":  textSource("x", "text/plain", "text/plain"),
		"no data":    {Mimes: []string{"text/plain"}},
		"both ways": {
			Mimes: []string{"text/plain"},
			Data:  func(string) []byte { return nil },
			Send:  func(_ string, w io.WriteCloser) { _ = w.Close() },
		},
		"send and pass": {
			Mimes: []string{"text/plain"},
			Send:  func(_ string, w io.WriteCloser) { _ = w.Close() },
			Pass:  func(_ string, f *os.File) { _ = f.Close() },
		},
		"data and pass": {
			Mimes: []string{"text/plain"},
			Data:  func(string) []byte { return nil },
			Pass:  func(_ string, f *os.File) { _ = f.Close() },
		},
	}
	for name, src := range cases {
		if _, err := d.SetSelection(Clipboard, src); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := d.SetSelection(Selection(9), textSource("x", "text/plain")); err == nil {
		t.Error("an unknown selection was accepted")
	}
	if len(mgr.sources) != 0 {
		t.Fatalf("invalid sources still created %d wire sources", len(mgr.sources))
	}
}

func TestPrimaryNeedsAProtocolThatCarriesIt(t *testing.T) {
	d, mgr, dev := newTestDevice(false)

	if d.PrimaryAvailable() {
		t.Fatal("PrimaryAvailable on a device without it")
	}
	if _, err := d.SetSelection(Primary, textSource("x", "text/plain")); !errors.Is(err, ErrPrimaryUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err := d.ClearSelection(Primary); !errors.Is(err, ErrPrimaryUnavailable) {
		t.Fatalf("clear err = %v", err)
	}
	if len(mgr.sources) != 0 || len(dev.primaries) != 0 {
		t.Fatal("a refused primary claim still reached the wire")
	}
	// The regular selection is unaffected.
	if _, err := d.SetSelection(Clipboard, textSource("x", "text/plain")); err != nil {
		t.Fatal(err)
	}
}

func TestOurOwnClaimComesBackMarkedOwn(t *testing.T) {
	d, mgr, _ := newTestDevice(true)
	if _, err := d.SetSelection(Clipboard, textSource("x", "text/plain")); err != nil {
		t.Fatal(err)
	}

	announce(d, Clipboard, &fakeOffer{}, "text/plain")
	if !d.Offer(Clipboard).Own() {
		t.Fatal("our own claim's offer is not marked Own")
	}

	// Another client copies: our source is cancelled, then its offer
	// arrives — which is foreign.
	d.cancelled(mgr.sources[0])
	announce(d, Clipboard, &fakeOffer{}, "text/plain")
	if d.Offer(Clipboard).Own() {
		t.Fatal("a foreign offer after our cancel is marked Own")
	}
	if !mgr.sources[0].destroyed {
		t.Fatal("a cancelled source was not destroyed")
	}
}

func TestAPrimaryClaimDoesNotMarkClipboardOffersOwn(t *testing.T) {
	d, _, _ := newTestDevice(true)
	if _, err := d.SetSelection(Primary, textSource("x", "text/plain")); err != nil {
		t.Fatal(err)
	}

	announce(d, Clipboard, &fakeOffer{}, "text/plain")

	if d.Offer(Clipboard).Own() {
		t.Fatal("a primary claim leaked into the clipboard's Own")
	}
}

// sendSync plays a receiver asking for mime and returns what it got.
func sendSync(t *testing.T, d *Device, src wireSource, mime string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fd, err := syscall.Dup(int(w.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	d.send(src, mime, uintptr(fd), nil)
	_ = r.SetReadDeadline(time.Now().Add(10 * time.Second))
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("receiver read: %v", err)
	}
	return string(got)
}

func TestAClaimServesItsPayloadPerMime(t *testing.T) {
	d, mgr, _ := newTestDevice(true)
	var asked []string
	_, err := d.SetSelection(Clipboard, Source{
		Mimes: []string{"text/plain", "text/html"},
		Data: func(m string) []byte {
			asked = append(asked, m)
			return []byte("<" + m + ">")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := mgr.sources[0]

	if got := sendSync(t, d, src, "text/html"); got != "<text/html>" {
		t.Fatalf("served %q", got)
	}
	// A mime the claim never offered gets EOF, not another payload.
	if got := sendSync(t, d, src, "image/png"); got != "" {
		t.Fatalf("an unoffered mime was served %q", got)
	}
	if !slices.Equal(asked, []string{"text/html"}) {
		t.Fatalf("Data asked for %v", asked)
	}

	// Once cancelled, a late request gets EOF too.
	d.cancelled(src)
	if got := sendSync(t, d, src, "text/plain"); got != "" {
		t.Fatalf("a cancelled claim still served %q", got)
	}
}

func TestReplacingAClaimCancelsTheOldOne(t *testing.T) {
	d, mgr, _ := newTestDevice(true)
	first, err := d.SetSelection(Clipboard, textSource("a", "text/plain"))
	if err != nil {
		t.Fatal(err)
	}
	cancelled := 0
	first.OnCancelled(func() { cancelled++ })

	second, err := d.SetSelection(Clipboard, textSource("b", "text/plain"))
	if err != nil {
		t.Fatal(err)
	}

	if first.Live() || cancelled != 1 || !mgr.sources[0].destroyed {
		t.Fatalf("old claim live=%v cancelled=%d destroyed=%v", first.Live(), cancelled, mgr.sources[0].destroyed)
	}
	if !second.Live() || mgr.sources[1].destroyed {
		t.Fatal("the new claim is not live")
	}
	// A late cancel of the old source cannot touch the new claim.
	d.cancelled(mgr.sources[0])
	if !second.Live() || cancelled != 1 {
		t.Fatal("a late cancel of the replaced source leaked")
	}
	// OnCancelled on an ended claim runs at once.
	ran := false
	first.OnCancelled(func() { ran = true })
	if !ran {
		t.Fatal("OnCancelled on an ended claim did not run")
	}
}

func TestReleaseGivesUpQuietly(t *testing.T) {
	d, mgr, _ := newTestDevice(true)
	c, err := d.SetSelection(Clipboard, textSource("a", "text/plain"))
	if err != nil {
		t.Fatal(err)
	}
	c.OnCancelled(func() { t.Error("Release ran OnCancelled") })

	c.Release()
	c.Release()

	if c.Live() || !mgr.sources[0].destroyed {
		t.Fatal("Release left the claim live")
	}
	announce(d, Clipboard, &fakeOffer{}, "text/plain")
	if d.Offer(Clipboard).Own() {
		t.Fatal("an offer after Release is marked Own")
	}
}

func TestClearSelectionSendsANullSource(t *testing.T) {
	d, _, dev := newTestDevice(true)
	c, err := d.SetSelection(Clipboard, textSource("a", "text/plain"))
	if err != nil {
		t.Fatal(err)
	}

	if err := d.ClearSelection(Clipboard); err != nil {
		t.Fatal(err)
	}

	if last := dev.selections[len(dev.selections)-1]; last != nil {
		t.Fatalf("clear sent %v, want a null source", last)
	}
	if c.Live() {
		t.Fatal("clearing left our claim live")
	}
}

func TestFinishedRetiresEverything(t *testing.T) {
	d, _, dev := newTestDevice(true)
	announce(d, Clipboard, &fakeOffer{}, "text/plain")
	announce(d, Primary, &fakeOffer{}, "text/plain")
	c, err := d.SetSelection(Clipboard, textSource("a", "text/plain"))
	if err != nil {
		t.Fatal(err)
	}
	cancelled := false
	c.OnCancelled(func() { cancelled = true })
	var cleared []Selection
	d.OnSelection(func(sel Selection, o *Offer) {
		if o == nil {
			cleared = append(cleared, sel)
		}
	})

	d.finish()

	if !slices.Equal(cleared, []Selection{Clipboard, Primary}) {
		t.Fatalf("cleared = %v", cleared)
	}
	if !cancelled || c.Live() || !dev.destroyed || !d.Finished() || d.PrimaryAvailable() {
		t.Fatal("finish left state behind")
	}
	if _, err := d.SetSelection(Clipboard, textSource("a", "text/plain")); !errors.Is(err, ErrFinished) {
		t.Fatalf("SetSelection after finish err = %v", err)
	}
}

func TestSourceCreationFailureReachesNoWire(t *testing.T) {
	d, mgr, dev := newTestDevice(true)
	mgr.fail = errors.New("boom")

	if _, err := d.SetSelection(Clipboard, textSource("a", "text/plain")); err == nil {
		t.Fatal("a failed source creation was not reported")
	}
	if len(dev.selections) != 0 {
		t.Fatal("a failed claim still set the selection")
	}
}

func TestNames(t *testing.T) {
	if Clipboard.String() != "clipboard" || Primary.String() != "primary" || Selection(7).String() != "Selection(7)" {
		t.Fatal("selection names")
	}
	if ProtocolExt.String() != "ext-data-control-v1" || ProtocolWlr.String() != "wlr-data-control-unstable-v1" {
		t.Fatal("protocol names")
	}
}

func TestASendSourceGetsTheReceiversPipe(t *testing.T) {
	d, mgr, _ := newTestDevice(true)
	var asked []string
	_, err := d.SetSelection(Clipboard, Source{
		Mimes: []string{"text/plain"},
		Send: func(m string, w io.WriteCloser) {
			asked = append(asked, m)
			// Answered later, from elsewhere, as a bridge would.
			go func() {
				_, _ = w.Write([]byte("bridged"))
				_ = w.Close()
			}()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := sendSync(t, d, mgr.sources[0], "text/plain"); got != "bridged" {
		t.Fatalf("served %q", got)
	}
	// An unoffered mime never reaches Send.
	if got := sendSync(t, d, mgr.sources[0], "image/png"); got != "" || len(asked) != 1 {
		t.Fatalf("unoffered mime served %q, Send asked %v", got, asked)
	}
}

func TestAPassSourceGetsTheRawPipe(t *testing.T) {
	d, mgr, _ := newTestDevice(true)
	var asked []string
	var blocking []bool
	_, err := d.SetSelection(Clipboard, Source{
		Mimes: []string{"text/plain"},
		Pass: func(m string, pipe *os.File) {
			asked = append(asked, m)
			// Untouched: the descriptor another process gets is blocking,
			// as the compositor made it, not the deadline writer's.
			flags, err := unix.FcntlInt(pipe.Fd(), unix.F_GETFL, 0)
			blocking = append(blocking, err == nil && flags&unix.O_NONBLOCK == 0)
			_, _ = pipe.Write([]byte("passed"))
			_ = pipe.Close()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sendSync(t, d, mgr.sources[0], "text/plain"); got != "passed" {
		t.Fatalf("served %q", got)
	}
	if !slices.Equal(blocking, []bool{true}) {
		t.Errorf("the passed pipe was made non-blocking: %v", blocking)
	}
	// An unoffered mime never reaches Pass.
	if got := sendSync(t, d, mgr.sources[0], "image/png"); got != "" || len(asked) != 1 {
		t.Fatalf("unoffered mime served %q, Pass asked %v", got, asked)
	}
}

// ReadTo streams the owner's bytes into a writer the caller owns, and
// over the cap leaves the first limit bytes there with ErrTooLarge.
func TestReadToStreamsIntoTheCallersWriter(t *testing.T) {
	d, _, _ := newTestDevice(true)
	readTo := func(limit int64, into *bytes.Buffer) (int64, error) {
		t.Helper()
		type result struct {
			n   int64
			err error
		}
		ch := make(chan result, 1)
		if err := d.Offer(Clipboard).ReadTo("text/plain", limit, into, func(n int64, err error) { ch <- result{n, err} }); err != nil {
			t.Fatalf("ReadTo: %v", err)
		}
		select {
		case r := <-ch:
			return r.n, r.err
		case <-time.After(10 * time.Second):
			t.Fatal("ReadTo never delivered")
			return 0, nil
		}
	}
	announce(d, Clipboard, &fakeOffer{write: writer([]byte("hello world"))}, "text/plain")
	var into bytes.Buffer
	if n, err := readTo(64, &into); err != nil || n != 11 || into.String() != "hello world" {
		t.Errorf("read %d bytes %q (%v), want 11 bytes of hello world", n, into.String(), err)
	}

	announce(d, Clipboard, &fakeOffer{write: writer([]byte("0123456789abcdef"))}, "text/plain")
	into.Reset()
	if n, err := readTo(4, &into); !errors.Is(err, xfer.ErrTooLarge) || n != 4 || into.String() != "0123" {
		t.Errorf("an oversized read left %d bytes %q (%v), want the first 4 and ErrTooLarge", n, into.String(), err)
	}
	if err := d.Offer(Clipboard).ReadTo("text/plain", 4, nil, func(int64, error) {}); err == nil {
		t.Error("ReadTo without a writer started a transfer")
	}
}
