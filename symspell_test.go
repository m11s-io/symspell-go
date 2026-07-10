package symspell_test

import (
	"bufio"
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	symspell "github.com/m11s-io/symspell-go"
)

// Ported from upstream SymSpell.Test/SymSpell.Test.cs.

func TestWordsWithSharedPrefixShouldRetainCounts(t *testing.T) {
	s := symspell.New(symspell.WithInitialCapacity(16), symspell.WithMaxDictionaryEditDistance(1), symspell.WithPrefixLength(3))
	s.CreateDictionaryEntry("pipe", 5, nil)
	s.CreateDictionaryEntry("pips", 10, nil)

	result := s.LookupMaxDistance("pipe", symspell.VerbosityAll, 1, false)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Term != "pipe" || result[0].Count != 5 {
		t.Errorf("result[0] = %+v, want {pipe, _, 5}", result[0])
	}
	if result[1].Term != "pips" || result[1].Count != 10 {
		t.Errorf("result[1] = %+v, want {pips, _, 10}", result[1])
	}

	result = s.LookupMaxDistance("pips", symspell.VerbosityAll, 1, false)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Term != "pips" || result[0].Count != 10 {
		t.Errorf("result[0] = %+v, want {pips, _, 10}", result[0])
	}
	if result[1].Term != "pipe" || result[1].Count != 5 {
		t.Errorf("result[1] = %+v, want {pipe, _, 5}", result[1])
	}

	result = s.LookupMaxDistance("pip", symspell.VerbosityAll, 1, false)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Term != "pips" || result[0].Count != 10 {
		t.Errorf("result[0] = %+v, want {pips, _, 10}", result[0])
	}
	if result[1].Term != "pipe" || result[1].Count != 5 {
		t.Errorf("result[1] = %+v, want {pipe, _, 5}", result[1])
	}
}

func TestAddAdditionalCountsShouldNotAddWordAgain(t *testing.T) {
	s := symspell.New()
	s.CreateDictionaryEntry("hello", 11, nil)
	if s.WordCount() != 1 {
		t.Fatalf("WordCount() = %d, want 1", s.WordCount())
	}
	s.CreateDictionaryEntry("hello", 3, nil)
	if s.WordCount() != 1 {
		t.Fatalf("WordCount() = %d, want 1", s.WordCount())
	}
}

func TestAddAdditionalCountsShouldIncreaseCount(t *testing.T) {
	s := symspell.New()
	word := "hello"
	s.CreateDictionaryEntry(word, 11, nil)

	result := s.Lookup(word, symspell.VerbosityTop)
	var count int64
	if len(result) == 1 {
		count = result[0].Count
	}
	if count != 11 {
		t.Fatalf("count = %d, want 11", count)
	}

	s.CreateDictionaryEntry(word, 3, nil)
	result = s.Lookup(word, symspell.VerbosityTop)
	count = 0
	if len(result) == 1 {
		count = result[0].Count
	}
	if count != 14 {
		t.Fatalf("count = %d, want 14", count)
	}
}

func TestAddAdditionalCountsShouldNotOverflow(t *testing.T) {
	const maxInt64 = int64(1<<63 - 1)
	s := symspell.New()
	word := "hello"
	s.CreateDictionaryEntry(word, maxInt64-10, nil)

	result := s.Lookup(word, symspell.VerbosityTop)
	var count int64
	if len(result) == 1 {
		count = result[0].Count
	}
	if count != maxInt64-10 {
		t.Fatalf("count = %d, want %d", count, maxInt64-10)
	}

	s.CreateDictionaryEntry(word, 11, nil)
	result = s.Lookup(word, symspell.VerbosityTop)
	count = 0
	if len(result) == 1 {
		count = result[0].Count
	}
	if count != maxInt64 {
		t.Fatalf("count = %d, want %d", count, maxInt64)
	}
}

func TestVerbosityShouldControlLookupResults(t *testing.T) {
	s := symspell.New()
	s.CreateDictionaryEntry("steam", 1, nil)
	s.CreateDictionaryEntry("steams", 2, nil)
	s.CreateDictionaryEntry("steem", 3, nil)

	if got := len(s.LookupMaxDistance("steems", symspell.VerbosityTop, 2, false)); got != 1 {
		t.Errorf("VerbosityTop: len = %d, want 1", got)
	}
	if got := len(s.LookupMaxDistance("steems", symspell.VerbosityClosest, 2, false)); got != 2 {
		t.Errorf("VerbosityClosest: len = %d, want 2", got)
	}
	if got := len(s.LookupMaxDistance("steems", symspell.VerbosityAll, 2, false)); got != 3 {
		t.Errorf("VerbosityAll: len = %d, want 3", got)
	}
}

func TestLookupShouldReturnMostFrequent(t *testing.T) {
	s := symspell.New()
	s.CreateDictionaryEntry("steama", 4, nil)
	s.CreateDictionaryEntry("steamb", 6, nil)
	s.CreateDictionaryEntry("steamc", 2, nil)

	result := s.LookupMaxDistance("steam", symspell.VerbosityTop, 2, false)
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Term != "steamb" || result[0].Count != 6 {
		t.Errorf("result[0] = %+v, want {steamb, _, 6}", result[0])
	}
}

