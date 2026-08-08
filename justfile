alias t := test
alias c := check

default:
  just --list

# install dev tools
deps:
  version="$(cat .golangci-lint-version)"; curl --fail --silent --show-error --location "https://raw.githubusercontent.com/golangci/golangci-lint/$version/install.sh" | sh -s -- -b "$(go env GOPATH)/bin" "$version"

fmt:
  golangci-lint fmt

fuzz:
  go test -fuzz=Fuzz .

test:
  go test -v -race -count=1 ./...

lint:
  golangci-lint run

check: fmt lint test
