// Package symspell is a Go port of SymSpell, the Symmetric Delete spelling
// correction algorithm by Wolf Garbe (https://github.com/wolfgarbe/SymSpell).
//
// The Symmetric Delete algorithm reduces the complexity of edit candidate
// generation and dictionary lookup for a given Damerau-Levenshtein distance.
// It is six orders of magnitude faster than the standard approach (deletes +
// transposes + replaces + inserts) and language independent, because only
// deletes are required at both dictionary precalculation and lookup time.
package symspell

import "math"

// Verbosity controls the closeness/quantity of returned spelling suggestions.
type Verbosity int

const (
	// VerbosityTop returns the top suggestion with the highest term frequency
	// among the suggestions of smallest edit distance found.
	VerbosityTop Verbosity = iota
	// VerbosityClosest returns all suggestions of smallest edit distance found,
	// ordered by term frequency.
	VerbosityClosest
	// VerbosityAll returns all suggestions within maxEditDistance, ordered by
	// edit distance, then by term frequency (slower, no early termination).
	VerbosityAll
)

const (
	defaultMaxEditDistance = 2
	defaultPrefixLength    = 7
	defaultCountThreshold  = 1
	defaultInitialCapacity = 16
	defaultCompactLevel    = 5
)

// N is the number of words in the corpus used to generate the default
// frequency dictionary. It is used to estimate word occurrence probability
// p = c/N from word counts c during word segmentation and compound lookup.
const N int64 = 1024908267229

// SymSpell holds a spelling-correction dictionary and its precomputed delete
// candidates. Create one with New, populate it with LoadDictionary or
// CreateDictionaryEntry, then query it with Lookup, LookupCompound, or
// WordSegmentation.
type SymSpell struct {
	initialCapacity           int
	maxDictionaryEditDistance int
	prefixLength              int
	countThreshold            int64
	compactMask               uint32
	maxDictionaryWordLength   int

	// deletes maps a hash of a delete-candidate string to the dictionary
	// words it was derived from. Hash collisions are tolerated because
	// suggestions are ultimately verified via edit distance.
	deletes map[uint32][]string
	// words maps every correctly spelled dictionary word to its frequency count.
	words map[string]int64
	// belowThresholdWords holds words that haven't yet reached countThreshold.
	belowThresholdWords map[string]int64

	// Bigrams maps two-word phrases to their frequency count, used by
	// LookupCompound to score word splits/merges against known bigrams.
	Bigrams map[string]int64
	// BigramCountMin is the minimum count across all loaded bigrams.
	BigramCountMin int64
}

// Option configures a SymSpell instance created by New.
type Option func(*symSpellConfig)

type symSpellConfig struct {
	initialCapacity           int
	maxDictionaryEditDistance int
	prefixLength              int
	countThreshold            int64
	compactLevel              byte
}

// WithInitialCapacity sets the expected number of words in the dictionary.
// Specifying an accurate value is not essential, but it can help speed up
// processing by alleviating the need for data restructuring as the
// dictionary grows.
func WithInitialCapacity(n int) Option {
	return func(c *symSpellConfig) { c.initialCapacity = n }
}

// WithMaxDictionaryEditDistance sets the maximum edit distance for dictionary
// precalculation.
func WithMaxDictionaryEditDistance(n int) Option {
	return func(c *symSpellConfig) { c.maxDictionaryEditDistance = n }
}

// WithPrefixLength sets the length of word prefixes used for spell checking.
func WithPrefixLength(n int) Option {
	return func(c *symSpellConfig) { c.prefixLength = n }
}

// WithCountThreshold sets the minimum frequency count for a dictionary word
// to be considered a correct spelling.
func WithCountThreshold(n int64) Option {
	return func(c *symSpellConfig) { c.countThreshold = n }
}

// WithCompactLevel sets the degree of favoring lower memory use over speed
// (0 = fastest/most memory, 16 = slowest/least memory).
func WithCompactLevel(n byte) Option {
	return func(c *symSpellConfig) { c.compactLevel = n }
}

// New creates a SymSpell instance with the given options applied over the
// defaults (maxDictionaryEditDistance=2, prefixLength=7, countThreshold=1,
// compactLevel=5). It panics if the resulting configuration is invalid
// (mirrors the C# constructor's ArgumentOutOfRangeException checks).
func New(opts ...Option) *SymSpell {
	cfg := symSpellConfig{
		initialCapacity:           defaultInitialCapacity,
		maxDictionaryEditDistance: defaultMaxEditDistance,
		prefixLength:              defaultPrefixLength,
		countThreshold:            defaultCountThreshold,
		compactLevel:              defaultCompactLevel,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.initialCapacity < 0 {
		panic("symspell: initialCapacity must be >= 0")
	}
	if cfg.maxDictionaryEditDistance < 0 {
		panic("symspell: maxDictionaryEditDistance must be >= 0")
	}
	if cfg.prefixLength < 1 || cfg.prefixLength <= cfg.maxDictionaryEditDistance {
		panic("symspell: prefixLength must be >= 1 and > maxDictionaryEditDistance")
	}
	if cfg.countThreshold < 0 {
		panic("symspell: countThreshold must be >= 0")
	}
	if cfg.compactLevel > 16 {
		panic("symspell: compactLevel must be <= 16")
	}

	return &SymSpell{
		initialCapacity:           cfg.initialCapacity,
		maxDictionaryEditDistance: cfg.maxDictionaryEditDistance,
		prefixLength:              cfg.prefixLength,
		countThreshold:            cfg.countThreshold,
		compactMask:               (^uint32(0) >> (3 + cfg.compactLevel)) << 2,
		words:                     make(map[string]int64, cfg.initialCapacity),
		belowThresholdWords:       make(map[string]int64),
		Bigrams:                   make(map[string]int64),
		BigramCountMin:            math.MaxInt64,
	}
}

// MaxDictionaryEditDistance returns the maximum edit distance for dictionary
// precalculation.
func (s *SymSpell) MaxDictionaryEditDistance() int { return s.maxDictionaryEditDistance }

// PrefixLength returns the length of prefix from which deletes are generated.
func (s *SymSpell) PrefixLength() int { return s.prefixLength }

// MaxLength returns the length of the longest word in the dictionary.
func (s *SymSpell) MaxLength() int { return s.maxDictionaryWordLength }

// CountThreshold returns the count threshold for a word to be considered a
// valid word for spelling correction.
func (s *SymSpell) CountThreshold() int64 { return s.countThreshold }

// WordCount returns the number of unique words in the dictionary.
func (s *SymSpell) WordCount() int { return len(s.words) }

// EntryCount returns the number of word prefixes and intermediate word
// deletes encoded in the dictionary.
func (s *SymSpell) EntryCount() int { return len(s.deletes) }

// SuggestItem is a spelling suggestion returned from Lookup, LookupCompound,
// or WordSegmentation.
type SuggestItem struct {
	// Term is the suggested correctly spelled word.
	Term string
	// Distance is the edit distance between the searched-for word and the suggestion.
	Distance int
	// Count is the frequency of the suggestion in the dictionary (a measure
	// of how common the word is).
	Count int64
}

// suggestItems implements sort.Interface, ordering by ascending edit
// distance and then by descending frequency count, matching SuggestItem's
// C# IComparable<SuggestItem>.CompareTo.
type suggestItems []SuggestItem

func (s suggestItems) Len() int      { return len(s) }
func (s suggestItems) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s suggestItems) Less(i, j int) bool {
	if s[i].Distance == s[j].Distance {
		return s[i].Count > s[j].Count
	}
	return s[i].Distance < s[j].Distance
}
