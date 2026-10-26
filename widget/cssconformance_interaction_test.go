package widget

import (
	"fmt"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/style"
)

// uiAdvance moves the pinned animation clock d past its current
// instant and ticks every running tween there.
func uiAdvance(f *confFixture, d time.Duration) {
	f.clock.set(f.clock.now().Add(d))
	animclock.Tick(f.clock.now())
}

// uiRich is a selectable RichLabel holding s.
func uiRich(sel, s string) confNode {
	return confNode{sel: sel, make: func(f *confFixture, _ []Widget) Widget {
		l := NewRichLabel(f.face, confFaceSize, s, render.RGB(0, 0, 0))
		l.SetSelectable(true)
		return l
	}}
}

// uiCheck is a checked CheckButton.
func uiCheck(sel string) confNode {
	return confNode{sel: sel, make: func(*confFixture, []Widget) Widget { return NewCheckButton(true) }}
}

// uiScroll is the snapping fixture: a 100x100 Scroll (#s) over a
// column (or row, horizontal) of n 100x60 (60x100) leaves #c0.. with
// the classes cls.
func uiScroll(horizontal bool, n int, cls string) confNode {
	kids := make([]confNode, n)
	for i := range kids {
		w, h := 100, 60
		if horizontal {
			w, h = 60, 100
		}
		kids[i] = nLeaf(fmt.Sprintf("#c%d.%s", i, cls), w, h)
	}
	if horizontal {
		return nScroll("#s", nBox("#k", kids...))
	}
	return nScroll("#s", nCol("#k", kids...))
}

// uiOverlayCSS stacks #over (and its #kid) on #under inside #st: two
// absolutely positioned 100px squares, the later painted and hit on
// top.
const uiOverlayCSS = `#st { position: relative; width: 100px; height: 100px; }
	#under, #over { position: absolute; left: 0; top: 0; width: 100px; height: 100px; }
	#kid { width: 100px; height: 100px; } `

// uiScrollCSS sizes uiScroll's viewport.
const uiScrollCSS = `#s { width: 100px; height: 100px; } `

// uiOffset expects #s scrolled to x, y.
func uiOffset(x, y int) confCheck {
	return wantProbe("scroll offset of #s", func(f *confFixture) any {
		ox, oy := f.get("s").(*Scroll).Offset()
		return fmt.Sprint(ox, ",", oy)
	}, fmt.Sprint(x, ",", y))
}

// uiWheel scrolls #s by wheel notches; a notch over the 100px
// viewport is 100^(2/3) = 21px (GTK's step).
func uiWheel(dx, dy int) func(f *confFixture) {
	return func(f *confFixture) { f.get("s").(*Scroll).ScrollBy(dx, dy) }
}

// uiEntrySelection reads #id's selection as [start,end) or "none".
func uiEntrySelection(id string) func(f *confFixture) any {
	return func(f *confFixture) any {
		start, end, on := f.get(id).(*Entry).Selection()
		if !on || start == end {
			return "none"
		}
		return fmt.Sprintf("[%d,%d)", start, end)
	}
}

// uiDrag presses #id near its start and drags across it.
func uiDrag(id string) func(f *confFixture) {
	return func(f *confFixture) {
		b := boundsOf(f.get(id))
		y := b.Y + b.H/2
		switch w := f.get(id).(type) {
		case *Entry:
			w.ClickAt(Point{X: b.X + 2, Y: y})
			w.DragMove(Point{X: b.X + b.W - 2, Y: y})
		case *RichLabel:
			w.PressAt(Point{X: b.X + 1, Y: y})
			w.DragMove(Point{X: b.X + b.W - 1, Y: y})
		}
	}
}

// uiTiming samples a 1000ms opacity 0→1 animation under timing at
// progress p, through a negative delay: the computed opacity is the
// eased progress.
func uiTiming(name, wpt, timing string, p, want float64) confCase {
	return confCase{
		name: name, wpt: wpt, spec: "css-easing-2 §2–3",
		css: fmt.Sprintf(`@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			#b { width: 10px; height: 10px; animation: fade 1000ms %s -%dms; }`, timing, int(p*1000)),
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, want)},
	}
}

// The Interaction group's computed values.
var (
	cpUICursor     = confProp{"cursor", func(v *style.Values) any { return cssCursorRequest(v.Cursor) }}
	cpUICaretShape = confProp{"caret-shape", func(v *style.Values) any { return v.CaretShape }}
	cpUICaretColor = confProp{"caret-color", func(v *style.Values) any { return v.CaretColor }}
	cpUIAccent     = confProp{"accent-color", func(v *style.Values) any { return accentOf(v, 0) }}
	cpUIAppearance = confProp{"appearance", func(v *style.Values) any { return v.Appearance }}
	cpUIBarThumb   = confProp{"scrollbar-color thumb", func(v *style.Values) any { return v.BarThumb }}
	cpUIBarWidth   = confProp{"scrollbar-width", func(v *style.Values) any { return v.BarWidth }}
	cpUIIsolate    = confProp{"isolation isolate", func(v *style.Values) any { return v.Isolate }}
	cpUIWidth      = confProp{"used width", func(v *style.Values) any { return v.Layout.Width.Len.Px }}
	cpUITextAlign  = confProp{"text-align", func(v *style.Values) any { return v.TextAlign }}
)

