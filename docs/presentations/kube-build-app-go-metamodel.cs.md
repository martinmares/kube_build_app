---
marp: true
theme: default
paginate: true
size: 16:9
title: kube-build-app: přechod na Go a vývoj metamodelu
description: Technická prezentace pro zákazníka a L2 support
---

<style>
  :root {
    --dl-red: #cf2e35;
    --dl-red-dark: #a71920;
    --dl-dark: #242b34;
    --dl-navy: #30394a;
    --dl-muted: #7b8492;
    --dl-light: #f6f7f9;
    --dl-code: #f8fafc;
    --dl-code-text: #1f2937;
    --dl-border: #dfe3ea;
    font-family: "Aptos", "Segoe UI", sans-serif;
  }

  section {
    background: var(--dl-light);
    color: var(--dl-navy);
    padding: 52px 64px 46px 64px;
    font-size: 25px;
    line-height: 1.35;
    display: flex;
    flex-direction: column;
    justify-content: flex-start !important;
    align-items: stretch !important;
  }

  section::after {
    color: var(--dl-muted);
    font-size: 16px;
    right: 32px;
    bottom: 22px;
  }

  h1, h2, h3 {
    color: var(--dl-dark);
    font-weight: 800;
    letter-spacing: -0.025em;
  }

  h1 {
    font-size: 52px;
    line-height: 1.05;
    margin: 0 0 26px 0;
    max-width: 900px;
  }

  h2 {
    font-size: 40px;
    margin: 0 0 22px 0;
  }

  h3 {
    font-size: 27px;
    margin: 0 0 10px 0;
  }

  strong {
    color: var(--dl-red);
  }

  a {
    color: var(--dl-red);
  }

  ul, ol {
    margin-top: 12px;
  }

  li {
    margin: 7px 0;
  }

  code {
    color: var(--dl-code-text);
    background: #eef2f7;
    border-radius: 6px;
    padding: 2px 6px;
    font-family: "Cascadia Code", "JetBrains Mono", monospace;
  }

  pre {
    background: var(--dl-code);
    border: 1px solid #cfd6df;
    border-radius: 12px;
    padding: 18px 20px;
    box-shadow: 0 10px 24px rgba(36, 43, 52, 0.10);
  }

  pre code {
    color: var(--dl-code-text);
    background: transparent;
    padding: 0;
    font-size: 15.5px;
    line-height: 1.18;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 20px;
  }

  th {
    background: var(--dl-dark);
    color: white;
    font-weight: 700;
  }

  td, th {
    border: 1px solid var(--dl-border);
    padding: 9px 12px;
  }

  .lead {
    font-size: 31px;
    max-width: 880px;
    color: #3b4658;
  }

  .muted {
    color: var(--dl-muted);
  }

  .kicker {
    color: var(--dl-red);
    font-weight: 800;
    text-transform: uppercase;
    letter-spacing: 0.09em;
    font-size: 16px;
    margin-bottom: 14px;
  }

  .brand {
    position: absolute;
    left: 64px;
    top: 30px;
    display: flex;
    align-items: center;
    gap: 12px;
    color: white;
    font-weight: 800;
    font-size: 21px;
  }

  .brand img {
    height: 27px;
    width: auto;
  }

  .footer-note {
    position: absolute;
    left: 64px;
    bottom: 28px;
    color: rgba(255,255,255,.68);
    font-size: 18px;
  }

  .dark {
    background: var(--dl-dark);
    color: white;
  }

  .dark h1, .dark h2, .dark h3 {
    color: white;
  }

  .dark .lead {
    color: rgba(255,255,255,.82);
  }

  .dark::after {
    color: rgba(255,255,255,.45);
  }

  .accent {
    color: var(--dl-red);
  }

  .grid-2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 28px;
    align-items: start;
  }

  .grid-3 {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 22px;
  }

  .card {
    background: white;
    border: 1px solid var(--dl-border);
    border-radius: 18px;
    padding: 24px 26px;
    box-shadow: 0 12px 26px rgba(36, 43, 52, 0.08);
  }

  .dark .card {
    background: rgba(255,255,255,.07);
    border-color: rgba(255,255,255,.14);
    box-shadow: none;
  }

  .card h3 {
    color: var(--dl-red);
  }

  .dark .card h3 {
    color: #ff626a;
  }

  .split-title {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 26px;
  }

  .before, .now {
    border-radius: 18px;
    overflow: hidden;
    background: white;
    border: 1px solid var(--dl-border);
  }

  .before h3, .now h3 {
    margin: 0;
    padding: 13px 18px;
    color: white;
    font-size: 21px;
  }

  .before h3 {
    background: #6b7280;
  }

  .now h3 {
    background: var(--dl-red);
  }

  .before pre, .now pre {
    margin: 0;
    border-radius: 0;
    box-shadow: none;
    min-height: 285px;
  }

  .small-code pre code {
    font-size: 13.2px;
    line-height: 1.12;
  }

  .tiny-code pre code {
    font-size: 12.2px;
    line-height: 1.13;
  }

  .code-card {
    border-radius: 18px;
    overflow: hidden;
    background: white;
    border: 1px solid var(--dl-border);
  }

  .code-card pre {
    margin: 0;
    border: 0;
    border-radius: 0;
    box-shadow: none;
  }

  .callout {
    margin-top: 30px;
    border-left: 7px solid var(--dl-red);
    background: white;
    border-radius: 12px;
    padding: 14px 18px;
    font-size: 21px;
    line-height: 1.28;
    box-shadow: 0 10px 24px rgba(36, 43, 52, 0.08);
  }

  .number {
    font-size: 58px;
    line-height: 1;
    font-weight: 900;
    color: var(--dl-red);
  }

  .pill {
    display: inline-block;
    padding: 7px 13px;
    border-radius: 999px;
    background: rgba(207, 46, 53, .12);
    color: var(--dl-red);
    font-size: 17px;
    font-weight: 800;
  }
