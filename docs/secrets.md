# Secrets

Bellows does not pass all local environment variables into workflows by
default.

This is intentional. GitHub Actions workflows can run arbitrary commands, so
local secrets should be explicit.

## No Secrets

Default behavior:

```yaml
secrets:
  mode: none
```

## Allow Environment Secrets

To pass selected environment variables to `act`, use `mode: env` and list the
allowed names:

```yaml
secrets:
  mode: env
  allow:
    - GITHUB_TOKEN
    - NPM_TOKEN
```

Then run Bellows with those variables set:

```sh
NPM_TOKEN=... bellows run
```

Bellows passes only the names listed under `allow`.

Audit workflow secret references:

```sh
bellows secrets audit
```

Example output:

```text
[allowed] .github/workflows/ci.yml job publish step Publish uses NPM_TOKEN
[blocked] .github/workflows/ci.yml job publish step Publish uses OTHER_TOKEN
```

## Recommended Practice

- Use low-privilege local tokens.
- Avoid deployment credentials in local runs.
- Skip deploy jobs unless you have an explicit local deployment simulation.
- Do not commit `.env` files or credential files.
