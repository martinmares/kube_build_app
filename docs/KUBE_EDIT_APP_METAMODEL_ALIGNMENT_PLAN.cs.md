# kube-edit-app: sladeni s aktualnim metamodellem builderu

Datum: 2026-09-11. Vychozi commit: `162e6e2`, VERSION `0.13.1`.
Stav: etapy P0-P2 implementovany; P3-P6 cekaji.
Autor zadani chce implementaci predat dalsi session/modelu.
Tento dokument nahrazuje stare poradi praci v
`KUBE_EDIT_APP_MODEL_WORK_PLAN.cs.md` pro tento konkretni ukol.

## 1. Cil a hranice

`kube-edit-app` musi umet srozumitelne prohlizet a bezpecne upravovat
komponovany model, ktery dnes zpracovava `kube-build-app`.
Nestaci pridat inputy pro nove YAML klice. Je treba rozlisit:

- zdrojovou deklaraci v app souboru;
- sdilene definice/defaulty a reference na ne;
- efektivni nastaveni po slozeni a po aplikaci build vstupu;
- vygenerovane Kubernetes objekty v existujicim Build preview.

Zachovat konzervativni Tabler UI, existujici navigaci, modalni editory,
Changed files, Git review, trusted proxy auth a read-only default.
Aktivni vyvoj je pouze Go. Ruby ani `kube-ops-app` nerozvijet.
Nevytvaret deployment/sync engine ani release management.
Nemenit metamodel/merge kontrakt builderu kvuli pohodli editoru.

Referencni externi repo, pri analyze ciste:
`/Users/mares/Development/CETIN/sandbox-zis-tsm/tsm-sda-environments`.
Prostredi `dev`, zejmena `apps/_defaults.yml`, `tsm-address-management.yml`,
`tsm-ui.yml`, `tsm-connector-worker.yml`, `tsm-user-management.yml`,
`shared.assets.yml`. Pri implementaci vytvorit anonymizovanou fixture;
neprepisovat tyto skutecne soubory a nepouzivat skutecne tokeny/keystory.

## 2. Co bylo overeno v aktualnim kodu

Analyza byla staticka, bez spusteni serveru, buildu referencniho prostredi
nebo kontaktovani vaultu/clusteru. Nize jsou zjisteni z kodu, nikoliv report
z browser testu. Vychozi pracovni strom projektu byl cisty.

| Oblast | Builder dnes | Editor dnes a dusledek |
| --- | --- | --- |
| Slozeni app | `loadApp`, `mergeDefaults`, sidecar refs, container profiles | `repository.AppModel` cte pouze app pres `renderVarsPreview`; zdedene porty/probes/startup/envs a sidecary v prehledu chybi |
| `_defaults.yml` | Bohaty model vcetne katalogu | `DefaultsModel` obsahuje jen vars/container_envs; `defaultsModelFromDetail` parsuje primo YAML a nezvlada spolehlive nequotovane template vyrazy |
| Container env defaults | `container_ref_name`, wildcard jen pro hlavni containers | Reader i writer stale pouzivaji `name`; writer vyrobi builderem explicitne odmitany klic |
| Envs reference | `workload_identity_token_ref_name`, `shared_asset_ref_name`, secret/resource/field formy, remove | `envVarModel` rozpoznava hlavne value/valueFrom; specialni metamodel reference vypadaji jako prazdna editovatelna hodnota |
| Probes | `http`, live/ready/start detaily, preset i legacy | DTO/formular jen preset/port/path; `replaceContainerProbesBlock` nahradi cely blok, muze zahodit timing a dalsi pole |
| Resources | Legacy from/to i requests/limits, take ephemeral-storage, externi policy | Formular a block writer jen CPU/memory, cely resources blok se nahrazuje |
| Ports/Route | Multi-host, tls, annotations, labels | DTO zobrazi jen prvni host; guard odmita tls/annotations/labels. Route z referencniho UI nelze bezne editovat |
| Build vstupy | EnvFile/URL, release, namespace, policy, profily, sync metadata aj. | `Server.buildOptions` predava pouze Environment/Root. UI preview se nemusi shodovat s CLI |
| Cluster namespace | `loadBuildVars`: explicitni Namespace ma prednost | `environmentNamespace` cte pouze env.unsecured.json |
| Identita zapisovaneho objektu | Containers + selected sidecars + local patches | Endpointy/patchery maji jen containers index; index efektivniho sidecaru neni index ve zdrojovem YAML |
| YAML zapis | Templates v numerickych i textovych polich | Radkove patchery predpokladaji konkretni odsazeni; prosty yaml.Unmarshal nad template YAML nestaci |
| Assets navigace | shared.assets.yml, dalsi build vstupy | `Repository.Assets` specialne zpristupnuje jen 4 JSON soubory a defaults; shared.assets.yml chybi |

Zastarale testy `TestDefaultsReadsAndUpdatesVarsAndContainerEnvs` aktualne
dokonce vyzaduji `container_envs[].name`. Je nutny test editor -> builder,
nejen test proti vlastnimu DTO nebo ocekavanemu textu.

## 3. Nemenny domenovy kontrakt

