package symspell

// stringHash computes a compact hash of s, used as the key into the deletes
// map. Collisions are tolerated because suggestions are ultimately verified
// via edit distance. The low 2 bits encode min(len(s), 3) so that
// deleteInSuggestionPrefix and Lookup can cheaply reject same-hash entries
// of clearly incompatible length.
func (s *SymSpell) stringHash(str string) uint32 {
	runes := []rune(str)
	length := len(runes)
	lenMask := length
	if lenMask > 3 {
		lenMask = 3
	}

	hash := uint32(2166136261)
	for _, r := range runes {
		hash ^= uint32(r)
		hash *= 16777619
	}

	hash &= s.compactMask
	hash |= uint32(lenMask)
	return hash
}

// edits recursively generates delete-candidates of word, up to
// maxDictionaryEditDistance deletes, adding each to deleteWords.
func (s *SymSpell) edits(word string, editDistance int, deleteWords map[string]struct{}) {
	editDistance++
	runes := []rune(word)
	if len(runes) <= 1 {
		return
	}
	for i := range runes {
		deleted := make([]rune, 0, len(runes)-1)
		deleted = append(deleted, runes[:i]...)
		deleted = append(deleted, runes[i+1:]...)
		deletedStr := string(deleted)
		if _, exists := deleteWords[deletedStr]; !exists {
			deleteWords[deletedStr] = struct{}{}
			if editDistance < s.maxDictionaryEditDistance {
				s.edits(deletedStr, editDistance, deleteWords)
			}
		}
	}
}

// editsPrefix returns the set of delete-candidates derived from the prefix
// of key (truncated to prefixLength), used to populate the deletes index
// when adding a dictionary entry.
func (s *SymSpell) editsPrefix(key string) map[string]struct{} {
	runes := []rune(key)
	hashSet := make(map[string]struct{})
	if len(runes) <= s.maxDictionaryEditDistance {
		hashSet[""] = struct{}{}
	}
	if len(runes) > s.prefixLength {
		runes = runes[:s.prefixLength]
	}
	prefixKey := string(runes)
	hashSet[prefixKey] = struct{}{}
	s.edits(prefixKey, 0, hashSet)
	return hashSet
}

// deleteInSuggestionPrefix checks whether all delete characters are present,
// in order, within the suggestion's prefix — otherwise the hash match found
// in the deletes index was just a collision.
func (s *SymSpell) deleteInSuggestionPrefix(deleteCandidate string, suggestion string) bool {
	deleteRunes := []rune(deleteCandidate)
	suggestionRunes := []rune(suggestion)
	if len(deleteRunes) == 0 {
		return true
	}
	suggestionLen := len(suggestionRunes)
	if s.prefixLength < suggestionLen {
		suggestionLen = s.prefixLength
	}
	j := 0
	for i := 0; i < len(deleteRunes); i++ {
		delChar := deleteRunes[i]
		for j < suggestionLen && delChar != suggestionRunes[j] {
			j++
		}
		if j == suggestionLen {
			return false
		}
	}
	return true
}

// saturatingAddInt64 adds a and b, clamping to math.MaxInt64 on overflow,
// matching the C# `(Int64.MaxValue - countPrevious > count) ? countPrevious
// + count : Int64.MaxValue` pattern used when merging word frequency counts.
func saturatingAddInt64(a, b int64) int64 {
	const maxInt64 = int64(1<<63 - 1)
	if maxInt64-a > b {
		return a + b
	}
	return maxInt64
}
