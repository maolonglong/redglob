# Redglob

[![PkgGoDev](https://pkg.go.dev/badge/github.com/maolonglong/redglob)](https://pkg.go.dev/github.com/maolonglong/redglob)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/maolonglong/redglob/go.yml?label=ci)](https://github.com/maolonglong/redglob/actions/workflows/go.yml)
[![Codecov](https://img.shields.io/codecov/c/github/maolonglong/redglob/main?logo=codecov)](https://codecov.io/gh/maolonglong/redglob)

Redglob is a Redis-style glob matcher for Go. It matches strings and byte slices against patterns with `*`, `?`, and character classes, with full Unicode support and optional case-insensitive matching. Pure Go, no third-party dependencies, no Cgo.

## Install

```bash
go get github.com/maolonglong/redglob
```

## Quick start

```go
package main

import (
	"fmt"

	"github.com/maolonglong/redglob"
)

func main() {
	// One-off match
	fmt.Println(redglob.Match("hello, world!", "h?ll*")) // true

	// Compile once when reusing the same pattern
	p := redglob.Compile("user:[0-9]*")
	fmt.Println(p.Match("user:42"))   // true
	fmt.Println(p.Match("admin:42"))  // false
}
```

## API

| Function | Description |
| --- | --- |
| `Match(str, pattern string) bool` | Match a string against a pattern |
| `MatchFold(str, pattern string) bool` | Case-insensitive `Match` |
| `MatchBytes(b []byte, pattern string) bool` | Match a byte slice |
| `MatchBytesFold(b []byte, pattern string) bool` | Case-insensitive `MatchBytes` |
| `Compile(pattern string) *Pattern` | Compile a pattern for repeated, concurrency-safe matching |

A compiled `*Pattern` exposes the same four methods: `Match`, `MatchFold`, `MatchBytes`, and `MatchBytesFold`.

Prefer the package-level functions for one-off checks; use `Compile` when the same pattern is applied many times.

## Pattern syntax

Syntax follows Redis `KEYS` / `SCAN` glob patterns:

| Pattern | Meaning |
| --- | --- |
| `*` | Any sequence of characters (including empty) |
| `?` | Any single character |
| `c` | The character `c` (except `*`, `?`, `\`) |
| `\c` | Escaped character `c` |
| `[abc]` | One of `a`, `b`, or `c` |
| `[^abc]` | Any character except `a`, `b`, or `c` |
| `[a-z]` | Inclusive range from `a` to `z` |
| `[^a-z]` | Any character outside that range |

Patterns are flat-string globs, not path globs: `*` and `?` do not treat `/` specially.

Invalid patterns (for example an unclosed `[`) never match, both for the one-shot helpers and for `Compile`.

## Performance

Redglob is written in pure Go and tuned for common cases (literal prefixes/suffixes, simple `*` patterns, and longer multi-segment patterns).

Cross-library comparisons live in a separate [`benchmarks`](benchmarks) module so benchmark-only dependencies stay out of redglob's `go.mod`. See that directory's README for how to run them.

### Experimental SIMD (Go 1.27+)

With a Go 1.27+ toolchain and `GOEXPERIMENT=simd`, long ASCII case-insensitive literal/prefix comparisons can use Go's experimental portable `simd` package. Short and non-ASCII inputs still use the scalar path.

```sh
GOEXPERIMENT=simd go test ./...
GOEXPERIMENT=simd go build ./...
```

SIMD is off by default. The module still declares `go 1.26`, and enabling the experiment does not add dependencies, Cgo, or API changes. Go's SIMD API is not stable yet, so this path may change in a future Go release.

## License

Apache License 2.0. See [LICENSE](LICENSE).
