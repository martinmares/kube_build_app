# kube-build-app metamodel plan

Last updated: 2026-05-17

## Core Principle

`kube-build-app` is not Helm and should not become a generic Kubernetes object templating engine.

Target contract:

```text
80 % of common deployment needs = covered by a clear metamodel
20 % of special Kubernetes cases = handled by explicit raw escape hatches
```

The metamodel should stay readable for L2/support/developers and should generate predictable Kubernetes YAML. When a feature becomes too Kubernetes-specific or too rare, prefer a `raw:` block over inventing a large DSL.

## Current Problems

- `health` and `probe` coexist and the names are confusing.
- `health` generates only liveness/readiness and cannot express startup probes.
- `probe` is flexible but verbose; Spring Boot services repeat the same actuator blocks many times.
- Scheduling is limited to `arch`, `node_selector` and `tolerations`; full affinity/topology support is missing.
- Volume-like declarations are currently mixed into `assets`. `temp`, `pvc`, `nfs-server` and `host-path` are volumes, not assets.
- `resources.cpu.from/to` works, but `from/to` is not Kubernetes vocabulary. Keep it for compatibility. New files should prefer `requests/limits`.
- Pod/deployment raw escape hatches now exist as explicit `deployment_raw` and `pod_raw` blocks.
- Pod/container `security_context` now has a dedicated first-class field; `raw.securityContext` remains only a compatibility escape hatch.

## Probes Direction

Add a new `probes` container-level block. Keep `health` and `probe` as legacy-compatible inputs.

Recommended simple Spring Boot form:

```yaml
containers:
  - name: tsm-ticket
    probes:
      preset: spring-actuator
      port: "{{var:EXPOSE_PORT}}"
```

This should generate:

```text
livenessProbe  -> /actuator/health/liveness
readinessProbe -> /actuator/health/readiness
startupProbe   -> /actuator/health
```

Recommended generic HTTP form:

```yaml
probes:
  preset: http
  port: "{{var:EXPOSE_PORT}}"
  path: /actuator/health
```

Recommended override form:

```yaml
probes:
  http:
    port: "{{var:EXPOSE_PORT}}"
    path: /actuator/health
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

Exec probes stay possible:

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

Precedence:

```text
legacy health -> legacy probe -> new probes -> container raw
```

This means `probes` intentionally overrides old `health`/`probe` output when both are present.

## Scheduling Direction

Add a new app-level `scheduling` block while keeping old `arch`, `node_selector`, `tolerations`.

Compatibility form:

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
```

Simple topology spread:

```yaml
scheduling:
  spread:
    by: hostname
    max_skew: 1
    when_unsatisfiable: ScheduleAnyway
```

Simple self anti-affinity:

```yaml
scheduling:
  anti_affinity:
    self: preferred
    topology: kubernetes.io/hostname
```

Raw Kubernetes fallback:

```yaml
scheduling:
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            topologyKey: kubernetes.io/hostname
            labelSelector:
              matchLabels:
                app.kubernetes.io/name: tsm-ticket
```

## Mounts Direction

This is the next priority after `probes` and `scheduling`.

The current `assets` block must remain backward compatible. Existing forms such as these must keep working:

```yaml
assets:
  - file: assets/app.conf
    to: /app/app.conf
  - temp: true
    to: /app/cache
  - pvc: true
    name: data-claim
    to: /data
  - nfs-server: nfs.local
    path: /export/data
    to: /nfs
  - host-path: /var/lib/host-data
    to: /host
```

But conceptually only the first item is an asset. The others are volume mounts.

Preferred future model:

```yaml
containers:
  - name: api
    mounts:
      - type: config
        file: assets/app.conf
        mount_path: /app/app.conf
        transform: true

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

Raw escape hatch:

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

Migration rules:

- keep `assets` as legacy-compatible input
- add `mounts` as the preferred new input
- generated Kubernetes output should be equivalent for old and new forms
- do not remove `assets.temp/pvc/nfs-server/host-path`
- add validation warnings later, not hard failures
- explicitly render `emptyDir: {}` for `temp` / `empty_dir`; a volume with only `name` is not a valid conceptual model
- treat `host_path` as a high-risk escape hatch and document it as such

## Security Context Direction

Add dedicated pod/container `security_context` fields for the common Kubernetes `securityContext` use cases.

App-level form:

```yaml
security_context:
  runAsNonRoot: true
  fsGroup: 2000
```

Container-level form:

```yaml
containers:
  - name: api
    security_context:
      allowPrivilegeEscalation: false
      runAsUser: 1000
      capabilities:
        drop: ["ALL"]
```

Compatibility rule:

```text
security_context -> container raw
```

This keeps existing `raw.securityContext` behavior working, while giving new files a readable first-class model field.

## Lifecycle and Termination Direction

Add app-level `termination_grace_period` and container-level `lifecycle.pre_stop` for common graceful shutdown cases.

App-level form:

```yaml
termination_grace_period: 45
```

Container-level simple exec form:

```yaml
containers:
  - name: api
    lifecycle:
      pre_stop:
        command: ["/bin/sh", "-c", "sleep 10"]
