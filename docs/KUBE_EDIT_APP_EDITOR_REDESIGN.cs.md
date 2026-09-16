# kube-edit-app: dva editory pro obsluhu aplikaci

Datum: 2026-09-13, aktualizovano 2026-09-14. Stav: vizualni smer a navrh UI
schvalen uzivatelem; produkcni implementace jeste neni hotova.
Tento dokument urcuje dalsi UX smer a ma prednost pred UX pokyny ve starsim
`KUBE_EDIT_APP_METAMODEL_ALIGNMENT_PLAN.cs.md`. Builder kontrakt se nemeni.

## Proc menime smer

Defaults nesmi byt katalog backendovych entit s velkymi kartami, ze kterych
vedou nesouvisejici modaly pro jednotlive hodnoty. Uzivatel upravuje soubor,
vybranou definici a jeji nastaveni. Primarni jsou dva editory:

1. Defaults: spolecne definice, ktere odstranuji opakovani v aplikacich.
2. Aplikace: reference, lokalni odchylky a bezne provozni nastaveni.
3. Assets a diagnostika az jako navazujici nastroje.

Pouzit existujici konzervativni Tabler, stejny dark/light rezim. Neprepisovat
vizualni system. Nepridavat velke dashboard karty pro jednu hodnotu. Rozlozeni
se neridi poctem endpointu. Existujici backendove funkce mohou byt znovu pouzity,
nikoli ale jako duvod pro dalsi samostatne tlacitko Save u kazdeho pole.

## Defaults: struktura

Jazykove rozhodnuti (2026-09-14): produkcni UI kompletne v anglictine,
vcetne navigace, formularu, napoved, validacnich chyb a potvrzovacich dialogu.
Nepridavat i18n vrstvu, prepinac jazyka ani prekladove katalogy. Ceske texty
klikaci ukazky nejsou zadanim pro produkcni copy. Uzivatelska data, nazvy
a hodnoty YAML se neprekladaji. Plan a spoluprace mohou zustat v cestine.

Vlevo navigace castmi `_defaults.yml`. Vpravo vyber definice a jeden formular.
Seznam se pri vetsim poctu polozek da vyhledavat. Nevypisovat vsechny formulare
pod sebe. Neotevirat cely profil v modalu. Pouziti definice je sbalena doplnkova
informace, ne hlavni obsah stranky.

| Cast | Co uzivatel upravuje |
| --- | --- |
| Promenne | `vars`, kompaktni tabulka nazvu a skutecnych zdrojovych hodnot |
| Profily kontejneru | jeden `container_profiles[]`, formular jeho `defaults` |
| Sidecary | jednu `sidecar_definitions[]`, stejny kontejnerovy formular |
| Runtime assety | oddelene spolecne `runtime_asset_defaults` a vybranou definici |
| Sdilene assety | existujici sdilene definice, s jasnym oznacenim zdrojoveho souboru |
| Identita | `workload_identity`, service account a pojmenovane tokeny |
| Ostatni nastaveni | labels, registry, pod a ostatni podporovana spolecna pole |

Skutecne umisteni kazdeho pole overit podle Go modelu. Sdilene assety v jinem
souboru nesmi editor potichu presunout do `_defaults.yml`. Historicke
`container_envs` a dalsi podporovane deklarace musi zustat dostupne; nepotrebne
pokrocile casti mohou byt sbalene, ne ztracene.

### Jeden profil = jeden formular = jedno ulozeni

Zalozky formulare:

- Zakladni: identifikator, image z release manifestu nebo explicitni image/sablona.
- Spousteni: seznam command a arguments, poradi, pridani a odebrani.
- Promenne kontejneru: tabulka nazvu, typu hodnoty/reference a hodnoty.
- CPU a pamet: request/limit, podporovane jednotky a sablony.
- Probes: typ kontroly a jeji nastaveni, startup/readiness/liveness parametry.
- Porty a sluzby: port kontejneru, services a jejich external routes/ingresses.

