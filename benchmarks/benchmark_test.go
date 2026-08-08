package benchmarks

import (
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/gobwas/glob"
	"github.com/maolonglong/redglob"
	tidwall "github.com/tidwall/match"
)

var (
	matchResult     bool
	compileError    error
	compiledGobwas  glob.Glob
	compiledRedglob *redglob.Pattern
)

type matchCase struct {
	name    string
	pattern string
	input   string
	want    bool
}

var commonCases = []matchCase{
	{"LiteralMatch", "customer:1234567890", "customer:1234567890", true},
	{"LiteralMiss", "customer:1234567890", "customer:1234567891", false},
	{"PrefixStar", "customer:*", "customer:1234567890", true},
	{"SuffixStar", "*:profile", "customer:1234567890:profile", true},
	{"InfixStar", "customer:*:profile", "customer:1234567890:profile", true},
	{"QuestionASCII", "file-??.txt", "file-ab.txt", true},
	{"UnicodeStar", "前*後", "前中間後", true},
	{"MultiStar", "a*b*c*d*e", "axbxcxdxe", true},
	{"BacktrackingMiss", "a*a*a*a*b", "aaaaaaaaaaaaaaaaaac", false},
}

var classCases = []matchCase{
	{"Class", "event:[a-z]*", "event:production", true},
	{"ASCIIRange", "file-[a-z].txt", "file-q.txt", true},
	{"DigitRanges", "id-[0-9][0-9]", "id-42", true},
	{"UnicodeRange", "[一-五]", "三", true},
}

func BenchmarkOneShotCommon(b *testing.B) {
	for _, tc := range commonCases {
		b.Run(tc.name, func(b *testing.B) {
			assertMatch(b, redglob.Match(tc.input, tc.pattern), tc.want)
			b.Run("Redglob", func(b *testing.B) {
				for b.Loop() {
					matchResult = redglob.Match(tc.input, tc.pattern)
				}
			})

			assertMatch(b, tidwall.Match(tc.input, tc.pattern), tc.want)
			b.Run("Tidwall", func(b *testing.B) {
				for b.Loop() {
					matchResult = tidwall.Match(tc.input, tc.pattern)
				}
			})

			assertMatch(b, doublestar.MatchUnvalidated(tc.pattern, tc.input), tc.want)
			b.Run("DoublestarUnvalidated", func(b *testing.B) {
				for b.Loop() {
					matchResult = doublestar.MatchUnvalidated(tc.pattern, tc.input)
				}
			})

			stdlibResult, err := path.Match(tc.pattern, tc.input)
			assertMatchError(b, stdlibResult, err, tc.want)
			b.Run("StandardLibrary", func(b *testing.B) {
				for b.Loop() {
					matchResult, compileError = path.Match(tc.pattern, tc.input)
				}
			})

			g, err := glob.Compile(tc.pattern)
			if err != nil {
				b.Fatal(err)
			}
			assertMatch(b, g.Match(tc.input), tc.want)
			b.Run("GobwasCompileAndMatch", func(b *testing.B) {
				for b.Loop() {
					compiledGobwas, compileError = glob.Compile(tc.pattern)
					if compileError == nil {
						matchResult = compiledGobwas.Match(tc.input)
					}
				}
			})
		})
	}
}

func BenchmarkCompiledCommon(b *testing.B) {
	for _, tc := range commonCases {
		b.Run(tc.name, func(b *testing.B) {
			redglobPattern := redglob.Compile(tc.pattern)
			assertMatch(b, redglobPattern.Match(tc.input), tc.want)
			b.Run("Redglob", func(b *testing.B) {
				for b.Loop() {
					matchResult = redglobPattern.Match(tc.input)
				}
			})

			gobwasPattern, err := glob.Compile(tc.pattern)
			if err != nil {
				b.Fatal(err)
			}
			assertMatch(b, gobwasPattern.Match(tc.input), tc.want)
			b.Run("Gobwas", func(b *testing.B) {
				for b.Loop() {
					matchResult = gobwasPattern.Match(tc.input)
				}
			})
		})
	}
}

