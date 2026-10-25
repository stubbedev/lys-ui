package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/render"
)

const (
	// toastSlide is the entrance rise in pixels, masked to the card's
	// own bounds so the motion stays inside its damage rect. The
	// enter/exit durations and curves come from the surfx per-kind
	// table (KindToast), so overrides and the reduced-motion switches
	// apply to toasts like every other surface.
	toastSlide = 12
	// toastPadX/toastPadY inset the card's content, Sonner's 16px;
	// toastGap separates text from the action button, whose own insets
	// and radius follow.
	toastPadX = 16
	toastPadY = 16
	toastGap  = 12
	// toastLineGap separates the title from the description under it.
	toastLineGap = 2
	// toastBtnPadX insets the action label inside its hit rect.
	toastBtnPadX   = 8
	toastBtnPadY   = 4
	toastBtnRadius = 4
)

// Toast is a transient feedback card: a line of text, an optional
// muted description under it, an optional action button, an auto-dismiss timer that hover pauses (hovering
// cancels the pending dismissal — even a mid-exit toast snaps back —
// and leaving arms a fresh full timeout), and a slide-and-fade
// entrance plus fade-out exit, all on the animation clock. A toast out
// of the tree is inert: it holds no timer, so a detached toast costs
// no wakeups.
//
// The host learns the toast is gone from OnDismissed, which fires
// exactly once when the exit fade lands, and detaches it there.
type Toast struct {
	node
	face    render.Font
	text    string
	desc    string
	sizePx  float64
	timeout time.Duration

	action   string
	OnAction func()

	// OnDismissed fires once when the toast finished going away — the
	// exit fade landed, or Dismiss hit a toast that was out of the
	// tree. The host detaches it inside.
	OnDismissed func()

	// progress is the visual reveal: 0 hidden, 1 shown. The entrance
	// raises it, the exit lowers it, hover rescue snaps it back.
	progress float64
	hovered  bool
	hoverPt  Point
	// leaving marks an exit fade in flight — the rescue path on hover.
	leaving bool
	// lastDy is the previous animated frame's slide offset, the delta's
	// other edge (slideExtent).
	lastDy      int
	actionRect  render.Rect
	ring        shadowTracker
	cancelSlide anim.Cancel
	cancelTimer anim.Cancel
	visible     bool
	dismissed   bool
}

// NewToast returns a toast showing text. A positive timeout
// auto-dismisses the card once it is on screen; zero or negative
// keeps it until Dismiss. Face may be a render.Chain for mixed-script
// fallback. A nil face panics here (see requireFace) instead of
// failing later, in shaping.
func NewToast(face render.Font, text string, timeout time.Duration) *Toast {
	return &Toast{face: requireFace("widget.NewToast", face), text: text, sizePx: Current().TextSize, timeout: timeout}
}

// Text returns the card's text.
func (t *Toast) Text() string { return t.text }

// SetText replaces the card's text.
func (t *Toast) SetText(text string) {
	if t.text == text {
		return
	}
	t.text = text
	t.InvalidateLayout()
}

// Description returns the line under the text, empty when there is
// none.
func (t *Toast) Description() string { return t.desc }

// SetDescription sets the muted line under the text; empty removes it.
func (t *Toast) SetDescription(desc string) {
	if t.desc == desc {
		return
	}
	t.desc = desc
	t.InvalidateLayout()
}

// textHeight is the text block's height: the line, and the description
// under it when there is one.
func (t *Toast) textHeight() int {
	h := t.face.Shape(t.text, t.sizePx).LineHeight()
	if t.desc != "" {
		h += toastLineGap + t.face.Shape(t.desc, t.sizePx).LineHeight()
	}
	return h
}

// SetAction attaches the optional action button; an empty label
// removes it. Firing the action dismisses the toast, the standard
// toast contract.
func (t *Toast) SetAction(label string, fn func()) {
	t.action = label
	t.OnAction = fn
	t.InvalidateLayout()
}

// Action returns the action label, empty when there is none.
func (t *Toast) Action() string { return t.action }

// Closed reports whether the toast is permanently gone.
func (t *Toast) Closed() bool { return t.dismissed }

// SetHovered implements HoverSetter: hover cancels the pending
// dismissal — the timer goes away and a mid-exit toast snaps back —
// and leaving arms a fresh full timeout.
func (t *Toast) SetHovered(on bool) {
	if on == t.hovered {
		return
	}
	t.hovered = on
	t.styleDirty = true
	markDamage()
	if on {
		t.stopTimer()
		if t.leaving {
			// Rescue: the card comes back, the entrance (if one is
			// still running) must not fight the restored value.
			t.leaving = false
			t.stopSlide()
			t.progress = 1
			t.invalidateFrame()
		}
		return
	}
	t.armTimer()
}

// HoverMove tracks the pointer for the action button's hover shade.
func (t *Toast) HoverMove(p Point) {
	if t.hoverPt == p {
		return
	}
	t.hoverPt = p
	t.Invalidate()
}

