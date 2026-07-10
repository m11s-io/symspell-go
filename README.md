# symspell-go

Go port of [SymSpell](https://github.com/wolfgarbe/SymSpell) — **1 million times faster** spelling correction and fuzzy search through the Symmetric Delete algorithm.

The Symmetric Delete algorithm reduces the complexity of edit-candidate generation and dictionary lookup for a given Damerau-Levenshtein distance. It is six orders of magnitude faster than the standard approach (deletes + transposes + replaces + inserts) and language independent: only deletes are required, both at dictionary precalculation and at lookup time.

This port covers upstream's public API: `Lookup` (single-word correction), `LookupCompound` (multi-word correction with split/merge), and `WordSegmentation` (inserting missing spaces, with correction). It bundles the upstream 82,765-word English frequency dictionary and 243,342-entry bigram dictionary via `go:embed`, so a working spell checker needs no external data files.

## Install

```sh
go get github.com/m11s-io/symspell-go
```

## Usage

```go
package main

import (
	"fmt"

	symspell "github.com/m11s-io/symspell-go"
)

func main() {
	s := symspell.New()

	dict, err := symspell.DefaultDictionary()
	if err != nil {
		panic(err)
	}
	if err := s.LoadDictionary(dict, 0, 1); err != nil {
		panic(err)
	}

	// Single-word correction.
	for _, suggestion := range s.Lookup("teh", symspell.VerbosityTop) {
		fmt.Println(suggestion.Term, suggestion.Distance, suggestion.Count)
	}

	// Multi-word correction, including split/merge (needs the bigram dictionary).
	bigrams, _ := symspell.DefaultBigramDictionary()
	s.LoadBigramDictionary(bigrams, 0, 2)
	for _, suggestion := range s.LookupCompound("whereis th elove hehad dated forImuch of thepast") {
		fmt.Println(suggestion.Term)
	}

	// Word segmentation: insert missing spaces.
	result := s.WordSegmentation("thequickbrownfoxjumpsoverthelazydog")
	fmt.Println(result.CorrectedString)
}
```

Load your own dictionary instead of (or merged with) the bundled one via `LoadDictionary`/`LoadDictionaryFile`, or build one from a text corpus with `CreateDictionary`/`CreateDictionaryFile`.

## Verbosity

`Lookup` and `LookupMaxDistance` take a `Verbosity`, controlling the number of returned suggestions:

- `VerbosityTop` — the single suggestion with the highest frequency among those of smallest edit distance.
- `VerbosityClosest` — all suggestions of smallest edit distance found, ordered by frequency.
- `VerbosityAll` — all suggestions within the max edit distance, ordered by edit distance then frequency.

## Differences from upstream

This is a faithful port, verified against upstream's own test suite (see `symspell_test.go`, including an exact-match regression test against 1000 noisy queries from upstream's benchmark data). A few deliberate adaptations were made for Go:

- **Options instead of positional optional parameters**: `New` takes functional options (`WithMaxDictionaryEditDistance`, `WithPrefixLength`, etc.) instead of C#'s default parameter values.
- **`error` instead of `bool`**: file-loading functions return an `error` rather than upstream's `bool`.
- **`float64` instead of `decimal`**: `WordSegmentation`'s `ProbabilityLogSum` is a `float64`. The value is only ever used for relative ranking between candidate segmentations, so this loses no meaningful precision.
- **`[]rune` instead of UTF-16 `char`**: string indexing operates on Unicode code points rather than UTF-16 code units, for more predictable behavior with non-ASCII/non-BMP text.
- **Simplified `SuggestionStage`**: upstream uses a custom chunked array to reduce large-object-heap pressure in .NET during bulk dictionary loading — an optimization that doesn't apply to Go's garbage collector, so this port just accumulates into a plain map of slices.

## License

MIT — see [LICENSE](./LICENSE). Original algorithm and dictionaries © Wolf Garbe ([wolfgarbe/SymSpell](https://github.com/wolfgarbe/SymSpell)); Damerau-Levenshtein edit distance algorithm © SoftWx, Inc. ([softwx/SoftWx.Match](https://github.com/softwx/SoftWx.Match)).
