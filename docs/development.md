# Development

## One toolchain

Run `flox activate` from the repository root. The committed manifest and lockfile
supply Go, Bun, Make, Git, act, Docker CLI, pre-commit, actionlint, golangci-lint,
and GoReleaser. Docker still needs a running daemon for local workflow execution.
CI activates the same environment on Linux and macOS.

```sh
flox activate
make setup
make check
```

`make setup` installs frozen Bun dependencies and both Git hooks. For a single
command without an interactive shell, use `flox activate -- make check`.

## Shared checks

| Target | Purpose |
| --- | --- |
| `make validate` | Markdown, Mermaid syntax, LOC budget, workflow and release configuration |
| `make check` | Validation, Go lint, race tests, coverage gate, build, Bellows self-checks |
| `make test` | Race tests and minimum 60% total coverage |
| `make release-check` | Full checks plus snapshot archive and binary smoke test |

Pre-commit runs `make validate`; pre-push runs `make check`. Both hooks activate
Flox themselves. To run them manually:

```sh
pre-commit run --all-files
pre-commit run --all-files --hook-stage pre-push
```

This repository's contributor hooks check the source directly. To install
Bellows hooks in another project, use the [user hook guide](pre-commit.md).

## Keep changes small

`scripts/budget.ts` enforces nonblank line budgets for production Go and custom
check scripts (5,100 and 80 lines respectively). Tests are excluded so coverage is not discouraged. Prefer shared
Make targets and existing tools over new wrappers. Any budget increase should
include a rationale in the pull request.

The 5,100-line production budget covers measured verification and reporting
integration: 4,409 foundation lines, roughly 170 native Flox adapter lines,
140 status-transport lines, and 190 reporting-policy lines. Their estimated
combined 4,909 lines leave a small integration margin. Strict receipt validation
and complete prerequisite selection exceeded their initial estimates; this
allowance reflects implemented scope. Services and CLI orchestration need a
separately reviewed, measured adjustment. This remains a hard check, not a
per-branch exemption.

Markdown uses markdownlint; all fenced Mermaid diagrams must parse with Mermaid.
Bun dependencies are pinned in `package.json` and `bun.lock`. After a toolchain
change, update the Flox lockfile with `flox install` or `flox upgrade`, commit it,
and run the checks again. Do not hand-edit the lockfile.

## Git isolation in hooks

Contributor hooks clear Git-local environment variables before running checks,
so tests that create temporary repositories cannot target the caller's worktree.
`make test` also ignores global and system Git configuration for its child
processes, avoiding user signing prompts and global hooks in fixtures. These
settings do not modify your Git configuration or affect normal commits.
