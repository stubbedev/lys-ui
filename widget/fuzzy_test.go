package widget

import (
	"slices"
	"testing"
)

func TestFuzzyScoreMatches(t *testing.T) {
	for _, tc := range []struct {
		query, text string
		ok          bool
	}{
		{"", "anything", true},
		{"cal", "Calendar", true},
		{"cdr", "Calendar", true},
		{"rdc", "Calendar", false},
		{"émo", "Émoji", true},
		{"xyz", "Calendar", false},
	} {
		if _, ok := FuzzyScore(tc.query, tc.text); ok != tc.ok {
			t.Errorf("FuzzyScore(%q, %q) ok = %v, want %v", tc.query, tc.text, ok, tc.ok)
		}
	}
}

// Prefixes outrank word starts, which outrank scattered matches.
func TestFuzzyScoreRanks(t *testing.T) {
	texts := []string{"Toggle sidebar", "Settings", "Search emoji", "Profile settings"}
	ranked := slices.Clone(texts)
	slices.SortStableFunc(ranked, func(a, b string) int {
		sa, _ := FuzzyScore("se", a)
		sb, _ := FuzzyScore("se", b)
		return sb - sa
	})
	want := []string{"Settings", "Search emoji", "Profile settings", "Toggle sidebar"}
	if !slices.Equal(ranked, want) {
		t.Errorf("ranked %v, want %v", ranked, want)
	}
}
