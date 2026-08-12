package redglob

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pattern is a compiled glob pattern. A Pattern is safe for concurrent use.
// Compiling is useful when the same pattern is matched against many inputs.
type Pattern struct {
	tokens       []token
	prefix       string
	suffix       string
	valid        bool
	simple       bool
	hasStar      bool
	literalStars bool
}

type token struct {
	kind  tokenKind
	char  rune   // single-rune literal (tokenLiteral)
	lit   string // multi-rune literal run (tokenLiteralRun)
	count int    // consecutive '?' count (tokenAnyN)
	class *compiledClass
}

// compiledClass holds a 128-bit membership map for ASCII. The common single
// range is inline; larger classes allocate an overflow slice.
type compiledClass struct {
	bits       [2]uint64
	rangeOne   charRange
	ranges     []charRange
	rangeCount int
	negated    bool
}

type tokenKind uint8

const (
	tokenLiteral tokenKind = iota
	tokenLiteralRun
	tokenAny
	tokenAnyN
	tokenStar
	tokenClass
)

type charRange struct {
	start, end rune
}

// Compile parses pattern for repeated matching. Invalid patterns compile to a
// Pattern that never matches, consistent with Match's existing behavior.
func Compile(pattern string) *Pattern {
	p := &Pattern{valid: true}
	if p.prefix, p.suffix, p.hasStar, p.simple = splitSimplePattern(pattern); p.simple {
		return p
	}
	if isLiteralStarsPattern(pattern) {
		p.literalStars = true
		p.prefix = pattern
		return p
	}
	// Most patterns compile to only a few tokens. Keep the initial allocation
	// bounded so a long literal does not retain a token array many times larger
	// than the pattern itself.
	p.tokens = make([]token, 0, min(len(pattern)/2+1, 32))
	var litBuf []byte
	flushLit := func() {
		if len(litBuf) == 0 {
			return
		}
		if len(litBuf) == 1 && litBuf[0] < utf8.RuneSelf {
			p.tokens = append(p.tokens, token{
				kind: tokenLiteral,
				char: rune(litBuf[0]),
			})
		} else if r, n := utf8.DecodeRune(litBuf); n == len(litBuf) && r != utf8.RuneError {
			// Single well-formed non-ASCII rune.
			p.tokens = append(p.tokens, token{
				kind: tokenLiteral,
				char: r,
			})
		} else {
			p.tokens = append(p.tokens, token{
				kind: tokenLiteralRun,
				lit:  string(litBuf),
			})
		}
		litBuf = litBuf[:0]
	}
	appendLiteral := func(char rune, raw string) {
		// Invalid UTF-8 is decoded as RuneError with size 1. Matching is by
		// rune identity (any invalid byte matches any other), so these must
		// stay as single-rune tokens and must not merge into byte runs.
		if char == utf8.RuneError && (len(raw) != 3 || raw != string(utf8.RuneError)) {
			flushLit()
			p.tokens = append(p.tokens, token{
				kind: tokenLiteral,
				char: utf8.RuneError,
			})
			return
		}
		litBuf = append(litBuf, raw...)
	}
	for len(pattern) > 0 {
		char, size := decodeRune(pattern)
		switch char {
		case '*':
			flushLit()
			if len(p.tokens) == 0 || p.tokens[len(p.tokens)-1].kind != tokenStar {
				p.tokens = append(p.tokens, token{kind: tokenStar})
			}
		case '?':
			flushLit()
			if n := len(p.tokens); n > 0 && p.tokens[n-1].kind == tokenAnyN {
				p.tokens[n-1].count++
			} else if n > 0 && p.tokens[n-1].kind == tokenAny {
				p.tokens[n-1] = token{kind: tokenAnyN, count: 2}
			} else {
				p.tokens = append(p.tokens, token{kind: tokenAny})
			}
		case '[':
			flushLit()
			var class token
			class, pattern, p.valid = compileClass(pattern[size:])
			if !p.valid {
				return p
			}
			p.tokens = append(p.tokens, class)
			continue
		case '\\':
			pattern = pattern[size:]
			if len(pattern) == 0 {
				p.valid = false
				return p
			}
			char, size = decodeRune(pattern)
			appendLiteral(char, pattern[:size])
		default:
			appendLiteral(char, pattern[:size])
		}
		pattern = pattern[size:]
	}
	flushLit()
	return p
}

