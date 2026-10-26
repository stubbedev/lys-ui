package widget

import (
	"slices"

	"github.com/stubbedev/lys-lib/render"
)

// The default level bar offsets, GTK's names and thresholds; the
// theme layer colors a block carrying each (warning, accent, success).
const (
	LevelBarOffsetLow  = "low"
	LevelBarOffsetHigh = "high"
	LevelBarOffsetFull = "full"
)

// LevelOffset is a named threshold: while the value is at or below it
// (and above every lower one), the block carries Name as a class.
type LevelOffset struct {
	Name  string
	Value float64
}

// LevelBar is GTK's continuous level bar: the levelbar element over a
// trough and one filled block (levelbar > trough > block.filled), the
// node tree stylesheets target for gauges. Every part paints from its
// cascade - there is no programmatic color; the theme layer gives the
// trough the view color and the offsets their tints, and a stylesheet
// overrides either. Offsets classify the value:
// the block carries the name of the lowest offset at or above it, so
// `levelbar block.low` restyles a nearly empty gauge.
type LevelBar struct {
	meter
	offsets []LevelOffset
	current string
}

// NewLevelBar returns a continuous level bar at value clamped to
// [0, 1], with GTK's default offsets (low 0.25, high 0.75, full 1).
func NewLevelBar(value float64) *LevelBar {
	b := &LevelBar{offsets: []LevelOffset{
		{LevelBarOffsetLow, 0.25},
		{LevelBarOffsetHigh, 0.75},
		{LevelBarOffsetFull, 1},
	}}
	b.SetElement("levelbar")
	b.initMeter(b, value, "block")
	b.roundTrough = true
	b.trough.fill.AddClass("filled")
	b.syncOffset()
	return b
}

// SetValue clamps v to [0, 1] and repaints the block.
func (b *LevelBar) SetValue(v float64) {
	if b.setValue(v) {
		b.syncOffset()
	}
}

// Offsets reports the thresholds, lowest first.
func (b *LevelBar) Offsets() []LevelOffset { return slices.Clone(b.offsets) }

// AddOffset adds or moves the named threshold.
func (b *LevelBar) AddOffset(name string, value float64) {
	b.offsets = slices.DeleteFunc(b.offsets, func(o LevelOffset) bool { return o.Name == name })
	b.offsets = append(b.offsets, LevelOffset{name, math01(value)})
	slices.SortStableFunc(b.offsets, func(x, y LevelOffset) int {
		switch {
		case x.Value < y.Value:
			return -1
		case x.Value > y.Value:
			return 1
		}
		return 0
	})
	b.syncOffset()
}

// RemoveOffset drops the named threshold.
func (b *LevelBar) RemoveOffset(name string) {
	b.offsets = slices.DeleteFunc(b.offsets, func(o LevelOffset) bool { return o.Name == name })
	b.syncOffset()
}

// Offset reports the offset class the block carries ("" for none).
func (b *LevelBar) Offset() string { return b.current }

// syncOffset moves the block's offset class to the lowest offset at or
// above the value.
func (b *LevelBar) syncOffset() {
	name := ""
	for _, o := range b.offsets {
		if b.value <= o.Value {
			name = o.Name
			break
		}
	}
	if name == b.current {
		return
	}
	if b.current != "" {
		b.trough.fill.RemoveClass(b.current)
	}
	if name != "" {
		b.trough.fill.AddClass(name)
	}
	b.current = name
	b.Invalidate()
}

// Measure wants the 160x10 trough inside the bar's CSS box.
func (b *LevelBar) Measure(con Constraints) Size { return b.measureMeter(b, con) }

// Paint draws the trough and the proportional block.
func (b *LevelBar) Paint(cv *render.Canvas) { b.paintMeter(cv, 0, b.value) }

// Role implements Roleer.
func (b *LevelBar) Role() Role { return RoleLevelBar }

// HitTest returns the bar when p is inside its bounds.
func (b *LevelBar) HitTest(p Point) Widget { return b.hitMeter(b, p) }

// Trough returns the bar's trough node (levelbar > trough), the
// diagnostic view a paint test reads.
func (b *LevelBar) Trough() Widget { return &b.trough }

// Block returns the filled block node (levelbar > trough >
// block.filled), the diagnostic view a paint test reads.
func (b *LevelBar) Block() Widget { return &b.trough.fill }
