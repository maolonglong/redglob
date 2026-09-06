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
| Unicode runes | Yes (`?` is one rune) | Yes | Yes (v1.0.0) | Yes | Yes |
| Case-insensitive | `MatchFold` / `MatchBytesFold` | `MatchNoCase` | No | Via FS layer | No |
| `[]byte` API | Yes (zero-copy) | No | `Match` on string | No | No |
| Compile API | `Compile` → `*Pattern` | One-shot only | `Compile` → `*Pattern` | No compile API | One-shot only |
| Invalid pattern | Never matches | — | Error on compile | Error / unvalidated | Error |
| Runtime deps / Cgo | None | None | None | None | Stdlib |

**When to pick redglob**

- You want **Redis `KEYS` / `SCAN` semantics** on flat strings (including character classes), not filesystem path globs.
- You need **one-shot** matching that stays allocation-free, or **fast compile** when patterns are short-lived.
- You need **Unicode-correct** `?`, optional **case folding**, and/or a **`[]byte`** path without converting to `string`.

**Trade-offs**

- Compiled matching depends on the pattern: redglob leads on literals and single-star cases in this suite, while gobwas is faster on the backtracking-miss case.
- On short `?`-heavy ASCII patterns, tidwall can win the one-shot race (it does less work and does not implement classes).
- doublestar / `path.Match` are the right tools when you need **path** semantics (`/` boundaries, `**`, etc.).

The doublestar benchmarks use `MatchUnvalidated`, which skips part of pattern validation for the suite's known-valid patterns; doublestar does not provide a compile API.

## Performance

Numbers below are median `ns/op` from the isolated [`benchmarks`](benchmarks) module in an Amp orb with an Intel Xeon @ 2.60 GHz (2 vCPUs, `linux/amd64`), Go 1.26.5, `CGO_ENABLED=0`, `-count 5`. Dependency versions: gobwas/glob v1.0.0, tidwall/match v1.2.0, doublestar/v4 v4.10.0. Results are specific to this environment; the experimental SIMD section lists its own environment. Re-run anytime:

```sh
cd benchmarks
CGO_ENABLED=0 go test -run '^$' -bench . -benchmem -count 5
```

### One-shot match (no prior compile)

Gobwas is measured as compile+match because it has no one-shot API. tidwall has no character classes.

| Case | Pattern sketch | redglob | tidwall | doublestar | `path.Match` | gobwas (compile+match) |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Literal match | `customer:123…` | **23** | 70 | 142 | 84 | 673 |
| Literal miss | same, last digit differs | **42** | 67 | 133 | 86 | 649 |
| Prefix `*` | `customer:*` | **24** | 38 | 95 | 52 | 702 |
| Suffix `*` | `*:profile` | **23** | 55 | 228 | 562 | 640 |
| Infix `*` | `customer:*:profile` | **40** | 86 | 210 | 357 | 980 |
| ASCII `?` | `file-??.txt` | 90 | **43** | 73 | 52 | 945 |
| Unicode `?` | `a?b` vs `a界b` | 39 | **17** | 26 | 23 | 732 |
| Unicode `*` | `前*後` | **24** | 37 | 73 | 116 | 752 |
| Multi `*` | `a*b*c*d*e` | 113 | 106 | **80** | 84 | 2086 |
| Backtracking miss | `a*a*a*a*b` vs long `a…c` | 28 | **16** | 174 | 152 | 1606 |

Takeaway: on the Redis-like hot path (literals and single-star prefix/suffix/infix), redglob is **1.6–3.0×** faster than tidwall in these measurements, and faster than path-oriented matchers and gobwas-when-you-must-compile-every-time. All redglob one-shot paths above are **0 allocs/op**.

### Compiled match (steady state)

| Case | redglob | gobwas |
| --- | ---: | ---: |
| Literal match | **3.5** | 9.0 |
| Prefix `*` | **6.0** | 8.9 |
| Suffix `*` | **5.8** | 8.9 |
| Infix `*` | **10.7** | 13.3 |
| Unicode `?` | **27.1** | 51.4 |
| Multi `*` | **45.8** | 403.0 |
| Backtracking miss | 15.7 | **5.9** |

Across the common cases, compilation costs **50–291 ns** and 1–5 allocs for redglob versus **626–1665 ns** and 9–31 allocs for gobwas. Lower compilation cost favors redglob when patterns are created often or only matched a few times.

### Long multi-segment input (~512 B padding)

| | redglob one-shot | tidwall | redglob compiled | gobwas compiled | doublestar |
| --- | ---: | ---: | ---: | ---: | ---: |
| `start*middle*end` hit | 80 | 3551 | **38** | 188 | 4405 |
| miss (`*missing*`) | 93 | 6769 | **54** | 116 | 4265 |

In these cases, redglob stays below 100 ns, gobwas takes 116–188 ns, and tidwall and doublestar take several microseconds.

### Case-insensitive ASCII (`MatchFold` / tidwall `MatchNoCase`)

| Input size | redglob | redglob compiled | tidwall |
| ---: | ---: | ---: | ---: |
| short key (`customer:*:profile`) | 58 | **35** | 92 |
| 32 B literal | 85 | **49** | 133 |
| 256 B literal | 698 | **369** | 1056 |
| 4096 B literal | 10704 | **5655** | 16951 |

### Experimental SIMD (Go 1.27+)

With a Go 1.27+ toolchain and `GOEXPERIMENT=simd`, long ASCII case-insensitive literal/prefix comparisons can use Go's experimental portable `simd` package. The threshold is 64 bytes; shorter and non-ASCII inputs stay on the scalar path.

Apple M1 Pro (`darwin/arm64`), Go 1.27rc2, scalar vs `GOEXPERIMENT=simd` (benchstat, `-count 5`):

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
