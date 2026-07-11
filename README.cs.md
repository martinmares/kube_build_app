# kube-build-app

`kube-build-app` generuje Kubernetes manifesty z environment repozitáře.

Nástroj má jednoduchý runtime: načte deklarativní metadata prostředí, app modely a assety, a zapíše Kubernetes YAML do cílového adresáře. Ruby implementace je historická reference; Go implementace je produktizované CLI jako single binary, s Cobra commandy a shell completion.

Repozitář teď obsahuje dvě související binárky:

```text
kube-build-app = build/render Kubernetes manifestů
kube-edit-app  = webový editor environment repozitářů
kube-ops-app   = prototyp operations/sync runneru nad vyrenderovanými manifesty
```

`kube-edit-app` je cílový Go přepis Rust aplikace `kube-environments-ui`. Viz `docs/KUBE_EDIT_APP_PLAN.md`.
`kube-ops-app` je raný prototyp operations portálu/sync runneru. Začíná načítáním configu, výpočtem render digestu, prototypovým desired/applied statusem, diffem proti applied snapshotu a CLI inspection příkazy. Viz `docs/KUBE_OPS_APP_PLAN.md`.

## kube-edit-app Workflow

Spuštění webového editoru v bezpečném read-only režimu:

```bash
kube-edit-app serve --root ./environments
```

Explicitní povolení strukturovaných zápisů:

```bash
kube-edit-app serve --root ./environments --allow-write
```

Doporučený postup editace:

1. Vybrat environment.
2. Otevřít `Apps` pro app model nebo `Assets` pro environment JSON/defaults/assets.
3. Ve write režimu provést strukturovanou editaci. Aktuální editory pokrývají local vars, defaults vars/container envs, replicas, autoscaling, resources, Java runtime, probes, ports/services/ingress, container envs a special env JSON entries.
4. Opravit případné chyby podle inline validačních hlášek přímo u polí.
5. Otevřít `Build`, spustit validaci a případně vykreslit build preview file tree.
6. Otevřít `Changed files`, rozbalit inline diffy a vybrat soubory k přijetí.
7. Použít `Accept selected`, které commitne jen vybrané dirty soubory. UI zobrazí výsledný commit hash a commitnuté cesty.

Read-only režim stále zobrazuje strukturované preview, build checks, generated file preview a diffy, ale mutační prvky jsou schované nebo vypnuté a mutační API endpointy vrací `403`. Je to preferovaný režim pro review, L2 kontrolu a dashboardy. `--allow-write` používejte jen pro záměrné editace repozitáře.

### Trusted Proxy Autentizace

`kube-edit-app` lze chránit přes důvěryhodnou reverse proxy, která provede
autentizaci v browseru a předá identitu přes `X-Auth-*` hlavičky:

```bash
kube-edit-app serve \
  --root ./environments \
  --trusted-proxy-auth
```

Podporované jsou i environment proměnné:

```bash
KUBE_EDIT_TRUSTED_PROXY_AUTH=true
KUBE_EDIT_AUTH_HEADER_USER=X-Auth-User
KUBE_EDIT_AUTH_HEADER_EMAIL=X-Auth-Email
KUBE_EDIT_AUTH_HEADER_GROUPS=X-Auth-Groups
KUBE_EDIT_AUTH_GROUP_PREFIX=kube-edit-app
```

Podporované skupiny:

- `kube-edit-app:role:admin`
- `kube-edit-app:env:*:reader`
- `kube-edit-app:env:*:writer`
- `kube-edit-app:env:<env>:reader`
- `kube-edit-app:env:<env>:writer`

`reader` může prohlížet povolené environmenty. `writer` může povolené environmenty
měnit, pokud je server zároveň spuštěný s `--allow-write`. Globální Git commit/restore
operace vyžadují `kube-edit-app:role:admin`.

`env.secured.json` lze editovat přes EncJson flow, pokud je `kube-edit-app` spuštěný s nakonfigurovanými EncJson cestami. Existující encrypted hodnoty se zachovají; nově přidané plaintext hodnoty zůstávají pro zašifrování nástrojem EncJson.

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

### Generator Commandy

Vytvoření základního environmentu:

```bash
kube-build-app skeleton env \
  --root environments \
  --env dev \
  --namespace app-dev \
  --registry-url registry.example.com/project \
  --release-id latest
```

Vytvoří:

```text
environments/dev/env.unsecured.json
environments/dev/apps/_defaults.yml
```

Přidání základního app modelu:

```bash
kube-build-app app add api --root environments --environment dev
```

Generovaný app image defaultně používá obecné placeholdery:

```yaml
image: "{{REGISTRY_URL}}/api:{{RELEASE_ID}}"
```

Image lze podle potřeby přepsat:

```bash
kube-build-app app add worker \
  --root environments \
  --environment dev \
  --image 'custom/worker:1.0.0'
```

Generator commandy nepřepisují existující soubory bez `--force`.

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
    --image              přepíše image ve formátu app/container=image; opakovatelné
    --image-policy       politika image override: fallback nebo strict
-w, --down               nastaví vybraným appkám replicas na 0
-E, --env-file           explicitní .env soubor
    --vars-source        env, json, dot-env; opakovatelné nebo comma-separated
-d, --decrypt-secured    zapne proměnné z env.secured.json
    --helm-escape-assets escapuje zbývající {{VAR}} placeholdery v textových assetech
    --verbose            vypíše build render eventy na stderr
    --log-format         formát verbose build logu: text nebo json
    --color              barvy ve verbose text logu: auto, always nebo never
```

## Vyhodnocení Image

`kube-build-app` vyhodnocuje container image podle dvojice `<app>/<container>`.

Pořadí precedence:

1. `--image app/container=image`
2. `--release-manifest release.yml`
3. `image:` z `<env>/apps/<app>.yml`

Přímý CLI override se hodí pro jednorázové CI/CD joby:

```bash
kube-build-app build -e test \
  --image 'tsm-dms/tsm-dms=registry.example.com/tsm-dms:2.0.0' \
  --image 'tsm-ui/tsm-ui=registry.example.com/tsm-ui@sha256:abcdef'
```

Mezi selektorem a image používáme `=`, ne `:`, protože container image reference sama používá `:` pro tagy a `@sha256:...` pro digesty.

Release manifest je určený pro řízené release pipeline. `kube-build-app` umí přímo použít manifest generovaný nástrojem `simple-release-management`:

```yaml
release_id: 2026.06.25.01
registry_base: registry.example.com/project
images:
  - app_name: tsm-dms
    container_name: tsm-dms
    image: registry.example.com/project/tsm-dms
    tag: 2026.06.25.01
    digest: sha256:abcdef
```

Pokud je vyplněný `digest`, výsledný deployment použije neměnnou digest referenci:

```text
registry.example.com/project/tsm-dms@sha256:abcdef
```

Pokud je vyplněný pouze `tag`, výsledný deployment použije:

```text
registry.example.com/project/tsm-dms:2026.06.25.01
```

Image policy:

```bash
kube-build-app build -e test --release-manifest release.yml --image-policy fallback
kube-build-app build -e test --release-manifest release.yml --image-policy strict
```

`fallback` ponechá image z app YAML, pokud override neexistuje. `strict` vyžaduje, aby každý renderovaný `<app>/<container>` měl image z `--image` nebo `--release-manifest`; to je doporučené pro release pipeline.

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
    envs:
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

#### Autoscaling / HPA

App-level `autoscaling` použijte ve chvíli, kdy má runtime počet replik řídit Kubernetes HPA:

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

Výsledné manifesty obsahují navíc `deployments/<app>-hpa.yml`:

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

Pokud `autoscaling.enabled=true`, runtime počet replik řídí HPA. Hodnota `replicas` zůstává ve výsledném Deployment/StatefulSet jako počáteční požadovaný stav, ale po spuštění objektu ji může HPA měnit.

CPU a memory utilization metriky vyžadují container `resources.requests`. Pro pokročilá HPA pole, například `spec.behavior`, použijte `autoscaling.raw`:

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

Přidejte:

```yaml
containers:
  - name: api
    image: nginx:stable
    envs:
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

#### Java Runtime Options

Pro Java containery preferujte `runtime.java` pro známé JVM memory parametry místo ručního skládání `JAVA_OPTS`:

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

Výsledný deployment obsahuje:

```yaml
env:
  - name: JAVA_OPTS
    value: "-Xms512m -Xmx2048m -XX:+UseG1GC"
```

`export.env_name` má default `JAVA_OPTS`. Pokud je stejná proměnná zároveň ručně uvedená v `vars`, validace skončí chybou. Díky tomu je JVM heap sizing viditelný v `kube-build-app summary` a nevznikají skryté konflikty se startup skripty.

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
      - service_name: api
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

`service_name` je název Kubernetes Service a zároveň krátké DNS jméno v namespace. Legacy `expose_as[].hostname` je dál podporované jako alias kvůli zpětné kompatibilitě.

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

### 15. Workload Identity a Downward API

`workload_identity` použijte ve chvíli, kdy aplikace potřebuje projektovaný Kubernetes/OpenShift ServiceAccount token pro service-to-service autentizaci:

```yaml
workload_identity:
  service_account:
    create: true
    name: order-api      # default: název appky
    automount: false     # default: false, pokud jsou nastavené tokens

  tokens:
    - name: simple-config
      audience: simple-config-server
      mount_path: /var/run/secrets/workload-identity/simple-config # default
      path: token                                                  # default
      expiration_seconds: 3600                                     # default
```

Výsledný deployment obsahuje `serviceAccountName`, `automountServiceAccountToken: false`, projektovaný `serviceAccountToken` volume a read-only mount. Pokud je `service_account.create=true`, `kube-build-app` zároveň vygeneruje `ServiceAccount` manifest.

Pro běžný Downward API mount s runtime informacemi o podu použijte `pod_info`:

```yaml
pod_info:
  enabled: true
  mount_path: /etc/podinfo
```

Vzniknou například soubory:

```text
/etc/podinfo/namespace
/etc/podinfo/pod_name
```

Pro vlastní Downward API mount použijte explicitní tvar:

```yaml
downward_api:
  mounts:
    - name: runtime-info
      mount_path: /etc/runtime-info
      items:
        - path: namespace
          field_path: metadata.namespace
        - path: pod_name
          field_path: metadata.name
```

Pod name je jen runtime/audit metadata. Autorizace má používat normalizovanou workload identitu `namespace/serviceAccount` z projektovaného tokenu.

### 16. Sidecars a Pod Options

App-level `sidecars` použijte pro pomocné containery, které běží ve stejném Podu, ale nejsou primární aplikační containery:

```yaml
workload_identity:
  service_account:
    create: true
  tokens:
    - name: simple-config
      audience: simple-config-server

sidecars:
  - name: simple-config-token-proxy
    image: "{{TSM_REGISTRY_URL}}/simple-config-token-proxy:{{TSM_RELEASE_ID}}"
    startup:
      command:
        - simple-config-token-proxy
      arguments:
        - --listen
        - 127.0.0.1:9999
        - --upstream
        - https://config.example.test/simple-config-server
        - --token-file
        - /var/run/secrets/workload-identity/simple-config/token
    resources:
      cpu:
        from: "10m"
        to: "100m"
      memory:
        from: "32Mi"
        to: "128Mi"

containers:
  - name: api
    image: "{{TSM_REGISTRY_URL}}/api:{{TSM_RELEASE_ID}}"
    envs:
      - name: SPRING_CLOUD_CONFIG_URI
        value: http://127.0.0.1:9999
```

Sidecary se renderují jako běžné Kubernetes containery ve stejném Podu. Síť v Podu sdílí automaticky, takže `127.0.0.1` funguje mezi aplikačním containerem a sidecarem. Workload identity token mounty i Downward API mounty se mountují i do sidecarů.

Pro pomocné containery, které potřebují vidět procesy ostatních containerů ve stejném Podu, zapněte sdílený process namespace:

```yaml
pod:
  share_process_namespace: true

sidecars:
  - name: cgroup-runtime-exporter
    image: "{{TSM_REGISTRY_URL}}/cgroup-runtime-exporter:{{TSM_RELEASE_ID}}"
    envs:
      - name: TARGET_PID
        value: "1"
```

Výsledkem je Kubernetes `shareProcessNamespace: true`. Porty sidecarů se nepoužívají pro generování Service; služby se generují jen z primárních `containers`.

### 17. Init Containers

App-level `init_containers` použijte pro obecné Kubernetes init containery:

```yaml
init_containers:
  - name: migrate
    image: registry.example.com/api-migrate:latest
    command: ["/bin/sh", "-c"]
    arguments: ["./migrate.sh"]
    envs:
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
- `vars`
- `env_from`
- `mounts`
- `security_context`
- `resources`
- `raw`

Existující `tools` zůstávají preferovaná zkratka pro vystavení statických utilit přes generované init containery.

### 18. Raw Escape Hatches

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

### 19. Raw Container Fields

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

### 20. Cgroup Exporter Defaults

Na úrovni containeru lze zapnout automatické vkládání variable pro cgroup exporter:

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

### 21. Ignorované Appky

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

container_envs:
  - name: "*"
    envs:
      - name: GLOBAL_FLAG
        value: "true"

  - name: api
    envs:
      - name: JAVA_OPTS
        value: "-Xms256m"
```

Semantika:

- obecné map klíče se rekurzivně mergují, app hodnoty vítězí
- `vars` se párují podle `name`; app položka plně nahradí default položku
- `container_envs` se aplikují podle `container.name`
- `name: "*"` se aplikuje na všechny containery jako první
- concrete container defaults se aplikují potom
- lokální `containers[].envs` se aplikují poslední
- shodné variables se plně nahrazují podle `name`

Mazání / tombstone:

```yaml
vars:
  - name: LOG_LEVEL
    remove: true
```

```yaml
containers:
  - name: api
    envs:
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
