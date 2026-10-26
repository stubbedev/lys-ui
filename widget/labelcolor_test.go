package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestLabelSetColor(t *testing.T) {
	blue := render.RGB(0, 0, 255)
	red := render.RGB(255, 0, 0)
	label := NewLabel(testFace(t), 14, "75%", blue)
	if label.Color() != blue {
		t.Fatalf("constructor color = %#08x, want blue", label.Color())
	}
	label.Arrange(render.Rect{X: 0, Y: 0, W: 30, H: 20})
	_, dirty := CollectDamage(label)
	if !dirty {
		t.Fatal("fresh label: want damage")
	}
	_, _ = CollectDamage(label)

	label.SetColor(red)
	if label.Color() != red {
		t.Fatalf("SetColor: color = %#08x, want red", label.Color())
	}
	if _, dirty := CollectDamage(label); !dirty {
		t.Fatal("SetColor: want the label dirty for the repaint")
	}
	_, _ = CollectDamage(label)
	label.SetColor(red)
	if _, dirty := CollectDamage(label); dirty {
		t.Error("SetColor to the same value: want no repaint")
	}
}
