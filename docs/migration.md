# Migration Guide

This guide describes how an existing GitHub Actions repository adopts Bellows.

## Expected Starting Point

```text
.github/workflows/ci.yml
.pre-commit-config.yaml
```

The repository may use GitHub-hosted runners:

```yaml
runs-on: ubuntu-latest
```

or Blacksmith runners:

```yaml
runs-on: blacksmith-4vcpu-ubuntu-2204
```

## Step 1: Install Dependencies

Required:

- Docker or a Docker-compatible local runtime.
- `act`.
- `pre-commit` if hook integration is desired.

Use `bellows doctor` after initialization to confirm Docker and `act` are
available.

## Step 2: Initialize

Preview a migration plan from existing workflows:

```sh
bellows migrate github-actions
```

Machine-readable output is available for agents and scripts:

```sh
bellows migrate github-actions --format json
```

```sh
bellows init
```

Preview the generated config without writing files:

```sh
bellows init --dry-run
```

Expected effects:

- discover `.github/workflows/*.yml`;
- create `.bellows.yml`;
- detect runner labels;
- add local mappings for common GitHub and Blacksmith labels;
- add a `pre-commit` hook when `--pre-commit` is passed.

To initialize with pre-push integration:

```sh
bellows init --pre-commit
```

## Step 3: Inspect

```sh
bellows doctor
```

The command reports:

- discovered workflows;
- default workflows;
- runner labels and mappings;
- unsupported features;
- jobs recommended for local skipping;
- missing secrets;
- missing local dependencies.

Apply safe config fixes:

```sh
bellows doctor --fix
```

`doctor --fix` patches safe values into `.bellows.yml` while preserving
hand-authored comments and existing key order where possible.

Use JSON output for agents or migration scripts:

```sh
bellows doctor --format json
bellows explain --format json
bellows compat --format json
```

## Step 4: Run Locally

```sh
bellows run
```

List the resolved local plan before invoking Docker:

```sh
bellows run --list
```

Specific workflow:

```sh
bellows run --workflow ci.yml
```

Specific job:

```sh
bellows run --workflow ci.yml --job test
```

Pre-push hook mode:

```sh
bellows run --precommit
```

## Step 5: Enable Pre-Push

Bellows can write the generated hook entry:

```sh
bellows hooks install --stage pre-push
pre-commit install --hook-type pre-push
```

Generated `.pre-commit-config.yaml` entry:

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

Install:

```sh
pre-commit install --hook-type pre-push
```

Remove a generated Bellows-only hook file:

```sh
bellows hooks uninstall
```

## Migration Principles

- Do not require workflow rewrites for common CI workflows.
- Prefer `.bellows.yml` mappings over editing `.github/workflows/*.yml`.
- Keep cloud CI enabled as the final authority.
- Skip deploy jobs locally unless the repo has an explicit local deployment
  simulation.
- Pass only explicitly allowed secrets.

## Common Adjustments

### Skip Deployment

```yaml
skip:
  jobs:
    - deploy
```

### Add a Runner Mapping

```yaml
runner:
  map:
    custom-ubuntu-runner: ghcr.io/dotbrains/kiln:ubuntu-22.04
```

### Allow a Secret

```yaml
secrets:
  mode: env
  allow:
    - NPM_TOKEN
```

Then run:

```sh
NPM_TOKEN=... bellows run
```

## Cloud CI Cost Reduction

Bellows reduces wasted cloud minutes by catching failures before push. It does
not skip required cloud CI status checks in v1.

Recommended workflow:

1. Bellows runs locally at `pre-push`.
2. Developers fix failures before pushing.
3. GitHub Actions or Blacksmith still runs on pull request.

## Cache Cleanup

Local runs are fastest when language package caches and Docker layers are
reused. Prefer existing setup actions where they are already in the workflow,
but expect local behavior to differ from GitHub-hosted cache services.

Recommended project-level cache choices:

- Go: keep `GOMODCACHE` and `GOCACHE` on the host or mounted by the local
  runner image.
- Node: use lockfiles and package-manager caches for npm, pnpm, yarn, or bun.
- Rust: keep Cargo registry and target directories reusable between local runs.
- Docker: use BuildKit layer cache and prune it intentionally when it grows.

Prune Docker builder cache used during local workflow runs:

```sh
bellows cache prune
```
