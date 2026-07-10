package symspell

import (
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SegmentationResult is the result of WordSegmentation.
type SegmentationResult struct {
	// SegmentedString is the input with missing spaces inserted, spelling
	// left uncorrected.
	SegmentedString string
	// CorrectedString is SegmentedString with each word spelling-corrected.
	CorrectedString string
	// DistanceSum is the total edit distance between the input string and
	// CorrectedString.
	DistanceSum int
	// ProbabilityLogSum is the sum of the log10 word-occurrence
	// probabilities of the segmentation (a measure of how common and
	// probable the segmentation is; higher/less-negative is more probable).
	//
	// Upstream represents this as a C# decimal; this port uses float64,
	// which is adequate since the value is only ever used for relative
	// ranking between candidate segmentations, not displayed as an exact
	// figure.
	ProbabilityLogSum float64
}

// WordSegmentation finds the most likely position of missing spaces in
// input, using the configured MaxDictionaryEditDistance and MaxLength.
// Misspelled words are corrected without affecting segmentation; existing
// spaces are allowed and considered for the optimum segmentation.
//
// It uses a linear-time O(n) dynamic programming approach (see
// https://seekstorm.com/blog/fast-word-segmentation-noisy-text/), rather
// than the naive 2^(n-1) approach of trying every possible composition.
func (s *SymSpell) WordSegmentation(input string) SegmentationResult {
	return s.WordSegmentationMax(input, s.maxDictionaryEditDistance, s.maxDictionaryWordLength)
}

// WordSegmentationMaxDistance is WordSegmentation with an explicit maximum
// edit distance between input and corrected words (0 = segmentation only,
// no spelling correction).
func (s *SymSpell) WordSegmentationMaxDistance(input string, maxEditDistance int) SegmentationResult {
	return s.WordSegmentationMax(input, maxEditDistance, s.maxDictionaryWordLength)
}

// WordSegmentationMax is WordSegmentation with explicit maxEditDistance and
// maxSegmentationWordLength (the maximum word length that should be
// considered).
func (s *SymSpell) WordSegmentationMax(input string, maxEditDistance, maxSegmentationWordLength int) SegmentationResult {
	// Normalize ligatures (e.g. "ﬁelds" -> "fields") and strip hyphens, same
	// as upstream v6.7's NFKC-normalize-then-strip-hyphen preprocessing.
	input = strings.ReplaceAll(norm.NFKC.String(input), "-", "")

	inputRunes := []rune(input)
	if len(inputRunes) == 0 {
		return SegmentationResult{}
	}
	if maxSegmentationWordLength <= 0 {
		maxSegmentationWordLength = len(inputRunes)
	}
	arraySize := min(maxSegmentationWordLength, len(inputRunes))
	compositions := make([]SegmentationResult, arraySize)
	circularIndex := -1

	// Outer loop (column): all possible part start positions.
	for j := 0; j < len(inputRunes); j++ {
		// Inner loop (row): all possible part lengths from the start
		// position; a part can't be longer than the longest dictionary word
		// (other than as a long unknown word).
		imax := min(len(inputRunes)-j, maxSegmentationWordLength)
		for i := 1; i <= imax; i++ {
			partRunes := append([]rune(nil), inputRunes[j:j+i]...)
			separatorLength := 0
			var topEd int
			var topProbabilityLog float64
			var topResult string

			if unicode.IsSpace(partRunes[0]) {
				// Remove the leading space for edit-distance calculation.
				partRunes = partRunes[1:]
			} else {
				// The separator didn't exist and would have to be inserted.
				separatorLength = 1
			}

			// Remove any remaining embedded spaces; each one counts as an edit.
			topEd = len(partRunes)
			stripped := partRunes[:0:0]
			for _, r := range partRunes {
				if r != ' ' {
					stripped = append(stripped, r)
				}
			}
			partRunes = stripped
			topEd -= len(partRunes)
			part := string(partRunes)

			results := s.LookupMaxDistance(strings.ToLower(part), VerbosityTop, maxEditDistance, false)
			if len(results) > 0 {
				topResult = results[0].Term
				// Preserve upper case of the first letter.
				if len(partRunes) > 0 && unicode.IsUpper(partRunes[0]) {
					r := []rune(topResult)
					r[0] = unicode.ToUpper(r[0])
					topResult = string(r)
				}
				topEd += results[0].Distance
				// Naive Bayes: the resulting probability of the word
				// combination is the product of the individual word
				// probabilities; sum the log10 of probabilities instead of
				// multiplying, to avoid floating point underflow.
				topProbabilityLog = math.Log10(float64(results[0].Count) / float64(N))
			} else {
				topResult = part
				topEd += len(partRunes)
				topProbabilityLog = math.Log10(10.0 / (float64(N) * math.Pow(10.0, float64(len(partRunes)))))
			}

			destinationIndex := (i + circularIndex) % arraySize

			switch {
			case j == 0:
				compositions[destinationIndex] = SegmentationResult{
					SegmentedString:   part,
					CorrectedString:   topResult,
					DistanceSum:       topEd,
					ProbabilityLogSum: topProbabilityLog,
				}
			case i == maxSegmentationWordLength ||
				((compositions[circularIndex].DistanceSum+topEd == compositions[destinationIndex].DistanceSum ||
					compositions[circularIndex].DistanceSum+separatorLength+topEd == compositions[destinationIndex].DistanceSum) &&
					compositions[destinationIndex].ProbabilityLogSum < compositions[circularIndex].ProbabilityLogSum+topProbabilityLog) ||
				(compositions[circularIndex].DistanceSum+separatorLength+topEd < compositions[destinationIndex].DistanceSum):

				topResultRunes := []rune(topResult)
				// Keep punctuation or an apostrophe adjacent to the previous word.
				if (len(topResultRunes) == 1 && unicode.IsPunct(topResultRunes[0])) ||
					(len(topResultRunes) == 2 && strings.HasPrefix(topResult, "’")) {
					compositions[destinationIndex] = SegmentationResult{
						SegmentedString:   compositions[circularIndex].SegmentedString + part,
						CorrectedString:   compositions[circularIndex].CorrectedString + topResult,
						DistanceSum:       compositions[circularIndex].DistanceSum + topEd,
						ProbabilityLogSum: compositions[circularIndex].ProbabilityLogSum + topProbabilityLog,
					}
				} else {
					compositions[destinationIndex] = SegmentationResult{
						SegmentedString:   compositions[circularIndex].SegmentedString + " " + part,
						CorrectedString:   compositions[circularIndex].CorrectedString + " " + topResult,
						DistanceSum:       compositions[circularIndex].DistanceSum + separatorLength + topEd,
						ProbabilityLogSum: compositions[circularIndex].ProbabilityLogSum + topProbabilityLog,
					}
				}
			}
		}
		circularIndex++
		if circularIndex == arraySize {
			circularIndex = 0
		}
	}
	return compositions[circularIndex]
}
