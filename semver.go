package main

import (
	"strconv"
	"strings"
)

// parseSemver splits "v3.2.1" or "3.2.1" into [3, 2, 1].
// Returns nil for anything that doesn't match v?D.D.D exactly.
//
// The bash CLI relied on `sort -t. -k1,1n -k2,2n -k3,3n` which is a *string*
// sort that only happens to work for single-digit components. We do real
// integer comparison instead — same result for our v3.x range, but
// future-proof for v10+ when GitHub eventually returns mixed-width tags.
func parseSemver(s string) []int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		out[i] = n
	}
	return out
}

// semverLess returns true if a < b. Either side can be "v"-prefixed or not.
// Returns false if either side is unparseable (caller should treat that as
// equal-or-greater rather than panicking).
func semverLess(a, b string) bool {
	pa := parseSemver(a)
	pb := parseSemver(b)
	if pa == nil || pb == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

// pickLatestSemverTag returns the highest tag matching v?D.D.D among the
// input slice. Empty string if none qualify. Used to resolve `--latest`
// (and the implicit "latest" when no pin or flag is set).
func pickLatestSemverTag(tags []string) string {
	var best string
	for _, t := range tags {
		if parseSemver(t) == nil {
			continue
		}
		if best == "" || semverLess(best, t) {
			best = t
		}
	}
	return best
}
