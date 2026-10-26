package widget

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"
)

// face is the shared test font for the connector widgets.
func bindingFace(t *testing.T) *render.Typeface {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestBindingSetNotifiesAndSuppressesEqual pins the echo rule: a Set
// with the current value notifies nobody, so a write-back from a
// subscriber cannot restart the loop.
func TestBindingSetNotifiesAndEqual(t *testing.T) {
	b := NewBinding("start")
	changes := 0
	b.Subscribe(func(string) { changes++ })
	b.Set("next")
	if changes != 1 || b.Get() != "next" {
		t.Fatalf("changes=%d value=%q, want 1 and next", changes, b.Get())
	}
	b.Set("next")
	if changes != 1 {
		t.Errorf("equal Set notified again: changes=%d", changes)
	}
}

// TestBindingWriteWhileNotifyingDefers pins the guard: a subscriber
// that Sets during its own notification neither recurses nor loses the
// write - the deferred value applies once the notification finished.
func TestBindingWriteWhileNotifyingDefers(t *testing.T) {
	b := NewBinding(0)
	depth := 0
	maxDepth := 0
	b.Subscribe(func(v int) {
		depth++
		maxDepth = max(maxDepth, depth)
		if v == 1 {
			b.Set(2) // write while being notified
		}
		depth--
	})
	b.Set(1)
	if maxDepth != 1 {
		t.Errorf("notification nested to depth %d, want 1 (no recursion)", maxDepth)
	}
	if b.Get() != 2 {
		t.Errorf("deferred write lost: value = %d, want 2", b.Get())
	}
}

// TestBindingTwoWayPingPongTerminates is the #81 acceptance: two
// bindings wired into each other settle after one notification each -
// the equal no-op is what makes two-way wiring safe without guards in
// the app.
func TestBindingTwoWayPingPongTerminates(t *testing.T) {
	left := NewBinding("same")
	right := NewBinding("same")
	cancelL := left.Subscribe(right.Set)
	defer cancelL()
	cancelR := right.Subscribe(left.Set)
	defer cancelR()

	left.Set("moved")
	if right.Get() != "moved" {
		t.Fatalf("right never followed: %q", right.Get())
	}
	if left.Get() != "moved" {
		t.Fatalf("left changed by the echo: %q", left.Get())
	}
}

// TestBindingConnectorsTwoWay drives every connector through both
// directions: a binding Set lands in the widget, a user edit lands in
// the binding, and unbind stops both.
func TestBindingConnectorsTwoWay(t *testing.T) {
	face := bindingFace(t)
	entry := NewEntry(face, 14, render.RGB(255, 255, 255))
	area := NewTextArea(face, 14, render.RGB(255, 255, 255))
	sw := NewSwitch(false)
	check := NewCheckButton(false)
	slider := NewSlider(0, 100, 1, 0)
	spin := NewSpinButton(face, 14, render.RGB(255, 255, 255), 0, 10, 1, 0)
	drop := NewDropdown(face, 14, []string{"a", "b", "c"}, 0)

	text := NewBinding("")
	unbindEntry := entry.BindText(text)
	text.Set("hello")
	if entry.Text() != "hello" {
		t.Errorf("entry text = %q, want the binding's", entry.Text())
	}
	entry.SetText("edited")
	if text.Get() != "edited" {
		t.Errorf("binding = %q, want the entry's edit", text.Get())
	}
	unbindEntry()
	entry.SetText("detached")
	if text.Get() != "edited" {
		t.Errorf("unbind did not detach the entry: binding = %q", text.Get())
	}

	contents := NewBinding("")
	unbindArea := area.BindText(contents)
	contents.Set("multi\nline")
	if area.Text() != "multi\nline" {
		t.Errorf("area text = %q", area.Text())
	}
	area.SetText("rewritten")
	if contents.Get() != "rewritten" {
		t.Errorf("binding = %q, want the area's edit", contents.Get())
	}
	unbindArea()

	on := NewBinding(false)
	unbindSw := sw.BindOn(on)
	on.Set(true)
	if !sw.On() {
		t.Error("switch never followed the binding")
	}
	sw.SetOn(false)
	if on.Get() {
		t.Error("binding never followed the switch")
	}
	unbindSw()
	sw.SetOn(true)
	if on.Get() {
		t.Error("unbind did not detach the switch")
	}

	checked := NewBinding(false)
	unbindCheck := check.BindChecked(checked)
	checked.Set(true)
	if !check.Checked() {
		t.Error("checkbox never followed the binding")
	}
	check.SetChecked(false)
	if checked.Get() {
		t.Error("binding never followed the checkbox")
	}
	unbindCheck()

	value := NewBinding(0.0)
	unbindSlider := slider.BindValue(value)
	value.Set(41.9)
	// SetValue snaps to the step: 41.9 on a step-1 slider settles on
	// 42, and the settle echoes back into the binding - no loop.
	if got := slider.Value(); got != 42 {
		t.Errorf("slider = %v, want the snapped 42", got)
	}
	if value.Get() != 42 {
		t.Errorf("binding = %v, want the slider's snapped value", value.Get())
	}
	slider.SetValue(7)
	if value.Get() != 7 {
		t.Errorf("binding = %v, want the slider's 7", value.Get())
	}
	unbindSlider()

	count := NewBinding(0.0)
	unbindSpin := spin.BindValue(count)
	count.Set(3)
	if spin.Value() != 3 {
		t.Errorf("spin = %v, want 3", spin.Value())
	}
	// Only a user edit fires OnValueChanged: type and commit the way
	// Enter does.
	spin.SetText("8")
	spin.OnActivate("")
	if count.Get() != 8 {
		t.Errorf("binding = %v, want the committed 8", count.Get())
	}
	unbindSpin()

	selected := NewBinding(0)
	unbindDrop := drop.BindSelected(selected)
	selected.Set(2)
	if drop.Selected() != 2 {
		t.Errorf("dropdown = %d, want 2", drop.Selected())
	}
	drop.SetSelected(1)
	if selected.Get() != 1 {
		t.Errorf("binding = %d, want the dropdown's 1", selected.Get())
	}
	unbindDrop()
}

// TestBindingLabelOneWay pins the one-way connector: the label follows
// the binding, and nothing exists to write back.
func TestBindingLabelOneWay(t *testing.T) {
	face := bindingFace(t)
	label := NewLabel(face, 14, "start", render.RGB(255, 255, 255))
	text := NewBinding("start")
	unbind := label.BindText(text)
	text.Set("follows")
	if label.Text() != "follows" {
		t.Errorf("label = %q, want the binding's", label.Text())
	}
	unbind()
	text.Set("detached")
	if label.Text() != "follows" {
		t.Errorf("unbind did not detach the label: %q", label.Text())
	}
}

// TestBindingTwoWidgetsOneValueNoEcho is the connected-widget version
// of the ping-pong test: two entries on one binding stay in sync after
// an edit, and the wiring settles without a loop.
func TestBindingTwoWidgetsOneValueNoEcho(t *testing.T) {
	face := bindingFace(t)
	one := NewEntry(face, 14, render.RGB(255, 255, 255))
	two := NewEntry(face, 14, render.RGB(255, 255, 255))
	text := NewBinding("")
	defer one.BindText(text)()
	defer two.BindText(text)()

	one.SetText("typed")
	if two.Text() != "typed" || text.Get() != "typed" {
		t.Errorf("sync after edit: one=%q two=%q binding=%q", one.Text(), two.Text(), text.Get())
	}
}

// TestBindingOffLoopTripsHook pins the loop enforcement: a Set from a
// foreign goroutine trips the same off-loop hook a widget mutation
// does, and an on-loop Set does not.
func TestBindingOffLoopTripsHook(t *testing.T) {
	UnmarkLoop()
	defer func() {
		UnmarkLoop()
		SetOffLoopHook(nil)
	}()
	var hits atomic.Int32
	SetOffLoopHook(func(string) { hits.Add(1) })

	b := NewBinding(0)
	MarkLoop()
	b.Set(1)
	if got := hits.Load(); got != 0 {
		t.Errorf("on-loop Set tripped the hook %d times", got)
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		defer close(done)
		b.Set(2) // foreign goroutine, guard still armed
	})
	<-done
	if got := hits.Load(); got != 1 {
		t.Errorf("off-loop Set tripped the hook %d times, want 1", got)
	}
}
