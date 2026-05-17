# kube-ops-app plan

> Stav: **návrh zafixován pro další implementaci**. Žádný kód zatím neexistuje.

`kube-ops-app` není klon ArgoCD. Cílem je doménový OpenShift/Kubernetes operations portal nad prostředími generovanými přes `kube-build-app`.

Základní kontrakt:

```text
metamodel commit -> rendered manifest digest -> apply record -> runtime status
```

Cluster nikdy neporovnáváme přímo proti `<app>.yml`. Cluster porovnáváme proti deterministicky vygenerovaným Kubernetes manifestům z konkrétní git revize.

---

## Cíl

Využít `kube-build-app` metamodel tak, aby z něj měli benefit:

- vývojáři a release pipeline,
- CLI-first uživatelé,
- L2 support, který potřebuje bezpečný a srozumitelný provozní pohled na OpenShift/Kubernetes.

`kube-ops-app` má odpovídat na otázky:

```text
Co je definováno v metamodelu?
Co bylo vyrenderováno?
Co bylo aplikováno do clusteru?
Co reálně běží?
Co s tím může L2 bezpečně udělat?
```

---

## Co explicitně neděláme

Nechceme stavět obecný GitOps systém ani plnohodnotnou náhradu ArgoCD.

Neděláme:

- vlastní Kubernetes API server,
- obecný Helm/Kustomize orchestrátor,
- univerzální multi-tenant GitOps controller,
- kompletní ArgoCD health engine,
- automatický prune jako výchozí chování,
- neomezený terminál do podu bez audit/RBAC pravidel.

`kube-ops-app` je operations portal a sync runner pro prostředí postavená nad `kube-build-app`.

---

## Základní vrstvy

```text
1. Build/GitOps vrstva
   git repo -> kube-build-app -> rendered manifests -> diff/apply

2. Runtime/Cluster vrstva
   deployments, pods, services, routes, logs, rollout status, events

3. Operations/UI vrstva
   bezpečné akce pro L2: sync, restart, shutdown, startup, plánované okno, diagnostika
```

První verze má být malá, ostrá a provozně použitelná.

---

## Flow

```text
repo s metamodelem
  -> kube-ops-app sleduje target revision
  -> checkout konkrétní revize
  -> kube-build-app build do pracovního/temp adresáře
  -> výpočet manifest digestu
  -> diff proti poslednímu aplikovanému digestu a/nebo clusteru
  -> apply přes Kubernetes API
  -> uložení historie runu
  -> UI/CLI zobrazí stav
```

Změna není samotná změna souboru. Změna relevantní pro deploy nastává až když:

```text
render(target_revision) -> manifest_digest
manifest_digest != applied_digest
```

Tím eliminujeme situace, kdy se změnil commit, ale výsledné manifesty jsou stejné.

---

## Target revision

Stejně jako dnes u ArgoCD musí být možné řídit, jaká git revize se má nasadit.

`target_revision` může být:

- branch,
- tag,
- commit SHA.

Rozlišujeme:

```text
target_revision   = co chceme nasadit
resolved_commit   = na jaký commit tag/branch ukázal v době syncu
applied_revision  = co bylo naposled úspěšně aplikováno
applied_digest    = digest naposled úspěšně aplikovaných manifestů
```

Pipeline tak může pouze posunout `target_revision` a zbytek provede `kube-ops-app`.

Příklad:

```sh
kube-ops-app env deploy test \
  --revision 2026.05.17.1 \
  --sync \
  --wait \
  --timeout 10m
```

Interně:

```text
set target_revision = 2026.05.17.1
render
if digest changed or force sync -> apply
wait for rollout status
store run result
```

---

## Co znamená sync

### Normal sync

```text
render target revision
spočti manifest digest
pokud digest != applied_digest -> apply
jinak no-op
```

### Force sync

```text
render target revision
apply vždy, i když manifest digest vypadá stejně
```

### Force conflicts

Samostatná nebezpečnější operace pro server-side apply ownership konflikty.