</style>

<!-- _class: dark -->
<!-- _paginate: false -->

<div class="brand"><img src="https://www.datalite.cz/assets/img/logo-light.png" alt="DataLite" /></div>

# kube-build-app<br>přechod na Go a vývoj metamodelu

<div class="lead">Technické shrnutí pro zákazníka a L2 support: co se mění, co zůstává kompatibilní a jak nově zapisovat běžné scénáře v <code>&lt;app&gt;.yml</code>.</div>

<div class="footer-note">DataLite · interní technická prezentace</div>

---

# Účel změny

<div class="grid-3">
<div class="card">
<h3>Produktizace</h3>
<p>Go verze je distribuovatelná jako samostatná binárka pro Linux, macOS a Windows.</p>
</div>
<div class="card">
<h3>Provozní jistota</h3>
<p>Jednodušší použití v GitLab pipeline, méně závislostí na runtime prostředí.</p>
</div>
<div class="card">
<h3>Lepší metamodel</h3>
<p>Nové bloky pro probes, scheduling a mounts s důrazem na čitelnost.</p>
</div>
</div>

<div class="callout">Cílem není změnit způsob práce se zákaznickými prostředími. Cílem je zjednodušit nasazování, validaci a údržbu.</div>

---

# Co zůstává stejné

- Environment repozitář zůstává hlavním zdrojem konfigurace.
- Adresářová struktura <code>&lt;env&gt;/apps/*.yml</code> zůstává zachována.
- <code>env.unsecured.json</code> a <code>env.secured.json</code> zůstávají podporované.
- Assety a shared assety zůstávají podporované.
- Stávající YAML soubory jsou zpětně kompatibilní.

<div class="callout">L2 support nemusí přepisovat existující konfigurace. Nový zápis je doporučený pro nové změny a postupné čištění.</div>

---

# Co přináší Go verze

<div class="grid-2">
<div class="card">
<h3>Předtím</h3>
<ul>
<li>Ruby runtime a gem závislosti</li>
<li>složitější distribuce mimo vývojářské prostředí</li>
<li>méně vhodné pro Windows stanice</li>
<li>vyšší riziko rozdílů mezi lokálem a pipeline</li>
</ul>
</div>
<div class="card">
<h3>Nyní</h3>
<ul>
<li>jedna binárka <code>kube-build-app</code></li>
<li>buildy pro Linux, macOS a Windows</li>
<li>Cobra CLI a kontextová nápověda</li>
<li>lepší základ pro CI/CD a podporu</li>
</ul>
</div>
</div>

---

# CLI pro L2 support a CI/CD

```bash
kube-build-app validate -e test -R environments
kube-build-app summary  -e test -R environments
kube-build-app inventory -e test -R environments
kube-build-app build    -e test -R environments -t deploy/test
```

<div class="grid-3">
<div class="card"><div class="number">1</div><h3>validate</h3><p>rychlá kontrola modelu</p></div>
<div class="card"><div class="number">2</div><h3>summary</h3><p>přehled CPU, paměti, replik a kontejnerů</p></div>
<div class="card"><div class="number">3</div><h3>inventory</h3><p>strojově čitelný přehled pro UI a automatizaci</p></div>
</div>

---

# Metamodel není Helm

<div class="lead">Záměrně nepřepisujeme Kubernetes do dalšího univerzálního templating systému.</div>

<div class="grid-2">
<div class="card">
<h3>80 %</h3>
<p>Běžné deployment scénáře mají mít jasná pole v metamodelu.</p>
</div>
<div class="card">
<h3>20 %</h3>
<p>Speciální Kubernetes případy mají jít přes explicitní výjimečný <code>raw</code> zápis.</p>
</div>
</div>

<div class="callout">Pravidlo: pokud existuje dedikované pole v metamodelu, použijeme ho. <code>raw</code> je výjimka, ne výchozí styl práce.</div>

---

# Proměnné: závazný význam

| Zápis | Význam | Kdy použít |
|---|---|---|
| <code>{{VAR}}</code> | runtime placeholder | ponechat pro apply-env / deploy fázi |
| <code>{{env:VAR}}</code> | build-time environment variable | hodnoty z env JSON, .env nebo process ENV |
| <code>{{var:VAR}}</code> | lokální app variable | hodnota z <code>vars</code> v <code>&lt;app&gt;.yml</code> |

<div class="callout">Důležité: existující šablony zůstávají kompatibilní. Cílem je pouze jasně pojmenovat odpovědnost jednotlivých scope.</div>

---

# Probes: jak to bylo

<div class="split-title tiny-code">
<div class="before">
<h3>Původní zápis: health</h3>

```yaml
health:
  http:
    path:
      live: /health/live
      ready: /health/ready
    port: 8080
  delay: 10
  period: 5
  timeout: 3
  success: 1
  failure: 2
```

</div>
<div class="before">
<h3>Původní zápis: probe</h3>

```yaml
probe:
  live:
    http:
      path: /actuator/health
      port: 8080
    period: 10
    failure: 5
  ready:
    http:
      path: /actuator/health
      port: 8080
    period: 2
    failure: 2
  start:
    http:
      path: /actuator/health
      port: 8080
    failure: 30
```

</div>
</div>

---

# Probes: správný nový zápis

<div class="split-title">
<div class="before small-code">
<h3>Dříve</h3>

```yaml
probe:
  live:
    http: { path: /actuator/health, port: "{{var:EXPOSE_PORT}}" }
  ready:
    http: { path: /actuator/health, port: "{{var:EXPOSE_PORT}}" }
  start:
    http: { path: /actuator/health, port: "{{var:EXPOSE_PORT}}" }
    failure: 30
```

</div>
<div class="now small-code">
<h3>Nyní doporučeno</h3>

```yaml
probes:
  preset: spring-actuator
  port: "{{var:EXPOSE_PORT}}"
```

</div>
</div>

<div class="callout">Výsledek generuje <code>livenessProbe</code>, <code>readinessProbe</code> i <code>startupProbe</code>. Zápis je kratší a méně chybový.</div>

---

# Probes: přesnější nastavení

<div class="code-card small-code">

```yaml
probes:
  http:
    path: /actuator/health
    port: "{{var:EXPOSE_PORT}}"
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

</div>

<div class="callout">Použijte tam, kde preset nestačí, ale stále nechceme opisovat celý Kubernetes probe objekt.</div>

---

# Scheduling: jak to bylo

<div class="split-title">
<div class="before small-code">
<h3>Dříve</h3>

```yaml
arch: amd64
node_selector:
  workload: tsm
tolerations:
  - key: dedicated
    operator: Equal
    value: tsm
    effect: NoSchedule
```

</div>
<div class="before small-code">
<h3>Omezení</h3>

```text
- žádné přirozené místo pro affinity
- žádné jednoduché topology spread
- scheduling pravidla jsou rozptýlená
- složitější rozšiřování metamodelu
```

</div>
</div>

---

# Scheduling: správný nový zápis

```yaml
scheduling:
  arch: amd64
  node_selector:
    workload: tsm
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

<div class="callout">Starý zápis zůstává platný. Nový blok <code>scheduling</code> je preferovaný pro nové změny.</div>

---

# Scheduling: raw alternativa pro affinity

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

<div class="grid-2">
<div class="card"><h3>Metamodel</h3><p>Běžný případ: architektura, tolerace, node selector, spread, self anti-affinity.</p></div>
<div class="card"><h3>Raw alternativa</h3><p>Speciální Kubernetes affinity pravidla bez rozšiřování DSL.</p></div>
</div>

---

# Mounts: jak to bylo

<div class="split-title small-code">
<div class="before">
<h3>Dříve</h3>

```yaml
assets:
  - file: assets/app.conf
    to: /app/app.conf
  - temp: true
    to: /app/cache
  - pvc: true
    name: data-claim
    to: /data
  - nfs-server: nfs.local
    path: /export/data
    to: /nfs
  - host-path: /var/lib/host-data
    to: /host
```

</div>
<div class="before">
<h3>Problém</h3>

```text
- jen první položka je skutečný asset
- temp/pvc/nfs/host-path jsou volumes
- klíč "to" má více významů
- host-path vypadá stejně bezpečně jako config asset
```

</div>
</div>

---

# Mounts: správný nový zápis

```yaml
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
```

<div class="callout">Nový zápis odděluje assety od volumes. Starý blok <code>assets</code> zůstává podporovaný kvůli kompatibilitě.</div>

---

# Mounts: NFS, host path a výjimečný raw zápis

<div class="split-title small-code">
<div class="now">
<h3>NFS a host path</h3>

```yaml
mounts:
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

</div>
<div class="now">
<h3>Výjimečný raw zápis</h3>

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

</div>
</div>

<div class="callout"><code>host_path</code> je bezpečnostně citlivý mechanismus. Používat jen při jasném provozním důvodu.</div>

---

# Rollout checksums

<div class="lead">Pokud změna konfigurace nemění jméno ConfigMap, ale má vyvolat rollout, použije se anotace nad pod template.</div>

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

<div class="callout">Změna obsahu uvedených souborů změní hodnotu anotace <code>checksum/...</code>, tím se změní pod template a Kubernetes provede rollout.</div>

---

# Bezpečnost a kompatibilita

<div class="grid-2">
<div class="card">
<h3>Kompatibilita</h3>
<ul>
<li>staré <code>health</code>, <code>probe</code>, <code>assets</code> fungují dál</li>
<li>migrace může probíhat postupně</li>
<li>Go verze drží parity testy nad reálnými vstupy</li>
</ul>
</div>
<div class="card">
<h3>Bezpečnost</h3>
<ul>
<li><code>host_path</code> je výjimka</li>
<li>EncJson zůstává externí bezpečnostní nástroj</li>
<li><code>raw</code> je řízená výjimka</li>
</ul>
</div>
</div>

---

# Doporučený postup pro L2 support

<div class="grid-3">
<div class="card"><div class="number">1</div><h3>Upravit model</h3><p>Preferovat nové bloky <code>probes</code>, <code>scheduling</code>, <code>mounts</code>.</p></div>
<div class="card"><div class="number">2</div><h3>Validovat</h3><p>Spustit <code>kube-build-app validate</code>.</p></div>
<div class="card"><div class="number">3</div><h3>Zkontrolovat dopad</h3><p>Použít <code>summary</code> a podle potřeby <code>inventory</code>.</p></div>
</div>

<div class="callout">Až poté commit a pipeline. Cílem je zachytit chybu dříve než ve fázi deploymentu.</div>

---

# Shrnutí

- Přechod na Go znamená jednodušší distribuci a provozní použití.
- Nové CLI je vhodnější pro L2 support i CI/CD.
- Metamodel je čitelnější a lépe rozlišuje běžné a speciální případy.
- Zpětná kompatibilita zůstává zachována.
- Doporučený směr pro nové změny: <code>probes</code>, <code>scheduling</code>, <code>mounts</code>.

<div class="callout">Výsledek: méně ruční Kubernetes YAML magie, více kontrolovatelného modelu a lepší provozní předvídatelnost.</div>

---

<!-- _class: dark -->
<!-- _paginate: false -->

<div class="brand"><img src="https://www.datalite.cz/assets/img/logo-light.png" alt="DataLite" /></div>

# Děkuji za pozornost

<div class="lead">Dotazy k přechodu na Go verzi, metamodelu a doporučenému způsobu práce v environment repozitářích.</div>

<div class="footer-note">kube-build-app · kube-edit-app · DataLite</div>
