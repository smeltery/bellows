# Quickstart

Run these commands from a repository that already has GitHub Actions workflows
under `.github/workflows`.

## 1. Initialize Bellows

```sh
bellows init
```

This creates `.bellows.yml` with detected default workflows and runner mappings.
Preview the generated config first with:

```sh
bellows init --dry-run
```

To also create a `pre-push` hook entry:

```sh
bellows init --pre-commit
```

## 2. Inspect Compatibility

```sh
bellows doctor
```

Doctor reports:

- discovered workflows and jobs;
- runner labels and local Docker image mappings;
- dynamic runner expressions such as `${{ matrix.os }}`;
- missing `act` or Docker dependencies;
- known local compatibility warnings.

Use strict mode when warnings should block the local check:

```sh
bellows doctor --strict
```

Use safe fixes to add known runner mappings and local skip recommendations:

```sh
bellows doctor --fix
```

## 3. Run a Workflow

Run the default workflow:

```sh
bellows run
```

Preview the execution plan without Docker:

```sh
bellows run --list
```

Run a specific workflow:

```sh
bellows run --workflow ci.yml
```

Run a specific job:

```sh
bellows run --workflow ci.yml --job test
```

## 4. Enable Pre-Push

If you initialized with `--pre-commit`, install the pre-push hook:

```sh
pre-commit install --hook-type pre-push
```

Then Bellows runs before pushes:

```sh
git push
```
