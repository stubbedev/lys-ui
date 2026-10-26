package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// WindowTitle is libadwaita's AdwWindowTitle: a title over a smaller,
// muted subtitle, centered in a header bar, each ellipsized; an empty
// subtitle takes no space. It styles as `windowtitle` with `.title`
// and `.subtitle` labels.
type WindowTitle struct {
	composite
	column   *Box
	title    *Label
	subtitle *Label
}

// NewWindowTitle returns the title pair in face at sizePx, the
// subtitle a step smaller.
func NewWindowTitle(face render.Font, sizePx float64, title, subtitle string) *WindowTitle {
	face = requireFace("widget.NewWindowTitle", face)
	w := &WindowTitle{
		column:   NewBox(Column, 0, 0),
		title:    NewLabel(face, sizePx, title, 0),
		subtitle: NewLabel(face, sizePx*0.85, subtitle, 0),
	}
	w.subtitle.AddClass(css.DimLabel)
	for _, l := range []*Label{w.title, w.subtitle} {
		l.SetEllipsize(EllipsizeEnd)
		l.SetAlignment(render.AlignCenter)
	}
	w.title.AddClass("title")
	w.subtitle.AddClass("subtitle")
	w.column.AppendAligned(w.title, false, AlignCenter)
	w.column.AppendAligned(w.subtitle, false, AlignCenter)
	w.subtitle.SetVisible(subtitle != "")
	w.initComposite(w, w.column)
	w.SetElement("windowtitle")
	return w
}

// Title returns the title.
func (w *WindowTitle) Title() string { return w.title.Text() }

// SetTitle sets the title.
func (w *WindowTitle) SetTitle(s string) { w.title.SetText(s) }

// Subtitle returns the subtitle.
func (w *WindowTitle) Subtitle() string { return w.subtitle.Text() }

// SetSubtitle sets the subtitle; empty hides it.
func (w *WindowTitle) SetSubtitle(s string) {
	w.subtitle.SetText(s)
	w.subtitle.SetVisible(s != "")
}
