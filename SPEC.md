# Bellows Spec

Bellows runs existing GitHub Actions workflows locally through `act` before they reach cloud runners.

Cloud CI remains the source of truth. Bellows is a local feedback tool, not a replacement for GitHub Actions status checks.

## Requirements

- Docker or a Docker-compatible local runtime must be running.
- `act` must be installed and available on `PATH`.
- Workflows live in `.github/workflows`.
- Repository configuration lives at `.bellows.yml`.

Use `bellows doctor` to check the local environment and workflow compatibility.

## Commands

### `bellows init`

Creates `.bellows.yml` in the repository root.

```sh
bellows init
```

The generated config includes detected workflows, default act runner mappings, and Blacksmith mappings when Blacksmith labels are present.
Dynamic runner expressions such as `${{ matrix.os }}` are reported as
matrix-dependent diagnostics instead of literal unmapped runner labels.

Use pre-commit integration during setup:

```sh
bellows init --pre-commit
```

This writes a local pre-commit hook entry that runs at `pre-push`.

### `bellows run`

Runs configured workflows through `act`.

```sh
bellows run
bellows run --workflow ci.yml
bellows run --workflow .github/workflows/ci.yml --job test
bellows run --precommit
```

`--workflow` accepts either a basename from `.github/workflows` or a repository-relative workflow path.

`--job` limits execution to one job.

`--precommit` uses the pre-push hook behavior: run configured workflows, preserve failing exit codes, print a concise summary, and print the exact rerun command for failures.

### `bellows doctor`

Checks whether the current repository can run locally.

```sh
bellows doctor
```

Doctor reports:

- missing Docker or `act`;
- unreadable or missing `.bellows.yml`;
- unsupported or risky workflow features;
- unmapped `runs-on` labels;
- Blacksmith labels and their local image mappings;
- secret names required by workflows but not allowed locally.

### `bellows cache prune`

Prunes Docker builder cache used during local workflow runs.

```sh
bellows cache prune
```

Current behavior delegates to `docker builder prune --force`. It does not remove
workflow files, Bellows config, or pre-commit configuration.

## Configuration

Bellows reads `.bellows.yml` from the repository root.

```yaml
version: 1
default_workflows:
  - ci.yml
event: pull_request
runner:
  map:
    ubuntu-latest: ghcr.io/smeltery/kiln:ubuntu-22.04
    macos-latest: ghcr.io/smeltery/kiln:ubuntu-22.04
    ubuntu-22.04: ghcr.io/smeltery/kiln:ubuntu-22.04
    blacksmith-4vcpu-ubuntu-2204: ghcr.io/smeltery/kiln:ubuntu-22.04
precommit:
  stage: pre-push
  mode: full
  timeout_seconds: 900
```

See [docs/configuration.md](docs/configuration.md) for the full schema.

## Pre-Commit Integration

Bellows integrates with the `pre-commit` framework but defaults to `pre-push`.

Generated hook:

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

Install the hook:

```sh
pre-commit install --hook-type pre-push
```

## Blacksmith Runner Mappings

Bellows does not require workflow rewrites for common Blacksmith labels.

```yaml
runner:
  map:
    blacksmith-2vcpu-ubuntu-2204: ghcr.io/smeltery/kiln:ubuntu-22.04
    blacksmith-4vcpu-ubuntu-2204: ghcr.io/smeltery/kiln:ubuntu-22.04
    blacksmith-8vcpu-ubuntu-2204: ghcr.io/smeltery/kiln:ubuntu-22.04
    blacksmith-2vcpu-ubuntu-2404: ghcr.io/smeltery/kiln:ubuntu-24.04
    blacksmith-4vcpu-ubuntu-2404: ghcr.io/smeltery/kiln:ubuntu-24.04
    blacksmith-8vcpu-ubuntu-2404: ghcr.io/smeltery/kiln:ubuntu-24.04
```

Cloud-only Blacksmith behavior, larger CPU and memory guarantees, GPUs, OIDC, deployment environments, and proprietary acceleration are not locally emulated.
macOS labels may be mapped to Ubuntu act images for local compatibility, but
macOS itself is not emulated.

## License

Bellows uses the PolyForm Shield License 1.0.0. The root `LICENSE` file must exactly match `smeltery/lodestone:LICENSE`.