// toastPlan resolves the toast kind's tween profile: the surfx table
// gated by the theme's motion switch (the package toggle and anim's
// reduced-motion state fold in inside Plan).
func toastPlan() surfx.Style { return surfx.Plan(surfx.KindToast, Current().Animations) }

// armTimer schedules the exit: hold for the timeout minus the fade,
// then fade out. It launches only on a visible toast — a hidden one
// holds no timer.
func (t *Toast) armTimer() {
	if t.timeout <= 0 || t.dismissed || !t.visible {
		return
	}
	t.stopTimer()
	st := toastPlan()
	fade := min(st.Exit.Duration, t.timeout)
	t.cancelTimer = anim.Play(
		anim.Delay(t.timeout-fade),
		anim.Animate(fade, t.fadeOut).Easing(st.Exit.Easing),
	)
}

// stopTimer cancels a pending dismissal.
func (t *Toast) stopTimer() {
	if t.cancelTimer != nil {
		t.cancelTimer()
		t.cancelTimer = nil
	}
}

// stopSlide cancels the entrance tween.
func (t *Toast) stopSlide() {
	if t.cancelSlide != nil {
		t.cancelSlide()
		t.cancelSlide = nil
	}
}

// fadeOut is the exit tween: progress back to zero, then the toast is
// gone and the host detaches it. The first call marks the exit as in
// flight, so a timeout-driven fade is rescuable exactly like an
// explicit Dismiss.
func (t *Toast) fadeOut(p float64) {
	if p >= 1 {
		t.progress = 0
		t.leaving = false
		t.invalidateFrame()
		t.finish()
		return
	}
	t.leaving = true
	t.progress = 1 - p
	t.invalidateFrame()
}

// finish marks the toast gone and fires OnDismissed exactly once.
func (t *Toast) finish() {
	if t.dismissed {
		return
	}
	t.dismissed = true
	t.stopSlide()
	t.stopTimer()
	if t.OnDismissed != nil {
		t.OnDismissed()
	}
}

// Dismiss starts the exit fade now. A toast out of the tree has no
// pixels to fade, so it finishes on the spot.
func (t *Toast) Dismiss() {
	if t.dismissed || t.leaving {
		return
	}
	if !t.visible {
		t.finish()
		return
	}
	t.leaving = true
	t.stopTimer()
	st := toastPlan()
	t.cancelTimer = anim.Play(anim.Animate(st.Exit.Duration, t.fadeOut).Easing(st.Exit.Easing))
}

// Close tears the toast down without a fade and without firing
// OnDismissed — the host calling this is already detaching it.
func (t *Toast) Close() {
	t.dismissed = true
	t.leaving = false
	t.progress = 0
	t.stopSlide()
	t.stopTimer()
	t.invalidateFrame()
}

// invalidateFrame schedules the repaint one animated frame owes: the
// bounds, plus the shadow ring grown by the slide distance the card
// moved through since its previous frame — the union of the two
// positions — never the full entrance rise. The fade repaints the
// card in place; only the motion adds region.
func (t *Toast) invalidateFrame() {
	t.Invalidate()
	if ext := t.slideExtent(); ext != t.bounds {
		t.InvalidateRect(ext)
	}
}

// slideExtent is the repaint region the animated card can touch
// beyond its bounds: the shadow ring grown by the larger of the last
// and current slide offsets, which covers both edges of the move.
func (t *Toast) slideExtent() render.Rect {
	_, blur := effShadow(t, Current())
	ring := ringFor(t.bounds, blur)
	if ring.Empty() {
		return t.bounds
	}
	dy := int((1 - math01(t.progress)) * toastSlide)
	ext := expandRect(ring, max(dy, t.lastDy))
	t.lastDy = dy
	return ext
}

// paintExtent returns the rect the card's paint can touch: the bare
// bounds while the shadow is off (the masked-slide contract — motion
// never paints outside its damage rect), otherwise the shadow ring
// grown by the slide, which the moving card and its falloff never
// exceed.
func (t *Toast) paintExtent() render.Rect {
	_, blur := effShadow(t, Current())
	ring := ringFor(t.bounds, blur)
	if ring.Empty() {
		return t.bounds
	}
	return expandRect(ring, toastSlide)
}

// Arrange records the rect. The first visible arrangement starts the
// entrance and arms the auto-dismiss timer; going off screen stops
// both — nothing animates while hidden.
func (t *Toast) Arrange(r render.Rect) {
	t.node.Arrange(r)
	_, blur := effShadow(t, Current())
	t.ring.syncRing(&t.node, ringFor(t.bounds, blur))
	vis := !r.Empty()
	if vis == t.visible {
		t.layoutAction()
		return
	}
	t.visible = vis
	switch {
	case vis && !t.dismissed:
		t.stopSlide()
		t.stopTimer()
		t.progress = 0
		t.lastDy = toastSlide
		t.cancelSlide = surfx.Rise(toastPlan().Enter, 0, func(p float64) {
			t.progress = min(p, 1)
			t.invalidateFrame()
		})
		t.armTimer()
	default:
		// Out of the tree (or already gone): hold nothing.
		t.stopSlide()
		t.stopTimer()
		t.leaving = false
		t.progress = 0
		t.lastDy = toastSlide
	}
	t.layoutAction()
}