Zalozky pouze prepinaji pohled na jeden draft. Spolecne `Ulozit profil` a
`Zrusit zmeny`. Zadna prubezna serverova ulozeni jednotlivych sekci. Pred
opustenim rozepsane definice se zeptat na zahozeni. Ukladani/chyba/konflikt
nesmi ztratit draft. Pri validacni chybe otevrit prislusnou zalozku a pole.

Image marker `<from release manifest>` zustava markerem, nikoli URL. Explicitni
image je zdrojova hodnota; formular nesmi slibovat zmenu priority vuci release
manifestu/CLI image policy. Tu urcuje stavajici builder. Bez editace image zadny
velky input. Reference a sablony se nesmeji pri ulozeni nahradit resolved hodnotou.

Rozlisovat nenastaveno, explicitne prazdny seznam, nulu a remove deklaraci podle
kontraktu konkretniho pole. CPU/memory pouziva v YAML `from`/`to`, i kdyz UI
rika request/limit. Nemenit slovnik builderu kvuli nazvum ve formulari.

Nazev pouzite definice neni obycejne textove pole pro bezpecne prejmenovani:
prvni produkcni verze ho zobrazi read-only. Create/delete/rename jsou samostatne
explicitni operace s kontrolou referenci; zadny tichy prepis odkazujicich app.

## Editor aplikace

Vyber aplikace otevre jeji zdrojovy formular, ne nekonecny Effective report.

- Reference: vyber profilu kontejneru, sidecaru a runtime assetu z definic.
- U reference kratke vysvetleni a proklik na definici v Defaults.
- App-level a container-level runtime assety drzet oddelene podle kontraktu;
  neprezentovat app-level referenci jako automaticke mountovani do vsech kontejneru.
- Vyber hlavniho kontejneru/sidecaru a stejny formular pro resources, startup,
  probes, envs a porty jako v Defaults.
- Zdedena hodnota ma zdroj, akci `Upravit lokalne` a u override `Vratit zdedene`.
  Reset odstrani jen lokalni deklaraci, ne zapise kopii efektivni hodnoty.
- Pridani sidecaru pres referenci je vychozi cesta; lokalni definice je explicitni.
- Pro lokalni patch sdilene sidecary zachovat nutnost `sidecar_ref_names`.
- Efektivni model, zdrojovy YAML, dependency map a build preview zustanou
  dostupne jako vedlejsi pohledy, ne prvni tri obrazovky formularoveho editoru.

Routes/Ingresses se edituji z konkretni sluzby. Zobrazit rozliseni OpenShift
Route/Ingress, host/path, TLS vcetne edge termination a anotace. Nezavest dalsi
nezavisly globalni katalog, ktery ztrati vazbu na port aplikace.

## Bezpecne ukladani a technicke hranice

- Zachovat read-only default, allow-write, autorizaci a concurrency guard.
- GET vraci zdrojovou deklaraci, aktualni hash a kontext/usage; resolved data
  slouzi jen pro vysvetleni, ne jako vychozi payload pro zapis.
- Ulozeni jedne definice prenese vsechny jeji zmeny jako jednu transakci.
  Nelze ho nahradit sekvenci stavajicich PATCH volani, ktera mohou selhat napul.
- Pred zapisem overit hash, validovat cely zamysleny vysledek a az potom
  atomicky nahradit soubor. Konflikt ponecha formular rozepsany.
- Zachovat poradi, sablony, neznama pole a nezmenene casti zdroje. No-op musi
  byt byte-stable. Zmena jednoho profilu nema preformatovat ostatni definice.
- Zkontrolovat existujici `defaults_container_profile.go`: dosavadni image-only
  writer neni automaticky vhodny pro plny formular. Overit flow/block YAML,
  multiline scalars, komentare, odsazeni a rozsah meneneho uzlu. Kde bezpecny
  zapis neni podporovan, vratit vysvetlenou chybu misto tiche ztraty dat.
