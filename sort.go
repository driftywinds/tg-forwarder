package main

import (
	"sort"
	"strings"
	"unicode"
)

// sortBundleMessages sorts a slice of BundleMessage in-place by FileName
// using natural (human-friendly) sort order.
//
// Natural sort splits each filename into alternating text and numeric segments,
// then compares segment-by-segment. This means:
//   - "S01E02" < "S01E10"  (numeric chunks compared as integers, not strings)
//   - "Episode 9" < "Episode 10"
//   - "file_a" < "file_b"
//
// Common filename separators (dot, dash, space) are normalised to underscore
// before comparison so that "S01E04.1080p" and "S01E04_1080p" sort identically.
//
// This function is used only when building new bundles; existing DB records
// are never re-ordered, preserving full backwards compatibility.
func sortBundleMessages(msgs []BundleMessage) {
	sort.SliceStable(msgs, func(i, j int) bool {
		return naturalLess(msgs[i].FileName, msgs[j].FileName)
	})
}

// normalizeSeps replaces dots, dashes and spaces with underscores so that
// naming styles like "Show.S01E01" and "Show_S01E01" compare identically.
func normalizeSeps(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '.' || r == '-' || r == ' ' {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// naturalLess returns true when a should sort before b using natural order.
func naturalLess(a, b string) bool {
	// Lowercase and normalise separators so that dots, dashes and underscores
	// all compare identically — e.g. "S01E04.1080p" == "S01E04_1080p".
	la := normalizeSeps(strings.ToLower(a))
	lb := normalizeSeps(strings.ToLower(b))

	for {
		if lb == "" {
			return false
		}
		if la == "" {
			return true
		}

		aIsDigit := unicode.IsDigit(rune(la[0]))
		bIsDigit := unicode.IsDigit(rune(lb[0]))

		switch {
		case aIsDigit && bIsDigit:
			// Both start with a digit run — compare as integers.
			an, aRest := leadingInt(la)
			bn, bRest := leadingInt(lb)
			if an != bn {
				return an < bn
			}
			la, lb = aRest, bRest

		case aIsDigit:
			// Digits sort before non-digits.
			return true

		case bIsDigit:
			return false

		default:
			// Both start with a non-digit run — consume until the next digit or end.
			aChunk, aRest := leadingText(la)
			bChunk, bRest := leadingText(lb)
			if aChunk != bChunk {
				return aChunk < bChunk
			}
			la, lb = aRest, bRest
		}
	}
}

// leadingInt returns the integer value of the leading digit run and the remainder.
func leadingInt(s string) (int, string) {
	i := 0
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		i++
	}
	n := 0
	for _, ch := range s[:i] {
		n = n*10 + int(ch-'0')
	}
	return n, s[i:]
}

// leadingText returns the leading non-digit run and the remainder.
func leadingText(s string) (string, string) {
	i := 0
	for i < len(s) && !unicode.IsDigit(rune(s[i])) {
		i++
	}
	return s[:i], s[i:]
}