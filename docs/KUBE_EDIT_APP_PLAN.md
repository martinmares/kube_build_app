# kube-edit-app plan

`kube-edit-app` is the Go rewrite of the Rust `kube-environments-ui` web application.

It is not a new product from scratch. The source of truth for the first production target is:

```text
/Users/mares/Development/Src/Rust/kube-environments-ui/crates/kube-env-web
```

The visual shell should be aligned with the newer Tabler-based applications:

```text
/Users/mares/Development/Src/Rust/simple-idm-server
/Users/mares/Development/Src/Rust/simple-artifacts-server
```

## Naming Contract

```text
kube-build-app = build/render Kubernetes manifests from environment repositories
kube-edit-app  = browse/edit/validate environment repositories through Web UI
```

`kube-env-app` was the temporary name and is being renamed to `kube-edit-app`.

Recommended target layout:

```text
cmd/kube-build-app
cmd/kube-edit-app
internal/buildapp
internal/editapp or internal/webapp
internal/repository
```

The module name can stay unchanged for now. The important product names are the binaries and CLI commands.

## Product Goal

`kube-edit-app` should let L2/support/developers edit environment repositories without manually editing YAML/JSON files in Vim/VSCode.

The application must support the same real repositories used for `kube-build-app` parity testing:

```text
/Users/mares/Development/Src/Ruby/tsm/cetin/tsm-environments
/Users/mares/Development/O2/NAC/dtl-nac/environments
/Users/mares/Development/O2sk/NAC/tsm-environments
```

The UI should work against a configured environments root and later may support switching between multiple repository roots.

## CLI Target

Replace the current `flag` based `kube-env-app` CLI with Cobra.

Target command shape:

```bash
kube-edit-app serve \
  --root /path/to/tsm-environments \
  --listen 127.0.0.1:8183 \
  --base-path / \
  --read-only
```

Target flags:

```text
--root                  environment repository root
--listen                HTTP listen address, default 127.0.0.1:8183
--base-path             reverse-proxy base path, default /
--read-only             disable mutating API endpoints
--allow-write           explicitly allow writes when needed
--encjson-path          modern EncJson binary, API 2.x
--encjson-legacy-path   legacy EncJson binary, API 1.x
--encjson-keydir        optional keydir; if empty, do not pass -k
--version               print JSON version info
```

Root command with no args should print help, same as `kube-build-app`.

## Rust API Parity

The first Go production target should preserve the useful API contract from Rust `kube-env-web`.

Already present in Go skeleton:

```text
GET /healthz
GET /api/v1/info
GET /api/v1/envs
GET /api/v1/envs/{env}/apps
GET /api/v1/envs/{env}/assets
```

Required API parity:

```text
GET  /health
GET  /api/v1/envs/{env}/apps/{app_file}
POST /api/v1/envs/{env}/apps/{app_file}/restore
PUT  /api/v1/envs/{env}/apps/{app_file}/replicas
PUT  /api/v1/envs/{env}/apps/{app_file}/startup
GET  /api/v1/envs/{env}/apps/{app_file}/rendered
GET  /api/v1/envs/{env}/apps/{app_file}/vars
PUT  /api/v1/envs/{env}/apps/{app_file}/vars
GET  /api/v1/envs/{env}/apps/{app_file}/model
PUT  /api/v1/envs/{env}/apps/{app_file}/resources
PUT  /api/v1/envs/{env}/apps/{app_file}/env-var
POST /api/v1/envs/{env}/apps/{app_file}/env-var
DELETE /api/v1/envs/{env}/apps/{app_file}/env-var/{container_index}/{env_var_index}
PUT  /api/v1/envs/{env}/apps/{app_file}/ingress
POST /api/v1/envs/{env}/apps/{app_file}/ports/expose-as
PUT  /api/v1/envs/{env}/apps/{app_file}/ports/expose-as
POST /api/v1/envs/{env}/apps/{app_file}/ingress/external
DELETE /api/v1/envs/{env}/apps/{app_file}/ingress/external/{container_index}/{port_index}/{expose_as_index}/{external_index}
GET  /api/v1/envs/{env}/assets/content/{asset_path}
PUT  /api/v1/envs/{env}/assets/content/{asset_path}
POST /api/v1/envs/{env}/assets/restore/{asset_path}
GET  /api/v1/envs/{env}/assets/special/{special_file}/entries
PUT  /api/v1/envs/{env}/assets/special/{special_file}/entries
GET  /api/v1/envs/{env}/assets/special/{special_file}/preflight
```

