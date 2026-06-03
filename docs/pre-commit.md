# Pre-Commit Integration

Bellows integrates with the `pre-commit` framework.

The default generated hook runs at `pre-push`, not `pre-commit`, because
full GitHub Actions workflows are often too slow for every commit.

Create the hook entry during Bellows setup:

```sh
bellows init --pre-commit
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

## Command Behavior

`bellows run --precommit`:

- loads `.bellows.yml`;
- runs configured default workflows through `act`;
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

## Performance Expectations

Bellows favors predictable behavior over clever skipping in v1.

Performance improvements can come later through:

- selective job execution;
- workflow-to-path mappings;
- cache reuse;
- persistent Docker images;
- dependency cache guidance.
