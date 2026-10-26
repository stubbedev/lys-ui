package widget

import (
	"math"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// Switch is a boolean toggle painted as a pill with a sliding knob. A
// stylesheet restyles it the GTK way: `switch` is the track (background,
// border-radius, min-width/min-height, the box layers; `:checked` while
// on) and `switch slider` the knob (min size, margin, background,
// radius, shadow). Unstyled, it is the theme's 40x22 pill. A flip
// slides the knob and blends the colors over the theme's medium motion.
type Switch struct {
	node
	on   bool
	knob stylePart
	// progress is the knob's travel: 0 off, 1 on, animated between on
	// a flip. The knob's place and the theme colors hang off it.
	progress float64
	cancel   anim.Cancel

	// OnChanged fires after every state change, including programmatic
	// ones.
	OnChanged func(on bool)
}

// NewSwitch returns a switch in the given state. The pointer turns to
// the hand over it, the way wayle's switch primitive asks.
func NewSwitch(on bool) *Switch {
	s := &Switch{on: on, progress: switchTarget(on)}
	s.cursorName = "pointer"
	s.knob.SetElement("slider")
	s.SetState(StateChecked, on)
	return s
}

// On reports the switch state.
func (s *Switch) On() bool { return s.on }

// SetOn changes the state and fires OnChanged when it flipped.
func (s *Switch) SetOn(on bool) {
	if on == s.on {
		return
	}
	s.on = on
	s.SetState(StateChecked, on)
	s.animate()
	if s.OnChanged != nil {
		s.OnChanged(on)
	}
}

// switchTarget is the progress a state rests at.
func switchTarget(on bool) float64 {
	if on {
		return 1
	}
	return 0
}

// animate moves progress toward the state over the theme's medium
// motion. A switch not yet arranged lands at once: nothing animates
// while hidden.
func (s *Switch) animate() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	to := switchTarget(s.on)
	if s.bounds.Empty() {
		s.progress = to
		s.Invalidate()
		return
	}
	from := s.progress
	_, medium, _ := Current().MotionDurations()
	s.cancel = anim.Play(anim.Animate(medium, func(t float64) {
		s.progress = from + (to-from)*t
		if t >= 1 {
			s.progress = to
		}
		s.placeKnob()
		s.Invalidate()
	}))
}

// Toggle flips the state and fires OnChanged.
func (s *Switch) Toggle() {
	s.SetOn(!s.on)
}

// Measure wants the 32x18 pill, floored by the stylesheet's min size,
// inside its CSS box, clamped to con.
func (s *Switch) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	v := s.style(s)
	return s.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(s.pill(v), inner)
	}))
}

// pill is the track's content size: the 32x18 pill, or none under the
// stylesheet's min size, which then floors it.
func (s *Switch) pill(v *style.Values) Size {
	if v.HasAny(style.PropMinWidth, style.PropMinHeight) {
		return Size{}
	}
	return Size{W: 32, H: 18}
}

// Arrange keeps the border box inside the margins, at the track's
// natural size: a taller or wider slot (a row's cross stretch) centers
// the pill across and keeps it at the start, the way a checkbox keeps
// its indicator, instead of stretching the knob with it. The knob takes
// its place here, not in Paint, so a first paint leaves nothing for the
// next frame to repaint.
func (s *Switch) Arrange(r render.Rect) {
	v := s.style(s)
	b := boxOf(v, render.Insets{})
	border, _ := boxRects(b, r)
	in := b.padding
	bw := b.border
	p := s.pill(v)
	w := max(p.W, picki(v, style.PropMinWidth, 0)) + in.Left + in.Right + bw.Left + bw.Right
	h := max(p.H, picki(v, style.PropMinHeight, 0)) + in.Top + in.Bottom + bw.Top + bw.Bottom
	if border.H > h {
		border.Y += (border.H - h) / 2
		border.H = h
	}
	border.W = min(border.W, w)
	s.node.Arrange(border)
	setParents(s, &s.knob)
	s.placeKnob()
}

// placeKnob arranges the knob at the track's start, its end when on,
// and progress of the way between while a flip slides it, inset by the
// knob's margins (1px unstyled).
func (s *Switch) placeKnob() {
	v := s.style(s)
	content := boxOf(v, render.Insets{}).padding.Shrink(borderOf(v).Shrink(s.bounds))
	kv := s.knob.style(&s.knob)
	m := render.UniformInsets(1)
	if kv.HasAny(style.PropMarginTop, style.PropMarginRight, style.PropMarginBottom, style.PropMarginLeft) {
		m = marginOf(kv)
	}
	d := max(0, content.H-m.Top-m.Bottom)
	kw, kh := picki(kv, style.PropMinWidth, d), picki(kv, style.PropMinHeight, d)
	off, on := content.X+m.Left, content.X+content.W-m.Right-kw
	kx := off + int(math.Round(float64(on-off)*s.progress))
	s.knob.Arrange(render.Rect{X: kx, Y: content.Y + m.Top + (d-kh)/2, W: kw, H: kh})
}