- Nepouzivat JS jako druhy effective resolver; zachovat sdileny Go inspect.
- Existujici funkcni validatory, git review a testy znovu pouzit. Nemazat
  rozpracovane zmeny jen proto, ze nahrazujeme jejich UI.

## Klikaci ukazka

Soubory: `docs/prototypes/defaults-editor/index.html` a `prototype.js`.
Samostatna stranka, zadna zmena produkcni aplikace ani realnych YAML souboru.
Pouziva Tabler 1.4.0 z CDN jako aktualni UI; pro CSS potrebuje sit.

Spusteni z korene projektu:

```sh
python3 -m http.server 8877 --bind 127.0.0.1 --directory docs/prototypes/defaults-editor
```

Otevrit `http://127.0.0.1:8877`. Ukonceni serveru: Ctrl+C.
Save uklada pouze do pameti stranky; reload vraci ukazkova data.

Co lze posoudit: kompletni rozsah vzoroveho `java-jib-service` (image,
command/arguments, envs, HTTP probes, resources, porty/sluzby), prepinani casti,
spolecne Save/Cancel, kompaktni vars a prepinani dvou sidecaru. Ukazka pouziva
anonymizovane hodnoty podle realneho dev modelu, ne nacitani repozitare.

Co ukazka NENI: produkcni validator, source writer, effective resolver nebo
uplny editor vsech variant modelu. Identita/ostatni jsou oznacene placeholdery,
externi routes/ingresses zatim nejsou editovatelne. Assety predstavuji jen
rozlozeni seznamu souboru. Pomocny JS wrapper `defaults` u sidecaru/assets
neni navrh zmeny jejich YAML schematu. Editace jmena funguje jen v pameti,
neni implementaci bezpecneho prejmenovani. Zadna z techto zkratek se nesmi
mechanicky prenest do produkcniho API.

## Postup a checklist

- [x] Zapsat novou produktovou hierarchii a hranice editoru.
- [x] Pripravit samostatny klikaci navrh nad reprezentativnim profilem.
- [x] Overit draft, Save/Cancel, dark/light a mobilni rozlozeni v prohlizeci.
- [x] Uzivatel potvrdil vizualni smer a navrh UI (2026-09-14).
- [ ] Uzivatelske overeni tri ukolu nize; schvaleni smeru neni potvrzeni jejich otestovani.
- [x] Implementovat prvni produkcni Defaults formular profilu od GET po jeden Save (rozsah kroku 1 nize).
- [x] Overit round-trip a konflikty automaticky i na anonymizovane fixture.
- [ ] Uzivatel vizualne a funkcne potvrdi krok 1; do te doby nezacinat dalsi krok.
- [ ] Stejny kontejnerovy formular pouzit pro sdilene sidecary.
- [ ] Doplnit runtime defaults/definitions, identitu a ostatni Defaults sekce.
- [ ] Upravit editor aplikace pro reference a lokalni overrides.
- [ ] Odstranit nahrazene UI cesty az po overeni funkcni parity.
- [ ] Aktualizovat produktove README a handoff podle skutecne integrovaneho stavu.

Prvni tri ukoly pro uzivatelske posouzeni:

1. V profilu zmenit startup argument a probe, prepnout zpet a ulozit oba najednou.
2. V Promennych najit a zmenit CONFIG_SERVER_URL bez prochazeni velkych karet.
3. Vybrat jinou sidecaru, upravit env hodnotu a zrusit zmenu.

Overeni 2026-09-13: `node --check` uspesny. Playwright overil zachovani
startup argumentu napric zalozkami, spolecne ulozeni s probe zmenou, Cancel,
optional resource editaci vracenou na prazdnou hodnotu bez zustatku v draftu,
prepnuti sidecary a Cancel jeji env hodnoty. V sirce 390 px overeny vsechny
zalozky profilu a vars bez horizontalniho preteceni cele stranky. Screenshoty
desktop dark, desktop light a mobile dark byly vizualne prohlednuty. Nejde
o test produkcniho API ani YAML zapisu; Go testy nebyly pro staticky navrh spousteny.

