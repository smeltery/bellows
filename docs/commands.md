# Commands

## `bellows init`

Create `.bellows.yml` in the current repository.

```sh
bellows init
```

Useful flags:

```sh
bellows init --force
bellows init --dry-run
bellows init --pre-commit
bellows init --force-pre-commit
```

`--dry-run` prints the generated `.bellows.yml` to stdout and does not write
`.bellows.yml` or `.pre-commit-config.yaml`.

`--pre-commit` writes a local `pre-push` hook entry to
`.pre-commit-config.yaml`.

## `bellows doctor`

Inspect local workflow compatibility.

```sh
bellows doctor
bellows doctor --format json
bellows doctor --format github
bellows doctor --format sarif
bellows doctor --strict
bellows doctor --fix
bellows doctor --fix --fix-dry-run
bellows doctor --image-preflight
bellows doctor bundle
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

GitHub output emits workflow annotations such as `::warning file=...::...`.
SARIF output emits SARIF 2.1.0 for code scanning and security tooling.

`--strict` fails on warnings as well as errors. Use it in local scripts when
warnings should block a push.

`--fix` applies only safe `.bellows.yml` changes:

- add mappings for known runner labels;
- add cloud-like job ids such as `deploy`, `publish`, and `release` to
  `skip.jobs`;
- add inferred changed-file path rules when no path rules are configured.

`--fix --fix-dry-run` prints the same fix plan, shows a unified diff preview,
and reports diagnostics using the fixed in-memory config, but does not write
`.bellows.yml`.

`--image-preflight` checks whether mapped runner images are already available
locally or pullable before invoking `act`.

`doctor bundle` emits sanitized JSON for support and agent workflows. It includes
config summaries, workflow/job summaries, doctor findings, compatibility,
policy, secret references, and hook status. It does not include secret values.

## `bellows self-test`

Check the Bellows binary, required local tools, repository config, workflow
discovery, and plan generation.

```sh
bellows self-test
bellows self-test --format json
```

Use this when validating a fresh install before running Docker-backed workflows.

## `bellows explain`

Show the configured event, default workflows, and discovered jobs.

```sh
bellows explain
bellows explain --format json
bellows explain changed-files
bellows explain changed-files --format json
```

Use this before `bellows run` when you want to confirm what Bellows sees.
JSON output includes the resolved execution plan, skipped jobs, runner labels,
and local runner images.

`explain changed-files` shows the Git files Bellows sees plus the workflows and
jobs selected by `precommit.paths`.

## `bellows run`

Run GitHub Actions workflows locally through `act`.

```sh
bellows run
bellows run --workflow ci.yml
bellows run --workflow .github/workflows/ci.yml
bellows run --workflow ci.yml --job test
bellows run --list
bellows run --list --estimate
```

Useful flags:

```sh
bellows run --event push
bellows run --precommit
```

`--list` prints the workflows and jobs Bellows would run after config and
`skip.jobs` are applied. It does not require Docker or `act`.

When `precommit.mode` is `changed-files`, `bellows run --precommit` uses
`precommit.paths` to choose workflows and jobs from changed Git files.

`--precommit` uses concise hook-oriented output and prints a rerun command on
failure.

Bellows passes `ACT=true` and `BELLOWS=true` to `act` so workflows can guard
cloud-only steps during local runs.

## `bellows cache prune`

Prune Docker builder cache used during local workflow runs.

```sh
bellows cache prune
```

## `bellows config`

Validate config and print embedded schemas.

```sh
bellows config validate
bellows config print-schema
bellows config print-schema doctor
bellows config print-schema explain
bellows config print-schema bundle
bellows config upgrade
bellows config upgrade --dry-run
```

## `bellows policy`

Validate repository policy separately from runtime compatibility.

```sh
bellows policy
bellows policy --format json
bellows policy --format github
```

Policy checks include cloud-like jobs not listed in `skip.jobs`, unguarded
artifact uploads, local OIDC gaps, deployment environments, and unmapped
runners.

Use `policy.waivers` in `.bellows.yml` for explicit exceptions with reasons.

## `bellows compat`

Report whether workflow features are supported locally, partially supported, or
unsupported.

```sh
bellows compat
bellows compat --format json
```

Use this during migration to find jobs that should stay in cloud CI, such as
release publishing, deployment environments, OIDC credentials, and registry
publishing.

## `bellows hooks`

Install or remove the generated Bellows pre-commit integration.

```sh
bellows hooks install
bellows hooks install --stage pre-commit
bellows hooks install --force
bellows hooks status
bellows hooks status --format json
bellows hooks uninstall
```

`hooks uninstall` only removes a generated Bellows-only
`.pre-commit-config.yaml`. If the file contains other hooks, edit it manually.

`hooks status --format json` reports whether the file exists, whether Bellows is
installed, whether the file is Bellows-only, and the configured stage.

## `bellows secrets audit`

Show workflow references to `${{ secrets.NAME }}` and whether each secret is
allowed for local runs.

```sh
bellows secrets audit
bellows secrets audit --format json
```

Secrets are considered allowed when listed under `secrets.allow` in
`.bellows.yml`.

## `bellows release check`

Run the local release-readiness suite from the CLI.

```sh
bellows release check
```

The command runs tests, lint, workflow validation, GoReleaser validation,
Bellows dogfood checks, a snapshot build, and a local release archive smoke.
It also verifies that the release changelog would include at least one
non-`docs:` and non-`chore:` commit since the previous tag.

## `bellows migrate github-actions`

Inspect existing `.github/workflows` files and print a recommended Bellows
migration plan.

```sh
bellows migrate github-actions
bellows migrate github-actions --format json
```

The report includes detected workflows, runner labels, local runner mappings,
cloud-like jobs to skip, a recommended `.bellows.yml`, and next commands.

## `bellows verify`

Verify committed head/base changes in a disposable worktree and save an advisory
receipt. The original checkout may contain local edits. Hosted CI remains in use;
this command neither publishes statuses nor enforces a GitHub merge gate.

```sh
bellows verify --repository owner/project --base origin/main \
  --daemon unix:///path/to/dedicated-docker.sock
