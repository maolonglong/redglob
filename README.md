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

Case-insensitive matching uses Unicode simple case folding, consistent with Go's `strings.EqualFold`. Folding remains one rune to one rune, so multi-rune expansions such as `ß` → `SS` do not match.

## Comparison

| | redglob | [tidwall/match](https://github.com/tidwall/match) | [gobwas/glob](https://github.com/gobwas/glob) | [doublestar](https://github.com/bmatcuk/doublestar) | [`path.Match`](https://pkg.go.dev/path#Match) |
| --- | --- | --- | --- | --- | --- |
| Style | Redis flat-string glob | Redis-like flat string | Compile-once glob | Path glob (`**`, `/`) | Stdlib path glob |
| `*` / `?` | Characters (not path segments) | Characters | Configurable | Path-aware | Path-aware |
| Character classes `[…]` | Yes | No | Yes | Yes | Yes |
| Unicode runes | Yes (`?` is one rune) | Yes | Partial (`?` is byte-oriented in places) | Yes | Yes |
| Case-insensitive | `MatchFold` / `MatchBytesFold` | `MatchNoCase` | Separators / options | Via FS layer | No |
| `[]byte` API | Yes (zero-copy) | No | `Match` on string | No | No |
| Compile API | `Compile` → `*Pattern` | One-shot only | `Compile` → `Glob` | Optional | One-shot only |
| Invalid pattern | Never matches | — | Error on compile | Error / unvalidated | Error |
| Runtime deps / Cgo | None | None | None | None | Stdlib |

**When to pick redglob**

- You want **Redis `KEYS` / `SCAN` semantics** on flat strings (including character classes), not filesystem path globs.
- You need **one-shot** matching that stays allocation-free, or **fast compile** when patterns are short-lived.
- You need **Unicode-correct** `?`, optional **case folding**, and/or a **`[]byte`** path without converting to `string`.

**Trade-offs**

- For **compile-once, match-many** simple prefixes/suffixes, gobwas is often a few nanoseconds faster in the steady state.
- On short `?`-heavy ASCII patterns, tidwall can win the one-shot race (it does less work and does not implement classes).
- doublestar / `path.Match` are the right tools when you need **path** semantics (`/` boundaries, `**`, etc.).

## Performance

Numbers below are median `ns/op` from the isolated [`benchmarks`](benchmarks) module on an Apple M1 Pro (`darwin/arm64`), Go 1.26, `CGO_ENABLED=0`, `-count 5`. Re-run anytime:

```sh
cd benchmarks
CGO_ENABLED=0 go test -run '^$' -bench . -benchmem -count 5
```

### One-shot match (no prior compile)

Gobwas is measured as compile+match because it has no one-shot API. tidwall has no character classes.

| Case | Pattern sketch | redglob | tidwall | doublestar | `path.Match` | gobwas (compile+match) |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Literal match | `customer:123…` | **14** | 50 | 76 | 71 | 520 |
| Literal miss | same, last digit differs | **30** | 50 | 75 | 71 | 519 |
| Prefix `*` | `customer:*` | **17** | 25 | 53 | 41 | 553 |
| Suffix `*` | `*:profile` | **17** | 37 | 150 | 430 | 504 |
| Infix `*` | `customer:*:profile` | **28** | 58 | 128 | 283 | 893 |
| ASCII `?` | `file-??.txt` | 65 | **28** | 46 | 44 | 1189 |
| Unicode `*` | `前*後` | **16** | 27 | 49 | 108 | 726 |
| Multi `*` | `a*b*c*d*e` | 84 | 72 | **53** | 63 | 2287 |
| Backtracking miss | `a*a*a*a*b` vs long `a…c` | 18 | **12** | 108 | 118 | 2230 |

Takeaway: on the Redis-like hot path (literals and single-star prefix/suffix/infix), redglob is typically **1.5–3.5×** faster than tidwall and far ahead of path-oriented matchers and gobwas-when-you-must-compile-every-time. All redglob one-shot paths above are **0 allocs/op**.

### Compiled match (steady state)

| Case | redglob | gobwas |
| --- | ---: | ---: |
| Literal match | **2.9** | 4.2 |
| Prefix / suffix / infix `*` | 4.5–6.5 | **2.6–4.5** |
| Multi `*` | **54** | 66 |
| Backtracking miss | **7.8** | 11 |

Compile cost itself is much lower for redglob (roughly **30–150 ns** and 1–2 allocs vs gobwas **500–2200 ns** and dozens of allocs), so redglob wins when patterns are created often or only matched a few times.

### Long multi-segment input (~512 B padding)

| | redglob one-shot | tidwall | redglob compiled | gobwas compiled | doublestar |
| --- | ---: | ---: | ---: | ---: | ---: |
| `start*middle*end` hit | **56** | 2368 | **25** | 26 | 2553 |
| miss (`*missing*`) | **66** | 4761 | 33 | **27** | 2579 |

Literal segment search keeps redglob in the tens of nanoseconds while naive backtracking matchers fall into the microseconds.

### Case-insensitive ASCII (`MatchFold` / tidwall `MatchNoCase`)

| Input size | redglob | redglob compiled | tidwall |
| ---: | ---: | ---: | ---: |
| short key (`customer:*:profile`) | **50** | **29** | 57 |
| 32 B literal | **79** | **46** | 88 |
| 256 B literal | **596** | **342** | 643 |
| 4096 B literal | **9334** | **5297** | 10184 |

### Experimental SIMD (Go 1.27+)

With a Go 1.27+ toolchain and `GOEXPERIMENT=simd`, long ASCII case-insensitive literal/prefix comparisons can use Go's experimental portable `simd` package. The threshold is 64 bytes; shorter and non-ASCII inputs stay on the scalar path.

Same machine, Go 1.27rc2, scalar vs `GOEXPERIMENT=simd` (benchstat, `-count 5`):

| Benchmark | Scalar | SIMD | Δ |
| --- | ---: | ---: | ---: |
| `MatchFold` 32 B (below threshold) | 78 ns | 79 ns | ~0% |
| `MatchFold` 256 B | 599 ns | 391 ns | **−35%** |
| `MatchFold` 256 B compiled | 343 ns | 138 ns | **−60%** |
| `MatchFold` 4096 B | 9271 ns | 5970 ns | **−36%** |
| `MatchFold` 4096 B compiled | 5278 ns | 1994 ns | **−62%** |
| Long multi-star (not fold/SIMD path) | 59 ns | 62 ns | ~0% |

```sh
GOEXPERIMENT=simd go test ./...
GOEXPERIMENT=simd go build ./...

# Compare fold throughput
cd benchmarks
go test -run '^$' -bench BenchmarkMatchFoldASCIILength -benchmem -count 5
GOEXPERIMENT=simd go test -run '^$' -bench BenchmarkMatchFoldASCIILength -benchmem -count 5
```

SIMD is off by default. The module still declares `go 1.26`, and enabling the experiment does not add dependencies, Cgo, or API changes. Go's SIMD API is not stable yet, so this path may change in a future Go release.

Full suite details and methodology: [`benchmarks/README.md`](benchmarks/README.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
