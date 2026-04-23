# Snadné generování Kubernetes manifestů

Jednoduchá abstrakce nad Kubernetes manifesty.

## Typický workflow

Chcete vytvořit deployment pro svůj nový produkt.

```bash
mkdir ~/my-brand-new-product-k8s
cd ~/my-brand-new-product-k8s
```

V adresáři `my-brand-new-product-k8s` vytvořte základní strukturu:

```bash
mkdir environments
mkdir deployments
```

V `environments` si připravte dvě prostředí, například `test` a `production`:

```bash
cd ~/my-brand-new-product-k8s/environments

mkdir -p test/apps
mkdir -p production/apps
mkdir -p test/assets
mkdir -p production/assets
```

V `deployments` si připravte cílové výstupní adresáře:

```bash
cd ~/my-brand-new-product-k8s/deployments

mkdir -p test/deploy
mkdir -p production/deploy
```

Výsledná struktura:

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

Vytvořte ukázkový app soubor `brand-new-product.yml` v `~/my-brand-new-product-k8s/environments/test/apps`:

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
      http:
        path:
          live: /index.html
          ready: /index.html
        port: 8080
```

## Utility binárky přes `tools:` (initContainers + `/app/tools`)

Do appky můžete přidat statické utility binárky z pomocných image.

Příklad:

```yaml
tools:
  - name: util-encjson-rs
    image: your-registry/util-encjson-rs:latest
    expose_bin: /usr/bin/encjson-rs
    as: /app/tools/encjson

  - name: util-apply-env
    image: your-registry/util-apply-env:latest
    expose_bin: /usr/bin/apply-env
