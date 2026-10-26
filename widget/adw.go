package widget

import (
	"image"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// The libadwaita miscellany of #95, each composed from the shared
// shapes: ToggleGroup (the segmented control ViewSwitcher is built
// on), ButtonContent, StatusPage, Banner, Avatar, and SplitButton.
// BottomSheet lives in sheet.go, WrapBox is FlowBox's justify
// option.

// ToggleGroup is the segmented control (adw 1.7): a pill of
// ToggleButtons in one radio group, exactly one active. Clicks and
// Left/Right activate; SetActive drives it from code. ViewSwitcher is
// a ToggleGroup bound to a Stack.
type ToggleGroup struct {
	composite
	row     *Box
	group   radioGroup
	buttons []*ToggleButton
	active  int

	// OnChanged fires after every change of the active toggle, from a
	// click, a key, or SetActive.
	OnChanged func(i int)
}

// NewToggleGroup returns a group over items (labels, ButtonContents,
// icons), the first active.
func NewToggleGroup(items ...Widget) *ToggleGroup {
	g := &ToggleGroup{}
	g.SetElement("togglegroup")
	g.row = NewBox(Row, 0, 0)
	g.initComposite(g, g.row)
	g.boxed = true
	for _, it := range items {
		g.Append(it)
	}
	return g
}

// Append adds a toggle; the first one appended is active. Appending
// never announces: OnChanged reports user and SetActive changes only.
func (g *ToggleGroup) Append(content Widget) {
	i := len(g.buttons)
	t := NewToggleButton(content, 6, 999)
	t.joinGroup(&g.group)
	t.OnToggled = func(on bool) {
		if on && g.active != i {
			g.active = i
			if g.OnChanged != nil {
				g.OnChanged(i)
			}
		}
	}
	g.buttons = append(g.buttons, t)
	g.row.Append(t, false)
	if i == 0 {
		g.active = 0
		t.setActive(true, false)
	}
	g.InvalidateLayout()
}

// Clear removes every toggle.
func (g *ToggleGroup) Clear() {
	g.row.Clear()
	g.buttons, g.active = nil, 0
	g.group = radioGroup{}
	g.InvalidateLayout()
}

// Len reports the toggle count.
func (g *ToggleGroup) Len() int { return len(g.buttons) }

// Active reports the active toggle (-1 when empty).
func (g *ToggleGroup) Active() int {
	if len(g.buttons) == 0 {
		return -1
	}
	return g.active
}

// SetActive activates toggle i (clamped); OnChanged fires when it
// moved.
func (g *ToggleGroup) SetActive(i int) {
	if len(g.buttons) == 0 {
		return
	}
	g.buttons[min(max(i, 0), len(g.buttons)-1)].SetActive(true)
}

// KeyAction moves the active toggle with Left/Right.
func (g *ToggleGroup) KeyAction(a KeyAction, _ Mods) {
	switch a {
	case KeyLeft:
		g.SetActive(g.active - 1)
	case KeyRight:
		g.SetActive(g.active + 1)
	}
}

// ButtonContent is the icon-and-label pair buttons wear (adw
// ButtonContent): either half may be empty.
type ButtonContent struct {
	composite
	row   *Box
	face  render.Font
	size  float64
	icon  *Icon
	label *Label
}

// NewButtonContent returns the pair; an empty icon name or label drops
// that half.
func NewButtonContent(face render.Font, sizePx float64, iconName, label string) *ButtonContent {
	face = requireFace("widget.NewButtonContent", face)
	c := &ButtonContent{face: face, size: sizePx}
	c.row = NewBox(Row, 6, 0)
	c.initComposite(c, c.row)
	c.label = NewLabel(face, sizePx, label, 0)
	c.SetIconName(iconName)
	return c
}

// SetLabel replaces the label text.
func (c *ButtonContent) SetLabel(s string) {
	c.label.SetText(s)
	c.rebuild()
}

// SetIconName replaces the icon; empty drops it.
func (c *ButtonContent) SetIconName(name string) {
	c.icon = nil
	if name != "" {
		c.icon = NewThemeIcon(name, int(c.size))
	}
	c.rebuild()
}

// rebuild lays out whichever halves exist.
func (c *ButtonContent) rebuild() {
	c.row.Clear()
	if c.icon != nil {
		c.row.AppendAligned(c.icon, false, AlignCenter)
	}
	if c.label.Text() != "" {
		c.row.AppendAligned(c.label, false, AlignCenter)
	}
	c.InvalidateLayout()
}

// StatusPage is the empty/error state page (adw StatusPage): an icon on
// a muted tile, a title, a wrapped description, and an optional content
// area (a call-to-action button), centered in whatever space it gets.
// It styles as the `statuspage` element with .icon, .title and
// .description parts.
type StatusPage struct {
	composite
	column  *Box
	media   *Box
	icon    Widget
	title   *Label
	desc    *Label
	content *Box
}

// statusPageWidth is the column's widest.
const statusPageWidth = 384

// NewStatusPage returns the page; empty icon or description drops it.
func NewStatusPage(face render.Font, iconName, title, description string) *StatusPage {
	face = requireFace("widget.NewStatusPage", face)
	p := &StatusPage{}
	p.SetElement("statuspage")
	p.column = NewBox(Column, 8, 24)
	p.column.Append(NewSpacer(0, 0), true)
	p.media = NewBox(Column, 0, 0)
	p.media.AddClass("icon")
	p.media.SetSizeRequest(40, 40)
	p.title = NewLabel(face, 18, title, 0)
	p.title.AddClass("title")
	p.title.SetAlignment(render.AlignCenter)
	p.column.AppendAligned(p.title, false, AlignCenter)
	p.desc = NewLabel(face, 14, description, 0)
	p.desc.AddClass("description")
	p.desc.SetWrap(true)
	p.desc.SetAlignment(render.AlignCenter)
	if description != "" {
		p.column.AppendAligned(p.desc, false, AlignFill)
	}
	p.content = NewBox(Column, 16, 0)
	p.content.SetPadding(render.Insets{Top: 16})
	p.column.Append(NewSpacer(0, 0), true)
	if iconName != "" {
		p.SetIcon(NewThemeIcon(iconName, 24))
	}
	p.initComposite(p, NewClamp(statusPageWidth, p.column))
	p.boxed = true
	return p
}

// SetIcon shows w (an icon, typically 24px) centered on the muted
// tile above the title; nil drops the tile.
func (p *StatusPage) SetIcon(w Widget) {
	p.media.Clear()
	switch {
	case w != nil && p.icon == nil:
		p.column.InsertAligned(1, p.media, false, AlignCenter)
	case w == nil && p.icon != nil:
		p.column.Remove(p.media)
	}
	p.icon = w
	if w != nil {
		p.media.Append(NewSpacer(0, 0), true)
		p.media.AppendAligned(w, false, AlignCenter)
		p.media.Append(NewSpacer(0, 0), true)
	}
	p.InvalidateLayout()
}

// SetChild adds w (a call-to-action button, typically) to the content
// area under the description; successive children stack.
func (p *StatusPage) SetChild(w Widget) {
	if len(p.content.Children()) == 0 {
		n := len(p.column.Children())
		p.column.InsertAligned(n-1, p.content, false, AlignCenter)
	}
	p.content.AppendAligned(w, false, AlignCenter)
	p.InvalidateLayout()
}

// Banner is the strip that slides down to announce a state (adw
// Banner): a title, an optional action button, revealed and hidden
// through SetRevealed on a slide.
type Banner struct {
	composite
	reveal *Revealer
	title  *Label
	button *Button

	// OnButton fires when the action button is clicked.
	OnButton func()
}

// NewBanner returns a hidden banner; an empty button label shows no
// button.
func NewBanner(face render.Font, sizePx float64, title, buttonLabel string) *Banner {
	face = requireFace("widget.NewBanner", face)
	b := &Banner{}
	b.SetElement("banner")
	b.title = NewLabel(face, sizePx, title, 0)
	row := NewBox(Row, 12, 6)
	row.Append(NewSpacer(0, 0), true)
	row.AppendAligned(b.title, false, AlignCenter)
	if buttonLabel != "" {
		b.button = NewButton(NewLabel(face, sizePx, buttonLabel, 0), 6, 4)
		b.button.OnClick = func() {
			if b.OnButton != nil {
				b.OnButton()
			}
		}
		row.AppendAligned(b.button, false, AlignCenter)
	}
	row.Append(NewSpacer(0, 0), true)
	b.reveal = NewRevealer(row)
	b.reveal.SetTransition(RevealSlideDown)
	b.reveal.SetDuration(200 * time.Millisecond)
	b.reveal.SetRevealed(false)
	b.initComposite(b, b.reveal)
	b.fillWidth = true
	b.surface = func(th *Theme) Color { return th.HoverSurface() }
	return b
}

// SetTitle replaces the message.
func (b *Banner) SetTitle(s string) { b.title.SetText(s) }

// SetRevealed slides the banner in or out.
func (b *Banner) SetRevealed(on bool) { b.reveal.SetRevealed(on) }

// Revealed reports whether the banner is showing.
func (b *Banner) Revealed() bool { return b.reveal.Revealed() }

// Avatar is the round portrait: an image cropped
// to a circle, or the fallback of the name's initials on the muted
// fill, a person's outline for a name without any. It styles as the `avatar` element: background-color is the
// fallback's fill and color its initials.
type Avatar struct {
	node
	face render.Font
	size int
	name string
	img  image.Image
}

// NewAvatar returns a size-pixel avatar for name.
func NewAvatar(face render.Font, size int, name string) *Avatar {
	a := &Avatar{face: requireFace("widget.NewAvatar", face), size: size, name: name}
	a.SetElement("avatar")
	return a
}

// SetImage shows img instead of the initials; nil returns to them. The
// image is handed over read-only, as to NewImage.
func (a *Avatar) SetImage(img image.Image) {
	a.img = heldImage(img)
	a.Invalidate()
}

// SetName changes the name and so the initials.
func (a *Avatar) SetName(name string) {
	a.name = name
	a.Invalidate()
}

// Initials are the name's first letters, at most two: the first and
// the last word's, uppercased.
func (a *Avatar) Initials() string {
	words := strings.FieldsFunc(a.name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) == 0 {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(words[0])
	out := string(unicode.ToUpper(first))
	if len(words) > 1 {
		last, _ := utf8.DecodeRuneInString(words[len(words)-1])
		out += string(unicode.ToUpper(last))
	}
	return out
}

// Measure wants the avatar's square.
func (a *Avatar) Measure(con Constraints) Size {
	return clampSize(Size{W: a.size, H: a.size}, con)
}

// Arrange records the rect.
func (a *Avatar) Arrange(r render.Rect) { a.node.Arrange(r) }

// Paint draws the cropped image or the fallback disc with initials.
func (a *Avatar) Paint(cv *render.Canvas) {
	d := min(a.bounds.W, a.bounds.H)
	r := render.Rect{X: a.bounds.X + (a.bounds.W-d)/2, Y: a.bounds.Y + (a.bounds.H-d)/2, W: d, H: d}
	if a.img != nil {
		cv.DrawImageCover(a.img, r, render.UniformCorners(d/2))
		return
	}
	v, th := a.style(a), Current()
	cv.RoundedRect(r, d/2, pickc(0, v, style.PropBackgroundColor, th.MutedColor()))
	fg := pickc(0, v, style.PropColor, th.Text)
	if initials := a.Initials(); initials != "" {
		a.face.DrawAligned(cv, initials, r, float64(d)*0.4, fg, render.AlignCenter)
		return
	}
	paintSymbol(cv, SymbolUser, r, d/2, fg)
}

// HitTest resolves inside the bounds.
func (a *Avatar) HitTest(p Point) Widget { return a.HitLeaf(a, p) }

// SplitButton is a button with a menu side (adw SplitButton): the main
// half fires OnClick, the arrow half reports its anchor through OnMenu
// - the app opens the menu popover there (app.AttachSplitButton), the
// same reporting shape a MenuBar's roots use.
type SplitButton struct {
	composite
	main  *Button
	arrow *Button

	// OnClick fires for the main half; OnMenu for the arrow, with the
	// arrow as the popover anchor.
	OnClick func()
	OnMenu  func(anchor Boundser)
}

// NewSplitButton returns the pair over content (a label or
// ButtonContent).
func NewSplitButton(content Widget, sizePx float64) *SplitButton {
	s := &SplitButton{}
	s.SetElement("splitbutton")
	s.main = NewButton(content, 6, 4)
	s.main.OnClick = func() {
		if s.OnClick != nil {
			s.OnClick()
		}
	}
	s.arrow = NewButton(NewSymbol(SymbolChevronDown, int(sizePx)), 4, 4)
	s.arrow.OnClick = func() {
		if s.OnMenu != nil {
			s.OnMenu(s.arrow)
		}
	}
	row := NewBox(Row, 1, 0)
	row.AppendAligned(s.main, false, AlignFill)
	row.AppendAligned(s.arrow, false, AlignFill)
	s.initComposite(s, row)
	return s
}

// Arrow returns the menu half, the popover anchor.
func (s *SplitButton) Arrow() Boundser { return s.arrow }
