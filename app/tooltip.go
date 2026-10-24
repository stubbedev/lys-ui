package app

import (
	"time"

	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/internal/popup"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// TooltipOptions tune hover tooltips; zero fields keep the defaults
// (a 500ms dwell, the card 12px right and 22px below the pointer, at
// most 400x200).
type TooltipOptions struct {
	// Delay is how long the pointer rests on a widget before its
	// tooltip opens.
	Delay time.Duration
	// OffsetX and OffsetY place the card from the pointer.
	OffsetX, OffsetY int
	// MaxWidth and MaxHeight cap the card; longer text wraps or clips.
	MaxWidth, MaxHeight int
}

// Tooltip defaults.
const (
	tooltipDelay   = 500 * time.Millisecond
	tooltipOffsetX = 12
	tooltipOffsetY = 22
	tooltipMaxW    = 400
	tooltipMaxH    = 200
)

// orDefault is v, or def when v is zero.
func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

func (o TooltipOptions) delay() time.Duration { return orDefault(o.Delay, tooltipDelay) }

func (o TooltipOptions) offset() (int, int) {
	return orDefault(o.OffsetX, tooltipOffsetX), orDefault(o.OffsetY, tooltipOffsetY)
}

func (o TooltipOptions) maxSize() (int, int) {
	return orDefault(o.MaxWidth, tooltipMaxW), orDefault(o.MaxHeight, tooltipMaxH)
}

// SetTooltipOptions tunes every window's tooltips.
func (a *Application) SetTooltipOptions(o TooltipOptions) { a.tooltipOpts = o }

// tooltipShouldOpen reports whether a resting hover earns a tooltip:
// none open yet, a hovered widget, hover text, and dwell past the
// delay.
func tooltipShouldOpen(open bool, hover widget.Widget, text string, since, now time.Time, delay time.Duration) bool {
	return !open && hover != nil && text != "" && now.Sub(since) >= delay
}

// tooltipShouldClose reports whether an open tooltip must go: the
// hover changed or the new hover carries no text.
func tooltipShouldClose(open, hoverChanged, hasText bool) bool {
	return open && (hoverChanged || !hasText)
}

// tooltipSurfacer supplies the xdg_surface an internal host's popups
// anchor to; layer hosts anchor through LayerSurfacer instead.
type tooltipSurfacer interface {
	TooltipSurface() *xdg.Surface
}

// tooltipWindow is the slice of a popup the tooltip state machine
// needs; real popups and tests both satisfy it. Dismissed — not a
// closed/destroyed flag — is what drops the reference: a tooltip
// running its exit tween is logically gone, so the next dwell may open
// a new one while the old surface is still fading out.
type tooltipWindow interface {
	Dismissed() bool
	Dismiss()
}

// tooltipPainter is the frame pipeline slice the tooltip state
// machine drives: the real popup.Painter and test fakes both satisfy
// it.
type tooltipPainter interface {
	Owes() bool
	Pass() (bool, error)
}

// tooltipCtl tracks the hover dwell and the one open tooltip. The
// open tooltip's painter is driven by the application loop (one Pass
// per loop pass): tooltips must never run their own dispatcher — one
// event-loop goroutine owns the connection.
type tooltipCtl struct {
	open    tooltipWindow
	painter tooltipPainter
	hover   widget.Widget
	since   time.Time
	// text is the hovered widget's last tooltip: a widget whose tooltip
	// follows the pointer (a calendar's per-day detail) changes it
	// without a hover change, and the change restarts the dwell.
	text string
	// delay is the dwell (TooltipOptions); zero is the default.
	delay   time.Duration
	pressed bool
}

func (t *tooltipCtl) press() {
	t.pressed = true
	if t.open != nil {
		t.open.Dismiss()
		t.open, t.painter = nil, nil
	}
}

// dwell is the configured delay, the default when unset.
func (t *tooltipCtl) dwell() time.Duration { return orDefault(t.delay, tooltipDelay) }

// next returns when a pending tooltip could open: a hovered widget
// with text, not yet open, one delay away. False when nothing is
// pending, so the loop need not wake for tooltips.
func (t *tooltipCtl) next() (time.Time, bool) {
	if t.open != nil || t.hover == nil || t.pressed {
		return time.Time{}, false
	}
	if text := hoverTooltipText(t.hover); text != "" {
		return t.since.Add(t.dwell()), true
	}
	return time.Time{}, false
}

// hoverTooltipText returns the hovered widget's tooltip text, empty
// when it has none.
func hoverTooltipText(h widget.Widget) string {
	if tter, ok := h.(widget.TooltipTexter); ok {
		return tter.TooltipText()
	}
	return ""
}

// update advances the tooltip state to now: reset the dwell on hover
// change, dismiss stale tooltips, open a due one through opener, and
// paint the open tooltip's tween frames.
func (t *tooltipCtl) update(router *widget.Router, now time.Time, opener func(widget.Widget, string) (tooltipWindow, tooltipPainter)) {
	if t.open != nil && t.open.Dismissed() {
		t.open, t.painter = nil, nil
	}
	h := router.Hovered()
	text := hoverTooltipText(h)
	changed := h != t.hover || text != t.text
	if changed {
		t.hover, t.text, t.since, t.pressed = h, text, now, false
	}
	if tooltipShouldClose(t.open != nil, changed, text != "") {
		// Traced as "tooltip closed": the headless harness waits on it.
		debug.Log("input", "tooltip closed")
		t.open.Dismiss()
		t.open, t.painter = nil, nil
	}
	if !t.pressed && tooltipShouldOpen(t.open != nil, h, text, t.since, now, t.dwell()) {
		p, pc := opener(h, text)
		switch {
		case p == nil:
		case t.pressed:
			p.Dismiss()
		default:
			t.open, t.painter = p, pc
		}
		// Advance the dwell clock even when the open failed, so a
		// tooltip that cannot open (host without a face, compositor
		// rejection) retries after a full delay instead of every loop.
		t.since = now
	}
	if t.painter != nil && t.painter.Owes() {
		if _, err := t.painter.Pass(); err != nil {
			debug.Log("frame", "tooltip paint: %v", err)
			t.painter = nil
		}
	}
}

// cursorFor resolves the pointer shape for the hovered widget: the
// stylesheet's cursor declaration first (it inherits, so a container
// covers its subtree and overrides widget defaults), else the
// widget's own CursorName (text fields ask for the caret), else the
// node-level SetCursorName, else the arrow.
func cursorFor(hover widget.Widget) string {
	if css := widget.CSSCursorOf(hover); css != "" {
		return css
	}
	if cn, ok := hover.(widget.CursorNamer); ok {
		return cn.CursorName()
	}
	return widget.CursorNameOf(hover)
}

// openTooltip maps a tooltip popup at the pointer, parented like a
// popover (a window's xdg surface, a panel's layer surface). The popup
// surface scales with the host window: frac120 is the window's current
// 120-based device scale. Frames are painted by the application loop
// through the returned Painter — the enter fade, the rest state, and
// the exit fade all repaint through the same tween machinery menus
// use, without a second wayland dispatcher. Tooltips are ungrabbed
// (NoGrab), so the fades never hold input hostage. A hovered widget
// with markup (SetTooltipMarkup) renders through a RichLabel.
func openTooltip(sess *wlsession.Session, host Host, face *render.Typeface, opts TooltipOptions, frac120 uint32, pointerX, pointerY int, hover widget.Widget, text string) (*popup.Popup, *popup.Painter) {
	xdgParent, layerParent, ok := popupParentOf(host)
	if !ok || face == nil {
		debug.Log("input", "tooltip unavailable: host %T or nil face", host)
		return nil, nil
	}
	markup := false
	if m, ok := hover.(widget.TooltipMarkupper); ok {
		if source, rich := m.TooltipMarkup(); rich {
			text, markup = source, true
		}
	}
	box := widget.NewTooltipCard(face, text, markup)
	maxW, maxH := opts.maxSize()
	size := box.Measure(widget.Constraints{Max: widget.Size{W: maxW, H: maxH}})
	// The shadow gutter rides on the surface (see OpenPopover): the
	// tooltip's plate is inset by it and the pointer offset stays on
	// the plate.
	gutter := widget.Current().ShadowGutter()
	dx, dy := opts.offset()
	tp, err := popup.New(sess, popup.Config{
		Parent:      xdgParent,
		LayerParent: layerParent,
		X:           pointerX + dx - gutter,
		Y:           pointerY + dy - gutter,
		Width:       size.W + 2*gutter, Height: size.H + 2*gutter,
		Gutter:  gutter,
		NoGrab:  true,
		NoPlate: true,
		Kind:    surfx.KindTooltip,
	})
	if err != nil {
		debug.Log("input", "tooltip popup: %v", err)
		return nil, nil
	}
	// Tooltips are pure display: an empty input region keeps the
	// compositor from ever routing the pointer to the popup, which
	// would steal clicks from the window beneath. Sealed once at open;
	// the state machine re-seals on dismissal, and later commits keep
	// the region (double-buffered state persists until changed).
	tp.SealInput()
	painter := tp.NewPainter(sess, frac120, box, widget.Current().Surface)
	debug.Log("input", "tooltip open %q at (%d,%d)", text, pointerX, pointerY)
	return tp, painter
}