func compileClass(pattern string) (token, string, bool) {
	class := &compiledClass{}
	if len(pattern) == 0 {
		return token{kind: tokenClass}, pattern, false
	}
	if pattern[0] == '^' {
		class.negated = true
		pattern = pattern[1:]
	}
	for {
		if len(pattern) == 0 {
			return token{kind: tokenClass}, pattern, false
		}
		start, size := decodeRune(pattern)
		if start == '\\' {
			pattern = pattern[size:]
			if len(pattern) == 0 {
				return token{kind: tokenClass}, pattern, false
			}
			start, size = decodeRune(pattern)
		} else if start == ']' {
			return token{kind: tokenClass, class: class}, pattern[size:], true
		} else if len(pattern) > size+1 && pattern[size] == '-' {
			pattern = pattern[size+1:]
			end, endSize := decodeRune(pattern)
			if start > end {
				start, end = end, start
			}
			class.addRange(newCharRange(start, end))
			addClassRangeBits(class, start, end)
			pattern = pattern[endSize:]
			continue
		}
		class.addRange(newCharRange(start, start))
		addClassRangeBits(class, start, start)
		pattern = pattern[size:]
	}
}

func (class *compiledClass) addRange(r charRange) {
	if class.rangeCount == 0 {
		class.rangeOne = r
	} else {
		if class.ranges == nil {
			class.ranges = append(class.ranges, class.rangeOne)
		}
		class.ranges = append(class.ranges, r)
	}
	class.rangeCount++
}

func (class *compiledClass) matchesRange(char rune) bool {
	if class.rangeCount == 1 {
		return char >= class.rangeOne.start && char <= class.rangeOne.end
	}
	for _, r := range class.ranges {
		if char >= r.start && char <= r.end {
			return true
		}
	}
	return false
}

func (class *compiledClass) contains(char rune) bool {
	if char <= unicode.MaxASCII {
		return asciiBit(class.bits, byte(char))
	}
	return class.matchesRange(char)
}

func (class *compiledClass) matchesFold(char rune) bool {
	candidate := char
	for {
		if class.contains(candidate) {
			return true
		}
		candidate = unicode.SimpleFold(candidate)
		if candidate == char {
			return false
		}
	}
}

func addClassRangeBits(class *compiledClass, start, end rune) {
	// start/end are already ordered by code point (same as stringmatch).
	// Sensitive bitset: every ASCII code point in [start, end].
	if start <= unicode.MaxASCII {
		hi := end
		if hi > unicode.MaxASCII {
			hi = unicode.MaxASCII
		}
		for r := start; r <= hi; r++ {
			if r >= 0 {
				setASCIIBit(&class.bits, byte(r))
			}
		}
	}
}

func setASCIIBit(bits *[2]uint64, b byte) {
	bits[b>>6] |= 1 << (b & 63)
}

func asciiBit(bits [2]uint64, b byte) bool {
	return bits[b>>6]&(1<<(b&63)) != 0
}

func newCharRange(start, end rune) charRange {
	return charRange{
		start: start,
		end:   end,
	}
}

// Match reports whether str matches the compiled pattern.
func (p *Pattern) Match(str string) bool {
	return p.match(str, false)
}

// MatchFold reports whether str matches the compiled pattern using Unicode
// simple case folding.
func (p *Pattern) MatchFold(str string) bool {
	return p.match(str, true)
}

// MatchBytes reports whether b matches the compiled pattern.
func (p *Pattern) MatchBytes(b []byte) bool {
	return p.Match(b2s(b))
}

// MatchBytesFold reports whether b matches the compiled pattern using Unicode
// simple case folding.
func (p *Pattern) MatchBytesFold(b []byte) bool {
	return p.MatchFold(b2s(b))
}

