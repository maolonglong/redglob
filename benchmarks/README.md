# Comparison benchmarks

Separate Go module used only for comparing redglob with other matchers. Keeping it isolated means benchmark dependencies never show up in redglob's own `go.mod`.

## Libraries compared

| Library | Role in the suite |
| --- | --- |
| [`tidwall/match`](https://github.com/tidwall/match) | Closest one-shot flat-string matcher (no character classes) |
| [`gobwas/glob`](https://github.com/gobwas/glob) | Compile-once matcher; still widely used |
| [`doublestar/v4`](https://github.com/bmatcuk/doublestar) | Path-oriented matcher; only compared on inputs without `/` |
| [`path.Match`](https://pkg.go.dev/path#Match) | Standard-library baseline |

All of the above are pure Go with no third-party runtime dependencies.

## What is (and isn't) measured

The suite splits three costs on purpose:

1. One-shot matching (compile + match together, or match-only APIs)
2. Compilation cost
3. Steady-state matching on an already compiled pattern

Longer inputs (`BenchmarkMatchFoldASCIILength`, `BenchmarkLongMultiStar`, `BenchmarkMatchBytes`) call `b.SetBytes` so results include MB/s throughput. `BenchmarkMatchBytes` covers the `[]byte` APIs (`MatchBytes` / `MatchBytesFold` and their compiled forms).

Cross-library rankings skip cases where semantics diverge: negated classes, path separators, malformed patterns, and non-ASCII case folding.

Unicode `?` matching is reported in its own set of benches and omits gobwas/glob. Its fixed-length optimization treats `?` as one byte in some paths, so `a?b` does not match `a界b` the way redglob does.

## Running

From this directory, with Cgo disabled:

```sh
CGO_ENABLED=0 go test -run '^$' -bench . -benchmem -count 5
```

For before/after comparisons, save the outputs and feed them to [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).

### Scalar vs experimental SIMD

Requires a Go 1.27+ toolchain. `BenchmarkMatchFoldASCIILength` covers both short and long inputs (SIMD only helps past a length threshold). `BenchmarkLongMultiStar` measures multi-segment literal search on its own.

```sh
go test -run '^$' -bench BenchmarkMatchFoldASCIILength -benchmem -count 5
GOEXPERIMENT=simd go test -run '^$' -bench BenchmarkMatchFoldASCIILength -benchmem -count 5
```
