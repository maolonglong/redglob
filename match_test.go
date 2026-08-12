package redglob

import (
	"fmt"
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
	{"KkZ", "*k?Z", true},
	{"ς", "Σ", true},
	{"ſ", "S", true},
	{"İ", "i", false},
	{"ς", "[Σ]", true},
	{"ſ", "[A-Z]", true},
	{"İ", "[A-Z]", false},
	{"A", "[b-C]", true},
	// Non-ASCII simple fold prefix/suffix paths.
	{"中文ABC", "中文abc*", true},
	{"中文XYZ", "中文abc*", false},
	{"ABC中文", "*abc中文", true},
	{"XYZ中文", "*abc中文", false},
	{"中文ABCtail中文", "中文abc*中文", true},
	{"中文ABCtail日文", "中文abc*中文", false},
	// Long multi-star fold path (len(str) >= 64).
	{"START" + longPad + "MID" + longPad + "END", "start*mid*end", true},
	{"START" + longPad + "MID" + longPad + "END", "start**mid**end", true},
	{"START" + longPad + "MAD" + longPad + "END", "start*mid*end", false},
	{"START" + longPad + "MID" + longPad + "ENO", "start*mid*end", false},
	{longPad + "MID" + longPad, "*mid*", true},
	{longPad + "KAkZ", "*k*A*Z", true},
	// Overlapping segment placement under fold.
	{"AA" + longPad + "AABB" + longPad + "BB", "aa*aabb*bb", true},
	// Kelvin sign (U+212A) folds to 'k'; forces non-ASCII haystack path.
	{"START" + longPad + "K" + longPad + "END", "start*k*end", true},
	{"START" + longPad + "X" + longPad + "END", "start*k*end", false},
	// Non-ASCII literal segments under multi-star fold.
	{"前" + longPad + "中" + longPad + "後", "前*中*後", true},
	{"前" + longPad + "中" + longPad + "后", "前*中*後", false},
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

func TestCompileBoundsInitialTokenCapacity(t *testing.T) {
	pattern := `\` + strings.Repeat("a", 4096)
	compiled := Compile(pattern)
	if len(compiled.tokens) != 1 || compiled.tokens[0].kind != tokenLiteralRun {
		t.Fatalf("Compile(long escaped literal) tokens = %v, want one literal run", tokenKinds(compiled.tokens))
	}
	if capacity := cap(compiled.tokens); capacity > 32 {
		t.Fatalf("Compile(long escaped literal) token capacity = %d, want at most 32", capacity)
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

func TestCompiledTokenShapes(t *testing.T) {
	cases := []struct {
		pattern string
		kinds   []tokenKind
		// optional checks
		anyN  int
		lit   string
		class bool
	}{
		{"????", []tokenKind{tokenAnyN}, 4, "", false},
		{"??", []tokenKind{tokenAnyN}, 2, "", false},
		{"?", []tokenKind{tokenAny}, 0, "", false},
		{"file-??.txt", []tokenKind{tokenLiteralRun, tokenAnyN, tokenLiteralRun}, 2, "file-", false},
		{"hello", []tokenKind{}, 0, "", false}, // simple
		{`hello\*world`, []tokenKind{tokenLiteralRun}, 0, "hello*world", false},
		{"[a-z]", []tokenKind{tokenClass}, 0, "", true},
		{"user:[0-9]*", []tokenKind{tokenLiteralRun, tokenClass, tokenStar}, 0, "user:", true},
		{"a*b*c", []tokenKind{}, 0, "", false}, // literal-star fast path
	}
	for _, tt := range cases {
		p := Compile(tt.pattern)
		if tt.pattern == "a*b*c" {
			if !p.literalStars || len(p.tokens) != 0 {
				t.Errorf("Compile(%q) did not use token-free literal-star path", tt.pattern)
			}
			continue
		}
		if p.simple {
			if len(tt.kinds) != 0 {
				t.Errorf("Compile(%q) unexpectedly simple", tt.pattern)
			}
			continue
		}
		if len(p.tokens) != len(tt.kinds) {
			t.Errorf("Compile(%q) tokens=%d kinds=%v, want %d %v", tt.pattern, len(p.tokens), tokenKinds(p.tokens), len(tt.kinds), tt.kinds)
			continue
		}
		for i, k := range tt.kinds {
			if p.tokens[i].kind != k {
				t.Errorf("Compile(%q) token[%d]=%v, want %v (all=%v)", tt.pattern, i, p.tokens[i].kind, k, tokenKinds(p.tokens))
			}
		}
		if tt.anyN > 0 {
			found := false
			for _, tok := range p.tokens {
				if tok.kind == tokenAnyN && tok.count == tt.anyN {
					found = true
				}
			}
			if !found {
				t.Errorf("Compile(%q) missing AnyN count=%d", tt.pattern, tt.anyN)
			}
		}
		if tt.lit != "" {
			found := false
			for _, tok := range p.tokens {
				if tok.kind == tokenLiteralRun && tok.lit == tt.lit {
					found = true
				}
			}
			if !found && len(tt.lit) != 1 {
				// also accept if pattern fully became one run
				if len(p.tokens) == 1 && p.tokens[0].kind == tokenLiteralRun && p.tokens[0].lit == tt.lit {
					found = true
				}
			}
			if !found {
				t.Errorf("Compile(%q) missing literal run %q; tokens=%v", tt.pattern, tt.lit, dumpTokens(p.tokens))
			}
		}
		if tt.class {
			tok := p.tokens[0]
			for _, t2 := range p.tokens {
				if t2.kind == tokenClass {
					tok = t2
					break
				}
			}
			if tok.kind != tokenClass {
				t.Errorf("Compile(%q) missing class", tt.pattern)
			} else if tt.pattern == "[a-z]" && !asciiBit(tok.class.bits, 'a') {
				t.Errorf("Compile([a-z]) bitset missing 'a'")
			}
		}
	}
}

func TestOptimizedMatcherEdges(t *testing.T) {
	for _, pattern := range []string{"*[", "**[abc", `*abc\`, "***[a-"} {
		checkMatchAPIs(t, "abc", pattern, false)
		checkMatchFoldAPIs(t, "ABC", pattern, false)
	}

	for _, tt := range []struct {
		str, pattern string
		want         bool
	}{
		{"abcd", "????", true},
		{"a三🚀", "???", true},
		{"a三🚀", "????", false},
		{string([]byte{0xff, 0xfe, 0xfd}), "???", true},
		{"prefix四字尾", "*????尾", true},
		{"三字尾", "*????尾", false},
		{"界", "[界🚀]", true},
		{"🚀", "[界🚀]", true},
		{"文", "[界🚀]", false},
		{"k", "[K]", true},
		{"K", "[k]", true},
	} {
		if tt.pattern == "[K]" || tt.pattern == "[k]" {
			checkMatchFoldAPIs(t, tt.str, tt.pattern, tt.want)
			continue
		}
		checkMatchAPIs(t, tt.str, tt.pattern, tt.want)
	}
}

func TestOneShotManyStarsDoesNotRecurse(t *testing.T) {
	const count = 10_000
	str := strings.Repeat("a", count)
	pattern := strings.Repeat("*?", count)
	if !Match(str, pattern) || !MatchFold(str, pattern) {
		t.Fatal("one-shot iterative matcher failed deep star pattern")
	}
}

func TestOneShotComplexMatchAllocations(t *testing.T) {
	if allocs := testing.AllocsPerRun(100, func() {
		Match("event:production:42", "event:[a-z]*:[0-9][0-9]")
		MatchFold("EVENT:PRODUCTION:42", "event:[a-z]*:[0-9][0-9]")
	}); allocs != 0 {
		t.Fatalf("one-shot complex matching allocated %v times, want 0", allocs)
	}
}

func TestStarLiteralSearch(t *testing.T) {
	literal := strings.Repeat("a", 256) + "b"
	pattern := "*" + literal + "*?"
	checkMatchAPIs(t, "prefix"+literal+"z", pattern, true)
	checkMatchAPIs(t, strings.Repeat("a", 1024)+"x", pattern, false)

	foldPattern := "*" + strings.ToUpper(literal) + "*?"
	checkMatchFoldAPIs(t, "prefix"+literal+"z", foldPattern, true)
	checkMatchFoldAPIs(t, strings.Repeat("a", 1024)+"x", foldPattern, false)
}

func tokenKinds(tokens []token) []tokenKind {
	out := make([]tokenKind, len(tokens))
	for i, tok := range tokens {
		out[i] = tok.kind
	}
	return out
}

func dumpTokens(tokens []token) string {
	parts := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		parts = append(parts, fmt.Sprintf("%d:%q/%d", tok.kind, tok.lit, tok.count))
	}
	return strings.Join(parts, ",")
}

func TestLiteralStarsFoldFastPath(t *testing.T) {
	// Below the 64-byte threshold the multi-star fold fast path is skipped.
	short := "STARTmidEND"
	if got, want := MatchFold(short, "start*mid*end"), stringmatch(short, "start*mid*end", true); got != want {
		t.Errorf("short MatchFold = %v, want %v", got, want)
	}

	cases := []struct {
		str, pattern string
		want         bool
	}{
		{"START" + longPad + "MID" + longPad + "END", "start*mid*end", true},
		{"start" + longPad + "mid" + longPad + "end", "START*MID*END", true},
		{"START" + longPad + "MID" + longPad + "END", "start*missing*end", false},
		// First segment match must leave room for later segments.
		{"X" + longPad + "MIDMID" + longPad + "END", "x*mid*mid*end", true},
		{"X" + longPad + "MID" + longPad + "END", "x*mid*mid*end", false},
		// Empty middle segments via collapsed stars.
		{"AA" + longPad + "BB", "aa***bb", true},
		// Prefix/suffix only multi-star.
		{longPad + "MID" + longPad, "*mid*", true},
		{longPad + "MAD" + longPad, "*mid*", false},
		// Non-ASCII fold of Kelvin inside a long haystack.
		{longPad + "KELVIN" + longPad, "*kelvin*", true},
		{longPad + "KELVIN" + longPad, "*Kelvin*", true},
		// Invalid UTF-8 must stay equivalent to the reference matcher.
		{strings.Repeat("A", 64) + string([]byte{0xfe}) + "Z", "a*" + string([]byte{0xff}) + "*z", true},
	}
	for _, tt := range cases {
		want := tt.want
		if ref := stringmatch(tt.str, tt.pattern, true); ref != want {
			t.Fatalf("test table disagree with stringmatch for %q ~ %q: table=%v ref=%v", tt.str, tt.pattern, want, ref)
		}
		checkMatchFoldAPIs(t, tt.str, tt.pattern, want)
		// Sensitive multi-star path must remain unchanged for pure lower input.
		lowerStr := strings.ToLower(tt.str)
		lowerPat := strings.ToLower(tt.pattern)
		if isASCII(lowerStr) && isASCII(lowerPat) {
			checkMatchAPIs(t, lowerStr, lowerPat, want)
		}
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
	f.Add("START"+longPad+"MID"+longPad+"END", "start*mid*end")
	f.Add(longPad+"K"+longPad, "*k*")
	f.Add("前"+longPad+"中"+longPad+"後", "前*中*後")
	f.Add("abc", `abc\`)
	f.Add("a", "[a-")
	f.Fuzz(func(t *testing.T, str, pattern string) {
		compiled := Compile(pattern)
		want := stringmatch(str, pattern, false)
		wantFold := stringmatch(str, pattern, true)
		if got := compiled.Match(str); got != want {
			t.Errorf("Compile(%q).Match(%q) = %v, want %v", pattern, str, got, want)
		}
		if got := compiled.MatchFold(str); got != wantFold {
			t.Errorf("Compile(%q).MatchFold(%q) = %v, want %v", pattern, str, got, wantFold)
		}
		if got := compiled.MatchBytes([]byte(str)); got != want {
			t.Errorf("Compile(%q).MatchBytes(%q) = %v, want %v", pattern, str, got, want)
		}
		if got := compiled.MatchBytesFold([]byte(str)); got != wantFold {
			t.Errorf("Compile(%q).MatchBytesFold(%q) = %v, want %v", pattern, str, got, wantFold)
		}
		if got := Match(str, pattern); got != want {
			t.Errorf("Match(%q, %q) = %v, want %v", str, pattern, got, want)
		}
		if got := MatchFold(str, pattern); got != wantFold {
			t.Errorf("MatchFold(%q, %q) = %v, want %v", str, pattern, got, wantFold)
		}
		if got := MatchBytes([]byte(str), pattern); got != want {
			t.Errorf("MatchBytes(%q, %q) = %v, want %v", str, pattern, got, want)
		}
		if got := MatchBytesFold([]byte(str), pattern); got != wantFold {
			t.Errorf("MatchBytesFold(%q, %q) = %v, want %v", str, pattern, got, wantFold)
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

func BenchmarkLiteralStarsFold(b *testing.B) {
	pad := strings.Repeat("x", 200)
	cases := []struct {
		name, str, pattern string
		want               bool
	}{
		{"Hit", "START" + pad + "MID" + pad + "END", "start*mid*end", true},
		{"Miss", "START" + pad + "MAD" + pad + "END", "start*mid*end", false},
		{"SensitiveHit", "start" + pad + "mid" + pad + "end", "start*mid*end", true},
	}
	for _, tt := range cases {
		b.Run(tt.name, func(b *testing.B) {
			compiled := Compile(tt.pattern)
			b.Run("DirectFold", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if got := MatchFold(tt.str, tt.pattern); got != tt.want {
						b.Fatalf("MatchFold() = %v, want %v", got, tt.want)
					}
				}
			})
			b.Run("CompiledFold", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if got := compiled.MatchFold(tt.str); got != tt.want {
						b.Fatalf("Pattern.MatchFold() = %v, want %v", got, tt.want)
					}
				}
			})
			if tt.name == "SensitiveHit" {
				b.Run("Direct", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						if got := Match(tt.str, tt.pattern); got != tt.want {
							b.Fatalf("Match() = %v, want %v", got, tt.want)
						}
					}
				})
				b.Run("Compiled", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						if got := compiled.Match(tt.str); got != tt.want {
							b.Fatalf("Pattern.Match() = %v, want %v", got, tt.want)
						}
					}
				})
			}
		})
	}
}

func BenchmarkOneShotQuestionRuns(b *testing.B) {
	for _, count := range []int{16, 64} {
		str := strings.Repeat("a", count)
		pattern := strings.Repeat("?", count)
		b.Run(fmt.Sprintf("ASCII%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if !Match(str, pattern) {
					b.Fatal("question run did not match")
				}
			}
		})
	}
}

func BenchmarkStarLiteralMiss(b *testing.B) {
	const size = 16_000
	str := strings.Repeat("a", size) + "x"
	pattern := "*" + strings.Repeat("a", size/2) + "b*?"
	compiled := Compile(pattern)

	b.Run("Direct", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if Match(str, pattern) {
				b.Fatal("unexpected match")
			}
		}
	})
	b.Run("Compiled", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if compiled.Match(str) {
				b.Fatal("unexpected match")
			}
		}
	})
	b.Run("DirectFold", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if MatchFold(str, pattern) {
				b.Fatal("unexpected match")
			}
		}
	})
	b.Run("CompiledFold", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if compiled.MatchFold(str) {
				b.Fatal("unexpected match")
			}
		}
	})
}