// confInteraction is the #159 Interaction and UI group: cursor,
// pointer-events, user-select, touch-action, the caret, accent-color,
// appearance, the scrolling properties, and transitions and
// animations.
var confInteraction = []confCase{
	{
		name: "cursor keywords compute",
		spec: "css-ui-4 §6.1",
		css: `#a { cursor: pointer; } #b { cursor: ew-resize; } #c { cursor: not-allowed; }
			#d { cursor: grabbing; } #e { cursor: none; } #f { cursor: auto; }`,
		tree: nCol("#p", nBox("#a"), nBox("#b"), nBox("#c"), nBox("#d"), nBox("#e"), nBox("#f")),
		want: []confCheck{
			wantComputed("a", cpUICursor, "pointer"), wantComputed("b", cpUICursor, "ew-resize"),
			wantComputed("c", cpUICursor, "not-allowed"), wantComputed("d", cpUICursor, "grabbing"),
			wantComputed("e", cpUICursor, "none"), wantComputed("f", cpUICursor, ""),
		},
	},
	{
		name: "cursor inherits",
		spec: "css-ui-4 §6.1 (cursor: inherited)",
		css:  `#p { cursor: crosshair; }`,
		tree: nCol("#p", nBox("#k")),
		want: []confCheck{wantComputed("k", cpUICursor, "crosshair")},
	},
	{
		name: "cursor url with a hotspot and its fallback",
		spec: "css-ui-4 §6.1",
		css:  `#a { cursor: url("hand.cur") 3 4, pointer; } #b { cursor: url("arrow.cur"), default; }`,
		tree: nCol("#p", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantComputed("a", cpUICursor, "file:hand.cur#3,4,pointer"),
			wantComputed("b", cpUICursor, "file:arrow.cur#0,0,default"),
		},
	},
	{
		name: "pointer-events none lets the hit fall through",
		spec: "css-ui-4 §8.1 (pointer-events)",
		css:  uiOverlayCSS + `#over { pointer-events: none; }`,
		tree: nBox("#st", nBox("#under"), nBox("#over")),
		want: []confCheck{wantHit(50, 50, "under")},
	},
	{
		name: "pointer-events auto on top takes the hit",
		spec: "css-ui-4 §8.1 (pointer-events)",
		css:  uiOverlayCSS,
		tree: nBox("#st", nBox("#under"), nBox("#over")),
		want: []confCheck{wantHit(50, 50, "over")},
	},
	{
		name: "pointer-events inherits into the subtree",
		spec: "css-ui-4 §8.1 (pointer-events: inherited)",
		css:  uiOverlayCSS + `#over { pointer-events: none; }`,
		tree: nBox("#st", nBox("#under"), nBox("#over", nBox("#kid"))),
		want: []confCheck{wantHit(50, 50, "under")},
	},
	{
		name: "pointer-events auto re-enables a descendant",
		spec: "css-ui-4 §8.1 (pointer-events: inherited)",
		css:  uiOverlayCSS + `#over { pointer-events: none; } #kid { pointer-events: auto; }`,
		tree: nBox("#st", nBox("#under"), nBox("#over", nBox("#kid"))),
		want: []confCheck{wantHit(50, 50, "kid")},
	},
	{
		name: "user-select none keeps a drag from selecting",
		spec: "css-ui-4 §7.1 (user-select)",
		css:  `#e { width: 200px; user-select: none; }`,
		tree: nEntry("#e", "select me"),
		act:  uiDrag("e"),
		want: []confCheck{wantProbe("selection of #e", uiEntrySelection("e"), "none")},
	},
	{
		name: "user-select text selects the dragged range",
		spec: "css-ui-4 §7.1 (user-select)",
		css:  `#e { width: 200px; user-select: text; }`,
		tree: nEntry("#e", "select me"),
		act:  uiDrag("e"),
		want: []confCheck{wantProbe("selection of #e", uiEntrySelection("e"), "[0,9)")},
	},
	{
		name: "user-select all selects everything on a press",
		spec: "css-ui-4 §7.1 (user-select)",
		css:  `#e { width: 200px; user-select: all; }`,
		tree: nEntry("#e", "select me"),
		act: func(f *confFixture) {
			b := boundsOf(f.get("e"))
			f.get("e").(*Entry).ClickAt(Point{X: b.X + 40, Y: b.Y + b.H/2})
		},
		want: []confCheck{wantProbe("selection of #e", uiEntrySelection("e"), "[0,9)")},
	},
	{
		name: "user-select auto takes none from the parent",
		spec: "css-ui-4 §7.1 (auto's used value)",
		css:  `#p { user-select: none; } #l { width: 200px; }`,
		tree: nCol("#p", uiRich("#l", "rich text here")),
		act:  uiDrag("l"),
		want: []confCheck{wantProbe("selection of #l", func(f *confFixture) any {
			_, on := f.get("l").(*RichLabel).SelectedText()
			return on
		}, false)},
	},
	{
		name: "user-select auto on an editable element is contain",
		spec: "css-ui-4 §7.1 (auto's used value)",
		css:  `#p { user-select: none; } #e { width: 200px; }`,
		tree: nCol("#p", nEntry("#e", "select me")),
		act:  uiDrag("e"),
		want: []confCheck{wantProbe("selection of #e", uiEntrySelection("e"), "[0,9)")},
	},
	{
		name: "user-select does not inherit",
		spec: "css-ui-4 §7.1 (user-select: not inherited)",
		css:  `#p { user-select: all; }`,
		tree: nCol("#p", nBox("#k")),
		want: []confCheck{wantProbe("declared user-select of #k", func(f *confFixture) any {
			return f.values("k").Has(style.PropUserSelect)
		}, false)},
	},
	{
		name: "touch-action pan-x gates a vertical touch scroll",
		spec: "pointerevents-3 §11 (touch-action)",
		css:  uiScrollCSS + `.c { touch-action: pan-x; }`,
		tree: uiScroll(false, 5, "c"),
		act: func(f *confFixture) {
			r := &Router{Root: f.root}
			r.Move(Point{X: 50, Y: 50})
			r.AxisPixels(0, 40, 0)
		},
		want: []confCheck{uiOffset(0, 0)},
	},
	{
		name: "touch-action pan-y lets a vertical touch scroll",
		spec: "pointerevents-3 §11 (touch-action)",
		css:  uiScrollCSS + `.c { touch-action: pan-y; }`,
		tree: uiScroll(false, 5, "c"),
		act: func(f *confFixture) {
			r := &Router{Root: f.root}
			r.Move(Point{X: 50, Y: 50})
			r.AxisPixels(0, 40, 0)
		},
		want: []confCheck{uiOffset(0, 40)},
	},
	{
		name: "touch-action intersects with the ancestors",
		spec: "pointerevents-3 §11 (touch-action)",
		css:  uiScrollCSS + `#k { touch-action: pan-x; } .c { touch-action: pan-y; }`,
		tree: uiScroll(false, 5, "c"),
		act: func(f *confFixture) {
			r := &Router{Root: f.root}
			r.Move(Point{X: 50, Y: 50})
			r.AxisPixels(0, 40, 0)
		},
		want: []confCheck{uiOffset(0, 0)},
	},
	{
		name: "touch-action never gates the wheel",
		spec: "pointerevents-3 §11 (touch-action)",
		css:  uiScrollCSS + `.c { touch-action: none; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 2),
		want: []confCheck{uiOffset(0, 42)},
	},
	{
		name: "caret-shape computes and inherits",
		spec: "css-ui-4 §5.3 (caret-shape: inherited)",
		css:  `#p { caret-shape: block; } #q { caret-shape: underscore; }`,
		tree: nCol("#c", nCol("#p", nEntry("#e", "x")), nEntry("#q", "x")),
		want: []confCheck{
			wantComputed("e", cpUICaretShape, style.CaretBlock),
			wantComputed("q", cpUICaretShape, style.CaretUnderscore),
		},
	},
	{
		name: "caret-color inherits",
		spec: "css-ui-4 §5.2 (caret-color: inherited)",
		css:  `#p { caret-color: #00ff00; }`,
		tree: nCol("#p", nEntry("#e", "x")),
		want: []confCheck{wantComputed("e", cpUICaretColor, render.Color(0xff00ff00))},
	},
	{
		name: "accent-color inherits",
		spec: "css-ui-4 §4.1 (accent-color: inherited)",
		css:  `#p { accent-color: #ff0000; } #q { accent-color: auto; }`,
		tree: nCol("#c", nCol("#p", uiCheck("#a")), uiCheck("#q")),
		want: []confCheck{
			wantComputed("a", cpUIAccent, render.Color(0xffff0000)),
			wantComputed("q", cpUIAccent, render.Color(0)),
		},
	},
	{
		name: "appearance computes and does not inherit",
		spec: "css-ui-4 §9.1",
		css:  `#p { appearance: none; }`,
		tree: nCol("#p", uiCheck("#a")),
		want: []confCheck{
			wantComputed("p", cpUIAppearance, style.AppearanceNone),
			wantComputed("a", cpUIAppearance, style.AppearanceAuto),
		},
	},
	{
		name: "scroll-behavior auto lands a reveal at once",
		spec: "cssom-view-1 §7 (scroll-behavior)",
		css:  uiScrollCSS,
		tree: uiScroll(false, 5, "c"),
		act:  func(f *confFixture) { RevealRect(f.get("c2"), render.Rect{X: 0, Y: 150, W: 100, H: 10}) },
		want: []confCheck{uiOffset(0, 60)},
	},
	{
		name: "scroll-behavior smooth animates a reveal",
		spec: "cssom-view-1 §7 (scroll-behavior)",
		css:  uiScrollCSS + `#s { scroll-behavior: smooth; }`,
		tree: uiScroll(false, 5, "c"),
		act: func(f *confFixture) {
			RevealRect(f.get("c2"), render.Rect{X: 0, Y: 150, W: 100, H: 10})
			if _, y := f.get("s").(*Scroll).Offset(); y != 0 {
				f.t.Errorf("smooth reveal jumped to %d at once", y)
			}
			f.clock.drive()
		},
		want: []confCheck{uiOffset(0, 60)},
	},
	{
		name: "scroll-margin grows the revealed rect",
		spec: "css-scroll-snap-1 §5.1 (scroll-margin)",
		css:  uiScrollCSS + `.c { scroll-margin: 4px 0 4px 0; }`,
		tree: uiScroll(false, 5, "c"),
		act:  func(f *confFixture) { RevealRect(f.get("c2"), render.Rect{X: 0, Y: 150, W: 100, H: 10}) },
		want: []confCheck{uiOffset(0, 64)},
	},
	{
		name: "scroll-padding insets the reveal viewport",
		spec: "css-scroll-snap-1 §4.1 (scroll-padding)",
		css:  uiScrollCSS + `#s { scroll-padding-bottom: 6px; }`,
		tree: uiScroll(false, 5, "c"),
		act:  func(f *confFixture) { RevealRect(f.get("c2"), render.Rect{X: 0, Y: 150, W: 100, H: 10}) },
		want: []confCheck{uiOffset(0, 66)},
	},
	{
		name: "scroll-snap mandatory start",
		spec: "css-scroll-snap-1 §6.1, §6.2",
		css:  uiScrollCSS + `#s { scroll-snap-type: y mandatory; } .c { scroll-snap-align: start; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 2),
		want: []confCheck{uiOffset(0, 60)},
	},
	{
		name: "scroll-snap mandatory center",
		spec: "css-scroll-snap-1 §6.2",
		css:  uiScrollCSS + `#s { scroll-snap-type: y mandatory; } .c { scroll-snap-align: center; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 2),
		want: []confCheck{uiOffset(0, 40)},
	},
	{
		name: "scroll-snap mandatory end",
		spec: "css-scroll-snap-1 §6.2",
		css:  uiScrollCSS + `#s { scroll-snap-type: y mandatory; } .c { scroll-snap-align: end; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 2),
		want: []confCheck{uiOffset(0, 20)},
	},
	{
		name: "scroll-snap proximity leaves a far resting offset",
		spec: "css-scroll-snap-1 §6.1 (proximity: UA threshold of half a viewport)",
		css:  `#s { width: 100px; height: 100px; scroll-snap-type: y proximity; } .c { scroll-snap-align: start; height: 200px; }`,
		tree: uiScroll(false, 3, "c"),
		act:  uiWheel(0, 4),
		want: []confCheck{uiOffset(0, 84)},
	},
	{
		name: "scroll-snap proximity snaps a near resting offset",
		spec: "css-scroll-snap-1 §6.1 (proximity)",
		css:  uiScrollCSS + `#s { scroll-snap-type: y proximity; } .c { scroll-snap-align: start; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 3),
		want: []confCheck{uiOffset(0, 60)},
	},
	{
		name: "scroll-snap positions inset by scroll-padding",
		spec: "css-scroll-snap-1 §4.1 (the snapport)",
		css: uiScrollCSS + `#s { scroll-snap-type: y mandatory; scroll-padding-top: 10px; }
			.c { scroll-snap-align: start; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 2),
		want: []confCheck{uiOffset(0, 50)},
	},
	{
		name: "scroll-snap areas grow by scroll-margin",
		spec: "css-scroll-snap-1 §5.1 (the snap area)",
		css:  uiScrollCSS + `#s { scroll-snap-type: y mandatory; } .c { scroll-snap-align: start; scroll-margin-top: 10px; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 2),
		want: []confCheck{uiOffset(0, 50)},
	},
	{
		name: "scroll-snap-stop always stops a long scroll",
		spec: "css-scroll-snap-1 §6.3",
		css: uiScrollCSS + `#s { scroll-snap-type: y mandatory; } .c { scroll-snap-align: start; }
			#c1 { scroll-snap-stop: always; }`,
		tree: uiScroll(false, 5, "c"),
		act:  uiWheel(0, 5),
		want: []confCheck{uiOffset(0, 60)},
	},
	{
		name: "scroll-snap on the x axis",
		spec: "css-scroll-snap-1 §6.1",
		css:  uiScrollCSS + `#s { scroll-snap-type: x mandatory; } .c { scroll-snap-align: start; }`,
		tree: uiScroll(true, 5, "c"),
		act:  uiWheel(2, 0),
		want: []confCheck{uiOffset(60, 0)},
	},
	{
		name: "scroll-snap-type y leaves the x axis free",
		spec: "css-scroll-snap-1 §6.1",
		css:  uiScrollCSS + `#s { scroll-snap-type: y mandatory; } .c { scroll-snap-align: start; }`,
		tree: uiScroll(true, 5, "c"),
		act:  uiWheel(2, 0),
		want: []confCheck{uiOffset(42, 0)},
	},
	{
		name: "overscroll-behavior auto chains to the parent scroll",
		spec: "css-overscroll-1 §2",
		css:  `#o { width: 100px; height: 100px; } #s { width: 100px; height: 100px; }`,
		tree: nScroll("#o", nCol("#oc", nLeaf("#pad", 100, 100), uiScroll(false, 2, "c"), nLeaf("#tail", 100, 200))),
		act: func(f *confFixture) {
			f.get("o").(*Scroll).SetOffset(0, 100)
			f.get("s").(*Scroll).ScrollPixels(0, 60)
		},
		want: []confCheck{
			uiOffset(0, 20),
			wantProbe("outer offset", func(f *confFixture) any { _, y := f.get("o").(*Scroll).Offset(); return y }, 140),
		},
	},
	{
		name: "overscroll-behavior contain stops the chain",
		spec: "css-overscroll-1 §2",
		css:  `#o { width: 100px; height: 100px; } #s { width: 100px; height: 100px; overscroll-behavior: contain; }`,
		tree: nScroll("#o", nCol("#oc", nLeaf("#pad", 100, 100), uiScroll(false, 2, "c"), nLeaf("#tail", 100, 200))),
		act: func(f *confFixture) {
			f.get("o").(*Scroll).SetOffset(0, 100)
			f.get("s").(*Scroll).ScrollPixels(0, 60)
		},
		want: []confCheck{
			uiOffset(0, 20),
			wantProbe("outer offset", func(f *confFixture) any { _, y := f.get("o").(*Scroll).Offset(); return y }, 100),
		},
	},
	{
		name: "overscroll-behavior-x leaves the y chain alone",
		spec: "css-overscroll-1 §2",
		css:  `#o { width: 100px; height: 100px; } #s { width: 100px; height: 100px; overscroll-behavior-x: none; }`,
		tree: nScroll("#o", nCol("#oc", nLeaf("#pad", 100, 100), uiScroll(false, 2, "c"), nLeaf("#tail", 100, 200))),
		act: func(f *confFixture) {
			f.get("o").(*Scroll).SetOffset(0, 100)
			f.get("s").(*Scroll).ScrollPixels(0, 60)
		},
		want: []confCheck{
			wantProbe("outer offset", func(f *confFixture) any { _, y := f.get("o").(*Scroll).Offset(); return y }, 140),
		},
	},
	{
		name: "scrollbar-gutter stable reserves the bar",
		spec: "css-overflow-4 §3.2 (scrollbar-gutter)",
		css:  `#s { scrollbar-gutter: stable; } #t { scrollbar-gutter: stable both-edges; }`,
		tree: nCol("#p", nBox("#r1", nScroll("#s", nLeaf("#a", 50, 40))), nBox("#r2", nScroll("#t", nLeaf("#b", 50, 40)))),
		want: []confCheck{
			wantProbe("width of #s", func(f *confFixture) any { return boundsOf(f.get("s")).W }, 58),
			wantProbe("width of #t", func(f *confFixture) any { return boundsOf(f.get("t")).W }, 66),
		},
	},
	{
		name: "scrollbar-color inherits, scrollbar-width does not",
		spec: "css-scrollbars-1 §2, §3",
		css:  `#p { scrollbar-color: #ff0000 #0000ff; scrollbar-width: thin; }`,
		tree: nCol("#p", nBox("#k")),
		want: []confCheck{
			wantComputed("k", cpUIBarThumb, render.Color(0xffff0000)),
			wantComputed("p", cpUIBarWidth, style.BarWidthThin),
			wantComputed("k", cpUIBarWidth, style.BarWidthAuto),
		},
	},
	uiTiming("timing linear", "css/css-easing/linear-timing-functions-output.html", "linear", 0.3, 0.3),
	uiTiming("timing ease", "css/css-easing/cubic-bezier-timing-functions-output.html", "ease", 0.5, 0.802403),
	uiTiming("timing ease-in", "css/css-easing/cubic-bezier-timing-functions-output.html", "ease-in", 0.25, 0.093465),
	uiTiming("timing ease-out", "css/css-easing/cubic-bezier-timing-functions-output.html", "ease-out", 0.75, 0.906535),
	uiTiming("timing ease-in-out", "css/css-easing/cubic-bezier-timing-functions-output.html", "ease-in-out", 0.25, 0.129162),
	uiTiming("timing cubic-bezier", "css/css-easing/cubic-bezier-timing-functions-output.html", "cubic-bezier(0.1, 0.6, 0.9, 0.4)", 0.25, 0.373014),
	uiTiming("timing steps jump-end", "css/css-easing/step-timing-functions-output.html", "steps(4, jump-end)", 0.5, 0.5),
	uiTiming("timing steps jump-start", "css/css-easing/step-timing-functions-output.html", "steps(4, jump-start)", 0.5, 0.75),
	uiTiming("timing steps jump-none", "css/css-easing/step-timing-functions-output.html", "steps(4, jump-none)", 0.5, 2.0/3),
	uiTiming("timing steps jump-both", "css/css-easing/step-timing-functions-output.html", "steps(4, jump-both)", 0.3, 0.4),
	uiTiming("timing steps start", "css/css-easing/step-timing-functions-output.html", "steps(2, start)", 0.25, 0.5),
	uiTiming("timing step-start", "css/css-easing/step-timing-functions-output.html", "step-start", 0.5, 1),
	uiTiming("timing step-end", "css/css-easing/step-timing-functions-output.html", "step-end", 0.5, 0),
	uiTiming("timing linear() evenly spread", "css/css-easing/linear-timing-functions-output.html", "linear(0, 0.25, 1)", 0.25, 0.125),
	uiTiming("timing linear() with a stop", "css/css-easing/linear-timing-functions-output.html", "linear(0, 0.25 75%, 1)", 0.5, 1.0/6),
	uiTiming("timing linear() two-percent point", "css/css-easing/linear-timing-functions-output.html", "linear(0, 0.5 25% 75%, 1)", 0.5, 0.5),
	uiTiming("timing linear() clamps a backward stop", "css/css-easing/linear-timing-functions-output.html", "linear(0, 0.25 50%, 0.75 25%, 1)", 0.75, 0.875),
	uiTiming("timing linear() takes the later of equal stops", "css/css-easing/linear-timing-functions-output.html", "linear(0, 0.5 50%, 0.8 50%, 1)", 0.5, 0.8),
	{
		name: "invalid timing functions drop the declaration",
		spec: "css-easing-2 §2.2–3",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			#a, #b, #c, #d { width: 10px; height: 10px; animation: fade 1000ms linear -500ms; }
			#a { animation-timing-function: steps(1, jump-none); }
			#b { animation-timing-function: steps(2.5); }
			#c { animation-timing-function: cubic-bezier(1.5, 0, 0, 1); }
			#d { animation-timing-function: linear(0.5); }`,
		tree: nCol("#p", nBox("#a"), nBox("#b"), nBox("#c"), nBox("#d")),
		want: []confCheck{
			wantComputed("a", cpOpacity, 0.5), wantComputed("b", cpOpacity, 0.5),
			wantComputed("c", cpOpacity, 0.5), wantComputed("d", cpOpacity, 0.5),
		},
	},
	{
		name: "animation applies at its start instant",
		spec: "css-animations-1 §3 (the active phase begins at the start)",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			#a { width: 10px; height: 10px; opacity: 1; animation: fade 1000ms linear; }
			#b { width: 10px; height: 10px; opacity: 1; animation: fade 1000ms steps(4, jump-start); }`,
		tree: nCol("#p", nBox("#a"), nBox("#b")),
		want: []confCheck{wantComputed("a", cpOpacity, 0.0), wantComputed("b", cpOpacity, 0.25)},
	},
	{
		name: "animation-direction reverse eases the directed progress",
		spec: "web-animations-1 §4.8 (directed then transformed progress)",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			#b { width: 10px; height: 10px; animation: fade 1000ms ease-in -250ms reverse; }`,
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, 0.621862)},
	},
	{
		name: "animation-direction alternate reverses odd iterations",
		spec: "css-animations-1 §4.5",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			#b { width: 10px; height: 10px; animation: fade 1000ms linear -1250ms infinite alternate; }`,
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, 0.75)},
	},
	{
		name: "animation timeline runs on after a negative delay",
		spec: "css-animations-1 §4.4 (negative delay starts part-way)",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			#b { width: 10px; height: 10px; animation: fade 1000ms linear -400ms infinite; }`,
		tree: nBox("#b"),
		act:  func(f *confFixture) { uiAdvance(f, 700*time.Millisecond) },
		want: []confCheck{wantComputed("b", cpOpacity, 0.1)},
	},
	{
		name: "animation-fill-mode forwards holds the end after the count",
		spec: "css-animations-1 §4.8",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 0.5; } }
			#a, #b { width: 10px; height: 10px; opacity: 1; }
			#a { animation: fade 1000ms linear -2500ms 2 forwards; }
			#b { animation: fade 1000ms linear -2500ms 2; }`,
		tree: nCol("#p", nBox("#a"), nBox("#b")),
		want: []confCheck{wantComputed("a", cpOpacity, 0.5), wantComputed("b", cpOpacity, 1.0)},
	},
	{
		name: "animation-fill-mode backwards shows the first frame during the delay",
		spec: "css-animations-1 §4.8",
		css: `@keyframes fade { from { opacity: 0.25; } to { opacity: 0.5; } }
			#a, #b { width: 10px; height: 10px; opacity: 1; }
			#a { animation: fade 1000ms linear 500ms backwards; }
			#b { animation: fade 1000ms linear 500ms; }`,
		tree: nCol("#p", nBox("#a"), nBox("#b")),
		act:  func(f *confFixture) { uiAdvance(f, 250*time.Millisecond) },
		want: []confCheck{wantComputed("a", cpOpacity, 0.25), wantComputed("b", cpOpacity, 1.0)},
	},
	{
		name: "animation-play-state paused holds the frame",
		spec: "css-animations-1 §4.7",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			#b { width: 10px; height: 10px; animation: fade 1000ms linear -300ms paused; }`,
		tree: nBox("#b"),
		act:  func(f *confFixture) { uiAdvance(f, 400*time.Millisecond) },
		want: []confCheck{wantComputed("b", cpOpacity, 0.3)},
	},
	{
		name: "keyframes without a to frame end on the underlying value",
		spec: "css-animations-1 §3 (missing 0%/100% keyframes)",
		css: `@keyframes fade { from { opacity: 0; } }
			#b { width: 10px; height: 10px; opacity: 1; animation: fade 1000ms linear -500ms; }`,
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, 0.5)},
	},
	{
		name: "keyframes with only a middle frame",
		spec: "css-animations-1 §3 (missing 0%/100% keyframes)",
		css: `@keyframes dip { 50% { opacity: 0; } }
			#b { width: 10px; height: 10px; opacity: 1; animation: dip 1000ms linear -250ms; }`,
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, 0.5)},
	},
	{
		name: "the timing function applies per keyframe interval",
		spec: "css-animations-1 §3.2 (timing per keyframe interval)",
		css: `@keyframes fade { 0% { opacity: 0; } 50% { opacity: 0.5; } 100% { opacity: 1; } }
			#b { width: 10px; height: 10px; animation: fade 1000ms ease -250ms; }`,
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, 0.401202)},
	},
	{
		name: "a keyframe's own timing function",
		spec: "css-animations-1 §3.2 (animation-timing-function in a keyframe)",
		css: `@keyframes fade { 0% { opacity: 0; animation-timing-function: steps(2); } 100% { opacity: 1; } }
			#b { width: 10px; height: 10px; animation: fade 1000ms linear -300ms; }`,
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, 0.0)},
	},
	{
		name: "animation-composition add sums onto the underlying value",
		spec: "css-animations-2 §4.1 (animation-composition)",
		css: `@keyframes lift { from { opacity: 0.5; } to { opacity: 0.5; } }
			#a, #b, #c { width: 10px; height: 10px; opacity: 0.25; }
			#a { animation: lift 1000ms linear -500ms; animation-composition: add; }
			#b { animation: lift 1000ms linear -500ms; animation-composition: accumulate; }
			#c { animation: lift 1000ms linear -500ms; animation-composition: replace; }`,
		tree: nCol("#p", nBox("#a"), nBox("#b"), nBox("#c")),
		want: []confCheck{
			wantComputed("a", cpOpacity, 0.75), wantComputed("b", cpOpacity, 0.75),
			wantComputed("c", cpOpacity, 0.5),
		},
	},
	{
		name: "transition interpolates on the timeline",
		spec: "css-transitions-1 §3",
		css:  `#b { width: 10px; height: 10px; opacity: 0; transition: opacity 1000ms linear; } #b.on { opacity: 1; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 250*time.Millisecond)
		},
		want: []confCheck{wantComputed("b", cpOpacity, 0.25)},
	},
	{
		name: "transition-timing-function eases the transition",
		spec: "css-transitions-1 §2.3",
		css:  `#b { width: 10px; height: 10px; opacity: 0; transition: opacity 1000ms ease; } #b.on { opacity: 1; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 500*time.Millisecond)
		},
		want: []confCheck{wantComputed("b", cpOpacity, 0.802403)},
	},
	{
		name: "transition-delay holds the start value",
		spec: "css-transitions-1 §2.4",
		css:  `#b { width: 10px; height: 10px; opacity: 0; transition: opacity 1000ms linear 200ms; } #b.on { opacity: 1; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 100*time.Millisecond)
			if got := f.values("b").Opacity; got != 0 {
				f.t.Errorf("opacity %v inside the delay, want the start 0", got)
			}
			uiAdvance(f, 600*time.Millisecond)
		},
		want: []confCheck{wantComputed("b", cpOpacity, 0.5)},
	},
	{
		name: "transition lists time each property",
		spec: "css-transitions-1 §2 (the transition-* lists)",
		css: `#b { width: 10px; height: 10px; opacity: 0; background-color: #000000;
				transition: opacity 1000ms linear, background-color 500ms linear; }
			#b.on { opacity: 1; background-color: #ffffff; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 250*time.Millisecond)
		},
		want: []confCheck{
			wantComputed("b", cpOpacity, 0.25),
			wantComputed("b", cpBackground, render.Color(0xff808080)),
		},
	},
	{
		name: "transition of a length",
		spec: "css-transitions-1 §3 (css-values-4 length interpolation)",
		css:  `#b { width: 100px; height: 10px; transition: width 1000ms linear; } #b.on { width: 200px; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 500*time.Millisecond)
		},
		want: []confCheck{wantComputed("b", cpUIWidth, 150)},
	},
	{
		name: "transition-behavior allow-discrete flips at the midpoint",
		spec: "css-transitions-2 §2.1 (transition-behavior)",
		css: `#b { width: 10px; height: 10px; isolation: isolate; transition: isolation 1000ms linear allow-discrete; }
			#b.on { isolation: auto; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 490*time.Millisecond)
			if !f.values("b").Isolate {
				f.t.Error("the discrete value flipped before halfway")
			}
			uiAdvance(f, 20*time.Millisecond)
		},
		want: []confCheck{wantComputed("b", cpUIIsolate, false)},
	},
	{
		name: "discrete properties snap without allow-discrete",
		spec: "css-transitions-2 §2.1 (transition-behavior)",
		css: `#b { width: 10px; height: 10px; isolation: isolate; transition: isolation 1000ms linear; }
			#b.on { isolation: auto; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 100*time.Millisecond)
		},
		want: []confCheck{wantComputed("b", cpUIIsolate, false)},
	},
	{
		name: "@starting-style starts the entry transition",
		spec: "css-transitions-2 §3 (@starting-style)",
		css:  `#b { width: 10px; height: 10px; opacity: 1; transition: opacity 1000ms linear; @starting-style { opacity: 0; } }`,
		tree: nBox("#b"),
		act:  func(f *confFixture) { uiAdvance(f, 500*time.Millisecond) },
		want: []confCheck{wantComputed("b", cpOpacity, 0.5)},
	},
	{
		name: "keyframes animate every animatable longhand",
		spec: "css-animations-1 §3, web-animations-1 §5.3 (discrete values flip at 50%)",
		css: `@keyframes grow { from { width: 100px; text-align: left; } to { width: 200px; text-align: right; } }
			#a, #b { height: 10px; width: 50px; }
			#a { animation: grow 1000ms linear -400ms; }
			#b { animation: grow 1000ms linear -600ms; }`,
		tree: nCol("#p", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantComputed("a", cpUIWidth, 140), wantComputed("b", cpUIWidth, 160),
			wantComputed("a", cpUITextAlign, style.AlignLeft), wantComputed("b", cpUITextAlign, style.AlignRight),
			wantRect("a", 0, 0, 140, 10),
		},
	},
	{
		name: "the later animation in the list wins the property",
		spec: "css-animations-1 §3 (animation-name order), web-animations-1 §5.4",
		css: `@keyframes lo { from, to { opacity: 0.25; } } @keyframes hi { from, to { opacity: 0.75; } }
			#b { width: 10px; height: 10px; animation: lo 1000ms linear -100ms, hi 1000ms linear -100ms; }`,
		tree: nBox("#b"),
		want: []confCheck{wantComputed("b", cpOpacity, 0.75)},
	},
	{
		name: "animation longhand lists repeat to the name count",
		spec: "css-animations-1 §3 (lists matched by index, repeated)",
		css: `@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
			@keyframes wash { from { background-color: #000000; } to { background-color: #ffffff; } }
			#b { width: 10px; height: 10px; animation-name: fade, wash;
				animation-duration: 1000ms, 2000ms; animation-timing-function: linear; animation-delay: -500ms; }`,
		tree: nBox("#b"),
		want: []confCheck{
			wantComputed("b", cpOpacity, 0.5),
			wantComputed("b", cpBackground, render.Color(0xff404040)),
		},
	},
	{
		name: "transition-delay negative starts part-way",
		spec: "css-transitions-1 §2.4 (a negative delay begins part-way)",
		css:  `#b { width: 10px; height: 10px; opacity: 0; transition: opacity 1000ms linear -250ms; } #b.on { opacity: 1; }`,
		tree: nBox("#b"),
		act: func(f *confFixture) {
			nodeOf(f.get("b")).AddClass("on")
			f.values("b")
			uiAdvance(f, 250*time.Millisecond)
		},
		want: []confCheck{wantComputed("b", cpOpacity, 0.5)},
	},
}

