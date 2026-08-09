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
	kind   tokenKind
	char   rune   // single-rune literal (tokenLiteral)
	folded rune   // lower(char) for fold matching
	lit    string // multi-rune literal run (tokenLiteralRun)
	count  int    // consecutive '?' count (tokenAnyN)
	class  []charRange
	// ascii is non-nil for tokenClass and holds ASCII membership bitsets.
	// Kept behind a pointer so non-class tokens stay small.
	ascii   *classASCIIBits
	negated bool
}

// classASCIIBits holds 256-bit membership maps for sensitive and folded ASCII.
type classASCIIBits struct {
	bits       [4]uint64
	foldedBits [4]uint64
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
	start, end             rune
	foldedStart, foldedEnd rune
}

// Compile parses pattern for repeated matching. Invalid patterns compile to a
// Pattern that never matches, consistent with Match's existing behavior.
func Compile(pattern string) *Pattern {
	p := &Pattern{valid: true}
	if p.prefix, p.suffix, p.hasStar, p.simple = splitSimplePattern(pattern); p.simple {
		return p
	}
	originalPattern := pattern
	literalOnly := true
	starCount := 0
	// Cap is a lower bound on tokens: one per meta char, plus room for runs.
	p.tokens = make([]token, 0, len(pattern)/2+1)
	var litBuf []byte
	flushLit := func() {
		if len(litBuf) == 0 {
			return
		}
		if len(litBuf) == 1 && litBuf[0] < utf8.RuneSelf {
			p.tokens = append(p.tokens, token{
				kind:   tokenLiteral,
				char:   rune(litBuf[0]),
				folded: lowerRune(rune(litBuf[0])),
			})
		} else if r, n := utf8.DecodeRune(litBuf); n == len(litBuf) && r != utf8.RuneError {
			// Single well-formed non-ASCII rune.
			p.tokens = append(p.tokens, token{
				kind:   tokenLiteral,
				char:   r,
				folded: lowerRune(r),
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
			literalOnly = false
			p.tokens = append(p.tokens, token{
				kind:   tokenLiteral,
				char:   utf8.RuneError,
				folded: utf8.RuneError,
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
				starCount++
			}
		case '?':
			flushLit()
			literalOnly = false
			if n := len(p.tokens); n > 0 && p.tokens[n-1].kind == tokenAnyN {
				p.tokens[n-1].count++
			} else if n > 0 && p.tokens[n-1].kind == tokenAny {
				p.tokens[n-1] = token{kind: tokenAnyN, count: 2}
			} else {
				p.tokens = append(p.tokens, token{kind: tokenAny})
			}
		case '[':
			flushLit()
			literalOnly = false
			var class token
			class, pattern, p.valid = compileClass(pattern[size:])
			if !p.valid {
				return p
			}
			p.tokens = append(p.tokens, class)
			continue
		case '\\':
			// Escaped literals break "literalOnly" multi-star fast path, same as before.
			literalOnly = false
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
	if literalOnly && starCount > 1 {
		p.literalStars = true
		p.prefix = originalPattern
	}
	return p
}

func compileClass(pattern string) (token, string, bool) {
	class := token{kind: tokenClass, ascii: &classASCIIBits{}}
	if len(pattern) == 0 {
		return class, pattern, false
	}
	if pattern[0] == '^' {
		class.negated = true
		pattern = pattern[1:]
	}
	for {
		if len(pattern) == 0 {
			return class, pattern, false
		}
		start, size := decodeRune(pattern)
		if start == '\\' {
			pattern = pattern[size:]
			if len(pattern) == 0 {
				return class, pattern, false
			}
			start, size = decodeRune(pattern)
		} else if start == ']' {
			return class, pattern[size:], true
		} else if len(pattern) > size+1 && pattern[size] == '-' {
			pattern = pattern[size+1:]
			end, endSize := decodeRune(pattern)
			if start > end {
				start, end = end, start
			}
			class.class = append(class.class, newCharRange(start, end))
			addClassRangeBits(class.ascii, start, end)
			pattern = pattern[endSize:]
			continue
		}
		class.class = append(class.class, newCharRange(start, start))
		addClassRangeBits(class.ascii, start, start)
		pattern = pattern[size:]
	}
}

func addClassRangeBits(bits *classASCIIBits, start, end rune) {
	// start/end are already ordered by code point (same as stringmatch).
	// Sensitive bitset: every ASCII code point in [start, end].
	if start <= unicode.MaxASCII {
		hi := end
		if hi > unicode.MaxASCII {
			hi = unicode.MaxASCII
		}
		for r := start; r <= hi; r++ {
			if r >= 0 {
				setASCIIBit(&bits.bits, byte(r))
			}
		}
	}

	// Fold bitset must mirror stringmatch: lower the ordered endpoints and
	// do NOT re-sort. If lower(start) > lower(end), the folded range is empty
	// (e.g. [b-C] → endpoints C..b → folded c..b matches nothing).
	foldedStart, foldedEnd := lowerRune(start), lowerRune(end)
	if foldedStart <= foldedEnd {
		lo, hi := foldedStart, foldedEnd
		if lo < 0 {
			lo = 0
		}
		if lo <= unicode.MaxASCII {
			if hi > unicode.MaxASCII {
				hi = unicode.MaxASCII
			}
			for r := lo; r <= hi; r++ {
				setASCIIBit(&bits.foldedBits, byte(r))
			}
		}
	}

	// Singleton non-ASCII that folds into ASCII (e.g. K → k). Only safe for
	// single-code-point entries; multi-code ranges keep range-list matching.
	if start == end && start > unicode.MaxASCII {
		if folded := lowerRune(start); folded <= unicode.MaxASCII {
			setASCIIBit(&bits.foldedBits, byte(folded))
		}
	}
}

func setASCIIBit(bits *[4]uint64, b byte) {
	bits[b>>6] |= 1 << (b & 63)
}

func asciiBit(bits [4]uint64, b byte) bool {
	return bits[b>>6]&(1<<(b&63)) != 0
}

func newCharRange(start, end rune) charRange {
	return charRange{
		start:       start,
		end:         end,
		foldedStart: lowerRune(start),
		foldedEnd:   lowerRune(end),
	}
}

// Match reports whether str matches the compiled pattern.
func (p *Pattern) Match(str string) bool {
	return p.match(str, false)
}

// MatchFold reports whether str matches the compiled pattern, ignoring case.
func (p *Pattern) MatchFold(str string) bool {
	return p.match(str, true)
}

// MatchBytes reports whether b matches the compiled pattern.
func (p *Pattern) MatchBytes(b []byte) bool {
	return p.Match(b2s(b))
}

// MatchBytesFold reports whether b matches the compiled pattern, ignoring case.
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
	if p.literalStars && len(str) >= 64 {
		if fold {
			return matchLiteralStarsValidFold(str, p.prefix)
		}
		return matchLiteralStarsValid(str, p.prefix)
	}
	tokens := p.tokens
	var suffixMatches bool
	str, tokens, suffixMatches = trimPatternSuffix(str, tokens, fold)
	if !suffixMatches {
		return false
	}
	tokenIndex, stringIndex := 0, 0
	starToken, starString := -1, 0
	for stringIndex < len(str) || tokenIndex < len(tokens) {
		if tokenIndex < len(tokens) {
			tok := &tokens[tokenIndex]
			switch tok.kind {
			case tokenStar:
				if tokenIndex == len(tokens)-1 {
					return true
				}
				starToken, starString = tokenIndex, stringIndex
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
					if tok.matches(char, fold) {
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

func trimPatternSuffix(str string, tokens []token, fold bool) (string, []token, bool) {
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
			if !tok.matches(char, fold) {
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
// For ASCII needles, try a byte-oriented search first. That is sufficient for
// pure-ASCII haystacks (the common Redis-key case). Only if that search misses
// do we fall back to a rune walk, which is required for code points such as
// U+212A (K) that case-fold onto ASCII letters.
func indexFold(str, literal string) (int, int) {
	if len(literal) == 0 {
		return 0, 0
	}
	if isASCII(literal) {
		if index := indexASCIIFold(str, literal); index >= 0 {
			return index, len(literal)
		}
		// No ASCII placement matched. A non-ASCII rune in str may still fold
		// onto the needle (e.g. K → k), so only then pay for a full rune scan.
		if isASCII(str) {
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

// indexASCIIFold finds literal in an ASCII haystack, ignoring ASCII case.
// It jumps with strings.IndexByte on both cases of the first needle byte.
func indexASCIIFold(str, literal string) int {
	n, m := len(str), len(literal)
	if m > n {
		return -1
	}
	first := literal[0]
	firstLo := first
	if firstLo >= 'A' && firstLo <= 'Z' {
		firstLo += 'a' - 'A'
	}
	// firstHi is the uppercase twin when first is a letter; otherwise unused.
	firstHi := firstLo - ('a' - 'A')
	letter := firstLo >= 'a' && firstLo <= 'z'
	limit := n - m
	for i := 0; i <= limit; {
		var rel int
		if letter {
			// Search for both cases; IndexByte is SIMD-backed on common arches.
			lo := strings.IndexByte(str[i:], firstLo)
			hi := strings.IndexByte(str[i:], firstHi)
			switch {
			case lo < 0 && hi < 0:
				return -1
			case lo < 0:
				rel = hi
			case hi < 0:
				rel = lo
			default:
				if lo < hi {
					rel = lo
				} else {
					rel = hi
				}
			}
		} else {
			rel = strings.IndexByte(str[i:], first)
			if rel < 0 {
				return -1
			}
		}
		i += rel
		if i > limit {
			return -1
		}
		if equalASCIIFold(str[i:i+m], literal) {
			return i
		}
		i++
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
		if sb >= 'A' && sb <= 'Z' {
			sb += 'a' - 'A'
		}
		if lb >= 'A' && lb <= 'Z' {
			lb += 'a' - 'A'
		}
		if sb != lb {
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
		if lowerRune(rune(str[consumed])) != lowerRune(rune(pattern[consumed])) {
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
		if lowerRune(strChar) != lowerRune(literalChar) {
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
		if lowerRune(strChar) != lowerRune(prefixChar) {
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
		if lowerRune(rune(str[remaining-1])) != lowerRune(rune(suffix[len(suffix)-1])) {
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
		if lowerRune(strChar) != lowerRune(suffixChar) {
			return 0, false
		}
		remaining -= strSize
		suffix = suffix[:len(suffix)-suffixSize]
	}
	return remaining, true
}

func (t token) matches(char rune, fold bool) bool {
	switch t.kind {
	case tokenLiteral:
		if fold {
			return t.folded == lowerRune(char)
		}
		return t.char == char
	case tokenAny:
		return true
	case tokenClass:
		matched := false
		if fold {
			folded := lowerRune(char)
			if folded <= unicode.MaxASCII && t.ascii != nil && asciiBit(t.ascii.foldedBits, byte(folded)) {
				matched = true
			} else {
				// Range fallback for non-ASCII folded chars and for singletons
				// whose fold-into-ASCII bit may be missing from sparse scans.
				for _, r := range t.class {
					if folded >= r.foldedStart && folded <= r.foldedEnd {
						matched = true
						break
					}
				}
			}
		} else if char <= unicode.MaxASCII {
			if t.ascii != nil {
				matched = asciiBit(t.ascii.bits, byte(char))
			}
		} else {
			for _, r := range t.class {
				if char >= r.start && char <= r.end {
					matched = true
					break
				}
			}
		}
		return matched != t.negated
	default:
		return false
	}
}
