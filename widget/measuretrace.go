package widget

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/stubbedev/lys-lib/debug"
)

// measureMisses counts, in lysdebug builds, the measures that missed
// their cache since the last MeasureMissReport, keyed by reason and
// widget type.
var measureMisses = map[string]int{}

// noteMeasureMiss records why n's measure missed its cache.
func noteMeasureMiss(n *node, con Constraints) {
	var reason string
	switch {
	case !n.measureSeen:
		reason = "uncached"
	case n.measureDirty:
		reason = "dirty"
	case n.styleDirty:
		reason = "style"
	case n.styleSeen != styleGen:
		reason = "stylegen"
	default:
		reason = fmt.Sprintf("constraints(%v, was %v | %v)", con.Max, n.measureIn.Max, n.measureIn2.Max)
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
