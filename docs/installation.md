# Installation

Bellows is distributed as a single binary.

## Homebrew

Homebrew distribution is available from the Dotbrains tap.

```sh
brew tap dotbrains/tap
brew install --cask dotbrains/tap/bellows
```

Verify:

```sh
bellows --version
bellows self-test
```

## Go Install

```sh
go install github.com/dotbrains/bellows@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

For private Dotbrains module access, configure Go to bypass the public proxy and
checksum database for the organization:

```sh
go env -w GOPRIVATE=github.com/dotbrains/*
go env -w GONOSUMDB=github.com/dotbrains/*
GOPROXY=direct go install github.com/dotbrains/bellows@latest
```

## Release Archives

Download archives from GitHub Releases when a tagged release is available:

```sh
gh release download --repo dotbrains/bellows --pattern "bellows_*_$(go env GOOS)_$(go env GOARCH).tar.gz"
```

## Runtime Dependencies

Bellows shells out to [`act`](https://github.com/nektos/act) and Docker.

Install `act`:

```sh
brew install act
```

Install and start a Docker-compatible runtime:

- Docker Desktop
- Rancher Desktop
- Colima
- another runtime that exposes the Docker CLI/socket

Check the local environment:

```sh
bellows doctor
```

`bellows doctor` reports whether `act` is available and Docker is reachable.

## Shell Completions

Generate completions with:

```sh
bellows completion zsh
bellows completion bash
bellows completion fish
```

For zsh with Homebrew:

```sh
bellows completion zsh > "$(brew --prefix)/share/zsh/site-functions/_bellows"
```

For bash with Homebrew:

```sh
mkdir -p "$(brew --prefix)/etc/bash_completion.d"
bellows completion bash > "$(brew --prefix)/etc/bash_completion.d/bellows"
```

For fish:

```sh
mkdir -p ~/.config/fish/completions
bellows completion fish > ~/.config/fish/completions/bellows.fish
```

## Optional: pre-commit

Bellows can generate a hook for the `pre-commit` framework.

```sh
brew install pre-commit
```

Then initialize Bellows with hook support:

```sh
bellows init --pre-commit
pre-commit install --hook-type pre-push
```