1. `vars` jsou sablonove promenne. `envs` jsou prostredi kontejneru.
   `{{var:NAME}}`, `{{env:NAME}}`, bare `{{NAME}}` jsou odlisne veci.
   Bare placeholder zustava passthrough; explicitni `LegacyApplyEnv` je
   samostatny postprocessing builderu s omezenym rozsahem vystupnich cest.
2. Sdilene sidecary se vybiraji pres app `sidecar_ref_names`. Lokalni
   `sidecars` stejneho jmena patchuji vybranou definici. Shodne jmeno bez
   reference je chyba; nezname jmeno muze byt plna app-only sidecara.
3. `container_profiles[].defaults` se aplikuji v poradi `profile_ref_names`,
   pak lokalni nastaveni. Profily plati pro containers i sidecars.
   Neni to totez jako replikacni `--profile` ani scaffold profil.
4. `mergeMappingNodes` rekurzivne slucuje mapy, ostatni hodnoty vcetne
   sekvenci nahrazuje. Vars/envs maji zvlastni name-based merge a remove.
   Nevymyslet obecne slucovani vsech seznamu podle name.
5. `container_envs` z defaults plati pouze pro hlavni containers:
   wildcard, jmenovana skupina, nasledne container envs po profile merge.
6. App `runtime_asset_ref_names` plati pro vsechny hlavni containers,
   nikdy automaticky pro sidecars nebo init containers. Ref u containeru/
   sidecaru prida asset pouze jemu. App-level a konkretni refs se kombinuji.
7. `runtime_asset_defaults` poskytuje cele `source`, `volume`, `fetcher`
   bloky. Definice s vlastnim blokem nahrazuje tento blok, ne jedno pole.
   Presne chovani prazdnych/zero-value bloku urcuji existujici
   `runtimeAsset*Configured` funkce; nezavadet odlisnou semantiku v UI.
8. Assety mohou sdilet shodne nakonfigurovany volume; konflikty volume
   parametru nebo cilovych souboru jsou chyby. Poradi fetch init containeru
   vychazi z poradi definic, nikoliv z klikani na refs.
9. Token/shared asset reference jsou jmena, ne URL, cesta nebo audience.
   Builder rozvaze `workload_identity.tokens` a `shared.assets.yml`.
10. Externi resource policy nahrazuje resources kompletne pro hlavni
    containers a sidecars; nepokriva automaticky init/tools/fetchers.
    Povinna CPU/memory from/to, nepovinna ephemeral-storage. Neprovadet
    tichy fallback na inline resources pri chybejici policy.
11. Release image marker a strict/fallback pravidla delegovat builderu.
    `RELEASE_ID` konflikt mezi release manifestem a promennymi je dnes
    chyba, nikoliv automaticka preference `.env`.
12. `service_name` je preferovane jmeno Service, hostname zustava alias.
    Route `tls.termination: edge` a ingress HTTPS nejsou totozny formular.

Dulezite nuance uz existuji v builderu. Regression testy musi zafixovat
soucasne chovani i pri extract/refaktoringu resolveru. Kdyz se najde chyba
builderu, zaznamenat ji oddelene; neopravovat ji skryte v JS.

## 4. Navrh architektury

### 4.1 Zdroj, efektivni model a provenance

Zavest explicitni source DTO a effective DTO. Dnesni `AppModel` neprepnout
na efektivni hodnoty, dokud formulare zapisuji jeho obsah zpatky do appky.

Zdrojovy dokument musi jit otevrit i bez vaultu, release manifestu nebo
kompletniho build kontextu. Uchovat puvodni template vyrazy a YAML typy.
Nequotovane `{{env:PORT}}` neni rozresene cislo a neni vadny input jen proto,
ze klientsky formular pouziva number input. V editaci pouzit text + validaci
literal/template; unresolved stav zobrazit a konkretni cislo overit pri buildu.

Rozsirit builder o uzke read-only inspection API (navrh jmena `Inspect`),
ktere vyuzije jeho existujici load/merge/resolve/validate funkce a vrati
exportovane DTO. Repository a JS nesmi implementovat druhy resolver.
Neni nutne exportovat vsechny interni builder structy nebo hned stehovat
cely soubor `build.go`. Extract pouze sdilenou pripravu, kterou potrebuje UI.
Zohlednit, ze dnes Build/Validate/Inventory/loadPreparedApps cast pipeline
duplikuji a Inventory nepouziva uplne stejny replica-profile flow.

Provenance musi vznikat pri skladani modelu, ne porovnavanim stejnych hodnot.
Minimalni zaznam pro editovatelne pole/blok:

```text
source: document path + YAML path + raw value + present/absent
effective: resolved value (nebo unresolved/error)
origins: ordered {kind, document, yaml_path, definition_name}
write_target: source document + selector (nikdy effective index)
capabilities: can_override, can_reset, can_edit_source, reason
```

Rozlisit raw spelling jmena (`{{var:APP_NAME}}`) a resolved jmeno. Selector
muze byt serverem vytvoreny descriptor obsahujici scope, source index a name
assertion, ktery je platny pouze s hashem dokumentu. Nepovolit klientovi
libovolnou filesystem cestu/obecny YAML path jako neomezeny write endpoint.
Scope minimalne app container, local sidecar/selected sidecar patch,
container profile defaults a sidecar definition.

