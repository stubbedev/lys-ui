package widget

import (
	"image"
	"slices"
	"strconv"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/transfer"
	"github.com/stubbedev/lys-ui/widget/css"
)

// TabMime is the drag payload of a dragged tab: its page id, which
// any TabBar in the process resolves to move the page.
const TabMime = "application/x-lys-ui-tab"

// TabBar is libadwaita's AdwTabBar: the tabs of a TabView in a row.
// Clicking a tab selects it, its close button closes it (pinned tabs
// have none), and dragging a tab drops it at a position in this bar or
// in another window's, which moves the page there. Tabs show an icon
// (or a spinner while loading), the title, an attention mark, and the
// page's indicator. It styles as `tabbar` with `tab` children.
type TabBar struct {
	composite
	view   *TabView
	face   render.Font
	sizePx float64
	row    *Box
	tabs   []*tabWidget
	cancel func()
}

type tabWidget struct {
	Button
	bar  *TabBar
	page *TabPage
	look tabLook
}

type tabLook struct {
	title, tooltip, icon, indicator string
	loading, attention, pinned      bool
}

func lookOf(p *TabPage) tabLook {
	return tabLook{p.title, p.tooltip, p.icon, p.indicator, p.loading, p.attention, p.pinned}
}

// DragContent implements DragSource: the page id, offered to move.
func (t *tabWidget) DragContent() *DragContent {
	return &DragContent{
		Content: transfer.Bytes(TabMime, []byte(strconv.FormatUint(t.page.id, 10))),
		Actions: transfer.ActionMove,
	}
}

// NewTabBar returns a bar presenting view's tabs.
func NewTabBar(face render.Font, sizePx float64, view *TabView) *TabBar {
	b := &TabBar{view: view, face: requireFace("widget.NewTabBar", face), sizePx: sizePx, row: NewBox(Row, 2, 0)}
	b.initComposite(b, b.row)
	b.fillWidth = true
	b.boxed = true
	b.SetElement("tabbar")
	b.cancel = view.Subscribe(b.rebuild)
	b.rebuild()
	return b
}

// View returns the presented TabView.
func (b *TabBar) View() *TabView { return b.view }

// Tabs returns the tab widgets in order (tests, styling).
func (b *TabBar) Tabs() []Widget {
	out := make([]Widget, len(b.tabs))
	for i, t := range b.tabs {
		out[i] = t
	}
	return out
}

func (b *TabBar) rebuild() {
	cached := make(map[*TabPage]*tabWidget, len(b.tabs))
	for _, t := range b.tabs {
		cached[t.page] = t
	}
	next := make([]*tabWidget, 0, len(b.view.pages))
	same := len(b.tabs) == len(b.view.pages)
	for i, p := range b.view.pages {
		t, ok := cached[p]
		if !ok || t.look != lookOf(p) {
			t = b.newTab(p)
			same = false
		} else if same && b.tabs[i] != t {
			same = false
		}
		t.SetState(StateSelected, b.view.selected == p)
		next = append(next, t)
	}
	b.tabs = next
	if same {
		return
	}
	b.row.Clear()
	for _, t := range b.tabs {
		b.row.Append(t, !t.page.pinned)
	}
	b.InvalidateLayout()
}

func (b *TabBar) newTab(p *TabPage) *tabWidget {
	content := NewBox(Row, 6, 0)
	switch {
	case p.loading:
		s := NewSpinner(int(b.sizePx))
		s.SetSpinning(true)
		content.AppendAligned(s, false, AlignCenter)
	case p.icon != "":
		content.AppendAligned(NewThemeIcon(p.icon, int(b.sizePx)), false, AlignCenter)
	}
	if !p.pinned {
		title := NewLabel(b.face, b.sizePx, p.title, 0)
		title.SetEllipsize(EllipsizeEnd)
		content.AppendAligned(title, true, AlignCenter)
	}
	if p.attention {
		dot := NewLabel(b.face, b.sizePx, "•", 0)
		dot.AddClass(css.Accent)
		dot.AddClass("needs-attention")
		content.AppendAligned(dot, false, AlignCenter)
	}
	if p.indicator != "" {
		ind := NewButton(NewThemeIcon(p.indicator, int(b.sizePx)), 2, 4)
		ind.AddClass("tab-indicator")
		ind.OnClick = func() {
			if p.OnIndicator != nil {
				p.OnIndicator()
			}
		}
		content.AppendAligned(ind, false, AlignCenter)
	}
	if !p.pinned {
		closeBtn := NewButton(NewSymbol(SymbolClose, int(b.sizePx)-2), 2, 4)
		closeBtn.AddClass("tab-close-button")
		closeBtn.SetTooltip(Tr("Close Tab"))
		closeBtn.OnClick = func() { b.view.ClosePage(p) }
		content.AppendAligned(closeBtn, false, AlignCenter)
	}
	t := &tabWidget{Button: *NewButton(content, 6, 6), bar: b, page: p, look: lookOf(p)}
	t.SetElement("tab")
	t.OnClick = func() { b.view.Select(p) }
	if p.tooltip != "" {
		t.SetTooltip(p.tooltip)
	}
	if p.pinned {
		t.AddClass("pinned")
	}
	return t
}

