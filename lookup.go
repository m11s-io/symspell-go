package symspell

import (
	"sort"
	"strings"
)

// Lookup finds suggested spellings for a single input word, using the
// configured MaxDictionaryEditDistance.
func (s *SymSpell) Lookup(input string, verbosity Verbosity) []SuggestItem {
	return s.LookupMaxDistance(input, verbosity, s.maxDictionaryEditDistance, false)
}

// LookupMaxDistance finds suggested spellings for a single input word.
//
//   - verbosity controls the quantity/closeness of the returned suggestions.
//   - maxEditDistance is the maximum edit distance between input and
//     suggested words; it must not exceed MaxDictionaryEditDistance.
//   - includeUnknown, if true, includes the input word itself in the
//     suggestions when no word within maxEditDistance is found.
//
// The result is sorted by ascending edit distance, then by descending
// frequency count.
func (s *SymSpell) LookupMaxDistance(input string, verbosity Verbosity, maxEditDistance int, includeUnknown bool) []SuggestItem {
	if maxEditDistance > s.maxDictionaryEditDistance {
		panic("symspell: maxEditDistance can't exceed MaxDictionaryEditDistance")
	}

	suggestions := make([]SuggestItem, 0)
	inputRunes := []rune(input)
	inputLen := len(inputRunes)

	finalize := func() []SuggestItem {
		if len(suggestions) > 1 {
			sort.Sort(suggestItems(suggestions))
		}
		if includeUnknown && len(suggestions) == 0 {
			suggestions = append(suggestions, SuggestItem{Term: input, Distance: maxEditDistance + 1, Count: 0})
		}
		return suggestions
	}

	// Early exit: word is too big to possibly match any dictionary word.
	if inputLen-maxEditDistance > s.maxDictionaryWordLength {
		return finalize()
	}

	// Quick look for an exact match.
	if count, ok := s.words[input]; ok {
		suggestions = append(suggestions, SuggestItem{Term: input, Distance: 0, Count: count})
		if verbosity != VerbosityAll {
			return finalize()
		}
	}

	// Early termination if we only wanted to check dictionary membership.
	if maxEditDistance == 0 {
		return finalize()
	}

	hashset1 := make(map[string]struct{}) // deletes we've considered already
	hashset2 := make(map[string]struct{}) // suggestions we've considered already
	hashset2[input] = struct{}{}

	maxEditDistance2 := maxEditDistance
	candidates := make([]string, 0, 4)

	inputPrefixLen := inputLen
	if inputPrefixLen > s.prefixLength {
		inputPrefixLen = s.prefixLength
		candidates = append(candidates, string(inputRunes[:inputPrefixLen]))
	} else {
		candidates = append(candidates, input)
	}

	candidatePointer := 0
	for candidatePointer < len(candidates) {
		candidate := candidates[candidatePointer]
		candidatePointer++
		candidateRunes := []rune(candidate)
		candidateLen := len(candidateRunes)
		lengthDiff := inputPrefixLen - candidateLen

		// Save time: if candidate distance already exceeds the best
		// suggestion distance, no better suggestions can be expected.
		if lengthDiff > maxEditDistance2 {
			if verbosity == VerbosityAll {
				continue
			}
			break
		}

		if dictSuggestions, ok := s.deletes[s.stringHash(candidate)]; ok {
			for _, suggestion := range dictSuggestions {
				if suggestion == input {
					continue
				}
				suggestionRunes := []rune(suggestion)
				suggestionLen := len(suggestionRunes)
				if absInt(suggestionLen-inputLen) > maxEditDistance2 ||
					suggestionLen < candidateLen ||
					(suggestionLen == candidateLen && suggestion != candidate) {
					continue
				}
				suggPrefixLen := min(suggestionLen, s.prefixLength)
				if suggPrefixLen > inputPrefixLen && suggPrefixLen-candidateLen > maxEditDistance2 {
					continue
				}

				// True Damerau-Levenshtein edit distance: adjust distance if
				// both sides have simultaneous edits, since inserts/deletes
				// on both sides can exceed maxEditDistance even though the
				// prefix delete-distance stayed within bounds.
				var distance int
				switch {
				case candidateLen == 0:
					distance = max(inputLen, suggestionLen)
					if distance > maxEditDistance2 {
						continue
					}
					if _, exists := hashset2[suggestion]; exists {
						continue
					}
					hashset2[suggestion] = struct{}{}
				case suggestionLen == 1:
					if strings.ContainsRune(input, suggestionRunes[0]) {
						distance = inputLen - 1
					} else {
						distance = inputLen
					}
					if distance > maxEditDistance2 {
						continue
					}
					if _, exists := hashset2[suggestion]; exists {
						continue
					}
					hashset2[suggestion] = struct{}{}
				default:
					// If the number of edits in the prefix equals
					// maxEditDistance and the suffix isn't identical, the
					// edit distance necessarily exceeds maxEditDistance and
					// no Levenshtein calculation is needed.
					skip := false
					if s.prefixLength-maxEditDistance == candidateLen {
						minLen := min(inputLen, suggestionLen) - s.prefixLength
						cond1 := minLen > 1 &&
							string(inputRunes[inputLen+1-minLen:]) != string(suggestionRunes[suggestionLen+1-minLen:])
						cond2 := minLen > 0 &&
							inputRunes[inputLen-minLen] != suggestionRunes[suggestionLen-minLen] &&
							(inputRunes[inputLen-minLen-1] != suggestionRunes[suggestionLen-minLen] ||
								inputRunes[inputLen-minLen] != suggestionRunes[suggestionLen-minLen-1])
						skip = cond1 || cond2
					}
					if skip {
						continue
					}

					// DeleteInSuggestionPrefix is somewhat expensive, and
					// only pays off when verbosity is Top or Closest.
					if verbosity != VerbosityAll && !s.deleteInSuggestionPrefix(candidate, suggestion) {
						continue
					}
					if _, exists := hashset2[suggestion]; exists {
						continue
					}
					hashset2[suggestion] = struct{}{}
					distance = DamerauLevenshteinDistance(input, suggestion, maxEditDistance2)
					if distance < 0 {
						continue
					}
				}

				// Save time: don't process higher distances than those
				// already found, if verbosity < All.
				if distance <= maxEditDistance2 {
					suggestionCount := s.words[suggestion]
					si := SuggestItem{Term: suggestion, Distance: distance, Count: suggestionCount}
					if len(suggestions) > 0 {
						switch verbosity {
						case VerbosityClosest:
							// We will calculate distance only to the
							// smallest found distance so far.
							if distance < maxEditDistance2 {
								suggestions = suggestions[:0]
							}
						case VerbosityTop:
							if distance < maxEditDistance2 || suggestionCount > suggestions[0].Count {
								maxEditDistance2 = distance
								suggestions[0] = si
							}
							continue
						}
					}
					if verbosity != VerbosityAll {
						maxEditDistance2 = distance
					}
					suggestions = append(suggestions, si)
				}
			}
		}

		// Derive further deletes from candidate and add them to the
		// candidates list; this recurses until maxEditDistance is reached.
		if lengthDiff < maxEditDistance && candidateLen <= s.prefixLength {
			// Save time: don't create edits with edit distance smaller than
			// suggestions already found.
			if verbosity != VerbosityAll && lengthDiff >= maxEditDistance2 {
				continue
			}
			for i := 0; i < candidateLen; i++ {
				deleted := make([]rune, 0, candidateLen-1)
				deleted = append(deleted, candidateRunes[:i]...)
				deleted = append(deleted, candidateRunes[i+1:]...)
				deletedStr := string(deleted)
				if _, exists := hashset1[deletedStr]; !exists {
					hashset1[deletedStr] = struct{}{}
					candidates = append(candidates, deletedStr)
				}
			}
		}
	}

	return finalize()
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
