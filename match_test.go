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

package redglob

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode"

	"go.chensl.me/redglob/internal"
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
		args{"a三c", "a[一-五]c"},
		true,
	},
	{
		args{"abc-🚀-emoji", `a*\-🚀\-em*`},
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
		want := internal.CGO_stringmatch(str, pattern)
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
	return !strings.Contains(s, "[^]")
}
