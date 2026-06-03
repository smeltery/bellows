# Commands

## `bellows init`

Create `.bellows.yml` in the current repository.

```sh
bellows init
```

Useful flags:

```sh
bellows init --force
bellows init --dry-run
bellows init --pre-commit
bellows init --force-pre-commit
```

`--dry-run` prints the generated `.bellows.yml` to stdout and does not write
`.bellows.yml` or `.pre-commit-config.yaml`.

`--pre-commit` writes a local `pre-push` hook entry to
`.pre-commit-config.yaml`.

## `bellows doctor`

Inspect local workflow compatibility.

```sh
bellows doctor
bellows doctor --format json
bellows doctor --strict
bellows doctor --fix
```

Doctor checks workflow discovery, runner mappings, dynamic runner expressions,
`act`, Docker, and known local compatibility issues.

Text output is intended for humans. JSON output is intended for editor
integrations, automation, or agents that need stable diagnostic fields:

```json
[
  {
    "level": "warn",
    "code": "dynamic_runner",
    "workflow": ".github/workflows/ci.yml",
    "job": "test",
    "message": "job test uses dynamic runner expression ${{ matrix.os }}",
    "suggestion": "ensure every possible matrix runner value is mapped in .bellows.yml"
  }
]
```

`--strict` fails on warnings as well as errors. Use it in local scripts when
warnings should block a push.

`--fix` applies only safe `.bellows.yml` changes:

- add mappings for known runner labels;
- add cloud-like job ids such as `deploy`, `publish`, and `release` to
  `skip.jobs`.

## `bellows explain`

Show the configured event, default workflows, and discovered jobs.

```sh
bellows explain
bellows explain --format json
```

Use this before `bellows run` when you want to confirm what Bellows sees.
JSON output includes the resolved execution plan, skipped jobs, runner labels,
and local runner images.

## `bellows run`

Run GitHub Actions workflows locally through `act`.

```sh
bellows run
bellows run --workflow ci.yml
bellows run --workflow .github/workflows/ci.yml
bellows run --workflow ci.yml --job test
bellows run --list
```

Useful flags:

```sh
bellows run --event push
bellows run --precommit
```

`--list` prints the workflows and jobs Bellows would run after config and
`skip.jobs` are applied. It does not require Docker or `act`.

`--precommit` uses concise hook-oriented output and prints a rerun command on
failure.

Bellows passes `ACT=true` and `BELLOWS=true` to `act` so workflows can guard
cloud-only steps during local runs.

## `bellows cache prune`

Prune Docker builder cache used during local workflow runs.

```sh
bellows cache prune
```
