# Go rewrite parity plan

This document describes how to use the current Ruby implementation as the executable reference for a future Go rewrite.

The goal is not to rewrite the tool and then manually inspect a few outputs. The goal is to prove that the Go implementation produces the same Kubernetes manifests for real environments.

## TL;DR

The Ruby implementation is the reference implementation.

Before switching production usage to Go:

1. Build each selected real environment with Ruby into a temporary directory.
2. Build the same environment with Go into another temporary directory.
3. Compare the generated file set.
4. Compare each YAML/JSON file semantically, not as raw text.
5. Run optional Kubernetes schema validation with `kubeconform`.
6. Treat every difference as either a Go bug or an explicitly approved behavior change.

The Go rewrite is justified by productization: single binary, simpler distribution, smaller Docker image, no Ruby runtime, easier handover to other teams.

## Reference inputs

Use real environment repositories as parity inputs.

## Related metamodel UI

There is also a Rust UI project:

```text
/Users/mares/Development/Src/Rust/kube-environments-ui
```

It is a workspace with:

```text
kube-env-core
kube-env-web
kube-env-gui
```

`kube-env-core` already contains an independent domain view over the same environment repository structure:

- environment discovery
- app listing and app documents
- asset listing and special root assets
- `apps/_defaults.yml`
- `env.unsecured.json`
- `env.secured.json`
- `assets.unsecured.json`
- `assets.secured.json`
- structured defaults editor model
- structured env editor model
- structured virtual asset editor model
- partial effective vars / container vars preview logic

This is useful for the Go rewrite, but it must not become the primary rendering reference.

Decision:

- Ruby `kube_build_app` remains the rendering reference.
- Rust `kube-env-core` is a useful second opinion for repository discovery, editor-facing model shape and UI semantics.
- Go must match Ruby output for generated manifests.
- UI model and Go model should use the same vocabulary where possible, but rendering parity is still measured against Ruby output.

Practical implication:

- When designing Go structs, compare them with both Ruby classes and Rust `kube-env-core` structs.
- When behavior differs between Ruby and Rust UI preview logic, Ruby wins for rendering.
- Rust UI preview logic can reveal useful intended semantics, especially for `_defaults.yml`, `vars`, `container_vars` and virtual assets.

### CETIN TSM

Root:

```text
/Users/mares/Development/Src/Ruby/tsm/cetin/tsm-environments
```

Environments:

```text
test
ref
ref2
prod
prod2
```

### O2 NAC

Root:

```text
/Users/mares/Development/O2/NAC/dtl-nac/environments
```

Environments:

```text
dev
prod
```

### O2 Slovakia NAC / TSM

Root:

```text
/Users/mares/Development/O2sk/NAC/tsm-environments
```

Environments:

```text
tst
ppt
prod
```

## Current manual build shape

Example for CETIN TSM `test`:

```bash
export TSM_ENV=test
export TSM_RELEASE_ID=2025.08.18.1

cd "$HOME/Development/Src/Ruby/tsm/cetin"

kube_build_app \
  -e "${TSM_ENV}" \
  -t "$HOME/Development/Src/Ruby/tsm/cetin/tsm-deploy/deploy/${TSM_ENV}"

eval "$(encjson env -f environments/${TSM_ENV}/env.unsecured.json)"
eval "$(encjson env -f environments/${TSM_ENV}/env.secured.json)"

find "./tsm-deploy/deploy/${TSM_ENV}/deployments" -type f -name "*.yml" \
  | awk '{print "apply-env -f "$0" -w"}' \
  | sh

find "./tsm-deploy/deploy/${TSM_ENV}/services/external" -type f -name "*.yml" \
  | awk '{print "apply-env -f "$0" -w"}' \
  | sh
```

For parity testing, do the same build into temporary Ruby and Go output directories instead of the real deployment directory.

## Important comparison rule

Do not compare YAML as raw text.

Ruby and Go may serialize mappings in different key order. That is not a functional difference.

The parity comparison should:

- compare relative file paths
- parse YAML files into data structures
- parse JSON files into data structures
- compare normalized objects
- preserve scalar values and types exactly
- preserve Kubernetes object names, namespaces, labels, annotations, ports, resources, probes, volumes and mounts exactly

Raw text comparison is acceptable only for files that are intentionally text payloads inside ConfigMaps and Secrets.

## Environment variables

The parity runner must make environment input explicit.

Required examples:

```bash
TSM_RELEASE_ID=2025.08.18.1
TSM_ENV=test
```

If the current workflow loads variables using:

```bash
eval "$(encjson env -f environments/${TSM_ENV}/env.unsecured.json)"
eval "$(encjson env -f environments/${TSM_ENV}/env.secured.json)"
```

then the parity runner must do the same for both Ruby and Go builds.

The same process environment must be used for both builds.

## Suggested parity command shape

Initial script:

```bash
./scripts/parity-build \
  --name cetin-test \
  --root /Users/mares/Development/Src/Ruby/tsm/cetin/tsm-environments \
  --env test \
  --release-id 2025.08.18.1
```

When a Go renderer binary is available:

```bash
./scripts/parity-build \
  --name cetin-test \
  --root /Users/mares/Development/Src/Ruby/tsm/cetin/tsm-environments \
  --env test \
  --release-id 2025.08.18.1 \
  --go-bin ./dist/kube-build-app
```

