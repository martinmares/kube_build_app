# kube-build-app TODO

## Local environment check

Add a local, cluster-independent command:

```bash
kube-build-app check <environment> -R environments
```

The command must run the complete build pipeline without writing into the
deployment target. It should verify that the environment is fully renderable,
including:

- `_defaults.yml` parsing and merge behavior
- app model validation
- referenced asset existence and path safety
- structured asset bundle validity
- ConfigMap keys, duplicate mount paths and generated object names
- tools, sidecars and init containers
- unresolved build-time placeholders
- ConfigMap size limits
- serialization of all generated Kubernetes objects
- deprecated fields, with `--fail-on-deprecated` support

`validate` remains the fast model-contract check. `check` verifies the complete
environment and all render-time dependencies. A pragmatic first implementation
may build into an automatically removed temporary directory.