Pri chybe efektivniho vyhodnoceni zachovat zdrojovy pohled; neukazovat
chybne "no probes/no sidecars". Vystupni diagnostika ma dokument, YAML path,
pokud mozno puvodni radek, a rozliseni chyby modelu/chybejiciho vstupu.
Jedna spatna app nesmi znemoznit otevrit zdroj ostatnich app.

### 4.2 Build context

Zavest jeden serverovy resolver build kontextu pro vybrane prostredi.
Minimalni nastaveni: env-file NEBO env-url + headers/insecure, vars sources,
decrypt mode, release manifest/image policy/reference/overrides,
force tag/prefix, resource-policy-root, namespace, replica profile/profiles
file/down, sync profile/prefix/set, YAML indent, legacy-apply-env,
helm-escape-assets. Kontrolovat proti `buildapp.Options` a CLI flagum.

Prvni inkrement: shodne relevantni flagy na `kube-edit-app serve`, jeden
kontext pro server, explicitne popsat rozsah platnosti pri prepinani env.
Pro plne multi-env pouziti nasledne `--build-config` s mapou prostredi na
nastaveni. Je to konfigurace editor serveru, ne nove klice v metamodelu.
Jeden zpusob skladani options; pri konfliktu nastaveni fail-fast. Konkretni
schema dopsat pred timto inkrementem do docs/testu, nikoliv vice paralelnich
nezavislych sad voleb v kazdem API handleru.

Build/validate/summary/inventory/inspect/cluster namespace musi pouzit tento
kontext. Nepredpokladat, ze konfigurace EncJson editoru se sama prenese do
builderu; propojit jeho explicitni zavislosti bez per-request `os.Setenv`.
Nactene vstupy drzet konzistentni v ramci operace; remote env nestahovat
znovu pro kazdou kartu/kontejner. Zobrazit cas vyhodnoceni a stale stav.
Zmena dokumentu/kontextu invaliduje effective view a preview indikaci.

URL/headers/cesty pochazeji ze serverove konfigurace, ne z libovolneho
neprivilegovaneho browser requestu. Headers a hodnoty secretu neserializovat
do kontextoveho API, URL hashe, localStorage ani logu. Endpoint s kontextem
vraci jen bezpecny popis zdroju. Efektivni env hodnoty mohou byt citlive:
zachovat aktualni auth/read opravneni, nevracet celou mapu build variables.
Preview vzdy do docasne slozky, nikdy do realneho deploy adresare.

### 4.3 Bezpecny zapis a reset

Ponechat atomic write, expected_hash a 409. Nove endpointy hash vyzaduji.
Pred fyzickym zapisem overit syntax a podporovane lokalni/reference
invarianty. Offline opravitelny source edit nesmi vyzadovat dostupny vault;
uplna build validace je samostatny stav, ne predstirany uspech.
Pokud write zavisi na katalogu defaults, overit i jeho hash; zmena defaults
behem otevreneho modalniho okna musi vyvolat refresh/conflict.

Nove patch operace pracuji s explicitnim field presence a minimalnim
zdrojovym patchem. Nerozesilat cely efektivni objekt jako update payload.
Zachovat nezname/pokrocile fieldy, komentare, poradi, template vyrazy,
folded values a odsazeni mimo upravovany usek. No-op Save = stejny soubor.
Pro zdrojovy parser pouzit template-aware vrstvu s vazbou na puvodni text;
prostou globalni nahradou placeholderu bez ohledu na YAML quoting/scalars
nepripravit dalsi chyby. yaml.Node muze pomoci lokalizaci, ale remarshal
celeho dokumentu neni prijatelna nahrada cileneho patchovani.

"Reset to inherited" smaze pouze lokalni override; prazdny retezec/false/0
neni automaticky reset. U seznamu (napr. ports) je lokalni prepsani celeho
seznamu explicitni operace s nadhledem dopadu; nepouzivat scalar patch
pravidla. Odstraneni zdedene env polozky pouziva builderem podporovane
remove: true, zatimco reset lokalniho override obnovi zdedenou polozku.
Otestovat remove v kombinaci vice vrstev; neslibovat fungovani, ktere
soucasne merge poradi neumi.

Odstraneni sidecar reference s lokalnim patchem: ukazat dopad a vyzadovat
explicitni odstraneni reference i patche v jedne zmene app dokumentu,
nebo akci odmitnout. Nevyrabet nevalidni app-only fragment.
Rename/delete sdilene definice v prvni verzi blokovat, pokud ma pouziti;
automaticky multi-file rename/cascade neni soucast tohoto planu.

## 5. UI/UX

Ponechat Apps/Assets/Build/Changed files. Pridat polozku **Defaults** pro
vybrane env (stejny obsah dostupny pres dosavadni asset deep link).
Defaults obsahuje prehledne sekce/karty s poctem pouziti, filtrem a Edit:
Variables, Container env defaults, Container profiles, Sidecar definitions,
Workload identity, Runtime asset defaults, Runtime asset definitions,
Pod/registry/labels. Definice se nesmi rozvinout do kilometru inputu.

