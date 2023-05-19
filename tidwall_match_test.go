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

// Modified from https://github.com/tidwall/match/blob/ec90e00ea409c4ce4a8ffb0827280810d17c96c8/match_test.go

package redglob

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestMatch_tidwall(t *testing.T) {
	if !Match("hello world", "hello world") {
		t.Fatal("fail")
	}
	if Match("hello world", "jello world") {
		t.Fatal("fail")
	}
	if !Match("hello world", "hello*") {
		t.Fatal("fail")
	}
	if Match("hello world", "jello*") {
		t.Fatal("fail")
	}
	if !Match("hello world", "hello?world") {
		t.Fatal("fail")
	}
	if Match("hello world", "jello?world") {
		t.Fatal("fail")
	}
	if !Match("hello world", "he*o?world") {
		t.Fatal("fail")
	}
	if !Match("hello world", "he*o?wor*") {
		t.Fatal("fail")
	}
	if !Match("hello world", "he*o?*r*") {
		t.Fatal("fail")
	}
	if !Match("hello*world", `hello\*world`) {
		t.Fatal("fail")
	}
	if !Match("he解lo*world", `he解lo\*world`) {
		t.Fatal("fail")
	}
	if !Match("的情况下解析一个", "*") {
		t.Fatal("fail")
	}
	if !Match("的情况下解析一个", "*况下*") {
		t.Fatal("fail")
	}
	if !Match("的情况下解析一个", "*况?*") {
		t.Fatal("fail")
	}
	if !Match("的情况下解析一个", "的情况?解析一个") {
		t.Fatal("fail")
	}
	if Match("hello world\\", "hello world\\") {
		t.Fatal("fail")
	}
}

