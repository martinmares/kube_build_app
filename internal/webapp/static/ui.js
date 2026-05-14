const state = { envs: [], env: null, apps: [], assets: [], inventory: null, appFile: null, assetPath: null, active: 'dashboard' };
const qs = (s) => document.querySelector(s);
const qsa = (s) => Array.from(document.querySelectorAll(s));
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const api = async (url) => { const r = await fetch(url, { credentials: 'same-origin' }); if (!r.ok) throw new Error(`${r.status} ${r.statusText}: ${await r.text()}`); return await r.json(); };
const apiPost = async (url) => { const r = await fetch(url, { method: 'POST', credentials: 'same-origin' }); if (!r.ok) throw new Error(`${r.status} ${r.statusText}: ${await r.text()}`); return await r.json(); };
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
  setText('#page-title', page === 'dashboard' ? 'Select environment' : page === 'apps' ? 'Inspect applications' : page === 'assets' ? 'Inspect assets' : 'Build preview');
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
  qs('#refresh-btn')?.addEventListener('click', () => loadAll());
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
  await loadAll();
}
async function loadAll() {
  clearError();
  try {
    const info = await api('/api/v1/info');
    setText('#app-version', `${info.version} / ${info.commit}`);
    qs('#read-only-badge').classList.toggle('hidden', !info.read_only);
    const data = await api('/api/v1/envs');
    state.envs = data.items || [];
    setText('#repo-root', data.root || '-');
    setText('#env-count', state.envs.length);
    renderEnvs();
    if (!state.env && state.envs.length) selectEnv(localStorage.getItem('activeEnv') || state.envs[0].name);
  } catch (e) { showError(e); }
}
async function selectEnv(env) {
  if (!state.envs.some((x) => x.name === env)) env = state.envs[0]?.name;
  if (!env) return;
  state.env = env; localStorage.setItem('activeEnv', env);
  renderEnvs();
  await Promise.all([loadApps(), loadAssets()]);
  setActive(state.active === 'dashboard' ? 'apps' : state.active);
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
          </div>
        </div>
      </a>
    </div>`).join('');
  qsa('[data-env]').forEach((x) => x.addEventListener('click', (e) => { e.preventDefault(); selectEnv(x.dataset.env); }));
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
  body.innerHTML = state.apps.map((a) => `<tr class="row-link ${state.appFile === a.file_name ? 'selected-row' : ''}" data-app="${esc(a.file_name)}"><td class="font-monospace">${esc(a.file_name)}</td><td>${esc(a.app_name || '-')}</td><td class="text-end">${a.replicas ?? '-'}</td><td class="text-end">${a.containers_count ?? 0}</td></tr>`).join('') || '<tr><td colspan="4" class="text-muted">No apps.</td></tr>';
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
    setText('#app-detail-title', detail.summary?.app_name || detail.file_name);
    setText('#app-detail-path', detail.path);
    setText('#app-detail-meta', `${model.containers?.length || 0} container(s), ${vars.items?.length || 0} local variable(s)`);
    setText('#app-raw', detail.content);
    setText('#app-rendered', rendered.content);
    qs('#app-vars').innerHTML = (vars.items || []).map((v) => `<tr><td class="font-monospace">${esc(v.name)}</td><td class="font-monospace text-break">${esc(v.value)}</td></tr>`).join('') || '<tr><td colspan="2" class="text-muted">No local variables.</td></tr>';
  } catch (e) { showError(e); }
}
async function loadAssets() {
  if (!state.env) return;
  const data = await api(`/api/v1/envs/${encodeURIComponent(state.env)}/assets`);
  state.assets = data.items || [];
  setText('#assets-count', state.assets.length);
  renderAssets();
}
function renderAssets() {
  const body = qs('#assets-table tbody');
  body.innerHTML = state.assets.map((a) => `<tr class="row-link ${state.assetPath === a.relative_path ? 'selected-row' : ''}" data-asset="${esc(a.relative_path)}"><td class="font-monospace">${esc(a.relative_path)}</td><td>${esc(a.driver)}</td><td class="text-end">${a.size_bytes ?? 0}</td></tr>`).join('') || '<tr><td colspan="3" class="text-muted">No assets.</td></tr>';
  qsa('[data-asset]').forEach((x) => x.addEventListener('click', () => selectAsset(x.dataset.asset)));
}
async function selectAsset(path) {
  state.assetPath = path; renderAssets(); clearError();
  try {
    setText('#asset-detail-title', path);
    if (isSpecial(path)) {
      const entries = await api(`/api/v1/envs/${encodeURIComponent(state.env)}/assets/special/${encodeURIComponent(path)}/entries`);
      setText('#asset-detail-path', `${entries.entries?.length || 0} entrie(s), editable=${entries.editable}`);
      setText('#asset-raw', (entries.entries || []).map((e) => `${e.key} [${e.value_type}] = ${e.value_text}`).join('\n'));
    } else {
      const detail = await api(`/api/v1/envs/${encodeURIComponent(state.env)}/assets/content/${path.split('/').map(encodeURIComponent).join('/')}`);
      setText('#asset-detail-path', detail.path);
      setText('#asset-raw', detail.content);
    }
  } catch (e) { showError(e); }
}
function isSpecial(path) { return ['env.secured.json','env.unsecured.json','assets.secured.json','assets.unsecured.json'].includes(path); }
async function runBuildValidate() {
  if (!state.env) return showError('Select environment first.');
  clearError();
  try {
    setBuildStatus('info', 'Validation is running...');
    const result = await apiPost(`/api/v1/envs/${encodeURIComponent(state.env)}/validate`);
    setBuildStatus(result.ok ? 'success' : 'danger', result.message || (result.ok ? 'Validation OK' : 'Validation failed'));
  } catch (e) { setBuildStatus('danger', String(e)); showError(e); }
}
async function loadBuildSummary() {
  if (!state.env) return;
  clearError();
  try {
    const summary = await apiPost(`/api/v1/envs/${encodeURIComponent(state.env)}/summary`);
    renderBuildSummary(summary);
  } catch (e) { showError(e); }
}
async function loadBuildInventory() {
  if (!state.env) return showError('Select environment first.');
  clearError();
  try {
    const inventory = await apiPost(`/api/v1/envs/${encodeURIComponent(state.env)}/inventory`);
    state.inventory = inventory;
    renderInventory();
  } catch (e) { state.inventory = null; renderInventory(); showError(e); }
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
    </tr>`).join('') || '<tr><td colspan="7" class="text-muted">No summary items.</td></tr>';
  qs('#build-summary-table tfoot').innerHTML = items.length ? `
    <tr class="fw-bold">
      <td colspan="2">Total</td>
      <td class="text-end">${totals.replicas ?? 0}</td>
      <td class="text-end">${fmt(totals.cpu_request_cores)} cores</td>
      <td class="text-end">${fmt(totals.cpu_limit_cores)} cores</td>
      <td class="text-end">${fmt(totals.memory_request_mib)} MiB</td>
      <td class="text-end">${fmt(totals.memory_limit_mib)} MiB</td>
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
