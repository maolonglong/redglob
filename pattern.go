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
	middle       string
	minLen       int
	lastStar     int
	valid        bool
	simple       bool
	hasStar      bool
	literalStars bool
	singleMiddle bool
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
	p := &Pattern{valid: true, lastStar: -1}
	if p.classifySimpleOrLiteralStars(pattern) {
		return p
	}
	// Most patterns compile to only a few tokens. Keep the initial allocation
	// bounded so a long literal does not retain a token array many times larger
	// than the pattern itself.
	p.tokens = make([]token, 0, min(len(pattern)/2+1, 32))
	litStart, litEnd := -1, -1
	var litBuf []byte
	flushLit := func() {
		var lit string
		if len(litBuf) > 0 {
			lit = b2s(litBuf)
			litBuf = nil
		} else if litStart >= 0 {
			lit = pattern[litStart:litEnd]
		} else {
			return
		}
		litStart, litEnd = -1, -1
		if len(lit) == 1 && lit[0] < utf8.RuneSelf {
			p.tokens = append(p.tokens, token{
				kind: tokenLiteral,
				char: rune(lit[0]),
			})
		} else if r, n := utf8.DecodeRuneInString(lit); n == len(lit) && r != utf8.RuneError {
			// Single well-formed non-ASCII rune.
			p.tokens = append(p.tokens, token{
				kind: tokenLiteral,
				char: r,
			})
		} else {
			p.tokens = append(p.tokens, token{
				kind: tokenLiteralRun,
				lit:  lit,
			})
		}
	}
	appendLiteral := func(char rune, start, end int) {
		// Both U+FFFD and invalid UTF-8 match by RuneError identity, not by
		// encoding. Keep them out of byte-oriented literal runs.
		if char == utf8.RuneError {
			flushLit()
			p.tokens = append(p.tokens, token{
				kind: tokenLiteral,
				char: utf8.RuneError,
			})
			return
		}
		if len(litBuf) > 0 {
			litBuf = append(litBuf, pattern[start:end]...)
		} else if litStart < 0 {
			litStart, litEnd = start, end
		} else if start == litEnd {
			litEnd = end
		} else {
			litBuf = make([]byte, 0, len(pattern)-litStart)
			litBuf = append(litBuf, pattern[litStart:litEnd]...)
			litBuf = append(litBuf, pattern[start:end]...)
		}
	}
	for index := 0; index < len(pattern); {
		char, size := decodeRune(pattern[index:])
		switch char {
		case '*':
			flushLit()
			if len(p.tokens) == 0 || p.tokens[len(p.tokens)-1].kind != tokenStar {
				p.lastStar = len(p.tokens)
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
			var rest string
			class, rest, p.valid = compileClass(pattern[index+size:])
			if !p.valid {
				return p
			}
			p.tokens = append(p.tokens, class)
			index = len(pattern) - len(rest)
			continue
		case '\\':
			index += size
			if index == len(pattern) {
				p.valid = false
				return p
			}
			char, size = decodeRune(pattern[index:])
			appendLiteral(char, index, index+size)
		default:
			appendLiteral(char, index, index+size)
		}
		index += size
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
			return matchLiteralStarsCompiledFold(str, p.prefix, p.middle, p.suffix, p.singleMiddle)
		}
		if len(str) < p.minLen || !strings.HasPrefix(str, p.prefix) || !strings.HasSuffix(str, p.suffix) {
			return false
		}
		if len(p.middle) == 0 {
			return true
		}
		str = str[len(p.prefix) : len(str)-len(p.suffix)]
		if p.singleMiddle {
			return indexCaseSensitive(str, p.middle) >= 0
		}
		return matchMiddleLiterals(str, p.middle)
	}
	if p.lastStar < 0 {
		return p.matchNoStar(str, fold)
	}
	tokens := p.tokens
	if p.lastStar < len(tokens)-1 {
		var suffixMatches bool
		str, tokens, suffixMatches = p.trimPatternSuffix(str, tokens, fold)
		if !suffixMatches {
			return false
		}
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
				if !ok {
					return false
				}
				tokenIndex++
				stringIndex = next
				continue
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

func (p *Pattern) matchNoStar(str string, fold bool) bool {
	stringIndex := 0
	for i := range p.tokens {
		tok := &p.tokens[i]
		switch tok.kind {
		case tokenAnyN:
			next, ok := consumeAnyN(str, stringIndex, tok.count)
			if !ok {
				return false
			}
			stringIndex = next
		case tokenLiteralRun:
			next, ok := consumeLiteralRun(str, stringIndex, tok.lit, fold)
			if !ok {
				return false
			}
			stringIndex = next
		default:
			if stringIndex >= len(str) {
				return false
			}
			char, size := decodeRune(str[stringIndex:])
			if !tok.matches(char, fold) {
				return false
			}
			stringIndex += size
		}
	}
	return stringIndex == len(str)
}

func (p *Pattern) trimPatternSuffix(str string, tokens []token, fold bool) (string, []token, bool) {
	starIndex := p.lastStar
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

func (p *Pattern) classifySimpleOrLiteralStars(pattern string) bool {
	firstStar, firstStarEnd, lastStar, lastStarEnd, starRuns, stars, specialIndex := scanPattern(pattern)
	if specialIndex >= 0 {
		return false
	}
	if starRuns == 0 {
		p.prefix = pattern
		p.simple = true
		return true
	}
	if starRuns == 1 {
		p.prefix = pattern[:firstStar]
		p.suffix = pattern[firstStarEnd:]
		p.hasStar = true
		p.simple = true
		return true
	}
	p.prefix = pattern[:firstStar]
	p.middle = pattern[firstStarEnd:lastStar]
	p.suffix = pattern[lastStarEnd:]
	p.minLen = len(pattern) - stars
	p.singleMiddle = starRuns == 2
	p.literalStars = true
	return true
}

func (c *compiledClass) matches(char rune, fold bool) bool {
	if fold {
		return c.matchesFold(char) != c.negated
	}
	return c.contains(char) != c.negated
}

func (t *token) matches(char rune, fold bool) bool {
	switch t.kind {
	case tokenLiteral:
		if fold {
			return runesEqualFold(t.char, char)
		}
		return t.char == char
	case tokenAny:
		return true
	case tokenClass:
		return t.class.matches(char, fold)
	default:
		return false
	}
}