Additional Go-native endpoints should use `internal/buildapp` directly:

```text
POST /api/v1/envs/{env}/validate
POST /api/v1/envs/{env}/summary
POST /api/v1/envs/{env}/inventory
POST /api/v1/envs/{env}/build-preview
```

`build-preview` should render into a temp directory and return generated file list, render events and optional diagnostics. It must not write into the real deploy repository.

## Repository Safety Rules

Mutating endpoints must be safe by default.

Rules:

- all path inputs must stay inside the configured environments root
- reject `..`, absolute paths and Windows path escapes
- mutating endpoints require Git repository unless explicitly overridden later
- read-only mode must reject all writes with clear error
- use atomic write: temp file in same directory, fsync if practical, rename
- preserve file permissions where reasonable
- use optimistic locking where possible: client sends previous content hash
- return structured errors as JSON
- no silent YAML full reserialization for operations that should preserve user formatting

## Git Integration

Minimum required:

```text
GET repo status summary
show dirty files per selected env
restore selected app/special/asset file through git restore
```

Rust behavior to preserve:

- dashboard warns when root is not inside Git work tree
- mutating API returns an explicit error in non-git mode
- restore button is enabled only for dirty files

Later:

```text
diff preview
commit helper
branch display
```

## EncJson Integration

The editor must support `env.secured.json` and `assets.secured.json` through external EncJson tools.

Contract:

- API marker `EncJson[@api=1.0` uses legacy binary
- API marker `EncJson[@api=2.0` uses modern binary
- unknown marker should prefer modern binary
- `ENCJSON_KEYDIR` / `--encjson-keydir` is optional
- when keydir is empty, do not pass `-k`; let the encjson binary choose its default

Preflight endpoint should report:

```text
selected binary
binary exists/executable
keydir configured or omitted
secured file exists
detected API mode
can decrypt/encrypt where practical
```

## UI Parity Target

Port the user workflow from Rust `kube-env-web`, then polish with Tabler style.

Required views:

```text
Dashboard
Apps
Assets
Virtual assets content
Special files
Build preview
```

Required app detail sections:

```text
Overview
Summary context
Local variables
Container variables
Services / ports / ingress / routes
Resources
Startup
Raw YAML
Rendered preview
```

Required assets behavior:

- directory tree + file list
- text/binary detection
- raw text editor for text files
- upload/download/copy/delete controls where applicable
- special files shown separately
- `assets.secured.json` / `assets.unsecured.json` opened as virtual asset filesystem

Required special files:

```text
<env>/apps/_defaults.yml
<env>/env.secured.json
<env>/env.unsecured.json
<env>/assets.secured.json
<env>/assets.unsecured.json
<env>/shared.assets.yml
<env>/replica-profiles.yml
```

## Visual Design Target

Use Tabler-based shell aligned with `simple-idm-server` and `simple-artifacts-server`:

- `data-bs-theme` with Auto/Light/Dark toggle
- top navbar with product name and repository badges
- second-level nav tabs
- `page`, `page-wrapper`, `page-header`, `page-body`, `container-xl`
- cards for sections
- `table table-vcenter table-hover` for lists
- `datagrid` for detail metadata
- `modal modal-blur fade` for dialogs
- global error alert
- consistent buttons and icon usage through Tabler Icons
- monospace text for paths, hashes and generated snippets

Avoid introducing a separate visual language.

## Implementation Order

## Current Work Snapshot

Last updated: 2026-05-17

Current implementation status:

- `cmd/kube-edit-app` exists and is built by `just build`
- Cobra `serve` command exists with root/listen/read-only/EncJson flags
- HTML/JS/CSS are externalized under `internal/webapp/templates` and `internal/webapp/static`
- read-only repository browsing works for environments, apps, assets and special files
- Git status endpoint exists and UI shows branch/dirty count
- environments, apps, assets and selected details expose dirty state
- app detail supports raw YAML, rendered preview, local vars and parsed model
- app detail now has a read-only structured overview for:
  - app kind, replicas, init containers and HPA/autoscaling
  - container resources
  - Java runtime JVM sizing
  - probes, env vars, env_from, mounts and ports counts
