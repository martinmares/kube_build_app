'use strict';
// Representative source values, never resolved or written to a repository.
const profile = {
  name: 'java-jib-service', defaults: {
    image: '<from release manifest>',
    startup: {command: ['/bin/sh'], arguments: ['/app/start-java.sh', '/app/jib-classpath-file', '/app/jib-main-class-file']},
    envs: [{name: 'SPRING_CONFIG_IMPORT', value: 'configserver:{{var:CONFIG_SERVER_URL}}'}],
    ports: [{name: 'http', port: '{{env:DEFAULT_EXPOSE_PORT}}', expose_as: [
      {hostname: '{{var:APP_NAME}}', port: '{{env:DEFAULT_HTTP_PORT}}'},
      {hostname: '{{var:APP_NAME}}-headless', port: '{{env:DEFAULT_EXPOSE_PORT}}', type: 'headless'}]}],
    probes: {http: {path: '/actuator/health', port: '{{env:DEFAULT_EXPOSE_PORT}}',}, live: {failure: 5, period: 10, timeout: 2}, ready: {failure: 2, period: 2, success: 2, timeout: 2}, start: {failure: 30, period: 10, timeout: 2}}
  }
};
const store = {
  profiles: [profile],
  variables: [
    {name: 'TOOLS_VERSION', value: '2026.09.10.1'},
    {name: 'CONFIG_SERVER_PROXY_LISTEN', value: '127.0.0.1:9999'},
    {name: 'CONFIG_SERVER_URL', value: 'http://127.0.0.1:9999/api/v1/tenants/default/envs/dev?fail-fast=true&max-attempts=20'},
    {name: 'CONFIG_SERVER_UPSTREAM_URL', value: 'https://config.example.test/simple-config-server'},
    {name: 'SIMPLE_CONFIG_TOKEN_NAME', value: 'simple-config'}
  ],
  sidecars: [
    {name: 'cgroup-runtime-exporter', defaults: {image: '{{env:DOCKER_HUB_URL}}/datalite/cgroup-runtime-exporter:{{var:TOOLS_VERSION}}', startup: {command: ['/usr/local/bin/cgroup-runtime-exporter']}, envs: [{name: 'CGROUP_EXPORTER_TARGET_PID_REGEXP', value: '(^|/)java(\\s|$)'}], resources: {cpu: {from: '2m', to: '10m'}, memory: {from: '8Mi', to: '32Mi'}}}},
    {name: 'simple-idm-token-proxy', defaults: {image: '{{env:DOCKER_HUB_URL}}/datalite/simple-idm-token-proxy:{{var:TOOLS_VERSION}}', startup: {command: ['simple-idm-token-proxy'], arguments: ['serve']}, envs: [{name: 'SIMPLE_IDM_TOKEN_PROXY_TOKEN_FILE', workload_identity_token_ref_name: '{{var:SIMPLE_CONFIG_TOKEN_NAME}}'}, {name: 'SIMPLE_IDM_TOKEN_PROXY_UPSTREAM', value: '{{var:CONFIG_SERVER_UPSTREAM_URL}}'}, {name: 'SIMPLE_IDM_TOKEN_PROXY_CA_FILE', shared_asset_ref_name: 'cetin-ca'}]}}
  ],
  assets: [{name: 'java-runtime-config', defaults: {files: [{source: 'files/ssl/tsm-client-truststore.jks', target: 'tsm-client-truststore.jks', mode: '0440'}, {source: 'files/ssl/tsm-client-keystore.jks', target: 'tsm-client-keystore.jks', mode: '0440'}]}}, {name: 'ui-runtime-config', defaults: {files: [{source: 'files/ui/config.json.tpl', target: 'config.json', mode: '0440'}]}}]
};
const clone = value => structuredClone(value);
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const $ = selector => document.querySelector(selector);
const sections = {profiles: 'Profily kontejnerů', variables: 'Proměnné', sidecars: 'Sidecary', assets: 'Runtime assety', identity: 'Identita', other: 'Ostatní nastavení'};
const tabs = {general: 'Základní', startup: 'Spouštění', envs: 'Proměnné kontejneru', resources: 'CPU a paměť', probes: 'Probes', ports: 'Porty a služby'};
let section = 'profiles', index = 0, tab = 'general', draft = clone(profile), baseline = clone(profile);
const get = path => path.split('.').reduce((v, k) => v?.[k], draft);
function set(path, value) {
  const parts = path.split('.');
  let parent = draft;
  const ancestors = [];
  for (const key of parts.slice(0, -1)) {
    if (value === undefined && parent[key] === undefined) return;
    ancestors.push([parent, key]);
    parent = parent[key] ??= {};
  }
  if (value === undefined) delete parent[parts.at(-1)]; else parent[parts.at(-1)] = value;
  // Removing an optional draft value must not leave newly created empty maps.
  for (let i = ancestors.length - 1; i >= 0; i--) {
    const [owner, key] = ancestors[i];
    const original = parts.slice(0, i + 1).reduce((v, k) => v?.[k], baseline);
    if (original === undefined && !Array.isArray(owner[key]) && Object.keys(owner[key]).length === 0) delete owner[key];
  }
}
function changed() {return JSON.stringify(draft) !== JSON.stringify(baseline);}
function input(path, label, options = {}) {
  const value = get(path);
  const present = value !== undefined;
  return `<input class="form-control form-control-sm font-monospace" aria-label="${esc(label)}" data-path="${esc(path)}" value="${esc(value)}" ${options.numeric ? 'data-numeric inputmode="numeric"' : ''} ${options.optional ? 'data-optional' : ''} ${options.placeholder ? `placeholder="${esc(options.placeholder)}"` : ''} data-present="${present}">`;
}
function field(path, label, options = {}) {return `<div class="mb-3 field-width"><label class="form-label">${esc(label)}</label>${input(path,label,options)}</div>`;}
function table(headers, rows) {return `<div class="table-responsive"><table class="table table-sm table-vcenter"><thead><tr>${headers.map(h=>`<th>${h}</th>`).join('')}</tr></thead><tbody>${rows}</tbody></table></div>`;}
function button(action, path, text, extra = '') {return `<button type="button" class="btn btn-sm btn-outline-secondary" data-action="${action}" data-target="${path}" ${extra}>${text}</button>`;}
function remove(path, i) {return button('remove', path, 'Odebrat', `data-index="${i}" aria-label="Odebrat řádek ${i+1}"`);}
function list(path, title) {
  const value = get(path);
  const enabled = Array.isArray(value);
  return `<div class="mb-4 field-width"><label class="form-check"><input class="form-check-input" type="checkbox" data-list="${path}" ${enabled?'checked':''}><span class="form-check-label fw-medium">${title}</span></label>
  ${enabled ? `${table(['Pořadí','Hodnota',''], value.map((v,i)=>`<tr><td class="text-secondary">${i+1}</td><td>${input(`${path}.${i}`,`${title} ${i+1}`)}</td><td>${remove(path,i)}</td></tr>`).join(''))}${button('add-string',path,'Přidat položku')}${value.length?'':'<span class="text-secondary small ms-2">Explicitně prázdný seznam.</span>'}` : '<div class="text-secondary small">Není nastaveno; použije se chování image nebo dalších vrstev konfigurace.</div>'}</div>`;
}
function general() {
  const release = get('defaults.image') === '<from release manifest>';
  return `${field('name', 'Název profilu')}<div class="field-width"><label class="form-label">Image</label><div class="d-flex flex-wrap align-items-center gap-3 mb-2">
  <label class="form-check mb-0"><input class="form-check-input" type="radio" name="image-mode" data-mode="release" ${release?'checked':''}><span class="form-check-label">Z release manifestu</span></label>
  <label class="form-check mb-0"><input class="form-check-input" type="radio" name="image-mode" data-mode="explicit" ${release?'':'checked'}><span class="form-check-label">Vlastní image / šablona</span></label></div>
  ${release?'<div class="text-secondary small">Konkrétní image se vybere při buildu pro danou aplikaci.</div>':input('defaults.image','Image',{placeholder:'registry.example/app:tag'})}</div>
  ${section==='profiles'?'<details class="mt-4"><summary class="text-secondary">Používá 15 kontejnerů · ukázkový seznam</summary><div class="small mt-2">tsm-calendar, tsm-catalog, tsm-address-management, tsm-gateway, tsm-dms a další aplikace.</div></details>':''}`;
}
const envKinds = {value:'Hodnota',secret_name:'Secret',field_path:'Pole podu',resource_name:'Resource',workload_identity_token_ref_name:'Token identity',shared_asset_ref_name:'Sdílený asset',remove:'Odebrat zděděnou proměnnou'};
function envs() {
  const items = get('defaults.envs') || [];
  return `<p class="text-secondary small">Hodnoty se ukládají tak, jak je zadáte, včetně šablon.</p>${table(['Název','Zdroj','Hodnota / reference',''],items.map((item,i)=>{
    const kind = Object.keys(envKinds).find(k=>Object.hasOwn(item,k)) || 'value';
    return `<tr><td>${input(`defaults.envs.${i}.name`,'Název proměnné')}</td><td><select class="form-select form-select-sm" data-env-kind="${i}" aria-label="Zdroj proměnné">${Object.entries(envKinds).map(([k,v])=>`<option value="${k}" ${k===kind?'selected':''}>${v}</option>`).join('')}</select></td><td>${kind==='remove'?'<span class="text-secondary">Odebrat</span>':input(`defaults.envs.${i}.${kind}`,'Hodnota nebo reference')}${kind==='secret_name'?`<div class="mt-1">${input(`defaults.envs.${i}.key`,'Klíč secretu',{placeholder:'Klíč secretu'})}</div>`:''}</td><td>${remove('defaults.envs',i)}</td></tr>`;
  }).join(''))}${button('add-env','defaults.envs','Přidat proměnnou')}`;
}
function resources() {
  return `<p class="text-secondary small">Prázdné pole ponechá hodnotu nenastavenou. Povolené jsou také celé šablony.</p>${table(['Zdroj','Request','Limit'],[['cpu','CPU','100m','500m'],['memory','Paměť','256Mi','1Gi']].map(([key,label,req,lim])=>`<tr><td>${label}</td><td>${input(`defaults.resources.${key}.from`,`${label} request`,{optional:true,placeholder:req})}</td><td>${input(`defaults.resources.${key}.to`,`${label} limit`,{optional:true,placeholder:lim})}</td></tr>`).join(''))}`;
}
function probes() {
  return `<div class="row"><div class="col-12 col-lg-8">${field('defaults.probes.http.path','Společná HTTP cesta',{optional:true,placeholder:'/actuator/health'})}</div><div class="col-12 col-lg-4">${field('defaults.probes.http.port','HTTP port',{optional:true,numeric:true,placeholder:'8080 nebo {{env:DEFAULT_EXPOSE_PORT}}'})}</div></div>
  ${table(['Kontrola','Delay (s)','Period (s)','Timeout (s)','Success','Failure'],[['live','Liveness'],['ready','Readiness'],['start','Startup']].map(([key,label])=>`<tr><td class="fw-medium">${label}</td>${['delay','period','timeout','success','failure'].map(f=>`<td>${input(`defaults.probes.${key}.${f}`,`${label} ${f}`,{optional:true,numeric:true})}</td>`).join('')}</tr>`).join(''))}
  <p class="text-secondary small">Prázdné pole použije výchozí chování builderu. Startup probe chrání pomalý start aplikace.</p>`;
}
function ports() {
  return `${(get('defaults.ports')||[]).map((port,i)=>`<section class="mb-4 pb-3 border-bottom"><div class="d-flex align-items-center justify-content-between mb-2"><h3 class="h4 mb-0">Port ${i+1}</h3>${remove('defaults.ports',i)}</div><div class="row"><div class="col-12 col-md-6">${field(`defaults.ports.${i}.name`,'Název portu')}</div><div class="col-12 col-md-6">${field(`defaults.ports.${i}.port`,'Port kontejneru',{numeric:true})}</div></div>
  <h4 class="h5">Services</h4>${table(['Název služby','Port služby','Typ',''],(port.expose_as||[]).map((s,j)=>`<tr><td>${input(`defaults.ports.${i}.expose_as.${j}.hostname`,'Název služby')}</td><td>${input(`defaults.ports.${i}.expose_as.${j}.port`,'Port služby',{numeric:true})}</td><td><select class="form-select form-select-sm" data-path="defaults.ports.${i}.expose_as.${j}.type" data-optional aria-label="Typ služby"><option value="">Standardní</option><option value="headless" ${s.type==='headless'?'selected':''}>Headless</option></select></td><td>${remove(`defaults.ports.${i}.expose_as`,j)}</td></tr>`).join(''))}${button('add-service',`defaults.ports.${i}.expose_as`,'Přidat službu')}
  <p class="text-secondary small mt-3 mb-0">Routes a Ingresses: navazující návrh detailu služby. V této ukázce nejsou editovatelné.</p></section>`).join('')}${button('add-port','defaults.ports','Přidat port')}`;
}
function variables() {
  return `${table(['Název','Hodnota',''],draft.map((v,i)=>`<tr><td>${input(`${i}.name`,'Název proměnné')}</td><td>${input(`${i}.value`,'Hodnota proměnné')}</td><td>${remove('',i)}</td></tr>`).join(''))}${button('add-variable','','Přidat proměnnou')}`;
}
function files() {
  return `<p class="text-secondary small">Připojené soubory ze společného runtime zdroje. Nastavení zdroje bude v samostatné části Runtime asset defaults.</p>${table(['Zdroj','Cíl','Mode',''],(get('defaults.files')||[]).map((v,i)=>`<tr>${['source','target','mode'].map(key=>`<td>${input(`defaults.files.${i}.${key}`,key)}</td>`).join('')}<td>${remove('defaults.files',i)}</td></tr>`).join(''))}${button('add-file','defaults.files','Přidat soubor')}`;
}
function diff(before, after, path = '') {
  if (JSON.stringify(before) === JSON.stringify(after)) return [];
  if (before && after && typeof before==='object' && typeof after==='object') return [...new Set([...Object.keys(before),...Object.keys(after)])].flatMap(k=>diff(before[k],after[k],path?`${path}.${k}`:k));
  return [{path, before, after}];
}
function status() {
  const dirty = changed();
  $('#dirty').textContent = dirty ? 'Neuložené změny' : 'Bez změn';
  $('#save').disabled = !dirty;
  $('#cancel').disabled = !dirty;
  $('#changes').innerHTML = diff(baseline,draft).map(d=>`<div class="border-bottom py-2"><code>${esc(d.path)}</code><div class="text-danger text-break">− ${esc(d.before===undefined?'(nenastaveno)':JSON.stringify(d.before))}</div><div class="text-success text-break">+ ${esc(d.after===undefined?'(odebráno)':JSON.stringify(d.after))}</div></div>`).join('') || '<span class="text-secondary small">Zatím žádné změny.</span>';
}
function render() {
  $('#sections').innerHTML = Object.entries(sections).map(([key,label])=>`<button class="nav-link text-start ${key===section?'active':''}" data-section="${key}">${label}</button>`).join('');
  const supported = ['profiles','sidecars','assets','variables'].includes(section);
  $('#selection').innerHTML = supported && section!=='variables' ? `<label class="form-label" for="selected">${section==='profiles'?'Profil':section==='sidecars'?'Sidecar':'Runtime asset'}</label><select id="selected" class="form-select field-width">${store[section].map((item,i)=>`<option value="${i}" ${index===i?'selected':''}>${esc(item.name)}</option>`).join('')}</select>` : '';
  $('#heading').innerHTML = `<h2 class="card-title">${esc(supported && section!=='variables' ? draft.name : sections[section])}</h2>`;
  $('#tabs').hidden = !['profiles','sidecars'].includes(section);
  $('#tabs').innerHTML = ['profiles','sidecars'].includes(section)?`<nav class="nav nav-tabs border-0 flex-wrap" aria-label="Nastavení profilu">${Object.entries(tabs).map(([key,label])=>`<button class="nav-link ${key===tab?'active':''}" type="button" data-tab="${key}">${label}</button>`).join('')}</nav>`:'';
  $('#editor').innerHTML = !supported ? '<p class="text-secondary mb-0">Tato část je zatím pouze umístěna v navigaci. Formulář vznikne po ověření editoru profilu.</p>' : section==='variables'?variables():section==='assets'?files():({general,startup:()=>`${list('defaults.startup.command','Command')}${list('defaults.startup.arguments','Arguments')}`,envs,resources,probes,ports}[tab])();
  $('#save').textContent = section==='variables'?'Uložit proměnné':section==='sidecars'?'Uložit sidecar':section==='assets'?'Uložit asset':'Uložit profil';
  $('#save').parentElement.parentElement.hidden = !supported;
  $('#review').hidden = !supported;
  status();
}
function choose(nextSection, nextIndex = 0) {
  if (changed() && !window.confirm('Zahodit neuložené změny a přejít jinam?')) {render(); return;}
  section=nextSection; index=nextIndex; tab='general';
  baseline=clone(section==='variables'?store.variables:store[section]?.[index] || {}); draft=clone(baseline);
  $('#notice').innerHTML=''; render();
}
document.addEventListener('input', e=>{
  const el=e.target;
  if (!el.hasAttribute('data-path')) return;
  let value=el.value;
  if (el.hasAttribute('data-numeric') && /^\d+$/.test(value)) value=Number(value);
  if (el.hasAttribute('data-optional') && value==='') value=undefined;
  set(el.dataset.path,value); status();
});
document.addEventListener('change', e=>{
  const el=e.target;
  if(el.id==='selected') choose(section,Number(el.value));
  if(el.dataset.mode) {set('defaults.image',el.dataset.mode==='release'?'<from release manifest>':''); render();}
  if(el.dataset.list) {set(el.dataset.list,el.checked?[]:undefined); render();}
  if(el.dataset.envKind!==undefined) {const i=Number(el.dataset.envKind), kind=el.value, old=get(`defaults.envs.${i}`); set(`defaults.envs.${i}`,{name:old.name,[kind]:kind==='remove'?true:'',...(kind==='secret_name'?{key:''}:{})}); render();}
});
document.addEventListener('click', e=>{
  const el=e.target.closest('button'); if(!el)return;
  if(el.dataset.section)choose(el.dataset.section);
  if(el.dataset.tab){tab=el.dataset.tab;render();}
  if(el.id==='theme')document.documentElement.dataset.bsTheme=document.documentElement.dataset.bsTheme==='dark'?'light':'dark';
  if(el.id==='cancel'){draft=clone(baseline);$('#notice').innerHTML='';render();}
  if(el.id==='save') {
    if(section!=='variables' && !draft.name?.trim()) {$('#notice').innerHTML='<div class="alert alert-danger">Vyplňte název.</div>';return;}
    if(section==='variables')store.variables=clone(draft);else store[section][index]=clone(draft);
    baseline=clone(draft);$('#notice').innerHTML='<div class="alert alert-success py-2">Uloženo v ukázce. Žádný YAML soubor nebyl změněn; obnovení stránky vrátí výchozí data.</div>';render();
  }
  if(el.dataset.action) {
    const path=el.dataset.target; const values=path?get(path)||[]:draft;
    if(el.dataset.action==='remove')values.splice(Number(el.dataset.index),1);
    else values.push({'add-string':'','add-env':{name:'',value:''},'add-variable':{name:'',value:''},'add-port':{name:'',port:'',expose_as:[]},'add-service':{hostname:'',port:''},'add-file':{source:'',target:'',mode:'0440'}}[el.dataset.action]);
    if(path)set(path,values);render();
  }
});
$('#editor').addEventListener('submit',e=>e.preventDefault());
window.addEventListener('beforeunload',e=>{if(changed()){e.preventDefault();e.returnValue='';}});
render();
