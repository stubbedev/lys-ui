package widget

import "testing"

func TestWindowTitleHidesAnEmptySubtitle(t *testing.T) {
	w := NewWindowTitle(testFace(t), 14, "Document", "")
	with := SizeWithin(w, Size{W: 400, H: 100})
	if IsVisible(w.subtitle) {
		t.Error("an empty subtitle is visible")
	}
	w.SetSubtitle("~/notes")
	if !IsVisible(w.subtitle) || SizeWithin(w, Size{W: 400, H: 100}).H <= with.H {
		t.Error("a subtitle did not add its line")
	}
	if w.Element() != "windowtitle" || !HasClass(w.title, "title") || !HasClass(w.subtitle, "subtitle") {
		t.Error("style nodes missing")
	}
}
