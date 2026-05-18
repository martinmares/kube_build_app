# kube-build-app

`kube-build-app` builds Kubernetes manifests from an environment repository.

The tool is intentionally small at runtime: it reads declarative environment metadata, app model files and assets, then writes Kubernetes YAML into a target directory. The Ruby implementation is the historical reference; the Go implementation is the productized CLI with a single binary, Cobra-based commands and shell completion.

This repository now contains two related binaries:

```text
kube-build-app = build/render Kubernetes manifests
kube-edit-app  = web editor for environment repositories
```

`kube-edit-app` is the Go rewrite target for the Rust `kube-environments-ui` web application. See `docs/KUBE_EDIT_APP_PLAN.md`.

## kube-edit-app Workflow

Start the web editor in safe read-only mode:

```bash
kube-edit-app serve --root ./environments
```

Enable structured writes explicitly:

```bash
kube-edit-app serve --root ./environments --allow-write
```

Recommended editing flow:

1. Select an environment.
2. Open `Apps`, `Assets` or `Changed files`.
3. Make a structured edit.
4. Review the colored `Git diff`.
5. Run `Build` validation.
6. Commit the environment repository change with Git.

Read-only mode still renders structured inputs, previews and diffs, but save buttons are disabled. This is the preferred mode for review, L2 inspection and dashboards. Use `--allow-write` only for intentional repository edits.

## Metamodel Goal

`kube-build-app` is not Helm. The app model intentionally follows an 80/20 rule:

```text
80 % of common deployment needs = clear first-class metamodel fields
20 % of special Kubernetes cases = explicit raw escape hatches
```

Prefer dedicated model fields when they exist, because they can be validated, summarized and edited by `kube-edit-app`. Use `raw:` only for Kubernetes fields that are too rare or too specific for the common model.

The long-term metamodel plan is tracked in `docs/KUBE_BUILD_APP_METAMODEL_PLAN.md`.

## Quick Start

Minimal repository layout:

```text
environments/
  test/
    env.unsecured.json
    apps/
      api.yml
    assets/
```

`environments/test/env.unsecured.json`:

```json
{
  "environment": {
    "NAMESPACE": "demo-test",
    "TSM_REGISTRY_URL": "registry.example.com/demo",
    "TSM_RELEASE_ID": "2026.05.13.1"
  }
}
```

`environments/test/apps/api.yml`:

```yaml
name: api
replicas: 1
containers:
  - name: api
    image: "{{env:TSM_REGISTRY_URL}}/api:{{env:TSM_RELEASE_ID}}"
```

Build manifests:

```bash
kube-build-app build -e test -R environments -t deploy/test
```

Generated files:

```text
deploy/test/
  deployments/api-deployment.yml
  services/
  assets/
```

If `-t/--target` is omitted, the default output directory is:

```text
<root>/<environment>/target
```

## CLI

Preferred modern commands:

```bash
kube-build-app build -e test -R environments -t deploy/test
kube-build-app validate -e test -R environments
kube-build-app summary -e test -R environments
kube-build-app summary --summary-format json -e test -R environments
kube-build-app inventory -e test -R environments
kube-build-app list -e test -R environments
kube-build-app completion zsh
```

### Generator Commands

Create a starter environment:

```bash
kube-build-app skeleton env \
  --root environments \
  --env dev \
  --namespace app-dev \
  --registry-url registry.example.com/project \
  --release-id latest
```

This creates:

```text
environments/dev/env.unsecured.json
environments/dev/apps/_defaults.yml
```

Add a starter app model:

```bash
kube-build-app app add api --root environments --environment dev
```

Generated app images use generic placeholders by default:

```yaml
image: "{{REGISTRY_URL}}/api:{{RELEASE_ID}}"
```

Override the image when needed:

```bash
kube-build-app app add worker \
  --root environments \
  --environment dev \
  --image 'custom/worker:1.0.0'
```

Generator commands do not overwrite existing files unless `--force` is used.

Legacy-compatible root flags are still supported:

```bash
kube-build-app -e test -R environments -t deploy/test
kube-build-app -s -e test -R environments
kube-build-app -i -e test -R environments
kube-build-app -l -e test -R environments
```

