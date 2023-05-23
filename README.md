# Redglob

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/maolonglong/redglob/go.yml)](https://github.com/maolonglong/redglob/actions/workflows/go.yml)
[![PkgGoDev](https://pkg.go.dev/badge/go.chensl.me/redglob)](https://pkg.go.dev/go.chensl.me/redglob)

Redglob is a simple glob-style pattern matcher library for Go, inspired by Redis's pattern matching implementation. It provides a fast and easy-to-use solution for matching strings and byte slices against patterns with wildcard support.

## Features

- Unicode support
- Case-insensitive matching
- Matching strings and byte slices
- Supports `*`, `?`, character classes `[abc]`, ranges `[a-z]`, and negation `[^abc]`

## Installing

To install Redglob, use `go get`:

```bash
go get go.chensl.me/redglob
```

## Usage

Here's a simple example of how to use Redglob:

```go
package main

import (
	"fmt"

	"go.chensl.me/redglob"
)

func main() {
	pattern := "h?ll*"
	str := "hello, world!"

	if redglob.Match(str, pattern) {
		fmt.Println("Match!")
	} else {
		fmt.Println("No match.")
	}
}
```

## Functions

- `Match(str, pattern string) bool`: returns true if str matches pattern.
- `MatchFold(str, pattern string) bool`: case-insensitive version of Match.
- `MatchBytes(b []byte, pattern string) bool`: same as Match, but accepts a byte slice instead of a string.
- `MatchBytesFold(b []byte, pattern string) bool`: case-insensitive version of MatchBytes.

## Syntax

The pattern syntax supported by redglob is similar to that of Redis's glob command:

- `*` matches any sequence of non-Separator characters
- `?` matches any single non-Separator character
- `c` matches character `c` (where `c` is any character except `*`, `?`, and `\`)
- `\c` matches character `c`
- `[abc]` matches `a` or `b` or `c`
- `[^abc]` matches any character except `a`, `b`, or `c`
- `[a-z]` matches `a` to `z`
- `[^a-z]` matches any character except `a` to `z`

In addition, redglob supports Unicode characters. When performing a case-insensitive match with MatchFold or MatchBytesFold, the matching is performed using Unicode case-folding, which takes into account complex case mappings in the Unicode standard.

## Performance

redglob is implemented in pure Go and is optimized for performance. It uses a simple and efficient algorithm to match patterns against strings, and takes advantage of Go's built-in Unicode support to handle Unicode characters correctly.

## License

Redglob is licensed under the Apache License, Version 2.0. See the [LICENSE file](LICENSE) for details.

## Acknowledgments

redglob is inspired by Redis's glob command, which provides similar pattern matching functionality.
