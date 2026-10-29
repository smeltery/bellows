BINARY := bellows
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install test lint vet clean cover setup validate check dogfood release-check
.DEFAULT_GOAL := build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go install -ldflags "$(LDFLAGS)" .

setup:
	bun install --frozen-lockfile
	pre-commit install --hook-type pre-commit --hook-type pre-push

validate:
	bun install --frozen-lockfile
	bun run docs
	bun run budget
	actionlint
	goreleaser check

check: validate lint test build dogfood

test:
	go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | awk '/^total:/ { print; if ($$3 + 0 < 60) exit 1 }'

vet:
	go vet ./...

lint:
	golangci-lint run

dogfood:
	go run . config validate
	go run . policy
	go run . compat
	go run . run --list --estimate

clean:
	rm -f $(BINARY) coverage.out

cover: test
	go tool cover -html=coverage.out

release-check: check
	goreleaser release --snapshot --clean --skip=publish
	rm -rf dist/smoke
	mkdir -p dist/smoke
	tar -xzf dist/bellows_$$(go env GOOS)_$$(go env GOARCH).tar.gz -C dist/smoke
	dist/smoke/bellows --version
	dist/smoke/bellows completion bash >/dev/null
