package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// SheetSide is the edge a Sheet slides in from.
type SheetSide uint8

// The sheet sides.
const (
	// SheetRight slides in from the right edge.
	SheetRight SheetSide = iota
	// SheetLeft slides in from the left edge.
	SheetLeft
	// SheetTop slides down from the top edge.
	SheetTop
	// SheetBottom slides up from the bottom edge.
	SheetBottom
)

const (
	// sheetHandle is a bottom sheet's drag strip at its top edge.
	sheetHandle = 24
	// sheetWidth caps a side sheet's width; it takes at most three
	// quarters of the widget below that.
	sheetWidth = 384
	// sheetPad is the panel's padding.
	sheetPad = 16
	// sheetDuration is the slide.
	sheetDuration = 300 * time.Millisecond
)

// Sheet is a panel over content (and adw's BottomSheet as
// NewBottomSheet): SetOpen slides the panel in from its side over a
// dimmed backdrop. A click on the backdrop and Escape close it, and
// while it is open Tab stays inside the panel. A bottom sheet carries a
// grab handle: dragging it down past a third of the sheet dismisses it,
// a shorter drag springs back. Side and top sheets take an optional
// header (SetHeader), footer (SetFooter) and a close button. It styles
// as the `sheet` element; the panel as `.sheet-panel`, its header's
// .title and .description.
type Sheet struct {
	composite
	side  SheetSide
	panel *Box
	body  Widget
	close *Button
	head  *Box

	// width is a side sheet's widest; sheetWidth until SetWidth.
	width int

	open     bool
	progress float64
	cancel   anim.Cancel
	drag     dragGesture
	dragDY   int

	// OnClosed fires when a close finished sliding out, whatever
	// caused it.
	OnClosed func()
}

// NewSheet returns a closed sheet sliding body in from side over
// content.
func NewSheet(content, body Widget, side SheetSide) *Sheet {
	s := &Sheet{side: side, body: body, panel: NewBox(Column, 16, 0), width: sheetWidth}
	s.SetElement("sheet")
	s.panel.AddClass("sheet-panel")
	s.panel.SetPadding(render.UniformInsets(sheetPad))
	s.panel.Append(body, true)
	s.initComposite(s, content)
	s.surfaceRadius = 12
	return s
}

// NewBottomSheet returns a closed sheet sliding sheet up from the
// bottom over content, with the grab handle (adw BottomSheet).
func NewBottomSheet(content, sheet Widget) *Sheet { return NewSheet(content, sheet, SheetBottom) }

// SetHeader puts a title and a description above the body, with a
// close button at the panel's corner.
func (s *Sheet) SetHeader(face render.Font, title, description string) {
	face = requireFace("widget.Sheet.SetHeader", face)
	if s.head != nil {
		s.panel.Remove(s.head)
	}
	s.head = NewBox(Column, 6, 0)
	align := render.AlignStart
	if s.side == SheetBottom {
		align = render.AlignCenter
	}
	t := NewLabel(face, 18, title, 0)
	t.AddClass("title")
	t.SetAlignment(align)
	s.head.Append(t, false)
	if description != "" {
		d := NewLabel(face, 14, description, 0)
		d.AddClass("description")
		d.SetWrap(true)
		d.SetAlignment(align)
		s.head.Append(d, false)
	}
	row := NewBox(Row, 16, 0)
	row.Append(s.head, true)
	if s.close == nil {
		s.close = NewButton(NewSymbol(SymbolClose, 16), 0, 0)
		s.close.AddClass("close")
		s.close.SetAccessibleName(Tr("Close"))
		s.close.OnClick = func() { s.SetOpen(false) }
	}
	row.AppendAligned(s.close, false, AlignStart)
	s.head = row
	s.panel.InsertAt(0, row, false)
	s.InvalidateLayout()
}

// SetFooter puts w (the actions, typically) under the body.
func (s *Sheet) SetFooter(w Widget) {
	s.panel.Append(w, false)
	s.InvalidateLayout()
}

// SetWidth sets a side sheet's widest, still at most three quarters of
// the widget.
func (s *Sheet) SetWidth(w int) {
	s.width = w
	s.InvalidateLayout()
}

