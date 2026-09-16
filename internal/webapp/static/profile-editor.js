// Source-only profile draft. No effective values are materialized into YAML.
const profileEditor = (() => {
  const clone = value => structuredClone(value);
  let current = null, draft = null, baseline = null, tab = 'general', saving = false, message = '', request = 0, explicitImage = '';
  const resourceSteps = {cpu: 10, memory: 64};
  const tabs = {general:'General',startup:'Startup',envs:'Environment',resources:'CPU / memory',probes:'Probes',ports:'Ports / services'};
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
  return `<input class="form-control form-control-sm font-monospace" aria-label="${esc(label)}" data-profile-path="${esc(path)}" value="${esc(value)}" ${options.numeric ? 'data-numeric inputmode="numeric"' : ''} ${options.optional ? 'data-optional' : ''} ${options.placeholder ? `placeholder="${esc(options.placeholder)}"` : ''} data-present="${present}">`;
}
function field(path, label, options = {}) {return `<div class="mb-3 field-width"><label class="form-label">${esc(label)}</label>${input(path,label,options)}</div>`;}
function templateField(path, label, kind) {return `<div data-profile-template-input data-template-path="${esc(path)}" data-template-label="${esc(label)}" data-template-kind="${esc(kind)}"></div>`;}
function table(headers, rows) {return `<div class="table-responsive"><table class="table table-sm table-vcenter"><thead><tr>${headers.map(h=>`<th>${h}</th>`).join('')}</tr></thead><tbody>${rows}</tbody></table></div>`;}
function button(action, path, text, extra = '') {return `<button type="button" class="btn btn-sm btn-outline-secondary" data-profile-action="${action}" data-target="${path}" ${extra}>${text}</button>`;}
function remove(path, i) {return button('remove', path, 'Remove', `data-index="${i}" aria-label="Remove row ${i+1}"`);}
function list(path, title) {
  const value = get(path);
  const enabled = Array.isArray(value);
  return `<div class="mb-4 field-width"><label class="form-check"><input class="form-check-input" type="checkbox" data-profile-list="${path}" ${enabled?'checked':''}><span class="form-check-label fw-medium">${title}</span></label>
  ${enabled ? `${table(['Order','Value',''], value.map((v,i)=>`<tr><td class="text-secondary">${i+1}</td><td>${input(`${path}.${i}`,`${title} ${i+1}`)}</td><td>${button('move-up',path,'<i class="ti ti-arrow-up"></i>',`data-index="${i}" aria-label="Move ${title} ${i+1} up" ${i===0?'disabled':''}`)} ${button('move-down',path,'<i class="ti ti-arrow-down"></i>',`data-index="${i}" aria-label="Move ${title} ${i+1} down" ${i===value.length-1?'disabled':''}`)} ${remove(path,i)}</td></tr>`).join(''))}${button('add-string',path,'Add item')}${value.length?'':'<span class="text-secondary small ms-2">Explicitly empty list.</span>'}` : '<div class="text-secondary small">Not set. Uses the image or other configuration layers.</div>'}</div>`;
}
function general() {
  const release = get('defaults.image') === '<from release manifest>';
  return `<div class="mb-3 text-secondary small">Edit shared defaults. The profile name is fixed; references remain unchanged.</div><div class="field-width"><label class="form-label">Image</label><div class="d-flex flex-wrap align-items-center gap-3 mb-2">
  <label class="form-check mb-0"><input class="form-check-input" type="radio" name="image-mode" data-profile-mode="release" ${release?'checked':''}><span class="form-check-label">From release manifest</span></label>
  <label class="form-check mb-0"><input class="form-check-input" type="radio" name="image-mode" data-profile-mode="explicit" ${release?'':'checked'}><span class="form-check-label">Explicit image / template</span></label></div>
  ${release?'<div class="text-secondary small">The build selects the image for each application and container.</div>':input('defaults.image','Image',{placeholder:'registry.example/app:tag'})}</div>
  `;
}
const envKinds = {value:'Value',secret_name:'Secret',field_path:'Pod field',resource_name:'Resource',workload_identity_token_ref_name:'Identity token',shared_asset_ref_name:'Shared asset',remove:'Remove inherited variable'};
function envs() {
  const items = get('defaults.envs') || [];
  return `<p class="text-secondary small">Source values and templates are preserved.</p>${table(['Name','Source','Value / reference',''],items.map((item,i)=>{
    const kind = Object.keys(envKinds).find(k=>Object.hasOwn(item,k)) || 'value';
    return `<tr><td>${input(`defaults.envs.${i}.name`,'Variable name')}</td><td><select class="form-select form-select-sm" data-profile-env-kind="${i}" aria-label="Value source">${Object.entries(envKinds).map(([k,v])=>`<option value="${k}" ${k===kind?'selected':''}>${v}</option>`).join('')}</select></td><td>${kind==='remove'?'<span class="text-secondary">Remove</span>':input(`defaults.envs.${i}.${kind}`,'Value or reference')}${kind==='secret_name'?`<div class="mt-1">${input(`defaults.envs.${i}.key`,'Secret key',{placeholder:'Secret key'})}</div>`:''}</td><td>${remove('defaults.envs',i)}</td></tr>`;
  }).join(''))}${button('add-env','defaults.envs','Add variable')}`;
}
function resourceControl(kind, field, label) {
  const values = get('defaults.resources.' + kind) || {};
  const alias = field === 'requests' ? 'from' : 'to';
  const path = 'defaults.resources.' + kind + '.' + (Object.hasOwn(values, field) ? field : alias);
  return `<div class="input-group input-group-sm flex-nowrap" data-resource-control="${kind}">
    <button type="button" class="btn btn-outline-secondary" data-resource-direction="-1" data-resource-path="${path}" aria-label="Decrease ${label}">−</button>
    ${input(path, label, {optional: true, placeholder: 'Not set'})}
    <button type="button" class="btn btn-outline-secondary" data-resource-direction="1" data-resource-path="${path}" aria-label="Increase ${label}">+</button>
  </div>`;
}
function resources() {
  return `<p class="text-secondary small">Empty means not set. Step controls change literal values only; templates stay untouched.</p>${table(['Resource','Request','Limit','Step'],[['cpu','CPU',[1,10,50,100,250],'m'],['memory','Memory',[1,16,64,128,256],'Mi']].map(([kind,label,steps,unit])=>`<tr><td>${label}</td><td>${resourceControl(kind,'requests',label+' request')}</td><td>${resourceControl(kind,'limits',label+' limit')}</td><td><select class="form-select form-select-sm" data-resource-step="${kind}" aria-label="${label} step">${steps.map(step=>`<option value="${step}" ${resourceSteps[kind]===step?'selected':''}>${step}${unit}</option>`).join('')}</select></td></tr>`).join(''))}
  <div class="text-secondary small">CPU steps use millicores (m); memory steps use Mi. Other formats remain editable as text. Changing the step does not change the profile.</div>`;
}
function syncResourceControls() {
  for (const control of qsa('#profile-workspace [data-resource-control]')) {
    const kind = control.dataset.resourceControl;
    for (const button of control.querySelectorAll('[data-resource-direction]')) {
      button.disabled = state.readOnly || saving || resourceStepper.next(kind, get(button.dataset.resourcePath), resourceSteps[kind], Number(button.dataset.resourceDirection)) === null;
      button.title = button.disabled ? 'Cannot step this value without changing its meaning. Edit it as text.' : '';
    }
  }
}
function probes() {
  return `<div class="row"><div class="col-12 col-lg-8">${field('defaults.probes.http.path','Shared HTTP path',{optional:true,placeholder:'/actuator/health'})}</div><div class="col-12 col-lg-4">${field('defaults.probes.http.port','HTTP port',{optional:true,numeric:true,placeholder:'8080 or {{env:DEFAULT_EXPOSE_PORT}}'})}</div></div>
  ${table(['Check','Delay (s)','Period (s)','Timeout (s)','Success','Failure'],[['live','Liveness'],['ready','Readiness'],['start','Startup']].map(([key,label])=>`<tr><td class="fw-medium">${label}</td>${['delay','period','timeout','success','failure'].map(f=>`<td>${input(`defaults.probes.${key}.${f}`,`${label} ${f}`,{optional:true,numeric:true})}</td>`).join('')}</tr>`).join(''))}
  <p class="text-secondary small">Empty fields use builder defaults. Startup probes protect slow-starting applications.</p>`;
}
function ports() {
  return `${(get('defaults.ports')||[]).map((port,i)=>`<section class="mb-4 pb-3 border-bottom"><div class="d-flex align-items-center justify-content-between mb-2"><h3 class="h4 mb-0">Port ${i+1}</h3>${remove('defaults.ports',i)}</div><div class="row"><div class="col-12 col-md-6">${field(`defaults.ports.${i}.name`,'Port name')}</div><div class="col-12 col-md-6">${field(`defaults.ports.${i}.port`,'Container port',{numeric:true})}</div></div>
  <h4 class="h5">Services</h4>${table(['Service name','Service port','Type',''],(port.expose_as||[]).map((s,j)=>`<tr><td>${input(`defaults.ports.${i}.expose_as.${j}.${Object.hasOwn(s,'service_name')?'service_name':'hostname'}`,'Service name')}</td><td>${templateField(`defaults.ports.${i}.expose_as.${j}.port`,'Service port','port')}</td><td><select class="form-select form-select-sm" data-profile-path="defaults.ports.${i}.expose_as.${j}.type" data-optional aria-label="Service type"><option value="">Standard</option><option value="headless" ${s.type==='headless'?'selected':''}>Headless</option></select></td><td>${remove(`defaults.ports.${i}.expose_as`,j)}</td></tr>`).join(''))}${button('add-service',`defaults.ports.${i}.expose_as`,'Add service')}
  <p class="text-secondary small mt-3 mb-0">Existing external routes and ingresses are preserved. Their editor is not part of this step.</p></section>`).join('')}${button('add-port','defaults.ports','Add port')}`;
}

function mountTemplateFields() {
  const references = state.inspection?.template_references || [];
  for (const host of qsa('#profile-workspace [data-profile-template-input]')) {
    const path = host.dataset.templatePath;
    templateInput.mount(host, {
      id: path,
      label: host.dataset.templateLabel,
      kind: host.dataset.templateKind,
      source: get(path),
      references,
      pickerMode: 'replace',
      disabled: state.readOnly || saving,
      onChange(value) {
        set(path, /^\d+$/.test(value) ? Number(value) : value);
        message = '';
        qs('[data-profile-feedback]').innerHTML = '';
        status();
      },
    });
  }
}

function active() { return current && current.env === state.env; }
function status() {
 const host=qs('#profile-workspace'); if(!host)return;
 host.querySelector('[data-profile-save]').disabled = state.readOnly || saving || !changed();
 host.querySelector('[data-profile-cancel]').disabled = saving || !changed();
 host.querySelector('[data-profile-status]').classList.toggle('d-none', !!message && !saving);
 syncResourceControls();
 host.querySelector('[data-profile-status]').textContent = saving ? 'Saving...' : changed() ? 'Unsaved changes' : 'No changes';
}
function render() {
 if(!active())return;
 qs('#defaults-filter')?.closest('.row')?.classList.add('d-none');
 const names=(state.inspection?.defaults?.model?.container_profiles || []).map(p=>p.name);
 const usages=(state.inspection?.usage || []).filter(u=>u.kind==='container_profile' && u.name===current.name);
 const extra=Object.keys(draft.defaults).filter(k=>!['image','startup','envs','resources','probes','ports'].includes(k));
 let body;
 try { body=({general,startup:()=>list('defaults.startup.command','Command')+list('defaults.startup.arguments','Arguments'),envs,resources,probes,ports}[tab])(); }
 catch (_) {body='<div class="alert alert-warning">This section uses a source structure not supported by this form. It will be preserved unchanged. Use source editing for this section.</div>';}
 const supported={startup:['command','arguments'],resources:['cpu','memory'],probes:['http','live','ready','start']};
 if (supported[tab]) {const value=get('defaults.'+tab)||{}; const additional=Object.keys(value).filter(k=>!supported[tab].includes(k)); if(additional.length) body+=`<div class="text-secondary small mt-3">Additional settings are preserved: ${esc(additional.join(', '))}.</div>`;}
 setHTML('#defaults-overview', `<div id="profile-workspace" style="max-width:960px">
 <nav aria-label="Breadcrumb" class="mb-3"><ol class="breadcrumb mb-0"><li class="breadcrumb-item"><a href="#defaults" data-profile-back>Defaults</a></li><li class="breadcrumb-item">Container profiles</li><li class="breadcrumb-item active text-break" aria-current="page">${esc(current.name)}</li></ol></nav>
 <div class="mb-3"><div style="max-width:400px"><label class="form-label" for="profile-choice">Container profile</label><select id="profile-choice" class="form-select" data-profile-choice ${saving?'disabled':''}>${names.map(n=>`<option ${n===current.name?'selected':''}>${esc(n)}</option>`).join('')}</select></div></div>
 <div class="card"><div class="card-header"><h2 class="card-title mb-0">${esc(current.name)}</h2><span class="ms-auto text-secondary small">${esc(current.file_name)}</span></div>
 <div class="card-header py-0"><nav class="nav nav-tabs border-0 flex-wrap" aria-label="Profile settings">${Object.entries(tabs).map(([k,v])=>`<button class="nav-link ${tab===k?'active':''}" data-profile-tab="${k}">${v}</button>`).join('')}</nav></div>
 <div class="card-body"><fieldset ${state.readOnly||saving?'disabled':''} class="m-0 p-0 border-0" style="min-width:0">${body}</fieldset>
 <details class="mt-3"><summary class="text-secondary small">Used by ${usages.length} containers</summary><div class="small mt-2">${usages.map(u=>esc(u.app+' / '+(u.container||''))).join('<br>') || 'Currently unused.'}</div></details>
 ${extra.length?`<details class="mt-2"><summary class="text-secondary small">Additional fields (preserved)</summary><pre class="mt-2">${esc(JSON.stringify(Object.fromEntries(extra.map(k=>[k,draft.defaults[k]])),null,2))}</pre></details>`:''}</div>
 <div class="card-footer d-flex flex-wrap gap-2 justify-content-between align-items-center"><div class="small" role="status" aria-live="polite"><span class="text-secondary" data-profile-status></span><div data-profile-feedback>${message}</div></div><div class="btn-list"><button class="btn" data-profile-cancel>Cancel changes</button><button class="btn btn-primary" data-profile-save>Save profile</button></div></div></div></div>`);
 mountTemplateFields();
 status();
}
async function leave() {
 if(saving)return false;
 if(current && changed() && !(await confirmAction({title:'Discard profile changes?',body:'Changes in all profile tabs will be discarded.',confirmLabel:'Discard changes',confirmClass:'btn-danger'})))return false;
 current=null;draft=null;baseline=null;request++;qs('#defaults-filter')?.closest('.row')?.classList.remove('d-none');return true;
}
async function open(name) {
 if(!name || !state.env || !(await leave()))return;
 const env=state.env, ticket=++request;
 setHTML('#defaults-overview', '<div class="text-secondary py-3" role="status">Loading profile...</div>');
 try {
 const value=await api(`/api/v1/envs/${encodeURIComponent(env)}/defaults/container-profiles/${encodeURIComponent(name)}`);
 if(ticket!==request || env!==state.env)return;
 current=value;explicitImage=value.image==='<from release manifest>'?'':value.image;draft={defaults:clone(value.defaults)};baseline=clone(draft);tab='general';message='';render();
 }catch(e){showError(e);renderDefaultsOverview();}
}
async function save() {
 if(!active() || state.readOnly || saving || !changed())return;
 saving=true;message='';render();
 try {
 const updated=await apiPatch(`/api/v1/envs/${encodeURIComponent(current.env)}/defaults/container-profiles/${encodeURIComponent(current.name)}`,{expected_hash:current.content_hash,profile:{defaults:draft.defaults}});
 current=updated;draft={defaults:clone(updated.defaults)};baseline=clone(draft);
 message='<span class="text-success"><i class="ti ti-check me-1"></i>Profile saved.</span>';
 try {await refreshRepositorySnapshot();const snapshot = await loadInspection(true);if (!snapshot) throw new Error('Inspection refresh failed');}catch(e){message+='<div class="alert alert-warning py-2">Saved, but the overview could not refresh. Refresh it manually.</div>';}
 }catch(e){const error=String(e.message || e); for(const [key,label] of Object.entries({resources:"resources",probes:"probes",startup:"startup",envs:"envs",ports:"port",general:"image"})){if(error.includes(label)){tab=key;break;}} message=`<div class="alert alert-danger py-2">${esc(error)} Your draft has been kept.</div>`;}
 finally {saving=false;render();}
}
document.addEventListener('input',e=>{
 const el=e.target;if(!el.closest('#profile-workspace') || !el.hasAttribute('data-profile-path') || saving || state.readOnly)return;
 let value=el.value;if(el.hasAttribute('data-numeric') && /^\d+$/.test(value))value=Number(value);
 if(el.hasAttribute('data-optional') && value==='')value=undefined;
 set(el.dataset.profilePath,value);message='';qs('[data-profile-feedback]').innerHTML='';status();
});
document.addEventListener('change',async e=>{
 const el=e.target;if(!el.closest('#profile-workspace') || saving)return;
 if(el.dataset.resourceStep && !state.readOnly){resourceSteps[el.dataset.resourceStep]=Number(el.value);syncResourceControls();return;}
 if(el.hasAttribute('data-profile-choice')) {await open(el.value);render();return;}
 if(state.readOnly)return;
 message='';
 if(el.dataset.profileMode){if (el.dataset.profileMode==='release') { explicitImage=get('defaults.image'); set('defaults.image','<from release manifest>'); } else {set('defaults.image',explicitImage || '');} render();}
 if(el.dataset.profileList){set(el.dataset.profileList,el.checked?[]:undefined);render();}
 if(el.dataset.profileEnvKind!==undefined){const i=Number(el.dataset.profileEnvKind),kind=el.value,old=get(`defaults.envs.${i}`);set(`defaults.envs.${i}`,{name:old.name,[kind]:kind==='remove'?true:'',...(kind==='secret_name'?{key:''}:{})});render();}
});
document.addEventListener('click',async e=>{
 const el=e.target.closest('button, [data-profile-back]');if(!el?.closest('#profile-workspace'))return;
 if(el.hasAttribute('data-profile-back'))e.preventDefault();
 if(saving)return;
 if(el.hasAttribute('data-profile-back')){if(await leave())renderDefaultsOverview();return;}
 if(el.dataset.profileTab){tab=el.dataset.profileTab;render();return;}
 if(el.hasAttribute('data-profile-cancel')){draft=clone(baseline);message='';render();return;}
 if(el.hasAttribute('data-profile-save')){await save();return;}
 if(el.hasAttribute('data-resource-direction') && !state.readOnly){const kind=el.closest('[data-resource-control]').dataset.resourceControl;const value=resourceStepper.next(kind,get(el.dataset.resourcePath),resourceSteps[kind],Number(el.dataset.resourceDirection));if(value!==null){set(el.dataset.resourcePath,value);message='';render();}return;}
 if(el.dataset.profileAction && !state.readOnly){message='';const path=el.dataset.target,values=get(path)||[];
 if(el.dataset.profileAction==='move-up' || el.dataset.profileAction==='move-down') {const i=Number(el.dataset.index),j=i+(el.dataset.profileAction==='move-up'?-1:1);if(j>=0 && j<values.length)[values[i],values[j]]=[values[j],values[i]];}
 else if(el.dataset.profileAction==='remove')values.splice(Number(el.dataset.index),1);
 else values.push({'add-string':'','add-env':{name:'',value:''},'add-port':{name:'',port:'',expose_as:[]},'add-service':{hostname:'',port:''}}[el.dataset.profileAction]);
 set(path,values);render();}
});
window.addEventListener('beforeunload',e=>{if(current && changed()){e.preventDefault();e.returnValue='';}});
return {open,active,render,leave};
})();