Pro produkcni akceptaci navic: klavesnice a focus, read-only uzivatel,
nenapadna usage informace, zadny horizontalni overflow cele stranky, zadne
ztracene hodnoty pri prepinani zalozek, zachovani templatu, no-op diff,
jednopolozkovy diff, validacni chyby a soubezna editace. Responzivni tabulka
muze mit vlastni horizontalni posuvnik, nikoli roztahovat celou stranku.

## Prompt pro navazujici implementaci

### Krok 1, 2026-09-14: pripraven k uzivatelskemu overeni

Produkce nyni otevre `Defaults > Container profiles > Edit profile` na strance,
ne v modalu. Zalozky General, Startup, Environment, CPU / memory, Probes,
Ports / services sdileji jeden draft a jeden Save/Cancel. Jmeno profilu je pevne.
Pole jsou anglicky. Ostatni Defaults katalog a jeho editory zatim zustavaji.

Rozsah: image/marker, ordered command/arguments, env hodnoty/reference,
CPU/memory, spolecny HTTP probe endpoint a casovani live/ready/start,
porty kontejneru a sluzby. Alias `service_name`/`hostname` a resources
`requests/limits`/`from/to` se zachova. Externi routes/ingresses,
per-probe HTTP/exec/preset a dalsi pokrocila pole se zachovavaji, nejsou timto
krokem kompletne editovatelna. To neni dokonceni vsech variant metamodelu.

API prijima `profile.defaults` jako cely source draft + `expected_hash`.
Writer validuje pred zapisem, meni jen upravene top-level bloky uvnitr vybraneho
defaults a kontroluje semantic shodu celeho vysledneho souboru. Uvnitr upraveneho
bloku zachovava poradi/kolekcni styly a komentare, muze upravit formatovani.
Nezmenene bloky a okolni profily zustavaji byte-stable. Anchors/aliases,
flow forma celeho profilu/defaults a vice YAML dokumentu jsou pro zapis
odmitnuty s chybou, nikoli potichu prepsany.

Overeni: `go test ./...`, `node --check` obou produkcnich JS souboru a build
`dist/kube-edit-app` prosly. Repository/API testy pokryvaji vice poli v jednom
save, validacni chybu bez castecneho zapisu, conflict/read-only, raw sablony,
multiline image, ruzne odsazeni, CRLF, no-op a odmitnuti nebezpecne YAML formy.
Browser test ulozil startup + probe, overil je po reloadu, zachovani draftu
pri validacni chybe i soubeznem zapisu, Cancel, potvrzeni odchodu, poradi
argumentu, alias sluzby a vsechny zalozky pri sirce 390 px. Dark/light byly
zkontrolovany i vizualne. Hlavicka dostala pouze Tabler flex-wrap kvuli mobilu.

Samostatna testovaci kopie: `.tmp/profile-editor-review/environments`.
Review server byl spusten na `http://127.0.0.1:8189/#env=dev&page=defaults`.
Jeho Save uz opravdu meni tuto testovaci kopii; nejde o stary in-memory prototyp.
Realne environments repo nebylo meneno. Zadny commit nebyl vytvoren.

**STOP: cekat na uzivatelovo potvrzeni kroku 1.** Dalsi sekce nerozsirovat
automaticky. Pripadne nalezene vady tohoto kroku nejdriv opravit a znovu predat.

