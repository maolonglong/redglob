# Redglob

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/maolonglong/redglob/go.yml)](https://github.com/maolonglong/redglob/actions/workflows/go.yml)
[![PkgGoDev](https://pkg.go.dev/badge/go.chensl.me/redglob)](https://pkg.go.dev/go.chensl.me/redglob)

Redglob is a very simple pattern maching with unicode support. (Go implementation of Redis's glob-style pattern matching)

## Installing

```bash
go get go.chensl.me/redglob
```

## Example

```go
redglob.Match("hello", "*llo")
redglob.Match("jello", "?ello")
redglob.Match("hello", "h*o")
```