// Side returns the edge the sheet slides in from.
func (s *Sheet) Side() SheetSide { return s.side }

// Open reports whether the sheet is open (or opening).
func (s *Sheet) Open() bool { return s.open }

// SetOpen slides the sheet in or out.
func (s *Sheet) SetOpen(on bool) {
	if s.open == on {
		return
	}
	s.open = on
	s.dragDY = 0
	if s.cancel != nil {
		s.cancel()
	}
	from, to := s.progress, 0.0
	if on {
		to = 1
	}
	s.cancel = anim.Start(sheetDuration, func(t float64) {
		s.progress = from + (to-from)*easeOut(t)
		s.InvalidateLayout()
		if t >= 1 && !on && s.OnClosed != nil {
			s.OnClosed()
		}
	})
}

// showing reports whether any of the sheet is on screen.
func (s *Sheet) showing() bool { return s.progress > 0 }

// sheetRect places the panel for the current progress and drag: a
// side sheet at most its width (three quarters of the widget) wide
// and full height, a top or bottom one its natural height capped at
// nine tenths of the widget (a bottom one's handle included).
func (s *Sheet) sheetRect() render.Rect {
	r := s.bounds
	switch s.side {
	case SheetLeft, SheetRight:
		w := min(s.width, r.W*3/4)
		shown := int(float64(w) * s.progress)
		if s.side == SheetLeft {
			return render.Rect{X: r.X - w + shown, Y: r.Y, W: w, H: r.H}
		}
		return render.Rect{X: r.X + r.W - shown, Y: r.Y, W: w, H: r.H}
	}
	nat := s.panel.Measure(Constraints{Max: Size{W: s.panelRect(r).W, H: r.H}})
	if s.side == SheetTop {
		h := min(nat.H, r.H*9/10)
		return render.Rect{X: r.X, Y: r.Y - h + int(float64(h)*s.progress), W: r.W, H: h}
	}
	h := min(nat.H+sheetHandle, r.H*9/10)
	y := r.Y + r.H - int(float64(h)*s.progress) + s.dragDY
	return render.Rect{X: r.X, Y: y, W: r.W, H: h}
}

// panelRect is where the panel's content lies in the sheet rect: below
// a bottom sheet's handle, the whole rect otherwise.
func (s *Sheet) panelRect(rect render.Rect) render.Rect {
	if s.side == SheetBottom {
		rect.Y += sheetHandle
		rect.H -= sheetHandle
		// The content column is a side sheet's width at most,
		// centered across the bottom edge.
		if w := sheetWidth + 2*sheetPad; rect.W > w {
			rect.X += (rect.W - w) / 2
			rect.W = w
		}
	}
	return rect
}

// Arrange lays the content over the whole rect and the panel where the
// slide and the drag put it: each animation frame relayouts the sheet.
func (s *Sheet) Arrange(r render.Rect) {
	s.composite.Arrange(r)
	if !s.showing() {
		return
	}
	pr := s.panelRect(s.sheetRect())
	s.panel.Measure(Constraints{Max: Size{W: pr.W, H: pr.H}})
	s.panel.Arrange(pr)
	setParents(s, s.panel)
}

// Paint draws the content, then - while showing - the dimmed backdrop,
// the panel's surface (a bottom sheet's with its handle) and the panel.
func (s *Sheet) Paint(cv *render.Canvas) {
	PaintChild(cv, s.root)
	if !s.showing() {
		return
	}
	th := Current()
	cv.FillRect(s.bounds, render.RGBA(0, 0, 0, uint8(128*s.progress)))
	rect := s.sheetRect()
	v := s.style(s)
	paintBoxBehind(cv, v, rect, s.corners(), borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Bg))
	if edge, ok := s.innerEdge(rect); ok {
		cv.FillRect(edge, th.Border)
	}
	if s.side == SheetBottom {
		grip := render.Rect{X: rect.X + rect.W/2 - 50, Y: rect.Y + 10, W: 100, H: 6}
		cv.RoundedRect(grip, 3, th.MutedColor())
	}
	PaintChild(cv, s.panel)
}