Precti tento dokument a aktualni `.codex-handoff/NEXT_PROMPT.md`. Nejdriv
zkontroluj git status a potvrzeni UX od uzivatele. Klikaci ukazku ber jako
navrh ovladani, nikoli jako hotovy datovy model/API. Zacni jednim produkcnim
Defaults profilem s jednym atomickym ulozenim celeho formulare a testy.
Nepridavej dalsi image-only modaly ani Save tlacitka ke kazdemu poli.
Nemen builder kontrakt, Ruby ani realne environments repo. Prubezne aktualizuj
checklist; neoznacuj sekci za hotovou, pokud je jen read-only nebo placeholder.

## Zpetna vazba ke kroku 1, 2026-09-14

- [x] Po Save vzdy dokoncit stav nacitani inspect i pri otevrenem profilu;
  pri chybe obnoveni zobrazit chybu, nikoli ponechat spinner.
- [x] Navrat do Defaults pres breadcrumb; radek vyberu obsahuje pouze profil.
- [x] Vysledek ulozeni presunout do paticky k Save/Cancel.
- [x] CPU/memory +/- a volba kroku pro kazdy resource, pouze lokalni preference UI.
  CPU pouziva millicores; memory kroky Mi. Binarni jednotky lze prevest bez
  zaokrouhlovani (1Gi + 64Mi = 1088Mi). Template, nepodporovany format,
  podteceni nebo unsafe integer vypne krokovani, ale ponecha textovou editaci.
  Prazdne pole je oznaceno Not set, nikoli prikladovou hodnotou podobnou ulozene.
- [x] Opravit profilovou validaci container/service portu: cela sablona je
  platna zdrojova deklarace, literal musi byt 1..65535. Build musi dale overit
  existenci promenne a skutecny vysledek. Nemeni se substitucni kontrakt builderu.
- [ ] Uzivatel potvrdi tyto opravy. Nepokracovat automaticky na dalsi editor.

Overeno: repository/webapp Go testy, JS syntax, Node regresni testy
`node --test internal/webapp/profile-inspection.test.cjs internal/webapp/resource-stepper.test.cjs`.
Browser: save image bez visiciho Loading, simulovane selhani inspect po save,
footer feedback, breadcrumb, volba kroku bez dirty stavu, literal stepping,
blokovani kroku u template, ulozeni templated service port, dark/light a 390px.
Testovaci zapisy byly provedeny v oddelene kopii v /private/tmp. Review server
na 8189 byl restartovan; uzivatelova `.tmp/profile-editor-review/environments`
nebyla pri teto kontrole prepisovana.

## Navrh: kontextovy template input (zatim NEIMPLEMENTOVAT)

Uzivatel upozornil na klicovou potrebu lepsi prace s placeholdery. Navrh:

- Ponechat kompaktni input s tlacitkem Insert variable, ne velky editor.
- Rozlisit literal, celou referenci a slozenou sablonu (URL/image s prefixem
  a vice tokeny). Neomezit model jen na vyber jedne promenne.
- Picker vyhledava nazvy a ukazuje namespace `var`/`env`, zdroj deklarace,
  rozsah platnosti a dostupnost. Neni to globalni seznam vsech promennych.
- Ve sdilenem profilu muze `APP_NAME` zaviset na konkretni aplikaci.
  Preview proto vybere kontext aplikace/kontejneru, pripadne ukaze Depends on
  application. Absence v samotnych Defaults neni dukaz neplatne reference.
- Save zachova zdroj; zadne nahrazovani resolved hodnotou. Kontrola syntaxe
  a typu pole pri editaci, vyhodnoceni existence/typu/range v dostupnem build
  kontextu pres stavajici Go resolver. Chybejici kontext oznacit jako neoverene,
  ne predstirat platnost nebo zablokovat validni sdilenou definici.
- Port umozni napr. `{{var:DEFAULT_SERVICE_PORT}}`, ale v pouzivajici aplikaci
  musi jit o definovanou promennou s vysledkem 1..65535. Resource po vyhodnoceni
  musi mit platnou jednotku. Schema pole muze substituci omezovat; neposkytovat
  automaticky stejnou nabidku u vsech enum/boolean/reference-name poli.
