# Blacksmith Compatibility

Bellows is generic GitHub Actions tooling with first-class migration support for
repositories using Blacksmith runner labels.

## Goal

Existing workflows like this do not need to be rewritten:

```yaml
jobs:
  test:
    runs-on: blacksmith-4vcpu-ubuntu-2204
```

Bellows maps the runner label to a local Docker image:

```yaml
runner:
  map:
    blacksmith-4vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
```

## Default Mappings

Bellows recognizes common labels:

```yaml
runner:
  map:
    blacksmith-2vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-4vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-8vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-16vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-32vcpu-ubuntu-2204: catthehacker/ubuntu:act-22.04
    blacksmith-2vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04
    blacksmith-4vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04
    blacksmith-8vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04
    blacksmith-16vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04
    blacksmith-32vcpu-ubuntu-2404: catthehacker/ubuntu:act-24.04
```

If an image tag is unavailable, Bellows uses the closest stable `act` image and reports
the substitution.

## Doctor Output

`bellows doctor` detects Blacksmith labels and reports mappings:

```text
Found Blacksmith runner: blacksmith-4vcpu-ubuntu-2204
Local image: catthehacker/ubuntu:act-22.04
Status: mapped
```

For unknown labels:

```text
Found Blacksmith runner: blacksmith-gpu-ubuntu-2204
Status: unmapped
Add this to .bellows.yml:

runner:
  map:
    blacksmith-gpu-ubuntu-2204: <local-docker-image>
```

## Unsupported Cloud Features

Bellows cannot emulate every cloud worker capability.

Likely unsupported or partial:

- larger CPU and memory guarantees;
- GPUs;
- privileged cloud network access;
- OIDC credentials;
- cloud deployment environments;
- hosted cache behavior;
- proprietary Blacksmith acceleration.

Diagnostics identify these as local compatibility differences, not workflow
errors.

## Migration Promise

The migration promise is:

> Keep your workflow files. Add Bellows config. Run the same jobs locally where
> local emulation is practical.

Bellows does not provide exact Blacksmith parity. Cloud-only behavior, worker
resources, and proprietary acceleration remain cloud-runner concerns.
