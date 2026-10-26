package style

import (
	"math"
	"reflect"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// px and pct build the sizing values the table expects.
func pxS(n int) Sizing             { return PxSizing(n) }
func pctS(p float64) Sizing        { return Sizing{Kind: SizingLength, Len: Length{Pct: p}} }
func mixS(n int, p float64) Sizing { return Sizing{Kind: SizingLength, Len: Length{Px: n, Pct: p}} }

// TestLayoutPropsCompute pins every layout property's computed value:
// the keywords, the length-percentage and calc() forms, the content
// keywords, and each shorthand's expansion.
func TestLayoutPropsCompute(t *testing.T) {
	type pick func(v *Values) any
	lay := func(f func(l Layout) any) pick { return func(v *Values) any { return f(v.Layout) } }
	cases := []struct {
		decl string
		got  pick
		want any
	}{
		{"display: block", lay(func(l Layout) any { return l.Display }), DisplayBlock},
		{"display: inline", lay(func(l Layout) any { return l.Display }), DisplayInline},
		{"display: inline-block", lay(func(l Layout) any { return l.Display }), DisplayInlineBlock},
		{"display: flow-root", lay(func(l Layout) any { return l.Display }), DisplayFlowRoot},
		{"display: flex", lay(func(l Layout) any { return l.Display }), DisplayFlex},
		{"display: inline-flex", lay(func(l Layout) any { return l.Display }), DisplayInlineFlex},
		{"display: grid", lay(func(l Layout) any { return l.Display }), DisplayGrid},
		{"display: inline-grid", lay(func(l Layout) any { return l.Display }), DisplayInlineGrid},
		{"display: contents", lay(func(l Layout) any { return l.Display }), DisplayContents},
		{"display: none", lay(func(l Layout) any { return l.Display }), DisplayNone},
		{"display: inline flex", lay(func(l Layout) any { return l.Display }), DisplayInlineFlex},
		{"display: flow-root block", lay(func(l Layout) any { return l.Display }), DisplayFlowRoot},
		{"display: inline flow-root", lay(func(l Layout) any { return l.Display }), DisplayInlineBlock},
		{"display: block grid", lay(func(l Layout) any { return l.Display }), DisplayGrid},

		{"position: relative", lay(func(l Layout) any { return l.Position }), PositionRelative},
		{"position: absolute", lay(func(l Layout) any { return l.Position }), PositionAbsolute},
		{"position: fixed", lay(func(l Layout) any { return l.Position }), PositionFixed},
		{"position: sticky", lay(func(l Layout) any { return l.Position }), PositionSticky},
		{"top: 10px", lay(func(l Layout) any { return l.Inset[0] }), pxS(10)},
		{"left: 50%", lay(func(l Layout) any { return l.Inset[3] }), pctS(50)},
		{"right: auto", lay(func(l Layout) any { return l.Inset[1] }), Sizing{}},
		{"bottom: -4px", lay(func(l Layout) any { return l.Inset[2] }), pxS(-4)},
		{"bottom: calc(100% - 10px)", lay(func(l Layout) any { return l.Inset[2] }), mixS(-10, 100)},
		{"inset: 1px 2px", lay(func(l Layout) any { return l.Inset }), [4]Sizing{pxS(1), pxS(2), pxS(1), pxS(2)}},
		{"inset: 0 auto", lay(func(l Layout) any { return l.Inset }), [4]Sizing{pxS(0), {}, pxS(0), {}}},
		{"z-index: 5", lay(func(l Layout) any { return l.ZIndex }), 5},
		{"z-index: -3", lay(func(l Layout) any { return l.ZIndex }), -3},
		{"z-index: auto", lay(func(l Layout) any { return l.ZIndex }), 0},

		{"width: 100px", lay(func(l Layout) any { return l.Width }), pxS(100)},
		{"width: 50%", lay(func(l Layout) any { return l.Width }), pctS(50)},
		{"width: auto", lay(func(l Layout) any { return l.Width }), Sizing{}},
		{"width: min-content", lay(func(l Layout) any { return l.Width }), Sizing{Kind: SizingMinContent}},
		{"width: max-content", lay(func(l Layout) any { return l.Width }), Sizing{Kind: SizingMaxContent}},
		{"width: fit-content", lay(func(l Layout) any { return l.Width }), Sizing{Kind: SizingFitContent, Len: Length{Pct: 100}}},
		{"width: fit-content(200px)", lay(func(l Layout) any { return l.Width }), Sizing{Kind: SizingFitContent, Len: Length{Px: 200}}},
		{"width: calc(50% + 2em)", lay(func(l Layout) any { return l.Width }), mixS(32, 50)},
		{"width: calc((100% - 20px) / 2)", lay(func(l Layout) any { return l.Width }), mixS(-10, 50)},
		{"width: calc(50% + 10px - 10px)", lay(func(l Layout) any { return l.Width }), pctS(50)},
		{"height: 3em", lay(func(l Layout) any { return l.Height }), pxS(48)},
		{"min-width: 0", lay(func(l Layout) any { return l.MinWidth }), pxS(0)},
		{"min-width: 30%", lay(func(l Layout) any { return l.MinWidth }), pctS(30)},
		{"min-height: auto", lay(func(l Layout) any { return l.MinHeight }), Sizing{}},
		{"max-width: none", lay(func(l Layout) any { return l.MaxWidth }), Sizing{}},
		{"max-width: 400px", lay(func(l Layout) any { return l.MaxWidth }), pxS(400)},
		{"max-height: max-content", lay(func(l Layout) any { return l.MaxHeight }), Sizing{Kind: SizingMaxContent}},
		{"box-sizing: border-box", lay(func(l Layout) any { return l.BoxSizing }), BoxSizingBorder},
		{"box-sizing: content-box", lay(func(l Layout) any { return l.BoxSizing }), BoxSizingContent},
		{"aspect-ratio: 16 / 9", lay(func(l Layout) any { return math.Round(l.AspectRatio*1000) / 1000 }), 1.778},
		{"aspect-ratio: 2", lay(func(l Layout) any { return l.AspectRatio }), 2.0},
		{"aspect-ratio: auto", lay(func(l Layout) any { return l.AspectRatio }), 0.0},
		{"aspect-ratio: auto 4 / 2", lay(func(l Layout) any { return l.AspectRatio }), 2.0},
		{"aspect-ratio: 3 / 1 auto", lay(func(l Layout) any { return l.AspectRatio }), 3.0},

		{"flex: none", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{0.0, 0.0, Sizing{}}},
		{"flex: auto", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{1.0, 1.0, Sizing{}}},
		{"flex: 2", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{2.0, 1.0, pxS(0)}},
		{"flex: 2 3", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{2.0, 3.0, pxS(0)}},
		{"flex: 1 0 auto", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{1.0, 0.0, Sizing{}}},
		{"flex: 10px", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{1.0, 1.0, pxS(10)}},
		{"flex: 30% 2", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{2.0, 1.0, pctS(30)}},
		{"flex: 0 0 0", lay(func(l Layout) any { return [3]any{l.FlexGrow, l.FlexShrink, l.FlexBasis} }), [3]any{0.0, 0.0, pxS(0)}},
		{"flex: content", lay(func(l Layout) any { return l.FlexBasis }), Sizing{Kind: SizingContent}},
		{"flex-grow: 2.5", lay(func(l Layout) any { return l.FlexGrow }), 2.5},
		{"flex-shrink: 0", lay(func(l Layout) any { return l.FlexShrink }), 0.0},
		{"flex-basis: 20%", lay(func(l Layout) any { return l.FlexBasis }), pctS(20)},
		{"flex-basis: max-content", lay(func(l Layout) any { return l.FlexBasis }), Sizing{Kind: SizingMaxContent}},
		{"flex-direction: column-reverse", lay(func(l Layout) any { return l.FlexDirection }), FlexColumnReverse},
		{"flex-wrap: wrap-reverse", lay(func(l Layout) any { return l.FlexWrap }), FlexWrapReverse},
		{"flex-flow: column wrap", lay(func(l Layout) any { return [2]any{l.FlexDirection, l.FlexWrap} }), [2]any{FlexColumn, FlexWrapOn}},
		{"flex-flow: wrap-reverse row-reverse", lay(func(l Layout) any { return [2]any{l.FlexDirection, l.FlexWrap} }), [2]any{FlexRowReverse, FlexWrapReverse}},
		{"flex-flow: wrap", lay(func(l Layout) any { return [2]any{l.FlexDirection, l.FlexWrap} }), [2]any{FlexRow, FlexWrapOn}},
		{"order: -1", lay(func(l Layout) any { return l.Order }), -1},

		{"justify-content: center", lay(func(l Layout) any { return l.JustifyContent }), BoxAlign{Kind: BoxCenter}},
		{"justify-content: space-between", lay(func(l Layout) any { return l.JustifyContent }), BoxAlign{Kind: BoxSpaceBetween}},
		{"justify-content: safe center", lay(func(l Layout) any { return l.JustifyContent }), BoxAlign{Kind: BoxCenter, Safe: true}},
		{"justify-content: unsafe flex-end", lay(func(l Layout) any { return l.JustifyContent }), BoxAlign{Kind: BoxFlexEnd}},
		{"justify-content: left", lay(func(l Layout) any { return l.JustifyContent }), BoxAlign{Kind: BoxLeft}},
		{"align-items: baseline", lay(func(l Layout) any { return l.AlignItems }), BoxAlign{Kind: BoxBaseline}},
		{"align-items: first baseline", lay(func(l Layout) any { return l.AlignItems }), BoxAlign{Kind: BoxBaseline}},
		{"align-items: last baseline", lay(func(l Layout) any { return l.AlignItems }), BoxAlign{Kind: BoxLastBaseline}},
		{"align-items: self-end", lay(func(l Layout) any { return l.AlignItems }), BoxAlign{Kind: BoxSelfEnd}},
		{"align-self: auto", lay(func(l Layout) any { return l.AlignSelf }), BoxAlign{Kind: BoxAuto}},
		{"align-self: flex-end", lay(func(l Layout) any { return l.AlignSelf }), BoxAlign{Kind: BoxFlexEnd}},
		{"align-content: space-evenly", lay(func(l Layout) any { return l.AlignContent }), BoxAlign{Kind: BoxSpaceEvenly}},
		{"place-content: center start", lay(func(l Layout) any { return [2]any{l.AlignContent, l.JustifyContent} }), [2]any{BoxAlign{Kind: BoxCenter}, BoxAlign{Kind: BoxStart}}},
		{"place-content: space-between", lay(func(l Layout) any { return [2]any{l.AlignContent, l.JustifyContent} }), [2]any{BoxAlign{Kind: BoxSpaceBetween}, BoxAlign{Kind: BoxSpaceBetween}}},
		{"place-content: safe end unsafe left", lay(func(l Layout) any { return [2]any{l.AlignContent, l.JustifyContent} }), [2]any{BoxAlign{Kind: BoxEnd, Safe: true}, BoxAlign{Kind: BoxLeft}}},
		{"gap: 10px 20px", lay(func(l Layout) any { return [2]any{l.RowGap, l.ColumnGap} }), [2]any{Length{Px: 10}, Length{Px: 20}}},
		{"gap: 5%", lay(func(l Layout) any { return [2]any{l.RowGap, l.ColumnGap} }), [2]any{Length{Pct: 5}, Length{Pct: 5}}},
		{"row-gap: normal", lay(func(l Layout) any { return l.RowGap }), Length{}},
		{"column-gap: 1em", lay(func(l Layout) any { return l.ColumnGap }), Length{Px: 16}},

		{"overflow: hidden", lay(func(l Layout) any { return [2]any{l.OverflowX, l.OverflowY} }), [2]any{OverflowHidden, OverflowHidden}},
		{"overflow: clip auto", lay(func(l Layout) any { return [2]any{l.OverflowX, l.OverflowY} }), [2]any{OverflowClip, OverflowAuto}},
		{"overflow-y: scroll", lay(func(l Layout) any { return l.OverflowY }), OverflowScroll},
		{"visibility: hidden", lay(func(l Layout) any { return l.Visibility }), VisibilityHidden},
		{"visibility: collapse", lay(func(l Layout) any { return l.Visibility }), VisibilityCollapse},
		{"float: inline-end", lay(func(l Layout) any { return l.Float }), FloatInlineEnd},
		{"clear: both", lay(func(l Layout) any { return l.Clear }), ClearBoth},

		{"object-fit: cover", lay(func(l Layout) any { return l.ObjectFit }), render.ImageCover},
		{"object-fit: fill", lay(func(l Layout) any { return l.ObjectFit }), render.ImageStretch},
		{"object-fit: scale-down", lay(func(l Layout) any { return l.ObjectFit }), render.ImageScaleDown},
		{"object-position: left", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{}, {Frac: 0.5}}},
		{"object-position: bottom", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{Frac: 0.5}, {Frac: 1}}},
		{"object-position: 25% 10px", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{Frac: 0.25}, {Px: 10}}},
		{"object-position: top right", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{Frac: 1}, {}}},
		{"object-position: center left", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{}, {Frac: 0.5}}},
		{"object-position: left 10px", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{}, {Px: 10}}},
		{"object-position: right 10px bottom 20%", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{Frac: 1, Px: -10}, {Frac: 0.8}}},
		{"object-position: bottom 5px center", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{Frac: 0.5}, {Frac: 1, Px: -5}}},
		{"object-position: center top 4px", lay(func(l Layout) any { return l.ObjectPosition }), [2]AxisPos{{Frac: 0.5}, {Px: 4}}},

		{"margin: auto", func(v *Values) any { return v.MarginAuto }, [4]bool{true, true, true, true}},
		{"margin: 4px auto", func(v *Values) any { return [2]any{v.MarginAuto, v.Margin} }, [2]any{[4]bool{false, true, false, true}, Sides{4, 0, 4, 0}}},
		{"margin-left: auto", func(v *Values) any { return v.MarginAuto }, [4]bool{false, false, false, true}},
		{"margin-top: -3px", func(v *Values) any { return [2]any{v.MarginAuto, v.Margin.Top} }, [2]any{[4]bool{}, -3}},
	}
	for _, c := range cases {
		s := parseOne(t, "e { "+c.decl+"; }")
		v := computeTree(s, el("e"))
		if got := c.got(&v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: computed %#v, want %#v", c.decl, got, c.want)
		}
	}
}

