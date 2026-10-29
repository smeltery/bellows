# Security

Bellows runs GitHub Actions workflow code on your machine through Docker and
`act`.

Treat local workflow execution like running any other project script.

## What Runs Locally

Workflow steps can execute shell commands, Docker actions, and third-party
actions supported by `act`.

Before running Bellows on an unfamiliar repository:

```sh
bellows doctor
```

Then inspect the workflows under:

```text
.github/workflows/
```

## Secrets

Bellows does not forward every environment variable by default.

Use explicit allowlists:

```yaml
secrets:
  mode: env
  allow:
    - NPM_TOKEN
```

Prefer local tokens with limited scope.

## Deployment and Cloud Credentials

Local workflow emulation is not a secure substitute for deployment controls.

Recommended defaults:

- skip deployment jobs locally;
- avoid passing cloud production credentials;
- keep OIDC-dependent deployment jobs in cloud CI;
- keep GitHub branch protection and required checks enabled.

## Cloud CI Remains Authoritative

Bellows reduces wasted cloud runner minutes by catching failures earlier. It
does not replace protected branch checks, GitHub Actions status checks, or
Blacksmith-hosted CI.
