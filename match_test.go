package redglob

import (
	"strings"
	"testing"
	"testing/quick"
)

type args struct {
	str     string
	pattern string
}

var tests = []struct {
	args args
	want bool
}{
	{
		args{"", "*"},
		true,
	},
	{
		args{"", "?"},
		false,
	},
	{
		args{"", "["},
		false,
	},
	{
		args{"", ""},
		true,
	},
	{
		args{"a", ""},
		false,
	},
	{
		args{"", "a"},
		false,
	},
	{
		args{"abc", "a[b]c"},
		true,
	},
	{
		args{"abc", "a[\\b]c"},
		true,
	},
	{
		args{"abc", "a[a-z]c"},
		true,
	},
	{
		args{"abc", "a[^db]c"},
		false,
	},
	{
		args{"adc", "a[^db]c"},
		false,
	},
	{
		args{"azc", "a[^db]c"},
		true,
	},
	{
		args{"aac", "a[^a-c]c"},
		false,
	},
	{
		args{"abc", "a[^a-c]c"},
		false,
	},
	{
		args{"acc", "a[^a-c]c"},
		false,
	},
	{
		args{"azc", "a[^a-c]c"},
		true,
	},
	{
		args{"a三c", "a[一-五]c"},
		true,
	},
	{
		args{"abc-🚀-emoji", `a*\-🚀\-em*`},
		true,
	},
	{
		args{"a", "[]"},
		false,
	},
	{
		args{"a", "["},
		false,
	},
	{
		args{"a", "[^]"},
		true,
	},
	{
		args{"abc", "ab*bc"},
		false,
	},
	{
		args{"abbc", "ab*bc"},
		true,
	},
	{
		args{"aaaab", "a*a*b"},
		true,
	},
	{
		args{"aaaac", "a*a*b"},
		false,
	},
}

func TestMatch(t *testing.T) {
	for _, tt := range tests {
		if got := Match(tt.args.str, tt.args.pattern); got != tt.want {
			t.Errorf(
				"Match(%q, %q) = %v, want %v",
				tt.args.str,
				tt.args.pattern,
				got,
				tt.want,
			)
		}
	}
}

func TestMatchFold(t *testing.T) {
	for _, tt := range tests {
		str := strings.ToUpper(tt.args.str)
		if got := MatchFold(str, tt.args.pattern); got != tt.want {
			t.Errorf(
				"MatchFold(%q, %q) = %v, want %v",
				str,
				tt.args.pattern,
				got,
				tt.want,
			)
		}
	}
}

func TestMatchBytes(t *testing.T) {
	f := func(b []byte, pattern string) bool {
		return Match(string(b), pattern)
	}
	g := MatchBytes
	if err := quick.CheckEqual(f, g, nil); err != nil {
		t.Error(err)
	}
}

func TestMatchBytesFold(t *testing.T) {
	f := func(b []byte, pattern string) bool {
		return MatchFold(string(b), pattern)
	}
	g := MatchBytesFold
	if err := quick.CheckEqual(f, g, nil); err != nil {
		t.Error(err)
	}
}

func TestCompile(t *testing.T) {
	for _, tt := range tests {
		compiled := Compile(tt.args.pattern)
		if got := compiled.Match(tt.args.str); got != Match(tt.args.str, tt.args.pattern) {
			t.Errorf("Compile(%q).Match(%q) = %v", tt.args.pattern, tt.args.str, got)
		}
		if got := compiled.MatchFold(tt.args.str); got != MatchFold(tt.args.str, tt.args.pattern) {
			t.Errorf("Compile(%q).MatchFold(%q) = %v", tt.args.pattern, tt.args.str, got)
		}
	}

	var nilPattern *Pattern
	if nilPattern.Match("anything") {
		t.Error("a nil Pattern matched")
	}

	invalid := string([]byte{0xff})
	if !Match(string([]byte{0xfe}), invalid) || !Compile(invalid).Match(string([]byte{0xfe})) {
		t.Error("invalid UTF-8 bytes should retain RuneError matching semantics")
	}
}

func TestOptimizedSuffixEdgeCases(t *testing.T) {
	foldCases := []struct {
		str, pattern string
		want         bool
	}{
		{"k", "*K", true},
		{"K", "*k", true},
		{"kk", "k*K", true},
		{"k", "k*K", false},
	}
	for _, tt := range foldCases {
		if got := MatchFold(tt.str, tt.pattern); got != tt.want {
			t.Errorf("MatchFold(%q, %q) = %v, want %v", tt.str, tt.pattern, got, tt.want)
		}
		if got := Compile(tt.pattern).MatchFold(tt.str); got != tt.want {
			t.Errorf("Compile(%q).MatchFold(%q) = %v, want %v", tt.pattern, tt.str, got, tt.want)
		}
	}

	str := string([]byte{'x', 0xfe})
	pattern := string([]byte{'*', 0xff})
	if !Match(str, pattern) || !Compile(pattern).Match(str) {
		t.Error("optimized suffix matching changed invalid UTF-8 semantics")
	}

	longASCII := strings.Repeat("AbCdEfGh", 32)
	longFoldCases := []struct {
		str, pattern string
		want         bool
	}{
		{strings.ToUpper(longASCII), strings.ToLower(longASCII), true},
		{longASCII[:128] + "x" + longASCII[129:], longASCII, false},
		{longASCII + "k", strings.ToLower(longASCII) + "K", true},
		{longASCII + string([]byte{0xfe}), strings.ToLower(longASCII) + string([]byte{0xff}), true},
	}
	for _, tt := range longFoldCases {
		if got := MatchFold(tt.str, tt.pattern); got != tt.want {
			t.Errorf("MatchFold(long input, long pattern) = %v, want %v", got, tt.want)
		}
		if got := Compile(tt.pattern).MatchFold(tt.str); got != tt.want {
			t.Errorf("Compile(long pattern).MatchFold(long input) = %v, want %v", got, tt.want)
		}
	}

	malformedMultiStar := string([]byte{'*', 0xc6, '*'})
	malformedInput := strings.Repeat("0", 64) + string([]byte{'H', 0xfc, 0xbc})
	if got, want := Compile(malformedMultiStar).Match(malformedInput), stringmatch(malformedInput, malformedMultiStar, false); got != want {
		t.Errorf("Compile(malformed multi-star).Match(malformed input) = %v, want %v", got, want)
	}
}

