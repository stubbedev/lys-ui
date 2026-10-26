package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestToolbarViewStacksBarsAroundContent(t *testing.T) {
	top, bottom, content := NewSpacer(100, 20), NewSpacer(100, 30), NewSpacer(100, 100)
	tv := NewToolbarView(content)
	tv.AddTopBar(top)
	tv.AddBottomBar(bottom)
	arrange(tv, 200, 300)
	if top.Bounds().Y != 0 || content.Bounds().Y != 20 || content.Bounds().H != 250 || bottom.Bounds().Y != 270 {
		t.Errorf("top %+v content %+v bottom %+v", top.Bounds(), content.Bounds(), bottom.Bounds())
	}

	tv.SetExtendContentToTopEdge(true)
	tv.SetExtendContentToBottomEdge(true)
	arrange(tv, 200, 300)
	if content.Bounds() != (render.Rect{W: 200, H: 300}) || top.Bounds().Y != 0 || bottom.Bounds().Y != 270 {
		t.Errorf("extended: content %+v top %+v bottom %+v", content.Bounds(), top.Bounds(), bottom.Bounds())
	}

	tv.SetExtendContentToTopEdge(false)
	tv.SetExtendContentToBottomEdge(false)
	tv.SetRevealTopBars(false)
	tv.topReveal.Finish()
	arrange(tv, 200, 300)
	if tv.RevealTopBars() || content.Bounds().Y != 0 {
		t.Errorf("hidden top bars: content at %d", content.Bounds().Y)
	}
}

func TestToolbarViewStylesBars(t *testing.T) {
	tv := NewToolbarView(NewSpacer(1, 1))
	if !HasClass(tv.top, "flat") {
		t.Error("the default top style is not flat")
	}
	tv.SetTopBarStyle(ToolbarRaisedBorder)
	if HasClass(tv.top, "flat") || !HasClass(tv.top, "raised-border") || !HasClass(tv.top, "top-bar") {
		t.Errorf("classes after raised-border: %v", tv.top.Classes())
	}
	if tv.Element() != "toolbarview" {
		t.Errorf("element %q", tv.Element())
	}
}