func (p *Pattern) match(str string, fold bool) bool {
	if p == nil || !p.valid {
		return false
	}
	if p.simple {
		if !p.hasStar {
			if fold {
				return matchLiteralFold(str, p.prefix)
			}
			return str == p.prefix
		}
		if fold {
			return matchSimpleFold(str, p.prefix, p.suffix)
		}
		return matchSimple(str, p.prefix, p.suffix)
	}
	if p.literalStars {
		if fold {
			return matchLiteralStarsValidFold(str, p.prefix)
		}
		return matchLiteralStarsValid(str, p.prefix)
	}
	tokens := p.tokens
	var suffixMatches bool
	str, tokens, suffixMatches = p.trimPatternSuffix(str, tokens, fold)
	if !suffixMatches {
		return false
	}
	tokenIndex, stringIndex := 0, 0
	starToken, starString, starLiteral := -1, 0, -1
	for stringIndex < len(str) || tokenIndex < len(tokens) {
		if tokenIndex < len(tokens) {
			tok := &tokens[tokenIndex]
			switch tok.kind {
			case tokenStar:
				if tokenIndex == len(tokens)-1 {
					return true
				}
				starToken, starString = tokenIndex, stringIndex
				starLiteral = -1
				if tokenIndex+1 < len(tokens) && tokens[tokenIndex+1].kind == tokenLiteralRun {
					starLiteral = tokenIndex + 1
					index, width := indexLiteral(str[stringIndex:], tokens[starLiteral].lit, fold)
					if index < 0 {
						return false
					}
					starString += index
					stringIndex = starString + width
					tokenIndex += 2
					continue
				}
				tokenIndex++
				continue
			case tokenAnyN:
				next, ok := consumeAnyN(str, stringIndex, tok.count)
				if ok {
					tokenIndex++
					stringIndex = next
					continue
				}
			case tokenLiteralRun:
				next, ok := consumeLiteralRun(str, stringIndex, tok.lit, fold)
				if ok {
					tokenIndex++
					stringIndex = next
					continue
				}
			default:
				if stringIndex < len(str) {
					char, size := decodeRune(str[stringIndex:])
					if p.tokenMatches(tok, char, fold) {
						tokenIndex++
						stringIndex += size
						continue
					}
				}
			}
		} else if stringIndex >= len(str) {
			break
		}
		if starToken < 0 {
			return false
		}
		if starString >= len(str) {
			return false
		}
		_, size := decodeRune(str[starString:])
		starString += size
		if starLiteral >= 0 {
			index, width := indexLiteral(str[starString:], tokens[starLiteral].lit, fold)
			if index < 0 {
				return false
			}
			starString += index
			stringIndex = starString + width
			tokenIndex = starLiteral + 1
			continue
		}
		stringIndex = starString
		tokenIndex = starToken + 1
	}
	for tokenIndex < len(tokens) && tokens[tokenIndex].kind == tokenStar {
		tokenIndex++
	}
	return tokenIndex == len(tokens) && stringIndex == len(str)
}

func consumeAnyN(str string, offset, count int) (int, bool) {
	for i := 0; i < count; i++ {
		if offset >= len(str) {
			return 0, false
		}
		// Fast path: ASCII byte is one rune.
		if str[offset] < utf8.RuneSelf {
			offset++
			continue
		}
		_, size := utf8.DecodeRuneInString(str[offset:])
		offset += size
	}
	return offset, true
}

func consumeLiteralRun(str string, offset int, lit string, fold bool) (int, bool) {
	if !fold {
		if offset+len(lit) > len(str) || str[offset:offset+len(lit)] != lit {
			return 0, false
		}
		return offset + len(lit), true
	}
	end, ok := matchPrefixFold(str[offset:], lit)
	if !ok {
		return 0, false
	}
	return offset + end, true
}

func (p *Pattern) trimPatternSuffix(str string, tokens []token, fold bool) (string, []token, bool) {
	starIndex := len(tokens) - 1
	for starIndex >= 0 && tokens[starIndex].kind != tokenStar {
		starIndex--
	}
	if starIndex < 0 || starIndex == len(tokens)-1 {
		return str, tokens, true
	}

	end := len(str)
	for tokenIndex := len(tokens) - 1; tokenIndex > starIndex; tokenIndex-- {
		tok := &tokens[tokenIndex]
		switch tok.kind {
		case tokenLiteralRun:
			next, ok := consumeLiteralRunSuffix(str, end, tok.lit, fold)
			if !ok {
				return str, tokens, false
			}
			end = next
		case tokenAnyN:
			next, ok := consumeAnyNSuffix(str, end, tok.count)
			if !ok {
				return str, tokens, false
			}
			end = next
		default:
			if end == 0 {
				return str, tokens, false
			}
			char, size := utf8.DecodeLastRuneInString(str[:end])
			if !p.tokenMatches(tok, char, fold) {
				return str, tokens, false
			}
			end -= size
		}
	}
	return str[:end], tokens[:starIndex+1], true
}

