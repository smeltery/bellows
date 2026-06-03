# Bellows Documentation

Bellows runs existing GitHub Actions workflows locally with Docker before they
reach cloud runners.

This directory is the source of truth for agents building the Bellows repo.
Start with these documents:

- [product-spec.md](product-spec.md): product goals, MVP scope, and non-goals.
- [architecture.md](architecture.md): system design and implementation shape.
- [configuration.md](configuration.md): `.bellows.yml` schema and defaults.
- [pre-commit.md](pre-commit.md): hook behavior and generated config.
- [blacksmith.md](blacksmith.md): Blacksmith runner compatibility.
- [migration.md](migration.md): migration flow for existing repositories.
- [agent-build-guide.md](agent-build-guide.md): implementation rules for coding agents.

## Project Decisions

- CLI name: `bellows`
- Config file: `.bellows.yml`
- Execution backend: `act`
- Container runtime: Docker-compatible local runtime
- Default hook stage: `pre-push`
- User commands: `bellows init`, `bellows run`, `bellows doctor`,
  `bellows cache prune`
- Default implementation language: Go, unless `cli-template/LANGUAGES.md` directs
  otherwise during scaffolding.
- License: PolyForm Shield License 1.0.0; copy the exact `LICENSE` text from
  `dotbrains/lodestone`.
