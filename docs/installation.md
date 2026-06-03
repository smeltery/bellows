# Installation

Bellows is distributed as a single binary.

## Homebrew

```sh
brew tap dotbrains/tap
brew install --cask dotbrains/tap/bellows
```

Verify:

```sh
bellows --version
```

## Go Install

```sh
go install github.com/dotbrains/bellows@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

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
