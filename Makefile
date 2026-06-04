BINARY := bellows
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test lint install clean vet release-check

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go install -ldflags "$(LDFLAGS)" .

test:
	go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY) coverage.out

cover: test
	go tool cover -html=coverage.out

release-check:
	go test ./...
	golangci-lint run
	actionlint
	goreleaser check
	go run . config validate
	go run . policy
	go run . compat
	go run . run --list --estimate
	goreleaser release --snapshot --clean --skip=publish
	rm -rf dist/smoke
	mkdir -p dist/smoke
	tar -xzf dist/bellows_$$(go env GOOS)_$$(go env GOARCH).tar.gz -C dist/smoke
	dist/smoke/bellows --version
	dist/smoke/bellows completion bash >/dev/null

.DEFAULT_GOAL := build
