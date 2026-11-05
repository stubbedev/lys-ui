package popup

import (
	"fmt"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

func popupScrollStep(t *testing.T, bg render.Color) []widget.ScrollMove {
	t.Helper()
	p, _ := wireFreePopup(t, surfx.KindMenu)
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	col := widget.NewBox(widget.Column, 0, 0)
	for i := range 2000 {
		col.Append(widget.NewLabel(face, 14, fmt.Sprintf("row %d", i), 0), false)
	}
	sc := widget.NewScroll(col)
	pc := p.NewPainter(&wlsession.Session{}, nil, 0, sc, bg)
	r := render.Rect{W: 300, H: 400}
	widget.SizeWithin(pc.visual, widget.Size{W: 300, H: 400})
	pc.visual.Arrange(r)
	widget.CollectFrameDamageInto(nil, nil, pc.visual)
	sc.SetOffset(0, 10)
	pc.visual.Arrange(r)
	_, moves, _ := widget.CollectFrameDamageInto(nil, nil, pc.visual)
	return moves
}

func TestAScrollInAPopupOnAnOpaquePlateMoves(t *testing.T) {
	if moves := popupScrollStep(t, render.RGB(30, 34, 40)); len(moves) != 1 || moves[0].DY != -10 {
		t.Errorf("moves %+v, want the popup's view copied up by 10", moves)
	}
}

func TestAScrollInAPopupOnATranslucentPlateRepaints(t *testing.T) {
	if moves := popupScrollStep(t, render.RGBA(30, 34, 40, 0x80)); len(moves) != 0 {
		t.Errorf("a step over a translucent plate was copied: %+v", moves)
	}
}
