package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/window"
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

func (k *keySpy) Measure(widget.Constraints) widget.Size { return widget.Size{} }
func (k *keySpy) Arrange(render.Rect)                    {}
func (k *keySpy) Paint(*render.Canvas)                   {}
func (k *keySpy) HitTest(widget.Point) widget.Widget     { return nil }
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

// An autohide popover holds the bar's keyboard exclusively; a
// NoAutohide one (GTK autohide=false) only on demand, leaving other
// windows their keys.
func TestPopoverKeyboardFollowsAutohide(t *testing.T) {
	if got := popoverKeyboard(PopoverConfig{}); got != KeyboardExclusive {
		t.Errorf("autohide popover holds %v, want exclusive", got)
	}
	if got := popoverKeyboard(PopoverConfig{NoAutohide: true}); got != KeyboardOnDemand {
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
	sticky.closeFn = func() { t.Error("a NoAutohide popover closed on a press") }
	a.popovers.openOrReplace(host, sticky)
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	if clicked != 2 {
		t.Errorf("a press beside a NoAutohide popover was swallowed")
	}
}