App detail defaultne prezentuje efektivni nastaveni a badge puvodu:
Local / Profile / Shared sidecar / Defaults / External policy / Release.
Prepinac nebo zalozky **Effective / Local source**, viditelne informace
o chybejicim build kontextu. Z prehledu lze prejit na zdroj definice.
Hlavni containers, sidecars a generovane init containers oddelit.
Refs zobrazit jako kratky seznam s odkazy, vyber a poradi resit v modalu.

U pole zdedeneho z profilu nabidnout **Override for this app** a
**Open profile**, u lokalniho override **Reset to inherited**.
Uprava sdilene definice ma modal s poctem a seznamem dotcenych app.
Indikovat, kdy seznam pouziti nelze kompletne vyhodnotit (unresolved refs).
Podrobny provenance seznam staci otevrit na vyzadani.

Runtime asset editor jasne rozlisuje **Use defaults** / **Replace block**
pro source/volume/fetcher; pri replace lze explicitne predvyplnit zdedeny
blok. Nabidnout token/shared asset reference selecty se zachovanim
template hodnot. Neni to prohlizec obsahu vzdaleneho config serveru.

Read-only skryva mutace, ale zachova prehled puvodu a vazeb. U externi
resource policy zobrazit hodnoty/cestu a proc lokalni override neni ucinny;
policy repo v teto iteraci z UI needitovat. Release image zobrazit vcetne
puvodu, nevkladat resolved image do YAML misto release markeru.

Pokrocile dosud nepodporovane sekce zustanou viditelne jako read-only YAML
s jasnym duvodem. Formular je smi upravit az kdyz umi zachovat jejich
obsah. Zachovat Tabler modaly, focus, Escape, scroll a browser back/forward.

## 6. Implementacni etapy a akceptace

Kazda etapa ma byt samostatne reviewovatelna. Nemenit vse najednou a
neprohlasovat cely plan za hotovy po dokonceni read-only prehledu.

### P0: Regresni fixture a ochrana soucasnych editoru

- [x] Pridat `fixtures/edit-metamodel/environments/dev` s minimalnim
  defaults, java app, UI app a app-only mtls sidecarem. Dummy env/release,
  shared CA text, lokalni assets a externi policy fixture bez realnych dat.
- [x] Opravit container_envs DTO/UI/reader/writer na container_ref_name
  vcetne default testu; nepodporovane env formy zachovat.
- [x] Guard pro probes/resources a dalsi block-replace editory: dokud
  nezachovaji pokrocily obsah, odmitnout destruktivni zjednodusenou editaci
  s uzitecnou hlaskou. Ports existujici guard ponechat do P5.
- [x] Parser/read DTO spravne prenese token/shared asset reference,
  remove i prazdne value; nevydava je za obycejnou prazdnou promennou.
- [x] Test zapis defaults pres repository/API -> buildapp.Validate/Build
  s fixture vstupy; no-op a unsupported update nesmi zmenit soubor.

Soubory: `internal/repository/content.go`, `repository_test.go`,
`internal/webapp/server*.go`, `static/ui.js`. P0 je rychla ochrana;
template-aware source cteni pro cely novy model patri do P1.

### P1: Sdileny inspect a build context, source DTO

- [x] Zavedeni build kontextu (prvni CLI inkrement) a inspection API.
- [x] Template-aware source dokument vcetne raw/present/origin metadat.
- [x] Effective DTO zahrne sidecars, profily, asset refs, identity,
  obrazek/image, resources, startup, probes a porty; source zustava dostupny
  pri selhani effective vyhodnoceni. Netahat citlive globalni env mapy.
- [x] Udelat provenance v builder pipeline minimalne pro editovane bloky
  a env polozky; zapsat presne pravidlo priority pri kombinaci vrstev.
- [x] Namespace cluster panelu pres stejny resolver vstupu.
- [x] Prenest build options do vsech Build akci, pridat popis pouziteho
  kontextu a chybejicich vstupu. Do konce planu multi-env build-config.
- [x] Test CLI vs web preview nad totoznou fixture/options: shodne
  manifesty i namespace/image/resource hodnoty; zadna zmena sync hash/ID
  jako vedlejsi efekt refaktoringu.

Soubory: `internal/buildapp/build.go`, novy `inspect.go`/testy dle potreby,
`cmd/kube-edit-app/main.go`, `internal/webapp/server.go`, source DTO/parser
v novych tematickych repository souborech. Nefoukat dal vse do content.go.

Implementacni zaznam P0/P1:

- anonymni fixture je v `fixtures/edit-metamodel`; externi resource policy
  je oddelena od environments root stejne jako v realnem pouziti;
- inspection API je `buildapp.Inspect`, HTTP endpointy jsou `inspect` a
  `build-context`; schema a bezpecnostni omezeni popisuje
  `KUBE_EDIT_APP_BUILD_CONTEXT.cs.md`;
- source pole maji raw value, present stav a pozici, effective bloky a env
  polozky maji ordered origins, hash-bound write target a capabilities;
