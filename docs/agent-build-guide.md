# Agent Build Guide

This document is for coding agents implementing Bellows.

## Required Setup Flow

Bellows is a CLI project. Follow the workspace instruction for CLI projects:

1. Use `dotbrains/cli-template`.
2. Read `cli-template/AGENTS.md`.
3. Read `cli-template/LANGUAGES.md`.
4. Run `generate.sh` to scaffold the project.
5. Move the output into `bellows`.
6. Customize for Bellows.

Default language recommendation from the product spec is Go, but the template's
language guidance takes precedence during scaffolding.

## License

The Bellows root `LICENSE` file must exactly match:

```text
dotbrains/lodestone:LICENSE
```

At the time of writing, that is PolyForm Shield License 1.0.0 with:

```text
Copyright (c) 2026 dotbrains
```

Do not paraphrase or regenerate the license text. Copy it exactly.

## Implementation Priorities

Build in this order:

1. CLI scaffold.
2. Config loading for `.bellows.yml`.
3. Workflow discovery.
4. `act` dependency detection.
5. Docker dependency detection.
6. `bellows run` command.
7. Runner label mapping.
8. `bellows doctor`.
9. `bellows init`.
10. Pre-commit config generation.
11. Cache commands.
12. Documentation examples.

## Behavior Rules

- Use `act` as the execution backend.
- Do not implement a custom GitHub Actions runner for v1.
- Preserve `act` exit codes for workflow failures.
- Print actionable errors for missing dependencies.
- Never pass all environment variables as secrets by default.
- Do not edit existing workflow files during `bellows init` unless a user
  explicitly asks for that.
- Prefer `.bellows.yml` for local compatibility configuration.
- Default generated hooks to `pre-push`.

## Testing Expectations

Add focused tests for:

- config parsing and defaults;
- invalid config errors;
- workflow discovery;
- runner label mapping;
- Blacksmith label mapping;
- command construction for `act`;
- pre-commit config generation;
- doctor diagnostics.

Integration tests may use fixture repos under:

```text
testdata/
```

Suggested fixtures:

```text
testdata/basic-node/
testdata/basic-python/
testdata/blacksmith-runner/
testdata/deploy-job/
testdata/matrix/
```

## Documentation Expectations

Keep docs short and command-oriented.

Required root docs after scaffold:

- `README.md`
- `LICENSE`
- `docs/`

README must include:

- what Bellows does;
- install instructions;
- quick start;
- `.bellows.yml` example;
- pre-commit example;
- Blacksmith migration note;
- security note about running workflow code locally.

## Release Shape

Target distribution:

- GitHub releases with binaries.
- Homebrew tap.
- Optional package manager installs depending on language.

Do not add AI attribution footers to commits, PRs, generated docs, or release
notes.