// DragEnter accepts dragged tabs.
func (b *TabBar) DragEnter(mimes []string, _ Point) string {
	if slices.Contains(mimes, TabMime) {
		return TabMime
	}
	return ""
}

// Drop moves the dropped tab's page to the position under p: a reorder
// within this bar's view, a transfer from another.
func (b *TabBar) Drop(mime string, data []byte, p Point) {
	if mime != TabMime {
		return
	}
	id, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return
	}
	page := tabPageByID(id)
	if page == nil {
		return
	}
	b.view.TransferPage(page, b.indexAt(p))
}

// DropAction moves.
func (b *TabBar) DropAction(transfer.Action) transfer.Action { return transfer.ActionMove }

func (b *TabBar) indexAt(p Point) int {
	for i, t := range b.tabs {
		r := t.Bounds()
		if p.X < r.X+r.W/2 {
			return i
		}
	}
	return len(b.tabs)
}

// Detach stops following the view; the bar keeps its last tabs.
func (b *TabBar) Detach() { b.cancel() }

// TabOverview is libadwaita's AdwTabOverview: while open it replaces
// its child with a grid of the view's pages as thumbnails; clicking
// one selects it and closes the overview.
type TabOverview struct {
	composite
	view   *TabView
	face   render.Font
	sizePx float64
	child  Widget
	stack  *Stack
	grid   *FlowBox
	open   bool

	// OnOpenChanged runs when the overview opens or closes.
	OnOpenChanged func(open bool)
}

// NewTabOverview returns an overview of view around child (usually the
// window content holding the TabView).
func NewTabOverview(face render.Font, sizePx float64, view *TabView, child Widget) *TabOverview {
	o := &TabOverview{view: view, face: requireFace("widget.NewTabOverview", face), sizePx: sizePx, child: child, stack: NewStack(), grid: NewFlowBox(12, 12)}
	o.grid.SetMaxChildrenPerLine(4)
	o.stack.Add("content", child).Add("overview", NewScroll(o.grid))
	o.initComposite(o, o.stack)
	o.SetElement("taboverview")
	return o
}

// Open reports whether the overview shows.
func (o *TabOverview) Open() bool { return o.open }

// SetOpen opens or closes the overview. Opening snapshots every page
// at the view's current size.
func (o *TabOverview) SetOpen(on bool) {
	if o.open == on {
		return
	}
	o.open = on
	if on {
		o.fillGrid()
		o.stack.Show("overview")
	} else {
		o.stack.Show("content")
	}
	if o.OnOpenChanged != nil {
		o.OnOpenChanged(on)
	}
}

func (o *TabOverview) fillGrid() {
	o.grid.Clear()
	size := o.view.Bounds()
	w, h := max(size.W, 320), max(size.H, 200)
	for _, p := range o.view.pages {
		thumb := NewImage(snapshot(p.child, w, h))
		thumb.SetScale(ImageFit)
		card := NewBox(Column, 4, 4)
		card.Append(NewClamp(200, NewAspectFrame(thumb, float64(w)/float64(h))), false)
		card.Append(NewLabel(o.face, o.sizePx, p.title, 0), false)
		btn := NewButton(card, 4, 8)
		btn.SetElement("tabthumbnail")
		btn.SetState(StateSelected, p == o.view.selected)
		btn.OnClick = func() {
			o.view.Select(p)
			o.SetOpen(false)
		}
		o.grid.Append(btn)
	}
}

func snapshot(w Widget, width, height int) *image.NRGBA {
	stride := render.Stride(width)
	data := make([]byte, stride*height)
	cv := render.New(data, stride, width, height)
	cv.Clear(cv.Rect(), Current().Bg)
	w.Measure(Constraints{Max: Size{W: width, H: height}})
	w.Arrange(render.Rect{W: width, H: height})
	w.Paint(cv)
	return render.NRGBA(data, stride, width, height)
}