// layoutAction places the action button's hit rect inside the card.
func (t *Toast) layoutAction() {
	t.actionRect = render.Rect{}
	if t.action == "" || t.bounds.Empty() || t.face == nil {
		return
	}
	h := min(t.bounds.H, t.face.Shape("lg", t.sizePx).LineHeight()+2*toastBtnPadY)
	t.actionRect = render.Rect{
		X: t.bounds.X + t.bounds.W - toastPadX - t.actionWidth(),
		Y: t.bounds.Y + max(0, (t.bounds.H-h)/2),
		W: t.actionWidth(),
		H: h,
	}
}

// actionWidth is the action hit rect's width.
func (t *Toast) actionWidth() int {
	if t.face == nil {
		return 0
	}
	return 2*toastBtnPadX + int(t.face.Shape(t.action, t.sizePx).Advance()+0.5)
}

// Measure wants the text plus the action at the theme text size,
// padded to a card; the host clamps the width.
func (t *Toast) Measure(con Constraints) Size {
	if sz, ok := t.measureHit(con); ok {
		return sz
	}
	w := 2 * toastPadX
	h := 2 * toastPadY
	if t.face != nil {
		w += int(max(t.face.Shape(t.text, t.sizePx).Advance(), t.face.Shape(t.desc, t.sizePx).Advance()) + 0.5)
		h += t.textHeight()
	}
	if t.action != "" {
		w += toastGap + t.actionWidth()
		if t.face != nil {
			h = max(h, t.face.Shape("lg", t.sizePx).LineHeight()+2*toastBtnPadY+2*toastPadY)
		}
	}
	return t.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Paint draws the card, its text, and the action button at the current
// reveal: alpha fades with progress, and the entrance rise is masked
// to the paint extent — the card's bounds, widened by the theme shadow
// ring plus the slide when one is owed — so the animation never paints
// outside its damage rect. The shadow rides the card (it shifts with
// the same dy), which is why it clips to the extent, not the bounds.
// The stylesheet's box-shadow, background-color, border, border-radius,
// and color override the theme's; the action is Sonner's small button
// on the primary fill.
func (t *Toast) Paint(cv *render.Canvas) {
	p := math01(t.progress)
	if p <= 0 || t.bounds.Empty() {
		return
	}
	th := Current()
	v := t.style(t)
	col, blur := effShadow(t, th)
	radii := radiusOr(v, th.Radius)
	prev := cv.PushClip(t.paintExtent())
	defer cv.PopClip(prev)
	defer cv.PopAlpha(cv.PushAlpha(p))
	dy := int((1 - p) * toastSlide)
	card := t.bounds
	card.Y += dy
	if blur > 0 {
		cv.Shadow(card, radii.TopLeft, blur, col)
	}
	paintBoxBehind(cv, v, card, radii, borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Surface))
	if t.face == nil {
		return
	}
	sh := t.face.Shape(t.text, t.sizePx)
	top := card.Y + (card.H-t.textHeight())/2
	t.face.Draw(cv, sh, card.X+toastPadX, top+int(sh.Ascent()+0.5), pickc(0, v, style.PropColor, th.Text))
	if t.desc != "" {
		top += sh.LineHeight() + toastLineGap
		sh = t.face.Shape(t.desc, t.sizePx)
		t.face.Draw(cv, sh, card.X+toastPadX, top+int(sh.Ascent()+0.5), th.TextMuted)
	}
	if t.action == "" {
		return
	}
	rect := t.actionRect
	rect.Y += dy
	fill := th.Accent
	if t.hovered && rect.Contains(t.hoverPt.X, t.hoverPt.Y) {
		fill = scaleAlpha(fill, 0.9)
	}
	cv.RoundedRect(rect, toastBtnRadius, fill)
	sh = t.face.Shape(t.action, t.sizePx)
	baseline := rect.Y + (rect.H-sh.LineHeight())/2 + int(sh.Ascent()+0.5)
	t.face.Draw(cv, sh, rect.X+toastBtnPadX, baseline, th.OnAccent)
}

// HitTest returns the toast when p is inside its bounds; the toast
// handles its own action clicks.
func (t *Toast) HitTest(p Point) Widget { return t.HitLeaf(t, p) }

// ClickAt fires the action when the release landed on it, dismissing
// the toast; clicks elsewhere are inert.
func (t *Toast) ClickAt(p Point) {
	if t.action == "" || !t.actionRect.Contains(p.X, p.Y) {
		return
	}
	if t.OnAction != nil {
		t.OnAction()
	}
	t.Dismiss()
}
