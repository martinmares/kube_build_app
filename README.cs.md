# kube-build-app

`kube-build-app` generuje Kubernetes manifesty z environment repozitáře.

Nástroj má jednoduchý runtime: načte deklarativní metadata prostředí, app modely a assety, a zapíše Kubernetes YAML do cílového adresáře. Ruby implementace je historická reference; Go implementace je produktizované CLI jako single binary, s Cobra commandy a shell completion.

Repozitář teď obsahuje dvě související binárky:

```text
kube-build-app = build/render Kubernetes manifestů
kube-edit-app  = webový editor environment repozitářů
```

`kube-edit-app` je cílový Go přepis Rust aplikace `kube-environments-ui`. Viz `docs/KUBE_EDIT_APP_PLAN.md`.

## Cíl Metamodelu

`kube-build-app` není Helm. App model záměrně používá pravidlo 80/20:

```text
80 % běžných deployment potřeb = jasná first-class pole metamodelu
20 % speciálních Kubernetes případů = explicitní raw escape hatch
```

Preferujte dedikovaná modelová pole, pokud existují, protože se dají validovat, sumarizovat a editovat přes `kube-edit-app`. `raw:` používejte jen pro Kubernetes pole, která jsou příliš speciální nebo vzácná pro běžný model.

Dlouhodobý plán metamodelu je v `docs/KUBE_BUILD_APP_METAMODEL_PLAN.md`.

## Rychlý Start

Minimální struktura repozitáře:

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

Vygenerování manifestů:

```bash
kube-build-app build -e test -R environments -t deploy/test
```

Výstup:

```text
deploy/test/
  deployments/api-deployment.yml
  services/
  assets/
```

Pokud chybí `-t/--target`, výchozí výstupní adresář je:

```text
<root>/<environment>/target
```

## CLI

Preferované moderní commandy:

```bash
kube-build-app build -e test -R environments -t deploy/test
kube-build-app validate -e test -R environments
kube-build-app summary -e test -R environments
kube-build-app summary --summary-format json -e test -R environments
kube-build-app inventory -e test -R environments
kube-build-app list -e test -R environments
kube-build-app completion zsh
```

Legacy root flagy zůstávají kvůli kompatibilitě:

```bash
kube-build-app -e test -R environments -t deploy/test
kube-build-app -s -e test -R environments
kube-build-app -i -e test -R environments
kube-build-app -l -e test -R environments
```

Nemíchejte action subcommandy s legacy action flagy. Tohle je záměrně nevalidní:

```bash
kube-build-app build -e test -s
```

Použijte:

```bash
kube-build-app summary -e test
```

Verbose build logování:

```bash
kube-build-app build -e test -R environments -t deploy/test --verbose
kube-build-app build -e test -R environments -t deploy/test --verbose --log-format json
kube-build-app build -e test -R environments -t deploy/test --verbose --color always
```

Build logy se zapisují na `stderr`. Normální výstup příkazu zůstává na `stdout`.

Užitečné flagy:

```text
-e, --environment        název prostředí
-R, --root               root adresář environments, výchozí: environments
-t, --target             výstupní adresář
-p, --profile            název replica profilu
    --profiles-file      cesta k replica profiles souboru
-r, --release-manifest   cesta k release manifest YAML
-w, --down               nastaví vybraným appkám replicas na 0
-E, --env-file           explicitní .env soubor
    --vars-source        env, json, dot-env; opakovatelné nebo comma-separated
-d, --decrypt-secured    zapne proměnné z env.secured.json
    --helm-escape-assets escapuje zbývající {{VAR}} placeholdery v textových assetech
    --verbose            vypíše build render eventy na stderr
    --log-format         formát verbose build logu: text nebo json
    --color              barvy ve verbose text logu: auto, always nebo never
```

## Kontrakt Placeholderů

Existují tři formy placeholderů. Mají odlišný scope a odlišný čas vyhodnocení.

