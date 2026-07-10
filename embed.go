package symspell

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
)

//go:embed data/frequency_dictionary_en_82_765.txt.gz
var defaultDictionaryGz []byte

//go:embed data/frequency_bigramdictionary_en_243_342.txt.gz
var defaultBigramDictionaryGz []byte

// DefaultDictionary returns a reader over the bundled English unigram
// frequency dictionary (82,765 words, derived from Wiktionary and the Google
// Books Ngram corpus), suitable for LoadDictionary(r, 0, 1).
func DefaultDictionary() (io.Reader, error) {
	return gzip.NewReader(bytes.NewReader(defaultDictionaryGz))
}

// DefaultBigramDictionary returns a reader over the bundled English bigram
// frequency dictionary (243,342 word pairs), suitable for
// LoadBigramDictionary(r, 0, 2), used to improve LookupCompound's splitting
// decisions.
func DefaultBigramDictionary() (io.Reader, error) {
	return gzip.NewReader(bytes.NewReader(defaultBigramDictionaryGz))
}
