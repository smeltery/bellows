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
check scripts (4,300 and 80 lines respectively). Tests are excluded so coverage is not discouraged. Prefer shared
Make targets and existing tools over new wrappers. Any budget increase should
include a rationale in the pull request.

The 4,300-line production budget reserves a bounded foundation envelope above
the 3,376-line baseline: roughly 150 contract, 90 Git-discovery, 200 worktree,
300 receipt-validation and 150 selection lines. It remains a hard check; native
adapters and reporting require separately reviewed, measured adjustments.

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