// Paint draws the track and knob. The knob sits at the right when on,
// in the accent's foreground over the accent track; off, the track is
// the input color under a knob that contrasts it (the window background
// on a light palette, the text color on a dark one). A flip blends the
// two by progress.
// Disabled, the theme's on-track and knob fade through the derived
// disabled colors (a stylesheet fades with its own :disabled rules).
func (s *Switch) Paint(cv *render.Canvas) {
	t := Current()
	v := s.style(s)
	offTrack, offKnob := t.InputColor(), t.Bg
	if t.IsDark() {
		offKnob = t.Text
	}
	onTrack, onKnob := accentOf(v, t.Accent), t.OnAccent
	if !IsEnabled(s) {
		offTrack, offKnob = t.DisabledSurface(), t.DisabledText()
		onTrack, onKnob = accentOf(v, t.DisabledAccent()), t.DisabledText()
	}
	track, knob := mix(offTrack, onTrack, s.progress), mix(offKnob, onKnob, s.progress)
	radii := radiusOr(v, s.bounds.H/2)
	fx := pushEffects(cv, v, s.bounds, radii)
	cv = fx.cv
	defer fx.pop()
	bw := borderOf(v)
	paintBoxBehind(cv, v, s.bounds, radii, bw, pickc(0, v, style.PropBackgroundColor, track))
	defer paintOutline(cv, v, s.bounds, radii)

	kv := s.knob.style(&s.knob)
	knobRect := s.knob.Bounds()
	kr := radiusOr(kv, min(knobRect.W, knobRect.H)/2)
	paintBoxBehind(cv, kv, knobRect, kr, borderOf(kv), pickc(0, kv, style.PropBackgroundColor, knob))
}

// Role implements Roleer.
func (s *Switch) Role() Role { return RoleSwitch }

// HitTest returns the switch when p is inside its bounds.
func (s *Switch) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

// ClickAt toggles the switch; the Router invokes it on press+release.
// Disabled switches ignore clicks.
func (s *Switch) ClickAt(Point) {
	if IsEnabled(s) {
		s.Toggle()
	}
}

// SetPressed is a no-op: the switch has no pressed visual.
func (s *Switch) SetPressed(bool) {}

// KeyAction implements KeyActionHandler: Enter toggles when focused,
// which makes the switch reachable and activatable by keyboard only.
// Disabled switches ignore keys.
func (s *Switch) KeyAction(a KeyAction, _ Mods) {
	if a == KeyEnter && IsEnabled(s) {
		s.Toggle()
	}
}

// InsertRune implements RuneHandler: Space toggles, the other half of
// the GTK activation pair. Disabled switches ignore it.
func (s *Switch) InsertRune(r rune) {
	if r == ' ' && IsEnabled(s) {
		s.Toggle()
	}
}

// CheckButton is a boolean checkbox with a painted check mark, and an
// optional child (its label) beside the mark that clicks toggle too.
type CheckButton struct {
	node
	checked bool
	// check is the `check` node under the checkbox (`checkbutton >
	// check`): the drawn indicator, styled and painted through it.
	check stylePart
	// child sits after the check (`checkbutton > label`); indicator is
	// the arranged check's box.
	child     Widget
	indicator render.Rect
	// mark is the themed icon the check node's -gtk-icon-source names,
	// cached per name and tint.
	mark     *Icon
	markName string
	markTint render.Color
	// group, when set, makes the checkbox a radio button (SetGroup).
	group *radioGroup

	// OnChanged fires after every state change, including programmatic
	// ones.
	OnChanged func(checked bool)
}

// NewCheckButton returns a checkbox in the given state. The pointer
// turns to the hand over it, the way wayle's checkbox primitive asks.
func NewCheckButton(checked bool) *CheckButton {
	c := &CheckButton{checked: checked}
	c.cursorName = "pointer"
	c.check.SetElement("check")
	setParents(c, &c.check)
	return c
}

// appendStyleKids is the check and the child (styleKids).
func (c *CheckButton) appendStyleKids(buf []Widget) []Widget {
	buf = append(buf, &c.check)
	if c.child != nil {
		buf = append(buf, c.child)
	}
	return buf
}

// SetChild places w after the check (gtk_check_button_set_child): a
// label, typically, styled as `checkbutton > label`. The checkbox stays
// the leaf: a click on the child toggles it. nil removes the child.
func (c *CheckButton) SetChild(w Widget) {
	if c.child != nil {
		notifyRemoved(c.child)
		clearParents(c.child)
	}
	c.child = w
	if w != nil {
		setParents(c, w)
	}
	restyleChildren(c)
	c.InvalidateLayout()
}