```

Chování:

- položky `tools` se generují jako `initContainers`
- každý initContainer zkopíruje `expose_bin` do sdíleného `emptyDir`
- všechny aplikační containery dostanou tento volume read-only namountovaný do `/app/tools`
- pokud `as` chybí, použije se výchozí `/app/tools/<basename(expose_bin)>`
- pokud `as` míří mimo `/app/tools`, použije se stejně jen basename pod `/app/tools`
- `tools` lze definovat globálně v `apps/_defaults.yml`
- app soubor má vyšší prioritu; `tools: null` v appce inherited defaults vypne

## App defaults přes `apps/_defaults.yml`

`apps/_defaults.yml` je jediný env-level defaults soubor pro načítání app modelu.

Není to samostatně buildovaná app. Naopak se načte a mergne do každého `apps/*.yml`.

Použití:

- `tools`
- `vars`
- `container_env_vars`

Příklad:

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

Semantika `vars`:

- páruje se podle `name`
- app-level položka plně nahradí default položku se stejným `name`
- neexistuje field-level merge

Semantika `container_env_vars`:

- validní jen v `apps/_defaults.yml`
- aplikují se na containery podle `container.name`
- `name: "*"` znamená wildcard defaults pro všechny containery
- potom se aplikují defaults pro konkrétní `container.name`
- nakonec se aplikují lokální `containers[].env_vars` z app YAML
- párování je podle `env var name`
- vyšší vrstva vždy plně nahradí nižší vrstvu

Mazání / tombstone:

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

Pravidla pro `remove: true`:

- položka se úplně odstraní z finálního modelu
- nesmí být kombinována s dalšími datovými poli
- nevalidní kombinace failnou hned

Shrnutí precedence:

- obecné hash klíče: rekurzivní merge `_defaults.yml`, app hodnoty vítězí
- `vars`: whole-object replace podle `name`
- `container_env_vars` -> `containers[].env_vars`: whole-object replace podle `name`

## Cgroup Exporter defaults (container-level)

Na úrovni containeru lze zapnout automatické vkládání env var pro cgroup exporter:

```yaml
containers:
  - name: your-app
    enable_cgroup_exporter: true
```

Automaticky se doplní pouze chybějící env var:

- `CGROUP_EXPORTER_METRICS_PREFIX`
- `CGROUP_EXPORTER_METRICS_STATIC_LABELS`
- `CGROUP_EXPORTER_LISTEN`
- `CGROUP_EXPORTER_CPU_REQUESTS_MCPU`
- `CGROUP_EXPORTER_CPU_LIMITS_MCPU`
- `CGROUP_EXPORTER_MEMORY_REQUESTS_MIB`
- `CGROUP_EXPORTER_MEMORY_LIMITS_MIB`
- `CGROUP_EXPORTER_NODE_NAME`

## Rollout checksums (pod template annotations)

Můžete vynutit rollout Kubernetes/ArgoCD při změně vybraných souborů prostředí.

Příklad:

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

Do pod template se zapíší anotace:

```yaml
spec:
  template:
    metadata:
      annotations:
        checksum/config: "..."
        checksum/mtls: "..."
```

Chování:

- cesty jsou relativní vůči `<environment_dir>`
- checksum se počítá z `relative_path + file_content`
- soubory se zpracují ve stabilním seřazeném pořadí
- změna checksum anotace změní pod template a tím vyvolá rollout
- chybějící soubor = fail-fast

Tohle je určené pro deterministické deklarativní vstupy. Nepoužívejte to pro hodnoty, které se injektují až později za běhu mimo `kube_build_app`.

## Ukázkový asset

Vytvořte `nginx.conf` v `~/my-brand-new-product-k8s/environments/test/assets`:

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

Struktura pak vypadá takto:

```bash
cd ~/my-brand-new-product-k8s
tree -f

.
├── ./deployments
│   ├── ./deployments/production
│   │   └── ./deployments/production/deploy
│   └── ./deployments/test
│       └── ./deployments/test/deploy
├── ./environments
│   ├── ./environments/production
│   │   ├── ./environments/production/apps
│   │   └── ./environments/production/assets
│   └── ./environments/test
│       ├── ./environments/test/apps
│       │   └── ./environments/test/apps/brand-new-product.yml
│       ├── ./environments/test/env.secured.json
│       ├── ./environments/test/env.unsecured.json
│       ├── ./environments/test/shared.assets.yml
│       └── ./environments/test/assets
│           └── ./environments/test/assets/nginx.conf

12 directories, 5 files
```

## První build

```bash
kube_build_app -e test -t deployments/test/deploy
```

Příklad výstupu:

```log
Build 0 shared asset/s
Application brand-new-product, with 1 container/s
 => container [1] brand-new-product has 1 asset/s
 => asset [1]: brand-new-product-asset-66535d3
 => container [1] brand-new-product has 1 port/s
 => service [1] brand-new-product, has 1 port/s
   => has external (ingress) brand-new-product
```

Výsledné soubory v `deployments/test/deploy`:

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

## Deployment profily (maintenance / normal mode)

Repliky můžete držet v `apps/*.yml`, ale přebít je jedním profilem:

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

Použití konkrétního profilu:

```bash
kube_build_app -e test -p db-maintenance -t deployments/test/deploy
```

Nebo explicitní soubor:

```bash
kube_build_app -e test -p normal --profiles-file /some/path/replica-profiles.yml
```

Priorita:

1. profil z `-p/--profile`
2. profil z `REPLICA_PROFILE`
3. `defaults.profile` z `replica-profiles.yml`

## Ignorování appky při buildu

V app YAML lze nastavit:

```yaml
ignore: true
```

Pak se appka přeskočí a negenerují se pro ni deployment/service/assets.

`-w/--down` stále funguje a aplikuje se až po profile overrides.

## Validace modelu (fail fast)

Validace bez generování manifestů:

```bash
kube_build_app validate -e test
```

Aktuálně se validuje například:

- pokud `simple_init.enabled: true`, container nesmí mít současně `startup` (XOR konflikt)
- v `simple_init` režimu musí být `simple_init.exec.command` neprázdné pole

## Explicitní env file (`-E`, `--env-file`)

Lze předat už vyřešený `.env` soubor a použít ho jako jediný zdroj proměnných:

```bash
kube_build_app -e test -E /path/to/release.env
```

Typický integrační bod:

```bash
simple-secrets resolve --env test -o dot-env --file /tmp/test.env
kube_build_app -e test -E /tmp/test.env
```

V tomto režimu `kube_build_app` nečte:

- `env.unsecured.json`
- `env.secured.json`
- process `ENV`

Podporovaný `.env` formát:

- prázdné řádky a řádky začínající `#` se ignorují
- volitelný prefix `export ` je povolený
- `KEY=VALUE`
- podporované jsou i quoted values (`"..."` nebo `'...'`)

Poznámky:

- `-E/--env-file` nelze kombinovat s `-d/--decrypt-secured`
- `-E/--env-file` nelze kombinovat s `--vars-source`

## Výběr zdroje proměnných (`--vars-source`)

Můžete explicitně určit, odkud se mají proměnné načíst (repeatable):

```bash
kube_build_app -e test --vars-source env
kube_build_app -e test --vars-source json
```

Podporované hodnoty:

- `env` – proměnné z process `ENV`
- `json` – proměnné z `env.unsecured.json` (+ `env.secured.json`, pokud je zapnuté `-d`)
- `dot-env` – proměnné z konvenčního souboru `<environment_dir>/.env`

Když použijete více zdrojů, aplikují se v pořadí, v jakém je zadáte, a pozdější zdroj přebíjí dřívější.

`--vars-source dot-env` používejte jen tehdy, když chcete implicitní soubor `<environment_dir>/.env`.
Pokud už máte explicitní cestu na vyřešený `.env`, použijte `-E/--env-file`.

Výchozí backward-compatible chování bez `--vars-source`:

- `json`
- `env`

## Helm-safe assets (`--helm-escape-assets`)

Pokud budou vygenerované manifesty znovu renderované Helm-em, může dojít ke kolizi placeholderů s Helm syntaxí.

Použití:

```bash
kube_build_app -e test --helm-escape-assets
```

U textových assetů se zbývající placeholdery přebalí z:

- `{{ VAR }}`

na:

- `{{`{{ VAR }}`}}`

Poznámky:

- binární assety se nemění
- pokud je `transform: true`, nejdřív proběhne normální variable transform a až potom Helm escape
- již zabalené `{{`{{ VAR }}`}}` se znovu neobalují
- per-asset override je možný přes `helm_escape: true|false`

## Inventory mode (`-i`)

Vypíše detailní pretty-formatted app/container inventory JSON a skončí:

```bash
kube_build_app -i -e test
```

Všechny containery jsou zahrnuté. Pro mTLS pipeline filtrujte položky, kde:

```yaml
mtls:
  enabled: true
```

JSON jde na stdout, takže je vhodný pro další piping:

```bash
kube_build_app -i -e test | simple-spiffe-pki generate -
```

Inventory položky obsahují i app-level rollout checksum annotations, pokud jsou definované:

```json
{
  "rollout_checksums": {
    "checksum/config": "..."
  }
}
```

Když má container:

```yaml
mtls:
  enabled: true
```

`kube_build_app` automaticky přimountuje šifrované mTLS soubory:

- `/app/mtls.enc/mtls.secured.json`
- `/app/mtls.enc/mtls.secured.schema.json`

Zdrojové soubory se očekávají v environment repu:

- `<env>/mtls/<app>/<container>.secured.json`
- `<env>/mtls/<app>/<container>.secured.schema.json`

## Environment root (`-R`, `--root-dir`)

Výchozí načítání environments:

- `environments/<env>`
- nebo `ENVIRONMENTS_DIR/<env>`, pokud je nastavené `ENVIRONMENTS_DIR`

Lze explicitně přepsat root directory:

```bash
kube_build_app -e test -R /Users/mares/Development/Src/Ruby/tsm/cetin/tsm-environments
```

## Docker (openSUSE / OpenShift friendly)

Build image:

```bash
docker build -t kube-build-app:latest .
```

Image vždy buildí a embeduje utility binárky:

- `apply-env` z `https://github.com/martinmares/apply-env-rs`
- `encjson-rs` z `https://github.com/martinmares/encjson-rs`
- `simple-policy-engine` se klonuje během build procesu `encjson-rs`
- legacy `encjson` z `https://github.com/martinmares/encjson`

Refy/branche lze připnout build-argy:

```bash
docker build -t kube-build-app:latest \
  --build-arg APPLY_ENV_REF=main \
  --build-arg ENCJSON_REF=main \
  --build-arg SIMPLE_POLICY_ENGINE_REF=main \
  --build-arg ENCJSON_LEGACY_REF=main \
  .
```

`kube_build_app` vybírá encjson binárku podle API markeru v souboru:

- `EncJson[@api=1.0` -> legacy binary (`ENCJSON_LEGACY_PATH`, default `/app/bin/encjson-legacy`)
- `EncJson[@api=2.0` -> rust binary (`ENCJSON_PATH`, default `/app/bin/encjson-rs`)

Run proti lokálnímu environments repo:

```bash
docker run --rm -it \
  -v /absolute/path/to/tsm-environments:/work/environments:ro \
  -v /absolute/path/to/output:/work/output \
  kube-build-app:latest \
  -e test -R /work/environments -t /work/output
```

Container konvence:

- base image: `opensuse/tumbleweed:latest`
- runtime user: `UID=1001`, `GID=1001`, `HOME=/app`
- `/app` je writable a OpenShift-friendly (`chgrp -R 0` + `chmod -R g=u`)

## Testy

Spuštění všech testů:

```bash
ruby -Itest -e 'Dir["test/*_test.rb"].sort.each { |f| require_relative f }'
```

Spuštění jednoho test souboru:

```bash
ruby -Itest test/comprehensive_features_test.rb
```