// corners rounds the edge facing the content of a top or bottom
// sheet; a side sheet is square.
func (s *Sheet) corners() render.Corners {
	switch s.side {
	case SheetBottom:
		return render.Corners{TopLeft: s.surfaceRadius, TopRight: s.surfaceRadius}
	case SheetTop:
		return render.Corners{BottomLeft: s.surfaceRadius, BottomRight: s.surfaceRadius}
	}
	return render.Corners{}
}

// innerEdge is the hairline along a side sheet's edge facing the
// content.
func (s *Sheet) innerEdge(rect render.Rect) (render.Rect, bool) {
	switch s.side {
	case SheetRight:
		return render.Rect{X: rect.X, Y: rect.Y, W: 1, H: rect.H}, true
	case SheetLeft:
		return render.Rect{X: rect.X + rect.W - 1, Y: rect.Y, W: 1, H: rect.H}, true
	}
	return render.Rect{}, false
}

// HitTest resolves into the panel while the sheet shows (the backdrop
// and a bottom sheet's handle are the sheet itself), else into the
// content.
func (s *Sheet) HitTest(p Point) Widget {
	if !s.showing() {
		return s.composite.HitTest(p)
	}
	if !s.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if rect := s.panelRect(s.sheetRect()); rect.Contains(p.X, p.Y) {
		if hit := s.panel.HitTest(p); hit != nil {
			return hit
		}
	}
	return s
}

// onHandle reports p on a bottom sheet's handle strip.
func (s *Sheet) onHandle(p Point) bool {
	rect := s.sheetRect()
	return s.side == SheetBottom && rect.Contains(p.X, p.Y) && p.Y < rect.Y+sheetHandle
}

// PressAt begins a drag on a bottom sheet's handle strip.
func (s *Sheet) PressAt(p Point) {
	if s.open && s.onHandle(p) {
		s.drag.begin(p)
	}
}

// DragMove follows a downward drag.
func (s *Sheet) DragMove(p Point) {
	if _, dy, ok := s.drag.travel(p); ok {
		s.dragDY = max(dy, 0)
		s.InvalidateLayout()
	}
}

// PressEnd dismisses past a third of the sheet's height, else springs
// back.
func (s *Sheet) PressEnd() {
	if !s.drag.end() {
		return
	}
	if s.dragDY > s.sheetRect().H/3 {
		s.SetOpen(false)
		return
	}
	s.dragDY = 0
	s.InvalidateLayout()
}

// ClickAt closes on a click on the backdrop.
func (s *Sheet) ClickAt(p Point) {
	if s.open && !s.sheetRect().Contains(p.X, p.Y) {
		s.SetOpen(false)
	}
}

// KeyAction closes on Escape while the sheet itself has the focus.
func (s *Sheet) KeyAction(a KeyAction, _ Mods) {
	if a == KeyDismiss && s.open {
		s.SetOpen(false)
	}
}

// InterceptKey implements KeyInterceptor: Escape anywhere in the open
// sheet closes it.
func (s *Sheet) InterceptKey(_ Widget, a KeyAction, _ Mods) bool {
	if a != KeyDismiss || !s.open {
		return false
	}
	s.SetOpen(false)
	return true
}

// FocusTrap implements FocusTrapper: while open, Tab stays in the
// panel.
func (s *Sheet) FocusTrap() Widget {
	if s.open {
		return s.panel
	}
	return nil
}

// Children is the content and, while showing, the panel: focus
// traversal, damage and the accessibility tree reach the panel's
// widgets.
func (s *Sheet) Children() []Widget {
	if s.showing() {
		return []Widget{s.root, s.panel}
	}
	return []Widget{s.root}
}

// appendChildren appends what Children holds.
func (s *Sheet) appendChildren(buf []Widget) []Widget {
	if s.showing() {
		return append(buf, s.root, s.panel)
	}
	return append(buf, s.root)
}

// appendStyleKids is the content and, while showing, the panel.
func (s *Sheet) appendStyleKids(buf []Widget) []Widget { return s.appendChildren(buf) }

// Content returns the widget the sheet slides over.
func (s *Sheet) Content() Widget { return s.root }
