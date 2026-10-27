//go:build race

package icons

// raceEnabled reports a -race build, whose instrumentation slows the
// reference walk over the installed themes past a test's budget.
const raceEnabled = true
