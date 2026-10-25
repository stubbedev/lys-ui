package widget

// The CSS conformance suite (#159 acceptance): selected Web Platform
// Tests css/ cases adapted to widget trees, plus spec-derived cases
// where no WPT test translates. Each case is data — a stylesheet, a
// small widget tree, and the outcomes the spec fixes for them, read
// through the observable surface: computed values, arranged bounds,
// hit tests and paint probes. Every expectation is derived by hand
// from the spec — an adapted WPT case keeps its test's setup — and
// none is copied from the engine's output.
//
// The cases live in one table per #159 feature group
// (cssconformance_<group>_test.go). Deliberate gaps sit in each
// group's exception table: those cases assert the documented current
// behavior, and the runner checks docs/css.md states each one.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// confCase is one conformance case.
type confCase struct {
	// name is the subtest name.
	name string
	// wpt is the Web Platform Tests css/ test the case adapts, empty
	// for a case derived from the spec alone.
	wpt string
	// spec names the specification section the expectations follow.
	spec string
	// css is the stylesheet loaded for the case.
	css string
	// tree is the widget tree; ids name the widgets the checks read.
	// It sits in an unstyled column Box with the id "root", the
	// document's body: the window root takes the window size, so the
	// case's own outermost box sizes by its CSS inside it.
	tree confNode
	// vp is the viewport the root is arranged in and the media
	// viewport; zero is 400x300.
	vp Size
	// act runs after the first layout (state flips, theme switches,
	// clock steps); the tree settles again after it.
	act func(f *confFixture)
	// want are the outcomes.
	want []confCheck
}

// confException is a case whose outcome deviates from the spec by
// design: want asserts the documented current behavior, and doc is a
// phrase docs/css.md must contain where it states the exception.
type confException struct {
	confCase
	// reason says why the engine deviates.
	reason string
	// doc is the docs/css.md wording that documents the exception.
	doc string
}

// confNode describes one widget of a case's tree: its "#id.class"
// selector form, its children, and the constructor that makes it from
// its built children. A group needing another widget writes its own
// builder beside its cases.
type confNode struct {
	sel  string
	kids []confNode
	make func(f *confFixture, kids []Widget) Widget
}

// nBox and nCol are row and column Boxes holding kids.
func nBox(sel string, kids ...confNode) confNode { return confBox(Row, sel, kids) }
func nCol(sel string, kids ...confNode) confNode { return confBox(Column, sel, kids) }

func confBox(axis Axis, sel string, kids []confNode) confNode {
	return confNode{sel: sel, kids: kids, make: func(_ *confFixture, ws []Widget) Widget {
		b := NewBox(axis, 0, 0)
		for _, w := range ws {
			b.Append(w, false)
		}
		return b
	}}
}

// nLeaf is a stub of natural size w x h; nBase one reporting a
// baseline b px below its top.
func nLeaf(sel string, w, h int) confNode {
	return confNode{sel: sel, make: func(*confFixture, []Widget) Widget { return newStub(w, h) }}
}

func nBase(sel string, w, h, b int) confNode {
	return confNode{sel: sel, make: func(*confFixture, []Widget) Widget {
		return &baseStub{nat: Size{W: w, H: h}, base: b}
	}}
}

// nText is a black single-line Label of s; nWrap one that wraps.
func nText(sel, s string) confNode { return confLabel(sel, s, false) }
func nWrap(sel, s string) confNode { return confLabel(sel, s, true) }

func confLabel(sel, s string, wrap bool) confNode {
	return confNode{sel: sel, make: func(f *confFixture, _ []Widget) Widget {
		l := NewLabel(f.face, confFaceSize, s, render.RGB(0, 0, 0))
		l.SetWrap(wrap)
		return l
	}}
}

// nButton is a Button around kid, with no built-in padding or radius.
func nButton(sel string, kid confNode) confNode {
	return confNode{sel: sel, kids: []confNode{kid}, make: func(_ *confFixture, ws []Widget) Widget {
		return NewButton(ws[0], 0, 0)
	}}
}

// nEntry is a black-text Entry holding s.
func nEntry(sel, s string) confNode {
	return confNode{sel: sel, make: func(f *confFixture, _ []Widget) Widget {
		e := NewEntry(f.face, confFaceSize, render.RGB(0, 0, 0))
		e.SetText(s)
		return e
	}}
}

// nScroll is a Scroll viewport over kid.
func nScroll(sel string, kid confNode) confNode {
	return confNode{sel: sel, kids: []confNode{kid}, make: func(_ *confFixture, ws []Widget) Widget {
		return NewScroll(ws[0])
	}}
}

// confFixture is a built case: the tree, its named widgets and the
// paint of its settled frame.
type confFixture struct {
	t     *testing.T
	root  Widget
	byID  map[string]Widget
	ids   map[Widget]string
	vp    Size
	face  *render.Typeface
	clock *animClock
	frame []byte
}

// confFaceSize is the font size of the suite's labels and entries.
const confFaceSize = 16