```

Raw handler passthrough remains available for uncommon Kubernetes lifecycle handler forms:

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

This supports graceful shutdown without forcing users into raw Kubernetes syntax for the common `preStop.exec.command` case.

## Service Account and Env From Direction

Add app-level `service_account` and container-level `env_from` for common identity and environment import use cases.

App-level form:

```yaml
service_account: tsm-api
```

Container-level simple form:

```yaml
containers:
  - name: api
    env_from:
      - config_map: api-config
      - secret: api-secret
        prefix: SECRET_
```

Native Kubernetes reference form remains available for less common options such as `optional`:

```yaml
env_from:
  - configMapRef:
      name: api-config
      optional: true
  - secretRef:
      name: api-secret
      optional: true
```

## Init Containers Direction

Add app-level `init_containers` for generic Kubernetes init containers while keeping `tools` as the preferred shortcut for static utility binaries.

Recommended form:

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

Supported fields intentionally mirror the common subset of regular containers:

```text
name, image, command, arguments, env_vars, env_from, mounts, security_context, resources, runtime.java, raw
```

## Resources Direction

Keep legacy `from` / `to`, but prefer Kubernetes-aligned `requests` / `limits` in new files.

Preferred form:

```yaml
resources:
  cpu:
    requests: "100m"
    limits: "500m"
  memory:
    requests: "128Mi"
    limits: "512Mi"
```

Legacy-compatible form:

```yaml
resources:
  cpu:
    from: "100m"
    to: "500m"
```

If both forms are present for the same resource, `requests` / `limits` win.

## Java Runtime Direction

Java is the only runtime with a currently standardized metamodel shortcut. The goal is to make JVM heap sizing visible next to Kubernetes `resources`, especially in `kube-build-app summary`.

```yaml
containers:
  - name: api
    runtime:
      java:
        xms: "512m"
        xmx: "2048m"
        opts:
          - "-XX:+UseG1GC"
        export:
          env_name: JAVA_OPTS
```

Contract:

- `runtime.java.xms` renders as `-Xms...`.
- `runtime.java.xmx` renders as `-Xmx...`.
- `runtime.java.opts` is appended after `xms` and `xmx`.
- `runtime.java.export.env_name` defaults to `JAVA_OPTS`.
- A manual `env_vars` item with the same name is a validation error.
- Other runtimes are intentionally left to `env_vars` and `raw` until a repeated real pattern appears.

## Raw Escape Hatch Direction

Use explicit raw blocks by target layer.

Deployment-level:

```yaml
deployment_raw:
  spec:
    revisionHistoryLimit: 2
```

Pod spec-level:

```yaml
pod_raw:
  dnsPolicy: ClusterFirst
  enableServiceLinks: false
```

Container-level `raw` remains supported:

```yaml
containers:
  - name: api
    raw:
      stdin: true
```

This avoids an ambiguous root-level `raw` block and makes the target layer obvious.

## Metamodel Candidate Status

Priority order and current status:

1. `probes` - done
2. `scheduling` - done
3. `mounts` - done
4. pod/container `security_context` - done
5. `lifecycle`, especially `preStop` - done
6. `termination_grace_period` - done
7. `service_account` - done
8. `env_from` - done
9. more general `init_containers` - done
10. pod/deployment/container `raw` escape hatches - done
11. optional HPA support - done
12. Java runtime options - done

## Autoscaling / HPA Direction

`autoscaling` is an app-level block that generates a separate Kubernetes `HorizontalPodAutoscaler` manifest:

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
  raw:
    spec:
      behavior:
        scaleDown:
          stabilizationWindowSeconds: 300
```

Generated file:

```text
deployments/<app>-hpa.yml
```

Contract:

- `autoscaling.enabled=true` means runtime replica count is controlled by HPA.
- `replicas` remains in the Deployment/StatefulSet as initial desired state.
- CPU and memory utilization metrics require `resources.requests`.
- `autoscaling.raw` is the escape hatch for advanced HPA fields, for example `spec.behavior`.

## Implementation Order

1. Document the metamodel contract and 80/20 principle.
2. Add `probes` without removing `health` or `probe`.
3. Add `scheduling` without removing `arch`, `node_selector`, `tolerations`.
4. Stabilize `probes` and `scheduling` with tests and real repository parity.
5. Add `mounts` without removing legacy `assets` volume forms.
6. Add tests proving old YAML still generates the same output.
7. Migrate real app YAML gradually only after the generator behavior is stable.
8. Add `security_context` as a dedicated pod/container field while keeping `raw.securityContext`.
9. Add `lifecycle.pre_stop` and `termination_grace_period` for graceful shutdown cases.
10. Add `service_account` and `env_from` as dedicated fields.
11. Add generic `init_containers` while keeping `tools` as a specialized shortcut.
12. Add `resources.requests/limits` aliases while keeping `from/to`.
13. Add explicit `deployment_raw` and `pod_raw` escape hatches.
14. Add `autoscaling` and generate `HorizontalPodAutoscaler`.
15. Add narrow `runtime.java` support for JVM heap/options.