| Placeholder | Kdo řeší | Kdy | Význam |
| --- | --- | --- | --- |
| `{{VAR}}` | `apply-env`, CI/CD, deploy fáze, Helm-safe post-processing | později | Runtime/deploy placeholder. `kube-build-app` ho v app YAML nechá beze změny. |
| `{{env:VAR}}` | `kube-build-app` | build time | Build-time environment proměnná načtená z env JSON, `.env` nebo process env podle nastavených zdrojů. |
| `{{var:VAR}}` | `kube-build-app` | build time | App-local proměnná z bloku `vars:` v aktuálním `<app>.yml`, po mergi `_defaults.yml`. |

Pravidlo:

```text
prefixovaný placeholder = vyřešit při kube-build-app
holý placeholder        = nechat na pozdější deploy/render fázi
```

Příklad:

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

Ve výsledném deploymentu zůstane jen holý runtime placeholder:

```yaml
containers:
  - name: api
    image: registry.example.com/demo/api:2026.05.13.1
    env:
      - name: RUNTIME_VALUE
        value: "{{RUNTIME_VALUE}}"
```

## Zdroje Proměnných

Výchozí chování je zpětně kompatibilní:

```text
env.unsecured.json + env.secured.json při -d, potom process environment
```

Explicitní `.env` soubor:

```bash
kube-build-app build -e test -E /path/to/release.env
```

Tohle je doporučený produkční workflow pro secured proměnné. Čím se hodnoty dešifrují je mimo `kube-build-app`; nástroj dostane už vyřešené key/value páry.

V tomto režimu je explicitní `.env` jediný zdroj proměnných. Nelze ho kombinovat s `-d` ani s `--vars-source`.

Zpětně kompatibilní decrypt secured JSON:

```bash
kube-build-app build -e test -d
```

Při použití `-d/--decrypt-secured` se `env.secured.json` dešifruje spuštěním externí EncJson binárky:

```text
encjson decrypt -k <keydir> -f env.secured.json
encjson-rs decrypt -k <keydir> -f env.secured.json
```

Výběr binárky:

```text
EncJson[@api=1.0  -> ENCJSON_LEGACY_PATH, ENCJSON_LEGACY_BIN, ENCJSON_BIN, fallback encjson
EncJson[@api=2.0  -> ENCJSON_PATH, ENCJSON_RS_BIN, fallback encjson-rs
neznámý marker    -> ENCJSON_BIN nebo legacy fallback
```

Adresář s klíči:

```text
pokud je ENCJSON_KEYDIR nastavené: decrypt -k "$ENCJSON_KEYDIR" -f env.secured.json
jinak:                              decrypt -f env.secured.json
```

Když `ENCJSON_KEYDIR` není nastavené, výběr keydir se nechává na samotné EncJson utilitě.

Explicitní výběr zdrojů:

```bash
kube-build-app build -e test --vars-source json
kube-build-app build -e test --vars-source env
kube-build-app build -e test --vars-source json --vars-source env
kube-build-app build -e test --vars-source json,env
```

Podporované zdroje:

```text
json     env.unsecured.json a env.secured.json při -d
env      process environment
dot-env  <environment_dir>/.env, pokud není použité -E/--env-file
```

## App Model Skládáním

App model je jeden YAML soubor v:

```text
<environment>/apps/<app>.yml
```

Soubory začínající `_` jsou speciální soubory, ne appky. Například:

```text
<environment>/apps/_defaults.yml
```

### 1. Minimální App

```yaml
name: api
containers:
  - name: api
    image: nginx:stable
```

Výsledný deployment obsahuje jeden container:

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

### 2. Repliky

Přidejte:

```yaml
replicas: 3
```

Výsledný deployment obsahuje:

```yaml
spec:
  replicas: 3
```

### 3. Resources

Přidejte container resources s Kubernetes slovníkem `requests` / `limits`:

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

Legacy `from` / `to` zůstává podporované:

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

Výsledný deployment obsahuje:

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

Summary command tyto hodnoty používá a celkové součty násobí počtem replik:

```bash
kube-build-app summary -e test -R environments
```

### 4. Container Environment Variables

Přidejte:

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

Výsledný deployment obsahuje:

```yaml
env:
  - name: JAVA_OPTS
    value: "-Xms256m -Xmx512m"
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
```

### 5. Porty a Services

Přidejte container port:

```yaml
containers:
  - name: api
    image: nginx:stable
    ports:
      - name: http
        port: 8080
```

Deployment dostane `containerPort: 8080`.

Service vznikne přes `expose_as`:

```yaml
ports:
  - name: http
    port: 8080
    expose_as:
      - hostname: api
        port: 80
```

Výsledný service:

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

Externí vystavení se přidává pod `expose_as[].external` a podle modelu generuje Ingress nebo OpenShift Route.

### 6. Probes

Preferovaný moderní zápis:

```yaml
containers:
  - name: api
    image: api:latest
    probes:
      preset: spring-actuator
      port: 8080
```

Vygeneruje:

```text
livenessProbe  -> /actuator/health/liveness
readinessProbe -> /actuator/health/readiness
startupProbe   -> /actuator/health
```

Obecný HTTP zápis:

```yaml
probes:
  preset: http
  port: 8080
  path: /healthz
```

Zápis s override:

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

Legacy bloky `health` a `probe` zůstávají podporované kvůli zpětné kompatibilitě:

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

`health` generuje jen liveness/readiness. `probe` umí liveness/readiness/startup, ale je ukecanější. Nové soubory by měly preferovat `probes`.

### 7. Assets

Soubor vložte do:

```text
<environment>/assets/nginx.conf
```

A referencujte ho z appky:

```yaml
containers:
  - name: api
    image: nginx:stable
    assets:
      - file: assets/nginx.conf
        to: /etc/nginx/conf.d/default.conf
```

Výstup obsahuje:

```text
assets/api-asset-<crc>.yml
```

Deployment dostane volume mount:

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

Digest v názvu assetu vychází z reálného asset vstupu a cílové mount cesty. ConfigMap jméno je stabilní a mění se jen při změně mountovaného obsahu.

Textové assety mohou použít build-time transform:

```yaml
assets:
  - file: assets/application.yml.tpl
    to: /app/config/application.yml
    transform: true
```

Legacy `assets` podporuje i volume-only formy jako `temp`, `pvc`, `nfs-server` a `host-path`. Kvůli zpětné kompatibilitě zůstávají podporované, ale nové soubory by měly pro volume deklarace preferovat `mounts`.

### 8. Mounts

`mounts` používejte pro non-ConfigMap volumes a pro čistší moderní zápis file mountů:

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

`host_path` je high-risk escape hatch, protože containeru vystavuje filesystem path přímo z nodu.

Pro vzácné Kubernetes volume typy použijte raw mount passthrough:

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

`<environment>/shared.assets.yml` slouží pro assety mountované do více appek.

Appka je může vypnout:

```yaml
disable_shared_assets: true
```

### 10. Tools

Statické utility binárky lze vystavit přes initContainers a mount do `/app/tools`:

```yaml
tools:
  - name: util-apply-env
    image: registry.example.com/tools/apply-env:latest
    expose_bin: /usr/bin/apply-env
    as: /app/tools/apply-env
```

Chování:

- každý tool se generuje jako initContainer
- initContainer zkopíruje `expose_bin` do sdíleného `emptyDir`
- app containery mountují volume read-only pod `/app/tools`
- pokud `as` chybí, cíl je `/app/tools/<basename(expose_bin)>`

### 11. Scheduling a Pod Metadata

Časté app-level fields:

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

Renderují se do deployment metadata a pod template.

Preferovaný moderní zápis pro scheduling:

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

Pro vzácné Kubernetes scheduling případy použijte raw affinity pod `scheduling.affinity`:

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

Legacy `arch`, `node_selector` a `tolerations` zůstávají podporované. Nové soubory by měly preferovat `scheduling`.

### 12. Security Context

Pro běžná pod a container security nastavení použijte `security_context`.

App-level `security_context` se renderuje do pod spec `securityContext`:

```yaml
security_context:
  runAsNonRoot: true
  fsGroup: 2000
```

Container-level `security_context` se renderuje do container `securityContext`:

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

Preferujte toto dedikované pole před `raw.securityContext`. Pokud se použije obojí, `raw` zůstává kompatibilní výjimka a může vygenerované pole přepsat.

### 13. Lifecycle a Termination Grace

