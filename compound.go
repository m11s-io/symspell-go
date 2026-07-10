package symspell

import (
	"math"
	"strings"
)

// LookupCompound finds suggested spellings for a multi-word input string,
// using the configured MaxDictionaryEditDistance. It supports compound-aware
// automatic spelling correction of three cases:
//
//  1. a mistakenly inserted space split a correct word into two incorrect terms
//  2. a mistakenly omitted space combined two correct words into one incorrect term
//  3. multiple independent input terms with/without spelling errors
func (s *SymSpell) LookupCompound(input string) []SuggestItem {
	return s.LookupCompoundMaxDistance(input, s.maxDictionaryEditDistance)
}

// LookupCompoundMaxDistance is LookupCompound with an explicit maximum edit
// distance between input and suggested words.
func (s *SymSpell) LookupCompoundMaxDistance(input string, editDistanceMax int) []SuggestItem {
	termList := ParseWords(input)

	var suggestionParts []SuggestItem
	lastCombi := false

	for i, term := range termList {
		suggestions := s.LookupMaxDistance(term, VerbosityTop, editDistanceMax, false)

		// Combination check, always before split.
		if i > 0 && !lastCombi {
			combined := termList[i-1] + term
			suggestionsCombi := s.LookupMaxDistance(combined, VerbosityTop, editDistanceMax, false)
			if len(suggestionsCombi) > 0 {
				best1 := suggestionParts[len(suggestionParts)-1]
				var best2 SuggestItem
				if len(suggestions) > 0 {
					best2 = suggestions[0]
				} else {
					// Unknown word: estimate word occurrence probability
					// P = 10 / (N * 10^word length).
					best2 = SuggestItem{
						Term:     term,
						Distance: editDistanceMax + 1,
						Count:    estimatedWordCount(term),
					}
				}

				// distance1 = edit distance between the 2 split terms and
				// their best corrections, as a comparative value for the
				// combination.
				distance1 := best1.Distance + best2.Distance
				if distance1 >= 0 &&
					(suggestionsCombi[0].Distance+1 < distance1 ||
						(suggestionsCombi[0].Distance+1 == distance1 &&
							float64(suggestionsCombi[0].Count) > float64(best1.Count)/float64(N)*float64(best2.Count))) {
					combi := suggestionsCombi[0]
					combi.Distance++
					suggestionParts[len(suggestionParts)-1] = combi
					lastCombi = true
					continue
				}
			}
		}
		lastCombi = false

		termRunes := []rune(term)

		// Always split terms without a suggestion / never split terms with
		// an exact-match suggestion / never split single-char terms.
		if len(suggestions) > 0 && (suggestions[0].Distance == 0 || len(termRunes) == 1) {
			suggestionParts = append(suggestionParts, suggestions[0])
			continue
		}

		// No perfect suggestion: split the term into pairs and pick the best split.
		var suggestionSplitBest *SuggestItem
		if len(suggestions) > 0 {
			best := suggestions[0]
			suggestionSplitBest = &best
		}

		if len(termRunes) > 1 {
			for j := 1; j < len(termRunes); j++ {
				part1 := string(termRunes[:j])
				part2 := string(termRunes[j:])

				suggestions1 := s.LookupMaxDistance(part1, VerbosityTop, editDistanceMax, false)
				if len(suggestions1) == 0 {
					continue
				}
				suggestions2 := s.LookupMaxDistance(part2, VerbosityTop, editDistanceMax, false)
				if len(suggestions2) == 0 {
					continue
				}

				suggestionSplit := SuggestItem{Term: suggestions1[0].Term + " " + suggestions2[0].Term}
				distance2 := DamerauLevenshteinDistance(term, suggestionSplit.Term, editDistanceMax)
				if distance2 < 0 {
					distance2 = editDistanceMax + 1
				}
				if suggestionSplitBest != nil {
					if distance2 > suggestionSplitBest.Distance {
						continue
					}
					if distance2 < suggestionSplitBest.Distance {
						suggestionSplitBest = nil
					}
				}
				suggestionSplit.Distance = distance2

				if bigramCount, ok := s.Bigrams[suggestionSplit.Term]; ok {
					suggestionSplit.Count = bigramCount
					// If the split terms are part of or identical to the
					// input, boost the count above the single-term
					// correction's count so the split wins ties.
					if len(suggestions) > 0 {
						if suggestions1[0].Term+suggestions2[0].Term == term {
							suggestionSplit.Count = max(suggestionSplit.Count, suggestions[0].Count+2)
						} else if suggestions1[0].Term == suggestions[0].Term || suggestions2[0].Term == suggestions[0].Term {
							suggestionSplit.Count = max(suggestionSplit.Count, suggestions[0].Count+1)
						}
					} else if suggestions1[0].Term+suggestions2[0].Term == term {
						suggestionSplit.Count = max(suggestionSplit.Count, max(suggestions1[0].Count, suggestions2[0].Count)+2)
					}
				} else {
					// No bigram found: estimate the combination's frequency
					// via the Naive Bayes assumption P(AB) = P(A) * P(B).
					suggestionSplit.Count = min(s.BigramCountMin, int64(float64(suggestions1[0].Count)/float64(N)*float64(suggestions2[0].Count)))
				}

				if suggestionSplitBest == nil || suggestionSplit.Count > suggestionSplitBest.Count {
					best := suggestionSplit
					suggestionSplitBest = &best
				}
			}

			if suggestionSplitBest != nil {
				suggestionParts = append(suggestionParts, *suggestionSplitBest)
			} else {
				suggestionParts = append(suggestionParts, SuggestItem{
					Term:     term,
					Count:    estimatedWordCount(term),
					Distance: editDistanceMax + 1,
				})
			}
		} else {
			suggestionParts = append(suggestionParts, SuggestItem{
				Term:     term,
				Count:    estimatedWordCount(term),
				Distance: editDistanceMax + 1,
			})
		}
	}

	count := float64(N)
	var sb strings.Builder
	for _, part := range suggestionParts {
		sb.WriteString(part.Term)
		sb.WriteByte(' ')
		count *= float64(part.Count) / float64(N)
	}

	resultTerm := strings.TrimRight(sb.String(), " ")
	resultDistance := DamerauLevenshteinDistance(input, resultTerm, math.MaxInt32)

	return []SuggestItem{{Term: resultTerm, Distance: resultDistance, Count: int64(count)}}
}

// estimatedWordCount estimates the occurrence probability of an unknown word
// as P = 10 / (N * 10^len(word)), matching upstream's heuristic for words
// with no dictionary suggestion.
func estimatedWordCount(word string) int64 {
	return int64(10 / math.Pow(10, float64(len([]rune(word)))))
}
