package app

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// fakePopoverHost is a wire-free Host for registry tests.
type fakePopoverHost struct {
	id   int
	surf wl.Surface
}

func (f *fakePopoverHost) EnsureUsable() error      { return nil }
func (f *fakePopoverHost) Closed() bool             { return false }
func (f *fakePopoverHost) Size() (int, int)         { return 400, 300 }
func (f *fakePopoverHost) HostSurface() *wl.Surface { return &f.surf }

// TestPopoverRegistry pins the re-anchor rule: opening a second
// popover on the same host closes the first; a different host is
// unaffected.
func TestPopoverRegistry(t *testing.T) {
	var reg popoverRegistry
	h1 := &fakePopoverHost{id: 1}

	h2 := &fakePopoverHost{id: 2}

	p1 := &Popover{}
	if prev := reg.openOrReplace(h1, p1); prev != nil {
		t.Error("first open replaced something")
	}

	p2 := &Popover{}
	if prev := reg.openOrReplace(h1, p2); prev != p1 {
		t.Errorf("re-anchor returned %v, want the first popover", prev)
	}

	p3 := &Popover{}
	if prev := reg.openOrReplace(h2, p3); prev != nil {
		t.Error("a different host should not replace")
	}

	if got := reg.take(h2); got != p3 {
		t.Error("take returned the wrong popover")
	}
	if got := reg.take(h2); got != nil {
		t.Error("take on an empty host returned a popover")
	}
}

// TestPopoverKeyRoot pins Esc dismissal and forwarding of other keys
// to the content.
func TestPopoverKeyRoot(t *testing.T) {
	dismissed := 0
	actions := []widget.KeyAction{}
	content := &keySpy{onAction: func(a widget.KeyAction, m widget.Mods) {
		actions = append(actions, a)
	}}
	root := &popoverKeyRoot{onDismiss: func() { dismissed++ }, content: content}

	root.KeyAction(widget.KeyDismiss, 0)
	if dismissed != 1 {
		t.Errorf("dismissals = %d, want 1", dismissed)
	}
	root.KeyAction(widget.KeyDown, 0)
	if len(actions) != 1 || actions[0] != widget.KeyDown {
		t.Errorf("forwarded actions = %v, want [KeyDown]", actions)
	}
}

// keySpy records forwarded KeyActions.
type keySpy struct {
	onAction func(widget.KeyAction, widget.Mods)
}

func (k *keySpy) Measure(widget.Axis, int) widget.Sizes { return widget.Sizes{} }
func (k *keySpy) Arrange(render.Rect)                   {}
func (k *keySpy) Paint(*render.Canvas)                  {}
func (k *keySpy) HitTest(widget.Point) widget.Widget    { return nil }
func (k *keySpy) KeyAction(a widget.KeyAction, m widget.Mods) {
	k.onAction(a, m)
}

// A toplevel Window carries popups from its xdg surface, a layer from
// its layer surface; anything else cannot.
func TestPopupParentOf(t *testing.T) {
	surface := &xdg.Surface{}
	if p, l, ok := popupParentOf(&Window{win: &window.Window{XdgSurface: surface}}); !ok || p != surface || l != nil {
		t.Errorf("a Window: parent %v layer %v ok %v, want its xdg surface", p, l, ok)
	}
	if _, _, ok := popupParentOf(&Window{}); ok {
		t.Error("a Window without a surface carried a popup")
	}
	if _, _, ok := popupParentOf(nil); ok {
		t.Error("no host carried a popup")
	}
}

// A grabbing popover holds the bar's keyboard exclusively; a sticky
// one only on demand, leaving other
// windows their keys.
func TestPopoverKeyboardFollowsDismissal(t *testing.T) {
	if got := popoverKeyboard(PopoverConfig{}); got != KeyboardExclusive {
		t.Errorf("autohide popover holds %v, want exclusive", got)
	}
	if got := popoverKeyboard(PopoverConfig{Dismissal: PopoverSticky}); got != KeyboardOnDemand {
		t.Errorf("no-autohide popover holds %v, want on demand", got)
	}
}

