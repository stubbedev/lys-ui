package widget

// The rendering-effect golden set (#159): each effect painted on a
// consumer widget. New effects add a shot here; the effect-free
// goldens above must stay untouched.

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestGoldenCSSFilterList(t *testing.T) {
	face := goldenFace(t)
	btn := NewButton(NewLabel(face, 14, "Filter", render.RGB(0xff, 0xff, 0xff)), 10, 6)
	btn.AddClass("filtered")
	cssShot(t, `
		.filtered {
			background-color: #7b3fa0;
			border-radius: 12;
			filter: blur(2px) sepia(60%) drop-shadow(3px 4px 2px rgba(0,0,0,.6));
		}
	`, btn, "css-filter-list")
}

func TestGoldenCSSBackdropFilter(t *testing.T) {
	face := goldenFace(t)
	card := NewBox(Column, 8, 8)
	lbl := NewLabel(face, 14, "blurred behind", render.RGB(0x11, 0x11, 0x18))
	behind := NewBox(Column, 4, 4)
	behind.Append(lbl, false)
	root := NewBox(Column, 10, 10)
	root.Append(behind, false)
	root.Append(card, false)
	card.Append(NewLabel(face, 14, "card", render.RGB(0xff, 0xff, 0xff)), false)
	card.AddClass("frost")
	cssShot(t, `
		.frost {
			background-color: rgba(255,255,255,0.25);
			border-radius: 10;
			backdrop-filter: blur(3px) grayscale(80%);
			min-height: 28;
		}
	`, root, "css-backdrop-filter")
}

func TestGoldenCSSMixBlendMode(t *testing.T) {
	face := goldenFace(t)
	base := NewLabel(face, 20, "BIG", render.RGB(0xe0, 0x55, 0x61))
	over := NewLabel(face, 20, "over", render.RGB(0x3f, 0xb6, 0x8b))
	over.AddClass("blended")
	root := NewBox(Column, 0, 0)
	root.Append(base, false)
	root.Append(over, false)
	cssShot(t, `
		.blended { mix-blend-mode: exclusion; margin-top: -10; }
	`, root, "css-mix-blend-mode")
}

func TestGoldenCSSClipPath(t *testing.T) {
	face := goldenFace(t)
	btn := NewButton(NewLabel(face, 14, "Clip", render.RGB(0xff, 0xff, 0xff)), 12, 6)
	btn.AddClass("clipped")
	cssShot(t, `
		.clipped {
			background-color: #0f7b6c;
			border-radius: 8;
			clip-path: inset(4px 6px round 8px);
		}
	`, btn, "css-clip-path-inset")
}

func TestGoldenCSSClipPathPolygon(t *testing.T) {
	face := goldenFace(t)
	btn := NewButton(NewLabel(face, 14, "Poly", render.RGB(0xff, 0xff, 0xff)), 12, 6)
	btn.AddClass("poly")
	cssShot(t, `
		.poly {
			background-color: #b0553f;
			clip-path: polygon(0 0, 100% 25%, 75% 100%, 0 75%);
		}
	`, btn, "css-clip-path-polygon")
}

func TestGoldenCSSMaskGradient(t *testing.T) {
	face := goldenFace(t)
	lbl := NewLabel(face, 16, "fading away", render.RGB(0xff, 0xff, 0xff))
	lbl.AddClass("masked")
	cssShot(t, `
		.masked {
			background-color: #3f6bb6;
			padding: 8 12;
			mask: linear-gradient(to right, black, transparent 90%);
		}
	`, lbl, "css-mask-gradient")
}

func TestGoldenCSSPerspective(t *testing.T) {
	face := goldenFace(t)
	btn := NewButton(NewLabel(face, 14, "Tilt", render.RGB(0xff, 0xff, 0xff)), 12, 6)
	btn.AddClass("tilted")
	cssShot(t, `
		.tilted {
			background-color: #6b3fb6;
			border-radius: 8;
			transform: perspective(300px) rotateY(55deg);
		}
	`, btn, "css-perspective-rotatey")
}
