# Commands

## `bellows init`

Create `.bellows.yml` in the current repository.

```sh
bellows init
```

Useful flags:

```sh
bellows init --force
bellows init --pre-commit
bellows init --force-pre-commit
```

`--pre-commit` writes a local `pre-push` hook entry to
`.pre-commit-config.yaml`.

## `bellows doctor`

Inspect local workflow compatibility.

```sh
bellows doctor
bellows doctor --format json
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

## `bellows explain`

Show the configured event, default workflows, and discovered jobs.

```sh
bellows explain
```

Use this before `bellows run` when you want to confirm what Bellows sees.

## `bellows run`

Run GitHub Actions workflows locally through `act`.

```sh
bellows run
bellows run --workflow ci.yml
bellows run --workflow .github/workflows/ci.yml
bellows run --workflow ci.yml --job test
```

Useful flags:

```sh
bellows run --event push
bellows run --precommit
```

`--precommit` uses concise hook-oriented output and prints a rerun command on
failure.

Bellows passes `ACT=true` and `BELLOWS=true` to `act` so workflows can guard
cloud-only steps during local runs.

## `bellows cache prune`

Prune Docker builder cache used during local workflow runs.

```sh
bellows cache prune
```
