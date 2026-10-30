# Bellows Docs

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/brand/logos/svg/bellows-reversed.svg">
  <img src="../assets/brand/logos/svg/bellows-primary.svg" alt="Bellows" width="320">
</picture>

Bellows runs existing GitHub Actions workflows locally through Docker and
[`act`](https://github.com/nektos/act), usually from a `pre-push` hook.

Use these docs when adding Bellows to a repository:

- [local-verification.md](local-verification.md): verify committed head/base inputs,
  inspect receipts and recover from failures.
- [native-profiles.md](native-profiles.md): run existing commands through Flox.
- [trusted-reporting.md](trusted-reporting.md): intentionally publish advisory
  statuses with fresh input checks.
- [services.md](services.md): use isolated dedicated Docker daemons.
- [caches.md](caches.md): reuse scoped downloads and build layers with explicit
  cold-cache controls.

- [how-it-works.md](how-it-works.md): execution model and workflow diagrams.
- [releases.md](releases.md): automatic versions and release recovery.

- [installation.md](installation.md): install Bellows and its local runtime
  dependencies.
- [quickstart.md](quickstart.md): initialize a repo and run the first local
  workflow.
- [commands.md](commands.md): command reference for the Bellows CLI.
- [configuration.md](configuration.md): `.bellows.yml` schema and examples.
- [compatibility.md](compatibility.md): supported, partial, and unsupported
  local workflow behavior.
- [schemas/](schemas/): JSON Schemas for config, support bundles, and
  machine-readable output.
- [pre-commit.md](pre-commit.md): run Bellows from the `pre-commit` framework.
- [blacksmith.md](blacksmith.md): use existing Blacksmith runner labels locally.
- [migration.md](migration.md): migrate an existing GitHub Actions repository.
- [../examples](../examples): sample Bellows configs for Go, Node, monorepos,
  deploy skipping, and Blacksmith runners.
- [local-differences.md](local-differences.md): understand local behavior that
  differs from cloud GitHub Actions.
- [secrets.md](secrets.md): pass local secrets intentionally.
- [troubleshooting.md](troubleshooting.md): fix common local runner issues.
- [security.md](security.md): understand the local execution security model.
- [development.md](development.md): develop Bellows itself with Flox.
- [brand assets and guidelines](../assets/brand/README.md): logos, fonts, colors,
  and repository artwork.

## What Bellows Does

Bellows does not replace cloud CI. It catches common workflow failures locally
before a push spends GitHub Actions or Blacksmith runner minutes.

The normal flow is:

1. Keep existing `.github/workflows/*.yml` files.
2. Run `bellows init`.
3. Review `.bellows.yml`.
4. Run `bellows doctor`.
5. Run `bellows run` or use the generated `pre-push` hook.

Cloud CI should remain the final authority for pull requests and protected
branches.
