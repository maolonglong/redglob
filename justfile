# Copyright 2023 Shaolong Chen <shaolong.chen@outlook.it>
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
  golines --max-len=99 --base-formatter="gofumpt -extra" -w .
  gosimports -local go.chensl.me -w .

fuzz:
  go test -fuzz=Fuzz .

test:
  go test -v -race -count=1 ./...

lint:
  golangci-lint run

check: fmt lint test
