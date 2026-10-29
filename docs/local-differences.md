# Local Differences

Bellows runs GitHub Actions workflows through Docker and `act`. That is useful
for fast pre-push feedback, but it is not a byte-for-byte GitHub Actions
emulator.

Cloud CI should remain the final authority for pull requests, protected
branches, release publishing, deployments, and environment approvals.

## Runner Images

GitHub-hosted runners and Blacksmith workers are virtual machines. Bellows maps
their `runs-on` labels to local Docker images.

Examples:

```yaml
runner:
  map:
    ubuntu-latest: ghcr.io/smeltery/kiln:ubuntu-22.04
    blacksmith-4vcpu-ubuntu-2204: ghcr.io/smeltery/kiln:ubuntu-22.04
```

macOS labels can map to Ubuntu-based act images for local compatibility checks,
but this does not emulate macOS frameworks, filesystems, signing, or packaging.

## Matrix Runners

Bellows detects simple matrix runner values:

```yaml
strategy:
  matrix:
    os: [ubuntu-latest, macos-latest]
runs-on: ${{ matrix.os }}
```

Run `bellows doctor` after changing matrix values so `.bellows.yml` stays in
sync with every possible local runner label.

## Cloud Runtime Services

Some actions rely on GitHub-hosted services that are absent locally:

- artifact upload and download;
- GitHub cache service;
- OIDC token exchange;
- deployment environments and manual approvals;
- GitHub-hosted credentials and organization secrets.

Bellows reports these as diagnostics when it can see them.

## Guarding Cloud-Only Steps

Bellows passes `ACT=true` and `BELLOWS=true` to local runs. Use these variables
to keep cloud-only steps out of local execution:

```yaml
- name: Upload coverage
  if: ${{ env.ACT != 'true' }}
  uses: actions/upload-artifact@v5
```

For jobs that should never run locally, add them to `.bellows.yml`:

```yaml
skip:
  jobs:
    - deploy
```

`bellows init` suggests skips for job ids such as `deploy`, `publish`, and
`release`, but the generated config should still be reviewed.

## Apple Silicon

On Apple Silicon, `act` may suggest:

```yaml
docker:
  platform: linux/amd64
```

This can help images that are amd64-only, but it can also expose action-specific
issues under emulation. Add it only when the repository needs amd64 containers
locally.

Exact-commit verification currently rejects Git submodule inputs. Detached
checkouts do not automatically materialize their contents, so a receipt must not
claim a complete tested tree while those inputs are absent.