// TestLayoutPropsInvalid pins the values the layout properties reject:
// each leaves its longhand undeclared and warns.
func TestLayoutPropsInvalid(t *testing.T) {
	for _, decl := range []string{
		"display: run-in", "display: block inline", "display: flex flex",
		"position: center", "z-index: 1.5", "order: 1.5",
		"width: -10px", "width: red", "max-width: auto", "min-width: none", "height: fit-content(-1px)",
		"width: min(100%, 300px)",
		"flex-grow: -1", "flex: 1 2 3 4", "flex: 1 auto 2", "flex-basis: none",
		"flex-direction: up", "flex-flow: row column",
		"justify-content: baseline", "align-items: space-between", "align-self: left",
		"justify-content: safe space-between", "align-content: left",
		"gap: -1px", "gap: 1px 2px 3px", "aspect-ratio: 0", "aspect-ratio: 16 9", "aspect-ratio: 1 / 0",
		"overflow: hidden hidden hidden", "visibility: gone", "float: center", "clear: top",
		"inset: 1px 2px 3px 4px 5px", "margin: auto auto auto auto auto",
		"object-fit: stretch", "object-position: top 10px", "object-position: left right",
		"object-position: center 10px top", "object-position: left 1px top 2px 3px",
		"object-position: 1px 2px 3px",
	} {
		warns := capture(t)
		s := Parse("e { " + decl + "; }")
		v := computeTree(s, el("e"))
		if !v.Set.And(LayoutProps).Empty() || v.MarginAuto != [4]bool{} {
			t.Errorf("%s: declared %v, want the invalid value dropped", decl, v.Layout)
		}
		if len(*warns) == 0 {
			t.Errorf("%s: no warning", decl)
		}
	}
}

