# Build Kubernetes manifests easily

Simple abstraction over kubernetes manifests.

## Typical workflow

You want create deployment for your awesome **brand new product**

```bash
mkdir ~/my-brand-new-product-k8s
cd ~/my-brand-new-product-k8s
```

Inside `my-brand-new-product-k8s` directory

```bash
cd ~/my-brand-new-product-k8s

mkdir environments
mkdir deployments
```

Inside `environments` (you want create deployment for two separate `environments`, first => `test`, second => `production`)

```bash
cd  ~/my-brand-new-product-k8s/environments

mkdir -p test/apps
mkdir -p production/apps
mkdir -p test/assets
mkdir -p production/assets
```

Inside `deployments` (you want create deployment for two separate `targets`, first => `test`, second => `production`)

```bash
cd  ~/my-brand-new-product-k8s/deployments

mkdir -p test/deploy
mkdir -p production/deploy
```

You should see the following directory structure (inside `my-brand-new-product-k8s` directory)

```bash
cd ~/my-brand-new-product-k8s
tree -d

.
├── deployments
│   ├── production
│   │   └── deploy
│   └── test
│       └── deploy
└── environments
    ├── production
    │   ├── apps
    │   └── assets
    └── test
        ├── apps
        └── assets

12 directories
```

You will now create a sample file describing the `application` for `test` environment

Create file with name `brand-new-product.yml` (inside directory `~/my-brand-new-product-k8s/environments/test/apps`). With the following content

```yaml
name: brand-new-product
disable_shared_assets: true
replicas: 1
containers:
  - name: brand-new-product
    image: docker.io/library/nginx:stable
    env_vars:
      - name: ENV_VAR
        value: "some value ..."
      - name: ANOTHER_VAR
        value: "another value ..."
    assets:
      - file: assets/nginx.conf
        to: /etc/nginx/conf.d/default.conf
    ports:
      - name: http
        port: 8080
    resources:
      cpu:
        from: "100m"
        to: "250m"
      memory:
        from: "50Mi"
        to: "100Mi"
    health:
    health:
      http:
        path:
          live: /index.html
          ready: /index.html
        port: 8080
```

### Tools utility binaries (initContainers + /app/tools)

You can expose static utility binaries from dedicated images into app containers.

Add app-level `tools:` block:

```yaml
tools:
  - name: util-encjson-rs
    image: your-registry/util-encjson-rs:latest
    expose_bin: /usr/bin/encjson-rs
    as: /app/tools/encjson

  - name: util-apply-env
    image: your-registry/util-apply-env:latest
    expose_bin: /usr/bin/apply-env
    # when `as` is omitted, default target is /app/tools/<basename(expose_bin)>
```

Behavior:

- `tools` entries are generated as `initContainers`.
- each initContainer copies `expose_bin` into shared `emptyDir` volume.
- all app containers get the shared volume mounted read-only to `/app/tools`.
- convention-over-configuration default target path is `/app/tools/...`.
- if `as` points outside `/app/tools`, only basename is used and binary is still exposed under `/app/tools`.
- you can define `tools` globally in `apps/_defaults.yml` and it will be merged into all apps.
- app file has higher priority than `_defaults.yml`; explicit `tools: null` in app disables inherited default tools.

### App defaults via `apps/_defaults.yml`

`apps/_defaults.yml` is the single env-level defaults file for app model loading.

It is not treated as a buildable app. Instead, it is loaded and merged into every `apps/*.yml`.

Use it for app-level defaults such as:

- `tools`
- `vars`
- `container_env_vars`

Example:

```yaml
tools:
  - name: util-encjson-rs
    image: your-registry/util-encjson-rs:latest
    expose_bin: /usr/bin/encjson-rs
    as: /app/tools/encjson

vars:
  - name: GLOBAL_FOO
    value: "bar"

container_env_vars:
  - name: "*"
    env_vars:
      - name: GLOBAL_FLAG
        value: "true"

  - name: "tsm-dms"
    env_vars:
      - name: JAVA_OPTS
        value: "-Xms256m"
```

`vars` semantics:

- matched by `name`
- app-level item fully replaces default item with the same `name`
- no field-level merge

`container_env_vars` semantics:

