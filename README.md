# Redglob

[![PkgGoDev](https://pkg.go.dev/badge/github.com/maolonglong/redglob)](https://pkg.go.dev/github.com/maolonglong/redglob)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/maolonglong/redglob/go.yml?label=ci)](https://github.com/maolonglong/redglob/actions/workflows/go.yml)
[![Codecov](https://img.shields.io/codecov/c/github/maolonglong/redglob/main?logo=codecov)](https://codecov.io/gh/maolonglong/redglob)
[![GoReportCard](https://goreportcard.com/badge/github.com/maolonglong/redglob)](https://goreportcard.com/report/github.com/maolonglong/redglob)

Redglob is a simple glob-style pattern matcher library for Go, inspired by Redis's pattern matching implementation. It provides a fast and easy-to-use solution for matching strings and byte slices against patterns with wildcard support.

## Features

- Unicode support
- Case-insensitive matching
- Capability to match strings and byte slices
- Supports `*`, `?`, character classes `[abc]`, ranges `[a-z]`, and negation `[^abc]`
- Zero third-party dependencies and no Cgo

## Installing

```bash
go get github.com/maolonglong/redglob
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/maolonglong/redglob"
)

func main() {
	pattern := redglob.Compile("h?ll*")

	if pattern.Match("hello, world!") {
		fmt.Println("Match!")
	} else {
		fmt.Println("No match.")
	}
}
```

## Functions

- `Match(str, pattern string) bool`: returns true if the given string matches the pattern.
- `MatchFold(str, pattern string) bool`: case-insensitive version of `Match`.
- `MatchBytes(b []byte, pattern string) bool`: similar to `Match`, but accepts a byte slice instead of a string.
- `MatchBytesFold(b []byte, pattern string) bool`: case-insensitive version of `MatchBytes`.
- `Compile(pattern string) *Pattern`: compiles a pattern for repeated, concurrency-safe matching.

Compiled patterns provide `Match`, `MatchFold`, `MatchBytes`, and `MatchBytesFold` methods. Use
the package-level functions for one-off matches and `Compile` when the same pattern is reused.

## Syntax

Redglob's pattern syntax is similar to that of Redis's `KEYS` command:

- `*` matches any sequence of non-Separator characters
- `?` matches any single non-Separator character
- `c` matches character `c` (where `c` is any character except `*`, `?`, and `\`)
- `\c` matches character `c`
- `[abc]` matches `a` or `b` or `c`
- `[^abc]` matches any character except `a`, `b`, or `c`
- `[a-z]` matches `a` to `z`
- `[^a-z]` matches any character except `a` to `z`

## Performance

Redglob is implemented in pure Go and is optimized for performance. It uses a simple and efficient algorithm to match patterns against strings, and takes advantage of Go's built-in Unicode support to handle Unicode characters correctly.

Cross-library benchmarks live in the isolated [`benchmarks`](benchmarks) module. Benchmark-only
dependencies never enter redglob's module graph; see its README for methodology and commands.

### Experimental SIMD

Go 1.27 adds an experimental, portable `simd` package. When redglob is built with a Go 1.27+
toolchain and `GOEXPERIMENT=simd`, long ASCII case-insensitive literal and prefix comparisons use
that package. Short or non-ASCII comparisons continue through the scalar/Unicode implementation.

```sh
GOEXPERIMENT=simd go test ./...
GOEXPERIMENT=simd go build ./...
```

SIMD remains disabled by default and the module's `go 1.26` directive is unchanged. Users do not
gain dependencies, Cgo, new API requirements, or special build settings unless they explicitly
enable the experiment. Since Go's SIMD API is not yet stable, this path may change before a future
Go release makes it generally available.

## License

Redglob is licensed under the Apache License, Version 2.0. See the [LICENSE file](LICENSE) for details.
