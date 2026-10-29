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

## Existing Lefthook repositories

Keep Lefthook as your hook manager. Add a named command under the existing
`pre-push.commands` mapping in `lefthook.yml`; retain the rest of your file,
including format and lint hooks. If a `bellows` command already exists, update
that entry instead of adding a duplicate YAML key.

```yaml
pre-push:
  commands:
    bellows:
      run: bellows run --precommit
```

Install Bellows and its runtime dependencies, then initialize its configuration
without generating another hook manager's configuration:

```sh
bellows init
lefthook install
lefthook run pre-push
```

Run `bellows init` only when `.bellows.yml` does not already exist. Review the
configuration before enabling the hook. Re-running `lefthook install` after
editing its configuration does not require replacing your existing commands.
Do not use `bellows hooks install` or `pre-commit install` alongside Lefthook;
those instructions above are for repositories using the pre-commit framework.

For a repository whose Flox environment supplies Bellows, act and Docker CLI,
replace only the command's `run` value:

```yaml
run: flox activate -- bellows run --precommit
```

Lefthook starts commands from the repository; no hard-coded checkout path is
needed, including when the directory contains spaces. Activate Flox explicitly
in the command so Git clients do not depend on an interactive shell activation.
Flox itself must be available to the Git client. A running Docker daemon is still
required. A failed command makes the pre-push hook fail.

Keep full verification for pre-push unless your changed-file rules are proven to
cover the changes being pushed. Local hooks can be bypassed and are developer
feedback, not enforced GitHub merge protection. Keep hosted CI and existing
repository protections in place when using advisory local verification.
