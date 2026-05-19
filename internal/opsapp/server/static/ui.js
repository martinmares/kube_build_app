const state = {
  info: null,
  envs: [],
  selected: null,
  filter: '',
};

const el = (id) => document.getElementById(id);

async function api(path) {
  const response = await fetch(path, { headers: { Accept: 'application/json' } });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.error || `${response.status} ${response.statusText}`);
  return payload;
}

function showError(error) {
  const box = el('ui-error');
  if (!error) {
    box.classList.add('hidden');
    box.textContent = '';
    return;
  }
  box.textContent = error.message || String(error);
  box.classList.remove('hidden');
}

function json(value) {
  return JSON.stringify(value, null, 2);
}

function shortHash(value) {
  return value ? value.slice(0, 12) : '-';
}

function digestTail(value) {
  if (!value) return '-';
  return value.length > 24 ? `${value.slice(0, 12)}...${value.slice(-10)}` : value;
}

function renderEnvList() {
  const list = el('env-list');
  const filter = state.filter.trim().toLowerCase();
  const envs = state.envs.filter((env) => !filter || [env.name, env.namespace, env.env_name, env.target_revision, env.branch].some((v) => String(v || '').toLowerCase().includes(filter)));
  el('env-count').textContent = `${envs.length} visible / ${state.envs.length} total`;
  list.innerHTML = '';
  for (const env of envs) {
    const item = document.createElement('button');
    item.type = 'button';
    item.className = `list-group-item list-group-item-action env-item ${state.selected?.name === env.name ? 'active' : ''}`;
    item.innerHTML = `
      <div class="d-flex justify-content-between align-items-start gap-2">
        <div>
          <div class="env-name">${escapeHTML(env.name)}</div>
          <div class="env-meta font-monospace">${escapeHTML(env.namespace || '-')}</div>
        </div>
        <span class="badge bg-azure-lt">${escapeHTML(env.target_revision || env.branch || 'HEAD')}</span>
      </div>
      <div class="env-meta text-truncate mt-2">${escapeHTML(env.repo || env.root_path || '-')}</div>
    `;
    item.addEventListener('click', () => selectEnv(env));
    list.appendChild(item);
  }
}

function selectEnv(env) {
  state.selected = env;
  el('detail-title').textContent = env.name;
  el('detail-subtitle').textContent = `${env.repo || env.root_path || '-'} :: ${env.root_path || '.'}`;
  el('metadata-output').textContent = json(env);
  el('diff-output').textContent = 'No diff loaded.';
  resetStatus();
  resetCluster();
  renderReleaseLane(env);
  renderSummaryCards(env);
  renderEnvList();
}

function resetCluster() {
  el('cluster-availability').className = 'badge bg-secondary-lt';
  el('cluster-availability').textContent = 'not loaded';
  el('cluster-summary').innerHTML = '';
  setTableBody('cluster-deployments', '<tr><td class="text-muted">No data loaded.</td></tr>');
  setTableBody('cluster-pods', '<tr><td class="text-muted">No data loaded.</td></tr>');
  setTableBody('cluster-services', '<tr><td class="text-muted">No data loaded.</td></tr>');
}

function resetStatus() {
  el('status-panel').className = 'status-panel';
  el('status-title').textContent = 'Not loaded';
  el('status-reason').textContent = 'Load status to compare desired render with the applied snapshot.';
}

function renderReleaseLane(payload) {
  el('target-revision').textContent = payload.target_revision || payload.branch || 'HEAD';
  el('resolved-commit').textContent = shortHash(payload.resolved_commit || payload.git?.resolved_commit);
  el('desired-digest').textContent = digestTail(payload.desired_digest || payload.digest);
  el('applied-revision').textContent = payload.applied_revision || payload.applied_state?.applied_revision || '-';
  el('applied-digest').textContent = digestTail(payload.applied_digest || payload.applied_state?.applied_digest);
}

function renderSummaryCards(payload) {
  const cards = [
    ['namespace', payload.namespace || '-'],
    ['env name', payload.env_name || payload.name || payload.environment || '-'],
    ['applied revision', payload.applied_revision || payload.applied_state?.applied_revision || '-'],
    ['applied digest', digestTail(payload.applied_digest || payload.applied_state?.applied_digest)],
  ];
  el('summary-cards').innerHTML = cards.map(([label, value]) => `
    <div class="col-12 col-md-6 col-xl-3">
      <div class="card summary-tile">
        <div class="card-body">
          <div class="summary-label">${escapeHTML(label)}</div>
          <div class="summary-value">${escapeHTML(value)}</div>
        </div>
      </div>
    </div>
  `).join('');
}

function renderStatus(payload) {
  const status = payload.sync_status || 'Unknown';
  const className = status === 'InSync' ? 'status-panel status-insync' : status === 'OutOfSync' ? 'status-panel status-outofsync' : 'status-panel status-unknown';
  el('status-panel').className = className;
  el('status-title').textContent = status;
  el('status-reason').textContent = payload.reason || 'No status reason.';
  renderReleaseLane(payload);
  renderSummaryCards(payload);
}

async function loadInfo() {
  state.info = await api('/api/v1/info');
  el('version-badge').textContent = state.info.version || '-';
  el('from-git-badge').textContent = state.info.from_git ? 'from git checkout' : 'local working tree';
}

