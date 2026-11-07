package widget

import "testing"

func TestFirstFocusableSkipsWhatCannotTakeFocus(t *testing.T) {
	face := testFace(t)
	disabled := NewButton(NewLabel(face, 12, "off", 0), 0, 0)
	disabled.SetEnabled(false)
	hidden := NewSwitch(false)
	hidden.SetVisible(false)
	target := NewSwitch(true)
	inner := NewBox(Row, 0, 0)
	inner.Append(hidden, false)
	inner.Append(target, false)
	root := NewBox(Column, 0, 0)
	root.Append(NewLabel(face, 12, "title", 0), false)
	root.Append(disabled, false)
	root.Append(inner, false)
	root.Append(NewSwitch(false), false)
	if got := FirstFocusable(root); got != Widget(target) {
		t.Fatalf("FirstFocusable = %T %p, want the enabled, shown switch %p", got, got, target)
	}
}

func TestFirstFocusableOfATreeWithoutControlsIsNil(t *testing.T) {
	face := testFace(t)
	root := NewBox(Column, 0, 0)
	root.Append(NewLabel(face, 12, "a", 0), false)
	root.Append(NewSeparator(Horizontal), false)
	if got := FirstFocusable(root); got != nil {
		t.Fatalf("FirstFocusable = %T, want nil", got)
	}
}
