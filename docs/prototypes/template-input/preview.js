'use strict';

// Finite preview fixtures, deliberately separate from the production resolver.
const variables = [
  {scope:'var',name:'DEFAULT_SERVICE_PORT',source:'apps/_defaults.yml > vars',values:{'java-api':'8080',worker:'9090'}},
  {scope:'var',name:'APP_NAME',source:'Application > vars',values:{'java-api':'java-api',worker:'worker'}},
  {scope:'env',name:'PUBLIC_SERVICE_PORT',source:'Build environment',value:'443'},
  {scope:'env',name:'TSM_REGISTRY_URL',source:'Build environment',value:'registry.example'},
  {scope:'env',name:'TSM_RELEASE_ID',source:'Build environment',value:'2026.20.5.01'},
];
const pattern = /\{\{(?:(var|env):)?([A-Za-z_][A-Za-z0-9_]*)\}\}/g;
const escapeHTML = value => String(value).replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[char]);

// tokenize() only describes source for the UI. It never resolves values.
function tokenize(source) {
  const parts=[]; let cursor=0;
  for (const match of source.matchAll(pattern)) {
    if (match.index > cursor) parts.push({type:'text',value:source.slice(cursor,match.index)});
    parts.push({type:match[1]||'passthrough',name:match[2],value:match[0]});
    cursor=match.index+match[0].length;
  }
  if (cursor < source.length) parts.push({type:'text',value:source.slice(cursor)});
  return parts;
}
const hasTemplate = source => tokenize(source).some(part => part.type !== 'text');
function renderParts(source) {
  const color={var:'blue',env:'cyan',passthrough:'yellow'};
  return tokenize(source).map(part => part.type === 'text'
    ? `<span class="template-text font-monospace">${escapeHTML(part.value)}</span>`
    : `<span class="badge bg-${color[part.type]}-lt template-token font-monospace">${escapeHTML(part.value)}</span>`).join('');
}

// resolveSample() is an optional demonstration adapter, not part of the component model.
function resolveSample(source,context) {
  const result={value:'',issues:[]};
  for (const part of tokenize(source)) {
    if (part.type === 'text') { result.value+=part.value; continue; }
    if (part.type === 'passthrough') { result.value+=part.value; result.issues.push(`passthrough ${part.name}`); continue; }
    const variable=variables.find(item => item.scope===part.type && item.name===part.name);
    const value=variable && (variable.values ? variable.values[context] : variable.value);
    if (value === undefined || value === '') { result.value+=part.value; result.issues.push(`missing ${part.type}:${part.name}`); }
    else result.value+=value;
  }
  return result;
}

