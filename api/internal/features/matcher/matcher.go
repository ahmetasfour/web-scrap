package matcher

import (
	"regexp"
	"strings"
	"unicode"
)

var legalFormRe = regexp.MustCompile(`(?i)\b(GmbH|AG|KG|OHG|eV|eG|GbR|UG|Co\.|mbH|KGaA|SE|Stiftung|Verein|Gesellschaft|Genossenschaft|Verwaltung|Wohnungsbau|Wohnungs)\b`)
var multiSpaceRe = regexp.MustCompile(`\s+`)

// Normalize lowercases, removes legal form suffixes, strips punctuation.
func Normalize(name string) string {
	name = legalFormRe.ReplaceAllString(name, " ")
	name = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return ' '
		}
		return r
	}, name)
	name = multiSpaceRe.ReplaceAllString(name, " ")
	return strings.ToLower(strings.TrimSpace(name))
}

// LevenshteinDistance computes the edit distance between two strings.
func LevenshteinDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 1; j <= lb; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			dp[i][j] = min3(dp[i-1][j]+1, dp[i][j-1]+1, dp[i-1][j-1]+cost)
		}
	}
	return dp[la][lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// Similarity returns a score between 0.0 and 1.0 based on normalized names.
func Similarity(query, candidate string) float64 {
	nq := Normalize(query)
	nc := Normalize(candidate)
	if nq == nc {
		return 1.0
	}
	rq, rc := []rune(nq), []rune(nc)
	maxLen := len(rq)
	if len(rc) > maxLen {
		maxLen = len(rc)
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := LevenshteinDistance(nq, nc)
	return 1.0 - float64(dist)/float64(maxLen)
}

// IsGoodMatch returns true when similarity >= threshold.
func IsGoodMatch(query, candidate string, threshold float64) bool {
	return Similarity(query, candidate) >= threshold
}
