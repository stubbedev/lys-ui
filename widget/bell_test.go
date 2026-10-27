package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A read-only Entry rings for refused keys (a disabled one is silent);
// a SpinButton rings for text that does not parse, not for its own
// formatted value; the bell names the widget whose tree RootOf finds.
func TestErrorBell(t *testing.T) {
	var rang []Widget
	SetErrorBell(func(w Widget) { rang = append(rang, w) })
	t.Cleanup(func() { SetErrorBell(nil) })

	face := testFace(t)
	e := NewEntry(face, 14, render.RGB(0, 0, 0))
	box := NewBox(Row, 0, 0)
	box.Append(e, false)
	SizeWithin(box, Size{W: 200, H: 100})
	box.Arrange(render.Rect{W: 200, H: 100}) // parent links form in Arrange
	e.InsertRune('a')
	if len(rang) != 0 {
		t.Fatalf("editable entry rang: %d", len(rang))
	}
	e.SetReadOnly(true)
	e.InsertRune('a')
	e.Insert("bc")
	if len(rang) != 2 || rang[0] != e {
		t.Fatalf("read-only entry rang %d times", len(rang))
	}
	if RootOf(rang[0]) != box {
		t.Error("RootOf did not reach the box")
	}
	e.SetEnabled(false)
	e.InsertRune('a')
	if len(rang) != 2 {
		t.Error("disabled entry rang")
	}

	rang = nil
	s := NewSpinButton(face, 14, render.RGB(0, 0, 0), 0, 10, 1, 0)
	s.commit()
	if len(rang) != 0 {
		t.Fatalf("own value rang: %d", len(rang))
	}
	s.SetText("abc")
	s.commit()
	if len(rang) != 1 || rang[0] != s {
		t.Fatalf("invalid text rang %d times", len(rang))
	}

	SetErrorBell(nil)
	ErrorBell(s) // silent without a bell
}
