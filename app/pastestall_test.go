package app_test

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/transfer"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	dataDeviceManagerGetDevice = 1
	dataDeviceEventDataOffer   = 0
	dataDeviceEventSelection   = 5
	dataOfferReceive           = 1
	dataOfferEventOffer        = 0
	serverOfferID              = 0xff000000
	pendingTicks               = 50
)

// selectionPeer plays the foreign client holding the selection, through
// the fake compositor: it announces one offer and keeps the write end a
// paste hands over, writing only when the test says.
type selectionPeer struct {
	f       *fakeCompositor
	device  atomic.Uint32
	mu      sync.Mutex
	writeTo []int
	gotFd   chan struct{}
}

func newSelectionPeer(t *testing.T) (*selectionPeer, *app.Application, *app.Clipboard) {
	t.Helper()
	globals := slices.Concat(kioskGlobals, []fakeGlobal{{"wl_seat", 7}, {"wl_data_device_manager", 3}})
	p := &selectionPeer{gotFd: make(chan struct{}, 1)}
	_, a := connectFakeWith(t, globals, func(f *fakeCompositor) {
		p.f = f
		f.onRequest = p.request
		f.onFds = p.fds
	})
	t.Cleanup(p.closeFds)
	if p.device.Load() == 0 {
		t.Fatal("the session never asked for a data device")
	}
	clip := app.NewClipboard(a.Session())
	a.SetClipboard(clip)
	return p, a, clip
}

func (p *selectionPeer) request(iface string, _, opcode uint32, body []byte) {
	if iface == "wl_data_device_manager" && opcode == dataDeviceManagerGetDevice {
		p.device.Store(word(body, 0))
	}
}

func (p *selectionPeer) fds(fds []int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeTo = append(p.writeTo, fds...)
	select {
	case p.gotFd <- struct{}{}:
	default:
	}
	return true
}

func (p *selectionPeer) closeFds() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, fd := range p.writeTo {
		_ = unix.Close(fd)
	}
	p.writeTo = nil
}

// announce makes a text offer the selection.
func (p *selectionPeer) announce() {
	device := p.device.Load()
	p.f.send(device, dataDeviceEventDataOffer, serverOfferID)
	p.f.sendString(serverOfferID, dataOfferEventOffer, transfer.MimeText)
	p.f.send(device, dataDeviceEventSelection, serverOfferID)
}

// write delivers payload to every held write end and closes them.
func (p *selectionPeer) write(t *testing.T, payload string) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, fd := range p.writeTo {
		if _, err := unix.Write(fd, []byte(payload)); err != nil {
			t.Errorf("the peer's write: %v", err)
		}
		_ = unix.Close(fd)
	}
	p.writeTo = nil
}

func (p *selectionPeer) awaitFd(t *testing.T) {
	t.Helper()
	select {
	case <-p.gotFd:
	case <-time.After(10 * time.Second):
		t.Fatal("the paste never handed the compositor a pipe")
	}
}

// loopWatch runs on the loop through Every: it counts passes and starts
// fn once ready holds.
type loopWatch struct {
	ticks   atomic.Int64
	reached chan struct{}
	want    atomic.Int64
}

func watchLoop(a *app.Application) *loopWatch {
	w := &loopWatch{reached: make(chan struct{}, 1)}
	a.Every(time.Millisecond, func() {
		if n := w.ticks.Add(1); w.want.Load() != 0 && n >= w.want.Load() {
			select {
			case w.reached <- struct{}{}:
			default:
			}
		}
	})
	return w
}

// await bounds the wait for n more loop passes; a loop blocked on the
// peer fails it rather than the test's timeout.
func (w *loopWatch) await(t *testing.T, n int64) {
	t.Helper()
	w.want.Store(w.ticks.Load() + n)
	select {
	case <-w.reached:
	case <-time.After(10 * time.Second):
		t.Fatalf("the loop made %d passes with a paste pending, want %d more", w.ticks.Load(), n)
	}
}