func FuzzCompile(f *testing.F) {
	for _, tt := range tests {
		f.Add(tt.args.str, tt.args.pattern)
	}
	f.Add(string([]byte{0xff, 0xfe, 'x'}), string([]byte{'*', 0xfd, 'x'}))
	f.Fuzz(func(t *testing.T, str, pattern string) {
		compiled := Compile(pattern)
		if got, want := compiled.Match(str), stringmatch(str, pattern, false); got != want {
			t.Errorf("Compile(%q).Match(%q) = %v, want %v", pattern, str, got, want)
		}
		if got, want := compiled.MatchFold(str), stringmatch(str, pattern, true); got != want {
			t.Errorf("Compile(%q).MatchFold(%q) = %v, want %v", pattern, str, got, want)
		}
		if got, want := compiled.MatchBytes([]byte(str)), stringmatch(str, pattern, false); got != want {
			t.Errorf("Compile(%q).MatchBytes(%q) = %v, want %v", pattern, str, got, want)
		}
		if got, want := compiled.MatchBytesFold([]byte(str)), stringmatch(str, pattern, true); got != want {
			t.Errorf("Compile(%q).MatchBytesFold(%q) = %v, want %v", pattern, str, got, want)
		}
		if got, want := Match(str, pattern), stringmatch(str, pattern, false); got != want {
			t.Errorf("Match(%q, %q) = %v, want %v", str, pattern, got, want)
		}
		if got, want := MatchFold(str, pattern), stringmatch(str, pattern, true); got != want {
			t.Errorf("MatchFold(%q, %q) = %v, want %v", str, pattern, got, want)
		}
		if got, want := MatchBytes([]byte(str), pattern), stringmatch(str, pattern, false); got != want {
			t.Errorf("MatchBytes(%q, %q) = %v, want %v", str, pattern, got, want)
		}
		if got, want := MatchBytesFold([]byte(str), pattern), stringmatch(str, pattern, true); got != want {
			t.Errorf("MatchBytesFold(%q, %q) = %v, want %v", str, pattern, got, want)
		}
	})
}

var benchmarkCases = []struct {
	name, str, pattern string
	want               bool
}{
	{"LiteralASCII", "customer:1234567890", "customer:1234567890", true},
	{"LiteralMismatch", "customer:1234567891", "customer:1234567890", false},
	{"LiteralUnicode", "用户:北京:一二三", "用户:北京:一二三", true},
	{"Wildcards", "customer:1234567890:profile", "customer:*:profile", true},
	{"CharacterClasses", "event:production:42", "event:[a-z]*:[0-9][0-9]", true},
	{
		"Backtracking",
		`*?**?**?**?**?**?***?**?**?**?**?*""`,
		`*?*?*?*?*?*?**?**?**?**?**?**?**?*""`,
		true,
	},
}

func BenchmarkRepeatedMatch(b *testing.B) {
	for _, tt := range benchmarkCases {
		b.Run(tt.name, func(b *testing.B) {
			compiled := Compile(tt.pattern)
			b.Run("Direct", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if got := Match(tt.str, tt.pattern); got != tt.want {
						b.Fatalf("Match() = %v, want %v", got, tt.want)
					}
				}
			})
			b.Run("Compiled", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if got := compiled.Match(tt.str); got != tt.want {
						b.Fatalf("Pattern.Match() = %v, want %v", got, tt.want)
					}
				}
			})
		})
	}
}

func BenchmarkCompile(b *testing.B) {
	for _, tt := range benchmarkCases {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Compile(tt.pattern)
			}
		})
	}
}

func BenchmarkRepeatedMatchFold(b *testing.B) {
	for _, tt := range benchmarkCases {
		str := strings.ToUpper(tt.str)
		b.Run(tt.name, func(b *testing.B) {
			compiled := Compile(tt.pattern)
			b.Run("Direct", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if got := MatchFold(str, tt.pattern); got != tt.want {
						b.Fatalf("MatchFold() = %v, want %v", got, tt.want)
					}
				}
			})
			b.Run("Compiled", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if got := compiled.MatchFold(str); got != tt.want {
						b.Fatalf("Pattern.MatchFold() = %v, want %v", got, tt.want)
					}
				}
			})
		})
	}
}
