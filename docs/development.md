# Development

This page is for people contributing to Bellows itself.

## Flox Environment

Bellows includes a Flox environment with the development toolchain:

- Go
- act
- Docker CLI
- pre-commit
- golangci-lint
- GoReleaser

Activate it:

```sh
flox activate
```

Or run commands directly:

```sh
flox activate -- go test ./...
flox activate -- golangci-lint run
flox activate -- goreleaser check
```

## Common Checks

```sh
go test ./...
golangci-lint run
goreleaser check
```

## Release Tool Versions

The scheduled release check pins tool versions in
[`.github/workflows/release-check.yml`](../.github/workflows/release-check.yml):

- actionlint `v1.7.9`
- golangci-lint `v2.13.2`
- GoReleaser `v2.18.0`

Keep local Flox tooling and the workflow pins aligned when changing release
checks.

The release and release dry-run workflows also pin GoReleaser to `v2.18.0`.

## Local Release Check

Use GoReleaser snapshot mode to verify archives and the Homebrew cask without
publishing:

```sh
goreleaser release --snapshot --clean --skip=publish
```

Remove generated artifacts afterward:

```sh
rm -rf dist
```