- only valid in `apps/_defaults.yml`
- applied to app containers by `container.name`
- `name: "*"` means wildcard defaults for all containers
- then a matching concrete `container.name` is applied
- finally local `containers[].env_vars` from app YAML are applied
- matching is done by env var `name`
- higher layer fully replaces lower layer item

Removal / tombstone:

```yaml
vars:
  - name: GLOBAL_FOO
    remove: true
```

```yaml
containers:
  - name: tsm-dms
    env_vars:
      - name: GLOBAL_FLAG
        remove: true
```

Rules for `remove: true`:

- removes the item completely from final effective model
- must not be combined with other data fields
- invalid combinations fail fast

Precedence summary:

- generic hash keys: current recursive `_defaults.yml` merge, app values win
- `vars`: whole-object replace by `name`
- `container_env_vars` -> `containers[].env_vars`: whole-object replace by `name`

### Cgroup Exporter Defaults (container-level)

At container level you can enable automatic env var injection for cgroup exporter:

```yaml
containers:
  - name: your-app
    enable_cgroup_exporter: true
```

When enabled, these defaults are injected automatically (only if not already present in `env_vars`):

- `CGROUP_EXPORTER_METRICS_PREFIX`
- `CGROUP_EXPORTER_METRICS_STATIC_LABELS`
- `CGROUP_EXPORTER_LISTEN`
- `CGROUP_EXPORTER_CPU_REQUESTS_MCPU`
- `CGROUP_EXPORTER_CPU_LIMITS_MCPU`
- `CGROUP_EXPORTER_MEMORY_REQUESTS_MIB`
- `CGROUP_EXPORTER_MEMORY_LIMITS_MIB`
- `CGROUP_EXPORTER_NODE_NAME`

Create file with name `nginx.conf` (inside directory `~/my-brand-new-product-k8s/environments/test/assets`). With the following content

```nginx
daemon off;
worker_processes  2;

server {
    listen       8080;
    server_name  localhost;

    location / {
        root   /usr/share/nginx/html;
        index  index.html index.htm;
    }

    error_page   500 502 503 504  /50x.html;
    location = /50x.html {
        root   /usr/share/nginx/html;
    }
}
```

You should see the following directory structure (inside `~/my-brand-new-product-k8s` directory)

```bash
cd ~/my-brand-new-product-k8s
tree -f

.
├── ./deployments
│   ├── ./deployments/production
│   │   └── ./deployments/production/deploy
│   └── ./deployments/test
│       └── ./deployments/test/deploy
├── ./env.secured.json
├── ./env.unsecured.json
├── ./environments
│   ├── ./environments/production
│   │   ├── ./environments/production/apps
│   │   └── ./environments/production/assets
│   └── ./environments/test
│       ├── ./environments/test/apps
│       │   └── ./environments/test/apps/brand-new-product.yml
│       └── ./environments/test/assets
│           └── ./environments/test/assets/nginx.conf
└── ./shared.assets.yml

12 directories, 5 files
```

**You are now ready to create your first `deployment`, run!**

```bash
kube_build_app -e test -t deployments/test/deploy
```

```log
Build 0 shared asset/s
Application brand-new-product, with 1 container/s
 => container [1] brand-new-product has 1 asset/s
 => asset [1]: brand-new-product-asset-66535d3
 => container [1] brand-new-product has 1 port/s
 => service [1] brand-new-product, has 1 port/s
   => has external (ingress) brand-new-product
```

You should see the following directory/file structure (inside `~/my-brand-new-product-k8s/deployments/test/deploy` directory)

```bash
cd ~/my-brand-new-product-k8s/deployments/test/deploy
tree -f

.
├── ./assets
│   └── ./assets/brand-new-product-asset-66535d3.yml
├── ./deployments
│   └── ./deployments/brand-new-product-deployment.yml
└── ./services
    ├── ./services/brand-new-product-service.yml
    └── ./services/external
        └── ./services/external/brand-new-product-ingress.yml

4 directories, 4 files
```

```yaml
---
apiVersion: v1
data:
  nginx.conf: |
    daemon off;
    worker_processes  2;

    server {
        listen       8080;
        server_name  localhost;

        location / {
            root   /usr/share/nginx/html;
            index  index.html index.htm;
        }

        error_page   500 502 503 504  /50x.html;
        location = /50x.html {
            root   /usr/share/nginx/html;
        }
    }
kind: ConfigMap
metadata:
  name: brand-new-product-asset-66535d3
  namespace:
```