- efektivni sidecar zatim zamerne nema write target, protoze effective index
  nelze bez mapovani na definici/lokalni patch bezpecne pouzit pro zapis;
- zjednodusene editory odmitaji advanced probes/resources/runtime,
  autoscaling a advanced defaults container envs; app container env editor
  zachovava read-only polozky;
- targeted testy P0/P1, `node --check internal/webapp/static/ui.js`, kompletni
  `go test ./...`, CLI/web parity a `just build` prosly 2026-09-11.

### P2: Read-only orientace v modelu

- [x] Defaults stranka + backwards/deep-link navigace pres Assets.
- [x] Katalogy, source/effective app, sidecars a puvod hodnot.
- [x] Index pouziti profil/sidecar/runtime asset/token/shared asset ->
  app/container. Nevytvaret DB; vypocitat ze zdroju a inspection snapshotu.
- [x] Assets zpristupni shared.assets.yml jako metadata dokument a odkazy
  na jeho soubory. Build vstupy ukazat v Build kontextu; secret `.env`
  neexponovat automaticky jako dalsi verejny asset.
- [x] Summary counts nepredstiraji jen lokalni kontejnery jako cele Pod.
- [x] Source mode jde otevrit offline, unquoted placeholders se zobrazi
  v puvodni podobe. Zvolene env ma vzdy vlastni stale/loading/error stav.

Implementacni checkpoint 2026-09-11:

- `buildapp.Inspect` vraci source dokumenty defaults/shared assets, efektivni
  aplikace, puvod poli a vypocteny used-by index vcetne tranzitivnich referenci;
- UI ma read-only Defaults katalog a prepinac Effective / Local source v Apps;
- Assets strom obsahuje korenova metadata a `shared.assets.yml` ma
  strukturovany detail s odkazy na fyzicke asset soubory;
- browser smoke overil deep-link Defaults -> Apps, browser Back, efektivni
  sidecary/provenance a shared asset metadata -> asset soubor.

### P3: Explicitni reference a lokalni override

- [x] App sidecar/runtime asset refs; container/sidecar profile a asset
  refs. Profiles vybirat s poradi, ne neserazenym checkbox setem.
- [ ] Bezpecny scope selector a writer pro hlavni container/local
  sidecar patch; source hash + zavisle defaults hash.
- [ ] Lokalni patch vytvorit pouze po explicitni akci, zachovat ref.
  App-only sidecar je samostatna akce s image a startup/env/resources.
- [ ] Znovupouzit resource/env/startup modal nad ruznymi source scopes.
- [x] Reset scalaru/map a explicitni whole-list override podle kontraktu.
- [x] Test UI cgroup override meni jen regexp; image/startup/resources
  zustavaji z definice. Reset obnovi java regexp. Odebrani ref s patchem
  nenecha nevalidni fragment. App-only mtls gateway zustava samostatny.

Checkpoint P3.1 2026-09-11:

- endpoint `apps/{app}/references` vraci katalogy a source scope refs a zapisuje
  app/main-container/local-sidecar reference listy;
- writer kontroluje app i defaults hash, stabilni index+jmeno scope, zname a
  neduplikovane definice a zachovava poradi profilu;
- no-op save je byte-for-byte beze zmeny a cilena editace zachovava komentare
  a nesouvisejici YAML bloky;
- Tabler modal pouziva ordered radky se selectem, move up/down a remove; nic
  nematerializuje z effective modelu. Lokalni patch scope pokracuje v P3.2.

Checkpoint P3.2a 2026-09-11:

- effective sidecar env s jednoduchou `value` lze editovat pres jmeno sidecaru
  a env polozky; effective index se pro zapis nepouziva;
- zapis je vazany na hash app dokumentu i `_defaults.yml` a pro vybranou
  sdilenou sidecaru vytvori pouze minimalni lokalni `sidecars[].envs` patch;
- **Reset to inherited** odstrani jen lokalni env override. Pokud po nem
  zustane patch pouze se jmenem, odstrani se cely patch, ale
  `sidecar_ref_names` zustane beze zmeny. Ostatni lokalni patch pole a
  app-only sidecara se zachovaji;
- backend, endpoint, read-only guard, unit testy a browser smoke jsou hotove.
  P3 dale pokracuje resources/startup scope a pravidlem pro odebrani ref s
  existujicim patchem; proto zbyle checkboxy zatim nejsou uzavrene.

Checkpoint P3.2b 2026-09-11:

- stejny name-based selector a dual-hash kontrakt plati pro resources
  sdilene i app-only sidecary; modal ukazuje effective hodnoty jako kontext,
  ale zapisuje jen explicitne vyplnene lokalni hodnoty;
- zmena jednoho pole proto vytvori napriklad pouze
  `resources.cpu.requests`, ne kopii celeho zdedeneho resources bloku;
- reset odstrani jen lokalni resources blok a zachova env patch, reference i
  app-only sidecaru. Prazdny shared patch se uklidi;
- pri externi resource policy UI editaci nenabizi a API ji odmitne, protoze
  policy je autoritativni. Unit/API testy a browser override/reset smoke
  prosly. Dalsi samostatny inkrement je startup.