- unquoted app placeholders such as `port: {{var:EXPOSE_PORT}}` are handled for model preview
- Build tab uses `internal/buildapp` through read-only endpoints:
  - `POST /api/v1/envs/{env}/validate`
  - `POST /api/v1/envs/{env}/summary`
  - `POST /api/v1/envs/{env}/inventory`
- Build tab UI currently shows:
  - Validate action
  - Resource summary metrics and table with totals
  - Inventory table with text filter and dot-path value extraction

Open uncommitted UI work at this snapshot:

```text
internal/repository/content.go
internal/repository/git.go
internal/repository/repository.go
internal/repository/repository_test.go
internal/webapp/server.go
internal/webapp/server_test.go
internal/webapp/templates/index.html
internal/webapp/static/ui.js
internal/webapp/static/ui.css
```

Verification already run for the snapshot:

```bash
GOCACHE="$PWD/.tmp/go-build-cache" go test ./...
```

Recommended commit message for the current snapshot:

```bash
git add docs/KUBE_EDIT_APP_PLAN.md internal/repository/content.go internal/repository/git.go internal/repository/repository.go internal/repository/repository_test.go internal/webapp/server.go internal/webapp/server_test.go internal/webapp/templates/index.html internal/webapp/static/ui.js internal/webapp/static/ui.css
git commit -m "Add kube-edit-app git dirty state"
```

Recommended next steps for `kube-edit-app` after returning to it:

1. Extend Git integration with restore endpoint and diff preview.
2. Extend read-only app structured overview with richer services/ports/expose_as details.
3. Add `build-preview` endpoint that renders into a temp directory and returns generated file list/events without writing to deploy repo.
4. Add safe write foundation: read-only guard, atomic writes, content hash precondition, structured JSON errors.
5. Port the first mutating editor from Rust: replicas/resources is the lowest-risk starting point.
6. Add special file editors for `_defaults.yml`, `env.unsecured.json`, `env.secured.json`.
7. Revisit virtual assets content only after secured/unsecured special file edit flow is designed.

Do not start with broad YAML reserialization. Prefer targeted patches that preserve existing file layout where practical.

### Phase 1: Rename and CLI

- rename `cmd/kube-env-app` to `cmd/kube-edit-app`
- update `Justfile`, GitLab packaging, README references
- add Cobra CLI with `serve`
- preserve temporary `kube-env-app` alias only if needed, with deprecation notice

### Phase 2: API Read Parity

- app detail endpoint
- rendered preview endpoint
- vars endpoint
- app model endpoint
- asset content endpoint
- special entries read endpoint
- preflight endpoint skeleton
- tests for all path safety rules

### Phase 3: Tabler UI Shell

- static `ui.css` and `ui.js`
- dashboard
- env selector
- Apps tab read-only
- Assets tab read-only
- app detail read-only
- build summary/inventory preview using `internal/buildapp`

### Phase 4: Safe Writes

- raw asset content save
- raw app file save only if explicitly enabled
- replicas patch
- vars patch
- resources patch
- startup patch
- env var add/update/delete
- git restore

### Phase 5: Structured Editors

- `_defaults.yml` editor
- `env.unsecured.json` editor
- `env.secured.json` EncJson edit flow
- `assets.unsecured.json` virtual asset filesystem
- `assets.secured.json` EncJson virtual asset filesystem

### Phase 6: Services/Ingress Editor

- ports/expose_as editor
- ingress/route external item add/update/delete
- template-preserving text patches

### Phase 7: Production Polish

- dirty state everywhere
- diff preview
- keyboard shortcuts where useful
- CI smoke tests over `fixtures/readme-smoke`
- optional smoke tests over real local repositories

## Definition of Done

`kube-edit-app` can be considered production-ready when:

- it can browse and edit the same real repositories used by `kube-build-app` parity tests
- it preserves formatting for targeted structured edits where Rust did text patches
- it can validate and preview build output through Go `internal/buildapp`
- it safely handles Git restore and dirty state
- it supports secured env/assets workflows through external EncJson binaries
- its UI is visually aligned with the Tabler shell used by the newer internal apps