```text
force sync       = znovu aplikuj stejné manifesty
force conflicts  = přepiš field ownership konflikty
```

Tyto dvě věci musí být v UI i CLI oddělené.

---

## Apply mechanismus

Preferovaný směr: jedna binárka běžící v clusteru se ServiceAccount.

Nepreferujeme dlouhodobě `kubectl` subprocess. Pro první prototyp by byl možný, ale cílově chceme Kubernetes Go klienta.

Doporučené řešení:

```text
rendered YAML
  -> decode na unstructured.Unstructured
  -> dynamic Kubernetes client
  -> Server-Side Apply
```

Parametry:

```text
fieldManager: kube-ops-app
force: false běžně
force: true jen pro explicitní force-conflicts
```

Výhody:

- jedna binárka,
- nativní in-cluster ServiceAccount,
- auditovatelný field manager,
- žádná závislost na `kubectl` v image.

---

## Metadata v Kubernetes objektech

Každý vygenerovaný objekt musí dostat labely/anotace, které umožní audit, status a bezpečné filtrování.

Příklad:

```yaml
metadata:
  labels:
    app.kubernetes.io/managed-by: kube-ops-app
    kube-ops-app.datalite.cz/environment: test
  annotations:
    kube-ops-app.datalite.cz/git-revision: "2026.05.17.1"
    kube-ops-app.datalite.cz/resolved-commit: "abc123..."
    kube-ops-app.datalite.cz/manifest-hash: "sha256:..."
```

Tyto hodnoty jsou důležité pro:

- UI status,
- audit,
- orphaned resource report,
- bezpečné budoucí prune.

---

## Prune

Nejtěžší a nejrizikovější část je mazání resources, které zmizely z metamodelu.

První verze:

```text
bez automatického prune
pouze report orphaned resources
```

Pozdější verze:

```text
volitelný prune
jen pro objekty s managed-by=kube-ops-app
jen v explicitně spravovaném namespace
jen po diff preview
ideálně s potvrzením nebo policy
```

Nikdy ne globálně přes namespace bez přesných label selectorů.

---

## Co by měla L2 vidět

UI nemá začínat Kubernetes objekty. UI má začínat doménovým pohledem.

```text
Environment
  Applications
    Deployments
    Pods
    Services
    Routes/Ingress
    ConfigMaps/Secrets references
    Events
    Logs
    Actions
```

L2 nepotřebuje primárně řešit `ReplicaSet`, `EndpointSlice`, `OwnerReference` nebo raw YAML. Tyto detaily mají být dostupné až po rozkliku.

### Environment view

- název prostředí,
- namespace,
- target revision,
- resolved commit,
- applied revision,
- applied digest,
- sync status,
- autosync enabled/disabled,
- poslední sync run,
- poslední chyba.

### Application view

- aplikace podle metamodelu,
- deployment/deployments,
- image/tag,
- replicas desired/available,
- rollout status,
- resource requests/limits,
- services,
- routes/ingress,
- relevantní events,
- poslední logy podů.

### Pod view

- stav podu,
- restart count,
- node,
- image,
- containers,
- logs,
- previous logs,
- events,
- describe-like diagnostika.

Terminál/exec do podu je možný později, ale pouze s RBAC, auditem a explicitním oprávněním.

---

## Bezpečné akce pro L2

První sada akcí:

```text
Sync
Force sync
Rollout restart deploymentu
Scale deploymentu na 0
Scale deploymentu zpět
Pause auto-sync
Resume auto-sync
View logs
View events
View rendered manifest
View diff
```

Akce musí být auditované:

```text
who
when
environment
object
action
parameters
result
```

---

## Maintenance / shutdown / startup

Tohle je zásadní doménová hodnota proti běžnému ArgoCD/OpenShift dashboardu.

Potřebujeme řídit provozní scénáře typu:

- odstavení prostředí před upgradem clusteru,
- řízený startup v pořadí,
- řízený shutdown v opačném pořadí,
- dočasné zakázání startu všech mikroslužeb,
- plánovaná údržbová okna.