Expected internal behavior:

```text
/tmp/kube-build-app-parity/<case>/ruby
/tmp/kube-build-app-parity/<case>/go
```

Then:

```text
ruby kube_build_app -> ruby output
go kube-build-app   -> go output
compare file sets
compare normalized YAML/JSON content
optionally run kubeconform
```

## Optional kubeconform validation

After generating manifests, run:

```bash
kubeconform -strict -summary -output json /tmp/kube-build-app-parity/<case>/ruby
kubeconform -strict -summary -output json /tmp/kube-build-app-parity/<case>/go
```

Both outputs should pass or fail identically.

If schema validation fails for both because of known custom resources or environment-specific limitations, the failure should be documented and filtered explicitly.

## What must be identical

At minimum:

- generated directory structure
- generated file names
- Kubernetes object `apiVersion`, `kind`, `metadata.name`, `metadata.namespace`
- labels and annotations
- Deployment / StatefulSet pod template
- containers, initContainers, images, commands and args
- variables and env value sources
- resources
- probes
- ports
- services
- ingress objects
- volumes and volume mounts
- ConfigMap names and data
- asset ConfigMap digest suffixes
- rollout checksum annotations
- inventory JSON output

## Known high-risk areas

These areas are easy to accidentally change during rewrite:

- env resolution order
- `apps/_defaults.yml` merge semantics
- `vars` replacement and `remove: true`
- `container_vars` wildcard and container-specific overrides
- asset `transform: true` vs `transform: false`
- asset ConfigMap CRC32 naming
- rollout checksum SHA256 annotations
- YAML scalar types, especially numeric ports
- `raw` passthrough fields
- service/external service generation
- mtls generated asset mounts
- profile-based replica overrides
- ignored apps

## Current Go renderer coverage

Implemented in the initial Go renderer skeleton:

- basic Deployment and StatefulSet generation
- PodDisruptionBudget generation
- `apps/_defaults.yml` recursive merge
- `vars` and `container_vars` override/remove semantics
- `ignore`, `disable_create_service` and `disable_shared_assets`
- `replica-profiles.yml` via `-p`, `REPLICA_PROFILE` or `defaults.profile`
- standard file assets, transformed assets, binary assets and MTLS assets
- temp/PVC/NFS/hostPath assets
- Helm escaping support for assets
- `shared.assets.yml`
- rollout checksum annotations
- services for `expose_as`
- external Route/Ingress generation for `expose_as[].external`
- inventory JSON output via `-i`
- tools initContainers and `/app/tools` mount
- cgroup exporter default variable injection
- `validate` subcommand for current model validation rules
- release manifest image overrides via `-r`
- scale-down overrides via `-w`
- `-l/--list` compatible app list output
- `-s/--summary` resource summary with text table output by default and JSON via `--summary-format json`
- `-b` accepted as compatibility flag
- Cobra-based CLI with explicit `build`, `validate`, `summary`, `inventory`, `list` and `completion` commands
- health/probe blocks, scheduling fields, registry secrets, host aliases and raw container passthrough

Known intentional differences / remaining risks:

- `-d/--decrypt-secured` is a compatibility path implemented by executing external EncJson binaries; production workflows should prefer resolved `.env` input through `-E/--env-file`
- `simple_init` runtime rendering if a future contract defines it; current Ruby behavior is validation/inventory only
- byte-for-byte Ruby summary table output is not preserved; Go uses a new CI/L2-oriented ASCII table and keeps JSON available through `--summary-format json`

`scripts/parity-build` now builds Ruby and Go outputs, compares generated file sets and compares YAML/JSON semantically.

## Current parity status

The Go renderer currently matches the Ruby renderer semantically for the agreed real inputs:

| Case | Environment | Result |
| --- | --- | --- |
| CETIN TSM | `test` | OK, 161 files |
| CETIN TSM | `ref` | OK, 118 files |
| CETIN TSM | `ref2` | OK, 118 files |
| CETIN TSM | `prod` | OK, 103 files |
| CETIN TSM | `prod2` | OK, 103 files |
| O2 Slovakia NAC / TSM | `tst` | OK, 146 files |
| O2 Slovakia NAC / TSM | `ppt` | OK, 146 files |
| O2 Slovakia NAC / TSM | `prod` | OK, 146 files |
| O2 NAC | `dev` | OK, 145 files |
| O2 NAC | `prod` | OK, 140 files |

All listed cases were validated with:

```bash
just build
scripts/parity-build --name <case> --root <env-root> --env <env> --release-id 2025.08.18.1 --go-bin ./dist/kube-build-app
```

The comparer checks generated file sets and YAML/JSON content semantically. Non-YAML/JSON payloads are compared byte-for-byte.

## Remaining production hardening

Before replacing Ruby in production workflows, finish these non-rendering items:

1. Add a CI job that runs `just test`, `just build`, and a curated subset of parity cases.
2. Optionally run `kubeconform` on Ruby and Go outputs and document any shared schema exceptions.
3. Publish release binaries for macOS, Linux and Windows using `just release <version>`.

## Decision rule

A parity difference is allowed only when it is intentional.

Every intentional difference must be written down as:

```text
case:
file:
difference:
reason:
approved:
```

If it is not documented, it is a regression.
