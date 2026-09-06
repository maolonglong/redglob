package redglob

// stringmatch is kept as a straightforward reference implementation for
// differential tests and fuzzing of the optimized matchers.
func stringmatch(str, pattern string, nocase bool) bool {
	if invalidLongPattern(pattern) {
		return false
	}
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
					if runesMatch(pc, sc, nocase) {
						matched = true
					}
				} else if pc == ']' {
					break
				} else if len(pattern) > ps+1 && pattern[ps] == '-' {
					start := pc
					pattern = pattern[ps+1:]
					pc, ps = decodeRune(pattern)
					end := pc
					if start > end {
						start, end = end, start
					}
					if runeMatchesRange(sc, start, end, nocase) {
						matched = true
					}
				} else {
					if runesMatch(pc, sc, nocase) {
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
			if !runesMatch(pc, sc, nocase) {
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