Důležité: shutdown nesmí být jen ad-hoc scale na 0. Musí existovat provozní stav uložený v DB, aby další sync služby znovu nenahodil.

Příklad DB stavu:

```text
environment_state:
  startup_policy: enabled | disabled | scheduled
  disabled_apps: [...]
  reason: "upgrade cluster A"
  created_by
  created_at
```

Příklad CLI:

```sh
kube-ops-app maintenance shutdown test --profile upgrade
kube-ops-app maintenance startup test --profile upgrade
kube-ops-app maintenance disable-startup test --reason "cluster upgrade"
kube-ops-app maintenance enable-startup test
```

---

## Server config vs environment ops config

Konfiguraci dělíme na dvě části.

### Server bootstrap config

Tato konfigurace je mimo sledované environment repo. Je součástí deploymentu `kube-ops-app`, typicky jako ConfigMap/Secret.

Příklad umístění:

```text
/etc/kube-ops-app/config.yml
```

Start:

```sh
kube-ops-app server --config /etc/kube-ops-app/config.yml
```

Příklad:

```yaml
server:
  http:
    listen: ":8080"

storage:
  postgres:
    dsn: "${KUBE_OPS_DATABASE_URL}"

auth:
  mode: openshift-oauth

environments:
  - name: test
    namespace: tsm-test
    repo: git@gitlab.cloud-app.cz:tsm/tsm-environments.git
    branch: main
    root_path: .
    env_name: test
    auto_sync: true

  - name: prod
    namespace: tsm-prod
    repo: git@gitlab.cloud-app.cz:tsm/tsm-environments.git
    branch: main
    root_path: .
    env_name: prod
    auto_sync: false
```

Do server configu patří:

- repo URL,
- namespace binding,
- DB konfigurace,
- auth mode,
- service/server nastavení,
- základní sync policy.

### Environment ops config

Tato konfigurace je součástí environment repozitáře.

Doporučený soubor:

```text
<env>/ops.yml
```

Příklad:

```yaml
display_name: TSM TEST

groups:
  - name: core
    apps:
      - tsm-config-server
      - tsm-gateway

  - name: business
    apps:
      - tsm-ticket
      - tsm-dms

  - name: ui
    apps:
      - tsm-ui

startup_order:
  - core
  - business
  - ui

shutdown_order:
  - ui
  - business
  - core

maintenance:
  profiles:
    upgrade:
      description: Řízené odstavení při upgrade clusteru
      shutdown:
        - group: ui
        - group: business
        - group: core
      startup:
        - group: core
        - group: business
        - group: ui
```

Do `<env>/ops.yml` patří:

- display name,
- doménové skupiny aplikací,
- startup/shutdown pořadí,
- maintenance profily,
- provozní popisky pro UI.

Do `<env>/ops.yml` nepatří:

- Git credentials,
- DB konfigurace,
- server listen config,
- globální cluster oprávnění,
- citlivé údaje.

---

## CLI/API kontrakt

CLI je povinná součást první verze, protože pipeline musí být schopná do `kube-ops-app` řízeně šťouchnout.

Preferovaný tvar: jedna binárka s command groups.

```sh
kube-ops-app server --config /etc/kube-ops-app/config.yml
```

Pipeline/release:

```sh
kube-ops-app env deploy test --revision 2026.05.17.1 --sync --wait --timeout 10m
kube-ops-app env set-revision test 2026.05.17.1
kube-ops-app env sync test --wait
kube-ops-app env status test
kube-ops-app env diff test
```

Rollback:

```sh
kube-ops-app env rollback test --to 2026.05.16.3 --sync --wait
```

Runtime operations:

```sh
kube-ops-app deployment restart test tsm-ui
kube-ops-app deployment scale test tsm-ui --replicas 0
kube-ops-app pod logs test tsm-ui-xxxxx --container tsm-ui
```

Maintenance:

