package symspell

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripBOM skips a leading UTF-8 byte-order mark, if present, matching the
// behavior of .NET's StreamReader (used by upstream to read dictionary
// files) which strips it automatically.
func stripBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	if bom, err := br.Peek(len(utf8BOM)); err == nil && bytes.Equal(bom, utf8BOM) {
		br.Discard(len(utf8BOM))
	}
	return br
}

// SuggestionStage temporarily stages dictionary data while adding many words
// at once, so it can be merged into the main deletes index with CommitStaged.
// This is a simplified version of upstream's SuggestionStage: the C#
// implementation uses a custom chunked array to avoid large-object-heap
// pressure during staging, an optimization that doesn't apply to Go's
// garbage collector, so this just accumulates into a plain map of slices.
type SuggestionStage struct {
	deletes map[uint32][]string
}

// NewSuggestionStage creates a SuggestionStage. Specifying an accurate
// initialCapacity is not essential, but can reduce reallocation as it grows.
func NewSuggestionStage(initialCapacity int) *SuggestionStage {
	return &SuggestionStage{deletes: make(map[uint32][]string, initialCapacity)}
}

// DeleteCount returns the number of unique delete candidates staged.
func (st *SuggestionStage) DeleteCount() int { return len(st.deletes) }

// Clear removes all staged data.
func (st *SuggestionStage) Clear() { st.deletes = make(map[uint32][]string) }

func (st *SuggestionStage) add(hash uint32, suggestion string) {
	st.deletes[hash] = append(st.deletes[hash], suggestion)
}

func (st *SuggestionStage) commitTo(permanent map[uint32][]string) {
	for hash, suggestions := range st.deletes {
		permanent[hash] = append(permanent[hash], suggestions...)
	}
}

// CommitStaged merges a SuggestionStage populated via CreateDictionaryEntry
// into the main dictionary index.
func (s *SymSpell) CommitStaged(staging *SuggestionStage) {
	if s.deletes == nil {
		s.deletes = make(map[uint32][]string, staging.DeleteCount())
	}
	staging.commitTo(s.deletes)
}

// CreateDictionaryEntry creates or updates a dictionary entry. For every new
// above-threshold word, delete-candidates with edit distance
// 1..maxDictionaryEditDistance are generated and indexed. The dictionary may
// be updated dynamically at any time.
//
// If staging is non-nil, new delete-candidates are added to it instead of
// directly to the dictionary, to speed up bulk loading; call CommitStaged
// afterward to merge them in.
//
// Returns true if the word was added as a new correctly spelled word, or
// false if it was added as a below-threshold word, or updated an existing
// correctly spelled word.
func (s *SymSpell) CreateDictionaryEntry(key string, count int64, staging *SuggestionStage) bool {
	if count <= 0 {
		if s.countThreshold > 0 {
			return false
		}
		count = 0
	}

	reachedThreshold := false
	if s.countThreshold > 1 {
		if countPrevious, found := s.belowThresholdWords[key]; found {
			count = saturatingAddInt64(countPrevious, count)
			if count >= s.countThreshold {
				delete(s.belowThresholdWords, key)
				reachedThreshold = true
			} else {
				s.belowThresholdWords[key] = count
				return false
			}
		}
	}
	if !reachedThreshold {
		if countPrevious, found := s.words[key]; found {
			s.words[key] = saturatingAddInt64(countPrevious, count)
			return false
		}
		if count < s.countThreshold {
			s.belowThresholdWords[key] = count
			return false
		}
	}

	// What we have at this point is a new, above-threshold word.
	s.words[key] = count

	// Deletes/suggestions are created only once, no matter how often the
	// word occurs, and only as soon as the word occurs in the corpus (even
	// if it existed before as a delete-derivative of another word).
	runeLen := len([]rune(key))
	if runeLen > s.maxDictionaryWordLength {
		s.maxDictionaryWordLength = runeLen
	}
	if s.deletes == nil {
		s.deletes = make(map[uint32][]string, s.initialCapacity)
	}

	edits := s.editsPrefix(key)
	if staging != nil {
		for deleteCandidate := range edits {
			staging.add(s.stringHash(deleteCandidate), key)
		}
	} else {
		for deleteCandidate := range edits {
			h := s.stringHash(deleteCandidate)
			s.deletes[h] = append(s.deletes[h], key)
		}
	}
	return true
}

// PurgeBelowThresholdWords removes all below-threshold words from the
// dictionary, which can be used to reduce memory consumption after
// populating the dictionary from a corpus via CreateDictionary.
func (s *SymSpell) PurgeBelowThresholdWords() {
	s.belowThresholdWords = make(map[string]int64)
}

