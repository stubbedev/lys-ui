// Package testbuild reports how the test binary was built.
package testbuild

import (
	"testing"

	"github.com/stubbedev/lys-lib/debug"
)

// SkipAllocBudget skips an allocation budget in a build whose instrumentation allocates.
func SkipAllocBudget(t testing.TB) {
	t.Helper()
	if debug.Enabled {
		t.Skip("allocation budgets hold for the default build, not lysdebug, whose traces allocate")
	}
	if Race {
		t.Skip("allocation budgets hold for the default build, not -race, whose instrumentation allocates")
	}
}