Checkpoint P3.2c 2026-09-11:

- sidecar `startup.command` a `startup.arguments` pouzivaji stejny name-based
  selector a dual-hash writer; oba seznamy jsou explicitni whole-list
  overrides s vlastnim prepinacem;
- nezaskrtnuty seznam se nematerializuje, zaskrtnuty prazdny seznam se zapise
  jako `[]`. Nezname klice v existujicim startup bloku zablokuji editaci;
- reset odstrani jen lokalni startup blok a zachova resources/env patch,
  reference i app-only sidecaru. Unit testy a browser override/reset smoke
  prosly;
- dalsi P3 inkrement musi vyresit odebrani sidecar reference s lokalnim
  patchem jako jednu explicitni, potvrzenou operaci.

Checkpoint P3.3 2026-09-11:

- odebrani sdilene sidecar reference s existujicim lokalnim patchem je jedna
  explicitni operace. Bez `remove_sidecar_patches` backend zapis odmitne;
- UI pred zapisem zobrazi potvrzovaci modal se jmeny sidecaru a po potvrzeni
  odstrani referenci i cely odpovidajici lokalni patch;
- writer pracuje podle jmena, zachova app-only sidecary i nesouvisejici YAML
  a prazdny `sidecar_ref_names` blok uklidi. Repository/API testy a browser
  smoke nad realnym UI prosly.

### P4: Editace sdilenych definic a identity

- [ ] Container profiles a sidecar definitions: create/edit/duplicate,
  startup command/arguments jako seznamy, image, envs vcetne ref typu,
  resources, profily sidecaru a runtime asset refs.
- [ ] Workload identity: service_account create/name/automount (tri stavy
  tam kde absent neni false); tokens name/audience/path/mount/expiration.
- [ ] Runtime asset defaults a definitions: source, volume, fetcher,
  files source/target/mode/sha256. Zachovat raw/security/resource rozsireni.
- [ ] Shared asset metadata name/file/to/options pro reference a CA,
  registry secret refs, labels/annotations a pod share_process_namespace.
- [ ] Used-by prehled pred shared edit, blokace pouziteho rename/delete.
- [ ] Structural validace bez site; uplna builder validace s kontextem;
  u chyb uvadet relevantni definici a pouzivajici app.
- [ ] Test block replacement runtime defaults, sdileny volume/dedup mount,
  kolize targetu, neznamy token/CA/ref a poradi generovanych init containeru.

Checkpoint P4.1a 2026-09-12:

- Defaults katalog umoznuje ve write rezimu upravit `image` existujici
  `sidecar_definitions` podle stabilniho jmena; create/rename/delete nejsou
  soucasti tohoto inkrementu;
- GET vraci raw template ze source YAML, nikdy interni preview marker. PATCH
  vyzaduje content hash a meni pouze image scalar; no-op je byte-for-byte;
- modal ukazuje used-by aplikace a pouziva pouze Tabler komponenty. Repository,
  API a browser smoke overily zachovani envs/startup/resources i okolnich
  komentaru. Dalsi inkrement muze pridat startup nebo resources definice.

Checkpoint P4.1b 2026-09-12:

- existujici `sidecar_definitions[].startup` ma samostatny name-based
  GET/PATCH kontrakt a Tabler modal pro `command` a `arguments`;
- oba seznamy rozlisuji absent, explicitni `[]` a hodnoty. Raw template
  segmenty se pri cteni presne zachovaji; interni preview marker se nepouziva;
- no-op nemeni source, odstraneni celeho startup bloku vyzaduje potvrzeni a
  writer zachova image, envs, resources, dalsi definice i komentare. Nezname
  startup klice editaci blokuji. Repository/API testy a browser create/diff
  smoke prosly.

Checkpoint P4.1c 2026-09-12:

- existujici `sidecar_definitions[].resources` ma samostatny name-based
  GET/PATCH kontrakt a Tabler modal pro CPU/memory request/limit;
- reader zachova raw `{{env:...}}` a `{{var:...}}` hodnoty. Writer pri
  skutecne zmene zachova zdrojovy dialekt `from/to` nebo `requests/limits`,
  no-op je byte-for-byte a odstraneni celeho bloku vyzaduje potvrzeni;
- neplatne quantity, smiseny resource dialekt a nepodporovana pole vcetne
  `ephemeral-storage` editaci blokuji misto tiche ztraty dat. Repository/API
  testy, plny Go test/build a browser render smoke prosly.

Checkpoint P4.1d 2026-09-12:

- existujici `sidecar_definitions[].envs` ma name-based GET/PATCH kontrakt a
  siroky Tabler tabulkovy modal se zachovanim poradi;
- editor podporuje builder varianty `value`, secret key, container resource,
  pod field, workload identity token ref, shared asset ref a `remove`.
  Zmena typu neposila skryta pole puvodni varianty;
- raw template value a explicitni prazdna value se zachovaji, no-op je
  byte-for-byte a odstraneni celeho bloku vyzaduje potvrzeni. Nezname nebo
  kombinovane source fields se odmitnou;