- `{{var:NAME}}` = YAML vars, `{{env:NAME}}` = env vstupy builderu,
  `{{NAME}}` = legacy passthrough. Posledni variantu nikdy potichu neprevest
  na env/var ani ji neoznacit za vyresenou v builderu.
- Picker/preview nesmi odhalovat tajne hodnoty. Browser nesmi primo volat
  env-url s pristupovymi hlavickami; pouzit serverovy kontext a bezpecna metadata.

Nejdriv schvalit malou ukazku komponenty na service portu. Nepropagovat ji
automaticky do vsech formularu bez uzivatelskeho potvrzeni.

### Klikaci ukazka template inputu, 2026-09-14

Uzivatel schvalil opravy kroku 1 a zadal pouze ukazku dalsi komponenty,
nikoli jeji produkcni implementaci. Ukazka je v `docs/prototypes/template-input/`,
spustena samostatne na http://127.0.0.1:8878. Produkcni soubory se v tomto kroku
nemenily. Vsechny texty UI jsou anglicky.

Kompaktni port input + Insert variable, prohledavatelny vyskovy omezeny seznam
se zdrojem a var/env rozlisenim, Preview for application/main, vysledek a
sbalene Source value. Volba promenne nahrazuje cely port (nikoli pripojeni k 80).
Pomocne scenare jsou sbalene mimo komponentu. Zadny Save ani pristup do repa.

Overeno browserem: search/selection, context-free Not verified, java-api=8080
vs worker=9090 bez prepsani source, missing/noninteger/range/legacy/sensitive
scenare, Escape a mobilni sirka 390 px. Dark/light screenshoty vizualne prohlednuty.
JS syntax check prosel. Jde o konecnou sadu simulovanych dat, nikoli druhy resolver.
Produkce musi pouzit existujici Go resolver; regex/fixture z ukazky se neprebira.

- [x] Pripravit ukazku pouze pro Service port.
- [x] Uzivatel potvrdil ukazku 2026-09-14.

### Upresneni ukazky: Template / Raw value

Uzivatel pozitivne hodnotil smer ukazky a pozadal o dva rezimy. Upraven pouze
prototyp na 8878, nikoli produkcni editor:

- Template: jedna i vice var/env/legacy referenci jako neprepisovatelne tokeny
  vcetne okolniho zdrojoveho textu.
- Raw value: volne editovatelny presny zdroj, vcetne reference nebo slozeneho textu.
- Detekce ze source hodnoty, zadny typovy discriminator v YAML. Explicitni Raw
  editace je docasna UI preference, nikoli zakaz substituce. Pri jejim opusteni
  se neprepisuje template vypoctenou hodnotou.
- Legacy passthrough a slozene sablony zustavaji Template source. Produkce nesmi
  prevzit omezenou regex gramatiku demonstratoru jako novy resolver.
- Volba Variable nad literalem otevre picker. Cancel ponecha puvodni source.

Browserem overeno: detekce cele reference, readonly token, raw editace bez
materializace, cancel pickeru bez zmeny source, zachovani explicitni raw editace,
legacy/compound raw klasifikace, kontextovy nahled, dark/light a 390px.
Uzivatel tuto upresnenou ukazku schvalil 2026-09-14.

### Produkcni krok: Service port

- [x] Samostatna source-only komponenta `static/template-input.js`.
- [x] Zapojena pouze na Service port v editoru container profilu.
- [x] Inspection poskytuje jen nazvy a zdroje referenci; hodnoty env se do JSON
  nevkladaji a implicitni procesni env se omezuje na reference pouzite v YAML.
- [x] Go/Node testy a save do docasne kopie fixture prosly.
- [ ] Uzivatel vizualne a funkcne potvrdi produkcni Service port komponentu.

STOP: dalsi pole, vcetne image, nenapojovat pred potvrzenim.