class TemplateInput {
  constructor(root) {
    this.root=root; this.field=root.dataset.field; this.label=root.dataset.label;
    this.source=root.dataset.value; this.pickerMode=root.dataset.pickerMode;
    this.rawMode=!hasTemplate(this.source); this.pending=false; this.selection=null; this.render();
  }
  render() {
    const group=`mode-${this.field}`;
    this.root.innerHTML=`
      <label class="form-label">${escapeHTML(this.label)}</label>
      <fieldset class="border-0 p-0 mb-2"><legend class="visually-hidden">Value type</legend><div class="d-flex gap-3">
        <label class="form-check mb-0"><input class="form-check-input" type="radio" name="${group}" value="template" ${this.rawMode?'':'checked'}><span class="form-check-label">Template</span></label>
        <label class="form-check mb-0"><input class="form-check-input" type="radio" name="${group}" value="raw" ${this.rawMode?'checked':''}><span class="form-check-label">Raw value</span></label>
      </div></fieldset>
      <div data-template-view ${this.rawMode?'hidden':''}><div class="template-parts form-control d-flex flex-wrap align-items-center gap-1" role="textbox" aria-readonly="true">${renderParts(this.source)}</div><div class="form-hint">Read-only source composition. Switch to Raw value to edit it.</div></div>
      <div data-raw-view ${this.rawMode?'':'hidden'}><div class="d-flex gap-2 align-items-start"><input class="form-control font-monospace" data-raw value="${escapeHTML(this.source)}" autocomplete="off" spellcheck="false"><button class="btn btn-outline-primary text-nowrap" data-open type="button">${this.pickerMode==='replace'?'Choose variable':'Insert variable'}</button></div><div class="form-hint">${this.pickerMode==='replace'?'Selecting a variable replaces the complete value.':'A selected variable is inserted at the cursor.'}</div></div>
      <section class="card card-sm mt-2 shadow-sm" data-picker hidden><div class="card-header py-2"><h3 class="card-title small">Choose a variable</h3><button class="btn-close ms-auto" data-close type="button" aria-label="Close variable picker"></button></div><div class="card-body pb-2"><input type="search" class="form-control form-control-sm" data-search placeholder="Search name or source..." autocomplete="off"></div><div class="list-group list-group-flush variable-list" data-list></div></section>
      <div class="mt-3 small" data-status role="status" aria-live="polite"></div>
      <details class="mt-2"><summary class="text-secondary small">Source value</summary><pre class="mt-2 mb-0"><code data-source></code></pre></details>`;
    this.bind(); this.evaluate();
  }
  bind() {
    this.root.querySelector('[value="template"]').addEventListener('change',()=>{
      if (hasTemplate(this.source)) { this.rawMode=false; this.render(); }
      else { this.pending=true; this.openPicker(); }
    });
    this.root.querySelector('[value="raw"]').addEventListener('change',()=>{ this.rawMode=true; this.pending=false; this.render(); this.root.querySelector('[data-raw]').focus(); });
    this.root.querySelector('[data-raw]')?.addEventListener('input',event=>{ this.source=event.target.value; this.evaluate(); });
    const open=this.root.querySelector('[data-open]');
    open?.addEventListener('pointerdown',()=>this.rememberSelection());
    open?.addEventListener('click',()=>{ this.rememberSelection(); this.openPicker(); });
    this.root.querySelector('[data-close]').addEventListener('click',()=>this.closePicker());
    this.root.querySelector('[data-search]').addEventListener('input',()=>this.renderVariables());
    this.root.querySelector('[data-picker]').addEventListener('keydown',event=>{ if(event.key==='Escape'){event.preventDefault();this.closePicker();} });
    this.root.querySelector('[data-list]').addEventListener('click',event=>{ const button=event.target.closest('[data-reference]'); if(button)this.selectVariable(button.dataset.reference); });
  }
  openPicker() { this.root.querySelector('[data-picker]').hidden=false; this.renderVariables(); this.root.querySelector('[data-search]').focus(); }
  rememberSelection() {
    const raw=this.root.querySelector('[data-raw]');
    if(raw&&document.activeElement===raw)this.selection={start:raw.selectionStart??this.source.length,end:raw.selectionEnd??this.source.length};
  }
  closePicker() {
    if (this.pending) { this.pending=false; this.rawMode=true; this.render(); return; }
    this.root.querySelector('[data-picker]').hidden=true; this.root.querySelector('[data-open]')?.focus();
  }
  renderVariables() {
    const query=this.root.querySelector('[data-search]').value.toLowerCase().trim();
    const rows=variables.filter(item=>`${item.scope}:${item.name} ${item.source}`.toLowerCase().includes(query));
    this.root.querySelector('[data-list]').innerHTML=rows.map(item=>{const ref=`{{${item.scope}:${item.name}}}`;return `<button type="button" class="list-group-item list-group-item-action text-start py-2" data-reference="${escapeHTML(ref)}"><span class="badge bg-${item.scope==='var'?'blue':'cyan'}-lt me-2">${item.scope}</span><strong>${escapeHTML(item.name)}</strong><span class="d-block text-secondary small mt-1">${escapeHTML(item.source)}</span></button>`;}).join('')||'<div class="p-3 text-secondary small">No matching sample variables.</div>';
  }
  selectVariable(reference) {
    const raw=this.root.querySelector('[data-raw]');
    if (this.pickerMode==='replace'||!raw) this.source=reference;
    else { const range=this.selection||{start:this.source.length,end:this.source.length}; this.source=this.source.slice(0,range.start)+reference+this.source.slice(range.end); }
    this.pending=false; this.selection=null; this.rawMode=false; this.render();
  }
  setSource(source) { this.source=source; this.rawMode=!hasTemplate(source); this.pending=false; this.render(); }
  evaluate() {
    this.root.querySelector('[data-source]').textContent=`${this.field}: ${JSON.stringify(this.source)}`;
    const status=this.root.querySelector('[data-status]'); const context=document.querySelector('#context').value;
    if (!hasTemplate(this.source)) {
      const valid=this.field!=='port'||(/^\d+$/.test(this.source)&&Number(this.source)>=1&&Number(this.source)<=65535);
      status.innerHTML=`<span class="badge bg-${valid?'green':'red'}-lt">${valid?'Raw value':'Invalid port'}</span> <span class="ms-1">${valid?'No template references.':'Port must be an integer from 1 to 65535.'}</span>`; return;
    }
    if (!context) { status.innerHTML='<span class="badge bg-yellow-lt">Not verified</span> <span class="ms-1">Choose an application for the finite sample preview.</span>'; return; }
    const resolved=resolveSample(this.source,context);
    if(this.field==='port'&&!resolved.issues.length&&!(/^\d+$/.test(resolved.value)&&Number(resolved.value)>=1&&Number(resolved.value)<=65535))resolved.issues.push('result is not a valid port');
    status.innerHTML=`<div><span class="badge bg-${resolved.issues.length?'yellow':'green'}-lt">${resolved.issues.length?'Partially resolved':'Sample resolved'}</span> <code class="ms-1">${escapeHTML(resolved.value)}</code></div>${resolved.issues.length?`<div class="text-secondary mt-1">${escapeHTML(resolved.issues.join('; '))}</div>`:''}`;
  }
}

const editors=new Map([...document.querySelectorAll('[data-template-input]')].map(root=>{const editor=new TemplateInput(root);return[editor.field,editor];}));
document.querySelector('#context').addEventListener('change',()=>editors.forEach(editor=>editor.evaluate()));
document.querySelector('#scenarios').addEventListener('click',event=>{const button=event.target.closest('[data-field]');if(button)editors.get(button.dataset.field).setSource(button.dataset.value);});
document.querySelector('#theme').addEventListener('click',event=>{const dark=document.documentElement.dataset.bsTheme!=='dark';document.documentElement.dataset.bsTheme=dark?'dark':'light';event.target.textContent=dark?'Switch to light':'Switch to dark';});