// Child is the widget SetChild placed, nil for none.
func (c *CheckButton) Child() Widget { return c.child }

// Checked reports the state.
func (c *CheckButton) Checked() bool { return c.checked }

// SetChecked changes the state and fires OnChanged when it flipped.
func (c *CheckButton) SetChecked(checked bool) {
	if checked == c.checked {
		return
	}
	c.checked = checked
	c.invalidateState(style.Checked)
	c.Invalidate()
	if checked && c.group != nil {
		c.group.activated(c)
	}
	if c.OnChanged != nil {
		c.OnChanged(checked)
	}
}

// Toggle flips the state and fires OnChanged; a radio button only ever
// checks - clicking the checked one keeps it checked.
func (c *CheckButton) Toggle() {
	if c.group != nil && c.checked {
		return
	}
	c.SetChecked(!c.checked)
}

// SetInconsistent marks the check indeterminate (`:indeterminate`),
// the neither-checked-nor-unchecked state a tri-state control shows.
func (c *CheckButton) SetInconsistent(on bool) { c.SetState(StateIndeterminate, on) }

// Inconsistent reports the indeterminate state.
func (c *CheckButton) Inconsistent() bool { return c.HasState(StateIndeterminate) }

// indicatorSize is the check's min-width/min-height, the theme's
// 20x20 unstyled.
func (c *CheckButton) indicatorSize() Size {
	kv := c.check.style(&c.check)
	return Size{W: picki(kv, style.PropMinWidth, 16), H: picki(kv, style.PropMinHeight, 16)}
}

// Measure sizes the indicator and the child after it, inside the
// checkbox's CSS box.
func (c *CheckButton) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	ind := c.indicatorSize()
	v := c.style(c)
	return c.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		sz := ind
		if c.child != nil {
			cs := measureChild(c, c.child, Constraints{Max: Size{W: max(0, inner.Max.W-ind.W), H: inner.Max.H}})
			sz = Size{W: ind.W + cs.W, H: max(ind.H, cs.H)}
		}
		return clampSize(sz, inner)
	}))
}

// Arrange records the box. Without a child the check fills the
// content box; with one it keeps its size at the start, centered
// across, and the child takes the rest.
func (c *CheckButton) Arrange(r render.Rect) {
	c.node.Arrange(r)
	v := c.style(c)
	content := paddingOr(v, render.Insets{}).Shrink(borderOf(v).Shrink(c.bounds))
	if c.child == nil {
		c.indicator = content
		return
	}
	ind := c.indicatorSize()
	ind.W, ind.H = min(ind.W, content.W), min(ind.H, content.H)
	c.indicator = render.Rect{X: content.X, Y: content.Y + (content.H-ind.H)/2, W: ind.W, H: ind.H}
	arrangeChild(c.child, render.Rect{X: content.X + ind.W, Y: content.Y, W: content.W - ind.W, H: content.H})
}

// checkRing is the unstyled indicator's border width.
const checkRing = 2

// Paint draws the indicator through the check node: the box layers
// (background, border, radius) it resolves, the tick or minus bar in
// its color. Unstyled, a 2px theme-border ring around the theme
// surface, accent-filled with a mark while checked or inconsistent.
// Disabled, ring and mark fade through the derived disabled colors.
func (c *CheckButton) Paint(cv *render.Canvas) {
	t := Current()
	kv := c.check.style(&c.check)
	fx := pushEffects(cv, kv, c.bounds, radiusOr(kv, min(c.bounds.W, c.bounds.H)/2))
	cv = fx.cv
	defer fx.pop()
	box := c.indicator
	if box.W > box.H {
		box.W = box.H
	}
	if box.H > box.W {
		box.H = box.W
	}
	var fill, tick render.Color
	v := c.style(c)
	if c.checked || c.Inconsistent() {
		fill, tick = accentOf(v, t.Accent), pickc(0, kv, style.PropColor, t.OnAccent)
		if !IsEnabled(c) {
			fill, tick = accentOf(v, t.DisabledAccent()), scaleAlpha(tick, disabledFade)
		}
	} else {
		fill = t.Bg
	}
	ring := ringOr(kv, checkRing)
	cols := borderColors(kv)
	if !kv.HasAny(style.PropBorderTopWidth, style.PropBorderRightWidth, style.PropBorderBottomWidth, style.PropBorderLeftWidth) && !IsEnabled(c) {
		cols = [4]render.Color{t.DisabledText(), t.DisabledText(), t.DisabledText(), t.DisabledText()}
	}
	radii := radiusOr(kv, 4)
	if c.group != nil {
		radii = radiusOr(kv, box.W/2)
	}
	c.check.Arrange(box)
	paintBoxBehindCol(cv, kv, box, radii, ring, pickc(0, kv, style.PropBackgroundColor, fill), cols)
	if src := kv.IconSource; src != "" && (c.checked || c.Inconsistent()) {
		// The stylesheet's themed mark (`-gtk-icon-source`) replaces the
		// painted tick, recolored by its palette over the mark color.
		tint := kv.PaletteTint
		if tint == 0 {
			tint = tick
		}
		if !IsEnabled(c) {
			tint = scaleAlpha(tint, disabledFade)
		}
		c.drawThemedMark(cv, src, box, tint)
	} else if c.group != nil && c.checked {
		// The radio's mark: a dot half the indicator.
		d := max(box.W/2, 2)
		cv.RoundedRect(render.Rect{X: box.X + (box.W-d)/2, Y: box.Y + (box.H-d)/2, W: d, H: d}, d/2, tick)
	} else if c.checked || c.Inconsistent() {
		drawCheckMark(cv, box, c.Inconsistent(), tick)
	}
	paintOutline(cv, kv, box, radii)
	if c.child != nil {
		PaintChild(cv, c.child)
	}
}