func TestLookupShouldFindExactMatch(t *testing.T) {
	s := symspell.New()
	s.CreateDictionaryEntry("steama", 4, nil)
	s.CreateDictionaryEntry("steamb", 6, nil)
	s.CreateDictionaryEntry("steamc", 2, nil)

	result := s.LookupMaxDistance("steama", symspell.VerbosityTop, 2, false)
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Term != "steama" {
		t.Errorf("result[0].Term = %q, want %q", result[0].Term, "steama")
	}
}

func TestLookupShouldNotReturnNonWordDelete(t *testing.T) {
	s := symspell.New(symspell.WithInitialCapacity(16), symspell.WithMaxDictionaryEditDistance(2), symspell.WithPrefixLength(7), symspell.WithCountThreshold(10))
	s.CreateDictionaryEntry("pawn", 10, nil)

	if got := len(s.LookupMaxDistance("paw", symspell.VerbosityTop, 0, false)); got != 0 {
		t.Errorf("len = %d, want 0", got)
	}
	if got := len(s.LookupMaxDistance("awn", symspell.VerbosityTop, 0, false)); got != 0 {
		t.Errorf("len = %d, want 0", got)
	}
}

func TestLookupShouldNotReturnLowCountWord(t *testing.T) {
	s := symspell.New(symspell.WithInitialCapacity(16), symspell.WithMaxDictionaryEditDistance(2), symspell.WithPrefixLength(7), symspell.WithCountThreshold(10))
	s.CreateDictionaryEntry("pawn", 1, nil)

	if got := len(s.LookupMaxDistance("pawn", symspell.VerbosityTop, 0, false)); got != 0 {
		t.Errorf("len = %d, want 0", got)
	}
}

func TestLookupShouldNotReturnLowCountWordThatsAlsoDeleteWord(t *testing.T) {
	s := symspell.New(symspell.WithInitialCapacity(16), symspell.WithMaxDictionaryEditDistance(2), symspell.WithPrefixLength(7), symspell.WithCountThreshold(10))
	s.CreateDictionaryEntry("flame", 20, nil)
	s.CreateDictionaryEntry("flam", 1, nil)

	if got := len(s.LookupMaxDistance("flam", symspell.VerbosityTop, 0, false)); got != 0 {
		t.Errorf("len = %d, want 0", got)
	}
}

// TestLookupShouldReplicateNoisyResults is a regression test against the
// exact result upstream's own test suite asserts (resultSum == 4955) when
// looking up 1000 noisy queries against the bundled 82,765-word English
// dictionary. Matching this exact reference value is strong evidence of
// behavioral parity with the original C# implementation.
func TestLookupShouldReplicateNoisyResults(t *testing.T) {
	const editDistanceMax = 2
	const prefixLength = 7
	const verbosity = symspell.VerbosityClosest

	s := symspell.New(symspell.WithInitialCapacity(83000), symspell.WithMaxDictionaryEditDistance(editDistanceMax), symspell.WithPrefixLength(prefixLength))

	dict, err := symspell.DefaultDictionary()
	if err != nil {
		t.Fatalf("DefaultDictionary() error = %v", err)
	}
	if err := s.LoadDictionary(dict, 0, 1); err != nil {
		t.Fatalf("LoadDictionary() error = %v", err)
	}

	testList := loadNoisyQueries(t)

	resultSum := 0
	for _, term := range testList {
		resultSum += len(s.LookupMaxDistance(term, verbosity, s.MaxDictionaryEditDistance(), false))
	}
	if resultSum != 4955 {
		t.Errorf("resultSum = %d, want 4955", resultSum)
	}
}

func TestLookupCompoundShouldMatchUpstreamReadmeExample(t *testing.T) {
	s := newDefaultSymSpell(t, true)

	input := "whereis th elove hehad dated forImuch of thepast who couqdn'tread in sixthgrade and ins pired him"
	result := s.LookupCompound(input)
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Term != "where is the love he had dated for much of the past who couldn't read in sixth grade and inspired him" {
		t.Fatalf("Term = %q", result[0].Term)
	}
	if result[0].Distance != 9 {
		t.Fatalf("Distance = %d, want 9", result[0].Distance)
	}
}

func TestWordSegmentationShouldMatchUpstreamReadmeExample(t *testing.T) {
	s := newDefaultSymSpell(t, false)

	result := s.WordSegmentationMaxDistance("thequickbrownfoxjumpsoverthelazydog", 0)
	if result.CorrectedString != "the quick brown fox jumps over the lazy dog" {
		t.Fatalf("CorrectedString = %q", result.CorrectedString)
	}
	if result.SegmentedString != "the quick brown fox jumps over the lazy dog" {
		t.Fatalf("SegmentedString = %q", result.SegmentedString)
	}
}

