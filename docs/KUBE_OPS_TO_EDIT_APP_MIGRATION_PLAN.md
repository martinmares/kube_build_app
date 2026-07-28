# kube-ops to kube-edit migration plan

> Status: **direction pivot as of 2026-05-20**. `kube-ops-app` remains only a prototype/sandbox. Do not continue it as a standalone product unless this decision is explicitly reversed.

## Decision

The standalone `kube-ops-app` direction overlaps too much with responsibilities that already belong elsewhere:

- `kube-edit-app` is the environment repository workbench: edit, review, build preview, changed files, safe Git-oriented workflow and read-only supporting diagnostics.
- `kube-build-app` is the deterministic render engine.
- ArgoCD remains the sync/reconcile engine for rendered manifests.
- release/image bundle and digest publication belongs to Git-managed manifests and `oci-toolbox`, not this repository.

The useful outcome of the `kube-ops-app` prototype is not a new product binary. It is a set of small concepts that can be migrated into `kube-edit-app` where they improve the existing workbench.

## Non-goals

Do not implement these in this repository as part of the migration:

- ArgoCD replacement.
- Kubernetes apply/sync/prune engine.
- production truth based on `.tmp/kube-ops-state/state.json`.
- `mark-applied` workflow as product behavior.
- release/image manifest management.
- L2 operations actions such as restart, shutdown, startup or force-conflicts.

Those may be revisited later only with a separate design.

## Prototype Pieces Worth Migrating

### 1. Read-only cluster panel

Status: first `kube-edit-app` implementation added.

Source idea:

- `internal/opsapp/cluster`
- `kube-ops-app server` cluster status card
- `scripts/ops-fixture-cluster`

Target in `kube-edit-app`:

- Optional read-only runtime panel for the selected environment.
- Show namespace, deployments, pods, services, readiness counts and last refresh time.
- Auto-refresh periodically after environment selection.
- Keep it read-only; no apply, no scale, no restart.
- Enabled explicitly by `kube-edit-app serve --cluster-status`; hidden by default.

Open design points:

- Whether this belongs in the existing Build page or a new Runtime/Cluster tab.
- Whether namespace comes from environment variables, explicit config, or a small mapping file.
- Whether the server should use default kubeconfig/context or require explicit flags.

Acceptance criteria:

- Disabled or hidden when no cluster access is configured.
- Errors are visible but non-blocking.
- No Kubernetes write verb is needed.

### 2. Render/status panel

Source idea:

- resolved commit
- manifest digest
- desired/applied comparison UI

Target in `kube-edit-app`:

- Build preview can expose render digest and generated-manifest metadata.
- Use this as diagnostic context, not as deployment authority.
- Do not migrate local applied-state semantics.

Acceptance criteria:

- `kube-edit-app` remains a repository/build workbench.
- The UI makes it clear that ArgoCD is still responsible for sync/reconcile.

### 3. Fixture helpers

Source idea:

- `scripts/ops-fixture`
- `scripts/ops-fixture-cluster`
- `just ops-fixture`
- `just ops-fixture-cluster`

Target:

- Keep existing helpers as prototype scaffolding for now.
- Later rename or generalize them if they become useful for `kube-edit-app` development.

Candidate future names:

- `edit-fixture`
- `runtime-fixture`
- `cluster-fixture`

Acceptance criteria:

- Test/demo fixtures must not require the real customer environment repository.
- Fixture commands should be safe to run repeatedly.

### 4. Line unified diff

Source idea:

- `internal/opsapp/diff`

Target in `kube-edit-app`:

- Reuse only if the existing Git diff UI does not cover generated/rendered comparisons.
- Candidate use case: diff rendered build preview outputs against a previous generated snapshot.

Acceptance criteria:

- Do not duplicate the Changed files Git diff implementation unless there is a concrete gap.
- Keep output deterministic and easy to test.

### 5. UX Patterns

Source idea:

- environment list + selected environment detail
- status cards
- compact read-only runtime tables

Target in `kube-edit-app`:

- Prefer automatic refresh for read-only status instead of requiring a manual tab switch.
- Keep debug JSON collapsible or secondary.
- Do not let diagnostics dominate editing/review workflows.

## Things To Drop

These prototype parts should not be promoted:

- `kube-ops-app` as a standalone production server.
- `env mark-applied` as operational truth.
- local applied snapshot as source of truth.
- sync/apply workflow in this binary.
- production-facing release controls.

## Migration Order

1. Record the pivot in docs and handoff context.
2. Leave existing `kube-ops-app` code as sandbox unless it blocks builds/tests.
3. Add a read-only cluster/runtime panel to `kube-edit-app` behind explicit optional configuration.
4. Add render digest/status metadata to the existing Build preview UI if it improves clarity.
5. Decide whether fixture helpers should be renamed/generalized after the first migrated feature exists.
6. Revisit whether `cmd/kube-ops-app` should be removed, hidden, or kept as an internal prototype command.

## Recommended Next Step

Implement the read-only runtime/cluster panel inside `kube-edit-app` first. It is the highest-value migration because it complements the existing repository editor without replacing ArgoCD or changing the build contract.