Do not mix action subcommands with legacy action flags. For example, this is intentionally invalid:

```bash
kube-build-app build -e test -s
```

Use this instead:

```bash
kube-build-app summary -e test
```

Verbose build logging:

```bash
kube-build-app build -e test -R environments -t deploy/test --verbose
kube-build-app build -e test -R environments -t deploy/test --verbose --log-format json
kube-build-app build -e test -R environments -t deploy/test --verbose --color always
```

Build logs are written to `stderr`. Normal command output stays on `stdout`.

Useful flags:

```text
-e, --environment        environment name
-R, --root               environments root directory, default: environments
-t, --target             target output directory
-p, --profile            replica profile name
    --profiles-file      replica profiles file path
-r, --release-manifest   release manifest YAML path
-w, --down               scale selected app replicas to 0
-E, --env-file           explicit .env file path
    --vars-source        env, json, dot-env; repeatable or comma-separated
-d, --decrypt-secured    enable env.secured.json variables
    --helm-escape-assets escape remaining {{VAR}} placeholders in text assets
    --verbose            print build render events to stderr
    --log-format         verbose build log format: text or json
    --color              verbose text color: auto, always or never
```

## Placeholder Contract

There are three placeholder forms. They have different scopes and different timing.

| Placeholder | Resolved by | Timing | Meaning |
| --- | --- | --- | --- |
| `{{VAR}}` | `apply-env`, CI/CD, deploy phase, Helm-safe post-processing | later | Runtime/deploy placeholder. `kube-build-app` leaves it unresolved in app YAML. |
| `{{env:VAR}}` | `kube-build-app` | build time | Build-time environment variable loaded from env JSON, `.env` or process env according to configured sources. |
| `{{var:VAR}}` | `kube-build-app` | build time | App-local variable from the `vars:` block in the current `<app>.yml`, after `_defaults.yml` merge. |

Use this rule of thumb:

```text
prefixed placeholder = resolve during kube-build-app
plain placeholder    = keep for later deployment/rendering phase
```

Example:

```yaml
vars:
  - name: APP_NAME
    value: api

name: "{{var:APP_NAME}}"
containers:
  - name: "{{var:APP_NAME}}"
    image: "{{env:TSM_REGISTRY_URL}}/{{var:APP_NAME}}:{{env:TSM_RELEASE_ID}}"
    env_vars:
      - name: RUNTIME_VALUE
        value: "{{RUNTIME_VALUE}}"
```

Resulting deployment keeps only the plain runtime placeholder:

```yaml
containers:
  - name: api
    image: registry.example.com/demo/api:2026.05.13.1
    env:
      - name: RUNTIME_VALUE
        value: "{{RUNTIME_VALUE}}"
```

## Variable Sources

Default behavior is backward-compatible:

```text
env.unsecured.json + env.secured.json when -d is enabled, then process environment
```

Explicit `.env` file:

```bash
kube-build-app build -e test -E /path/to/release.env
```

This is the recommended production workflow for secured variables. The decrypting tool can be anything; `kube-build-app` receives already resolved key/value pairs.

In this mode the explicit `.env` is the only variable source. It cannot be combined with `-d` or `--vars-source`.

Backward-compatible secured JSON decrypt:

```bash
kube-build-app build -e test -d
```

When `-d/--decrypt-secured` is used, `env.secured.json` is decrypted by executing an external EncJson binary:

```text
encjson decrypt -k <keydir> -f env.secured.json
encjson-rs decrypt -k <keydir> -f env.secured.json
```

Binary selection:

```text
EncJson[@api=1.0  -> ENCJSON_LEGACY_PATH, ENCJSON_LEGACY_BIN, ENCJSON_BIN, fallback encjson
EncJson[@api=2.0  -> ENCJSON_PATH, ENCJSON_RS_BIN, fallback encjson-rs
unknown marker    -> ENCJSON_BIN or legacy fallback
```

Key directory:

```text
if ENCJSON_KEYDIR is set: decrypt -k "$ENCJSON_KEYDIR" -f env.secured.json
otherwise:                 decrypt -f env.secured.json
```

When `ENCJSON_KEYDIR` is not set, key directory selection is delegated to the EncJson utility itself.