// confExceptionsInteraction are the Interaction group's documented
// deviations.
var confExceptionsInteraction = []confException{
	{
		name: "appearance none keeps the widget chrome",
		spec: "css-ui-4 §9.1",
		css:  `#a { appearance: none; }`,
		tree: uiCheck("#a"),
		want: []confCheck{wantProbe("the check still paints", func(f *confFixture) any {
			b := boundsOf(f.get("a"))
			for y := b.Y; y < b.Y+b.H; y++ {
				for x := b.X; x < b.X+b.W; x++ {
					if f.pixel(x, y) != render.RGB(0xff, 0xff, 0xff) {
						return true
					}
				}
			}
			return false
		}, true)},
		reason: "widget chrome is programmatic theme painting, not CSS pseudo-element chrome; stripping it needs a per-widget unstyled design",
		doc:    "`appearance: none` parses and computes and leaves the widget's chrome painted",
	},
	{
		name: "touch-action manipulation pans both axes",
		spec: "pointerevents-3 §11 (touch-action)",
		css:  uiScrollCSS + `.c { touch-action: manipulation; }`,
		tree: uiScroll(false, 5, "c"),
		act: func(f *confFixture) {
			r := &Router{Root: f.root}
			r.Move(Point{X: 50, Y: 50})
			r.AxisPixels(0, 40, 0)
		},
		want:   []confCheck{uiOffset(0, 40)},
		reason: "the gesture layer exposes pinch only as a claimed GesturePinch a widget opts into, not an axis the scroll gates can mask",
		doc:    "`touch-action`'s pinch semantics (`manipulation`'s zoom half, `pinch-zoom`) are not supported",
	},
}

func TestCSSConformanceInteraction(t *testing.T) { runConformance(t, confInteraction) }

func TestCSSConformanceInteractionExceptions(t *testing.T) {
	runConformanceExceptions(t, confExceptionsInteraction)
}
