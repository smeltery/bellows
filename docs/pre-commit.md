# Pre-Commit Integration

Bellows integrates with the `pre-commit` framework.

The default generated hook runs at `pre-push`, not `pre-commit`, because
full GitHub Actions workflows are often too slow for every commit.

Create the hook entry during Bellows setup:

```sh
bellows init --pre-commit
pre-commit install --hook-type pre-push
```

Or install the Bellows hook later:

```sh
bellows hooks install --stage pre-push
pre-commit install --hook-type pre-push
```

## Generated Hook

Generated config:

```yaml
repos:
  - repo: local
    hooks:
      - id: bellows
        name: Run GitHub Actions locally with Bellows
        entry: bellows run --precommit
        language: system
        pass_filenames: false
        stages: [pre-push]
```

## Hook ID

Use:

```text
bellows
```

## Stage

Default:

```yaml
stages: [pre-push]
```

Users may opt into `pre-commit`, but Bellows does not generate that by
default.

```sh
bellows hooks install --stage pre-commit
```

## Command Behavior

`bellows run --precommit`:

- loads `.bellows.yml`;
- runs configured default workflows through `act`;
- uses `precommit.mode: changed-files` when configured;
- preserves the underlying workflow failure exit code;
- prints a concise summary;
- prints the exact command to rerun manually when a failure occurs.

Example failure footer:

```text
Bellows failed while running .github/workflows/ci.yml job test.
Rerun: bellows run --workflow .github/workflows/ci.yml --job test
```

## Manual Installation

Users can add Bellows manually:

```sh
pre-commit install --hook-type pre-push
```

Then run:

```sh
pre-commit run bellows --hook-stage pre-push
```

## Changed Files Mode

Use `changed-files` mode to select workflows or jobs from Git changes before a
push:

```yaml
precommit:
  mode: changed-files
  paths:
    - pattern: "docs/*"
      workflows: [docs.yml]
    - pattern: "**/*.go"
      workflows: [ci.yml]
      jobs: [test]
```

When no changed file matches a rule, `bellows run --precommit` exits without
running Docker.

Patterns support `**` for nested monorepo paths.

Debug rule selection without running Docker:

```sh
bellows explain changed-files
```
