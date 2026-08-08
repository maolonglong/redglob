package redglob

import (
	"strings"
	"sync"
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
	// Invalid patterns never match.
	{
		args{"abc", `abc\`},
		false,
	},
	{
		args{"", `\`},
		false,
	},
	{
		args{"a", "[abc"},
		false,
	},
	{
		args{"a", "[a-"},
		false,
	},
	{
		args{"a", `[a\`},
		false,
	},
	// Character-class edge cases.
	{
		args{"m", "[z-a]"},
		true,
	},
	{
		args{"-", `[a\-z]`},
		true,
	},
	{
		args{"b", `[a\-z]`},
		false,
	},
	{
		args{"]", "[]]"},
		false,
	},
	// Collapsed stars and only-star patterns.
	{
		args{"ab", "a**b"},
		true,
	},
	{
		args{"axb", "a**b"},
		true,
	},
	{
		args{"xyz", "***"},
		true,
	},
	{
		args{"", "***"},
		true,
	},
}

// longPad is long enough to enter the len(str) >= 64 multi-star fast path.
var longPad = strings.Repeat("x", 70)

var extendedTests = []struct {
	args args
	want bool
}{
	// Long multi-star literal path (Match + Compile).
	{
		args{"start" + longPad + "mid" + longPad + "end", "start*mid*end"},
		true,
	},
	{
		args{"start" + longPad + "MID" + longPad + "end", "start*mid*end"},
		false,
	},
	{
		args{"start" + longPad + "end", "start*mid*end"},
		false,
	},
	{
		args{"start" + longPad + "mid" + longPad + "end", "start**mid**end"},
		true,
	},
	{
		args{"start" + longPad + "mid" + longPad + "nope", "start*mid*end"},
		false,
	},
	{
		args{longPad + "mid" + longPad, "*mid*"},
		true,
	},
}

var foldTests = []struct {
	str, pattern string
	want         bool
}{
	{"ABC", "abc", true},
	{"abc", "ABC", true},
	{"ABCxyz", "abc*", true},
	{"xyzABC", "*abc", true},
	{"ABCxyzABC", "abc*abc", true},
	{"ABCxyzABD", "abc*abc", false},
	{"xyz", "abc*", false},
	{"xyz", "*abc", false},
	{"xyZabc", "*ABC", true},
	{"a", "[A-Z]", true},
	{"A", "[a-z]", true},
	{"1", "[A-Z]", false},
	{"ß", "SS", false},
	{"k", "K", true},
	{"K", "k", true},
	// Non-ASCII simple fold prefix/suffix paths.
	{"中文ABC", "中文abc*", true},
	{"中文XYZ", "中文abc*", false},
	{"ABC中文", "*abc中文", true},
	{"XYZ中文", "*abc中文", false},
	{"中文ABCtail中文", "中文abc*中文", true},
	{"中文ABCtail日文", "中文abc*中文", false},
}

func allMatchCases() []struct {
	args args
	want bool
} {
	out := make([]struct {
		args args
		want bool
	}, 0, len(tests)+len(extendedTests))
	out = append(out, tests...)
	out = append(out, extendedTests...)
	return out
}

func checkMatchAPIs(t *testing.T, str, pattern string, want bool) {
	t.Helper()
	if got := Match(str, pattern); got != want {
		t.Errorf("Match(%q, %q) = %v, want %v", str, pattern, got, want)
	}
	compiled := Compile(pattern)
	if got := compiled.Match(str); got != want {
		t.Errorf("Compile(%q).Match(%q) = %v, want %v", pattern, str, got, want)
	}
	if got := MatchBytes([]byte(str), pattern); got != want {
		t.Errorf("MatchBytes(%q, %q) = %v, want %v", str, pattern, got, want)
	}
	if got := compiled.MatchBytes([]byte(str)); got != want {
		t.Errorf("Compile(%q).MatchBytes(%q) = %v, want %v", pattern, str, got, want)
	}
}

func checkMatchFoldAPIs(t *testing.T, str, pattern string, want bool) {
	t.Helper()
	if got := MatchFold(str, pattern); got != want {
		t.Errorf("MatchFold(%q, %q) = %v, want %v", str, pattern, got, want)
	}
	compiled := Compile(pattern)
	if got := compiled.MatchFold(str); got != want {
		t.Errorf("Compile(%q).MatchFold(%q) = %v, want %v", pattern, str, got, want)
	}
	if got := MatchBytesFold([]byte(str), pattern); got != want {
		t.Errorf("MatchBytesFold(%q, %q) = %v, want %v", str, pattern, got, want)
	}
	if got := compiled.MatchBytesFold([]byte(str)); got != want {
		t.Errorf("Compile(%q).MatchBytesFold(%q) = %v, want %v", pattern, str, got, want)
	}
}

func TestMatch(t *testing.T) {
	for _, tt := range allMatchCases() {
		checkMatchAPIs(t, tt.args.str, tt.args.pattern, tt.want)
	}
}

func TestMatchFold(t *testing.T) {
	for _, tt := range tests {
		str := strings.ToUpper(tt.args.str)
		checkMatchFoldAPIs(t, str, tt.args.pattern, tt.want)
	}
	for _, tt := range foldTests {
		checkMatchFoldAPIs(t, tt.str, tt.pattern, tt.want)
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
	for _, tt := range allMatchCases() {
		compiled := Compile(tt.args.pattern)
		if got, want := compiled.Match(tt.args.str), Match(tt.args.str, tt.args.pattern); got != want {
			t.Errorf("Compile(%q).Match(%q) = %v, want %v", tt.args.pattern, tt.args.str, got, want)
		}
		if got, want := compiled.MatchFold(tt.args.str), MatchFold(tt.args.str, tt.args.pattern); got != want {
			t.Errorf("Compile(%q).MatchFold(%q) = %v, want %v", tt.args.pattern, tt.args.str, got, want)
		}
		if got, want := compiled.Match(tt.args.str), tt.want; got != want {
			t.Errorf("Compile(%q).Match(%q) = %v, want %v", tt.args.pattern, tt.args.str, got, want)
		}
	}

	var nilPattern *Pattern
	if nilPattern.Match("anything") {
		t.Error("a nil Pattern matched")
	}
	if nilPattern.MatchFold("anything") || nilPattern.MatchBytes(nil) || nilPattern.MatchBytesFold(nil) {
		t.Error("a nil Pattern matched via fold/bytes APIs")
	}

	invalid := string([]byte{0xff})
	if !Match(string([]byte{0xfe}), invalid) || !Compile(invalid).Match(string([]byte{0xfe})) {
		t.Error("invalid UTF-8 bytes should retain RuneError matching semantics")
	}
}

func TestPatternConcurrent(t *testing.T) {
	pattern := Compile("user:[0-9]*:profile")
	const goroutines = 8
	const iterations = 200

	var wg sync.WaitGroup
	errCh := make(chan string, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < iterations; n++ {
				hit := "user:42:profile"
				miss := "admin:42:profile"
				if !pattern.Match(hit) || pattern.Match(miss) {
					errCh <- "unexpected Match result"
					return
				}
				if !pattern.MatchFold(strings.ToUpper(hit)) || pattern.MatchFold(strings.ToUpper(miss)) {
					errCh <- "unexpected MatchFold result"
					return
				}
				if !pattern.MatchBytes([]byte(hit)) || pattern.MatchBytes([]byte(miss)) {
					errCh <- "unexpected MatchBytes result"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
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
	for _, tt := range extendedTests {
		f.Add(tt.args.str, tt.args.pattern)
	}
	for _, tt := range foldTests {
		f.Add(tt.str, tt.pattern)
	}
	f.Add(string([]byte{0xff, 0xfe, 'x'}), string([]byte{'*', 0xfd, 'x'}))
	f.Add("start"+longPad+"mid"+longPad+"end", "start*mid*end")
	f.Add("abc", `abc\`)
	f.Add("a", "[a-")
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
