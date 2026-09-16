globalThis.templateInput = (() => {
  const tokenPattern = /\{\{(?:(var|env):)?([A-Za-z_][A-Za-z0-9_]*)\}\}/g;
  const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, character => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[character]);

  // Display parsing never resolves or rewrites the source expression.
  function tokenize(source) {
    const text = String(source ?? '');
    const parts = [];
    let cursor = 0;
    for (const match of text.matchAll(tokenPattern)) {
      if (match.index > cursor) parts.push({type:'text', value:text.slice(cursor, match.index)});
      parts.push({type:match[1] || 'passthrough', name:match[2], value:match[0]});
      cursor = match.index + match[0].length;
    }
    if (cursor < text.length) parts.push({type:'text', value:text.slice(cursor)});
    return parts;
  }

  const hasTemplate = source => tokenize(source).some(part => part.type !== 'text');
  function partsHTML(source) {
    const colors = {var:'blue', env:'cyan', passthrough:'yellow'};
    return tokenize(source).map(part => part.type === 'text'
      ? `<span class="template-input-text font-monospace">${escapeHTML(part.value)}</span>`
      : `<span class="badge bg-${colors[part.type]}-lt template-input-token font-monospace">${escapeHTML(part.value)}</span>`).join('');
  }

  function mount(root, options) {
    let source = String(options.source ?? '');
    let mode = hasTemplate(source) ? 'template' : 'raw';
    let selection = null;

    function statusHTML() {
      if (hasTemplate(source)) {
        const legacy = tokenize(source).some(part => part.type === 'passthrough');
        return `<span class="badge bg-${legacy ? 'yellow' : 'blue'}-lt">${legacy ? 'Legacy passthrough' : 'Template source'}</span><span class="text-secondary ms-2">Source is preserved and resolved only by the build context.</span>`;
      }
      const valid = options.kind !== 'port' || (/^\d+$/.test(source.trim()) && Number(source) >= 1 && Number(source) <= 65535);
      return `<span class="badge bg-${valid ? 'secondary' : 'red'}-lt">${valid ? 'Raw value' : 'Invalid port'}</span>${valid ? '' : '<span class="text-danger ms-2">Use an integer from 1 to 65535.</span>'}`;
    }

    function renderVariables() {
      const query = root.querySelector('[data-template-search]').value.toLowerCase().trim();
      const rows = (options.references || []).filter(item => `${item.namespace}:${item.name} ${(item.sources || []).join(' ')}`.toLowerCase().includes(query));
      root.querySelector('[data-template-options]').innerHTML = rows.map(item => {
        const reference = `{{${item.namespace}:${item.name}}}`;
        const context = item.depends_on_application ? '<span class="badge bg-secondary-lt ms-2">Application context</span>' : '';
        return `<button type="button" class="list-group-item list-group-item-action text-start py-2" data-template-reference="${escapeHTML(reference)}"><span class="badge bg-${item.namespace === 'var' ? 'blue' : 'cyan'}-lt me-2">${escapeHTML(item.namespace)}</span><strong class="font-monospace">${escapeHTML(item.name)}</strong>${context}<span class="d-block text-secondary small mt-1">${escapeHTML((item.sources || []).join(', '))}</span></button>`;
      }).join('') || '<div class="p-3 text-secondary small">No matching variables in the current build context.</div>';
    }

    function closePicker() {
      root.querySelector('[data-template-picker]').hidden = true;
      root.querySelector('[data-template-open]')?.focus();
    }

    function openPicker() {
      root.querySelector('[data-template-picker]').hidden = false;
      renderVariables();
      root.querySelector('[data-template-search]').focus();
    }

    function render() {
      root.innerHTML = `<fieldset class="border-0 p-0 m-0" ${options.disabled ? 'disabled' : ''}>
        <legend class="visually-hidden">${escapeHTML(options.label)} value type</legend>
        <div class="d-flex gap-3 mb-2">
          <label class="form-check mb-0"><input class="form-check-input" type="radio" name="template-mode-${escapeHTML(options.id)}" value="template" ${mode === 'template' ? 'checked' : ''}><span class="form-check-label">Template</span></label>
          <label class="form-check mb-0"><input class="form-check-input" type="radio" name="template-mode-${escapeHTML(options.id)}" value="raw" ${mode === 'raw' ? 'checked' : ''}><span class="form-check-label">Raw value</span></label>
        </div>
        <div data-template-display ${mode === 'template' ? '' : 'hidden'}><div class="form-control form-control-sm template-input-parts d-flex flex-wrap align-items-center gap-1" role="textbox" aria-label="${escapeHTML(options.label)}" aria-readonly="true">${partsHTML(source)}</div><div class="form-hint">Read-only source composition. Switch to Raw value to edit it.</div></div>
        <div data-template-raw-view ${mode === 'raw' ? '' : 'hidden'}><div class="d-flex gap-2"><input class="form-control form-control-sm font-monospace" data-template-raw aria-label="${escapeHTML(options.label)}" value="${escapeHTML(source)}" inputmode="${options.kind === 'port' ? 'numeric' : 'text'}"><button class="btn btn-sm btn-outline-primary text-nowrap" type="button" data-template-open>${options.pickerMode === 'replace' ? 'Choose variable' : 'Insert variable'}</button></div><div class="form-hint">${options.pickerMode === 'replace' ? 'Selecting a variable replaces the complete value.' : 'A selected variable is inserted at the cursor.'}</div></div>
        <section class="card card-sm mt-2 shadow-sm" data-template-picker hidden><div class="card-header py-2"><h4 class="card-title small">Choose a variable</h4><button class="btn-close ms-auto" type="button" data-template-close aria-label="Close variable picker"></button></div><div class="card-body pb-2"><input class="form-control form-control-sm" type="search" data-template-search placeholder="Search name or source..." autocomplete="off"></div><div class="list-group list-group-flush template-input-options" data-template-options></div></section>
        <div class="small mt-2" data-template-status>${statusHTML()}</div>
      </fieldset>`;

      root.querySelector('[value="raw"]').addEventListener('change', () => { mode = 'raw'; render(); root.querySelector('[data-template-raw]').focus(); });
      root.querySelector('[value="template"]').addEventListener('change', event => {
        if (hasTemplate(source)) { mode = 'template'; render(); return; }
        event.target.checked = false;
        root.querySelector('[value="raw"]').checked = true;
        openPicker();
      });
      const raw = root.querySelector('[data-template-raw]');
      raw?.addEventListener('input', event => {
        source = event.target.value;
        root.querySelector('[data-template-status]').innerHTML = statusHTML();
        options.onChange(source);
      });
      const open = root.querySelector('[data-template-open]');
      open?.addEventListener('pointerdown', () => {
        if (document.activeElement === raw) selection = {start:raw.selectionStart ?? source.length, end:raw.selectionEnd ?? source.length};
      });
      open?.addEventListener('click', openPicker);
      root.querySelector('[data-template-close]').addEventListener('click', closePicker);
      root.querySelector('[data-template-search]').addEventListener('input', renderVariables);
      root.querySelector('[data-template-picker]').addEventListener('keydown', event => { if (event.key === 'Escape') { event.preventDefault(); closePicker(); } });
      root.querySelector('[data-template-options]').addEventListener('click', event => {
        const button = event.target.closest('[data-template-reference]');
        if (!button) return;
        const reference = button.dataset.templateReference;
        if (options.pickerMode === 'replace') source = reference;
        else {
          const range = selection || {start:source.length, end:source.length};
          source = source.slice(0, range.start) + reference + source.slice(range.end);
        }
        mode = 'template';
        selection = null;
        options.onChange(source);
        render();
      });
    }

    render();
  }

  return {hasTemplate, mount, tokenize};
})();
