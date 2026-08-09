// Package redglob implements a simple pattern matcher with Unicode support.
// It provides a Go implementation of Redis's glob-style pattern matching.
package redglob // import "github.com/maolonglong/redglob"

import (
	"strings"
	"unicode"
	"unicode/utf8"
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
	if prefix, suffix, hasStar, ok := splitSimplePattern(pattern); ok {
		if !hasStar {
			return str == prefix
		}
		return matchSimple(str, prefix, suffix)
	}
	if len(str) >= 64 {
		if matches, ok := matchLiteralStars(str, pattern); ok {
			return matches
		}
	}
	if starEnd, suffix, ok := splitFixedSuffix(pattern); ok {
		if !strings.HasSuffix(str, suffix) {
			return false
		}
		return stringmatch(str[:len(str)-len(suffix)], pattern[:starEnd], false)
	}
	return stringmatch(str, pattern, false)
}

// MatchFold is a case-insensitive version of the Match function.
// This function is similar to Match, but it ignores the case of the characters
// in `str` and `pattern` when checking for a match.
func MatchFold(str, pattern string) bool {
	if prefix, suffix, hasStar, ok := splitSimplePattern(pattern); ok {
		if !hasStar {
			return matchLiteralFold(str, prefix)
		}
		return matchSimpleFold(str, prefix, suffix)
	}
	if len(str) >= 64 {
		if matches, ok := matchLiteralStarsFold(str, pattern); ok {
			return matches
		}
	}
	if starEnd, suffix, ok := splitFixedSuffix(pattern); ok {
		suffixStart, matches := matchSuffixFold(str, suffix)
		if !matches {
			return false
		}
		return stringmatch(str[:suffixStart], pattern[:starEnd], true)
	}
	return stringmatch(str, pattern, true)
}

// MatchBytes is similar to Match, but it receives a byteslice instead of a string as input.
// This function converts the byte slice to a string and then calls the Match function.
func MatchBytes(b []byte, pattern string) bool {
	return Match(b2s(b), pattern)
}

// MatchBytesFold is a case-insensitive version of the MatchBytes function.
// This function is similar to MatchBytes, but it ignores the case of the characters
// in the byte slice and `pattern` when checking for a match.
func MatchBytesFold(b []byte, pattern string) bool {
	return MatchFold(b2s(b), pattern)
}

func stringmatch(str, pattern string, nocase bool) bool {
	skipLongerMatches := false
	return stringmatchImpl(str, pattern, nocase, &skipLongerMatches)
}

//gocyclo:ignore
func stringmatchImpl(str, pattern string, nocase bool, skipLongerMatches *bool) bool {
	for len(pattern) > 0 {
		pc, ps := decodeRune(pattern)
		var sc rune
		var ss int
		if len(str) > 0 {
			sc, ss = decodeRune(str)
		}
		switch pc {
		case '*':
			for len(pattern) > 1 && pattern[1] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 1 {
				return true
			}
			for len(str) > 0 {
				if stringmatchImpl(str, pattern[1:], nocase, skipLongerMatches) {
					return true
				}
				if *skipLongerMatches {
					return false
				}
				_, size := decodeRune(str)
				str = str[size:]
			}
			*skipLongerMatches = true
			return false
		case '?':
			if ss == 0 {
				return false
			}
			str = str[ss:]
		case '[':
			if ss == 0 || len(pattern) < 3 {
				return false
			}
			pattern = pattern[1:]
			not := pattern[0] == '^'
			if not {
				pattern = pattern[1:]
			}
			matched := false
			for {
				if len(pattern) == 0 {
					return false
				}
				pc, ps = decodeRune(pattern)
				if pc == '\\' {
					if len(pattern) == 1 {
						return false
					}
					pattern = pattern[ps:]
					pc, ps = decodeRune(pattern)
					if !nocase {
						if pc == sc {
							matched = true
						}
					} else if lowerRune(pc) == lowerRune(sc) {
						matched = true
					}
				} else if pc == ']' {
					break
				} else if len(pattern) > ps+1 && pattern[ps] == '-' {
					start := pc
					pattern = pattern[ps+1:]
					pc, ps = decodeRune(pattern)
					end := pc
					c := sc
					if start > end {
						start, end = end, start
					}
					if nocase {
						start = lowerRune(start)
						end = lowerRune(end)
						c = lowerRune(c)
					}
					if c >= start && c <= end {
						matched = true
					}
				} else {
					if !nocase {
						if pc == sc {
							matched = true
						}
					} else if lowerRune(pc) == lowerRune(sc) {
						matched = true
					}
				}
				pattern = pattern[ps:]
			}
			if not {
				matched = !matched
			}
			if !matched {
				return false
			}
			str = str[ss:]
		case '\\':
			if len(pattern) == 1 {
				return false
			}
			pattern = pattern[1:]
			pc, ps = decodeRune(pattern)
			fallthrough
		default:
			if ss == 0 {
				return false
			}
			if !nocase {
				if pc != sc {
					return false
				}
			} else if lowerRune(pc) != lowerRune(sc) {
				return false
			}
			str = str[ss:]
		}

		pattern = pattern[ps:]
		if len(str) == 0 {
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			break
		}
	}

	return len(pattern) == 0 && len(str) == 0
}

func decodeRune(s string) (rune, int) {
	r, size := rune(s[0]), 1
	if r > unicode.MaxASCII {
		r, size = utf8.DecodeRuneInString(s)
	}
	return r, size
}

func lowerRune(r rune) rune {
	if r <= unicode.MaxASCII {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}
	return unicode.ToLower(r)
}
