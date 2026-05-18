const state = { envs: [], env: null, apps: [], assets: [], inventory: null, git: null, appFile: null, assetPath: null, active: 'dashboard' };
const qs = (s) => document.querySelector(s);
const qsa = (s) => Array.from(document.querySelectorAll(s));
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const api = async (url) => { const r = await fetch(url, { credentials: 'same-origin' }); if (!r.ok) throw new Error(`${r.status} ${r.statusText}: ${await r.text()}`); return await r.json(); };
const apiPost = async (url) => { const r = await fetch(url, { method: 'POST', credentials: 'same-origin' }); if (!r.ok) throw new Error(`${r.status} ${r.statusText}: ${await r.text()}`); return await r.json(); };
const apiPatch = async (url, payload) => { const r = await fetch(url, { method: 'PATCH', credentials: 'same-origin', headers: {'Content-Type':'application/json'}, body: JSON.stringify(payload) }); if (!r.ok) throw new Error(`${r.status} ${r.statusText}: ${await r.text()}`); return await r.json(); };
function showError(e) { const el = qs('#ui-error'); el.textContent = String(e); el.classList.remove('hidden'); }
function clearError() { const el = qs('#ui-error'); el.textContent = ''; el.classList.add('hidden'); }
function setText(id, value) { const el = qs(id); if (el) el.textContent = value ?? '-'; }
function setHTML(id, value) { const el = qs(id); if (el) el.innerHTML = value ?? ''; }
function setActive(page) {
  state.active = page;
  for (const link of qsa('[data-nav]')) {
    const active = link.dataset.nav === page;
    link.classList.toggle('active', active);
    link.closest('.nav-item')?.classList.toggle('active', active);
  }
  for (const pageEl of qsa('[data-page]')) pageEl.classList.toggle('hidden', pageEl.dataset.page !== page);
  setText('#page-pretitle', state.env ? `Environment ${state.env}` : 'Environment repository');
  setText('#page-title', page === 'dashboard' ? 'Select environment' : page === 'changes' ? 'Changed files' : page === 'apps' ? 'Inspect applications' : page === 'assets' ? 'Inspect assets' : 'Build preview');
  if (page === 'build') loadBuildData();
}
function applyTheme(theme) {
  if (theme === 'auto') theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  document.documentElement.setAttribute('data-bs-theme', theme);
}
function setupTheme() {
  const saved = localStorage.getItem('theme') || 'auto';
  applyTheme(saved);
  qs('#theme-toggle')?.addEventListener('click', (e) => { e.preventDefault(); qs('#theme-menu')?.classList.toggle('show'); });
  for (const item of qsa('[data-theme]')) item.addEventListener('click', (e) => { e.preventDefault(); const t = item.dataset.theme || 'auto'; localStorage.setItem('theme', t); applyTheme(t); qs('#theme-menu')?.classList.remove('show'); });
}
async function init() {
  setupTheme();
  qsa('[data-nav]').forEach((x) => x.addEventListener('click', (e) => { e.preventDefault(); if (x.dataset.nav !== 'dashboard' && !state.env) return showError('Select environment first.'); setActive(x.dataset.nav); }));
  qs('#refresh-btn')?.addEventListener('click', () => refreshCurrentView());
  qs('#apps-filter')?.addEventListener('input', () => renderApps());
  qs('#assets-filter')?.addEventListener('input', () => renderAssets());
  qs('#build-validate-btn')?.addEventListener('click', () => runBuildValidate());
  qs('#build-refresh-btn')?.addEventListener('click', () => loadBuildData());
  qs('#inventory-filter')?.addEventListener('input', () => renderInventory());
  qs('#inventory-path')?.addEventListener('input', () => renderInventory());
  qs('#inventory-clear-btn')?.addEventListener('click', () => {
    const filter = qs('#inventory-filter');
    const path = qs('#inventory-path');
    if (filter) filter.value = '';
    if (path) path.value = '';
    renderInventory();
  });
  qs('#app-vars-save-btn')?.addEventListener('click', () => saveAppVars());
  qs('#app-vars-add-btn')?.addEventListener('click', () => addAppVarRow());
  qs('#app-vars')?.addEventListener('click', (e) => {
    const button = e.target.closest('[data-remove-app-var]');
    if (!button || state.readOnly) return;
    button.closest('tr')?.remove();
    syncAppVarsEmptyState();
  });
  await loadAll();
}
async function loadAll() {
  clearError();
  try {
    const info = await api('/api/v1/info');
    state.readOnly = !!info.read_only;
    setText('#app-version', `${info.version} / ${info.commit}`);
    qs('#read-only-badge').classList.toggle('hidden', !info.read_only);
    qs('#read-only-hint')?.classList.toggle('hidden', !info.read_only);
    await refreshRepositorySnapshot();
    if (state.envs.length) await selectEnv(state.env || localStorage.getItem('activeEnv') || state.envs[0].name);
  } catch (e) { showError(e); }
}
async function refreshRepositorySnapshot() {
  const [git, data] = await Promise.all([
    api('/api/v1/git/status'),
    api('/api/v1/envs'),
  ]);
  state.git = git;
  renderGitStatus();
  state.envs = data.items || [];
  setText('#repo-root', data.root || '-');
  setText('#env-count', state.envs.length);
  renderEnvs();
  renderEnvChanges();
}
async function refreshCurrentView() {
  clearError();
  try {
    await refreshRepositorySnapshot();
    if (state.env) {
      await Promise.all([loadApps(), loadAssets()]);
      if (state.active === 'apps' && state.appFile) await selectApp(state.appFile);
      if (state.active === 'assets' && state.assetPath) await selectAsset(state.assetPath);
      if (state.active === 'build') resetBuildView();
    }
  } catch (e) { showError(e); }
}
function renderGitStatus() {
  const badge = qs('#git-status-badge');
  if (!badge) return;
  const git = state.git || {};
  if (!git.available) {
    badge.className = 'badge bg-secondary-lt';
    badge.innerHTML = '<i class="ti ti-git-branch me-1"></i>no git';
    return;
  }
  badge.className = `badge ${git.dirty ? 'bg-yellow-lt' : 'bg-green-lt'}`;
  const branch = git.branch || git.commit || 'detached';
  badge.innerHTML = `<i class="ti ti-git-branch me-1"></i>${esc(branch)}${git.dirty ? ` · ${git.dirty_count}` : ''}`;
}
function gitFilesForEnv(env) {
  const prefix = `${env}/`;
  return (state.git?.files || []).filter((file) => file.path === env || file.path.startsWith(prefix));
}
function gitFile(path) {
  return (state.git?.files || []).find((file) => file.path === path);
}
function gitCodeLabel(code) {
  if (code === '??') return 'untracked';
  if (code === 'M') return 'modified';
  if (code === 'A') return 'added';
  if (code === 'D') return 'deleted';
  return code || 'dirty';
}
function dirtyBadge(file) {
  if (!file) return '';
  return `<span class="badge bg-yellow-lt ms-2"><i class="ti ti-alert-triangle me-1"></i>${esc(gitCodeLabel(file.code))}</span>`;
}
async function selectEnv(env) {
  if (!state.envs.some((x) => x.name === env)) env = state.envs[0]?.name;
  if (!env) return;
  const changedEnv = state.env !== env;
  state.env = env; localStorage.setItem('activeEnv', env);
  if (changedEnv) resetSelectedDetails();
  renderEnvs();
  renderEnvChanges();
  await Promise.all([loadApps(), loadAssets()]);
  const hasChanges = gitFilesForEnv(env).length > 0;
  setActive(state.active === 'dashboard' ? (hasChanges ? 'changes' : 'apps') : state.active);
}
function renderEnvs() {
  const host = qs('#env-list');
  if (!state.envs.length) { host.innerHTML = '<div class="text-muted">No environments found.</div>'; return; }
  host.innerHTML = state.envs.map((e) => `
    <div class="col-sm-6 col-xl-4">
      <a href="#" class="card env-card text-reset text-decoration-none h-100 ${state.env === e.name ? 'border-primary' : ''}" data-env="${esc(e.name)}">
        <div class="card-body">
          <div class="d-flex align-items-start justify-content-between gap-3">
            <div><div class="text-uppercase text-muted small fw-semibold">Environment</div><h3 class="mb-1">${esc(e.name)}</h3><div class="text-muted small font-monospace text-break">${esc(e.path)}</div></div>
            <span class="avatar bg-blue-lt text-blue"><i class="ti ti-stack-2"></i></span>
          </div>
          <div class="mt-3 d-flex flex-wrap gap-2">
            <span class="badge bg-blue-lt">apps ${e.app_files_count ?? 0}</span>
            <span class="badge bg-cyan-lt">assets ${e.asset_files_count ?? 0}</span>
            <span class="badge ${e.has_env_secured_json ? 'bg-yellow-lt' : 'bg-secondary-lt'}">secured ${e.has_env_secured_json ? 'yes' : 'no'}</span>
            ${e.is_dirty ? `<span class="badge bg-yellow-lt"><i class="ti ti-alert-triangle me-1"></i>dirty ${gitFilesForEnv(e.name).length || ''}</span>` : ''}
          </div>
        </div>
      </a>
    </div>`).join('');
  qsa('[data-env]').forEach((x) => x.addEventListener('click', (e) => { e.preventDefault(); selectEnv(x.dataset.env); }));
  renderEnvChanges();
}
function renderEnvChanges() {
  const list = qs('#env-changes-list');
  const navItem = qs('#changes-nav-item');
  const count = qs('#changes-count');
  if (!list || !state.env) return;
  const files = gitFilesForEnv(state.env);
  if (navItem) navItem.classList.toggle('hidden', files.length === 0);
  if (count) count.textContent = String(files.length);
  list.innerHTML = files.length ? renderChangedFileGroups(files) : '<div class="list-group-item text-muted">No changed files in selected environment.</div>';
  qsa('[data-dirty-path]').forEach((item) => item.addEventListener('click', (event) => {
    event.preventDefault();
    openDirtyPath(item.dataset.dirtyPath);
  }));
}
function renderChangedFileGroups(files) {
  const groups = [
    { label: 'Apps', icon: 'ti-apps', items: [] },
    { label: 'Assets', icon: 'ti-folders', items: [] },
    { label: 'Env files', icon: 'ti-file-settings', items: [] },
    { label: 'Other', icon: 'ti-dots', items: [] },
  ];
  for (const file of files) groups[changedFileGroupIndex(file.path)].items.push(file);
  return groups.filter((group) => group.items.length).map((group) => `
    <div class="list-group-header"><i class="ti ${group.icon} me-2"></i>${esc(group.label)} · ${group.items.length}</div>
    ${group.items.map(renderChangedFileRow).join('')}`).join('');
}
function changedFileGroupIndex(path) {
  if (!state.env || !path.startsWith(`${state.env}/`)) return 3;
  const rest = path.slice(state.env.length + 1);
  if (rest.startsWith('apps/')) return 0;
  if (rest.startsWith('assets/') || rest.startsWith('assets.')) return 1;
  if (rest.startsWith('env.')) return 2;
  return 3;
}
function renderChangedFileRow(file) {
  const target = dirtyNavigationTarget(file.path);
  return `
    <a href="#" class="list-group-item list-group-item-action d-flex align-items-center justify-content-between gap-3 ${target ? '' : 'disabled'}" data-dirty-path="${esc(file.path)}">
      <span class="font-monospace text-break">${esc(file.path)}</span>
      <span class="badge bg-yellow-lt">${esc(gitCodeLabel(file.code))}</span>
    </a>`;
}
function dirtyNavigationTarget(path) {
  if (!state.env || !path.startsWith(`${state.env}/`)) return null;
  const rest = path.slice(state.env.length + 1);
  if (rest.startsWith('apps/')) return { page: 'apps', value: rest.slice('apps/'.length) };
  if (rest.startsWith('assets/')) return { page: 'assets', value: rest.slice('assets/'.length) };
  if (isSpecial(rest)) return { page: 'assets', value: rest };
  return null;
}
function openDirtyPath(path) {
  const target = dirtyNavigationTarget(path);
  if (!target) return;
  setActive(target.page);
  if (target.page === 'apps') selectApp(target.value);
  if (target.page === 'assets') selectAsset(target.value);
}
function resetSelectedDetails() {
  state.appFile = null;
  state.assetPath = null;
  state.inventory = null;
  setText('#app-detail-title', 'Select app');
  setText('#app-detail-path', '');
  setText('#app-detail-meta', '');
  setHTML('#app-detail-badges', '');
  setHTML('#app-overview', '<div class="text-muted">Select an application to show structured model overview.</div>');
  setText('#app-raw', '');
  setText('#app-rendered', '');
  const appVarsBody = qs('#app-vars tbody');
  if (appVarsBody) appVarsBody.innerHTML = '';
  qs('#app-diff-section')?.classList.add('hidden');
  setText('#app-diff', '');
  setText('#asset-detail-title', 'Select asset');
  setText('#asset-detail-path', '');
  setHTML('#asset-detail-badges', '');
  setText('#asset-raw', '');
  qs('#asset-diff-section')?.classList.add('hidden');
  setText('#asset-diff', '');
  resetBuildView();
}