Explicit source selection:

```bash
kube-build-app build -e test --vars-source json
kube-build-app build -e test --vars-source env
kube-build-app build -e test --vars-source json --vars-source env
kube-build-app build -e test --vars-source json,env
```

Supported sources:

```text
json     env.unsecured.json and env.secured.json when -d is enabled
env      process environment
dot-env  <environment_dir>/.env unless -E/--env-file is used
```

## App Model by Composition

An app model is one YAML file in:

```text
<environment>/apps/<app>.yml
```

Files starting with `_` are special files, not apps. For example:

```text
<environment>/apps/_defaults.yml
```

### 1. Minimal App

```yaml
name: api
containers:
  - name: api
    image: nginx:stable
```

Generated deployment contains one container:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: api
          image: nginx:stable
```

### 2. Replicas

Add:

```yaml
replicas: 3
```

Generated deployment contains:

```yaml
spec:
  replicas: 3
```

### 3. Resources

Add container resources with Kubernetes-aligned `requests` / `limits`:

```yaml
containers:
  - name: api
    image: nginx:stable
    resources:
      cpu:
        requests: "100m"
        limits: "500m"
      memory:
        requests: "128Mi"
        limits: "512Mi"
```

Legacy `from` / `to` remains supported:

```yaml
containers:
  - name: api
    image: nginx:stable
    resources:
      cpu:
        from: "100m"
        to: "500m"
      memory:
        from: "128Mi"
        to: "512Mi"
```

Generated deployment contains:

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

The summary command uses these values and multiplies totals by replicas:

```bash
kube-build-app summary -e test -R environments
```

#### Autoscaling / HPA

Use app-level `autoscaling` when runtime replica count should be controlled by Kubernetes HPA:

```yaml
replicas: 2
autoscaling:
  enabled: true
  min_replicas: 2
  max_replicas: 6
  cpu:
    average_utilization: 75
  memory:
    average_utilization: 80
containers:
  - name: api
    image: nginx:stable
    resources:
      cpu:
        requests: "100m"
        limits: "500m"
      memory:
        requests: "128Mi"
        limits: "512Mi"
```

Generated manifests contain an additional `deployments/<app>-hpa.yml`:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  minReplicas: 2
  maxReplicas: 6
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api
```

If `autoscaling.enabled=true`, the runtime replica count is controlled by HPA. The `replicas` value remains in the generated Deployment/StatefulSet as the initial desired state, but HPA can change it after the object is running.

CPU and memory utilization metrics require container `resources.requests`. Use `autoscaling.raw` for advanced HPA fields such as `spec.behavior`:

```yaml
autoscaling:
  enabled: true
  min_replicas: 2
  max_replicas: 6
  cpu:
    average_utilization: 75
  raw:
    spec:
      behavior:
        scaleDown:
          stabilizationWindowSeconds: 300
```

### 4. Container Environment Variables

Add:

```yaml
containers:
  - name: api
    image: nginx:stable
    env_vars:
      - name: JAVA_OPTS
        value: "-Xms256m -Xmx512m"
      - name: POD_NAME
        field_path: metadata.name
```

Generated deployment contains:

```yaml
env:
  - name: JAVA_OPTS
    value: "-Xms256m -Xmx512m"
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
```

#### Java Runtime Options

For Java containers, prefer `runtime.java` for well-known JVM memory options instead of hand-writing `JAVA_OPTS`:

```yaml
containers:
  - name: api
    image: nginx:stable
    runtime:
      java:
        xms: "512m"
        xmx: "2048m"
        opts:
          - "-XX:+UseG1GC"
        export:
          env_name: JAVA_OPTS
    resources:
      cpu:
        requests: "100m"
        limits: "1000m"
      memory:
        requests: "1024Mi"
        limits: "2560Mi"
```

Generated deployment contains:

```yaml
env:
  - name: JAVA_OPTS
    value: "-Xms512m -Xmx2048m -XX:+UseG1GC"
```

`export.env_name` defaults to `JAVA_OPTS`. If the same variable is also specified manually in `env_vars`, validation fails. This keeps JVM heap sizing visible in `kube-build-app summary` and avoids hidden conflicts with startup scripts.