// build makes n's widget and its subtree.
func (f *confFixture) build(n confNode) Widget {
	kids := make([]Widget, len(n.kids))
	for i, k := range n.kids {
		kids[i] = f.build(k)
	}
	w := n.make(f, kids)
	f.name(w, n.sel)
	return w
}

// name applies a "#id.class" selector form to w and records the id.
func (f *confFixture) name(w Widget, sel string) {
	n := nodeOf(w)
	for part := range strings.FieldsFuncSeq(sel, func(r rune) bool { return r == '.' }) {
		if id, ok := strings.CutPrefix(part, "#"); ok {
			n.SetID(id)
			f.byID[id] = w
			f.ids[w] = id
			continue
		}
		n.AddClass(part)
	}
}

// get is the widget named id; the case fails without one.
func (f *confFixture) get(id string) Widget {
	f.t.Helper()
	w, ok := f.byID[id]
	if !ok {
		f.t.Fatalf("no widget #%s in the case tree", id)
	}
	return w
}

// values is id's computed style.
func (f *confFixture) values(id string) *style.Values {
	w := f.get(id)
	return nodeOf(w).style(w)
}

// settle lays the tree out in the viewport until it is stable and
// repaints the frame the paint probes read.
func (f *confFixture) settle() {
	f.t.Helper()
	settle(f.t, f.root, f.vp.W, f.vp.H)
	stride := render.Stride(f.vp.W)
	f.frame = make([]byte, stride*f.vp.H)
	cv := render.New(f.frame, stride, f.vp.W, f.vp.H)
	cv.Clear(cv.Rect(), render.RGB(0xff, 0xff, 0xff))
	f.root.Paint(cv)
}

// pixel reads the settled frame at x, y.
func (f *confFixture) pixel(x, y int) render.Color {
	return render.ColorFromBytes(f.frame[y*render.Stride(f.vp.W)+x*4:])
}

// owner is the nearest named widget at or above w.
func (f *confFixture) owner(w Widget) string {
	for ; w != nil; w = parentOf(w) {
		if id, ok := f.ids[w]; ok {
			return id
		}
	}
	return "<none>"
}

// confCheck is one expected outcome of a case.
type confCheck interface {
	// what describes the probe in a failure.
	what() string
	// probe reads the outcome and reports it with the expectation.
	probe(f *confFixture) (got, want string, ok bool)
}

// confRect checks a widget's arranged bounds.
type confRect struct {
	id         string
	x, y, w, h int
}

// wantRect expects id arranged at x, y with size w x h.
func wantRect(id string, x, y, w, h int) confCheck { return confRect{id, x, y, w, h} }

func (c confRect) what() string { return "bounds of #" + c.id }

func (c confRect) probe(f *confFixture) (string, string, bool) {
	got := boundsOf(f.get(c.id))
	want := render.Rect{X: c.x, Y: c.y, W: c.w, H: c.h}
	return fmt.Sprint(got), fmt.Sprint(want), got == want
}

// confProp names a computed value and reads it from the values.
type confProp struct {
	name string
	read func(v *style.Values) any
}

// The computed values several groups read; a group's own readers live
// beside its cases.
var (
	cpColor      = confProp{"color", func(v *style.Values) any { return v.Color }}
	cpBackground = confProp{"background-color", func(v *style.Values) any { return v.Background }}
	cpOpacity    = confProp{"opacity", func(v *style.Values) any { return v.Opacity }}
	cpFontSize   = confProp{"font-size", func(v *style.Values) any { return v.FontSize }}
	cpFontWeight = confProp{"font-weight", func(v *style.Values) any { return v.FontWeight }}
)

// confComputed checks one computed value.
type confComputed struct {
	id   string
	prop confProp
	want any
}

// wantComputed expects id's computed prop to be want: floats within 1e-3,
// colors exactly, anything else by its printed form.
func wantComputed(id string, prop confProp, want any) confCheck { return confComputed{id, prop, want} }

func (c confComputed) what() string { return "computed " + c.prop.name + " of #" + c.id }

func (c confComputed) probe(f *confFixture) (string, string, bool) {
	got := c.prop.read(f.values(c.id))
	return confFormat(got), confFormat(c.want), confEqual(got, c.want)
}

// confComputedLike expects id's computed prop to equal like's: the
// value a bare widget of the same kind takes from the theme layer, for
// a rule that must not have applied.
type confComputedLike struct {
	id, like string
	prop     confProp
}

// wantComputedLike expects id's computed prop to be like's.
func wantComputedLike(id, like string, prop confProp) confCheck {
	return confComputedLike{id, like, prop}
}

func (c confComputedLike) what() string {
	return "computed " + c.prop.name + " of #" + c.id + " against #" + c.like
}

func (c confComputedLike) probe(f *confFixture) (string, string, bool) {
	got, want := c.prop.read(f.values(c.id)), c.prop.read(f.values(c.like))
	return confFormat(got), confFormat(want), confEqual(got, want)
}

// confEqual compares a probed value with its expectation.
func confEqual(got, want any) bool {
	g, gok := got.(float64)
	w, wok := want.(float64)
	if gok && wok {
		return math.Abs(g-w) <= 1e-3
	}
	return confFormat(got) == confFormat(want)
}

