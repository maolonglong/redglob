// Copyright 2023 Shaolong Chen <shaolong.chen@outlook.it>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package redglob provides a simple pattern matcher with unicode support. (Go implementation of Redis's glob-style pattern matching)
package redglob // import "go.chensl.me/redglob"

import (
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// Match returns true if str matches pattern. This is a very
// simple wildcard match where '*' matches on any number characters
// and '?' matches on any one character.
//
// pattern:
//
//	{ term }
//
// term:
//
//	'*'         matches any sequence of non-Separator characters
//	'?'         matches any single non-Separator character
//	c           matches character c (c != '*', '?', '\\')
//	'\\' c      matches character c
//	'[abc]'     matches 'a' or 'b' or 'c'
//	'[a-z]'     matches 'a' or 'b' or 'c' ... or 'z'
//	'[^abc]'    not matches 'a' or 'b' or 'c'
//	'[^a-z]'    not matches 'a' or 'b' or 'c' ... or 'z'
func Match(str, pattern string) bool {
	return stringmatch(str, pattern, false)
}

// MatchFold is case-insensitive Match.
func MatchFold(str, pattern string) bool {
	return stringmatch(str, pattern, true)
}

// MatchBytes is the same as Match, but it receives bytes.
func MatchBytes(b []byte, pattern string) bool {
	return Match(unsafe.String(unsafe.SliceData(b), len(b)), pattern)
}

// MatchBytesFold is case-insensitive MatchBytes.
func MatchBytesFold(b []byte, pattern string) bool {
	return MatchFold(unsafe.String(unsafe.SliceData(b), len(b)), pattern)
}

func stringmatch(str, pattern string, nocase bool) bool {
	skipLongerMatches := false
	return stringmatch_impl(str, pattern, nocase, &skipLongerMatches)
}

func stringmatch_impl(str, pattern string, nocase bool, skipLongerMatches *bool) bool {
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
				if stringmatch_impl(str, pattern[1:], nocase, skipLongerMatches) {
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
			if ss == 0 || len(pattern) == 1 {
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
					break
				}
				pc, ps = decodeRune(pattern)
				if pc == '\\' && len(pattern) > 1 {
					pattern = pattern[ps:]
					pc, ps = decodeRune(pattern)
					if !nocase {
						if pc == sc {
							matched = true
						}
					} else if unicode.ToLower(pc) == unicode.ToLower(sc) {
						matched = true
					}
				} else if pc == ']' {
					break
				} else if utf8.RuneCountInString(pattern) >= 3 && pattern[ps] == '-' {
					start := pc
					pattern = pattern[ps+1:]
					pc, ps = decodeRune(pattern)
					end := pc
					c := sc
					if start > end {
						tmp := start
						start = end
						end = tmp
					}
					if nocase {
						start = unicode.ToLower(start)
						end = unicode.ToLower(end)
						c = unicode.ToLower(c)
					}
					if c >= start && c <= end {
						matched = true
					}
				} else {
					if !nocase {
						if pc == sc {
							matched = true
						}
					} else if unicode.ToLower(pc) == unicode.ToLower(sc) {
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
			if len(pattern) > 1 {
				pattern = pattern[1:]
				pc, ps = decodeRune(pattern)
			}
			fallthrough
		default:
			if ss == 0 {
				return false
			}
			if !nocase {
				if pc != sc {
					return false
				}
			} else if unicode.ToLower(pc) != unicode.ToLower(sc) {
				return false
			}
			str = str[ss:]
		}

		if len(pattern) > 0 {
			pattern = pattern[ps:]
		}
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