func BenchmarkCompileCommon(b *testing.B) {
	for _, tc := range commonCases {
		b.Run(tc.name, func(b *testing.B) {
			b.Run("Redglob", func(b *testing.B) {
				for b.Loop() {
					compiledRedglob = redglob.Compile(tc.pattern)
				}
			})
			b.Run("Gobwas", func(b *testing.B) {
				for b.Loop() {
					compiledGobwas, compileError = glob.Compile(tc.pattern)
				}
			})
		})
	}
}

func BenchmarkCharacterClasses(b *testing.B) {
	for _, tc := range classCases {
		b.Run(tc.name, func(b *testing.B) {
			redglobPattern := redglob.Compile(tc.pattern)
			gobwasPattern, err := glob.Compile(tc.pattern)
			if err != nil {
				b.Fatal(err)
			}
			assertMatch(b, redglobPattern.Match(tc.input), tc.want)
			assertMatch(b, gobwasPattern.Match(tc.input), tc.want)
			assertMatch(b, doublestar.MatchUnvalidated(tc.pattern, tc.input), tc.want)
			stdlibResult, err := path.Match(tc.pattern, tc.input)
			assertMatchError(b, stdlibResult, err, tc.want)

			b.Run("RedglobOneShot", func(b *testing.B) {
				for b.Loop() {
					matchResult = redglob.Match(tc.input, tc.pattern)
				}
			})
			b.Run("RedglobCompiled", func(b *testing.B) {
				for b.Loop() {
					matchResult = redglobPattern.Match(tc.input)
				}
			})
			b.Run("GobwasCompiled", func(b *testing.B) {
				for b.Loop() {
					matchResult = gobwasPattern.Match(tc.input)
				}
			})
			b.Run("DoublestarUnvalidated", func(b *testing.B) {
				for b.Loop() {
					matchResult = doublestar.MatchUnvalidated(tc.pattern, tc.input)
				}
			})
			b.Run("StandardLibrary", func(b *testing.B) {
				for b.Loop() {
					matchResult, compileError = path.Match(tc.pattern, tc.input)
				}
			})
		})
	}
}

func BenchmarkMatchFoldASCII(b *testing.B) {
	const pattern = "customer:*:profile"
	const input = "CUSTOMER:1234567890:PROFILE"
	redglobPattern := redglob.Compile(pattern)
	assertMatch(b, redglob.MatchFold(input, pattern), true)
	assertMatch(b, redglobPattern.MatchFold(input), true)
	assertMatch(b, tidwall.MatchNoCase(input, pattern), true)

	b.Run("Redglob", func(b *testing.B) {
		for b.Loop() {
			matchResult = redglob.MatchFold(input, pattern)
		}
	})
	b.Run("RedglobCompiled", func(b *testing.B) {
		for b.Loop() {
			matchResult = redglobPattern.MatchFold(input)
		}
	})
	b.Run("Tidwall", func(b *testing.B) {
		for b.Loop() {
			matchResult = tidwall.MatchNoCase(input, pattern)
		}
	})
}

func BenchmarkMatchFoldASCIILength(b *testing.B) {
	for _, size := range []int{32, 256, 4096} {
		input := strings.Repeat("AbCdEfGh", (size+7)/8)[:size]
		pattern := strings.ToUpper(input)
		compiled := redglob.Compile(pattern)
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.Run("Redglob", func(b *testing.B) {
				for b.Loop() {
					matchResult = redglob.MatchFold(input, pattern)
				}
			})
			b.Run("RedglobCompiled", func(b *testing.B) {
				for b.Loop() {
					matchResult = compiled.MatchFold(input)
				}
			})
			b.Run("Tidwall", func(b *testing.B) {
				for b.Loop() {
					matchResult = tidwall.MatchNoCase(input, pattern)
				}
			})
		})
	}
}

