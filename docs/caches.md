# Reusing downloads, results, and Docker layers

Verification executes every check by default. Bellows never restores `node_modules`
or publisher credentials. Successful pure native checks can be reused only with
the separate result-cache opt-in described below.
Its optional native cache adapter points npm at a private download store using
`npm_config_cache`; npm logs are disabled for that store. npm validates cached
content and refetches corrupt data when online. See the [npm cache documentation](https://docs.npmjs.com/cli/cache/).

## Use the cache

Opt in for a committed native profile:

```sh
bellows verify --repository example/project --base origin/main \
  --profile native --download-cache
bellows verify --repository example/project --base origin/main \
  --profile native --cold-cache
```

Both flags require `--profile`. `--cold-cache` creates a disposable empty store
without requiring `--download-cache`. Reusable stores live under the private user
cache directory at `bellows/downloads`. Cold and warm operation have the same
semantic environment fingerprint; temporary filesystem paths are not included.
Docker verification automatically binds its dedicated daemon to the repository,
and rejects these npm-cache flags rather than claiming a cold Docker run.

## Scope and concurrency

The key includes repository, execution platform, toolchain fingerprint and
dependency fingerprint. Every identity field must be resolved before opening a
store. Changing a commit alone does not discard unchanged downloads; changing the
repository, platform, dependency lock or toolchain creates another store.

`Dependencies(preparedRoot)` hashes committed paths and Git blob IDs for npm
manifests, lockfiles and `.npmrc` across workspaces. It ignores source-only edits,
untracked files and installed `node_modules`; gitlinks invalidate conservatively,
and symlink dependency inputs are rejected. Secret configuration contents never
appear in its output. This is a download-cache scope, not test-result identity:
receipts still require the complete Git tree, config and other input fingerprints.
Local `file:` dependencies are freshly installed, never restored from this cache.

```mermaid
flowchart LR
  I[Resolved verification identity] --> K[Scoped download-store key]
  K --> L{Exclusive lease available?}
  L -->|Yes| S[Reusable npm download store]
  L -->|No| T[Private disposable cold store]
  S --> N[Fresh npm ci and checks]
  T --> N
```

Only one run uses a reusable entry at a time. Another run falls back to a private
cold store rather than waiting or sharing partially updated state. Missing or
unsafe cache metadata also falls back to a disposable store. Root directories
must be private and cannot be symlinks. These controls prevent accidental reuse
between repositories; they do not isolate hostile programs running as the same
operating-system user.

`cache.Open(root, identity, cold)` returns a lease. Append its `Env()` entries to
the native execution environment, and compose `Close()` with executor cleanup.
`State()` reports `cold`, `reuse`, `busy` or `corrupt`; `reuse` means an existing
store was available, not that npm found every requested package in it. Operational
cache paths do not replace semantic input fingerprints.

The CLI calls `Prune(root, 32, 7*24*time.Hour)` before opening a reusable native cache to
keep at most 32 inactive owned entries and
expire inactive entries older than seven days. Active leases are excluded.
Pruning never deletes unrelated paths or follows a cached directory symlink.
A crashed process can leave a lease; an operator must confirm the process ended
before removing that lease. Bellows does not break leases based on age alone.
Disposable stores are deleted on close; interrupted process cleanup remains the
operator's responsibility.

## Cold runs and Docker

An explicit cold npm run always gets a new disposable store. It never reads or
writes a reusable store. There is no restoration of installed dependencies.

Container npm-cache reuse is unsupported without an explicit mount interface.
Bellows does not mount a workstation cache into an unrelated daemon or create a
cache service. Docker's built-in BuildKit cache remains inside the dedicated
Linux daemon described in [services](services.md).

Before reusing Docker layers, call `BindDaemon(root, executor.DaemonID(), repo)`.
The immutable binding rejects reuse by another repository, including another
endpoint that resolves to the same daemon. It does not prove the daemon is new.
A cold Docker run requires a freshly provisioned owned daemon; an arbitrary
existing daemon cannot claim cold parity from an empty filesystem binding record.
The cache adapter never prunes Docker resources. Destroy only the daemon you own
when its retention period ends, then remove its binding record.

## Reproduce the measurements

The npm fixture serves one synthetic tarball from a loopback HTTP server and uses
an integrity-pinned lockfile. It installs into a new project directory each time:

```sh
flox activate -- env BELLOWS_NPM_INTEGRATION=1 \
  go test -v ./internal/verification/cache -run TestNPMColdWarmAndCorruption
```

Observed on macOS arm64 with Node v20.9.0/npm 10.8.2:

| Run | Elapsed | Tarball downloads | Result |
| --- | ---: | ---: | --- |
| Cold | 300.5 ms | 1 | `42` |
| Warm, offline | 220.9 ms | 0 | `42` |
| Corrupted, offline | Not compared | 0 | Explicit `ENOTCACHED` failure |
| Online repair | 240.2 ms | 1 | `42` |

An explicit cold bypass fetched the tarball again. These single-run timings
measure this small local fixture, not a hardware-independent speedup or production
benchmark. The offline warm run and request count provide the cache-hit evidence.

With privileged Docker available, run the BuildKit fixture:

```sh
flox activate -- env BELLOWS_SERVICE_INTEGRATION=1 \
  go test -v -timeout 15m ./internal/verification/cache \
  -run TestDockerCacheIntegration
```

It creates one owned DinD daemon, builds a synthetic scratch image cold and warm,
checks identical image IDs and a `CACHED` build step, rejects cross-repository
binding, and verifies no containers, volumes or custom networks remain. The shared
Ubuntu service CI job runs this fixture alongside concurrent service lifecycle
tests. Docker Desktop cache parity remains unmeasured.

## Reuse pure native results

Result reuse is off by default. Mark only deterministic checks that need no
external services and produce no files or state needed by later checks:

```yaml
profiles:
  native:
    executor: flox
    checks:
      harness:
        argv: [node, --test, scripts/harness.test.mjs]
        cacheable: true
```

```sh
bellows verify --repository example/project --base origin/main \
  --profile native --reuse-results
bellows verify --repository example/project --base origin/main \
  --profile native --reuse-results --force-rerun
```

`cacheable` must be a boolean and defaults to false. `--reuse-results` requires
`--profile`; `--force-rerun` requires `--reuse-results` and bypasses reads while
refreshing eligible results. Neither flag enables npm download caching, and
`--cold-cache` alone does not force tests to execute. Add `--force-rerun` when
measuring fresh execution with result reuse enabled.

Do not mark installation, build preparation, migrations, service startup, image
builds, network checks, or checks with required output artifacts as cacheable.
Bellows does not restore their side effects. The operator is responsible for
choosing pure checks; a successful exit does not establish purity.

The private `bellows/results` store binds entries to the complete verification
identity, check definition, and executing Bellows binary. Head, base, tree,
configuration, toolchain, platform, environment, or binary changes cause a miss.
Unlike downloads, results do not survive source-only changes. Entries expire
after 24 hours; retention is bounded to 128 entries and 16 MiB. Missing, expired,
or corrupt entries execute normally. Operational cache failures warn and execute
rather than count as a successful check.

A reused check still requires passing prerequisites and a fresh run lifecycle.
Its new receipt records reuse provenance instead of claiming fresh execution.
Only locally authenticated successful evidence is eligible; importing a JSON
receipt does not seed the store. Same-user hostile code can access the local
key, so this is accidental-corruption protection on a trusted workstation, not
an isolation or remote-attestation boundary. Keep hosted CI and required checks.

For comparisons, keep the launcher path, binary, committed source, `PATH`,
`HOME`, and `TMPDIR` stable and use `umask 077`. Record hashes of those three
environment values, not a complete environment dump. Compare cache-hit evidence
and per-check timing separately from whole-pipeline timing; skipped pure work
cannot explain unrelated service or download variability.