func TestWordSegmentationShouldNormalizeLigaturesAndHandleEmptyInputs(t *testing.T) {
	s := newDefaultSymSpell(t, false)

	result := s.WordSegmentationMaxDistance("scientiﬁcfields", 0)
	if result.CorrectedString != "scientific fields" {
		t.Fatalf("CorrectedString = %q, want %q", result.CorrectedString, "scientific fields")
	}

	if got := s.WordSegmentation(""); got != (symspell.SegmentationResult{}) {
		t.Fatalf("WordSegmentation(\"\") = %+v, want zero result", got)
	}

	empty := symspell.New()
	if got := empty.WordSegmentation("abc"); got.SegmentedString == "" || got.CorrectedString == "" {
		t.Fatalf("WordSegmentation with empty dictionary = %+v, want non-empty unknown-word result", got)
	}
}

func TestDamerauLevenshteinDistanceShouldUseOSA(t *testing.T) {
	tests := []struct {
		a, b        string
		maxDistance int
		want        int
	}{
		{a: "example", b: "example", maxDistance: 0, want: 0},
		{a: "ab", b: "ba", maxDistance: 1, want: 1},
		{a: "CA", b: "ABC", maxDistance: 3, want: 3},
		{a: "CA", b: "ABC", maxDistance: 2, want: -1},
		{a: "mañana", b: "manana", maxDistance: 1, want: 1},
	}

	for _, tt := range tests {
		if got := symspell.DamerauLevenshteinDistance(tt.a, tt.b, tt.maxDistance); got != tt.want {
			t.Errorf("DamerauLevenshteinDistance(%q, %q, %d) = %d, want %d", tt.a, tt.b, tt.maxDistance, got, tt.want)
		}
	}
}

func TestParseWordsShouldMatchUpstreamCharacterClassIntent(t *testing.T) {
	got := symspell.ParseWords("Can yu couldn't foo_bar 123 abc-DEF rock’n’roll")
	want := []string{"can", "yu", "couldn't", "foo", "bar", "123", "abc-def", "rock’n’roll"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseWords() = %#v, want %#v", got, want)
	}
}

func TestDictionaryLoadersShouldValidateIndexesAndSkipShortRows(t *testing.T) {
	s := symspell.New()
	if err := s.LoadDictionary(strings.NewReader("hello 10\nshort\nworld 7\n"), 0, 1); err != nil {
		t.Fatalf("LoadDictionary() error = %v", err)
	}
	if got := s.LookupMaxDistance("hello", symspell.VerbosityTop, 0, false); len(got) != 1 || got[0].Count != 10 {
		t.Fatalf("Lookup hello = %+v, want count 10", got)
	}

	if err := s.LoadDictionary(strings.NewReader("hello 10\n"), -1, 1); err == nil {
		t.Fatal("LoadDictionary() with negative termIndex returned nil error")
	}
	if err := s.LoadDictionary(strings.NewReader("hello 10\n"), 0, -1); err == nil {
		t.Fatal("LoadDictionary() with negative countIndex returned nil error")
	}

	bigrams := "hello world 12\nshort\nfoo bar 3\n"
	if err := s.LoadBigramDictionary(strings.NewReader(bigrams), 0, 2); err != nil {
		t.Fatalf("LoadBigramDictionary() error = %v", err)
	}
	if got := s.Bigrams["hello world"]; got != 12 {
		t.Fatalf("Bigrams[hello world] = %d, want 12", got)
	}

	tabSeparated := bytes.NewBufferString("good morning\t22\nbadrow\n")
	if err := s.LoadBigramDictionary(tabSeparated, 0, 1, '\t'); err != nil {
		t.Fatalf("LoadBigramDictionary(tab) error = %v", err)
	}
	if got := s.Bigrams["good morning"]; got != 22 {
		t.Fatalf("Bigrams[good morning] = %d, want 22", got)
	}
	if err := s.LoadBigramDictionary(strings.NewReader(bigrams), 0, -1); err == nil {
		t.Fatal("LoadBigramDictionary() with negative countIndex returned nil error")
	}
}

func TestNewShouldValidateCompactLevelLikeUpstream(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New(WithCompactLevel(17)) did not panic")
		}
	}()
	_ = symspell.New(symspell.WithCompactLevel(17))
}

func loadNoisyQueries(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("testdata/noisy_query_en_1000.txt")
	if err != nil {
		t.Fatalf("open testdata: %v", err)
	}
	defer f.Close()

	var terms []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			terms = append(terms, fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan testdata: %v", err)
	}
	return terms
}

func newDefaultSymSpell(t *testing.T, loadBigrams bool) *symspell.SymSpell {
	t.Helper()

	s := symspell.New(symspell.WithInitialCapacity(83000))
	dict, err := symspell.DefaultDictionary()
	if err != nil {
		t.Fatalf("DefaultDictionary() error = %v", err)
	}
	if err := s.LoadDictionary(dict, 0, 1); err != nil {
		t.Fatalf("LoadDictionary() error = %v", err)
	}
	if loadBigrams {
		bigrams, err := symspell.DefaultBigramDictionary()
		if err != nil {
			t.Fatalf("DefaultBigramDictionary() error = %v", err)
		}
		if err := s.LoadBigramDictionary(bigrams, 0, 2); err != nil {
			t.Fatalf("LoadBigramDictionary() error = %v", err)
		}
	}
	return s
}