func consumeAnyNSuffix(str string, end, count int) (int, bool) {
	for i := 0; i < count; i++ {
		if end == 0 {
			return 0, false
		}
		if str[end-1] < utf8.RuneSelf {
			end--
			continue
		}
		_, size := utf8.DecodeLastRuneInString(str[:end])
		end -= size
	}
	return end, true
}

func consumeLiteralRunSuffix(str string, end int, lit string, fold bool) (int, bool) {
	if !fold {
		if end < len(lit) || str[end-len(lit):end] != lit {
			return 0, false
		}
		return end - len(lit), true
	}
	start, ok := matchSuffixFold(str[:end], lit)
	if !ok {
		return 0, false
	}
	return start, true
}

func splitSimplePattern(pattern string) (prefix, suffix string, hasStar, ok bool) {
	starStart, starEnd := -1, -1
	for index, char := range pattern {
		switch char {
		case utf8.RuneError, '?', '[', '\\':
			return "", "", false, false
		case '*':
			if starStart < 0 {
				starStart = index
			} else if index != starEnd {
				return "", "", false, false
			}
			starEnd = index + 1
		}
	}
	if starStart < 0 {
		return pattern, "", false, true
	}
	return pattern[:starStart], pattern[starEnd:], true, true
}

func splitFixedSuffix(pattern string) (starEnd int, suffix string, ok bool) {
	lastStar := strings.LastIndexByte(pattern, '*')
	if lastStar < 0 || lastStar == len(pattern)-1 {
		return 0, "", false
	}
	for _, char := range pattern {
		switch char {
		case utf8.RuneError, '?', '[', '\\':
			return 0, "", false
		}
	}
	return lastStar + 1, pattern[lastStar+1:], true
}

func matchLiteralStars(str, pattern string) (bool, bool) {
	if !isLiteralStarsPattern(pattern) {
		return false, false
	}
	return matchLiteralStarsValid(str, pattern), true
}

func matchLiteralStarsFold(str, pattern string) (bool, bool) {
	if !isLiteralStarsPattern(pattern) {
		return false, false
	}
	return matchLiteralStarsValidFold(str, pattern), true
}

func isLiteralStarsPattern(pattern string) bool {
	firstStar := strings.IndexByte(pattern, '*')
	lastStar := strings.LastIndexByte(pattern, '*')
	if firstStar < 0 || firstStar == lastStar {
		return false
	}
	for _, char := range pattern {
		switch char {
		case utf8.RuneError, '?', '[', '\\':
			return false
		}
	}
	return true
}

func matchLiteralStarsValid(str, pattern string) bool {
	firstStar := strings.IndexByte(pattern, '*')
	lastStar := strings.LastIndexByte(pattern, '*')
	prefix, suffix := pattern[:firstStar], pattern[lastStar+1:]
	if len(str) < len(prefix)+len(suffix) ||
		!strings.HasPrefix(str, prefix) || !strings.HasSuffix(str, suffix) {
		return false
	}
	str = str[len(prefix) : len(str)-len(suffix)]
	pattern = pattern[firstStar+1 : lastStar]
	for len(pattern) > 0 {
		if pattern[0] == '*' {
			pattern = pattern[1:]
			continue
		}
		star := strings.IndexByte(pattern, '*')
		if star < 0 {
			star = len(pattern)
		}
		literal := pattern[:star]
		index := strings.Index(str, literal)
		if index < 0 {
			return false
		}
		str = str[index+len(literal):]
		pattern = pattern[star:]
	}
	return true
}