// confFormat prints a probed value; colors print as #aarrggbb.
func confFormat(v any) string {
	if c, ok := v.(render.Color); ok {
		return fmt.Sprintf("#%08x", uint32(c))
	}
	return fmt.Sprintf("%v", v)
}

// confHit checks which named widget a point hits.
type confHit struct {
	x, y int
	id   string
}

// wantHit expects the point x, y to hit id (or a descendant of it that
// carries no id); "<none>" expects no named widget.
func wantHit(x, y int, id string) confCheck { return confHit{x, y, id} }

func (c confHit) what() string { return fmt.Sprintf("hit test at (%d,%d)", c.x, c.y) }

func (c confHit) probe(f *confFixture) (string, string, bool) {
	got := "<none>"
	if w := f.root.HitTest(Point{X: c.x, Y: c.y}); w != nil {
		got = f.owner(w)
	}
	return got, c.id, got == c.id
}

// confPixel probes the settled frame's color at one point.
type confPixel struct {
	x, y int
	want render.Color
	tol  int
}

// wantPixel expects the frame at x, y to be want, each channel within 2
// counts of rounding.
func wantPixel(x, y int, want render.Color) confCheck { return confPixel{x, y, want, 2} }

// wantPixelNear is wantPixel with a wider per-channel tolerance, for
// approximations the spec allows (a box-blur Gaussian).
func wantPixelNear(x, y int, want render.Color, tol int) confCheck { return confPixel{x, y, want, tol} }

func (c confPixel) what() string { return fmt.Sprintf("pixel at (%d,%d)", c.x, c.y) }

func (c confPixel) probe(f *confFixture) (string, string, bool) {
	got := f.pixel(c.x, c.y)
	ok := true
	for _, sh := range []uint{0, 8, 16, 24} {
		d := int(uint8(got>>sh)) - int(uint8(c.want>>sh))
		if d < -c.tol || d > c.tol {
			ok = false
		}
	}
	return confFormat(got), confFormat(c.want), ok
}

// confProbe is the escape hatch: a named reading of the fixture.
type confProbe struct {
	desc string
	read func(f *confFixture) any
	want any
}

// wantProbe expects read to report want.
func wantProbe(desc string, read func(f *confFixture) any, want any) confCheck {
	return confProbe{desc, read, want}
}

func (c confProbe) what() string { return c.desc }

func (c confProbe) probe(f *confFixture) (string, string, bool) {
	got := c.read(f)
	return confFormat(got), confFormat(c.want), confEqual(got, c.want)
}

// runConformance runs a group's cases, each as its own subtest.
func runConformance(t *testing.T, cases []confCase) {
	t.Helper()
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.name] {
			t.Fatalf("duplicate conformance case %q", c.name)
		}
		seen[c.name] = true
		t.Run(c.name, func(t *testing.T) { c.run(t) })
	}
}

// run builds and settles the case, then checks every outcome.
func (c confCase) run(t *testing.T) {
	if c.spec == "" || len(c.want) == 0 {
		t.Fatalf("case %q needs a spec reference and at least one check", c.name)
	}
	resetMedia(t)
	prevTheme := Current()
	SetTheme(DarkTheme())
	t.Cleanup(func() { SetTheme(prevTheme) })
	f := &confFixture{
		t:     t,
		byID:  map[string]Widget{},
		ids:   map[Widget]string{},
		vp:    c.vp,
		face:  goldenFace(t),
		clock: pinAnimClock(t),
	}
	if f.vp == (Size{}) {
		f.vp = Size{W: 400, H: 300}
	}
	SetMediaViewport(f.vp.W, f.vp.H)
	body := NewBox(Column, 0, 0)
	body.Append(f.build(c.tree), false)
	f.name(body, "#root")
	f.root = body
	loadCSS(t, c.css)
	f.settle()
	if c.act != nil {
		c.act(f)
		f.settle()
	}
	source := c.wpt
	if source == "" {
		source = c.spec
	}
	for _, chk := range c.want {
		if got, want, ok := chk.probe(f); !ok {
			t.Errorf("%s [%s]: %s: got %s, want %s", c.name, source, chk.what(), got, want)
		}
	}
}

// runConformanceExceptions runs a group's documented deviations: each
// asserts the engine's current behavior, and docs/css.md must state it
// (compared with whitespace runs collapsed, so the wording may wrap).
func runConformanceExceptions(t *testing.T, list []confException) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "docs", "css.md"))
	if err != nil {
		t.Fatalf("read docs/css.md: %v", err)
	}
	doc := strings.Join(strings.Fields(string(raw)), " ")
	cases := make([]confCase, len(list))
	for i, e := range list {
		if e.reason == "" || e.doc == "" {
			t.Fatalf("exception %q needs a reason and its docs/css.md wording", e.name)
		}
		if !strings.Contains(doc, strings.Join(strings.Fields(e.doc), " ")) {
			t.Errorf("exception %q: docs/css.md does not state %q", e.name, e.doc)
		}
		cases[i] = e.confCase
	}
	runConformance(t, cases)
}
