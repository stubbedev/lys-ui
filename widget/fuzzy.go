package widget

import (
	"unicode"
	"unicode/utf8"
)

// The fuzzy match bonuses, per matched rune.
const (
	fuzzyMatch       = 1
	fuzzyConsecutive = 5
	fuzzyWordStart   = 6
	fuzzyTextStart   = 8
	// fuzzyBonusScale weighs the bonuses against the gap penalty, one
	// per unmatched rune inside the match.
	fuzzyBonusScale = 16
	// fuzzyTieScale keeps the length tie-break, one per unmatched rune
	// of the whole text, below every other difference: of two equal
	// matches the shorter text ranks first.
	fuzzyTieScale = 256
)

// FuzzyScore ranks how well query matches text, cmdk's way: every
// query rune appears in text in order, case-insensitively; runs of
// consecutive matches, matches at word starts and at the very start
// score higher, unmatched runes inside the match cost, and of two
// otherwise equal matches the shorter text wins. ok is false
// when query is not a subsequence of text. An empty query matches with
// score 0.
func FuzzyScore(query, text string) (score int, ok bool) {
	if query == "" {
		return 0, true
	}
	q, size := utf8.DecodeRuneInString(query)
	bonus, matched, first, last := 0, 0, -1, -1
	prev, i := rune(0), -1
	for _, r := range text {
		i++
		if unicode.ToLower(r) == unicode.ToLower(q) {
			b := fuzzyMatch
			switch {
			case i == 0:
				b += fuzzyTextStart
			case !unicode.IsLetter(prev) && !unicode.IsDigit(prev), unicode.IsLower(prev) && unicode.IsUpper(r):
				b += fuzzyWordStart
			}
			if matched > 0 && last == i-1 {
				b += fuzzyConsecutive
			}
			bonus += b
			matched++
			if first < 0 {
				first = i
			}
			last = i
			query = query[size:]
			if query == "" {
				rest := utf8.RuneCountInString(text) - matched
				return (bonus*fuzzyBonusScale-(last-first+1-matched))*fuzzyTieScale - rest, true
			}
			q, size = utf8.DecodeRuneInString(query)
		}
		prev = r
	}
	return 0, false
}
