# Product Spec

## Summary

Bellows is a CLI that runs GitHub Actions workflows locally with Docker. It is
designed for repositories that already use GitHub Actions and may also use
Blacksmith runner labels. Bellows should make local workflow execution easy to
add to pre-commit or pre-push hooks so teams can catch common CI failures before
spending cloud runner minutes.

Tagline:

> Run GitHub Actions locally before you push.

Longer positioning:

> Bellows runs your GitHub Actions workflows locally with Docker before they
> reach the cloud forge.

## Goals

- Run existing `.github/workflows/*.yml` files locally.
- Integrate with `pre-commit` using a generated hook.
- Default to `pre-push` so full workflows do not slow down every commit.
- Support migration from GitHub-hosted runners and Blacksmith workers with
  runner label mappings.
- Report dynamic `runs-on` expressions such as `${{ matrix.os }}` as
  matrix-dependent diagnostics instead of treating them as literal runner
  labels.
- Use `act` as the workflow execution backend instead of implementing a custom
  GitHub Actions runner.
- Provide a `doctor` command that identifies unsupported or risky workflow
  features.
- Keep cloud CI as the authoritative source of truth for v1.

## Non-Goals

Bellows v1 does not attempt perfect GitHub Actions emulation.

Out of scope for v1:

- A custom workflow runner.
- Remote execution.
- Billing or cloud worker management.
- Replacing GitHub Actions status checks.
- OIDC authentication parity.
- Deployment environment parity.
- Full event simulation for every GitHub webhook payload.
- Secret manager integrations beyond explicit local environment variables or
  local secret files.

## Primary Users

- Developers who want quick feedback before pushing.
- Teams using GitHub Actions and Blacksmith who want to reduce failed cloud CI
  runs.
- Repo maintainers who want a low-friction migration path.
- Automation agents that need a documented local CI command.

## MVP Commands

```sh
bellows init
bellows run
bellows run --workflow ci.yml
bellows run --workflow .github/workflows/ci.yml --job test
bellows run --precommit
bellows doctor
bellows cache prune
```

## MVP Success Criteria

- A repository with a normal `.github/workflows/ci.yml` can run `bellows init`
  and get a usable `.bellows.yml`.
- `bellows run` executes the configured workflow through `act`.
- `bellows init --pre-commit` or `bellows init` can add a working `pre-commit`
  hook.
- A workflow using `runs-on: blacksmith-4vcpu-ubuntu-2204` can run locally via
  a default mapping.
- `bellows doctor` reports unsupported features without failing silently.
- Errors explain the missing dependency, unsupported workflow feature, or
  failing job clearly.

## Compatibility Targets

MVP should support common GitHub Actions features when supported by `act`:

- `uses: actions/checkout`
- `uses: actions/setup-node`
- `uses: actions/setup-python`
- `uses: actions/cache`
- `run:` steps
- `env`
- `defaults`
- `strategy.matrix`
- service containers
- `needs`
- simple `if` expressions
- `push`
- `pull_request`
- `workflow_dispatch`

Partial or diagnostic-only support:

- reusable workflows
- composite actions
- private actions
- Docker actions
- `permissions`
- `concurrency`
- OIDC
- deployment environments

## UX Principles

- Prefer clear defaults over extensive setup.
- Do not require users to edit existing workflows for the common path.
- Explain what will run before running it.
- Keep generated config small and readable.
- Fail loudly when Bellows cannot emulate something important.
- Make escape hatches explicit: skipped jobs, skipped steps, runner mappings,
  secrets, and event payloads.
