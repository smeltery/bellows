# Troubleshooting

## `act is required but was not found in PATH`

Install `act`:

```sh
brew install act
```

Then rerun:

```sh
bellows doctor
```

## `docker is not reachable`

Bellows found the Docker CLI but cannot reach the Docker daemon.

Start Docker Desktop, Rancher Desktop, Colima, or your preferred Docker-compatible
runtime, then check:

```sh
docker info
bellows doctor
```

## Unmapped Runner Label

If a workflow uses a runner label Bellows does not know:

```text
job test uses unmapped runner custom-ubuntu-runner
```

Add a local mapping:

```yaml
runner:
  map:
    custom-ubuntu-runner: catthehacker/ubuntu:act-22.04
```

## Dynamic Runner Expressions

Workflows with matrix runners often use:

```yaml
runs-on: ${{ matrix.os }}
```

Bellows reports this as a warning because the concrete runner label is resolved
by `act` during matrix expansion.

Make sure `.bellows.yml` includes mappings for the possible matrix values:

```yaml
runner:
  map:
    ubuntu-latest: catthehacker/ubuntu:act-22.04
    macos-latest: catthehacker/ubuntu:act-22.04
```

The macOS mapping is a local compatibility fallback. It does not emulate macOS.

## Deployment Jobs Fail Locally

Skip deployment jobs in local runs:

```yaml
skip:
  jobs:
    - deploy
```

Cloud CI should remain responsible for production deployment checks.

## Clear Local Docker Cache

```sh
bellows cache prune
```