// drawThemedMark draws the check node's -gtk-icon-source icon centered
// in the indicator, re-tinted when the color moved on.
func (c *CheckButton) drawThemedMark(cv *render.Canvas, name string, box render.Rect, tint render.Color) {
	if c.mark == nil || c.markName != name || c.markTint != tint {
		size := max(8, min(box.W, box.H)-2*checkRing)
		c.mark = NewThemeIcon(name, size)
		c.mark.SetTint(tint)
		c.markName, c.markTint = name, tint
		setParents(c, c.mark)
	}
	c.mark.Arrange(render.Rect{
		X: box.X + (box.W-c.mark.Bounds().W)/2, Y: box.Y + (box.H-c.mark.Bounds().H)/2,
		W: c.mark.Bounds().W, H: c.mark.Bounds().H,
	})
	c.mark.Measure(Constraints{Max: Size{W: box.W, H: box.H}})
	PaintChild(cv, c.mark)
}

// drawCheckMark paints the mark inside an indicator box: the tick, or
// the minus bar when indeterminate.
func drawCheckMark(cv *render.Canvas, box render.Rect, minus bool, col render.Color) {
	bw, bh := float64(box.W), float64(box.H)
	stroke := max(2, box.W/7)
	if minus {
		y := box.Y + box.H/2
		cv.Line(box.X+int(0.26*bw), y, box.X+int(0.74*bw), y, stroke, col)
		return
	}
	x0, y0 := box.X+int(0.24*bw), box.Y+int(0.55*bh)
	x1, y1 := box.X+int(0.42*bw), box.Y+int(0.73*bh)
	x2, y2 := box.X+int(0.78*bw), box.Y+int(0.27*bh)
	cv.Line(x0, y0, x1, y1, stroke, col)
	cv.Line(x1, y1, x2, y2, stroke, col)
}

// Role implements Roleer.
func (c *CheckButton) Role() Role { return RoleCheckBox }

// HitTest returns the checkbox when p is inside its bounds.
func (c *CheckButton) HitTest(p Point) Widget {
	return c.HitLeaf(c, p)
}

// ClickAt toggles the checkbox; the Router invokes it on press+release.
// Disabled checkboxes ignore clicks.
func (c *CheckButton) ClickAt(Point) {
	if IsEnabled(c) {
		c.Toggle()
	}
}

// KeyAction implements KeyActionHandler: Enter toggles when focused,
// which makes the checkbox reachable and activatable by keyboard only.
// Disabled checkboxes ignore keys.
func (c *CheckButton) KeyAction(a KeyAction, _ Mods) {
	if a == KeyEnter && IsEnabled(c) {
		c.Toggle()
	}
}

// InsertRune implements RuneHandler: Space toggles, the other half of
// the GTK activation pair. Disabled checkboxes ignore it.
func (c *CheckButton) InsertRune(r rune) {
	if r == ' ' && IsEnabled(c) {
		c.Toggle()
	}
}

// Spacer is empty layout space of a fixed size.
type Spacer struct {
	node
	nat Size
}

// NewSpacer returns a spacer wanting w x h.
func NewSpacer(w, h int) *Spacer {
	return &Spacer{nat: Size{W: w, H: h}}
}

// Measure returns the spacer's size, clamped to con.
func (s *Spacer) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	return s.measureStore(con, clampSize(s.nat, con))
}

// Paint paints nothing.
func (s *Spacer) Paint(*render.Canvas) {}

// HitTest returns nil: empty space is not interactive.
func (s *Spacer) HitTest(Point) Widget { return nil }

// appendStyleKids is the knob (styleKids).
func (s *Switch) appendStyleKids(buf []Widget) []Widget { return append(buf, &s.knob) }
