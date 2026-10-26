package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// An ancestor KeyInterceptor pre-empts the focused widget's key
// actions, GTK's key controller on a parent: a consumed action never
// reaches the focused widget, an unconsumed one still does.
func TestRouterKeyInterceptor(t *testing.T) {
	intercepted := 0
	box := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	hook := &interceptBox{Box: NewBox(Column, 0, 0), hook: func(target Widget, a KeyAction, mods Mods) bool {
		if a == KeyBackspace {
			intercepted++
			if target != Widget(box) {
				t.Error("the interceptor saw another target")
			}
			return true
		}
		return false
	}}
	hook.Append(box, false)
	// The interceptor is an ancestor in a real tree, so the outer-type
	// parent links resolve through it.
	root := NewBox(Column, 0, 0)
	root.Append(hook, false)
	arrangeTree(t, root, 100, 32)

	r := &Router{Root: root}
	r.SetFocus(box)
	if r.Focused() != Widget(box) {
		t.Fatal("the entry did not take focus")
	}
	r.KeyAction(KeyBackspace, 0)
	if intercepted != 1 {
		t.Errorf("the interceptor saw %d backspaces, want one", intercepted)
	}
	if len(box.Text()) != 0 || box.Text() != "" {
		t.Error("the entry processed the intercepted backspace")
	}
	r.KeyAction(KeyDelete, 0)
	if intercepted != 1 {
		t.Error("an unconsumed action reached the interceptor")
	}
}

// interceptBox is a Box implementing KeyInterceptor.
type interceptBox struct {
	*Box
	hook func(target Widget, a KeyAction, mods Mods) bool
}

func (b *interceptBox) InterceptKey(target Widget, a KeyAction, mods Mods) bool {
	return b.hook(target, a, mods)
}