function resetBuildView() {
  state.inventory = null;
  setBuildStatus('info', 'Select an environment and run a build check.');
  setBuildDataEnv(null);
  setHTML('#build-totals', '');
  const summaryBody = qs('#build-summary-table tbody');
  if (summaryBody) summaryBody.innerHTML = '<tr><td colspan="9" class="text-muted">No summary loaded.</td></tr>';
  const summaryFoot = qs('#build-summary-table tfoot');
  if (summaryFoot) summaryFoot.innerHTML = '';
  const inventoryBody = qs('#build-inventory-table tbody');
  if (inventoryBody) inventoryBody.innerHTML = '<tr><td colspan="5" class="text-muted">No inventory loaded.</td></tr>';
}

async function loadApps() {
  if (!state.env) return;
  const data = await api(`/api/v1/envs/${encodeURIComponent(state.env)}/apps`);
  state.apps = data.items || [];
  setText('#apps-count', state.apps.length);
  renderApps();
}
function renderApps() {
  const body = qs('#apps-table tbody');
  const dirtyApps = state.apps.filter((a) => gitFile(`${state.env}/apps/${a.file_name}`));
  const dirtyBadgeEl = qs('#apps-dirty-count');
  if (dirtyBadgeEl) {
    dirtyBadgeEl.classList.toggle('hidden', dirtyApps.length === 0);
    dirtyBadgeEl.textContent = dirtyApps.length ? `${dirtyApps.length} changed` : '';
  }
  const query = (qs('#apps-filter')?.value || '').trim().toLowerCase();
  const apps = state.apps.filter((a) => {
    if (!query) return true;
    return [a.app_name, a.file_name, a.replicas, a.containers_count].some((value) => String(value ?? '').toLowerCase().includes(query));
  });
  setText('#apps-filter-count', `${apps.length}/${state.apps.length}`);
  body.innerHTML = apps.map((a) => `
    <tr class="row-link ${state.appFile === a.file_name ? 'selected-row' : ''}" data-app="${esc(a.file_name)}">
      <td>
        <div class="fw-semibold app-list-name">${esc(a.app_name || a.file_name)}${dirtyBadge(gitFile(`${state.env}/apps/${a.file_name}`))}</div>
        <div class="text-muted small font-monospace text-break">${esc(a.file_name)}</div>
      </td>
      <td class="text-end">
        <div class="badge bg-blue-lt" title="replicas">${a.replicas ?? '-'}</div>
        <div class="text-muted small mt-1">${a.containers_count ?? 0} ctr</div>
      </td>
    </tr>`).join('') || '<tr><td colspan="2" class="text-muted">No apps.</td></tr>';
  qsa('[data-app]').forEach((x) => x.addEventListener('click', () => selectApp(x.dataset.app)));
}
async function selectApp(file) {
  state.appFile = file; renderApps(); clearError();
  try {
    const [detail, rendered, vars, model] = await Promise.all([
      api(`/api/v1/envs/${encodeURIComponent(state.env)}/apps/${encodeURIComponent(file)}`),
      api(`/api/v1/envs/${encodeURIComponent(state.env)}/apps/${encodeURIComponent(file)}/rendered`),
      api(`/api/v1/envs/${encodeURIComponent(state.env)}/apps/${encodeURIComponent(file)}/vars`),
      api(`/api/v1/envs/${encodeURIComponent(state.env)}/apps/${encodeURIComponent(file)}/model`),
    ]);
    setText('#app-detail-title', model.app_name || detail.summary?.app_name || detail.file_name);
    setText('#app-detail-path', detail.path);
    setText('#app-detail-meta', `${model.containers?.length || 0} container(s), ${vars.items?.length || 0} local variable(s)${detail.is_dirty ? ', dirty file' : ''}`);
    setHTML('#app-detail-badges', renderAppDetailBadges(model, detail));
    renderAppOverview(model);
    setText('#app-raw', detail.content);
    setText('#app-rendered', rendered.content);
    renderAppVarsEditor(vars.items || []);
    await loadGitDiff(`${state.env}/apps/${detail.file_name}`, detail.is_dirty, '#app-diff-section', '#app-diff');
  } catch (e) { showError(e); }
}
function renderAppVarsEditor(items) {
  const body = qs('#app-vars tbody');
  if (!body) return;
  body.innerHTML = items.map((v) => renderAppVarRow(v.name, v.value)).join('');
  syncAppVarsEmptyState();
  const save = qs('#app-vars-save-btn');
  if (save) save.disabled = state.readOnly || !state.appFile;
  const add = qs('#app-vars-add-btn');
  if (add) add.disabled = state.readOnly || !state.appFile;
}
function renderAppVarRow(name = '', value = '') {
  const disabled = state.readOnly ? 'disabled' : '';
  return `
    <tr>
      <td><input class="form-control form-control-sm font-monospace app-var-name" placeholder="NAME" value="${esc(name)}" ${disabled}></td>
      <td><input class="form-control form-control-sm font-monospace app-var-value" placeholder="value" value="${esc(value)}" ${disabled}></td>
      <td class="table-action-col"><button class="btn btn-sm btn-outline-danger btn-icon" type="button" data-remove-app-var title="Remove variable" ${disabled}><i class="ti ti-trash"></i></button></td>
    </tr>`;
}
function syncAppVarsEmptyState() {
  const body = qs('#app-vars tbody');
  if (!body) return;
  const hasRows = qsa('#app-vars tbody tr').some((row) => row.querySelector('.app-var-name'));
  if (!hasRows) body.innerHTML = '<tr><td colspan="3" class="text-muted">No local variables.</td></tr>';
}
function addAppVarRow() {
  if (state.readOnly || !state.appFile) return;
  const body = qs('#app-vars tbody');
  if (!body) return;
  if (!qsa('#app-vars tbody tr').some((row) => row.querySelector('.app-var-name'))) body.innerHTML = '';
  body.insertAdjacentHTML('beforeend', renderAppVarRow());
  qsa('#app-vars tbody tr').at(-1)?.querySelector('.app-var-name')?.focus();
}
async function saveAppVars() {
  if (!state.env || !state.appFile || state.readOnly) return;
  clearError();
  const items = qsa('#app-vars tbody tr').map((row) => ({
    name: row.querySelector('.app-var-name')?.value?.trim() || '',
    value: row.querySelector('.app-var-value')?.value || '',
  })).filter((item) => item.name !== '');
  try {
    await apiPatch(`/api/v1/envs/${encodeURIComponent(state.env)}/apps/${encodeURIComponent(state.appFile)}/vars`, {items});
    await refreshRepositorySnapshot();
    await loadApps();
    await selectApp(state.appFile);
  } catch (e) { showError(e); }
}
function renderAppDetailBadges(model, detail) {
  const containers = model.containers || [];
  const hasJava = containers.some((container) => container.runtime?.java?.enabled);
  const hasLegacyProbes = containers.some((container) => container.probes?.legacy);
  const hasProbes = containers.some((container) => container.probes?.enabled);
  const badges = [
    detail.is_dirty ? badge('dirty', 'bg-yellow-lt', 'ti-alert-triangle') : '',
    model.autoscaling?.enabled ? badge('autoscaling', 'bg-blue-lt', 'ti-arrows-maximize') : '',
    model.init_containers_count ? badge(`init ${model.init_containers_count}`, 'bg-purple-lt', 'ti-player-skip-forward') : '',
    hasJava ? badge('java runtime', 'bg-orange-lt', 'ti-coffee') : '',
    hasLegacyProbes ? badge('legacy probes', 'bg-yellow-lt', 'ti-alert-triangle') : hasProbes ? badge('probes', 'bg-green-lt', 'ti-heartbeat') : '',
  ].filter(Boolean);
  return badges.join('') || '<span class="badge bg-secondary-lt">basic app</span>';
}
function badge(label, color, icon) {
  return `<span class="badge ${color}"><i class="ti ${icon} me-1"></i>${esc(label)}</span>`;
}
function renderAppOverview(model) {
  const host = qs('#app-overview');
  if (!host) return;
  const autoscaling = model.autoscaling || {};
  const appFacts = [
    fact('Kind', model.kind || 'Deployment', 'ti-cube'),
    fact('Replicas', model.replicas ?? '-', 'ti-copy'),
    fact('Containers', (model.containers || []).length, 'ti-box'),
    fact('Init containers', model.init_containers_count || 0, 'ti-player-skip-forward'),
    fact('Autoscaling', autoscaling.enabled ? `HPA ${autoscaling.min_replicas ?? '?'}-${autoscaling.max_replicas ?? '?'}` : 'off', autoscaling.enabled ? 'ti-arrows-maximize' : 'ti-arrows-minimize'),
  ];
  const autoscalingMetrics = autoscaling.enabled ? `
    <div class="overview-section">
      <div class="overview-title"><i class="ti ti-arrows-maximize me-1"></i>Autoscaling</div>
      <div class="chip-row">
        ${chip('min', autoscaling.min_replicas ?? '-')}
        ${chip('max', autoscaling.max_replicas ?? '-')}
        ${chip('cpu avg', autoscaling.cpu_average_utilization ? `${autoscaling.cpu_average_utilization}%` : '-')}
        ${chip('memory avg', autoscaling.memory_average_utilization ? `${autoscaling.memory_average_utilization}%` : '-')}
      </div>
    </div>` : '';
  const containers = (model.containers || []).map((container) => renderContainerOverview(container)).join('');
  host.innerHTML = `
    <div class="overview-grid">${appFacts.join('')}</div>
    ${autoscalingMetrics}
    <div class="overview-section">
      <div class="overview-title"><i class="ti ti-box me-1"></i>Containers</div>
      <div class="container-stack">${containers || '<div class="text-muted">No containers.</div>'}</div>
    </div>`;
}
function renderContainerOverview(container) {
  const resources = container.resources || {};
  const java = container.runtime?.java || {};
  const probes = container.probes || {};
  const ports = container.ports || [];
  const envVars = container.env_vars || [];
  const javaBlock = java.enabled ? `
    <div class="chip-row">
      ${chip('JVM Xms', java.xms || '-')}
      ${chip('JVM Xmx', java.xmx || '-')}
      ${chip('export', java.export_env_name || 'JAVA_OPTS')}
      ${java.opts?.length ? chip('opts', java.opts.length) : ''}
    </div>` : '<div class="text-muted small">Java runtime not configured.</div>';
  const probeText = probes.enabled ? [probes.preset && `preset ${probes.preset}`, probes.port && `port ${probes.port}`, probes.path && `path ${probes.path}`, probes.legacy && 'legacy'].filter(Boolean).join(', ') : 'not configured';
  return `
    <div class="container-card">
      <div class="d-flex align-items-start justify-content-between gap-2 mb-2">
        <div>
          <div class="fw-semibold font-monospace">${esc(container.name || `container-${container.index}`)}</div>
          <div class="text-muted small">${envVars.length} env var(s), ${container.env_from_count || 0} env_from, ${container.mounts_count || 0} mount(s), ${ports.length} port(s)</div>
        </div>
        <span class="badge bg-blue-lt">#${container.index + 1}</span>
      </div>
      <div class="row g-2">
        <div class="col-12 col-xl-4"><div class="overview-subtitle">Resources</div><div class="chip-row">
          ${chip('CPU req', resources.cpu_request || '-')}
          ${chip('CPU lim', resources.cpu_limit || '-')}
          ${chip('Mem req', resources.memory_request || '-')}
          ${chip('Mem lim', resources.memory_limit || '-')}
        </div></div>
        <div class="col-12 col-xl-4"><div class="overview-subtitle">Java runtime</div>${javaBlock}</div>
        <div class="col-12 col-xl-4"><div class="overview-subtitle">Probes</div><div class="text-muted small">${esc(probeText)}</div></div>
      </div>
    </div>`;
}
function fact(label, value, icon) {
  return `<div class="overview-fact"><div class="text-muted small"><i class="ti ${icon} me-1"></i>${esc(label)}</div><div class="fw-semibold">${esc(value)}</div></div>`;
}
function chip(label, value) {
  return `<span class="model-chip"><span>${esc(label)}</span><strong>${esc(value)}</strong></span>`;
}
async function loadAssets() {
  if (!state.env) return;
  const data = await api(`/api/v1/envs/${encodeURIComponent(state.env)}/assets`);
  state.assets = data.items || [];
  setText('#assets-count', state.assets.length);
  renderAssets();
}
function renderAssets() {
  const host = qs('#assets-list');
  const dirtyAssets = state.assets.filter((a) => gitFile(assetGitPath(a.relative_path)));
  const dirtyBadgeEl = qs('#assets-dirty-count');
  if (dirtyBadgeEl) {
    dirtyBadgeEl.classList.toggle('hidden', dirtyAssets.length === 0);
    dirtyBadgeEl.textContent = dirtyAssets.length ? `${dirtyAssets.length} changed` : '';
  }
  if (!host) return;
  const query = (qs('#assets-filter')?.value || '').trim().toLowerCase();
  const assets = state.assets.filter((asset) => {
    if (!query) return true;
    return [asset.relative_path, asset.file_name, asset.driver, asset.size_bytes].some((value) => String(value ?? '').toLowerCase().includes(query));
  });
  setText('#assets-filter-count', `${assets.length}/${state.assets.length}`);
  host.innerHTML = renderAssetTree(assets);
  qsa('[data-asset]').forEach((x) => x.addEventListener('click', () => selectAsset(x.dataset.asset)));
}
function renderAssetTree(assets) {
  if (!assets.length) return '<div class="asset-empty text-muted">No assets.</div>';
  const root = { dirs: new Map(), files: [] };
  for (const asset of assets) addAssetTreeNode(root, asset);
  return renderAssetTreeNode(root, 0);
}
function addAssetTreeNode(root, asset) {
  const parts = asset.relative_path.split('/').filter(Boolean);
  if (parts.length <= 1) {
    root.files.push(asset);
    return;
  }
  let node = root;
  for (const part of parts.slice(0, -1)) {
    if (!node.dirs.has(part)) node.dirs.set(part, { name: part, dirs: new Map(), files: [] });
    node = node.dirs.get(part);
  }
  node.files.push(asset);
}
function renderAssetTreeNode(node, depth) {
  const dirs = Array.from(node.dirs.values()).sort((a, b) => a.name.localeCompare(b.name));
  const files = node.files.slice().sort((a, b) => a.relative_path.localeCompare(b.relative_path));
  return [
    ...dirs.map((dir) => renderAssetDirectory(dir, depth) + renderAssetTreeNode(dir, depth + 1)),
    ...files.map((asset) => renderAssetFile(asset, depth)),
  ].join('');
}
function renderAssetDirectory(dir, depth) {
  const padding = 12 + depth * 18;
  return `
    <div class="asset-row asset-dir" style="--asset-indent:${padding}px">
      <div class="asset-main">
        <i class="ti ti-folder asset-icon folder"></i>
        <span class="asset-name font-monospace">${esc(dir.name)}</span>
      </div>
      <div class="asset-meta text-muted">dir</div>
    </div>`;
}
function renderAssetFile(asset, depth) {
  const padding = 12 + depth * 18;
  const dirty = gitFile(assetGitPath(asset.relative_path));
  return `
    <div class="asset-row asset-file ${state.assetPath === asset.relative_path ? 'selected-row' : ''}" style="--asset-indent:${padding}px" data-asset="${esc(asset.relative_path)}">
      <div class="asset-main">
        <i class="ti ${assetIcon(asset)} asset-icon"></i>
        <div class="min-w-0">
          <div class="asset-name font-monospace text-truncate">${esc(assetFileName(asset.relative_path))}${dirtyBadge(dirty)}</div>
          <div class="asset-subtitle text-muted small font-monospace text-truncate">${esc(asset.relative_path)}</div>
        </div>
      </div>
      <div class="asset-meta text-muted">
        <span>${esc(formatBytes(asset.size_bytes ?? 0))}</span>
        <span>${esc(asset.driver)}</span>
      </div>
    </div>`;
}
function assetFileName(path) {
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] || path;
}
function assetIcon(asset) {
  if (asset.driver === 'special') return 'ti-lock-square-rounded';
  const name = asset.relative_path.toLowerCase();
  if (name.endsWith('.yml') || name.endsWith('.yaml')) return 'ti-file-code';
  if (name.endsWith('.json')) return 'ti-json';
  if (name.endsWith('.conf') || name.endsWith('.tpl')) return 'ti-file-settings';
  return 'ti-file';
}
function formatBytes(bytes) {
  const value = Number(bytes || 0);
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${value} B`;
}
function assetGitPath(relativePath) {
  return isSpecial(relativePath) ? `${state.env}/${relativePath}` : `${state.env}/assets/${relativePath}`;
}
async function selectAsset(path) {
  state.assetPath = path; renderAssets(); clearError();
  try {
    const selectedAsset = state.assets.find((asset) => asset.relative_path === path);
    setText('#asset-detail-title', path);
    if (isSpecial(path)) {
      const entries = await api(`/api/v1/envs/${encodeURIComponent(state.env)}/assets/special/${encodeURIComponent(path)}/entries`);
      setText('#asset-detail-path', `${entries.entries?.length || 0} entrie(s), editable=${entries.editable}`);
      setHTML('#asset-detail-badges', renderAssetDetailBadges(selectedAsset, entries));
      setText('#asset-raw', (entries.entries || []).map((e) => `${e.key} [${e.value_type}] = ${e.value_text}`).join('\n'));
      await loadGitDiff(`${state.env}/${path}`, entries.is_dirty, '#asset-diff-section', '#asset-diff');
    } else {
      const detail = await api(`/api/v1/envs/${encodeURIComponent(state.env)}/assets/content/${path.split('/').map(encodeURIComponent).join('/')}`);
      setText('#asset-detail-path', detail.path);
      setHTML('#asset-detail-badges', renderAssetDetailBadges(selectedAsset, detail));
      setText('#asset-raw', detail.content);
      await loadGitDiff(`${state.env}/assets/${detail.relative_path}`, detail.is_dirty, '#asset-diff-section', '#asset-diff');
    }
  } catch (e) { showError(e); }
}
function renderAssetDetailBadges(asset, detail) {
  return [
    asset ? badge(asset.driver, 'bg-cyan-lt', 'ti-tag') : '',
    asset ? badge(formatBytes(asset.size_bytes ?? 0), 'bg-secondary-lt', 'ti-database') : '',
    detail?.is_dirty ? badge('dirty', 'bg-yellow-lt', 'ti-alert-triangle') : '',
    detail && 'editable' in detail ? badge(detail.editable ? 'editable' : 'read-only', detail.editable ? 'bg-green-lt' : 'bg-secondary-lt', detail.editable ? 'ti-pencil' : 'ti-lock') : '',
  ].filter(Boolean).join('');
}
function isSpecial(path) { return ['env.secured.json','env.unsecured.json','assets.secured.json','assets.unsecured.json'].includes(path); }
async function loadGitDiff(relativePath, isDirty, sectionSelector, targetSelector) {
  const section = qs(sectionSelector);
  const target = qs(targetSelector);
  if (!section || !target) return;
  if (!isDirty) {
    section.classList.add('hidden');
    target.textContent = '';
    return;
  }
  const encoded = relativePath.split('/').map(encodeURIComponent).join('/');
  const diff = await api(`/api/v1/git/diff/${encoded}`);
  section.classList.remove('hidden');
  target.textContent = diff.content || diff.error || 'No textual diff available.';
}
async function runBuildValidate() {
  if (!state.env) return showError('Select environment first.');
  clearError();
  const env = state.env;
  try {
    setBuildStatus('info', 'Validation is running...');
    const result = await apiPost(`/api/v1/envs/${encodeURIComponent(env)}/validate`);
    if (state.env !== env) return;
    setBuildStatus(result.ok ? 'success' : 'danger', result.message || (result.ok ? 'Validation OK' : 'Validation failed'));
  } catch (e) {
    if (state.env !== env) return;
    setBuildStatus('danger', String(e));
    showError(e);
  }
}
async function loadBuildSummary() {
  if (!state.env) return;
  clearError();
  const env = state.env;
  try {
    setBuildDataEnv(env, 'loading');
    const summary = await apiPost(`/api/v1/envs/${encodeURIComponent(env)}/summary`);
    if (state.env !== env) return;
    renderBuildSummary(summary);
    setBuildDataEnv(env, 'loaded');
  } catch (e) {
    if (state.env !== env) return;
    showError(e);
  }
}
async function loadBuildInventory() {
  if (!state.env) return showError('Select environment first.');
  clearError();
  const env = state.env;
  try {
    const inventory = await apiPost(`/api/v1/envs/${encodeURIComponent(env)}/inventory`);
    if (state.env !== env) return;
    state.inventory = inventory;
    renderInventory();
  } catch (e) {
    if (state.env !== env) return;
    state.inventory = null;
    renderInventory();
    showError(e);
  }
}
async function loadBuildData() {
  if (!state.env) return;
  clearError();
  try {
    await Promise.all([loadBuildSummary(), loadBuildInventory()]);
  } catch (e) { showError(e); }
}
function setBuildStatus(kind, text) {
  const el = qs('#build-status');
  if (!el) return;
  el.className = `alert alert-${kind} mb-0`;
  el.textContent = text;
}
function setBuildDataEnv(env, stateName) {
  const el = qs('#build-data-env');
  if (!el) return;
  if (!env) {
    el.className = 'badge bg-secondary-lt';
    el.innerHTML = '<i class="ti ti-stack-2 me-1"></i>not loaded';
    return;
  }
  el.className = `badge ${stateName === 'loading' ? 'bg-blue-lt' : 'bg-green-lt'}`;
  el.innerHTML = `<i class="ti ti-stack-2 me-1"></i>${esc(stateName || 'loaded')} for ${esc(env)}`;
}
function renderBuildSummary(summary) {
  const totals = summary.totals || {};
  setHTML('#build-totals', [
    metricCard('Apps', totals.apps ?? 0, 'ti-apps'),
    metricCard('Containers', totals.containers ?? 0, 'ti-box'),
    metricCard('Replicas', totals.replicas ?? 0, 'ti-copy'),
    metricCard('CPU req', `${fmt(totals.cpu_request_cores)} cores`, 'ti-cpu'),
    metricCard('Mem req', `${fmt(totals.memory_request_mib)} MiB`, 'ti-database'),
    metricCard('Mem lim', `${fmt(totals.memory_limit_mib)} MiB`, 'ti-gauge'),
  ].join(''));
  const items = summary.items || [];
  qs('#build-summary-table tbody').innerHTML = items.map((item) => `
    <tr>
      <td class="font-monospace">${esc(item.app)}</td>
      <td class="font-monospace">${esc(item.container)}</td>
      <td class="text-end">${item.replicas ?? 0}</td>
      <td class="text-end">${esc(item.cpu_request)}</td>
      <td class="text-end">${esc(item.cpu_limit)}</td>
      <td class="text-end">${esc(item.memory_request)}</td>
      <td class="text-end">${esc(item.memory_limit)}</td>
      <td class="text-end">${esc(item.java_xms)}</td>
      <td class="text-end">${esc(item.java_xmx)}</td>
    </tr>`).join('') || '<tr><td colspan="9" class="text-muted">No summary items.</td></tr>';
  qs('#build-summary-table tfoot').innerHTML = items.length ? `
    <tr class="fw-bold">
      <td colspan="2">Total</td>
      <td class="text-end">${totals.replicas ?? 0}</td>
      <td class="text-end">${fmt(totals.cpu_request_cores)} cores</td>
      <td class="text-end">${fmt(totals.cpu_limit_cores)} cores</td>
      <td class="text-end">${fmt(totals.memory_request_mib)} MiB</td>
      <td class="text-end">${fmt(totals.memory_limit_mib)} MiB</td>
      <td class="text-end">-</td>
      <td class="text-end">-</td>
    </tr>` : '';
}
function renderInventory() {
  const table = qs('#build-inventory-table tbody');
  if (!table) return;
  const items = state.inventory?.items || [];
  const query = (qs('#inventory-filter')?.value || '').trim().toLowerCase();
  const dotPath = (qs('#inventory-path')?.value || '').trim();
  const filtered = items.filter((item) => {
    if (!query) return true;
    return [item.app, item.container, item.image, item.app_kind].some((value) => String(value || '').toLowerCase().includes(query));
  });
  table.innerHTML = filtered.map((item) => `
    <tr>
      <td class="font-monospace">${esc(item.app)}</td>
      <td class="font-monospace">${esc(item.container)}</td>
      <td class="font-monospace text-break">${esc(item.image)}</td>
      <td class="text-end">${item.replicas ?? '-'}</td>
      <td class="font-monospace text-break">${esc(dotPath ? valueAtPath(item, dotPath) : 'resources, mtls, rollout_checksums')}</td>
    </tr>`).join('') || '<tr><td colspan="5" class="text-muted">No inventory rows.</td></tr>';
}
function valueAtPath(value, path) {
  let current = value;
  for (const part of path.split('.').filter(Boolean)) {
    if (current == null || typeof current !== 'object' || !(part in current)) return '';
    current = current[part];
  }
  if (current == null) return '';
  if (typeof current === 'object') return JSON.stringify(current);
  return String(current);
}
function metricCard(label, value, icon) {
  return `<div class="col-6 col-lg-4"><div class="metric-card"><div class="text-muted small"><i class="ti ${icon} me-1"></i>${esc(label)}</div><div class="h3 mb-0">${esc(value)}</div></div></div>`;
}
function fmt(value) {
  const n = Number(value || 0);
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}
document.addEventListener('DOMContentLoaded', init);