```sh
kube-ops-app maintenance shutdown test --profile upgrade
kube-ops-app maintenance startup test --profile upgrade
```

API musí být navržené tak, aby CLI bylo pouze tenký klient nad stejnými endpointy, které používá UI.

---

## Storage návrh

Minimální DB model:

```sql
environments
  id
  name
  namespace
  repo_url
  repo_path
  env_name
  branch
  target_revision
  applied_revision
  desired_digest
  applied_digest
  auto_sync
  sync_policy

environment_revision_history
  id
  environment_id
  old_revision
  new_revision
  changed_by
  changed_from
  created_at

sync_runs
  id
  environment_id
  requested_revision
  resolved_commit
  manifest_digest
  status
  trigger_type -- auto | manual | cli | api | pipeline
  started_at
  finished_at

sync_logs
  id
  sync_run_id
  level
  message
  created_at

audit_events
  id
  environment_id
  actor
  action
  target_kind
  target_name
  parameters_json
  status
  created_at

environment_runtime_state
  environment_id
  startup_policy
  disabled_apps_json
  maintenance_reason
  updated_by
  updated_at
```

---

## UI návrh

Technologie:

- Go HTTP server,
- Tabler CSS,
- HTMX,
- SSE eventy pro live logy,
- AlpineJS pouze tam, kde HTMX nestačí,
- PostgreSQL jako stavové úložiště.

Základní obrazovky:

```text
Environments
Environment detail
Applications
Application detail
Pods
Sync runs
Diff preview
Rendered manifests
Maintenance plans
Audit log
Settings
```

UI nesmí být jen Kubernetes dashboard. Musí ukazovat doménový pohled odvozený z `kube-build-app` metamodelu a teprve potom technický Kubernetes detail.

---

## Interní struktura v repozitáři

Doporučená varianta: stejný repozitář jako `kube-build-app` a `kube-edit-app`.

Důvody:

- přímý import `internal/buildapp`,
- konzistentní release,
- jeden lifecycle nástrojů kolem metamodelu,
- bez `replace` hacků v `go.mod`.

Navržená struktura:

```text
cmd/
  kube-build-app/
  kube-edit-app/
  kube-ops-app/

internal/
  buildapp/
  webapp/
  opsapp/
    config/
    git/
    render/
    digest/
    apply/
    cluster/
    sync/
    maintenance/
    audit/
    web/
```

---

## MVP

První použitelná verze:

1. Server config s jedním nebo více environmenty.
2. Git fetch/checkout pro `target_revision`.
3. Render přes `buildapp.Build` do temp/work adresáře.
4. Výpočet manifest digestu.
5. Stav `InSync` / `OutOfSync`.
6. Diff preview.
7. Server-side apply přes Go dynamic client.
8. Historie sync runů a logů v PostgreSQL.
9. UI: environments, application list, deployment/pod/service/route detail.
10. CLI: `env status`, `env diff`, `env deploy`, `env sync`, `env rollback`.
11. Runtime akce: rollout restart deploymentu.
12. Maintenance základ: pause/resume auto-sync, scale selected apps to 0/back.

Mimo MVP:

- automatický prune,
- terminál do podu,
- komplexní workflow approvals,
- multi-cluster orchestrace,
- vlastní plný health engine.

---

## Otevřené otázky

1. Název binárky definitivně potvrdit: aktuálně `kube-ops-app`.
2. Auth model pro UI: OpenShift OAuth, reverse proxy auth, nebo vlastní OIDC?
3. Přesná podoba `ops.yml` a jestli má být validovaná přes schema/testy.
4. Jak hluboký runtime status chceme v první verzi: pouze deployments/pods/services/routes, nebo i configmaps/secrets/events/logs?
5. Jak reprezentovat plánované údržbové akce: DB-only, nebo také auditovaný zápis do git repa?
6. Kdy a jak povolit prune.
7. Zda `kube-edit-app` a `kube-ops-app` později spojit v jeden web, nebo držet odděleně.