async function loadEnvs() {
  const payload = await api('/api/v1/envs');
  state.envs = payload.environments || [];
  if (!state.selected && state.envs.length) selectEnv(state.envs[0]);
  renderEnvList();
}

async function resolveSelected() {
  if (!state.selected) return;
  showError(null);
  const payload = await api(`/api/v1/envs/${encodeURIComponent(state.selected.name)}/resolve`);
  el('metadata-output').textContent = json(payload);
  renderReleaseLane({ ...payload.environment, resolved_commit: payload.git?.resolved_commit });
  renderSummaryCards(payload.environment);
}

async function loadStatus() {
  if (!state.selected) return;
  showError(null);
  const payload = await api(`/api/v1/envs/${encodeURIComponent(state.selected.name)}/status`);
  el('metadata-output').textContent = json(payload);
  renderStatus(payload);
}

async function loadDiff() {
  if (!state.selected) return;
  showError(null);
  const payload = await api(`/api/v1/envs/${encodeURIComponent(state.selected.name)}/diff`);
  el('metadata-output').textContent = json({ environment: payload.environment, applied: payload.applied, desired: payload.desired });
  el('diff-output').textContent = formatDiff(payload.diff);
  renderReleaseLane(payload.desired || state.selected);
}

async function loadCluster() {
  if (!state.selected) return;
  showError(null);
  const payload = await api(`/api/v1/envs/${encodeURIComponent(state.selected.name)}/cluster`);
  renderCluster(payload);
}

function renderCluster(payload) {
  el('metadata-output').textContent = json(payload);
  el('cluster-availability').className = `badge ${payload.available ? 'bg-green-lt' : 'bg-red-lt'}`;
  el('cluster-availability').textContent = payload.available ? `namespace ${payload.namespace}` : 'unavailable';
  el('cluster-summary').innerHTML = [
    ['deployments', `${payload.ready_deployments || 0}/${payload.deployment_count || 0} ready`],
    ['pods', `${payload.ready_pods || 0}/${payload.pod_count || 0} ready`],
    ['services', String(payload.service_count || 0)],
  ].map(([label, value]) => `
    <div class="col-12 col-md-4">
      <div class="card summary-tile">
        <div class="card-body py-2">
          <div class="summary-label">${escapeHTML(label)}</div>
          <div class="summary-value">${escapeHTML(value)}</div>
        </div>
      </div>
    </div>
  `).join('');
  if (!payload.available) {
    setTableBody('cluster-deployments', `<tr><td class="text-danger">${escapeHTML(payload.error || 'Cluster unavailable')}</td></tr>`);
    setTableBody('cluster-pods', '<tr><td class="text-muted">No pod data.</td></tr>');
    setTableBody('cluster-services', '<tr><td class="text-muted">No service data.</td></tr>');
    return;
  }
  setTableBody('cluster-deployments', tableRows(payload.deployments || [], (deployment) => `
    <tr><td>${escapeHTML(deployment.name)}</td><td class="text-end">${escapeHTML(`${deployment.ready}/${deployment.desired}`)}</td><td class="text-end">${escapeHTML(deployment.available)}</td></tr>
  `, '<tr><td class="text-muted">No deployments.</td></tr>'));
  setTableBody('cluster-pods', tableRows(payload.pods || [], (pod) => `
    <tr><td>${escapeHTML(pod.name)}</td><td>${escapeHTML(pod.phase)}</td><td class="text-end">${escapeHTML(pod.ready)}</td><td class="text-end">${escapeHTML(pod.restarts)}</td></tr>
  `, '<tr><td class="text-muted">No pods.</td></tr>'));
  setTableBody('cluster-services', tableRows(payload.services || [], (service) => `
    <tr><td>${escapeHTML(service.name)}</td><td>${escapeHTML(service.type)}</td><td>${escapeHTML(service.cluster_ip)}</td><td>${escapeHTML(service.ports)}</td></tr>
  `, '<tr><td class="text-muted">No services.</td></tr>'));
}

function tableRows(items, render, empty) {
  if (!items.length) return empty;
  return items.map(render).join('');
}

function setTableBody(tableID, html) {
  el(tableID).querySelector('tbody').innerHTML = html;
}

function formatDiff(diff) {
  if (!diff) return 'No diff.';
  if (!diff.changed) return 'NoDiff';
  const lines = [];
  for (const file of diff.files || []) {
    lines.push(`file ${file.status} ${file.path}`);
    lines.push(...(file.unified || []));
  }
  return lines.join('\n');
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"]/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[char]));
}

async function refresh() {
  showError(null);
  try {
    await loadInfo();
    await loadEnvs();
  } catch (error) {
    showError(error);
  }
}

el('refresh-btn').addEventListener('click', refresh);
el('resolve-btn').addEventListener('click', () => resolveSelected().catch(showError));
el('status-btn').addEventListener('click', () => loadStatus().catch(showError));
el('cluster-btn').addEventListener('click', () => loadCluster().catch(showError));
el('diff-btn').addEventListener('click', () => loadDiff().catch(showError));
el('env-filter').addEventListener('input', (event) => { state.filter = event.target.value; renderEnvList(); });

refresh();
