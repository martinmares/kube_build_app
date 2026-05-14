# kube-build-app metamodel plan

Last updated: 2026-05-14

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
- `resources.cpu.from/to` works, but `from/to` is not Kubernetes vocabulary. Keep it for compatibility, but consider accepting `requests/limits` later.
- `raw` exists at container level only; pod/deployment-level escape hatches should be added later.

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

## Future Metamodel Candidates

Priority order:

1. `probes`
2. `scheduling`
3. pod/container `security_context`
4. `lifecycle`, especially `preStop`
5. `termination_grace_period`
6. `service_account`
7. `env_from`
8. more general `init_containers`
9. pod/deployment/container `raw` escape hatches
10. optional HPA support, likely as a separate manifest/metamodel

## Implementation Order

1. Document the metamodel contract and 80/20 principle.
2. Add `probes` without removing `health` or `probe`.
3. Add `scheduling` without removing `arch`, `node_selector`, `tolerations`.
4. Add tests proving old YAML still generates the same output.
5. Migrate real app YAML gradually only after the generator behavior is stable.

