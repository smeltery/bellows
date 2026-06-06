# Troubleshooting

## `act is required but was not found in PATH`

Install `act`:

```sh
brew install act
```

Then rerun:

```sh
bellows doctor
bellows self-test
```

Use runner image preflight when failures look like missing or stale local runner
images:

```sh
bellows doctor --image-preflight
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
    custom-ubuntu-runner: ghcr.io/dotbrains/kiln:ubuntu-22.04
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
    ubuntu-latest: ghcr.io/dotbrains/kiln:ubuntu-22.04
    macos-latest: ghcr.io/dotbrains/kiln:ubuntu-22.04
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

## Artifact Uploads Fail Locally

GitHub artifact uploads require cloud runtime credentials that local `act` runs
usually do not have.

Guard artifact steps so they stay enabled in cloud CI and are skipped locally:

```yaml
- name: Upload coverage
  if: ${{ env.ACT != 'true' }}
  uses: actions/upload-artifact@v5
```

Bellows passes both `ACT=true` and `BELLOWS=true` to local workflow runs.

## `doctor` JSON For Automation

Use JSON output when another tool needs to inspect diagnostics:

```sh
bellows doctor --format json
```

Each finding includes a `level`, stable `code`, message, and optional workflow,
job, and suggestion fields.

## Clear Local Docker Cache

```sh
bellows cache prune
```

## Refresh Local Action And Image Caches

`act` caches downloaded actions and Docker images. If a workflow action version
or runner image changes but local behavior looks stale, refresh the local cache:

```sh
rm -rf ~/.cache/act
docker image prune
```

For a full Docker layer cleanup:

```sh
bellows cache prune
```