```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: brand-new-product
  namespace:
spec:
  selector:
    app.kubernetes.io/name: brand-new-product
  ports:
    - name: http-80
      port: 80
      targetPort: 8080
```

```yaml
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: brand-new-product
  namespace:
spec:
  rules:
    - host: brand-new-product.my-domain-name.io
      http:
        paths:
          - path: "/"
            backend:
              service:
                name: brand-new-product
                port:
                  number: 80
            pathType: ImplementationSpecific
```

```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/name: brand-new-product
  name: brand-new-product
  namespace:
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: brand-new-product
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app.kubernetes.io/name: brand-new-product
    spec:
      containers:
        - name: brand-new-product
          image: docker.io/library/nginx:stable
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
          env:
            - name: ENV_VAR
              value: some value ...
            - name: ANOTHER_VAR
              value: another value ...
          imagePullPolicy: Always
          resources:
            requests:
              cpu: 100m
              memory: 50Mi
            limits:
              cpu: 250m
              memory: 100Mi
          volumeMounts:
            - mountPath: "/etc/nginx/conf.d/default.conf"
              name: brand-new-product-asset-66535d3
              readOnly: true
              subPath: nginx.conf
          livenessProbe:
            httpGet:
              path: "/index.html"
              port: 8080
            initialDelaySeconds:
            periodSeconds:
            timeoutSeconds:
            successThreshold:
            failureThreshold:
          readinessProbe:
            httpGet:
              path: "/index.html"
              port: 8080
            initialDelaySeconds:
            periodSeconds:
            timeoutSeconds:
            successThreshold:
            failureThreshold:
      imagePullSecrets: []
      volumes:
        - name: brand-new-product-asset-66535d3
          configMap:
            defaultMode: 420
            name: brand-new-product-asset-66535d3
```

## Deployment profiles (maintenance / normal mode)

You can keep app-level replicas in `apps/*.yml`, but override them using one profile file:

`environments/<env>/replica-profiles.yml`

```yaml
defaults:
  profile: normal

profiles:
  normal:
    apps:
      tsm-gateway: 2
      tsm-ticket: 2

  db-maintenance:
    all: 0
    apps:
      tsm-log-server: 1
      tsm-health-checker: 1
```

Run with explicit profile:

```bash
kube_build_app -e test -p db-maintenance -t deployments/test/deploy
```

Or choose file manually:

```bash
kube_build_app -e test -p normal --profiles-file /some/path/replica-profiles.yml
```

Priority order:

1. Profile selected by `-p/--profile`
2. Profile selected by `REPLICA_PROFILE` env var
3. `defaults.profile` from `replica-profiles.yml`

## Ignore app from build

You can keep app file in git but skip all generated output:

```yaml
ignore: true
```

When set on app root, `kube_build_app` excludes that app from loading/building (no deployment/service/assets are generated for it).

`-w/--down` still works and is applied after profile overrides.

## Model validation (fail fast)

You can validate app model without generating manifests:

```bash
kube_build_app validate -e test
```

Validation rules include:

- if `simple_init.enabled: true`, container must not define `startup` (XOR conflict)
- in `simple_init` mode, `simple_init.exec.command` must be a non-empty array

## Explicit env file (`-E`, `--env-file`)

You can pass an already resolved `.env`-style file and use it as the only variable source:

```bash
kube_build_app -e test -E /path/to/release.env
```

This is the intended integration point for outputs produced elsewhere, for example:

```bash
simple-secrets resolve --env test -o dot-env --file /tmp/test.env
kube_build_app -e test -E /tmp/test.env
```

In this mode, `kube_build_app` does not read:

- `env.unsecured.json`
- `env.secured.json`
- process `ENV`

Supported `.env` format:

- empty lines and lines starting with `#` are ignored
- optional `export ` prefix is allowed
- `KEY=VALUE`
- quoted values (`"..."` or `'...'`) are supported

Notes:

- `-E/--env-file` cannot be combined with `-d/--decrypt-secured`
- `-E/--env-file` cannot be combined with `--vars-source`

## Variable source selection (`--vars-source`)

You can explicitly choose where variables are loaded from (repeatable):

