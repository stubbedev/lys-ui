package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A stack is a CSS box like GtkStack: padding and min sizes size it,
// its pages fill the content box, and its minimum counts the padding.
func TestStackHonorsItsCSSBox(t *testing.T) {
	loadCSS(t, `.padded { padding: 10px 20px; min-height: 120px; }`)
	page := NewBox(Column, 0, 0)
	page.SetSizeRequest(100, 50)
	s := NewStack()
	s.Add("a", page)
	s.AddClass("padded")
	con := Size{W: 400, H: 400}
	if got := SizeWithin(s, con); got != (Size{W: 140, H: 140}) {
		t.Errorf("Measure = %+v, want the page plus padding, min-height flooring the content box", got)
	}
	s.Arrange(render.Rect{X: 5, Y: 7, W: 300, H: 200})
	if got, want := page.Bounds(), (render.Rect{X: 25, Y: 17, W: 260, H: 180}); got != want {
		t.Errorf("page arranged at %+v, want the content box %+v", got, want)
	}
	if got := s.Measure(Column, 300).Min; got != 140 {
		t.Errorf("minimum height %d, want the 120 min-height plus the 20 vertical padding", got)
	}

	plain := NewStack()
	plain.Add("a", NewBox(Column, 0, 0))
	plainPage := plain.Children()[0].(*Box)
	plainPage.SetSizeRequest(100, 50)
	if got := SizeWithin(plain, con); got != (Size{W: 100, H: 50}) {
		t.Errorf("an unstyled stack measures %+v, want its page", got)
	}
}

func TestStackPageFindsHiddenPages(t *testing.T) {
	a, b := NewBox(Column, 0, 0), NewBox(Column, 0, 0)
	s := NewStack().Add("a", a).Add("b", b)
	if s.Page("b") != Widget(b) || s.Page("a") != Widget(a) {
		t.Error("Page did not return the pages by name")
	}
	if s.Page("missing") != nil {
		t.Error("an unknown page name returned a widget")
	}
}
