# kube-build-app sync metadata contract

Status: design draft, confirmed direction on 2026-05-22.

## Core Decision

`kube-build-app` owns deterministic manifest rendering and the sync metadata contract.

`kube-deploy-sync` must not understand application metamodel internals and must not do Kubernetes field-by-field drift detection. It should compare and apply rendered objects by metadata produced by `kube-build-app`.

```text
kube-build-app    -> deterministic manifests + sync metadata
kube-deploy-sync  -> compare/apply/prune by sync metadata
kube-edit-app     -> edit/review/build preview
```

Explicitly avoid:

- field diff logic for `Deployment`, `Service`, `Ingress`, `ConfigMap` internals,
- build/render logic inside `kube-deploy-sync`,
- sync/apply logic inside `kube-build-app`,
- database-backed field-level drift state as the core sync model.

## Why

A sync engine that compares selected Kubernetes fields becomes incomplete immediately. Every new field, controller default, mutation webhook, generated value and Kubernetes kind adds another special case.

The renderer already knows the intended object identity and intended object content. Therefore it should emit stable identity and content fingerprint metadata directly into each rendered object.

## Metadata Placement

Use both labels and annotations, following Kubernetes conventions:

- labels: short values used for selecting/listing managed objects,
- annotations: full metadata values, hashes and ordering data.

Recommended default prefix:

```text
kube-build-app.io
```

Recommended generated metadata:

```yaml
metadata:
  labels:
    app.kubernetes.io/managed-by: kube-build-app
    kube-build-app.io/sync-set: dev
    kube-build-app.io/sync-id-hash: "a1b2c3d4e5f6"
  annotations:
    kube-build-app.io/sync-id: "dev/app/api/deployment"
    kube-build-app.io/sync-hash: "sha256:..."
    kube-build-app.io/sync-order: "120"
```

Rationale:

- Full `sync-id` belongs in an annotation because label values have strict length and character constraints.
- `sync-id-hash` is a short label suitable for selection/indexing.
- `sync-set` groups all objects belonging to one rendered target, usually an environment or release scope.
- `sync-hash` is the desired object fingerprint.
- `sync-order` is an ordering hint for sync engines and adapters.

## Required Metadata Semantics

### `sync-set`

A stable group name for one desired rendered set.

Typical values:

```text
dev
test
prod
nac-dev
```

`kube-deploy-sync` can list live managed objects by:

```text
app.kubernetes.io/managed-by=kube-build-app
kube-build-app.io/sync-set=<sync-set>
```

### `sync-id`

Stable object identity. It must not change when object content changes.

For regular objects, identity can be derived from environment, kind, namespace and logical name:

```text
<sync-set>/<kind>/<namespace>/<name>
```

Examples:

```text
dev/Deployment/nac-test/api
dev/Service/nac-test/api
dev/Ingress/nac-test/api
```

For generated content-addressed ConfigMaps, identity must not use the generated ConfigMap name because that name changes with content. It should use the logical source identity instead:

```text
<sync-set>/app/<app>/container/<container>/asset/<source-file>:<mount-path>
```

Example:

```text
dev/app/api/container/api/asset/assets/app.conf:/app/app.conf
```

The generated object name may change from:

```text
api-asset-11111111
```

to:

```text
api-asset-22222222
```

but `sync-id` stays the same.

### `sync-id-hash`

A short deterministic hash of `sync-id`, safe for label values.

Recommended:

```text
hex(sha256(sync-id))[0:16]
```

### `sync-hash`

A deterministic hash of the desired object content.

Recommended:

```text
sha256:<canonical-object-hash>
```

Hash input should be canonical JSON after removing runtime and volatile fields:

- `status`,
- `metadata.creationTimestamp`,
- `metadata.resourceVersion`,
- `metadata.uid`,
- `metadata.generation`,
- `metadata.managedFields`,
- current `kube-build-app.io/sync-hash` value.