func matchLiteralStarsValidFold(str, pattern string) bool {
	firstStar := strings.IndexByte(pattern, '*')
	lastStar := strings.LastIndexByte(pattern, '*')
	prefix, suffix := pattern[:firstStar], pattern[lastStar+1:]

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

	str = str[prefixEnd:suffixStart]
	pattern = pattern[firstStar+1 : lastStar]
	for len(pattern) > 0 {
		if pattern[0] == '*' {
			pattern = pattern[1:]
			continue
		}
		star := strings.IndexByte(pattern, '*')
		if star < 0 {
			star = len(pattern)
		}
		literal := pattern[:star]
		index, width := indexFold(str, literal)
		if index < 0 {
			return false
		}
		str = str[index+width:]
		pattern = pattern[star:]
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
	if isASCII(literal) && isASCII(str) {
		if index := indexASCIIFold(str, literal); index >= 0 {
			return index, len(literal)
		}
		return -1, 0
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

// indexASCIIFold finds literal in an ASCII haystack, ignoring ASCII case. It
// anchors comparisons on the least frequent literal byte in the haystack so a
// repetitive prefix does not repeatedly rescan the whole literal.
func indexASCIIFold(str, literal string) int {
	n, m := len(str), len(literal)
	if m > n {
		return -1
	}
	if m < 64 {
		return indexASCIIFoldShort(str, literal)
	}

	var frequencies [utf8.RuneSelf]int
	for index := range len(str) {
		frequencies[lowerASCIIByte(str[index])]++
	}
	anchor := 0
	anchorFrequency := n + 1
	for index := range len(literal) {
		frequency := frequencies[lowerASCIIByte(literal[index])]
		if frequency < anchorFrequency {
			anchor = index
			anchorFrequency = frequency
		}
	}
	anchorByte := lowerASCIIByte(literal[anchor])
	limit := n - m
	for start := 0; start <= limit; start++ {
		if lowerASCIIByte(str[start+anchor]) == anchorByte && equalASCIIFold(str[start:start+m], literal) {
			return start
		}
	}
	return -1
}

func indexASCIIFoldShort(str, literal string) int {
	n, m := len(str), len(literal)
	first := literal[0]
	firstLower := lowerASCIIByte(first)
	firstUpper := firstLower - ('a' - 'A')
	letter := firstLower >= 'a' && firstLower <= 'z'
	limit := n - m
	for index := 0; index <= limit; {
		var relative int
		if letter {
			lower := strings.IndexByte(str[index:], firstLower)
			upper := strings.IndexByte(str[index:], firstUpper)
			switch {
			case lower < 0 && upper < 0:
				return -1
			case lower < 0:
				relative = upper
			case upper < 0:
				relative = lower
			default:
				relative = min(lower, upper)
			}
		} else {
			relative = strings.IndexByte(str[index:], first)
			if relative < 0 {
				return -1
			}
		}
		index += relative
		if index > limit {
			return -1
		}
		if equalASCIIFold(str[index:index+m], literal) {
			return index
		}
		index++
	}
	return -1
}

func equalASCIIFold(str, literal string) bool {
	for i := 0; i < len(literal); i++ {
		sb := str[i]
		lb := literal[i]
		if sb == lb {
			continue
		}
		if lowerASCIIByte(sb) != lowerASCIIByte(lb) {
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
		v := uint64(s[i]) |
			uint64(s[i+1])<<8 |
			uint64(s[i+2])<<16 |
			uint64(s[i+3])<<24 |
			uint64(s[i+4])<<32 |
			uint64(s[i+5])<<40 |
			uint64(s[i+6])<<48 |
			uint64(s[i+7])<<56
		if v&0x8080808080808080 != 0 {
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

func matchASCIIFoldPrefixScalar(str, pattern string) (int, bool) {
	consumed := 0
	for consumed < len(str) && consumed < len(pattern) &&
		str[consumed] < utf8.RuneSelf && pattern[consumed] < utf8.RuneSelf {
		if lowerASCIIRune(rune(str[consumed])) != lowerASCIIRune(rune(pattern[consumed])) {
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
	for len(suffix) > 0 && remaining > 0 && str[remaining-1] < utf8.RuneSelf && suffix[len(suffix)-1] < utf8.RuneSelf {
		if lowerASCIIRune(rune(str[remaining-1])) != lowerASCIIRune(rune(suffix[len(suffix)-1])) {
			return 0, false
		}
		remaining--
		suffix = suffix[:len(suffix)-1]
	}
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

func (p *Pattern) tokenMatches(t *token, char rune, fold bool) bool {
	switch t.kind {
	case tokenLiteral:
		if fold {
			return runesEqualFold(t.char, char)
		}
		return t.char == char
	case tokenAny:
		return true
	case tokenClass:
		class := t.class
		matched := false
		if fold {
			matched = class.matchesFold(char)
		} else {
			matched = class.contains(char)
		}
		return matched != class.negated
	default:
		return false
	}
}