### 5. Ports and Services

Add a container port:

```yaml
containers:
  - name: api
    image: nginx:stable
    ports:
      - name: http
        port: 8080
```

Generated deployment contains `containerPort: 8080`.

To create a service, add `expose_as`:

```yaml
ports:
  - name: http
    port: 8080
    expose_as:
      - hostname: api
        port: 80
```

Generated service:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: api
spec:
  ports:
    - name: http-80
      port: 80
      targetPort: 8080
```

External exposure can be attached under `expose_as[].external` and generates Ingress or OpenShift Route according to the model.

### 6. Probes

Preferred modern form:

```yaml
containers:
  - name: api
    image: api:latest
    probes:
      preset: spring-actuator
      port: 8080
```

This generates:

```text
livenessProbe  -> /actuator/health/liveness
readinessProbe -> /actuator/health/readiness
startupProbe   -> /actuator/health
```

Generic HTTP form:

```yaml
probes:
  preset: http
  port: 8080
  path: /healthz
```

Override form:

```yaml
probes:
  http:
    path: /actuator/health
    port: 8080
  live:
    period: 10
    timeout: 2
    failure: 5
  ready:
    period: 2
    timeout: 2
    success: 2
    failure: 2
  start:
    period: 10
    timeout: 2
    failure: 30
```

Exec probes:

```yaml
probes:
  live:
    command: ["/bin/sh", "/app/liveness.sh"]
  ready:
    command: ["/bin/sh", "/app/liveness.sh"]
  start:
    command: ["/bin/sh", "/app/liveness.sh"]
    failure: 30
```

Legacy `health` and `probe` blocks remain supported for backward compatibility:

```yaml
containers:
  - name: api
    image: nginx:stable
    health:
      http:
        path:
          live: /health/live
          ready: /health/ready
        port: 8080
```

`health` generates liveness/readiness only. `probe` can generate liveness/readiness/startup but is more verbose. New files should prefer `probes`.

### 7. Assets

Place a file under:

```text
<environment>/assets/nginx.conf
```

Reference it from the app:

```yaml
containers:
  - name: api
    image: nginx:stable
    assets:
      - file: assets/nginx.conf
        to: /etc/nginx/conf.d/default.conf
```

Generated output includes:

```text
assets/api-asset-<crc>.yml
```

and deployment gets a volume mount:

```yaml
volumeMounts:
  - mountPath: /etc/nginx/conf.d/default.conf
    name: api-asset-<crc>
    readOnly: true
    subPath: nginx.conf
volumes:
  - name: api-asset-<crc>
    configMap:
      name: api-asset-<crc>
```

Asset name digest is based on the real asset input and mount target. This keeps ConfigMap names stable and changes them only when the mounted asset content changes.

Text assets can be transformed with build-time variables:

```yaml
assets:
  - file: assets/application.yml.tpl
    to: /app/config/application.yml
    transform: true
```

Legacy `assets` also supports volume-only forms such as `temp`, `pvc`, `nfs-server` and `host-path`. They remain supported for backward compatibility, but new files should prefer `mounts` for volume declarations.

### 8. Mounts

Use `mounts` for non-ConfigMap volumes and for a clearer modern form of file mounts:

```yaml
containers:
  - name: api
    image: api:latest
    mounts:
      - type: config
        file: assets/app.conf
        mount_path: /app/app.conf

      - type: empty_dir
        name: cache
        mount_path: /app/cache

      - type: pvc
        name: data
        claim_name: data-claim
        mount_path: /data

      - type: nfs
        name: data-nfs
        server: nfs.local
        path: /export/data
        mount_path: /nfs

      - type: host_path
        name: host-data
        path: /var/lib/host-data
        mount_path: /host
```

`host_path` is a high-risk escape hatch because it exposes node filesystem paths to the container.

For rare Kubernetes volume types, use raw mount passthrough:

```yaml
mounts:
  - type: raw
    volume:
      name: special
      projected:
        sources: []
    mount:
      name: special
      mountPath: /app/special
      readOnly: true
