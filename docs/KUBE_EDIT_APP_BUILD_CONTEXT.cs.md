# kube-edit-app: build context a inspection API

Stav po etape P1, 2026-09-11.

`kube-edit-app serve` pouziva pro Build validate, summary, inventory,
preview, inspection a zjisteni cluster namespace jednu kopii
`buildapp.Options`. Prostredi vybrane v URL meni pouze `Environment`;
ostatni volby jsou v tomto prvnim inkrementu spolecne pro cely server.
Plne nastaveni po prostredich bude resit pozdejsi `--build-config`.

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
