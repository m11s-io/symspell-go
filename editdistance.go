package symspell

// This file is a Go port of the Damerau-Levenshtein optimal string alignment
// (OSA) distance algorithm from SoftWx.Match (Copyright ©2015-2018 SoftWx,
// Inc., MIT License, https://github.com/softwx/SoftWx.Match), as used by
// upstream SymSpell's DamerauOSA.cs. Unlike the original's UTF-16 char
// indexing, this operates on []rune for correct behavior with non-ASCII
// text.
//
// Note that OSA distance is not a true metric (it does not satisfy the
// triangle inequality): it only counts transpositions of adjacent
// characters as a single edit, differing from the classic
// Damerau-Levenshtein distance for transpositions across longer spans.

// DamerauLevenshteinDistance computes the Damerau-Levenshtein OSA edit
// distance between a and b. If maxDistance is > 0 and the true distance
// exceeds it, -1 is returned without computing the exact value.
func DamerauLevenshteinDistance(a, b string, maxDistance int) int {
	r1 := []rune(a)
	r2 := []rune(b)

	if maxDistance <= 0 {
		if a == b {
			return 0
		}
		return -1
	}

	// Ensure the shorter string is r1: spends more time in the inner loop
	// over the (now-longer) r2, which is a little faster.
	if len(r1) > len(r2) {
		r1, r2 = r2, r1
	}
	if len(r2)-len(r1) > maxDistance {
		return -1
	}

	len1, len2, start := prefixSuffixPrep(r1, r2)
	if len1 == 0 {
		if len2 <= maxDistance {
			return len2
		}
		return -1
	}

	char1Costs := make([]int, len2)
	prevChar1Costs := make([]int, len2)
	if maxDistance < len2 {
		return damerauOSADistanceBounded(r1, r2, len1, len2, start, maxDistance, char1Costs, prevChar1Costs)
	}
	return damerauOSADistance(r1, r2, len1, len2, start, char1Costs, prevChar1Costs)
}

// prefixSuffixPrep calculates the starting position and lengths of two rune
// slices such that common prefix and suffix runs are excluded. Expects
// len(r1) <= len(r2).
func prefixSuffixPrep(r1, r2 []rune) (len1, len2, start int) {
	len2 = len(r2)
	len1 = len(r1)
	for len1 != 0 && r1[len1-1] == r2[len2-1] {
		len1--
		len2--
	}
	start = 0
	for start != len1 && r1[start] == r2[start] {
		start++
	}
	if start != 0 {
		len2 -= start
		len1 -= start
	}
	return len1, len2, start
}

// damerauOSADistance computes the exact (unbounded) OSA distance.
func damerauOSADistance(s1, s2 []rune, len1, len2, start int, char1Costs, prevChar1Costs []int) int {
	for j := 0; j < len2; j++ {
		char1Costs[j] = j + 1
	}
	char1 := ' '
	currentCost := 0
	for i := 0; i < len1; i++ {
		prevChar1 := char1
		char1 = s1[start+i]
		char2 := ' '
		leftCharCost, aboveCharCost := i, i
		nextTransCost := 0
		for j := 0; j < len2; j++ {
			thisTransCost := nextTransCost
			nextTransCost = prevChar1Costs[j]
			currentCost = leftCharCost // cost of diagonal (substitution)
			prevChar1Costs[j] = currentCost
			leftCharCost = char1Costs[j] // left now equals current cost (diagonal at next iteration)
			prevChar2 := char2
			char2 = s2[start+j]
			if char1 != char2 {
				if aboveCharCost < currentCost {
					currentCost = aboveCharCost // deletion
				}
				if leftCharCost < currentCost {
					currentCost = leftCharCost // insertion
				}
				currentCost++
				if i != 0 && j != 0 && char1 == prevChar2 && prevChar1 == char2 && thisTransCost+1 < currentCost {
					currentCost = thisTransCost + 1 // transposition
				}
			}
			aboveCharCost = currentCost
			char1Costs[j] = currentCost
		}
	}
	return currentCost
}

// damerauOSADistanceBounded computes the OSA distance restricted to a band
// of width maxDistance around the diagonal, returning -1 as soon as it's
// certain the true distance exceeds maxDistance.
func damerauOSADistanceBounded(s1, s2 []rune, len1, len2, start, maxDistance int, char1Costs, prevChar1Costs []int) int {
	var i, j int
	for j = 0; j < maxDistance; j++ {
		char1Costs[j] = j + 1
	}
	for ; j < len2; j++ {
		char1Costs[j] = maxDistance + 1
	}
	lenDiff := len2 - len1
	jStartOffset := maxDistance - lenDiff
	jStart := 0
	jEnd := maxDistance
	char1 := ' '
	currentCost := 0
	for i = 0; i < len1; i++ {
		prevChar1 := char1
		char1 = s1[start+i]
		char2 := ' '
		leftCharCost, aboveCharCost := i, i
		nextTransCost := 0
		// No need to look beyond the window of the lower right diagonal
		// (i - lenDiff) minus maxDistance cells, and the upper left
		// diagonal (i) plus maxDistance cells.
		if i > jStartOffset {
			jStart++
		}
		if jEnd < len2 {
			jEnd++
		}
		for j = jStart; j < jEnd; j++ {
			thisTransCost := nextTransCost
			nextTransCost = prevChar1Costs[j]
			currentCost = leftCharCost
			prevChar1Costs[j] = currentCost
			leftCharCost = char1Costs[j]
			prevChar2 := char2
			char2 = s2[start+j]
			if char1 != char2 {
				if aboveCharCost < currentCost {
					currentCost = aboveCharCost
				}
				if leftCharCost < currentCost {
					currentCost = leftCharCost
				}
				currentCost++
				if i != 0 && j != 0 && char1 == prevChar2 && prevChar1 == char2 && thisTransCost+1 < currentCost {
					currentCost = thisTransCost + 1
				}
			}
			aboveCharCost = currentCost
			char1Costs[j] = currentCost
		}
		if char1Costs[i+lenDiff] > maxDistance {
			return -1
		}
	}
	if currentCost <= maxDistance {
		return currentCost
	}
	return -1
}