Open decision: whether adapter-specific annotations such as `argocd.argoproj.io/sync-wave` are included in `sync-hash`. Default recommendation is to include all desired metadata except `sync-hash` itself, because a desired metadata change should usually be visible to sync.

### `sync-order`

Ordering hint for apply/sync.

Recommended ranges:

```text
000 namespace / prerequisites
100 config and secrets
200 workloads
300 services
400 ingress/routes
900 post-sync/support objects
```

The value should be emitted as a string annotation for portability:

```yaml
kube-build-app.io/sync-order: "200"
```

## kube-deploy-sync Algorithm

The sync engine should only depend on the metadata contract.

Input:

- desired rendered manifests,
- live objects in the same `sync-set`.

Algorithm:

```text
parse desired objects
require sync metadata on every desired object
index desired by sync-id
list live objects by managed-by + sync-set
index live by sync-id

for each desired sync-id:
  live missing        -> create/apply desired
  live hash equal     -> no-op
  live hash different -> apply desired

for each live sync-id not present in desired:
  mark prune candidate
```

No field-level semantic diff is allowed in the core sync decision.

## ConfigMap Rollout Strategy

Generated ConfigMaps may be content-addressed by name. This requires special handling, but still by metadata, not by field diff.

When desired and live share `sync-id` but have different object names:

1. Create the new ConfigMap first.
2. Apply workloads that reference the new ConfigMap name.
3. Wait for rollout if configured.
4. Delete old ConfigMap objects with the same `sync-id` that are no longer desired.

This preserves safe rollout behavior while keeping stable object identity.

## Profiles / Adapters

`kube-build-app` should support sync metadata profiles. Profiles define metadata key names and optional adapter-specific output.

Initial profiles:

```text
none              -> default today, no sync metadata
kube-deploy-sync  -> full sync metadata contract
argocd            -> sync metadata plus ArgoCD-compatible sync wave output
```

Possible CLI shape:

```bash
kube-build-app build \
  --sync-metadata-profile kube-deploy-sync
```

```bash
kube-build-app build \
  --sync-metadata-profile argocd
```

Possible config shape:

```yaml
sync_metadata:
  profile: kube-deploy-sync
  prefix: kube-build-app.io
  sync_set: dev
```

For ArgoCD adapter:

```yaml
sync_metadata:
  profile: argocd
  prefix: kube-build-app.io
  sync_set: dev
  argocd:
    sync_wave: true
```

The ArgoCD profile may map `sync-order` to:

```yaml
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "200"
```

Open decision: whether existing app-level `argocd.argoproj.io/sync-wave` remains authoritative, or whether it is generated from `sync-order` when profile `argocd` is active.

## Required Invariants

- Every rendered object in a sync metadata profile has `sync-set`, `sync-id`, `sync-id-hash`, `sync-hash` and `sync-order`.
- `sync-id` is stable across content changes.
- `sync-hash` changes when the desired canonical object changes.
- YAML formatting changes do not change `sync-hash`.
- Runtime Kubernetes fields do not change `sync-hash`.
- The contract is generic and does not mention `Deployment`, `Service`, `Ingress` or `ConfigMap` fields as sync decision inputs.

## Implementation Plan

1. Add internal canonical object hash helper.
2. Add sync metadata profile options to `buildapp.Options` and CLI flags.
3. Add object metadata injection right before YAML marshal/write.
4. Define stable `sync-id` generators for each generated object class:
   - deployment/statefulset,
   - service,
   - external ingress/route,
   - generated asset ConfigMap,
   - shared asset ConfigMap,
   - pod disruption budget,
   - autoscaling object.
5. Add tests proving:
   - metadata exists on all generated objects,
   - `sync-id` remains stable for content-addressed ConfigMaps,
   - `sync-hash` changes when object content changes,
   - YAML formatting does not affect `sync-hash`,
   - ArgoCD profile emits expected adapter annotations.

## Out Of Scope

- Applying objects to Kubernetes.
- Pruning live objects.
- Persisting sync history.
- Replacing ArgoCD in `kube-build-app`.
- Field-level diff of Kubernetes object internals.