bellows verify --repository owner/project --base origin/main --head feature \
  --daemon tcp://127.0.0.1:2375 --format json
```

Use an operator-owned, empty Linux Docker daemon dedicated to verification, with
runner and service images already pulled. Supported endpoints are Unix sockets
and explicit loopback TCP addresses. Configure `default_workflows` and
`secrets.mode: none` in `.bellows.yml`; cloud deployment jobs must be skipped.
Unsupported or skipped required checks cannot produce a passing result.

`--mode merge` verifies the proposed merge tree by default; `--mode head` verifies
the committed head tree. `--head` defaults to `HEAD`. `--artifacts` and `--leases`
override private directories under the user cache's `bellows` directory. All
processes using the same dedicated daemon must share its lease directory.

`--format json` writes the outcome and receipt to stdout; check logs go to stderr.
Failures, cancellation, unsupported inputs, and receipt-write errors return a
nonzero exit status. Local receipts record observations and do not authenticate
the execution of hostile code.

To observe a reported run after it finishes, add `--watch-advisory 5m` to
`verify --report --trust-workstation --pull-request <number>`. The watch is
bounded to one hour and defaults to off. Stale inputs produce an advisory error
on the original head and a nonzero exit; the saved receipt remains historical.

`bellows verify capabilities --repository owner/project --branch main` inspects
read-only GitHub API availability. It always reports advisory mode and never
certifies merge authorization. See [freshness and limitations](design/advisory-freshness.md).

## `bellows completion`

Generate shell completion scripts.

```sh
bellows completion bash
bellows completion zsh
bellows completion fish
```
