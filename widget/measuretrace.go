package widget

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"
)

// measureMisses counts, in lysdebug builds, the measures that missed
// their cache since the last MeasureMissReport, keyed by reason and
// widget type.
var measureMisses = map[string]int{}

// noteMeasureMiss records why n's measure along axis at forSize missed
// its cache.
func noteMeasureMiss(n *node, axis Axis, forSize int) {
	var reason string
	switch {
	case !n.measureFresh:
		reason = "uncached"
	case n.measureDirty:
		reason = "dirty"
	case n.styleDirty:
		reason = "style"
	case n.styleSeen != styleGen:
		reason = "stylegen"
	case n.subStyle:
		reason = "substyle"
	default:
		var seen []string
		for _, e := range n.measures {
			if e.ok {
				seen = append(seen, fmt.Sprintf("%v@%d", e.axis, e.forSize))
			}
		}
		reason = fmt.Sprintf("new size (%v@%d, cached %s)", axis, forSize, strings.Join(seen, " | "))
	}
	kind := "unparented"
	if n.self != nil {
		kind = fmt.Sprintf("%T", n.self)
	}
	measureMisses[reason+" "+kind]++
}

// MeasureMissReport returns the measure-cache misses since the last
// call, most frequent first ("12 uncached *widget.Box, ..."), and clears
// them; "" when there were none or the build lacks the lysdebug tag.
// The app logs it per frame under LYS_DEBUG=measure.
func MeasureMissReport() string {
	if !debug.Enabled || len(measureMisses) == 0 {
		return ""
	}
	keys := slices.SortedFunc(maps.Keys(measureMisses), func(a, b string) int {
		if d := measureMisses[b] - measureMisses[a]; d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, 0, min(len(keys), 12))
	for _, k := range keys[:min(len(keys), 12)] {
		parts = append(parts, fmt.Sprintf("%d %s", measureMisses[k], k))
	}
	clear(measureMisses)
	return strings.Join(parts, ", ")
}

// boundsDamage records, in lysdebug builds, the widgets that owed their
// whole bounds since the last DamageReport and why, keyed by size,
// type and reason.
var boundsDamage = map[string]int{}

// damageReasons is why n would owe its whole bounds now, read before
// takeDamage clears the marks.
func damageReasons(n *node) string {
	if n == nil {
		return ""
	}
	var why []string
	if n.invalid {
		why = append(why, "invalid")
	}
	if n.subInvalid {
		why = append(why, "descendant")
	}
	if n.themeSeen != themeGen {
		why = append(why, "theme")
	}
	if n.styleDmg != styleGen {
		why = append(why, "stylegen")
	}
	return strings.Join(why, "+")
}

// styleBumps records, in lysdebug builds, what stamped every style
// stale since the last DamageReport, keyed by reason.
var styleBumps = map[string]int{}

// bumpStyleGen stamps every widget's style stale and owes the repaint;
// why names the change in the LYS_DEBUG=damage report.
func bumpStyleGen(why string) {
	styleGen++
	markDamage()
	if debug.Enabled {
		styleBumps[why]++
	}
}

// noteDamage records that w owed its whole bounds this frame, and why.
func noteDamage(w Widget, bounds render.Rect, why string) {
	boundsDamage[fmt.Sprintf("%dx%d %T %s", bounds.W, bounds.H, w, why)]++
}

// DamageReport returns the widgets that owed their whole bounds since
// the last call, largest first ("1920x1080 *widget.Box invalid, ..."),
// and clears them; "" when there were none or the build lacks the
// lysdebug tag. The app logs it per frame under LYS_DEBUG=damage.
func DamageReport() string {
	if !debug.Enabled || len(boundsDamage) == 0 && len(styleBumps) == 0 {
		return ""
	}
	area := func(k string) int {
		var w, h int
		_, _ = fmt.Sscanf(k, "%dx%d", &w, &h)
		return w * h
	}
	keys := slices.SortedFunc(maps.Keys(boundsDamage), func(a, b string) int {
		if d := area(b) - area(a); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, 0, min(len(keys), 8))
	for _, k := range keys[:min(len(keys), 8)] {
		if c := boundsDamage[k]; c > 1 {
			k = fmt.Sprintf("%s (x%d)", k, c)
		}
		parts = append(parts, k)
	}
	clear(boundsDamage)
	if len(styleBumps) > 0 {
		bumps := make([]string, 0, len(styleBumps))
		for _, k := range slices.Sorted(maps.Keys(styleBumps)) {
			bumps = append(bumps, fmt.Sprintf("%s (x%d)", k, styleBumps[k]))
		}
		clear(styleBumps)
		parts = append(parts, "stylegen bumped by: "+strings.Join(bumps, ", "))
	}
	return strings.Join(parts, ", ")
}
