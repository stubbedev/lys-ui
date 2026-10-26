package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestNodeTooltip(t *testing.T) {
	l := NewSpacer(4, 4)
	if l.TooltipText() != "" {
		t.Error("fresh widget has tooltip text")
	}
	l.SetTooltip("does the thing")
	if l.TooltipText() != "does the thing" {
		t.Errorf("TooltipText = %q", l.TooltipText())
	}
	l.SetTooltip("")
	if l.TooltipText() != "" {
		t.Error("empty SetTooltip did not clear")
	}
	_ = render.RGB
}
