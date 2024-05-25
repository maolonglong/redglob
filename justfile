alias t := test
alias c := check

default:
  just --list

# install dev tools
deps:
  go install github.com/segmentio/golines@latest
  go install mvdan.cc/gofumpt@latest
  go install github.com/rinchsan/gosimports/cmd/gosimports@latest
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

fmt:
  gosimports -local github.com/maolonglong -w .
  golines --max-len=99 --base-formatter="gofumpt -extra" -w .

fuzz:
  go test -fuzz=Fuzz .

test:
  go test -v -race -count=1 ./...

lint:
  golangci-lint run

check: fmt lint test