func splitLine(line string, separator rune, hasSeparator bool) []string {
	if !hasSeparator {
		return strings.Fields(line)
	}
	return strings.Split(line, string(separator))
}

// LoadDictionaryFile loads dictionary entries from a file of word/frequency
// count pairs, merging with any dictionary data already loaded. termIndex
// and countIndex are the 0-based column positions of the word and count. If
// separator is provided, fields are split on that rune; otherwise any
// whitespace is treated as a separator.
func (s *SymSpell) LoadDictionaryFile(path string, termIndex, countIndex int, separator ...rune) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return s.LoadDictionary(f, termIndex, countIndex, separator...)
}

// LoadDictionary loads dictionary entries from a stream of word/frequency
// count pairs, merging with any dictionary data already loaded.
func (s *SymSpell) LoadDictionary(r io.Reader, termIndex, countIndex int, separator ...rune) error {
	if err := validateColumnIndexes(termIndex, countIndex); err != nil {
		return err
	}
	sep, hasSeparator := firstRune(separator)
	staging := NewSuggestionStage(16384)
	scanner := bufio.NewScanner(stripBOM(r))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineParts := splitLine(scanner.Text(), sep, hasSeparator)
		if !hasIndexes(lineParts, termIndex, countIndex) {
			continue
		}
		key := lineParts[termIndex]
		count, err := strconv.ParseInt(lineParts[countIndex], 10, 64)
		if err != nil {
			continue
		}
		s.CreateDictionaryEntry(key, count, staging)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	s.CommitStaged(staging)
	return nil
}

// LoadBigramDictionaryFile loads bigram entries from a file of
// two-word-term/frequency-count pairs, merging with any bigram data already
// loaded.
func (s *SymSpell) LoadBigramDictionaryFile(path string, termIndex, countIndex int, separator ...rune) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return s.LoadBigramDictionary(f, termIndex, countIndex, separator...)
}

// LoadBigramDictionary loads bigram entries from a stream of
// two-word-term/frequency-count pairs, merging with any bigram data already
// loaded. With the default whitespace separator, the bigram key is formed by
// joining the two term columns at termIndex and termIndex+1 with a space;
// with an explicit separator, only the single column at termIndex is used as
// the key.
func (s *SymSpell) LoadBigramDictionary(r io.Reader, termIndex, countIndex int, separator ...rune) error {
	if err := validateColumnIndexes(termIndex, countIndex); err != nil {
		return err
	}
	sep, hasSeparator := firstRune(separator)
	scanner := bufio.NewScanner(stripBOM(r))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineParts := splitLine(scanner.Text(), sep, hasSeparator)
		if !hasSeparator && !hasIndexes(lineParts, termIndex, termIndex+1, countIndex) {
			continue
		}
		if hasSeparator && !hasIndexes(lineParts, termIndex, countIndex) {
			continue
		}
		key := lineParts[termIndex]
		if !hasSeparator {
			key = lineParts[termIndex] + " " + lineParts[termIndex+1]
		}
		count, err := strconv.ParseInt(lineParts[countIndex], 10, 64)
		if err != nil {
			continue
		}
		s.Bigrams[key] = count
		if count < s.BigramCountMin {
			s.BigramCountMin = count
		}
	}
	return scanner.Err()
}

// CreateDictionaryFile loads dictionary words from a file containing plain
// text, merging with any dictionary data already loaded.
func (s *SymSpell) CreateDictionaryFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return s.CreateDictionary(f)
}

// CreateDictionary loads dictionary words from a stream containing plain
// text, merging with any dictionary data already loaded. Words are extracted
// with ParseWords and each occurrence increments its frequency count by 1.
func (s *SymSpell) CreateDictionary(r io.Reader) error {
	staging := NewSuggestionStage(16384)
	scanner := bufio.NewScanner(stripBOM(r))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		for _, key := range ParseWords(scanner.Text()) {
			s.CreateDictionaryEntry(key, 1, staging)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	s.CommitStaged(staging)
	return nil
}

func firstRune(separator []rune) (rune, bool) {
	if len(separator) == 0 {
		return 0, false
	}
	if len(separator) > 1 {
		panic(fmt.Sprintf("symspell: at most one separator rune is supported, got %d", len(separator)))
	}
	return separator[0], true
}

func validateColumnIndexes(indexes ...int) error {
	for _, index := range indexes {
		if index < 0 {
			return errors.New("symspell: column indexes must be >= 0")
		}
	}
	return nil
}

func hasIndexes(parts []string, indexes ...int) bool {
	for _, index := range indexes {
		if index >= len(parts) {
			return false
		}
	}
	return true
}