App-level `termination_grace_period` se renderuje do pod spec `terminationGracePeriodSeconds`:

```yaml
termination_grace_period: 45
```

Container-level `lifecycle.pre_stop` použijte pro graceful shutdown hooky:

```yaml
containers:
  - name: api
    image: nginx:stable
    lifecycle:
      pre_stop:
        command: ["/bin/sh", "-c", "sleep 10"]
```

Výsledkem je Kubernetes `lifecycle.preStop.exec.command`.

Pro vzácné lifecycle handler formy použijte explicitní raw passthrough:

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

### 14. Service Account a Env From

App-level `service_account` se renderuje do pod spec `serviceAccountName`:

```yaml
service_account: tsm-api
```

Container-level `env_from` použijte pro běžné importy environment proměnných z ConfigMap/Secret:

```yaml
containers:
  - name: api
    image: api:latest
    env_from:
      - config_map: api-config
      - secret: api-secret
        prefix: SECRET_
```

Pro méně běžné Kubernetes volby použijte nativní tvar reference:

```yaml
env_from:
  - configMapRef:
      name: api-config
      optional: true
  - secretRef:
      name: api-secret
      optional: true
```

Výsledkem je Kubernetes `envFrom`.

### 15. Init Containers

App-level `init_containers` použijte pro obecné Kubernetes init containery:

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

Podporovaná pole init containeru záměrně kopírují běžnou podmnožinu standardních containerů:

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

Existující `tools` zůstávají preferovaná zkratka pro vystavení statických utilit přes generované init containery.

### 16. Raw Escape Hatches

`deployment_raw` použijte pro vzácná Deployment-level pole:

```yaml
deployment_raw:
  spec:
    revisionHistoryLimit: 2
```

`pod_raw` použijte pro vzácná pod spec pole:

```yaml
pod_raw:
  dnsPolicy: ClusterFirst
  enableServiceLinks: false
```

`deployment_raw` se rekurzivně merguje do vygenerovaného Deployment objektu. `pod_raw` se aplikuje do `spec.template.spec`.

### 17. Raw Container Fields

Raw passthrough používejte jen tehdy, když model nemá dedikované pole:

```yaml
containers:
  - name: api
    image: nginx:stable
    raw:
      securityContext:
        runAsNonRoot: true
```

Dedikovaná modelová pole jsou lepší, protože se dají validovat a zobrazit v UI nástrojích.

### 18. Cgroup Exporter Defaults

Na úrovni containeru lze zapnout automatické vkládání env var pro cgroup exporter:

```yaml
containers:
  - name: api
    image: api:latest
    enable_cgroup_exporter: true
```

Vkládají se jen chybějící proměnné:

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

### 19. Ignorované Appky

Vynechání appky z buildu:

```yaml
ignore: true
name: experimental-api
```

Pro ignorované appky se negeneruje deployment, service ani assety.

## App Defaults

`apps/_defaults.yml` se merguje do každého app modelu v daném prostředí.

Příklad:

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

Semantika:

- obecné map klíče se rekurzivně mergují, app hodnoty vítězí
- `vars` se párují podle `name`; app položka plně nahradí default položku
- `container_env_vars` se aplikují podle `container.name`
- `name: "*"` se aplikuje na všechny containery jako první
- concrete container defaults se aplikují potom
- lokální `containers[].env_vars` se aplikují poslední
- shodné env vars se plně nahrazují podle `name`

Mazání / tombstone:

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

`remove: true` položku úplně odstraní a nesmí být kombinováno s dalšími datovými poli.

## Replica Profily

Env-level profily umožní změnit repliky bez editace app souborů:

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

Spuštění:

```bash
kube-build-app build -e test -p maintenance -R environments -t deploy/test
```

Přímé stažení vybraných appek na nulu:

```bash
kube-build-app build -e test -w worker -R environments -t deploy/test
```

## Rollout Checksums

Rollout checksum anotace použijte, když se má pod restartovat po změně vybraných souborů.

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

Výsledný deployment pod template obsahuje:

```yaml
spec:
  template:
    metadata:
      annotations:
        checksum/config: "..."
        checksum/mtls: "..."
```

Chování:

- cesty jsou relativní k `<environment_dir>`
- soubory se před hashováním seřadí
- checksum zahrnuje relativní cestu a obsah souboru
- chybějící soubor failne build
- změna anotace změní pod template a vyvolá rollout

Používejte to jen pro deterministické deklarativní soubory. Ne pro hodnoty injektované později sidecarem nebo čistě runtime mechanismem.

## Inventory

Inventory vypíše strukturovaný JSON pohled na to, co se bude buildit:

```bash
kube-build-app inventory -e test -R environments
```

Legacy ekvivalent:

```bash
kube-build-app -i -e test -R environments
```

Je určené pro automatizaci, UI a generátory typu PKI helperů.

## Validace

Validace modelu bez zápisu manifestů:

```bash
kube-build-app validate -e test -R environments
```

Validace chytá známé nevalidní kombinace, například konflikt startup/simple-init nebo nevalidní tombstones.

## Helm-Safe Assets

Pokud se vygenerované manifesty renderují ještě Helmem, zbývající holé placeholdery v textových assetech mohou kolidovat s Helm syntaxí.

Použijte:

```bash
kube-build-app build -e test -R environments -t deploy/test --helm-escape-assets
```

Zbývající placeholdery se přepíšou z:

```text
{{VAR}}
```

na:

```text
{{`{{VAR}}`}}
```

Pokud má asset `transform: true`, nejdřív se aplikují build-time proměnné a až potom Helm escaping zbývajících placeholderů.

## mTLS Assets

Při container-level `mtls.enabled: true` `kube-build-app` automaticky mountuje šifrované mTLS soubory do containeru.

Inventory obsahuje očekávané cesty, aby externí tooling mohl vytvořit odpovídající secured materiál.

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

## Vývoj

Testy:

```bash
just test
```

Lokální binárky:

```bash
just build
```

Cross-platform build:

```bash
just build-cross
just build-cross-all
```

Release binárky s version metadata a SHA256 checksumy:

```bash
just release 0.1.0
```

README smoke fixture:

```bash
just readme-smoke
```

Fixture je v `fixtures/readme-smoke/environments/dev` a ověřuje dokumentovaný CLI flow nad malým environment repozitářem:

```bash
kube-build-app validate -e dev -R fixtures/readme-smoke/environments
kube-build-app list -e dev -R fixtures/readme-smoke/environments
kube-build-app summary -e dev -R fixtures/readme-smoke/environments
kube-build-app inventory -e dev -R fixtures/readme-smoke/environments
kube-build-app build -e dev -R fixtures/readme-smoke/environments -t /tmp/kube-build-app-readme-smoke
```

Parity proti Ruby referenci:

```bash
scripts/parity-build \
  --name cetin-test \
  --root /path/to/tsm-environments \
  --env test \
  --release-id 2025.08.18.1 \
  --go-bin ./dist/kube-build-app
```

Reprezentativní smoke parity cases:

```bash
just parity-smoke
```

## GitLab Release Pipeline

GitLab pipeline používá soubor `VERSION` jako release trigger.

Při změně `VERSION` na default branch se spustí:

- vygenerování release notes z git historie
- cross-platform build `kube-build-app`
- vytvoření `tar.gz` balíčků
- vygenerování `SHA256SUMS`
- vytvoření nebo aktualizace GitLab Release
- upload do GitLab Generic Package Registry

Názvy release balíčků:

```text
kube-build-app-<version>-darwin-arm64.tar.gz
kube-build-app-<version>-darwin-amd64.tar.gz
kube-build-app-<version>-linux-amd64.tar.gz
kube-build-app-<version>-linux-arm64.tar.gz
kube-build-app-<version>-windows-amd64.tar.gz
SHA256SUMS
```

Každý platformní balíček obsahuje obě binárky:

```text
kube-build-app
kube-edit-app
```

Volitelný push `CHANGELOG.md`:

```text
RELEASE_PUSH_TOKEN
```

Pokud je `RELEASE_PUSH_TOKEN` nastavený v GitLab CI/CD variables, publish job aktualizuje `CHANGELOG.md` a pushne změnu zpět s `[skip ci]`.
