# syngit suite

In-process integration tests for the syngit operator. No Kind cluster, no Helm, no Gitea but instead, a controller-runtime [envtest] control plane is started in-process alongside two lightweight HTTP(S) git servers (built on `github.com/sosedoff/gitkit`, wrapped in `pkg/envtest`).

## Layout

```
test/e2e/syngit/
  utils/            shared bootstrap + Fixture helper (regular Go package)
  tests/            Ginkgo spec files; single TestEndToEnd entry point
```

## Run the whole suite

```sh
make e2e
```

The first invocation downloads the `setup-envtest` helper and the Kubernetes control-plane binaries into `bin/`; subsequent runs are warm.

## Run a focused subset

Ginkgo's `--focus` takes a regex matched against each spec's full path (Describe + It texts concatenated):

```sh
# Everything whose Describe starts with "02 CommitOnly"
make e2e-focus FOCUS='02 CommitOnly'

# All TLS-related specs
make e2e-focus FOCUS='CA bundle|x509'

# Every spec in a single file
make e2e-file FILE=13_remotesyncer_tls
```

`make e2e-debug FOCUS=...` is like `e2e-focus` but adds `--fail-fast` and full stack traces. Can be useful when iterating on a flaky spec.

## The 3-user model

| Name          | K8s RBAC                                                           | Git ReadWrite |
|---------------|--------------------------------------------------------------------|---------------|
| `admin`       | cluster-admin via `system:masters`                                 | all repos     |
| `developer`   | cluster-admin via ClusterRoleBinding                               | all repos     |
| `restricted`  | narrow ClusterRole (create secrets/RUs/RUBs; named-resource CRUD)  | all repos     |

Baseline git permissions are granted in `Fixture.grantBaseline`. Specs that need a "no git access" identity point a RemoteUser at the secret returned by `Fixture.NewBogusCredsSecret(...)` (the bogus credentials don't match any registered git user, so the push fails authentication).

## Per-spec isolation

Each `It` block constructs a `utils.Fixture` via `suite.NewFixture(ctx)` which:

1. Creates a uniquely-named namespace (`e2e-N`) and registers a `DeferCleanup` to delete it at spec end.
2. Creates a uniquely-named bare repo on the primary git server.
3. Creates `admin-creds` / `developer-creds` / `restricted-creds` basic-auth secrets in the namespace.
4. Grants the baseline permission matrix on the new repo.

Multi-repo specs call `fx.SecondRepo("suffix")`. Specs needing a second git host (for `GitBaseDomainFQDN` diversity, e.g. file 06) use `fx.AltFQDN()` and `fx.AltRepo("suffix")`.