// TestLayoutWideKeywords pins the interaction rule's cascade half: a
// CSS-wide keyword returns a layout longhand to undeclared — the
// widget's programmatic layout — while inherit still copies the parent
// and visibility inherits like CSS.
func TestLayoutWideKeywords(t *testing.T) {
	s := parseOne(t, `
		p { display: flex; flex-direction: column; visibility: hidden; }
		.reset { all: unset; }
		.initial { display: initial; flex-grow: initial; }
		.inherit { display: inherit; }
		.shown { visibility: visible; }
	`)
	parent := el("p")
	for _, c := range []struct {
		class string
		check func(v Values) bool
		want  string
	}{
		{"reset", func(v Values) bool {
			return !v.Has(PropDisplay) && !v.Has(PropFlexDirection) && v.Has(PropMinWidth) && v.Layout.Visibility == VisibilityHidden
		}, "layout undeclared, min-width zeroed, visibility inherited"},
		{"initial", func(v Values) bool { return !v.Has(PropDisplay) && !v.Has(PropFlexGrow) }, "display and flex-grow undeclared"},
		{"inherit", func(v Values) bool { return v.Has(PropDisplay) && v.Layout.Display == DisplayFlex }, "display copied from the parent"},
		{"shown", func(v Values) bool { return v.Layout.Visibility == VisibilityVisible }, "visibility visible inside a hidden parent"},
	} {
		v := computeTree(s, parent.add("c", c.class))
		if !c.check(v) {
			t.Errorf(".%s: %+v, want %s", c.class, v.Layout, c.want)
		}
	}
}

// TestLayoutSupports pins @supports for the layout properties: the
// flexbox and sizing values report supported.
func TestLayoutSupports(t *testing.T) {
	s := parseOne(t, `@supports (display: flex) and (gap: 1px) and (width: fit-content) { e { order: 3; } }`)
	if v := computeTree(s, el("e")); v.Layout.Order != 3 {
		t.Error("@supports for flexbox and sizing values did not apply")
	}
}
