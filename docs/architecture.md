# Architecture

## Overview

Bellows is an opinionated CLI wrapper around `act`. It discovers GitHub Actions
workflows, reads `.bellows.yml`, maps runner labels to local Docker images, and
executes `act` with predictable flags.

Bellows should not implement a custom GitHub Actions runtime in v1.

## High-Level Flow

```text
user command
  -> load repository context
  -> load .bellows.yml
  -> discover workflows
  -> resolve workflow/job selection
  -> build local event payload
  -> resolve runner image mappings
  -> resolve secrets/env policy
  -> execute act
  -> normalize output and exit code
```

## Suggested Go Package Layout

```text
cmd/bellows/
internal/config/
internal/doctor/
internal/exec/
internal/precommit/
internal/runner/
internal/workflow/
internal/act/
internal/cache/
docs/
examples/
```

Responsibilities:

- `cmd/bellows`: CLI command wiring.
- `internal/config`: `.bellows.yml` loading, validation, defaults.
- `internal/workflow`: workflow discovery and lightweight parsing.
- `internal/runner`: `runs-on` label mapping.
- `internal/act`: command construction for `act`.
- `internal/exec`: process execution and output handling.
- `internal/precommit`: generated hook configuration.
- `internal/doctor`: compatibility analysis.
- `internal/cache`: Docker and action cache maintenance.

## Execution Backend

Bellows shells out to `act`.

The CLI should:

- detect whether `act` is installed;
- detect whether Docker is available;
- report installation guidance if either dependency is missing;
- construct `act` commands from Bellows config;
- preserve `act` exit codes;
- avoid hiding the failing command when execution fails.

Example generated command:

```sh
act pull_request \
  --workflows .github/workflows/ci.yml \
  --job test \
  --platform ubuntu-latest=catthehacker/ubuntu:act-22.04 \
  --platform blacksmith-4vcpu-ubuntu-2204=catthehacker/ubuntu:act-22.04
```

## Dependency Strategy

MVP may require users to install `act` and Docker separately.

Future versions may support:

- bundled `act`;
- automatic dependency installation;
- alternate runtimes such as Podman;
- Homebrew formula installation helpers.

## Workflow Parsing

Use structured YAML parsing. Do not parse workflow YAML with ad hoc string
matching except for simple diagnostic display.

The parser only needs enough structure for:

- workflow names;
- jobs;
- job `runs-on`;
- job dependencies;
- matrix declarations;
- obvious unsupported features;
- step names;
- step `uses`;
- step `run`;
- step `if`;
- environment and secrets references.

`act` remains responsible for actual workflow execution semantics.

## Error Handling

Prefer explicit error messages.

Good:

```text
Docker is not reachable. Start Docker Desktop and rerun `bellows run`.
```

Good:

```text
Workflow job `deploy` requests OIDC permissions. Bellows cannot emulate OIDC
locally. Add the job to `skip.jobs` or provide a local replacement flow.
```

Bad:

```text
failed
```

## Exit Codes

- `0`: command succeeded.
- `1`: workflow failed or validation failed.
- `2`: usage error.
- `3`: missing dependency.
- `4`: unsupported workflow feature without configured skip.

## Security Model

Bellows runs arbitrary workflow code locally. Documentation and diagnostics
must make that explicit.

Defaults:

- never load all local environment variables as secrets by default;
- only pass explicitly allowed secret names;
- do not read `.env` automatically unless configured;
- avoid printing secret values;
- redact configured secret values in Bellows-managed logs.

## License

The root `LICENSE` file must exactly match `dotbrains/lodestone`:

- PolyForm Shield License 1.0.0
- `Copyright (c) 2026 dotbrains`

