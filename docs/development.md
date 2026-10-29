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
check scripts (6,000 and 80 lines respectively). Tests are excluded so coverage is not discouraged. Prefer shared
Make targets and existing tools over new wrappers. Any budget increase should
include a rationale in the pull request.

The 6,000-line production budget allocates roughly 4,580 lines for the merged
foundations and native Flox adapter, 330 for reporting, 320 for dedicated Docker
execution, and 400 for orchestration and CLI integration: about 5,630 lines, with
370 lines of bounded integration headroom. This supersedes the 4,700 foundation
and 5,100 reporting allowances. Strict receipt validation, prerequisite closure,
and service lifecycle checks exceeded early estimates. The hard limit applies
to the combined implementation; later features require another measured review.

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
