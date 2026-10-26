// Package inspect is lys-ui's debug inspector (the GTK-inspector
// counterpart): a widget-tree overlay drawn over the live UI, the
// tree dump for bug reports, and the doctor diagnostics block.
//
// Prod visibility: everything here is always compiled in but inert
// until asked for - LYS_UI_INSPECT=1 at startup, an explicit
// Application.SetInspect/ToggleInspect call, or (for the dump and
// doctor) an explicit call. With the inspector off, an app pays one
// passthrough call per frame operation and one bool check per key
// press; nothing traces, draws, or allocates. The lysdebug build tag
// was deliberately NOT used: the inspector exists to debug prod
// binaries in the field, where tagged builds do not exist. Real
// trace instrumentation stays behind the tag (internal/debug).
package inspect

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/widget"
)

// Enabled reports whether the environment asked for the inspector at
// startup: LYS_UI_INSPECT set to 1, true, yes, or on (case-insensitive).
// Anything else - unset, empty, 0, off - leaves it off.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LYS_UI_INSPECT"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Overlay wraps a widget tree and paints inspector annotations over
// it: bounds with a per-depth alternating tint, the debug name or
// type as a label, the hovered subtree, the router's hover target,
// and the focus target, each marked distinctly.
//
// The overlay is transparent plumbing while it is off: Measure,
// Arrange, HitTest, and the tree walks all pass straight through to
// the wrapped root, so wrapping never changes layout, hit testing, or
// damage. Painting annotations only reads state (widget.InspectTree);
// it never invalidates or mutates a widget. It is a node in the parent
// chain, though: the root's invalidations climb through it to the
// window's caches, which a parentless root would cut off.
type Overlay struct {
	widget.Base
	root   widget.Widget
	router *widget.Router
	on     bool
}

// NewOverlay wraps root. The overlay starts off; SetOn turns the
// annotations on. It never wraps nil: a nil root passes through as a
// harmless empty widget.
func NewOverlay(root widget.Widget) *Overlay {
	return &Overlay{root: root}
}

// SetRouter attaches the router whose focus, hover, and press targets
// the overlay marks. The app installs its window's router right after
// creating it.
func (o *Overlay) SetRouter(r *widget.Router) { o.router = r }

// SetOn turns the annotation painting on or off.
func (o *Overlay) SetOn(on bool) { o.on = on }

// On reports whether annotations are painted.
func (o *Overlay) On() bool { return o.on }

// Root returns the wrapped widget tree.
func (o *Overlay) Root() widget.Widget { return o.root }

// Children exposes the wrapped root, so tree walks (damage
// collection, focus traversal) see the real tree.
func (o *Overlay) Children() []widget.Widget {
	if o.root == nil {
		return nil
	}
	return []widget.Widget{o.root}
}

// Measure implements widget.Widget, passing through to the root.
func (o *Overlay) Measure(con widget.Constraints) widget.Size {
	if o.root == nil {
		return widget.Size{}
	}
	return o.root.Measure(con)
}

// Arrange implements widget.Widget, passing through to the root and
// parenting it.
func (o *Overlay) Arrange(r render.Rect) {
	o.ArrangeSelf(r)
	if o.root == nil {
		return
	}
	o.root.Arrange(r)
	widget.SetParents(o, o.root)
}

// HitTest implements widget.Widget, passing through to the root so
// input routing lands on real widgets, never on the overlay.
func (o *Overlay) HitTest(p widget.Point) widget.Widget {
	if o.root == nil {
		return nil
	}
	return o.root.HitTest(p)
}

// Paint implements widget.Widget: the root paints first, then the
// annotations when the overlay is on. The annotation pass is a pure
// function of widget state - no invalidation, no state changes.
func (o *Overlay) Paint(cv *render.Canvas) {
	if o.root == nil {
		return
	}
	o.root.Paint(cv)
	if o.on {
		PaintAnnotations(cv, widget.InspectTree(o.root, o.router))
	}
}

// labelFont is the face overlay labels draw with, resolved lazily from
// the system monospace. SetLabelFont overrides it (tests, or apps that
// want the inspector in their own face).
var (
	labelOnce   sync.Once
	labelFace   *render.Typeface
	labelFontMu sync.Mutex
)

// SetLabelFont overrides the face overlay labels use.
func SetLabelFont(f *render.Typeface) {
	labelFontMu.Lock()
	defer labelFontMu.Unlock()
	labelFace = f
	labelOnce.Do(func() {})
}

// labels returns the label face, or nil when none could be resolved
// (annotations then paint without text).
func labels() *render.Typeface {
	labelFontMu.Lock()
	defer labelFontMu.Unlock()
	labelOnce.Do(func() {
		f, err := sysfont.Monospace()
		if err != nil {
			return
		}
		labelFace = f
	})
	return labelFace
}

// labelSize is the label pixel size, and chipPad the padding around a
// label chip.
const (
	labelSize = 11.0
	chipPad   = 2
)

