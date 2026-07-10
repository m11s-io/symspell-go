package symspell

import (
	"regexp"
	"strings"
)

// parseWordsRE matches a non-unique word list from sample text, language
// independent (works with non-Latin characters). It is a Go/RE2 equivalent
// of the upstream .NET regex `['’\w-[_]]+`, which uses .NET's character
// class subtraction syntax to mean "word characters (Unicode letters,
// digits, underscore), apostrophe, right single quote, or hyphen, minus
// underscore." RE2 has no class subtraction, so the classes are spelled out
// directly: Unicode letters, Unicode digits, apostrophe, right single quote,
// and hyphen.
var parseWordsRE = regexp.MustCompile(`[\p{L}\p{N}'’-]+`)

// ParseWords splits text into a non-unique list of words, lowercased,
// suitable for building a dictionary from a text corpus via CreateDictionary.
func ParseWords(text string) []string {
	return parseWordsRE.FindAllString(strings.ToLower(text), -1)
}
