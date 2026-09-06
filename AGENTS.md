# AGENTS.md

## Project contracts

Redglob is a pure-Go Redis-style glob matcher (`*`, `?`, character classes) with Unicode support and optional case-insensitive matching.

- Keep the root module dependency-free: `go list -m all` must return a single module line. No Cgo.
- Invalid patterns (e.g. unclosed `[`) never match, through either one-shot helpers or compiled patterns.
- These are flat-string globs, not path globs: `*` and `?` do not treat `/` specially.
- Preserve Unicode correctness and Redis-style semantics when optimizing hot paths.
- Keep the public API stable and minimal: `Match`, `MatchFold`, `MatchBytes`, `MatchBytesFold`, and `Compile` → concurrency-safe `*Pattern`. Prefer one-shot helpers for single checks and `Compile` for reuse.
- Cross-library comparison code and dependencies belong only in the separate `benchmarks/` module; do not vendor them into the root module.
- Experimental SIMD (`fold_simd.go`, tagged `go1.27 && goexperiment.simd`) must remain behaviorally equivalent to the scalar path (`fold.go`), without API changes or added dependencies.

## Development

Go requirements and tool versions live in `go.mod`, `mise.toml`, and `.golangci-lint-version`. Formatting and lint rules live in `.golangci.yml`.

Prefer the existing `justfile` targets:

```sh
just deps    # install the pinned golangci-lint
just fmt     # format files in place
just lint    # run lint checks
just test    # run tests with the race detector, without cached results
just check   # fmt + lint + test; modifies formatting in place
just fuzz    # open-ended fuzzing; stop manually
```

Godoc examples belong in `match_example_test.go` (`package redglob_test`) with working `Output:` blocks.

## Commits

- Use Conventional Commits: `<type>(<scope>): <summary>` (imperative, <= 72 chars, no trailing period).
- **Always write a commit body** explaining the *why* (bullets welcome), not just the *what*.

## Verification by change scope

- Documentation-only changes: check accuracy and the diff; no Go test run is needed.
- Go code changes: run `just check`. For matching behavior changes, cover both one-shot and compiled APIs, including affected invalid-pattern and case-fold variants. Consider a bounded fuzz run for parser or matcher changes: `go test -fuzz=Fuzz -fuzztime=30s .`.
- Fold/SIMD changes: also test hardware and emulated SIMD with a Go 1.27+ toolchain supporting the experiment; report if unavailable:

  ```sh
  GOEXPERIMENT=simd go test -count=1 ./...
  GODEBUG=simd=0 GOEXPERIMENT=simd go test -count=1 ./...
  ```

- Benchmark changes: run `CGO_ENABLED=0 go test ./...` from `benchmarks/`. For performance comparisons, run `CGO_ENABLED=0 go test -run '^$' -bench . -benchmem -count 5` there.
- Module/dependency changes: run `go mod tidy` in the affected module and review the resulting diff; intentional module edits are not failures. Confirm the root still has a single module and no Cgo dependencies:

  ```sh
  go list -m all
  go list -deps -f '{{if .CgoFiles}}{{.ImportPath}}{{end}}' .
  ```

CI coverage and toolchain details live in `.github/workflows/go.yml`.