func TestPressOnTheHostDismissesAnAutohidePopover(t *testing.T) {
	a := &Application{}
	host := &fakeHost{w: 100, h: 100}
	dismissed := 0
	pop := &Popover{host: host, autohide: true}
	pop.closeFn = func() {
		dismissed++
		pop.markClosed()
	}
	a.popovers.openOrReplace(host, pop)

	clicked := 0
	clickable := &clickCountingBox{onClick: func() { clicked++ }}
	clickable.Arrange(render.Rect{W: 100, H: 100})
	in := &surfaceInput{
		router:         &widget.Router{Root: clickable},
		request:        func() {},
		dismissPopover: func() bool { return a.dismissAutohide(host) },
	}
	in.HandlePointerEnter(50, 50)
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	if dismissed != 1 || clicked != 0 {
		t.Fatalf("dismissed=%d clicked=%d, want the press to close the popover and go nowhere else", dismissed, clicked)
	}
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	if dismissed != 1 || clicked != 1 {
		t.Errorf("with the popover closed: dismissed=%d clicked=%d, want the press delivered", dismissed, clicked)
	}

	sticky := &Popover{host: host}
	sticky.closeFn = func() { t.Error("a sticky popover closed on a press") }
	a.popovers.openOrReplace(host, sticky)
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	if clicked != 2 {
		t.Errorf("a press beside a sticky popover was swallowed")
	}
}

// A click-away popover leaves the keys with the focused window unless a
// Focus widget must have them as it opens.
func TestPopoverClickAwayKeyboard(t *testing.T) {
	if got := popoverKeyboard(PopoverConfig{Dismissal: PopoverClickAway}); got != KeyboardNone {
		t.Errorf("click-away popover holds %v as it opens, want none", got)
	}
	focus := widget.NewBox(widget.Row, 0, 0)
	if got := popoverKeyboard(PopoverConfig{Dismissal: PopoverClickAway, Focus: focus}); got != KeyboardExclusive {
		t.Errorf("click-away popover with a Focus widget holds %v, want exclusive", got)
	}
}

// A popover without the grab holds focus only on its own chain.
func TestHoldsFocusIsTheChainsSurfaces(t *testing.T) {
	var other wl.Surface
	host := &fakeHost{}
	hostSurf := host.HostSurface()
	root := &Popover{host: host}
	child := &Popover{host: host, parent: root}
	if root.holdsFocus(nil) {
		t.Error("no focus counted as held")
	}
	if !root.holdsFocus(hostSurf) || !child.holdsFocus(hostSurf) {
		t.Error("the host's focus did not count")
	}
	if root.holdsFocus(&other) || child.holdsFocus(&other) {
		t.Error("another surface's focus counted")
	}
}

// keyPopover hands keys to a grabbing popover whatever the focus, and to
// one without the grab only while it holds focus.
func TestKeyPopoverSkipsAnUnfocusedNoGrabPopover(t *testing.T) {
	a := &Application{}
	noGrab := &openPopover{pop: &fakePopSurface{}, popover: &Popover{host: &fakeHost{}, noGrab: true}}
	a.openPopovers = []*openPopover{noGrab}
	if got := a.keyPopover(); got != nil {
		t.Errorf("an unfocused no-grab popover took keys: %v", got)
	}
	grab := &openPopover{pop: &fakePopSurface{}, popover: &Popover{}}
	a.openPopovers = append(a.openPopovers, grab)
	if got := a.keyPopover(); got != grab {
		t.Errorf("keyPopover = %v, want the grabbing popover", got)
	}
	grab.popover.exiting = true
	if got := a.keyPopover(); got != nil {
		t.Errorf("a popover on its way out took keys: %v", got)
	}
}

// fakeCatcher counts its destruction.
type fakeCatcher struct{ destroyed int }

func (f *fakeCatcher) destroy() { f.destroyed++ }

func fakeCatcherWire(made map[*wlsession.Output]*fakeCatcher, presses map[*wlsession.Output]func(), releases map[*wlsession.Output]func()) catcherWire {
	return func(out *wlsession.Output, press, release func()) (catcher, error) {
		c := &fakeCatcher{}
		made[out], presses[out], releases[out] = c, press, release
		return c, nil
	}
}

// noEscape is a compositor without lys_escape_v1.
func noEscape(func()) (func(), error) { return nil, nil }

// The Escape arm closes the popover like a press on a catcher, and goes
// the moment the close starts: a press beside an open popover fires
// once, and the wire is disarmed before any later one.
func TestClickAwayEscapeClosesAndDisarmsOnce(t *testing.T) {
	var pressed func()
	disarmed, fired := 0, 0
	escape := func(p func()) (func(), error) {
		pressed = p
		return func() { disarmed++ }, nil
	}
	made, presses, releases := map[*wlsession.Output]*fakeCatcher{}, map[*wlsession.Output]func(){}, map[*wlsession.Output]func(){}
	out := &wlsession.Output{}
	var c *clickAway
	c, err := newClickAway(fakeCatcherWire(made, presses, releases), []*wlsession.Output{out}, escape, func() {
		fired++
		c.close()
	})
	if err != nil {
		t.Fatal(err)
	}
	if disarmed != 0 {
		t.Fatalf("disarmed %d times while open", disarmed)
	}
	pressed()
	if fired != 1 || disarmed != 1 {
		t.Fatalf("after Escape: fired %d, disarmed %d, want 1 and 1", fired, disarmed)
	}
	if made[out].destroyed != 1 {
		t.Errorf("after Escape: catcher destroyed %d, want 1", made[out].destroyed)
	}
	c.close()
	if disarmed != 1 {
		t.Errorf("a second close disarmed again: %d", disarmed)
	}
}

