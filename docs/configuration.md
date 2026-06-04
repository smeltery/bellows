# Configuration

Bellows uses `.bellows.yml` at the repository root.

JSON Schema is available at
[`docs/schemas/bellows.schema.json`](schemas/bellows.schema.json).

## Example

```yaml
version: 1

default_workflows:
  - ci.yml

event: pull_request

docker:
  platform: linux/amd64
  pull: missing

runner:
  default_image: catthehacker/ubuntu:act-22.04
  map:
    ubuntu-latest: catthehacker/ubuntu:act-22.04
    ubuntu-22.04: catthehacker/ubuntu:act-22.04
    macos-latest: catthehacker/ubuntu:act-22.04
    blacksmith-2vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-4vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-8vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-16vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-32vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-4vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04

secrets:
  mode: env
  allow:
    - GITHUB_TOKEN
    - NPM_TOKEN

skip:
  jobs:
    - deploy
  steps_matching:
    - Configure AWS credentials

precommit:
  stage: pre-push
  mode: full
  timeout_seconds: 900
  paths:
    - pattern: "*.go"
      workflows: [ci.yml]
      jobs: [test]
```

## Policy

Policy settings control how `bellows policy` treats repository-level local
execution rules.

```yaml
policy:
  cloud_jobs: error
```

`policy.cloud_jobs` values:

- `error`: fail when cloud-like jobs such as `deploy`, `publish`, or `release`
  are not listed in `skip.jobs`.
- `warn`: report the same issue as a warning.
- `off`: disable this rule.

## Fields

### `version`

Required. Current schema version is `1`.

### `default_workflows`

Workflow files to run when `bellows run` is called without `--workflow`.

Entries may be:

- basename: `ci.yml`
- relative workflow path: `.github/workflows/ci.yml`

### `event`

Default event passed to `act`.

Default:

```yaml
event: pull_request
```

Supported values:

- `pull_request`
- `push`
- `workflow_dispatch`

### `docker`

Docker runtime options.

```yaml
docker:
  platform: linux/amd64
  pull: missing
```

`pull` values:

- `missing`: pull images only when absent.
- `always`: always pull.
- `never`: never pull.

On Apple Silicon Macs, `act` may suggest `docker.platform: linux/amd64` when an
image has architecture-specific problems. Treat that as a troubleshooting knob,
not a universal default; some actions work better with the host architecture.

### `runner`

Maps GitHub Actions runner labels to local Docker images.

```yaml
runner:
  default_image: catthehacker/ubuntu:act-22.04
  map:
    ubuntu-latest: catthehacker/ubuntu:act-22.04
    macos-latest: catthehacker/ubuntu:act-22.04
```

If a workflow uses an unmapped `runs-on` label, Bellows:

1. use `default_image` when configured;
2. otherwise fail with a message that tells the user how to add a mapping.

### `secrets`

Controls which secrets are passed into local workflow execution.

```yaml
secrets:
  mode: env
  allow:
    - GITHUB_TOKEN
```

Supported modes:

- `none`: pass no secrets.
- `env`: read allowed names from the local process environment.

Bellows must not pass every environment variable by default.

### `skip`

Allows local-only exclusions.

```yaml
skip:
  jobs:
    - deploy
  steps_matching:
    - Configure AWS credentials
```

Bellows supports skipped jobs. Step skipping depends on `act` support and may be
reported as a diagnostic instead.

`bellows init` pre-populates `skip.jobs` for job ids that look cloud-only, such
as `deploy`, `publish`, or `release`. Review the generated list before
committing `.bellows.yml`.

### `precommit`

Controls generated hook behavior.

```yaml
precommit:
  stage: pre-push
  mode: full
  timeout_seconds: 900
```

`stage` defaults to `pre-push`.

`mode` values:

- `full`: run configured workflows.
- `changed-files`: choose workflows and jobs from changed Git files using
  `paths`.

`paths` entries use Go-style glob patterns. Each matching changed file can
select workflows, jobs, or both:

```yaml
precommit:
  mode: changed-files
  paths:
    - pattern: "docs/*"
      workflows: [docs.yml]
    - pattern: "*.go"
      workflows: [ci.yml]
      jobs: [test]
```

## Generated Minimal Config

`bellows init` writes a small repo-local config:

```yaml
version: 1
default_workflows:
  - ci.yml
event: pull_request
runner:
  map:
    ubuntu-latest: catthehacker/ubuntu:act-22.04
```

Add Blacksmith mappings only when Blacksmith runner labels are detected.
Known Blacksmith Ubuntu 22.04 and 24.04 labels from 2 to 32 vCPU are mapped to
the closest `catthehacker/ubuntu` act image.

macOS runner labels may be mapped to Ubuntu-based act images for local
compatibility. This catches some cross-platform failures locally but does not
emulate macOS.

Matrix runner values are detected when workflows use simple expressions such as
`${{ matrix.os }}` with scalar matrix values.

During `bellows run`, Bellows passes these environment variables to `act`:

```yaml
env:
  ACT: "true"
  BELLOWS: "true"
```

Use either variable to guard cloud-only steps in workflow files.

## Related Commands

```sh
bellows init
bellows run
bellows run --workflow ci.yml
bellows doctor
bellows cache prune
```

`bellows doctor` validates this file and reports unmapped runner labels,
unsupported workflow features, and missing Docker or `act` dependencies.

Machine-readable outputs also have schemas:

- [`docs/schemas/doctor.schema.json`](schemas/doctor.schema.json)
- [`docs/schemas/explain.schema.json`](schemas/explain.schema.json)