```

### 9. Shared Assets

Use `<environment>/shared.assets.yml` for assets mounted into multiple apps.

An app can opt out:

```yaml
disable_shared_assets: true
```

### 10. Tools

Static utility binaries can be exposed through initContainers and mounted under `/app/tools`:

```yaml
tools:
  - name: util-apply-env
    image: registry.example.com/tools/apply-env:latest
    expose_bin: /usr/bin/apply-env
    as: /app/tools/apply-env
```

Behavior:

- every tool becomes an initContainer
- the initContainer copies `expose_bin` into a shared `emptyDir`
- app containers mount that volume read-only under `/app/tools`
- if `as` is omitted, target defaults to `/app/tools/<basename(expose_bin)>`

### 11. Scheduling and Pod Metadata

Common app-level fields:

```yaml
labels:
  team: tsm
annotations:
  app.example.com/owner: l2
pod_annotations:
  prometheus.io/scrape: "true"
arch: amd64
node_selector:
  node-role.kubernetes.io/worker: ""
tolerations:
  - key: dedicated
    operator: Equal
    value: tsm
    effect: NoSchedule
```

These are rendered into deployment metadata and pod template fields.

Preferred modern scheduling form:

```yaml
scheduling:
  arch: amd64
  node_selector:
    node-role.kubernetes.io/worker: ""
  tolerations:
    - key: dedicated
      operator: Equal
      value: tsm
      effect: NoSchedule
  spread:
    by: hostname
    max_skew: 1
    when_unsatisfiable: ScheduleAnyway
  anti_affinity:
    self: preferred
    topology: kubernetes.io/hostname
```

For rare Kubernetes scheduling cases, use raw affinity under `scheduling.affinity`:

```yaml
scheduling:
  affinity:
    nodeAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 10
          preference:
            matchExpressions:
              - key: disk
                operator: In
                values: [ssd]
```

Legacy `arch`, `node_selector` and `tolerations` remain supported. New files should prefer `scheduling`.

### 12. Security Context

Use `security_context` for common pod and container security settings.

App-level `security_context` is rendered to pod spec `securityContext`:

```yaml
security_context:
  runAsNonRoot: true
  fsGroup: 2000
```

Container-level `security_context` is rendered to container `securityContext`:

```yaml
containers:
  - name: api
    image: nginx:stable
    security_context:
      allowPrivilegeEscalation: false
      runAsUser: 1000
      capabilities:
        drop: ["ALL"]
```

Prefer this dedicated field over `raw.securityContext`. If both are used, `raw` remains a compatibility escape hatch and can override the generated field.

### 13. Lifecycle and Termination Grace

Use app-level `termination_grace_period` to render pod spec `terminationGracePeriodSeconds`:

```yaml
termination_grace_period: 45
```

Use container-level `lifecycle.pre_stop` for graceful shutdown hooks:

```yaml
containers:
  - name: api
    image: nginx:stable
    lifecycle:
      pre_stop:
        command: ["/bin/sh", "-c", "sleep 10"]
```

This generates Kubernetes `lifecycle.preStop.exec.command`.

For rare lifecycle handler forms, use explicit raw passthrough:

```yaml
containers:
  - name: api
    lifecycle:
      pre_stop:
        raw:
          httpGet:
            path: /shutdown
            port: 8080
```

### 14. Service Account and Env From

Use app-level `service_account` to render pod spec `serviceAccountName`:

```yaml
service_account: tsm-api
```

Use container-level `env_from` for common ConfigMap/Secret environment imports:

```yaml
containers:
  - name: api
    image: api:latest
    env_from:
      - config_map: api-config
      - secret: api-secret
        prefix: SECRET_
```

For less common Kubernetes options, use the native reference shape:

```yaml
env_from:
  - configMapRef:
      name: api-config
      optional: true
  - secretRef:
      name: api-secret
      optional: true
```

This renders to Kubernetes `envFrom`.

### 15. Init Containers

Use app-level `init_containers` for generic Kubernetes init containers:

```yaml
init_containers:
  - name: migrate
    image: registry.example.com/api-migrate:latest
    command: ["/bin/sh", "-c"]
    arguments: ["./migrate.sh"]
    env_vars:
      - name: LOG_LEVEL
        value: INFO
    env_from:
      - config_map: api-config
    mounts:
      - type: empty_dir
        name: work
        mount_path: /work
    security_context:
      runAsNonRoot: true
    resources:
      cpu:
        from: "50m"
        to: "100m"
      memory:
        from: "64Mi"
        to: "128Mi"