// A press on a catcher closes it through the same path, so the arm goes
// with it.
func TestClickAwayPressDisarmsEscape(t *testing.T) {
	disarmed := 0
	escape := func(func()) (func(), error) { return func() { disarmed++ }, nil }
	made, presses, releases := map[*wlsession.Output]*fakeCatcher{}, map[*wlsession.Output]func(){}, map[*wlsession.Output]func(){}
	out := &wlsession.Output{}
	var c *clickAway
	c, err := newClickAway(fakeCatcherWire(made, presses, releases), []*wlsession.Output{out}, escape, func() { c.close() })
	if err != nil {
		t.Fatal(err)
	}
	presses[out]()
	if disarmed != 1 {
		t.Errorf("disarmed %d times, want 1", disarmed)
	}
}

// A failed arm fails the open and leaves no catcher behind.
func TestClickAwayEscapeFailureDropsTheCatchers(t *testing.T) {
	boom := errors.New("boom")
	made, presses, releases := map[*wlsession.Output]*fakeCatcher{}, map[*wlsession.Output]func(){}, map[*wlsession.Output]func(){}
	out := &wlsession.Output{}
	_, err := newClickAway(fakeCatcherWire(made, presses, releases), []*wlsession.Output{out}, func(func()) (func(), error) { return nil, boom }, func() {})
	if !errors.Is(err, boom) {
		t.Fatalf("newClickAway = %v, want the arm's failure", err)
	}
	if made[out].destroyed != 1 {
		t.Errorf("catcher destroyed %d, want 1", made[out].destroyed)
	}
}

// A press on any output's catcher closes the popover once; the catcher
// that heard it stays up until the button is released, so the window
// beneath never sees a lone release; the others go at once.
func TestClickAwayHoldsTheCatcherUntilRelease(t *testing.T) {
	a, b := &wlsession.Output{}, &wlsession.Output{}
	made, presses, releases := map[*wlsession.Output]*fakeCatcher{}, map[*wlsession.Output]func(){}, map[*wlsession.Output]func(){}
	fired := 0
	var c *clickAway
	c, err := newClickAway(fakeCatcherWire(made, presses, releases), []*wlsession.Output{a, b}, noEscape, func() {
		fired++
		c.close()
	})
	if err != nil {
		t.Fatal(err)
	}
	presses[a]()
	if fired != 1 {
		t.Fatalf("fired %d, want 1", fired)
	}
	if made[a].destroyed != 0 || made[b].destroyed != 1 {
		t.Fatalf("mid-press: held destroyed %d, other %d", made[a].destroyed, made[b].destroyed)
	}
	presses[a]()
	if fired != 1 {
		t.Errorf("a second press fired again: %d", fired)
	}
	releases[a]()
	releases[a]()
	if made[a].destroyed != 1 {
		t.Errorf("after release: held destroyed %d, want 1", made[a].destroyed)
	}
}

// Closing with no press held drops every catcher, and outputs coming
// and going while open gain and lose theirs.
func TestClickAwayFollowsOutputs(t *testing.T) {
	a, b := &wlsession.Output{}, &wlsession.Output{}
	made, presses, releases := map[*wlsession.Output]*fakeCatcher{}, map[*wlsession.Output]func(){}, map[*wlsession.Output]func(){}
	c, err := newClickAway(fakeCatcherWire(made, presses, releases), []*wlsession.Output{a}, noEscape, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.sync([]*wlsession.Output{a, b}); err != nil {
		t.Fatal(err)
	}
	if made[b] == nil {
		t.Fatal("the new output got no catcher")
	}
	if err := c.sync([]*wlsession.Output{b}); err != nil {
		t.Fatal(err)
	}
	if made[a].destroyed != 1 {
		t.Errorf("the gone output's catcher: destroyed %d, want 1", made[a].destroyed)
	}
	c.close()
	c.close()
	if made[b].destroyed != 1 {
		t.Errorf("after close: destroyed %d, want 1", made[b].destroyed)
	}
	if err := c.sync([]*wlsession.Output{a}); err != nil || len(made) != 2 {
		t.Errorf("sync after close armed a catcher: err=%v made=%d", err, len(made))
	}
}