// TestWildcardMatch - Tests validate the logic of wild card matching.
// `WildcardMatch` supports '*' and '?' wildcards.
// Sample usage: In resource matching for folder policy validation.
func TestWildcardMatch_tidwall(t *testing.T) {
	testCases := []struct {
		pattern string
		text    string
		matched bool
	}{
		// Test case - 1.
		// Test case with pattern containing key name with a prefix. Should accept the same text without a "*".
		{
			pattern: "my-folder/oo*",
			text:    "my-folder/oo",
			matched: true,
		},
		// Test case - 2.
		// Test case with "*" at the end of the pattern.
		{
			pattern: "my-folder/In*",
			text:    "my-folder/India/Karnataka/",
			matched: true,
		},
		// Test case - 3.
		// Test case with prefixes shuffled.
		// This should fail.
		{
			pattern: "my-folder/In*",
			text:    "my-folder/Karnataka/India/",
			matched: false,
		},
		// Test case - 4.
		// Test case with text expanded to the wildcards in the pattern.
		{
			pattern: "my-folder/In*/Ka*/Ban",
			text:    "my-folder/India/Karnataka/Ban",
			matched: true,
		},
		// Test case - 5.
		// Test case with the  keyname part is repeated as prefix several times.
		// This is valid.
		{
			pattern: "my-folder/In*/Ka*/Ban",
			text:    "my-folder/India/Karnataka/Ban/Ban/Ban/Ban/Ban",
			matched: true,
		},
		// Test case - 6.
		// Test case to validate that `*` can be expanded into multiple prefixes.
		{
			pattern: "my-folder/In*/Ka*/Ban",
			text:    "my-folder/India/Karnataka/Area1/Area2/Area3/Ban",
			matched: true,
		},
		// Test case - 7.
		// Test case to validate that `*` can be expanded into multiple prefixes.
		{
			pattern: "my-folder/In*/Ka*/Ban",
			text:    "my-folder/India/State1/State2/Karnataka/Area1/Area2/Area3/Ban",
			matched: true,
		},
		// Test case - 8.
		// Test case where the keyname part of the pattern is expanded in the text.
		{
			pattern: "my-folder/In*/Ka*/Ban",
			text:    "my-folder/India/Karnataka/Bangalore",
			matched: false,
		},
		// Test case - 9.
		// Test case with prefixes and wildcard expanded for all "*".
		{
			pattern: "my-folder/In*/Ka*/Ban*",
			text:    "my-folder/India/Karnataka/Bangalore",
			matched: true,
		},
		// Test case - 10.
		// Test case with keyname part being a wildcard in the pattern.
		{
			pattern: "my-folder/*",
			text:    "my-folder/India",
			matched: true,
		},
		// Test case - 11.
		{
			pattern: "my-folder/oo*",
			text:    "my-folder/odo",
			matched: false,
		},

		// Test case with pattern containing wildcard '?'.
		// Test case - 12.
		// "my-folder?/" matches "my-folder1/", "my-folder2/", "my-folder3" etc...
		// doesn't match "myfolder/".
		{
			pattern: "my-folder?/abc*",
			text:    "myfolder/abc",
			matched: false,
		},
		// Test case - 13.
		{
			pattern: "my-folder?/abc*",
			text:    "my-folder1/abc",
			matched: true,
		},
		// Test case - 14.
		{
			pattern: "my-?-folder/abc*",
			text:    "my--folder/abc",
			matched: false,
		},
		// Test case - 15.
		{
			pattern: "my-?-folder/abc*",
			text:    "my-1-folder/abc",
			matched: true,
		},
		// Test case - 16.
		{
			pattern: "my-?-folder/abc*",
			text:    "my-k-folder/abc",
			matched: true,
		},
		// Test case - 17.
		{
			pattern: "my??folder/abc*",
			text:    "myfolder/abc",
			matched: false,
		},
		// Test case - 18.
		{
			pattern: "my??folder/abc*",
			text:    "my4afolder/abc",
			matched: true,
		},
		// Test case - 19.
		{
			pattern: "my-folder?abc*",
			text:    "my-folder/abc",
			matched: true,
		},
		// Test case 20-21.
		// '?' matches '/' too. (works with s3).
		// This is because the namespace is considered flat.
		// "abc?efg" matches both "abcdefg" and "abc/efg".
		{
			pattern: "my-folder/abc?efg",
			text:    "my-folder/abcdefg",
			matched: true,
		},
		{
			pattern: "my-folder/abc?efg",
			text:    "my-folder/abc/efg",
			matched: true,
		},
		// Test case - 22.
		{
			pattern: "my-folder/abc????",
			text:    "my-folder/abc",
			matched: false,
		},
		// Test case - 23.
		{
			pattern: "my-folder/abc????",
			text:    "my-folder/abcde",
			matched: false,
		},
		// Test case - 24.
		{
			pattern: "my-folder/abc????",
			text:    "my-folder/abcdefg",
			matched: true,
		},
		// Test case 25-26.
		// test case with no '*'.
		{
			pattern: "my-folder/abc?",
			text:    "my-folder/abc",
			matched: false,
		},
		{
			pattern: "my-folder/abc?",
			text:    "my-folder/abcd",
			matched: true,
		},
		{
			pattern: "my-folder/abc?",
			text:    "my-folder/abcde",
			matched: false,
		},
		// Test case 27.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mnop",
			matched: false,
		},
		// Test case 28.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mnopqrst/mnopqr",
			matched: true,
		},
		// Test case 29.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mnopqrst/mnopqrs",
			matched: true,
		},
		// Test case 30.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mnop",
			matched: false,
		},
		// Test case 31.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mnopq",
			matched: true,
		},
		// Test case 32.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mnopqr",
			matched: true,
		},
		// Test case 33.
		{
			pattern: "my-folder/mnop*?and",
			text:    "my-folder/mnopqand",
			matched: true,
		},
		// Test case 34.
		{
			pattern: "my-folder/mnop*?and",
			text:    "my-folder/mnopand",
			matched: false,
		},
		// Test case 35.
		{
			pattern: "my-folder/mnop*?and",
			text:    "my-folder/mnopqand",
			matched: true,
		},
		// Test case 36.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mn",
			matched: false,
		},
		// Test case 37.
		{
			pattern: "my-folder/mnop*?",
			text:    "my-folder/mnopqrst/mnopqrs",
			matched: true,
		},
		// Test case 38.
		{
			pattern: "my-folder/mnop*??",
			text:    "my-folder/mnopqrst",
			matched: true,
		},
		// Test case 39.
		{
			pattern: "my-folder/mnop*qrst",
			text:    "my-folder/mnopabcdegqrst",
			matched: true,
		},
		// Test case 40.
		{
			pattern: "my-folder/mnop*?and",
			text:    "my-folder/mnopqand",
			matched: true,
		},
		// Test case 41.
		{
			pattern: "my-folder/mnop*?and",
			text:    "my-folder/mnopand",
			matched: false,
		},
		// Test case 42.
		{
			pattern: "my-folder/mnop*?and?",
			text:    "my-folder/mnopqanda",
			matched: true,
		},
		// Test case 43.
		{
			pattern: "my-folder/mnop*?and",
			text:    "my-folder/mnopqanda",
			matched: false,
		},
		// Test case 44.

		{
			pattern: "my-?-folder/abc*",
			text:    "my-folder/mnopqanda",
			matched: false,
		},
	}
	// Iterating over the test cases, call the function under test and asert the output.
	for i, testCase := range testCases {
		// println("=====", i+1, "=====")
		actualResult := Match(testCase.text, testCase.pattern)
		if testCase.matched != actualResult {
			t.Errorf(
				"Test %d: Expected the result to be `%v`, but instead found it to be `%v`",
				i+1,
				testCase.matched,
				actualResult,
			)
		}
	}
}

func TestRandomInput_tidwall(t *testing.T) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	b1 := make([]byte, 100)
	b2 := make([]byte, 100)
	for i := 0; i < 1000000; i++ {
		if _, err := rnd.Read(b1); err != nil {
			t.Fatal(err)
		}
		if _, err := rnd.Read(b2); err != nil {
			t.Fatal(err)
		}
		Match(string(b1), string(b2))
	}
}

func BenchmarkAscii(t *testing.B) {
	for i := 0; i < t.N; i++ {
		if !Match("hello", "hello") {
			t.Fatal("fail")
		}
	}
}

func BenchmarkUnicode(t *testing.B) {
	for i := 0; i < t.N; i++ {
		if !Match("h情llo", "h情llo") {
			t.Fatal("fail")
		}
	}
}

func TestLotsaStars_tidwall(t *testing.T) {
	// This tests that a pattern with lots of stars will complete quickly.
	var str, pat string

	str = `,**,,**,**,**,**,**,**,`
	pat = `,**********************************************{**",**,,**,**,` +
		`**,**,"",**,**,**,**,**,**,**,**,**,**]`
	Match(pat, str)

	str = strings.Replace(str, ",", "情", -1)
	pat = strings.Replace(pat, ",", "情", -1)
	Match(pat, str)

	str = strings.Repeat("hello", 100)
	pat = `*?*?*?*?*?*?*""`
	Match(str, pat)

	str = `*?**?**?**?**?**?***?**?**?**?**?*""`
	pat = `*?*?*?*?*?*?**?**?**?**?**?**?**?*""`
	Match(str, pat)
}