```

Supported init container fields intentionally mirror the common subset of regular containers:

- `name`
- `image`
- `command`
- `arguments`
- `env_vars`
- `env_from`
- `mounts`
- `security_context`
- `resources`
- `raw`

Existing `tools` remain the preferred shortcut for exposing static utility binaries through generated init containers.

### 16. Raw Escape Hatches

Use `deployment_raw` for rare Deployment-level fields:

```yaml
deployment_raw:
  spec:
    revisionHistoryLimit: 2
```

Use `pod_raw` for rare pod spec fields:

```yaml
pod_raw:
  dnsPolicy: ClusterFirst
  enableServiceLinks: false
```

`deployment_raw` is recursively merged into the generated Deployment object. `pod_raw` is applied to `spec.template.spec`.

### 17. Raw Container Fields

Use raw passthrough only when the model does not expose a dedicated field:

```yaml
containers:
  - name: api
    image: nginx:stable
    raw:
      securityContext:
        runAsNonRoot: true
```

Dedicated model fields are preferred because they can be validated and represented in UI tooling.

### 18. Cgroup Exporter Defaults

At container level you can enable automatic env var injection for cgroup exporter:

```yaml
containers:
  - name: api
    image: api:latest
    enable_cgroup_exporter: true
```

Only missing variables are injected:

```text
CGROUP_EXPORTER_METRICS_PREFIX
CGROUP_EXPORTER_METRICS_STATIC_LABELS
CGROUP_EXPORTER_LISTEN
CGROUP_EXPORTER_CPU_REQUESTS_MCPU
CGROUP_EXPORTER_CPU_LIMITS_MCPU
CGROUP_EXPORTER_MEMORY_REQUESTS_MIB
CGROUP_EXPORTER_MEMORY_LIMITS_MIB
CGROUP_EXPORTER_NODE_NAME
```

### 19. Ignored Apps

Exclude an app from build:

```yaml
ignore: true
name: experimental-api
```

No deployment, service or assets are generated for ignored apps.

## App Defaults

`apps/_defaults.yml` is merged into every app model in that environment.

Example:

```yaml
tools:
  - name: util-apply-env
    image: registry.example.com/tools/apply-env:latest
    expose_bin: /usr/bin/apply-env

vars:
  - name: LOG_LEVEL
    value: INFO

container_env_vars:
  - name: "*"
    env_vars:
      - name: GLOBAL_FLAG
        value: "true"

  - name: api
    env_vars:
      - name: JAVA_OPTS
        value: "-Xms256m"
```

Semantics:

- generic map keys are recursively merged, app values win
- `vars` are matched by `name`; app-level item fully replaces default item
- `container_env_vars` are applied by `container.name`
- `name: "*"` applies to all containers first
- concrete container defaults are applied next
- local `containers[].env_vars` are applied last
- matching env vars are fully replaced by `name`

Removal / tombstone:

```yaml
vars:
  - name: LOG_LEVEL
    remove: true
```

```yaml
containers:
  - name: api
    env_vars:
      - name: GLOBAL_FLAG
        remove: true
```

`remove: true` removes the item completely and must not be combined with other data fields.

## Replica Profiles

Use environment-level profiles to override replicas without editing app files:

`<environment>/replica-profiles.yml`:

```yaml
defaults:
  profile: normal

profiles:
  normal:
    apps:
      api: 2
      worker: 1

  maintenance:
    all: 0
    apps:
      api: 1
```

Run:

```bash
kube-build-app build -e test -p maintenance -R environments -t deploy/test
```

Scale selected apps down directly:

```bash
kube-build-app build -e test -w worker -R environments -t deploy/test
```

## Rollout Checksums

Use rollout checksum annotations when a pod must restart after selected files change.

```yaml
rollout_on:
  checksums:
    config:
      files:
        - env.unsecured.json
        - env.secured.json
    mtls:
      files:
        - assets/infrastructure/mtls-gateway-config.tpl
