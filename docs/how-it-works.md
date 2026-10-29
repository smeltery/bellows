# How Bellows works

Bellows reads GitHub Actions workflow files and `.bellows.yml`, resolves a local
execution plan, then calls act to run jobs in Docker containers.

```mermaid
flowchart LR
    workflow[GitHub Actions workflows] --> plan[Local execution plan]
    config[.bellows.yml] --> plan
    plan --> preview[Preview and diagnostics]
    plan --> act[act]
    act --> docker[Docker runner containers]
    docker --> result[Local result]
```

## Plan before running

Use `bellows run --list --estimate` to inspect jobs and skipped work without
starting Docker. `bellows doctor` checks dependencies and compatibility;
`bellows explain --format json` exposes the plan to other tools.
See [commands](commands.md) for the full interface.

## Runner mappings

Runner labels map to container images through `runner.map`. Blacksmith labels
can use the same local image as Ubuntu jobs. Mapping a macOS label to a Linux
image does not emulate macOS: native platform checks still belong in cloud CI.
See [compatibility](compatibility.md) and [local differences](local-differences.md).

## Catch failures before pushing

```mermaid
flowchart TD
    edit[Edit code or workflows] --> hook[Pre-push hook]
    hook --> check[Bellows local checks]
    check --> outcome{Passed?}
    outcome -->|No| edit
    outcome -->|Yes| push[Push to GitHub]
    push --> ci[Cloud CI]
```

Install a hook using the [pre-commit guide](pre-commit.md). Local checks execute
repository code with access to the Docker runtime; run trusted workflows and
pass only required secrets. See [security](security.md) and [secrets](secrets.md).