- token a shared-asset reference se naseptavaji a pri PATCH validuji proti
  lokalnim katalogum. Repository/API testy, plny Go test/build a JS syntax
  check prosly. Browser smoke zustava k rucnimu overeni kvuli selhani
  lokalniho Playwright CLI lifecycle pred otevrenim stranky.

Checkpoint P4.1e 2026-09-12:

- existujici sdilena sidecar definice ma samostatny name-based editor pro
  ordered `profile_ref_names` a `runtime_asset_ref_names`;
- modal znovupouziva stejny Tabler ordered-reference pattern jako app editor.
  Vybirat lze pouze z lokalnich katalogu a poradi profilu se zachovava;
- PATCH kontroluje content hash, duplicity i nezname definice, prazdny seznam
  odstrani odpovidajici blok a no-op je byte-for-byte. Repository/API testy,
  plny Go test/build a JS syntax check prosly.

Checkpoint P4.2 2026-09-12:

- [x] Pridat ke kazde aplikaci vizualni mapu efektivnich zavislosti bez
  paralelniho resolveru v JavaScriptu.
- [x] Rozlisit app-level reference, hlavni kontejnery a sidecary a zobrazit
  jejich profily, runtime assets, workload identity tokeny a shared assets.
- [x] Udelat uzly proklikavaci do vyfiltrovaneho katalogu Defaults a z mapy
  zpristupnit existujici modal pro editaci explicitnich referenci aplikace.
- [x] Rozsirit inspection usage o tranzitivni
  `sidecar_definition -> profile/runtime asset -> token/shared asset` vazby
  s ochranou proti cyklum a regresnim testem.

Mapa pouziva pouze existujici Tabler/Bootstrap komponenty a nepridava novou
CSS ani grafovou knihovnu. Browser smoke nad `fixtures/edit-metamodel` overil
render i proklik profilu do presne vyfiltrovaneho Defaults katalogu.

### P5: Doplnit existujici editory na dnesni model

- [x] Probes: shared http a live/ready/start HTTP/exec/timing hodnoty,
  preset/path varianty; no-op neztrati zadne detaily. Neni nutny formular
  na kazdy escape hatch, ale nezname casti se musi zachovat.
- [x] Ports: Route tls vcetne termination a existujicich tls poli,
  annotations/labels, seznamy HTTP/HTTPS hostu; zachovat service_name alias.
  TLS private-key/certificate data nevypisovat zbytecne do souhrnnych karet.
- [x] Resources: ephemeral-storage a zachovani dalsich resources,
  literal/template vstupy, local/inherited/external-policy indikace.
- [ ] Env typy: value, secret/resource/field formy builderu, token/shared
  asset refs, remove. Nezamichat raw Kubernetes valueFrom s metamodellem;
  zachovat existujici advanced varianty, nedeklarovat nepodporovany build.
- [ ] Image z release/profilu zobrazeno spravne; JAVA_ARGS zustava env
  retezec, automaticky jej neprevadet na runtime.java.
- [x] Doplnit read-only prehled ostatnich poli builderu: scheduling,
  securityContext, mounts/assets, env_from, tools, explicit init,
  pod_info/downward_api, rollout_on, autoscaling.raw, raw escape hatches.
  Jejich kompletni nove strukturovane editory nejsou podminkou tohoto planu.

Checkpoint P5.1 2026-09-12:

- Effective inspection DTO zverejnuje top-level deployment chovani, metadata,
  pod identity/security, scheduling, rollout, tools/registry/DNS a raw overlays.
- Advanced app, container a init-container hodnoty jsou v UI pouze read-only a
  standardne sbalene, aby nezvetsovaly bezny prehled. Blok se zobrazi jen pro
  skutecne nakonfigurovane oblasti a pouziva pouze Tabler/Bootstrap komponenty.
- Cileny DTO test, cela seriova Go suite, build, JS syntax a browser smoke na
  desktopu i uzkem viewportu prosly. Fixture ani builder kontrakt se nezmenily.

Checkpoint P5.2 2026-09-12:

- Main container, app-local sidecar override i shared sidecar definition
  resource editory podporuji `ephemeral-storage` request/limit.
- Repository zachovava `from/to` nebo `requests/limits` dialekt shared
  definice. Quantity i cele source templates se validuji na klientu i serveru.
- Nezname resource keys, nejednoznacny dialekt a nemapove bloky se odmitnou
  pred zapisem, takze je editor nemuze tise odstranit.
- Pozitivni repository/API testy, cela seriova Go suite, build a browser
  render nad anonymni fixture prosly.

Checkpoint P5.3 2026-09-12:

- Probes editor podporuje preset, scalarni i live/ready/start varianty cesty,
  shared HTTP a per-probe HTTP/exec handler i delay/period/timeout/success/failure.
- Repository meni pouze zname probe hodnoty. Nezname top-level i vnorene YAML
  klice zachovava a shodny payload vraci byte-identicky zdroj bez diffu.
- Nejednoznacne zname YAML tvary se pred zapisem odmitnou; porty, timing hodnoty
  a kompletni source templates se validuji na klientu i serveru.
- Repository/API testy, cela seriova Go suite, build, JS syntax a browser no-op
  smoke nad anonymni fixture prosly.

