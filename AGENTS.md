# AGENTS.md

## Project

Redglob is a pure-Go Redis-style glob matcher (`*`, `?`, character classes) with Unicode support and optional case-insensitive matching.

Hard constraints:

- No third-party runtime dependencies in the root module (`go list -m all` must stay a single module line).
- No Cgo.
- Invalid patterns (e.g. unclosed `[`) never match, for both one-shot helpers and `Compile`.
- Patterns are flat-string globs, not path globs: `*` / `?` do not treat `/` specially.

Public API surface is intentionally small: `Match`, `MatchFold`, `MatchBytes`, `MatchBytesFold`, and `Compile` → `*Pattern` (concurrency-safe). Prefer package-level functions for one-off checks; use `Compile` when the same pattern is reused.

## Tooling

- Go `1.26.x` (see `go.mod` / `mise.toml`)
- Task runner: `just`
- Lint/format: `golangci-lint` (config `.golangci.yml`, version pin `.golangci-lint-version`)
- Formatters: `gofumpt` + `goimports` (local prefix `github.com/maolonglong/redglob`)
- Dev tools can also be installed via `mise` (see `.agents/setup`)

## Commands

Prefer `just` targets:

```sh
just deps    # install golangci-lint at the pinned version
just fmt     # golangci-lint fmt (gofumpt + goimports)
just lint    # golangci-lint run
just test    # go test -v -race -count=1 ./...
just check   # fmt + lint + test
just fuzz    # go test -fuzz=Fuzz .
```

Equivalent direct commands:

```sh
go test -race -count=1 ./...
golangci-lint run
golangci-lint fmt
```

CI also runs `go test` with `-shuffle=on` and coverage on Linux/macOS/Windows. After dependency edits, keep modules tidy:

```sh
go mod tidy
git diff --exit-code -- go.mod go.sum
```

### Benchmarks module

Cross-library benchmarks live in a **separate** module under `benchmarks/` so comparison deps never enter the root `go.mod`.

```sh
cd benchmarks
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go test -run '^$' -bench . -benchmem -count 5
```

Do not add benchmark-only dependencies to the root module.

### Experimental SIMD

`fold_simd.go` is build-tagged `go1.27 && goexperiment.simd`; the scalar fallback is `fold.go`. SIMD is optional and must not change API or add deps.

```sh
GOEXPERIMENT=simd go test -count=1 ./...
```

Requires a Go 1.27+ toolchain. Keep scalar and SIMD paths behaviorally equivalent.

## Layout

| Path | Role |
| --- | --- |
| `match.go` | One-shot match entry points and core matcher |
| `pattern.go` | `Compile` / `*Pattern` tokenized matcher |
| `fold.go` / `fold_simd.go` | Case-fold helpers (scalar vs experimental SIMD) |
| `bytesconv*.go` | `[]byte` ↔ `string` helpers (version-tagged) |
| `*_test.go` | Unit, fuzz, and compatibility tests |
| `match_example_test.go` | Godoc examples (`package redglob_test`) |
| `testdata/fuzz/` | Fuzz corpora |
| `benchmarks/` | Isolated comparison benchmark module |
| `.github/workflows/go.yml` | Lint, multi-OS test, benchmarks, SIMD CI |

## Code conventions

- Keep the public API stable and minimal; avoid new exports unless necessary.
- Match existing style; let `gofumpt` / `goimports` / `golangci-lint` enforce formatting and lint.
- No naked returns (`nakedret` max-func-lines: 0).
- Optimize hot paths carefully; preserve Unicode correctness and Redis-style semantics.
- Tests should cover both `Match*` helpers and `Compile`/`*Pattern`, including invalid patterns and fold variants.
- Example tests belong in `package redglob_test` and must keep working `Output:` blocks.
- Do not vendor comparison libraries into the root module; put them under `benchmarks/` only.

## Verification checklist

Before finishing a change:

1. `just check` (or at least `just test` + `just lint`)
2. If you touched matching semantics: run relevant tests and consider `just fuzz` for longer sessions
3. If you touched fold/SIMD: also run `GOEXPERIMENT=simd go test -count=1 ./...` when a 1.27+ toolchain is available
4. If you touched benchmarks: `cd benchmarks && CGO_ENABLED=0 go test ./...`
5. Confirm root module still has zero third-party deps and no Cgo
