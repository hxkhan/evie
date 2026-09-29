package diag

import (
	"sort"
	"strings"
)

// Suggest returns the candidate that looks like what the author meant by name:
// the same name in another case, or one a typo or two away. The second result
// is false when nothing is close enough.
func Suggest(name string, candidates []string) (string, bool) {
	if name == "" {
		return "", false
	}

	// how far a candidate may be, scaled to the length of the name
	allowed := 1
	switch {
	case len(name) > 8:
		allowed = 3
	case len(name) > 4:
		allowed = 2
	}

	best, bestDist := "", allowed+1
	sorted := append([]string(nil), candidates...)
	sort.Strings(sorted)
	for _, c := range sorted {
		if c == name {
			continue
		}
		if strings.EqualFold(c, name) {
			return c, true
		}
		if d := levenshtein(name, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best, best != ""
}

// levenshtein is the edit distance between two strings
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