// Overlay palette. Two alternating tint/border pairs distinguish
// nesting depths; the hover target and focus target get their own
// saturated colors so router-level state reads apart from the
// widget-level tints at a glance.
var (
	tintEven   = render.RGBA(0x6c, 0xc6, 0xff, 0x18) // blue wash
	borderEven = render.RGBA(0x6c, 0xc6, 0xff, 0x90)
	tintOdd    = render.RGBA(0xff, 0xc6, 0x6c, 0x18) // orange wash
	borderOdd  = render.RGBA(0xff, 0xc6, 0x6c, 0x90)
	hoverColor = render.RGBA(0xff, 0x6c, 0xc6, 0xe0) // magenta: router hover target
	focusColor = render.RGBA(0x6c, 0xff, 0x8a, 0xe0) // green: focus target
	chipBg     = render.RGBA(0x00, 0x00, 0x00, 0xb4)
	chipFg     = render.RGB(0xff, 0xff, 0xff)
)

// PaintAnnotations draws the inspector overlay for a snapshot taken
// with widget.InspectTree: per-widget bounds with an alternating
// tint per depth, a name/type label chip, the hovered subtree washed
// stronger, and the router's hover and focus targets outlined in
// their own colors. It only reads the snapshot and draws; it never
// touches widget state, so tests can pin it as side-effect free.
func PaintAnnotations(cv *render.Canvas, nodes []widget.NodeInfo) {
	f := labels()
	// hoverPath collects the hovered subtree: the router's hover
	// target plus its ancestors, recovered from the paint-order
	// snapshot with a depth stack.
	hoverPath := map[int]bool{}
	hoverMark, focusMark := -1, -1
	var stack []int
	for i, ni := range nodes {
		for len(stack) > ni.Depth {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, i)
		switch {
		case ni.Hovered:
			hoverMark = i
			for _, j := range stack {
				hoverPath[j] = true
			}
		case ni.Focused:
			focusMark = i
		}
	}
	for i, ni := range nodes {
		b := ni.Bounds
		if b.Empty() {
			continue
		}
		tint, border := tintEven, borderEven
		if ni.Depth%2 == 1 {
			tint, border = tintOdd, borderOdd
		}
		if hoverPath[i] {
			tint = render.RGBA(tint.R(), tint.G(), tint.B(), 0x38)
		}
		cv.FillRect(b, tint)
		cv.BorderRect(b, 1, border)
		switch i {
		case hoverMark:
			cv.BorderRect(b, 2, hoverColor)
		case focusMark:
			cv.BorderRect(b, 2, focusColor)
		}
		if f != nil {
			drawLabel(cv, f, b, ni)
		}
	}
}

// drawLabel paints one chip with the widget's debug name (or type)
// plus its router-state markers, clipped so it never escapes the
// widget's own bounds.
func drawLabel(cv *render.Canvas, f *render.Typeface, b render.Rect, ni widget.NodeInfo) {
	text := ni.DebugName
	if text == "" {
		text = shortType(ni.TypeName)
	}
	var marks []string
	if ni.Focused {
		marks = append(marks, "F")
	}
	if ni.Hovered {
		marks = append(marks, "H")
	}
	if ni.Pressed {
		marks = append(marks, "P")
	}
	if len(marks) > 0 {
		text += " [" + strings.Join(marks, "") + "]"
	}
	shaped := f.Shape(text, labelSize)
	w := int(shaped.Advance()) + 2*chipPad
	h := shaped.LineHeight() + 2*chipPad
	if w >= b.W || h >= b.H {
		return // the chip would cover the whole widget; draw none
	}
	prev := cv.PushClip(b)
	chip := render.Rect{X: b.X + 1, Y: b.Y + 1, W: w, H: h}
	cv.FillRect(chip, chipBg)
	baseline := chip.Y + chipPad + int(shaped.Ascent())
	f.Draw(cv, shaped, chip.X+chipPad, baseline, chipFg)
	cv.PopClip(prev)
}

// shortType renders a %T type name for overlay labels: the widget
// package qualifier is noise on screen, so "*widget.Button" becomes
// "Button" and other packages keep "pkg.Type".
func shortType(t string) string {
	t = strings.TrimPrefix(t, "*widget.")
	return strings.TrimPrefix(t, "*")
}

// Dump renders the tree dump for bug reports: one banner line, then
// the stable widget.DumpTree format (one indented line per widget
// with type, debug name, bounds, role, tooltip, and input-state
// flags). Print it to stdout on demand; the exact bytes are pinned by
// tests, so a dump in an issue means the same thing to everyone.
func Dump(root widget.Widget, r *widget.Router) string {
	nodes := widget.InspectTree(root, r)
	var b strings.Builder
	fmt.Fprintf(&b, "lys-ui tree dump: %d widgets\n", len(nodes))
	b.WriteString(widget.DumpTree(root, r))
	return b.String()
}
