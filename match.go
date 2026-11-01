// Package redglob implements a simple pattern matcher with Unicode support.
// It provides a Go implementation of Redis's glob-style pattern matching.
package redglob // import "github.com/maolonglong/redglob"

import (
	"math/bits"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// Match checks whether the input string `str` matches the pattern `pattern`.
// This function uses a simple wildcard matching algorithm where '*' matches
// any number of characters and '?' matches any single character.
// The function returns true if `str` matches `pattern`, and false otherwise.
//
// The pattern syntax is as follows:
//
//	{ term }
//
// term:
//
//	'*'         matches any sequence of characters
//	'?'         matches any single character
//	c           matches character c (c != '*', '?', '\\')
//	'\\' c      matches character c
//	'[abc]'     matches 'a' or 'b' or 'c'
//	'[a-z]'     matches characters 'a' to 'z'
//	'[^abc]'    matches any character except 'a', 'b', or 'c'
//	'[^a-z]'    matches any character except 'a' to 'z'
func Match(str, pattern string) bool {
	if str == pattern && !strings.ContainsAny(pattern, `[\`) {
		return true
	}
	firstStar, firstStarEnd, lastStar, lastStarEnd, starRuns, stars, specialIndex := scanPattern(pattern)
	if specialIndex >= 0 {
		if invalidLongPattern(pattern[specialIndex:]) {
			return false
		}
		if starRuns == 0 {
			if !strings.HasPrefix(str, pattern[:specialIndex]) {
				return false
			}
			return stringmatchIter(str[specialIndex:], pattern[specialIndex:], false)
		}
		if !strings.HasPrefix(str, pattern[:firstStar]) {
			return false
		}
		return stringmatchIter(str[firstStar:], pattern[firstStar:], false)
	}
	if starRuns == 0 {
		return false
	}
	if starRuns == 1 {
		return matchSimple(str, pattern[:firstStar], pattern[firstStarEnd:])
	}
	return matchLiteralStarsCompiled(str, pattern[:firstStar], pattern[firstStarEnd:lastStar], pattern[lastStarEnd:], len(pattern)-stars, starRuns == 2)
}

// MatchFold is a case-insensitive version of Match. It uses Unicode simple case
// folding while preserving the matcher's one-pattern-rune-per-input-rune
// semantics.
func MatchFold(str, pattern string) bool {
	firstStar, firstStarEnd, lastStar, lastStarEnd, starRuns, _, specialIndex := scanPattern(pattern)
	if specialIndex >= 0 {
		if invalidLongPattern(pattern[specialIndex:]) {
			return false
		}
		if starRuns == 0 {
			prefixEnd, ok := matchPrefixFold(str, pattern[:specialIndex])
			if !ok {
				return false
			}
			return stringmatchIter(str[prefixEnd:], pattern[specialIndex:], true)
		}
		prefixEnd, ok := matchPrefixFold(str, pattern[:firstStar])
		if !ok {
			return false
		}
		return stringmatchIter(str[prefixEnd:], pattern[firstStar:], true)
	}
	if starRuns == 0 {
		return matchLiteralFold(str, pattern)
	}
	if starRuns == 1 {
		return matchSimpleFold(str, pattern[:firstStar], pattern[firstStarEnd:])
	}
	return matchLiteralStarsCompiledFold(str, pattern[:firstStar], pattern[firstStarEnd:lastStar], pattern[lastStarEnd:], starRuns == 2)
}

// MatchBytes is similar to Match, but it receives a byteslice instead of a string as input.
// This function converts the byte slice to a string and then calls the Match function.
func MatchBytes(b []byte, pattern string) bool {
	return Match(b2s(b), pattern)
}

// MatchBytesFold is a case-insensitive version of MatchBytes. It uses Unicode
// simple case folding.
func MatchBytesFold(b []byte, pattern string) bool {
	return MatchFold(b2s(b), pattern)
}

// stringmatchIter matches without allocations or recursive star expansion.
// The most recent star is the only checkpoint needed for a flat glob: after a
// mismatch it consumes one more input rune and retries the pattern after it.
func stringmatchIter(str, pattern string, nocase bool) bool {
	patternIndex, stringIndex := 0, 0
	starPattern, starString, starLiteralEnd := -1, 0, -1
	for stringIndex < len(str) {
		if patternIndex < len(pattern) {
			pc, patternSize := decodeRune(pattern[patternIndex:])
			if pc == '*' {
				for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
					patternIndex++
				}
				if patternIndex == len(pattern) {
					return true
				}
				starPattern, starString = patternIndex, stringIndex
				starLiteralEnd = starPattern + literalPrefixLen(pattern[starPattern:])
				if starLiteralEnd > starPattern {
					index, width := indexLiteral(str[stringIndex:], pattern[starPattern:starLiteralEnd], nocase)
					if index < 0 {
						return false
					}
					starString += index
					stringIndex = starString + width
					patternIndex = starLiteralEnd
				}
				continue
			}
			sc, stringSize := decodeRune(str[stringIndex:])
			switch pc {
			case '?':
				patternIndex++
				stringIndex += stringSize
				for patternIndex < len(pattern) && pattern[patternIndex] == '?' {
					if stringIndex >= len(str) {
						return false
					}
					if str[stringIndex] < utf8.RuneSelf {
						stringIndex++
					} else {
						_, size := utf8.DecodeRuneInString(str[stringIndex:])
						stringIndex += size
					}
					patternIndex++
				}
				continue
			case '[':
				consumed, matched, valid := matchPatternClass(pattern[patternIndex+patternSize:], sc, nocase)
				if !valid {
					return false
				}
				if matched {
					patternIndex += patternSize + consumed
					stringIndex += stringSize
					continue
				}
			case '\\':
				patternIndex += patternSize
				if patternIndex == len(pattern) {
					return false
				}
				pc, patternSize = decodeRune(pattern[patternIndex:])
				if runesMatch(sc, pc, nocase) {
					patternIndex += patternSize
					stringIndex += stringSize
					continue
				}
			default:
				if runesMatch(sc, pc, nocase) {
					patternIndex += patternSize
					stringIndex += stringSize
					continue
				}
			}
		}

		if starPattern < 0 || starString >= len(str) {
			return false
		}
		_, size := decodeRune(str[starString:])
		starString += size
		if starLiteralEnd > starPattern {
			index, width := indexLiteral(str[starString:], pattern[starPattern:starLiteralEnd], nocase)
			if index < 0 {
				return false
			}
			starString += index
			stringIndex = starString + width
			patternIndex = starLiteralEnd
			continue
		}
		stringIndex = starString
		patternIndex = starPattern
	}

	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

func literalPrefixLen(pattern string) int {
	length := 0
	for length+8 <= len(pattern) {
		mask := specialASCII8Mask(readUint64(pattern, length))
		if mask == 0 {
			length += 8
			continue
		}
		length += bits.TrailingZeros64(mask) >> 3
		if pattern[length] < utf8.RuneSelf {
			return length
		}
		break
	}
	for length < len(pattern) {
		char, size := decodeRune(pattern[length:])
		switch char {
		case utf8.RuneError, '*', '?', '[', '\\':
			return length
		}
		length += size
	}
	return length
}

func indexLiteral(str, literal string, fold bool) (int, int) {
	if fold {
		return indexFold(str, literal)
	}
	index := indexCaseSensitive(str, literal)
	if index < 0 {
		return -1, 0
	}
	return index, len(literal)
}

func invalidLongPattern(pattern string) bool {
	return len(pattern) >= 64 && !validPattern(pattern)
}

func validPattern(pattern string) bool {
	for len(pattern) > 0 {
		char, size := decodeRune(pattern)
		switch char {
		case '[':
			consumed, _, valid := matchPatternClass(pattern[size:], 0, false)
			if !valid {
				return false
			}
			pattern = pattern[size+consumed:]
			continue
		case '\\':
			pattern = pattern[size:]
			if len(pattern) == 0 {
				return false
			}
			_, size = decodeRune(pattern)
		}
		pattern = pattern[size:]
	}
	return true
}

// matchPatternClass parses pattern immediately after '[' and reports the
// bytes consumed through ']', whether char matched, and whether it was valid.
func matchPatternClass(pattern string, char rune, nocase bool) (int, bool, bool) {
	negated := len(pattern) > 0 && pattern[0] == '^'
	index := 0
	if negated {
		index++
	}
	matched := false
	for {
		if index >= len(pattern) {
			return 0, false, false
		}
		start, size := decodeRune(pattern[index:])
		if start == '\\' {
			index += size
			if index >= len(pattern) {
				return 0, false, false
			}
			start, size = decodeRune(pattern[index:])
			if runesMatch(char, start, nocase) {
				matched = true
			}
			index += size
			continue
		}
		if start == ']' {
			return index + size, matched != negated, true
		}
		if len(pattern[index:]) > size+1 && pattern[index+size] == '-' {
			endIndex := index + size + 1
			end, endSize := decodeRune(pattern[endIndex:])
			if start > end {
				start, end = end, start
			}
			if runeMatchesRange(char, start, end, nocase) {
				matched = true
			}
			index = endIndex + endSize
			continue
		}
		if runesMatch(char, start, nocase) {
			matched = true
		}
		index += size
	}
}

func scanPattern(pattern string) (firstStar, firstStarEnd, lastStar, lastStarEnd, starRuns, stars, specialIndex int) {
	firstStar, firstStarEnd = -1, -1
	lastStar, lastStarEnd = -1, -1
	index := 0
	for index < len(pattern) {
		for index+8 <= len(pattern) {
			mask := specialASCII8Mask(readUint64(pattern, index))
			if mask == 0 {
				index += 8
				continue
			}
			index += bits.TrailingZeros64(mask) >> 3
			break
		}
		if index >= len(pattern) {
			break
		}
		b := pattern[index]
		if b >= utf8.RuneSelf {
			char, size := utf8.DecodeRuneInString(pattern[index:])
			if char == utf8.RuneError {
				return firstStar, firstStarEnd, lastStar, lastStarEnd, starRuns, stars, index
			}
			index += size
			continue
		}
		switch b {
		case '?', '[', '\\':
			return firstStar, firstStarEnd, lastStar, lastStarEnd, starRuns, stars, index
		case '*':
			stars++
			if firstStar < 0 {
				firstStar = index
				firstStarEnd = index + 1
				lastStar = index
				lastStarEnd = index + 1
				starRuns = 1
			} else if index == lastStarEnd {
				if starRuns == 1 {
					firstStarEnd = index + 1
				}
				lastStarEnd = index + 1
			} else {
				lastStar = index
				lastStarEnd = index + 1
				starRuns++
			}
			index++
		default:
			index++
		}
	}
	return firstStar, firstStarEnd, lastStar, lastStarEnd, starRuns, stars, -1
}

func matchSimple(str, prefix, suffix string) bool {
	if len(prefix) == 0 {
		return strings.HasSuffix(str, suffix)
	}
	if len(suffix) == 0 {
		return strings.HasPrefix(str, prefix)
	}
	return len(str) >= len(prefix)+len(suffix) &&
		strings.HasPrefix(str, prefix) && strings.HasSuffix(str, suffix)
}

func matchSimpleFold(str, prefix, suffix string) bool {
	prefixEnd := 0
	if len(prefix) > 0 {
		var matches bool
		prefixEnd, matches = matchPrefixFold(str, prefix)
		if !matches {
			return false
		}
	}
	suffixStart := len(str)
	if len(suffix) > 0 {
		var matches bool
		suffixStart, matches = matchSuffixFold(str, suffix)
		if !matches {
			return false
		}
	}
	return prefixEnd <= suffixStart
}

func matchLiteralStarsCompiled(str, prefix, middle, suffix string, minLen int, singleMiddle bool) bool {
	if len(str) < minLen || !strings.HasPrefix(str, prefix) || !strings.HasSuffix(str, suffix) {
		return false
	}
	if len(middle) == 0 {
		return true
	}
	str = str[len(prefix) : len(str)-len(suffix)]
	if singleMiddle {
		return indexCaseSensitive(str, middle) >= 0
	}
	return matchMiddleLiterals(str, middle)
}

func matchMiddleLiterals(str, middle string) bool {
	for len(middle) > 0 {
		if middle[0] == '*' {
			middle = middle[1:]
			continue
		}
		star := strings.IndexByte(middle, '*')
		if star < 0 {
			return indexCaseSensitive(str, middle) >= 0
		}
		literal := middle[:star]
		index := indexCaseSensitive(str, literal)
		if index < 0 {
			return false
		}
		str = str[index+len(literal):]
		middle = middle[star+1:]
	}
	return true
}

func indexCaseSensitive(str, literal string) int {
	if len(literal) >= 64 {
		return indexLongLiteral(str, literal)
	}
	return strings.Index(str, literal)
}

func indexLongLiteral(str, literal string) int {
	n, m := len(str), len(literal)
	if m > n {
		return -1
	}
	anchor := m - 1
	anchorByte := literal[anchor]
	guard := 0
	guardByte := literal[0]
	if guardByte == anchorByte {
		for i := m - 2; i > 0; i-- {
			if literal[i] != anchorByte {
				guard = i
				guardByte = literal[i]
				break
			}
		}
	}
	if guardByte != anchorByte && str[anchor] == anchorByte {
		anchor, guard = guard, anchor
		anchorByte, guardByte = guardByte, anchorByte
	}
	limit := n - m
	fails := 0
	for start := 0; start <= limit; {
		rel := strings.IndexByte(str[start+anchor:limit+anchor+1], anchorByte)
		if rel < 0 {
			return -1
		}
		start += rel
		if str[start+guard] != guardByte {
			if guardByte != anchorByte {
				anchor, guard = guard, anchor
				anchorByte, guardByte = guardByte, anchorByte
			}
			start++
			continue
		}
		if str[start:start+m] == literal {
			return start
		}
		fails++
		if fails >= 8 {
			rel := strings.Index(str[start+1:], literal)
			if rel < 0 {
				return -1
			}
			return start + 1 + rel
		}
		start++
	}
	return -1
}

func matchLiteralStarsCompiledFold(str, prefix, middle, suffix string, singleMiddle bool) bool {
	prefixEnd := 0
	if len(prefix) > 0 {
		var matches bool
		prefixEnd, matches = matchPrefixFold(str, prefix)
		if !matches {
			return false
		}
	}
	suffixStart := len(str)
	if len(suffix) > 0 {
		var matches bool
		suffixStart, matches = matchSuffixFold(str, suffix)
		if !matches {
			return false
		}
	}
	if prefixEnd > suffixStart {
		return false
	}
	if len(middle) == 0 {
		return true
	}
	str = str[prefixEnd:suffixStart]
	if singleMiddle {
		index, _ := indexFold(str, middle)
		return index >= 0
	}
	return matchMiddleLiteralsFold(str, middle)
}

func matchMiddleLiteralsFold(str, middle string) bool {
	for len(middle) > 0 {
		if middle[0] == '*' {
			middle = middle[1:]
			continue
		}
		star := strings.IndexByte(middle, '*')
		if star < 0 {
			index, _ := indexFold(str, middle)
			return index >= 0
		}
		literal := middle[:star]
		index, width := indexFold(str, literal)
		if index < 0 {
			return false
		}
		str = str[index+width:]
		middle = middle[star+1:]
	}
	return true
}

// indexFold reports the byte index of the first case-insensitive occurrence of
// literal in str, and the number of bytes matched in str. It returns -1, 0 if
// literal is not found.
//
// Pure-ASCII inputs use a byte-oriented search. Haystacks containing non-ASCII
// runes require a rune walk so the result is the first folded occurrence, not
// merely the first ASCII occurrence after it (for example, K before K).
func indexFold(str, literal string) (int, int) {
	if len(literal) == 0 {
		return 0, 0
	}
	// Immediate hits need no whole-haystack ASCII scan.
	if width, matches := matchPrefixFold(str, literal); matches {
		return 0, width
	}
	if isASCII(str) {
		if isASCII(literal) {
			if index := indexASCIIFold(str, literal); index >= 0 {
				return index, len(literal)
			}
			return -1, 0
		}
		if !canFoldToASCII(literal) {
			return -1, 0
		}
	}
	for offset := 0; offset <= len(str); {
		if width, matches := matchPrefixFold(str[offset:], literal); matches {
			return offset, width
		}
		if offset == len(str) {
			break
		}
		_, size := decodeRune(str[offset:])
		offset += size
	}
	return -1, 0
}

func canFoldToASCII(s string) bool {
	for len(s) > 0 {
		if s[0] < utf8.RuneSelf {
			s = s[1:]
			continue
		}
		r, size := utf8.DecodeRuneInString(s)
		if r != '\u017f' && r != '\u212a' {
			return false
		}
		s = s[size:]
	}
	return true
}

// indexASCIIFold finds literal in an ASCII haystack, ignoring ASCII case.
// It uses an adaptive two-byte anchor/guard pair with 8-byte SWAR scanning so
// repetitive prefixes never rescan the whole literal or require a frequency pass.
func indexASCIIFold(str, literal string) int {
	n, m := len(str), len(literal)
	if m > n {
		return -1
	}
	if m == 1 {
		return indexByteASCIIFold(str, literal[0])
	}
	anchor := m - 1
	anchorByte := literal[anchor]
	anchorLower := lowerASCIIByte(anchorByte)

	guard := 0
	guardByte := literal[0]
	guardLower := lowerASCIIByte(guardByte)
	if guardLower == anchorLower {
		for i := m - 2; i > 0; i-- {
			b := literal[i]
			bl := lowerASCIIByte(b)
			if bl != anchorLower {
				guard = i
				guardByte = b
				guardLower = bl
				break
			}
		}
	}
	if guardLower != anchorLower && lowerASCIIByte(str[anchor]) == anchorLower {
		anchor, guard = guard, anchor
		anchorByte, guardByte = guardByte, anchorByte
		anchorLower, guardLower = guardLower, anchorLower
	}

	limit := n - m
	for start := 0; start <= limit; {
		rel := indexByteASCIIFold(str[start+anchor:limit+anchor+1], anchorByte)
		if rel < 0 {
			return -1
		}
		start += rel
		if lowerASCIIByte(str[start+guard]) != guardLower {
			if guardLower != anchorLower {
				anchor, guard = guard, anchor
				anchorByte, guardByte = guardByte, anchorByte
				anchorLower, guardLower = guardLower, anchorLower
			}
			start++
			continue
		}
		if equalASCIIFold(str[start:start+m], literal) {
			return start
		}
		start++
	}
	return -1
}

func indexByteASCIIFold(s string, target byte) int {
	lower := lowerASCIIByte(target)
	if lower < 'a' || lower > 'z' {
		return strings.IndexByte(s, target)
	}
	const lo = 0x0101010101010101
	const hi = 0x8080808080808080
	const caseBit = 0x2020202020202020
	mask := uint64(lower) * lo
	i := 0
	n := len(s)
	for ; i+16 <= n; i += 16 {
		x0 := (readUint64(s, i) | caseBit) ^ mask
		m0 := (x0 - lo) &^ x0 & hi
		if m0 != 0 {
			return i + (bits.TrailingZeros64(m0) >> 3)
		}
		x1 := (readUint64(s, i+8) | caseBit) ^ mask
		m1 := (x1 - lo) &^ x1 & hi
		if m1 != 0 {
			return i + 8 + (bits.TrailingZeros64(m1) >> 3)
		}
	}
	for ; i+8 <= n; i += 8 {
		x := (readUint64(s, i) | caseBit) ^ mask
		m := (x - lo) &^ x & hi
		if m != 0 {
			return i + (bits.TrailingZeros64(m) >> 3)
		}
	}
	for ; i < n; i++ {
		if (s[i] | 0x20) == lower {
			return i
		}
	}
	return -1
}

func matchASCIIFoldPrefixScalar(str, pattern string) (int, bool) {
	limit := min(len(str), len(pattern))
	consumed := 0
	for consumed+8 <= limit {
		w1 := readUint64(str, consumed)
		w2 := readUint64(pattern, consumed)
		if (w1|w2)&0x8080808080808080 != 0 {
			break
		}
		if w1 != w2 && lowerASCIIWord(w1) != lowerASCIIWord(w2) {
			return 0, false
		}
		consumed += 8
	}
	for consumed < limit {
		b1, b2 := str[consumed], pattern[consumed]
		if b1 >= utf8.RuneSelf || b2 >= utf8.RuneSelf {
			break
		}
		if b1 != b2 && lowerASCIIByte(b1) != lowerASCIIByte(b2) {
			return 0, false
		}
		consumed++
	}
	return consumed, true
}

func matchLiteralFold(str, literal string) bool {
	consumed, matches := matchASCIIFoldPrefix(str, literal)
	if !matches {
		return false
	}
	str = str[consumed:]
	literal = literal[consumed:]
	for len(str) > 0 && len(literal) > 0 {
		strChar, strSize := decodeRune(str)
		literalChar, literalSize := decodeRune(literal)
		if !runesEqualFold(strChar, literalChar) {
			return false
		}
		str = str[strSize:]
		literal = literal[literalSize:]
	}
	return len(str) == 0 && len(literal) == 0
}

func matchPrefixFold(str, prefix string) (int, bool) {
	consumed, matches := matchASCIIFoldPrefix(str, prefix)
	if !matches {
		return 0, false
	}
	str = str[consumed:]
	prefix = prefix[consumed:]
	for len(prefix) > 0 {
		if len(str) == 0 {
			return 0, false
		}
		strChar, strSize := decodeRune(str)
		prefixChar, prefixSize := decodeRune(prefix)
		if !runesEqualFold(strChar, prefixChar) {
			return 0, false
		}
		str = str[strSize:]
		prefix = prefix[prefixSize:]
		consumed += strSize
	}
	return consumed, true
}

func matchSuffixFold(str, suffix string) (int, bool) {
	remaining := len(str)
	suffixEnd := len(suffix)
	for suffixEnd >= 8 && remaining >= 8 {
		w1 := readUint64(str, remaining-8)
		w2 := readUint64(suffix, suffixEnd-8)
		if (w1|w2)&0x8080808080808080 != 0 {
			break
		}
		if w1 != w2 && lowerASCIIWord(w1) != lowerASCIIWord(w2) {
			return 0, false
		}
		remaining -= 8
		suffixEnd -= 8
	}
	for suffixEnd > 0 && remaining > 0 {
		b1, b2 := str[remaining-1], suffix[suffixEnd-1]
		if b1 >= utf8.RuneSelf || b2 >= utf8.RuneSelf {
			break
		}
		if b1 != b2 && lowerASCIIByte(b1) != lowerASCIIByte(b2) {
			return 0, false
		}
		remaining--
		suffixEnd--
	}
	suffix = suffix[:suffixEnd]
	for len(suffix) > 0 {
		if remaining == 0 {
			return 0, false
		}
		strChar, strSize := utf8.DecodeLastRuneInString(str[:remaining])
		suffixChar, suffixSize := utf8.DecodeLastRuneInString(suffix)
		if !runesEqualFold(strChar, suffixChar) {
			return 0, false
		}
		remaining -= strSize
		suffix = suffix[:len(suffix)-suffixSize]
	}
	return remaining, true
}

func equalASCIIFold(str, literal string) bool {
	i := 0
	n := len(literal)
	for ; i+8 <= n; i += 8 {
		w1 := readUint64(str, i)
		w2 := readUint64(literal, i)
		if w1 != w2 && lowerASCIIWord(w1) != lowerASCIIWord(w2) {
			return false
		}
	}
	for ; i < n; i++ {
		sb := str[i]
		lb := literal[i]
		if sb != lb && lowerASCIIByte(sb) != lowerASCIIByte(lb) {
			return false
		}
	}
	return true
}

func isASCII(s string) bool {
	// Check 8 bytes at a time: any high bit means a non-ASCII byte.
	i := 0
	n := len(s)
	for ; i+8 <= n; i += 8 {
		if readUint64(s, i)&0x8080808080808080 != 0 {
			return false
		}
	}
	for ; i < n; i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func readUint64(s string, i int) uint64 {
	b := s[i : i+8]
	return uint64(b[0]) |
		uint64(b[1])<<8 |
		uint64(b[2])<<16 |
		uint64(b[3])<<24 |
		uint64(b[4])<<32 |
		uint64(b[5])<<40 |
		uint64(b[6])<<48 |
		uint64(b[7])<<56
}

func lowerASCIIWord(w uint64) uint64 {
	isUpper := ((w + 0x3f3f3f3f3f3f3f3f) &^ (w + 0x2525252525252525)) & 0x8080808080808080
	return w | (isUpper >> 2)
}

func specialASCII8Mask(w uint64) uint64 {
	const lo = 0x0101010101010101
	const hi = 0x8080808080808080
	star := (w ^ 0x2a2a2a2a2a2a2a2a) - lo
	ques := (w ^ 0x3f3f3f3f3f3f3f3f) - lo
	brack := (w ^ 0x5b5b5b5b5b5b5b5b) - lo
	bslash := (w ^ 0x5c5c5c5c5c5c5c5c) - lo
	return (w | star | ques | brack | bslash) & hi
}

func runesMatch(a, b rune, nocase bool) bool {
	if nocase {
		return runesEqualFold(a, b)
	}
	return a == b
}

func runesEqualFold(a, b rune) bool {
	if a == b {
		return true
	}
	return runesEqualFoldSlow(a, b)
}

func runesEqualFoldSlow(a, b rune) bool {
	if a <= unicode.MaxASCII && b <= unicode.MaxASCII {
		return lowerASCIIRune(a) == lowerASCIIRune(b)
	}
	for folded := unicode.SimpleFold(a); folded != a; folded = unicode.SimpleFold(folded) {
		if folded == b {
			return true
		}
	}
	return false
}

func runeMatchesRange(char, start, end rune, fold bool) bool {
	if !fold {
		return char >= start && char <= end
	}
	return runeMatchesRangeFold(char, start, end)
}

func runeMatchesRangeFold(char, start, end rune) bool {
	candidate := char
	for {
		if candidate >= start && candidate <= end {
			return true
		}
		candidate = unicode.SimpleFold(candidate)
		if candidate == char {
			return false
		}
	}
}

func decodeRune(s string) (rune, int) {
	if s[0] < utf8.RuneSelf {
		return rune(s[0]), 1
	}
	return utf8.DecodeRuneInString(s)
}

func lowerASCIIRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

func lowerASCIIByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func b2s(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
