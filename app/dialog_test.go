package app

import (
	"testing"

	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

// TestDialogRespondOnce pins the once-guard: the first Respond fires
// OnResponse and closes the dialog; further Responds are ignored.
func TestDialogRespondOnce(t *testing.T) {
	app := &Application{}
	responses := 0
	d := &Dialog{
		app: app,
		win: &Window{app: app},
		cfg: DialogConfig{OnResponse: func(string) { responses++ }},
	}
	d.Respond("ok")
	d.Respond("ok")
	if responses != 1 {
		t.Errorf("responses = %d after two Responds, want 1", responses)
	}
	if !d.Closed() {
		t.Error("dialog did not close after responding")
	}
}

// TestDialogResponseForKey pins the key mapping: Esc is the cancel
// response, Enter the default, and an empty mapping disables the key.
func TestDialogResponseForKey(t *testing.T) {
	cfg := DialogConfig{DefaultResponse: "ok", CancelResponse: "cancel"}

	if resp, ok := dialogResponseForKey(cfg, xkb.KeyEscape); !ok || resp != "cancel" {
		t.Errorf("escape = %q %v, want cancel", resp, ok)
	}
	if resp, ok := dialogResponseForKey(cfg, xkb.KeyReturn); !ok || resp != "ok" {
		t.Errorf("return = %q %v, want ok", resp, ok)
	}
	if resp, ok := dialogResponseForKey(cfg, xkb.KeyKPEnter); !ok || resp != "ok" {
		t.Errorf("kp-enter = %q %v, want ok", resp, ok)
	}

	empty := DialogConfig{}
	if _, ok := dialogResponseForKey(empty, xkb.KeyEscape); ok {
		t.Error("escape fired with no cancel response configured")
	}
	if _, ok := dialogResponseForKey(empty, xkb.KeyReturn); ok {
		t.Error("return fired with no default response configured")
	}
	if _, ok := dialogResponseForKey(cfg, xkb.KeyLeft); ok {
		t.Error("an unrelated key produced a dialog response")
	}
}

// TestMessageBoxDefaults pins the preset button mapping: the first
// button is the default response, the last the cancel.
func TestMessageBoxDefaults(t *testing.T) {
	buttons := []DialogButton{
		{Label: "Delete", Response: "delete"},
		{Label: "Cancel", Response: "cancel"},
	}
	if got := defaultResponse(buttons); got != "delete" {
		t.Errorf("default = %q, want delete", got)
	}
	if got := cancelResponse(buttons); got != "cancel" {
		t.Errorf("cancel = %q, want cancel", got)
	}
}

// TestModalBlocksParentInput pins the modal rule: a blocked
// surfaceInput drops pointer presses, motion, and wheel, so the parent
// window is inert while a dialog is open.
func TestModalBlocksParentInput(t *testing.T) {
	clicked := 0
	clickable := &clickCountingBox{onClick: func() { clicked++ }}
	blocked := true

	in := &surfaceInput{
		router:  &widget.Router{Root: clickable},
		request: func() {},
		blocked: func() bool { return blocked },
	}

	clickable.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
	in.HandlePointerEnter(50, 50)
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	in.HandlePointerAxis(0, 80)
	if clicked != 0 {
		t.Errorf("blocked parent received %d clicks", clicked)
	}

	// Once the dialog closes, blocking lifts.
	blocked = false
	in.HandlePointerEnter(50, 50)
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	if clicked != 1 {
		t.Errorf("clicks after unblock = %d, want 1", clicked)
	}
}

// clickCountingBox is a Box that counts clicks.
type clickCountingBox struct {
	widget.Box
	onClick func()
	clicked bool
}

// HitTest returns the box itself so the outer Clicker is reachable.
func (c *clickCountingBox) HitTest(p widget.Point) widget.Widget {
	if c.Bounds().Contains(p.X, p.Y) {
		return c
	}
	return nil
}

// ClickAt implements Clicker.
func (c *clickCountingBox) ClickAt(p widget.Point) {
	c.clicked = true
	c.onClick()
}

// A bare dialog's window is its content alone, on the asked
// background: no toolkit card or button row around it. The default
// wraps the content and adds the buttons.
func TestDialogRootBare(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	a := &Application{tooltipFace: face}
	content := widget.NewBox(widget.Column, 0, 0)
	root, bg, err := a.dialogRoot(DialogConfig{Content: content, Bare: true, Background: render.RGB(1, 2, 3)}, func(string) {})
	if err != nil || root != widget.Widget(content) || bg != render.RGB(1, 2, 3) {
		t.Errorf("bare root %v bg %#x err %v, want the content on its background", root, uint32(bg), err)
	}
	var got []string
	root, _, err = a.dialogRoot(DialogConfig{Content: content}, func(r string) { got = append(got, r) })
	if err != nil || root == widget.Widget(content) {
		t.Fatalf("default root %v err %v, want a card around the content", root, err)
	}
	var ok *widget.Button
	var walk func(w widget.Widget)
	walk = func(w widget.Widget) {
		if b, isBtn := w.(*widget.Button); isBtn {
			ok = b
		}
		if c, has := w.(interface{ Children() []widget.Widget }); has {
			for _, k := range c.Children() {
				walk(k)
			}
		}
	}
	walk(root)
	if ok == nil {
		t.Fatal("the default dialog has no button")
	}
	ok.ClickAt(widget.Point{})
	if len(got) != 1 || got[0] != "ok" {
		t.Errorf("the default button responded %v, want ok", got)
	}
}

func TestDialogCloseSkipsTheResponse(t *testing.T) {
	app := &Application{}
	responses := 0
	d := &Dialog{app: app, win: &Window{app: app}, cfg: DialogConfig{OnResponse: func(string) { responses++ }}}
	d.Close()
	d.Respond("ok")
	if responses != 0 || !d.Closed() {
		t.Errorf("responses=%d closed=%v after Close, want 0 and closed", responses, d.Closed())
	}
}

// A dialog button's appearance picks its variant; the zero value
// keeps the default response suggested and the rest plain (#268).
func TestResponseAppearanceVariant(t *testing.T) {
	for _, tc := range []struct {
		a         ResponseAppearance
		isDefault bool
		want      string
	}{
		{AppearanceAuto, true, css.SuggestedAction},
		{AppearanceAuto, false, css.Outline},
		{AppearancePlain, true, css.Outline},
		{AppearanceSuggested, false, css.SuggestedAction},
		{AppearanceDestructive, true, css.DestructiveAction},
		{AppearanceDestructive, false, css.DestructiveAction},
	} {
		if got := tc.a.variant(tc.isDefault); got != tc.want {
			t.Errorf("appearance %d default %v = %q, want %q", tc.a, tc.isDefault, got, tc.want)
		}
	}
}

// The title and the description form the header, 8px apart, and the
// header stands 16px above the buttons: the description was the
// dialog's content, 16px under the title like any content (#281).
func TestDialogHeaderPairsTitleAndDescription(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	a := &Application{tooltipFace: face}
	root, _, err := a.dialogRoot(DialogConfig{Title: "Are you absolutely sure?", Description: "This action cannot be undone."}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	sz := root.Measure(widget.Constraints{Max: widget.Size{W: 440, H: 600}})
	root.Arrange(render.Rect{W: 440, H: sz.H})
	var labels []*widget.Label
	var footer widget.Widget
	var walk func(w widget.Widget)
	walk = func(w widget.Widget) {
		if l, ok := w.(*widget.Label); ok && (l.Text() == "Are you absolutely sure?" || l.Text() == "This action cannot be undone.") {
			labels = append(labels, l)
		}
		if b, ok := w.(*widget.Box); ok && widget.HasClass(b, "footer") {
			footer = b
		}
		if c, ok := w.(interface{ Children() []widget.Widget }); ok {
			for _, k := range c.Children() {
				walk(k)
			}
		}
	}
	walk(root)
	if len(labels) != 2 || footer == nil {
		t.Fatalf("found %d header labels and footer %v", len(labels), footer)
	}
	title, desc := labels[0].Bounds(), labels[1].Bounds()
	if gap := desc.Y - (title.Y + title.H); gap != 8 {
		t.Errorf("title to description %dpx, want 8", gap)
	}
	if gap := footer.(widget.Boundser).Bounds().Y - (desc.Y + desc.H); gap != 16 {
		t.Errorf("description to buttons %dpx, want 16", gap)
	}
}

// A dialog carries its parent window's app id, so the compositor groups
// it with its application; it had none, and a compositor rule or a
// taskbar keyed on the app id passed it by.
func TestDialogTakesItsParentsAppID(t *testing.T) {
	parent := &Window{cfg: WindowConfig{AppID: "dev.example.app"}}
	if got := dialogAppID(parent); got != "dev.example.app" {
		t.Errorf("dialog app id %q, want the parent's", got)
	}
	if got := dialogAppID(nil); got != "" {
		t.Errorf("parentless dialog app id %q, want none", got)
	}
}
