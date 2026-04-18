# syngit suite

In-process integration tests for the syngit operator. No Kind cluster, no
Helm, no Gitea — a controller-runtime [envtest] control plane is started
in-process alongside two lightweight HTTP(S) git servers (built on
`github.com/sosedoff/gitkit`, wrapped in `pkg/envtest`).

## Layout

```
test/e2e/syngit/
  utils/            shared bootstrap + Fixture helper (regular Go package)
  tests/            Ginkgo spec files (01_*…30_*); single TestEndToEnd entry point
```

## Run the whole suite

```sh
make e2e
```

The first invocation downloads the `setup-envtest` helper and the
Kubernetes control-plane binaries into `bin/`; subsequent runs are warm.

Equivalent one-liner:

```sh
KUBEBUILDER_ASSETS="$(bin/setup-envtest-latest use 1.35.0 --bin-dir bin -p path)" \
  bin/ginkgo-v2.28.1 -timeout 25m -v ./test/e2e/syngit/tests
```

## Run a focused subset

Ginkgo's `--focus` takes a regex matched against each spec's full path
(Describe + It texts concatenated):

```sh
# Everything whose Describe starts with "02 CommitOnly"
make e2e-focus FOCUS='02 CommitOnly'

# All TLS-related specs
make e2e-focus FOCUS='CA bundle|x509'

# Every spec in a single file
make e2e-file FILE=13_remotesyncer_tls
```

`make e2e-debug FOCUS=...` is like `e2e-focus` but adds `--fail-fast` and
full stack traces. Can be useful when iterating on a flaky spec.

## The 3-user model

| Name          | K8s RBAC                                                           | Git ReadWrite |
|---------------|--------------------------------------------------------------------|---------------|
| `admin`       | cluster-admin via `system:masters`                                 | all repos     |
| `developer`   | cluster-admin via ClusterRoleBinding                               | all repos     |
| `restricted`  | narrow ClusterRole (create secrets/RUs/RUBs; named-resource CRUD)  | all repos     |

Baseline git permissions are granted in `Fixture.grantBaseline`. Specs
that need a "no git access" identity point a RemoteUser at the secret
returned by `Fixture.NewBogusCredsSecret(...)` — the bogus credentials
don't match any registered git user, so the push fails authentication.

## Per-spec isolation

Each `It` block constructs a `utils.Fixture` via `suite.NewFixture(ctx)`
which:

1. Creates a uniquely-named namespace (`e2e-N`) and registers a
   `DeferCleanup` to delete it at spec end.
2. Creates a uniquely-named bare repo on the primary git server.
3. Creates `admin-creds` / `developer-creds` / `restricted-creds`
   basic-auth secrets in the namespace.
4. Grants the baseline permission matrix on the new repo.

Multi-repo specs call `fx.SecondRepo("suffix")`. Specs needing a second
git host (for `GitBaseDomainFQDN` diversity, e.g. file 06) use
`fx.AltFQDN()` and `fx.AltRepo("suffix")`.

## File index

| # | File                                        | What it covers                                                  |
|---|---------------------------------------------|-----------------------------------------------------------------|
| 01 | `01_setup_remoteusers_test.go`             | RemoteUser + managed RemoteUserBinding creation                 |
| 02 | `02_commitonly_cm_test.go`                 | CommitOnly blocks cluster apply                                 |
| 03 | `03_commitapply_cm_test.go`                | CommitApply lifecycle (create + delete)                         |
| 04 | `04_excludedfields_test.go`                | ExcludedFields inline and via ConfigMap ref                     |
| 05 | `05_defaultuser_test.go`                   | UseDefaultUser fallback with bogus/valid creds                  |
| 06 | `06_objects_lifecycle_test.go`             | RemoteUserBinding aggregation across two distinct git hosts     |
| 07 | `07_bypass_subject_test.go`                | BypassInterceptionSubjects (CommitApply + CommitOnly)           |
| 08 | `08_webhook_rbac_test.go`                  | RBAC enforcement on RemoteSyncer scope                          |
| 09 | `09_multi_remotesyncer_test.go`            | Multi-repo push; lock-ref collision                             |
| 10 | `10_remoteuser_secret_access_test.go`      | RemoteUser rejected when referenced secret is out of RBAC       |
| 11 | `11_remoteuserbinding_permissions_test.go` | RUB rejected when referenced RU is out of RBAC                  |
| 12 | `12_remoteuserbinding_managed_test.go`     | Managed-RUB name collision produces numeric suffix              |
| 13 | `13_remotesyncer_tls_test.go`              | Custom CA bundle (same-ns / explicit-ns / operator-ns / none)   |
| 14 | `14_remoteuserbinding_cross_user_test.go`  | Cross-user RemoteUser update denial                             |
| 15 | `15_conversion_webhook_test.go`            | v1beta3 → v1beta4 conversion webhook                            |
| 16 | `16_wrong_reference_value_test.go`         | Fake secret / RU / RT / default-RU / default-RT error paths     |
| 17 | `17_remoteuserbinding_selector_test.go`    | `RemoteUserBindingSelector` match / mismatch / absent           |
| 18 | `18_cluster_default_excludedfields_test.go`| Cluster-wide excluded-fields ConfigMap in operator ns           |
| 19 | `19_remotetarget_same_repo_branch_test.go` | RemoteTarget validation (same upstream / empty merge strategy)  |
| 20 | `20_objects_without_annotations_test.go`   | Full workflow without syngit annotations                        |
| 21 | `21_remotetarget_one_different_branch_test.go` | OneUserOneBranch (OneTarget + MultipleTarget)              |
| 22 | `22_remotetarget_multiple_different_branch_test.go` | Multi-branch push; wrong-strategy denial               |
| 23 | `23_remotetarget_selector_test.go`         | `RemoteTargetSelector` match / multi-match / absent / orphan    |
| 24 | `24_remotesyncers_scope_remotetarget_test.go` | Two RemoteSyncers scoping one RemoteTarget, release cycle    |
| 25 | `25_fastforward_merge_test.go`             | `TryFastForwardOrDie` pulls upstream before pushing             |
| 26 | `26_hardreset_merge_test.go`               | `TryHardResetOrDie` with / without external merge               |
| 27 | `27_fastforward_hardreset_merge_test.go`   | `TryFastForwardOrHardReset` fall-back                           |
| 28 | `28_remoteuser_created_after_test.go`      | Managed RT back-fill when RU is created after the RemoteSyncer  |
| 29 | `29_pattern_remove_test.go`                | Dynamic add/remove of branch & user-specific annotations        |
| 30 | `30_push_to_existing_path_test.go`         | Existing-path & multi-document YAML reconciliation              |

[envtest]: https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest
