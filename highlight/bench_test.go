package highlight

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/testbuild"
)

// benchSource is forty lines of C-family code, the most a launcher
// preview shows at once.
var benchSource = strings.Repeat(`// Sum adds the values up, skipping none.
fn sum(values: &[u32]) -> Option<u64> {
	let mut total: u64 = 0; // running
	for v in values.iter() { total += *v as u64; }
	println!("total {}", total);
	Some(total)
}
/* a block
   comment */
`, 5)[:]

func benchLines() [][]rune {
	lines := strings.Split(benchSource, "\n")[:40]
	out := make([][]rune, len(lines))
	for i, l := range lines {
		out[i] = []rune(l)
	}
	return out
}

// BenchmarkLexicon40Lines highlights forty lines as a preview does.
func BenchmarkLexicon40Lines(b *testing.B) {
	lines := benchLines()
	b.ReportAllocs()
	for b.Loop() {
		state := 0
		for _, l := range lines {
			_, state = langRust.Highlight(l, state)
		}
	}
}

func BenchmarkGo40Lines(b *testing.B) {
	lines := benchLines()
	b.ReportAllocs()
	for b.Loop() {
		state := 0
		for _, l := range lines {
			_, state = Go{}.Highlight(l, state)
		}
	}
}

// Highlighting a preview's forty lines allocates about one span list per
// line, whatever the language.
func TestHighlightingFortyLinesAllocatesPerLine(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	lines := benchLines()
	allocs := testing.AllocsPerRun(20, func() {
		state := 0
		for _, l := range lines {
			_, state = langRust.Highlight(l, state)
		}
	})
	if allocs > 2*float64(len(lines)) {
		t.Errorf("forty lines allocate %.0f times, want at most %d", allocs, 2*len(lines))
	}
}