```

Generated deployment pod template contains:

```yaml
spec:
  template:
    metadata:
      annotations:
        checksum/config: "..."
        checksum/mtls: "..."
```

Behavior:

- paths are relative to `<environment_dir>`
- files are sorted before hashing
- checksum includes relative path and file content
- missing files fail fast
- changing annotation changes pod template and triggers rollout

Use this for deterministic declarative files only. Do not use it for values injected later by sidecars or runtime-only mechanisms.

## Inventory

Inventory prints a structured JSON view of what will be built:

```bash
kube-build-app inventory -e test -R environments
```

Legacy equivalent:

```bash
kube-build-app -i -e test -R environments
```

It is intended for automation, UI tooling and generators such as PKI helpers.

## Validation

Validate model files without writing manifests:

```bash
kube-build-app validate -e test -R environments
```

Validation catches known invalid combinations, for example conflicting startup/simple-init settings and invalid tombstones.

## Helm-Safe Assets

If generated manifests are rendered again by Helm, remaining plain placeholders in text assets can collide with Helm syntax.

Use:

```bash
kube-build-app build -e test -R environments -t deploy/test --helm-escape-assets
```

Remaining placeholders are wrapped from:

```text
{{VAR}}
```

into:

```text
{{`{{VAR}}`}}
```

If `transform: true` is set on the asset, build-time variable transform is applied first, then remaining placeholders are Helm-escaped.

## mTLS Assets

When container-level `mtls.enabled: true` is used, `kube-build-app` automatically mounts encrypted mTLS files into the container.

Inventory exposes the expected generated paths so external tooling can produce the corresponding secured material.

## Docker

Build image:

```bash
docker build -t kube-build-app:latest .
```

Run:

```bash
docker run --rm \
  -v "$PWD/environments:/work/environments:ro" \
  -v "$PWD/deploy:/work/deploy" \
  kube-build-app:latest \
  kube-build-app build -e test -R /work/environments -t /work/deploy/test
```

## Development

Run tests:

```bash
just test
```

Build local binaries:

```bash
just build
```

Build cross-platform binaries:

```bash
just build-cross
just build-cross-all
```

Build release binaries with version metadata and SHA256 checksums:

```bash
just release 0.1.0
```

Run README smoke fixture:

```bash
just readme-smoke
```

The fixture lives in `fixtures/readme-smoke/environments/dev` and verifies the documented CLI flow against a small environment repository:

```bash
kube-build-app validate -e dev -R fixtures/readme-smoke/environments
kube-build-app list -e dev -R fixtures/readme-smoke/environments
kube-build-app summary -e dev -R fixtures/readme-smoke/environments
kube-build-app inventory -e dev -R fixtures/readme-smoke/environments
kube-build-app build -e dev -R fixtures/readme-smoke/environments -t /tmp/kube-build-app-readme-smoke
```

Run parity against Ruby reference implementation:

```bash
scripts/parity-build \
  --name cetin-test \
  --root /path/to/tsm-environments \
  --env test \
  --release-id 2025.08.18.1 \
  --go-bin ./dist/kube-build-app
```

Run representative smoke parity cases:

```bash
just parity-smoke
```

## GitLab Release Pipeline

The GitLab pipeline uses `VERSION` as the release trigger.

On default branch, changing `VERSION` runs:

- release notes generation from git history
- cross-platform `kube-build-app` builds
- `tar.gz` package creation
- `SHA256SUMS` generation
- GitLab Release creation/update
- upload to GitLab Generic Package Registry

Release package names:

```text
kube-build-app-<version>-darwin-arm64.tar.gz
kube-build-app-<version>-darwin-amd64.tar.gz
kube-build-app-<version>-linux-amd64.tar.gz
kube-build-app-<version>-linux-arm64.tar.gz
kube-build-app-<version>-windows-amd64.tar.gz
SHA256SUMS
```

Each platform package contains both binaries:

```text
kube-build-app
kube-edit-app
```

Optional `CHANGELOG.md` push:

```text
RELEASE_PUSH_TOKEN
```

When `RELEASE_PUSH_TOKEN` is set in GitLab CI/CD variables, the publish job updates `CHANGELOG.md` and pushes it back with `[skip ci]`.
