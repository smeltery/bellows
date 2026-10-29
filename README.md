# Bellows 🔥🪨

[![CI](https://github.com/smeltery/bellows/actions/workflows/ci.yml/badge.svg)](https://github.com/smeltery/bellows/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/smeltery/bellows)](https://github.com/smeltery/bellows/releases)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](go.mod)
[![Flox](https://img.shields.io/badge/dev-Flox-purple)](.flox/env/manifest.toml)
[![Bun](https://img.shields.io/badge/checks-Bun-black?logo=bun)](package.json)
[![License](https://img.shields.io/badge/license-PolyForm_Shield-blue)](LICENSE)

Bellows runs your existing GitHub Actions workflows locally using
[act](https://github.com/nektos/act) and Docker. Preview execution plans, diagnose
compatibility issues, and catch failures before pushing. Blacksmith runner
mappings, explicit secret allowlists, and pre-push hooks fit existing workflows.
Use exact-commit verification with native Flox commands or a dedicated Docker
daemon, and retain hosted CI for platform coverage and merge enforcement.

## Get started

Install Docker and act, then:

```sh
go install github.com/smeltery/bellows@latest
cd your-repo
bellows init
bellows doctor
bellows run --list --estimate
bellows run
```

Default runners use the
[kiln](https://github.com/smeltery/kiln) image; configure `.bellows.yml` to choose
other act-compatible images.

## Documentation

Start with the [documentation guide](docs/README.md), or jump to
[installation](docs/installation.md), [quickstart](docs/quickstart.md),
[configuration](docs/configuration.md),
[local verification](docs/local-verification.md), [commands](docs/commands.md), and
[troubleshooting](docs/troubleshooting.md).
[How it works](docs/how-it-works.md) explains the execution model with diagrams.

## Contribute

```sh
flox activate
make setup
make check
```

The locked Flox environment supplies the toolchain; Bun runs documentation and
source-budget checks. Hooks and CI use the same Make targets. See
[development](docs/development.md) and [releases](docs/releases.md).

Licensed under [PolyForm Shield 1.0.0](LICENSE).