func awaitOffer(t *testing.T, a *app.Application, clip *app.Clipboard) {
	t.Helper()
	offered := make(chan struct{})
	var once sync.Once
	stop := a.Every(time.Millisecond, func() {
		if clip.Offers(transfer.MimeText) {
			once.Do(func() { close(offered) })
		}
	})
	defer stop()
	select {
	case <-offered:
	case <-time.After(10 * time.Second):
		t.Fatal("the selection never reached the clipboard")
	}
}

// A paste from a peer that never writes costs the loop no roundtrip and
// no wait: it keeps making passes with the paste pending, and the payload
// lands when the peer finally answers.
func TestPasteFromASilentPeerLeavesTheLoopRunning(t *testing.T) {
	p, a, clip := newSelectionPeer(t)
	a.Hold()
	watch := watchLoop(a)
	runUntilCleanup(t, a)
	p.announce()
	awaitOffer(t, a, clip)

	syncsBefore := p.f.syncs.Load()
	delivered := make(chan string, 1)
	a.Invoke(func() {
		clip.ReadText(func(text string, err error) {
			if err != nil {
				t.Errorf("paste: %v", err)
			}
			delivered <- text
		})
	})
	p.awaitFd(t)
	watch.await(t, pendingTicks)
	select {
	case text := <-delivered:
		t.Fatalf("a peer that never wrote delivered %q", text)
	default:
	}
	if got := p.f.syncs.Load() - syncsBefore; got != 0 {
		t.Errorf("the paste cost %d wl_display.sync, want 0", got)
	}

	p.write(t, "late payload")
	select {
	case text := <-delivered:
		if text != "late payload" {
			t.Errorf("paste = %q, want the peer's payload", text)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the payload never landed")
	}
}

// A pending ctrl+v lands in the widget that held focus at the press, even
// after focus moved and the user typed on, and nothing the wait let
// through is lost.
func TestPendingPasteLandsWhereItWasAsked(t *testing.T) {
	p, a, clip := newSelectionPeer(t)
	a.Hold()
	watch := watchLoop(a)
	runUntilCleanup(t, a)
	p.announce()
	awaitOffer(t, a, clip)

	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	asked := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	other := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	root := widget.NewBox(widget.Row, 0, 0)
	root.Append(asked, false)
	root.Append(other, false)
	root.Arrange(render.Rect{W: 400, H: 30})
	router := &widget.Router{Root: root}
	done := make(chan struct{})
	a.Invoke(func() {
		defer close(done)
		if err := router.SetFocus(asked); err != nil {
			t.Error(err)
		}
		a.PasteInto(router)
	})
	<-done
	p.awaitFd(t)

	moved := make(chan struct{})
	a.Invoke(func() {
		defer close(moved)
		asked.Insert("typed ")
		if err := router.SetFocus(other); err != nil {
			t.Error(err)
		}
	})
	<-moved
	watch.await(t, pendingTicks)

	p.write(t, "pasted")
	got := make(chan [2]string, 1)
	stop := a.Every(time.Millisecond, func() {
		if asked.Text() != "typed " {
			select {
			case got <- [2]string{asked.Text(), other.Text()}:
			default:
			}
		}
	})
	defer stop()
	select {
	case texts := <-got:
		if texts != [2]string{"typed pasted", ""} {
			t.Errorf("asked %q, other %q; want the paste after what was typed, in the entry it was asked of", texts[0], texts[1])
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the paste never landed")
	}
}

// The client's own copy of the write end is closed once the request is
// out: with the compositor's copy gone too, the read ends at EOF rather
// than at its deadline, which would report a stalled peer.
func TestPasteClosesItsWriteEnd(t *testing.T) {
	p, a, clip := newSelectionPeer(t)
	a.Hold()
	runUntilCleanup(t, a)
	p.announce()
	awaitOffer(t, a, clip)
	delivered := make(chan error, 1)
	a.Invoke(func() { clip.ReadText(func(_ string, err error) { delivered <- err }) })
	p.awaitFd(t)
	p.closeFds()
	select {
	case err := <-delivered:
		if err != nil {
			t.Errorf("an empty transfer ended with %v, want EOF", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the read never ended")
	}
}