Checkpoint P5.4 2026-09-12:

- Ports editor podporuje vsechny HTTP/HTTPS hosty, ingress class, Route TLS
  termination/insecure policy a scalarni annotations/labels.
- Repository aktualizuje polozky podle zdrojovych indexu, zachovava nezname
  vnorene klice i legacy `hostname` dialekt a shodny save je byte-identicky.
- Souhrn ukazuje typ external exposure, host/path a Route TLS termination;
  privatni TLS certificate/key data do karet ani telemetrie nekopiruje.
- Dependency badge jsou primo klikatelne Tabler badge bez svetleho hover
  podkladu. Dark-mode browser kontrola vsech typu odkazu a ports no-op prosly.

### P6: Integrace, browser overeni, dokumentace

- [ ] Multi-env konfigurace build kontextu, pokud nebyla dokoncena v P1.
- [ ] Auth pro vsechny nove endpointy: read-only, env reader/writer, admin;
  nevytvaret pres defaults edit pristup do jineho env. Git pravidla zachovat.
- [ ] Regrese env.secured/unsecured special editoru, Git diff/commit flow,
  no-op, concurrent edit 409, base-path, back/forward a modal lifecycle.
- [ ] Browser smoke nad anonymni fixture desktop i uzsi viewport;
  zadne bezici testy proti skutecnemu clusteru/vaultu nejsou nutne.
- [ ] Docs README.md/README.cs.md, screenshoty pouze pokud uzitecne,
  aktualni handoff a checklist skutecne hotovych/odlozenych bodu.

## 7. Testovaci matice a definice hotovo

Klicove testy musi proverovat chovani, ne pouze tvar implementacniho DTO:

| Scenar | Ocekavani |
| --- | --- |
| Minimalni Java app jen s profile_ref_names | UI vidi image marker/startup/probes/port/env z profilu; local source zustane maly |
| Edit request CPU | Patch jen lokalni CPU, zadne materializovani ports/envs/identity |
| Zmena sdileneho profilu | Dotcene apps se invaliduji a ukazi novy vysledek, ostatni source soubory beze zmen |
| UI cgroup patch | Vybrana sidecara ma zdedenou image, pouze regexp je lokalni |
| Runtime ref na app vs sidecar | App asset jen v hlavnim kontejneru, mtls asset jen v gateway, init separatne |
| Runtime block replacement | Vlastni source nezdedi chybejici pole ze source defaults; chyby odpovidaji builderu |
| Nequotovane env/var placeholders | Source modal zachova vyraz; po effective eval se zobrazi cislo, no-op nic neprepise |
| Pokrocile probes/Route | Save jednoho pole zachova timings, TLS, anotace i vsechny hosty |
| Externi policy | UI ukaze authoritative hodnoty, chybejici soubor/entry je chyba, inline edit se netvari ucinne |
| Remote env / release / namespace | Shodne vysledky CLI a webu; release konflikt je chyba; headers se nevraci klientovi |
| Vypadek remote env nebo chybna app | Source zustane pristupny, effective stav je error/stale a ne prazdny uspesny model |
| Konkurentni zmena defaults | Otevreny override modal neulozi patch proti zastaralemu katalogu |
| YAML round-trip | Zachovani komentaru/poradi/unknown fields, flow a block sekvenci, folded multiline, prazdnych hodnot |

Zacitat targeted Go testy pro menene balicky. Po integraci:

```bash
node --check internal/webapp/static/ui.js
GOCACHE="$PWD/.tmp/go-build-cache" go test ./...
just build
```

Pouzit httptest pro remote env a synteticke release/policy soubory.
Pri zasahu do template parseru zafixovat oba stavajici i nove tvary YAML.
Po refaktoringu builderu porovnat vygenerovane manifesty vcetne sync metadat.
Build neznamena kubectl apply; pro tento plan nic nenasazovat.

Hotovo znamena: anonymizovana verze dev repa je plne prohlizitelna,
nejcastejsi zmeny profilu/refs/sidecaru/runtime assetu/identity/Route jsou
editovatelne bez rucniho YAML, UI preview odpovida CLI se stejnymi vstupy
a zadny editor nezahazuje pole, ktera nezobrazuje. Zamerne read-only
pokrocile sekce jsou viditelne a explicitne popsane, ne tise ignorovane.

## 8. Doporuceny start navazujici session

Precist tento dokument a git status. Zacit **P0**, pak **P1**; nevytvaret
vsechny modaly pred vyresenim source/effective kontraktu. Po kazde etape
zapsat do tohoto checklistu skutecny stav, testy a dalsi presny krok.
Pri potrebe zkratit session predat funkcni inkrement a konkretni TODO,
neopakovat celou analyzu repa. User bude implementaci poustet v dalsim
modelu; nazvy modelu nejsou technicka zavislost projektu.

Referencni tsm-user-management.yml ma take lokalni kopie sdilenych sidecaru
a podezrele zanoreny headless zaznam uvnitr external. Nekopirovat slepe cely
soubor jako golden fixture; pouzit overeny minimalni model. Externi repo
v ramci tohoto planu automaticky nenormalizovat.
