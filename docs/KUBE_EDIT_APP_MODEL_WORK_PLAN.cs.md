# kube-edit-app: pracovní plán podle modelů

Tento dokument slouží jako praktický checklist pro další vývoj `kube-edit-app`.
Cílem je šetřit tokeny a používat dražší model jen tam, kde má jasný přínos.

## Aktuální rozdělané commity

Nejdřív doporučeně oddělit současné změny do dvou commitů:

```bash
git add internal/repository/git.go internal/repository/git_test.go
git commit -m "Fix git status path parsing"

git add internal/webapp/static/ui.js internal/webapp/static/ui.css
git commit -m "Colorize git diff preview"
```

## Jak volit model

### gpt-5.5 low

Používat na levné a jasně ohraničené úkoly:

- UI polish
- drobné CSS/JS změny
- změny textů, badge, rozložení
- malé endpointy bez větší doménové logiky
- opravy typu refresh, výběr položky, zobrazení stavu
- dokumentace a nápověda

Nepoužívat na složitější zápisy do YAML/JSON nebo EncJson flow.

### gpt-5.5 medium

Výchozí model pro běžnou implementaci:

- strukturované editory
- zápisy do YAML/JSON
- bezpečnost zápisu
- dirty state a Git diff/restore
- testy nad repository vrstvou
- editace app metamodelu
- `_defaults.yml`
- build preview

Toto je nejlepší poměr cena/výkon pro většinu další práce.

### gpt-5.3-codex

Použít cíleně pro rizikovější části:

- větší refaktoring backendu
- komplexnější parser/patcher YAML
- obecný write engine s content hash precondition
- EncJson decrypt/edit/encrypt flow
- `env.secured.json`
- `assets.secured.json`
- virtuální filesystem assets
- konfliktní scénáře zápisu

Nepoužívat na běžný UI polish.

## Doporučené pořadí práce

### 1. UI polish po aktuálním stavu

Model: `gpt-5.5 low`

Stav: první low-pass hotový.

Úkoly:

- doladit barevný diff vizuálně - hotovo v prvním low-passu
- lepší empty states - hotovo v prvním low-passu
- jasnější read-only/write mode indikace - hotovo v prvním low-passu
- menší UX opravy v `Changed files`, `Apps`, `Assets` - průběžně

### 2. Git restore

Model: `gpt-5.5 medium`

Stav: první verze hotová.

Úkoly:

- endpoint typu `POST /api/v1/git/restore/{path}` - hotovo
- tlačítko `Discard changes` - hotovo
- potvrzovací dialog - hotovo
- ochrana proti nechtěnému smazání untracked souborů - hotovo
- testy - hotovo

Poznámka: s mazáním untracked souborů zacházet opatrně. Ideálně oddělit restore tracked změn a delete untracked file.

### 3. Bezpečný write foundation

Model: `gpt-5.5 medium`

Stav: základ hotový pro první mutující editor.

Úkoly:

- atomic writes - hotovo pro `UpdateAppVars`
- content hash precondition - hotovo pro `UpdateAppVars`
- strukturované JSON chyby - hotovo pro nové mutující endpointy
- jednotný mechanismus pro všechny editory - základ připraven
- jasné chování při konfliktu - hotovo, API vrací `409 conflict`

Toto je důležité před rozšiřováním dalších mutujících editorů.

### 4. Editor `replicas`

Model: `gpt-5.5 medium`

Stav: první verze hotová.

Úkoly:

- editovat app-level `replicas` - hotovo
- validovat celé číslo >= 0 - hotovo
- po uložení refreshnout app detail, dirty state a diff - hotovo
- testy repository a web endpointu - hotovo

Nízké riziko, dobrý další editor po `Local variables`.

### 5. Editor `resources`

Model: `gpt-5.5 medium`

Stav: první verze hotová.

Úkoly:

- CPU requests/limits - hotovo
- memory requests/limits - hotovo
- zobrazit i legacy `from/to` - hotovo přes normalized overview
- nové změny zapisovat preferovaně jako `requests/limits` - hotovo
- zachovat kompatibilitu se starým zápisem - hotovo
- promítnout do structured overview - hotovo

Kontrakt:

- `requests/limits` mají přednost před `from/to`
- staré YAML soubory musí zůstat validní

### 6. Editor `Container envs`

Model: `gpt-5.5 medium`

Stav: první verze hotová.

Úkoly:

