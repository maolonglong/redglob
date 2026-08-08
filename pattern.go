package redglob

import (
	"strings"
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
	kind    tokenKind
	char    rune
	folded  rune
	class   []charRange
	negated bool
}

type tokenKind uint8

const (
	tokenLiteral tokenKind = iota
	tokenAny
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
	p.tokens = make([]token, 0, utf8.RuneCountInString(pattern))
	for len(pattern) > 0 {
		char, size := decodeRune(pattern)
		if char == utf8.RuneError {
			literalOnly = false
		}
		switch char {
		case '*':
			if len(p.tokens) == 0 || p.tokens[len(p.tokens)-1].kind != tokenStar {
				p.tokens = append(p.tokens, token{kind: tokenStar})
				starCount++
			}
		case '?':
			literalOnly = false
			p.tokens = append(p.tokens, token{kind: tokenAny})
		case '[':
			literalOnly = false
			var class token
			class, pattern, p.valid = compileClass(pattern[size:])
			if !p.valid {
				return p
			}
			p.tokens = append(p.tokens, class)
			continue
		case '\\':
			literalOnly = false
			pattern = pattern[size:]
			if len(pattern) == 0 {
				p.valid = false
				return p
			}
			char, size = decodeRune(pattern)
			fallthrough
		default:
			p.tokens = append(p.tokens, token{
				kind:   tokenLiteral,
				char:   char,
				folded: lowerRune(char),
			})
		}
		pattern = pattern[size:]
	}
	if literalOnly && starCount > 1 {
		p.literalStars = true
		p.prefix = originalPattern
	}
	return p
}

func compileClass(pattern string) (token, string, bool) {
	class := token{kind: tokenClass}
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
			pattern = pattern[endSize:]
			continue
		}
		class.class = append(class.class, newCharRange(start, start))
		pattern = pattern[size:]
	}
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
	if !fold && p.literalStars && len(str) >= 64 {
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
	for stringIndex < len(str) {
		char, size := decodeRune(str[stringIndex:])
		if tokenIndex < len(tokens) && tokens[tokenIndex].kind == tokenStar {
			if tokenIndex == len(tokens)-1 {
				return true
			}
			starToken, starString = tokenIndex, stringIndex
			tokenIndex++
			continue
		}
		if tokenIndex < len(tokens) && tokens[tokenIndex].matches(char, fold) {
			tokenIndex++
			stringIndex += size
			continue
		}
		if starToken < 0 {
			return false
		}
		_, size = decodeRune(str[starString:])
		starString += size
		stringIndex = starString
		tokenIndex = starToken + 1
	}
	for tokenIndex < len(tokens) && tokens[tokenIndex].kind == tokenStar {
		tokenIndex++
	}
	return tokenIndex == len(tokens)
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
		if end == 0 {
			return str, tokens, false
		}
		char, size := utf8.DecodeLastRuneInString(str[:end])
		if !tokens[tokenIndex].matches(char, fold) {
			return str, tokens, false
		}
		end -= size
	}
	return str[:end], tokens[:starIndex+1], true
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
	firstStar := strings.IndexByte(pattern, '*')
	lastStar := strings.LastIndexByte(pattern, '*')
	if firstStar < 0 || firstStar == lastStar {
		return false, false
	}
	for _, char := range pattern {
		switch char {
		case utf8.RuneError, '?', '[', '\\':
			return false, false
		}
	}
	return matchLiteralStarsValid(str, pattern), true
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
			char = lowerRune(char)
			for _, r := range t.class {
				if char >= r.foldedStart && char <= r.foldedEnd {
					matched = true
					break
				}
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
