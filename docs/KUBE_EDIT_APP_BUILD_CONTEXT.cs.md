# kube-edit-app: build context a inspection API

Stav po etape P6, 2026-09-13.

`kube-edit-app serve` pouziva pro Build validate, summary, inventory,
preview, inspection a zjisteni cluster namespace jeden resolver
`buildapp.Options`. Server podporuje bud jeden spolecny kontext z CLI, nebo
explicitni kontext pro kazde prostredi pres `--build-config`.

## Vstupy serveru

Build kontext lze sestavit z techto voleb:

- `--env-file` nebo `--env-url`, nikdy oboji soucasne;
- `--env-url-header`, `--env-url-insecure`, `--vars-source`;
- `--decrypt-secured` a explicitni EncJson cesty serveru;
- `--release-manifest`, `--image`, `--image-policy`, `--image-reference`;
- `--force-image-tag`, `--force-image-prefix`;
- `--resource-policy-root`, `--namespace`;
- `--profile`, `--profiles-file`, `--down`;
- `--sync-metadata-profile`, `--sync-metadata-prefix`, `--sync-set`;
- `--yaml-indent`, `--legacy-apply-env`, `--helm-escape-assets`.

Neplatne nebo konfliktni volby ukonci server pred otevrenim listen socketu.
Remote env se v ramci jedne builder operace stahuje pouze jednou.

## Kontexty pro vice prostredi

Server nad repository s rozdilnymi vstupy pro jednotliva prostredi lze
spustit takto:

```bash
kube-edit-app serve \
  --root ./environments \
  --build-config ./kube-edit-build.yml
```

```yaml
environments:
  dev:
    namespace: app-dev
    env_url: https://config.example.test/dev/render
    env_url_headers:
      - 'Authorization: Bearer token'
    env_url_insecure: true
    release_manifest: releases/dev.yml
    resource_policy_root: policies
    image_policy: strict
    image_reference: digest
    yaml_indent: 2
  test:
    env_file: inputs/test.env
    vars_sources: []
    decrypt_secured: false
```

Podporovane klice odpovidaji per-build CLI volbam: `namespace`, `env_file`,
`env_url`, `env_url_headers`, `env_url_insecure`, `vars_sources`,
`decrypt_secured`, `release_manifest`, `images`, `image_policy`,
`image_reference`, `force_image_tag`, `force_image_prefix`,
`resource_policy_root`, `replica_profile`, `replica_profiles_file`, `down`,
`sync_metadata_profile`, `sync_metadata_prefix`, `sync_set`, `yaml_indent`,
`legacy_apply_env` a `helm_escape_assets`.

Pravidla jsou zamerne fail-fast:

- config musi obsahovat prave vsechna prostredi nalezena pod `--root`;
- nezname YAML klice a dalsi YAML dokument jsou chyba;
- relativni `env_file`, `release_manifest`, `resource_policy_root` a
  `replica_profiles_file` se vyhodnocuji vuci adresari config souboru;
- `--build-config` nelze kombinovat s per-build CLI flagy, aby nevznikla
  skryta precedence; globalni server/auth/EncJson/kubeconfig volby zustavaji;
- kazdy kontext se validuje pred otevrenim listen socketu.

Hodnoty hlavicek ani remote env promenne se klientovi neposilaji. Config
soubor proto chranit jako serverovou konfiguraci a nevkladat ho do UI.

## Reverse proxy base path

`--base-path /editor` namountuje HTML, staticke soubory i API pod
`/editor/`; pozadavek na `/editor` je presmerovan na kanonickou cestu.
Frontend vsechny API adresy sklada z tohoto serverem predaneho prefixu.

## HTTP API

- `GET /api/v1/envs/{env}/build-context` vraci bezpecny popis konfigurace.
  Nevraci hodnoty HTTP hlavicek, URL, nactene promenne, image overrides ani
  hodnoty force tag/prefix. U citlivych voleb vraci jen boolean nebo pocet.
- `GET /api/v1/envs/{env}/inspect` vraci source a effective model. Source
  obsahuje puvodni YAML, hash, template-aware model a metadata pritomnych
  YAML cest. Zdroje zustanou dostupne i pri chybe remote env nebo composition.
- Effective container bloky a env polozky obsahuji serazene `origins`,
  `write_target` vazany na source hash a `capabilities`. Efektivni sidecar
  zatim nema write target, protoze jeho index neni bezpecny source selector.

## Priorita a puvod hodnot

Inspection pouziva primo builder pipeline. Pro hlavni container je poradi
vrstev katalog defaults, `profile_ref_names` v deklarovanem poradi a lokalni
app container. `container_envs` doplnuji env polozky podle builder kontraktu.
Externi resource policy je autoritativni pro resources. Release manifest a
explicitni image options zpracovava stejny image resolver jako Build.

`origins` popisuji znamy smer od sdilenych vrstev k lokalni deklaraci;
nejsou odvozovany porovnavanim shodnych hodnot. Detailni bezpecny selector
pro lokalni sidecar patch a editaci sdilenych definic je zamerne odlozen do
P3/P4.
