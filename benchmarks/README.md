# Comparison benchmarks

This directory is a separate Go module so benchmark-only dependencies never enter redglob's
`go.mod` or its users' dependency graphs. All compared libraries are pure Go and have no
third-party runtime dependencies:

- `github.com/tidwall/match`: closest one-shot flat-string matcher; no character classes.
- `github.com/gobwas/glob`: compile-once matcher; dormant but widely used.
- `github.com/bmatcuk/doublestar/v4`: actively maintained path matcher; compared only on inputs
  without `/`, where its semantics overlap.
- `path.Match`: standard-library baseline.

The suites intentionally separate one-shot matching, compilation cost, and compiled steady-state
matching. Negated classes, paths, malformed patterns, and non-ASCII case folding are excluded from
cross-library rankings because their semantics differ. Unicode `?` matching is reported separately
without gobwas/glob: although its single-rune matcher decodes UTF-8, its combined fixed-length
optimization makes `a?b` fail to match `a界b`.

Run the benchmarks without Cgo:

```sh
cd benchmarks
CGO_ENABLED=0 go test -run '^$' -bench . -benchmem -count 5
```

For statistically useful before/after comparisons, save outputs and analyze them with `benchstat`.

To compare the scalar and experimental SIMD paths using a Go 1.27+ toolchain:

```sh
go test -run '^$' -bench BenchmarkMatchFoldASCIILength -benchmem -count 5
GOEXPERIMENT=simd go test -run '^$' -bench BenchmarkMatchFoldASCIILength -benchmem -count 5
```

`BenchmarkMatchFoldASCIILength` includes short and long inputs because SIMD setup is not beneficial
for small comparisons. `BenchmarkLongMultiStar` measures literal-segment searching independently
from the case-folding optimization.