```bash
kube_build_app -e test --vars-source env
kube_build_app -e test --vars-source json
```

Supported values:

- `env` - load variables from process `ENV`
- `json` - load variables from `env.unsecured.json` (+ `env.secured.json` when `-d` is enabled)
- `dot-env` - load variables from conventional file `<environment_dir>/.env`

When multiple sources are used, they are applied in the order you pass them and later sources override earlier ones.

Use `--vars-source dot-env` only when you want the implicit conventional file `<environment_dir>/.env`.
If you already have an explicit resolved `.env` file path, use `-E/--env-file` instead.

Default behavior (for backward compatibility) when `--vars-source` is not specified:

- `json`
- `env`

## Helm-safe assets (`--helm-escape-assets`)

If your generated manifests are rendered again by Helm, asset placeholders can collide with Helm template syntax.

Use:

```bash
kube_build_app -e test --helm-escape-assets
```

For text assets, remaining placeholders are wrapped from:

- `{{ VAR }}`

to:

- `{{`{{ VAR }}`}}`

Notes:

- binary assets are not modified
- if `transform: true`, normal variable transform is applied first, then remaining placeholders are Helm-escaped
- already wrapped `{{`{{ VAR }}`}}` values are not wrapped again
- per-asset override is possible with `helm_escape: true|false`

## Inventory mode (`-i`)

Print detailed app/container inventory JSON and exit:

```bash
kube_build_app -i -e test
```

All containers are included. For mTLS pipeline, filter items where:

```yaml
mtls:
  enabled: true
```

Output is JSON on stdout, intended for piping:

```bash
kube_build_app -i -e test | simple-spiffe-pki generate -
```

When container has:

```yaml
mtls:
  enabled: true
```

`kube_build_app` also auto-mounts encrypted mTLS files into container:

- `/app/mtls.enc/mtls.secured.json`
- `/app/mtls.enc/mtls.secured.schema.json`

Source files are expected in environment repo:

- `<env>/mtls/<app>/<container>.secured.json`
- `<env>/mtls/<app>/<container>.secured.schema.json`

## Environment root (`-R`, `--root-dir`)

By default, environments are loaded from:

- `environments/<env>` (or `ENVIRONMENTS_DIR/<env>` when `ENVIRONMENTS_DIR` is set)

You can override root directory explicitly:

```bash
kube_build_app -e test -R /Users/mares/Development/Src/Ruby/tsm/cetin/tsm-environments
```

## Docker (openSUSE / OpenShift-friendly)

Build image:

```bash
docker build -t kube-build-app:latest .
```

Image always builds and embeds static utility binaries:

- `apply-env` from `https://github.com/martinmares/apply-env-rs`
- `encjson-rs` from `https://github.com/martinmares/encjson-rs`
- `simple-policy-engine` is also cloned during `encjson-rs` build
- legacy `encjson` from `https://github.com/martinmares/encjson`

You can pin refs/branches at build time:

```bash
docker build -t kube-build-app:latest \
  --build-arg APPLY_ENV_REF=main \
  --build-arg ENCJSON_REF=main \
  --build-arg SIMPLE_POLICY_ENGINE_REF=main \
  --build-arg ENCJSON_LEGACY_REF=main \
  .
```

`kube_build_app` chooses encjson binary by file API marker:

- `EncJson[@api=1.0` -> legacy binary (`ENCJSON_LEGACY_PATH`, default `/app/bin/encjson-legacy`)
- `EncJson[@api=2.0` -> rust binary (`ENCJSON_PATH`, default `/app/bin/encjson-rs`)

Run against local environments repo:

```bash
docker run --rm -it \
  -v /absolute/path/to/tsm-environments:/work/environments:ro \
  -v /absolute/path/to/output:/work/output \
  kube-build-app:latest \
  -e test -R /work/environments -t /work/output
```

Container conventions:

- base image: `opensuse/tumbleweed:latest`
- runtime user: `UID=1001`, `GID=1001`, `HOME=/app`
- `/app` is writable and OpenShift-friendly (`chgrp -R 0` + `chmod -R g=u`)

## Tests

Run all tests:

```bash
ruby -Itest -e 'Dir["test/*_test.rb"].sort.each { |f| require_relative f }'
```

Run a single test file:

```bash
ruby -Itest test/comprehensive_features_test.rb
```
