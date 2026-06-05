# Compatibility

This table is generated from Bellows compatibility classifications.

| Status | Feature | Local Behavior | Guidance |
|---|---|---|---|
| supported | `runner` | Mapped GitHub, macOS fallback, and Blacksmith runner labels | Review `.bellows.yml` runner mappings before relying on local parity. |
| partial | `dynamic-runner` | Matrix-driven `runs-on` expressions | Map every possible matrix runner value. |
| partial | `artifact-upload` | `actions/upload-artifact` during local runs | Guard upload steps with `if: ${{ env.ACT != 'true' }}` when needed. |
| partial | `actions-cache` | `actions/cache` local behavior | Treat local cache hits as best effort and keep builds deterministic. |
| partial | `registry-login` | Container registry login actions | Provide local credentials or skip publish jobs. |
| partial | `github-api` | GitHub API calls from workflow steps | Provide a scoped token or guard the step for local runs. |
| unsupported | `runner` | Unmapped runner labels | Add `runner.map` entries before running locally. |
| unsupported | `oidc` | GitHub OIDC token minting | Keep OIDC-backed cloud credential flows in cloud CI or skip locally. |
| unsupported | `environment-protection` | GitHub deployment environments and approvals | Keep protected deployments in cloud CI. |
| unsupported | `cloud-credentials` | Cloud credential setup actions such as AWS OIDC | Skip these jobs locally unless a local credential path is explicit. |
| unsupported | `release-publish` | Release publishing actions | Add release jobs to `skip.jobs`. |

## Action Registry

This table is generated from Bellows action compatibility rules.

| Match | Status | Feature | Guidance |
|---|---|---|---|
| `docker/login-action` | partial | `registry-login` | skip publish jobs or provide local registry credentials |
| `github-script` | partial | `github-api` | guard the step for local runs or provide a scoped token |
| `softprops/action-gh-release` | unsupported | `release-publish` | add this job to skip.jobs |
| `goreleaser/goreleaser-action` | unsupported | `release-publish` | add this job to skip.jobs |