- editovat `containers[].envs` - hotovo
- add/update/delete - hotovo
- per-container zobrazení - hotovo
- po uložení refresh detailu a diffu - hotovo
- testy na více containerů - hotovo
- write endpoint `PATCH /api/v1/envs/{env}/apps/{app_file}/containers/{container_index}/envs` - hotovo
- read-only režim schovává write controls - hotovo

Kontrakt:

- app-level lokální proměnné jsou `vars:`
- container-level lokální proměnné jsou `containers[].envs`
- `_defaults.yml` používá `container_envs`
- staré `env_vars`, `container_env_vars`, `container_vars` a `containers[].vars` už nejsou podporovaný metamodel

### 7. Editor `probes`

Model: `gpt-5.5 medium`

Úkoly:

- preset `spring-actuator`
- ruční HTTP nastavení
- zobrazit legacy `probe` / `health` jako legacy - preview hotovo
- nové změny zapisovat do `probes:`
- testy renderování a zápisu

Kontrakt:

- starý zápis zůstává platný
- nový zápis preferuje `probes:`

### 8. Editor `_defaults.yml`

Model: `gpt-5.5 medium`

Úkoly:

- zobrazit `Defaults` - read-only view hotovo přes Assets
- zobrazit a editovat top-level `vars` - hotovo
- zobrazit a editovat `container_envs` - hotovo
- podporovat `name: "*"` wildcard group - hotovo
- podporovat konkrétní container override group - hotovo
- safe write přes `expected_hash` - hotovo
- diff preview po změně - hotovo
- `remove: true` zůstává read/merge kontrakt app-level override, ne editor field v `_defaults.yml`

Toto je důležité pro praktickou použitelnost v reálných environment repozitářích.

### 9. Editor `env.unsecured.json`

Model: `gpt-5.5 medium`

Úkoly:

- tabulkový editor klíč/hodnota - hotovo pro `env.unsecured.json`
- explicitní typ hodnoty `string` / `number` / `bool` / `null` / `json` - hotovo
- add/update/delete - hotovo
- validace JSON hodnot - hotovo
- diff preview - hotovo
- bezpečný zápis JSON přes `expected_hash` - hotovo

### 10. Editor `env.secured.json`

Model: `gpt-5.3-codex`

Úkoly:

- EncJson preflight
- decrypt/edit/encrypt flow
- práce s externím `encjson-rs`
- chyby klíčů zobrazit srozumitelně
- read-only fallback, pokud není EncJson dostupný
- testovat minimálně API vrstvu mockem/executable fake

### 11. Virtual assets editor

Model: `gpt-5.3-codex`

Úkoly:

- `assets.unsecured.json`
- `assets.secured.json`
- virtual filesystem
- decoded content editor
- binary/text rozlišení
- sync zpět do JSON
- EncJson flow pro secured variantu

Tady nešetřit modelem. Chyby mohou znamenat poškození binárních assetů nebo secured dat.

### 12. Build preview endpoint

Model: `gpt-5.5 medium`

Stav: první read-only verze hotová.

Úkoly:

- render do temp adresáře - hotovo
- vrátit seznam generovaných souborů - hotovo
- vrátit build events/log - events hotovo
- nepřepisovat deploy repo
- UI zobrazí generated files a summary - hotovo

Použitelné pro L2 i CI kontrolu.

### 13. Dokumentace a nápověda

Model: `gpt-5.5 low`

Stav: základní README sekce hotová.

Úkoly:

- README sekce pro `kube-edit-app` - hotovo
- popsat read-only vs `--allow-write` - hotovo
- popsat bezpečný workflow - hotovo:
  - editace
  - diff
  - validate
  - commit
- doplnit screenshot-friendly texty v UI - průběžně

## Praktické pravidlo

- Pokud úkol mění jen UI bez zápisu: `gpt-5.5 low`.
- Pokud úkol zapisuje do repo: `gpt-5.5 medium`.
- Pokud úkol šahá na EncJson, virtuální filesystem nebo obecný YAML patch engine: `gpt-5.3-codex`.

## Produkční minimum

`kube-edit-app` bude prakticky použitelný, až zvládne:

- browse real environment repositories
- dirty state a barevný diff
- safe restore
- editaci `Local variables`
- editaci `replicas`
- editaci `resources`
- editaci `Container envs`
- editaci `_defaults.yml`
- validaci/build preview přes `kube-build-app` internals

Secured env/assets flow může následovat potom, ale pro plnou náhradu Rust UI bude nutný.