func BenchmarkLongMultiStar(b *testing.B) {
	padding := strings.Repeat("x", 256)
	for _, tc := range []matchCase{
		{"Match", "start*middle*end", "start" + padding + "middle" + padding + "end", true},
		{"Miss", "start*missing*end", "start" + padding + "middle" + padding + "end", false},
	} {
		b.Run(tc.name, func(b *testing.B) {
			redglobPattern := redglob.Compile(tc.pattern)
			gobwasPattern, err := glob.Compile(tc.pattern)
			if err != nil {
				b.Fatal(err)
			}
			b.Run("Redglob", func(b *testing.B) {
				for b.Loop() {
					matchResult = redglob.Match(tc.input, tc.pattern)
				}
			})
			b.Run("Tidwall", func(b *testing.B) {
				for b.Loop() {
					matchResult = tidwall.Match(tc.input, tc.pattern)
				}
			})
			b.Run("RedglobCompiled", func(b *testing.B) {
				for b.Loop() {
					matchResult = redglobPattern.Match(tc.input)
				}
			})
			b.Run("GobwasCompiled", func(b *testing.B) {
				for b.Loop() {
					matchResult = gobwasPattern.Match(tc.input)
				}
			})
			b.Run("DoublestarUnvalidated", func(b *testing.B) {
				for b.Loop() {
					matchResult = doublestar.MatchUnvalidated(tc.pattern, tc.input)
				}
			})
		})
	}
}

func TestBenchmarkSemantics(t *testing.T) {
	for _, tc := range commonCases {
		t.Run(tc.name, func(t *testing.T) {
			assertMatch(t, redglob.Match(tc.input, tc.pattern), tc.want)
			assertMatch(t, redglob.Compile(tc.pattern).Match(tc.input), tc.want)
			assertMatch(t, tidwall.Match(tc.input, tc.pattern), tc.want)
			assertMatch(t, doublestar.MatchUnvalidated(tc.pattern, tc.input), tc.want)
			stdlibResult, err := path.Match(tc.pattern, tc.input)
			assertMatchError(t, stdlibResult, err, tc.want)
			gobwasPattern, err := glob.Compile(tc.pattern)
			if err != nil {
				t.Fatal(err)
			}
			assertMatch(t, gobwasPattern.Match(tc.input), tc.want)
		})
	}

	for _, tc := range classCases {
		t.Run(tc.name, func(t *testing.T) {
			assertMatch(t, redglob.Match(tc.input, tc.pattern), tc.want)
			assertMatch(t, redglob.Compile(tc.pattern).Match(tc.input), tc.want)
			assertMatch(t, doublestar.MatchUnvalidated(tc.pattern, tc.input), tc.want)
			stdlibResult, err := path.Match(tc.pattern, tc.input)
			assertMatchError(t, stdlibResult, err, tc.want)
			gobwasPattern, err := glob.Compile(tc.pattern)
			if err != nil {
				t.Fatal(err)
			}
			assertMatch(t, gobwasPattern.Match(tc.input), tc.want)
		})
	}
}

func BenchmarkUnicodeQuestion(b *testing.B) {
	const pattern = "a?b"
	const input = "a界b"
	redglobPattern := redglob.Compile(pattern)

	assertMatch(b, redglob.Match(input, pattern), true)
	assertMatch(b, redglobPattern.Match(input), true)
	assertMatch(b, tidwall.Match(input, pattern), true)
	assertMatch(b, doublestar.MatchUnvalidated(pattern, input), true)
	stdlibResult, err := path.Match(pattern, input)
	assertMatchError(b, stdlibResult, err, true)

	b.Run("Redglob", func(b *testing.B) {
		for b.Loop() {
			matchResult = redglob.Match(input, pattern)
		}
	})
	b.Run("RedglobCompiled", func(b *testing.B) {
		for b.Loop() {
			matchResult = redglobPattern.Match(input)
		}
	})
	b.Run("Tidwall", func(b *testing.B) {
		for b.Loop() {
			matchResult = tidwall.Match(input, pattern)
		}
	})
	b.Run("DoublestarUnvalidated", func(b *testing.B) {
		for b.Loop() {
			matchResult = doublestar.MatchUnvalidated(pattern, input)
		}
	})
	b.Run("StandardLibrary", func(b *testing.B) {
		for b.Loop() {
			matchResult, compileError = path.Match(pattern, input)
		}
	})
}

func assertMatch(t testing.TB, got, want bool) {
	t.Helper()
	if got != want {
		t.Fatalf("match = %v, want %v", got, want)
	}
}

func assertMatchError(t testing.TB, got bool, err error, want bool) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	assertMatch(t, got, want)
}
