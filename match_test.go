package redglob

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode"

	"github.com/maolonglong/redglob/internal"
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

func BenchmarkMatch(b *testing.B) {
	str := `*?**?**?**?**?**?***?**?**?**?**?*""`
	pat := `*?*?*?*?*?*?**?**?**?**?**?**?**?*""`
	for i := 0; i < b.N; i++ {
		if !Match(str, pat) {
			b.FailNow()
		}
	}
}

func FuzzMatch(f *testing.F) {
	for _, tt := range tests {
		if allow(tt.args.str) && allow(tt.args.pattern) {
			f.Add(tt.args.str, tt.args.pattern)
		}
	}
	f.Fuzz(func(t *testing.T, str, pattern string) {
		if !allow(str) || !allow(pattern) {
			return
		}
		got := Match(str, pattern)
		want := internal.OriginStringMatch(str, pattern)
		if got != want {
			t.Errorf("Match(%q, %q) = %v, want %v", str, pattern, got, want)
		}
	})
}

func allow(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII || s[i] == 0 {
			return false
		}
	}
	return true
}
