<script>
  import { onMount } from 'svelte'
  import { listEndpoints, createEndpoint, updateEndpoint, deleteEndpoint, saveToConfig, listLogs } from './lib/api.js'
  import JsonEditor from './components/JsonEditor.svelte'
  import ConditionTree from './components/ConditionTree.svelte'

  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let endpoints = $state([])
  let error = $state('')
  let success = $state('')
  let saving = $state(false)
  let editing = $state(null)
  let tab = $state('endpoints')
  let logs = $state([])
  let form = $state({ method: 'GET', path: '', status: 200, delay: '', headers: [], body: '' })
  let editorRef = $state(null)
  let errors = $state({})

  const exampleEndpoint = `{
  "method": "POST",
  "path": "/api/users/:id",
  "status": 201,
  "delay": "500ms",
  "headers": {"X-Echo": "{{header.x-request-id}}"},
  "body": "{\\"userId\\": \\"{{path.id}}\\", \\"name\\": \\"{{body.name}}\\", \\"page\\": \\"{{query.page}}\\"}"
}`

  function emptyForm() {
    return { method: 'GET', path: '', status: 200, delay: '', headers: [], body: '', conditions: [] }
  }

  async function load() {
    try {
      endpoints = await listEndpoints()
    } catch (e) {
      error = e.message
    }
  }

  async function loadLogsData() {
    try {
      logs = await listLogs()
    } catch (e) {
      // logs API may not be available
    }
  }

  onMount(load)
  onMount(loadLogsData)

  function startAdd() {
    editing = 'new'
    form = emptyForm()
    errors = {}
  }

  function startEdit(ep) {
    editing = ep.id
    errors = {}
    form = {
      method: ep.method,
      path: ep.path,
      status: ep.status,
      delay: ep.delay || '',
      headers: Object.entries(ep.headers || {}).map(([k, v]) => ({ key: k, value: v })),
      body: ep.body || '',
      conditions: ep.responses ? ep.responses.map(r => copyCond(r)) : [],
    }
  }

  function copyCond(r) {
    return {
      condition: r.condition || '',
      status: r.status || '',
      delay: r.delay || '',
      headers: Object.entries(r.headers || {}).map(([k, v]) => ({ key: k, value: v })),
      body: r.body || '',
      _default: r.default || false,
      _uid: Date.now() + Math.random(),
      children: (r.responses || []).map(c => copyCond(c)),
    }
  }

  function cancelEdit() {
    editing = null
  }

  function addCondition() {
    form.conditions = [...form.conditions, {
      condition: '',
      status: '', delay: '', headers: [], body: '',
      _uid: Date.now() + Math.random(),
      children: [],
    }]
  }

  function removeCondition(node) {
    const removeRecursive = (list) => {
      return list.filter(c => c !== node).map(c => ({ ...c, children: removeRecursive(c.children) }))
    }
    form.conditions = removeRecursive(form.conditions)
  }

  function collectCond(r) {
    const obj = {}
    if (r.condition) obj.condition = r.condition
    if (r.status) obj.status = r.status
    if (r.delay) obj.delay = r.delay
    if (r.body) obj.body = r.body
    const hdrs = {}
    for (const h of r.headers) {
      if (h.key.trim()) hdrs[h.key.trim()] = h.value
    }
    if (Object.keys(hdrs).length) obj.headers = hdrs
    if (r.children && r.children.length) {
      obj.responses = r.children.map(c => collectCond(c))
    }
    return obj
  }

  function collectData() {
    const headers = {}
    for (const h of form.headers) {
      if (h.key.trim()) headers[h.key.trim()] = h.value
    }
    const data = { method: form.method, path: form.path, status: form.status }
    if (form.delay) data.delay = form.delay
    if (Object.keys(headers).length) data.headers = headers
    data.body = form.body
    if (form.conditions.length) {
      data.responses = form.conditions.map(c => collectCond(c))
    }
    return data
  }

  async function handleSave() {
    if (!validate()) return
    const data = collectData()
    try {
      saving = true
      if (editing === 'new') {
        await createEndpoint(data)
      } else {
        await updateEndpoint(editing, data)
      }
      editing = null
      await load()
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }

  async function handleDelete(id) {
    if (!confirm('Delete this endpoint?')) return
    try {
      saving = true
      await deleteEndpoint(id)
      if (editing === id) editing = null
      await load()
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }

  function handleSaveConfig() {
    saving = true
    saveToConfig()
      .then(() => { success = 'Saved to config file'; setTimeout(() => success = '', 3000) })
      .catch(e => { error = e.message })
      .finally(() => { saving = false })
  }

  function addHeader() { form.headers = [...form.headers, { key: '', value: '' }] }
  function removeHeader(i) { form.headers = form.headers.filter((_, idx) => idx !== i) }

  function formatBody() {
    try {
      const parsed = JSON.parse(form.body)
      form.body = JSON.stringify(parsed, null, 2)
    } catch { /* ignore */ }
  }

  function validate() {
    const errs = {}

    if (!form.path.trim()) {
      errs.path = 'Path is required'
    } else if (!form.path.trim().startsWith('/')) {
      errs.path = 'Path must start with /'
    }

    const status = Number(form.status)
    if (form.status === '' || form.status === null || form.status === undefined) {
      errs.status = 'Status is required'
    } else if (!Number.isInteger(status) || status < 100 || status > 599) {
      errs.status = 'Status must be between 100 and 599'
    }

    if (form.delay && !/^\d+(\.\d+)?(ms|s|m|h)$/.test(form.delay)) {
      errs.delay = 'Invalid format. Use e.g. 500ms, 2s, 1m'
    }

    const emptyConds = []
    function findEmptyConds(list) {
      for (const c of list) {
        if (!c.condition.trim()) {
          emptyConds.push(c._uid)
        }
        findEmptyConds(c.children)
      }
    }
    findEmptyConds(form.conditions)
    if (emptyConds.length) {
      errs.condErrors = Object.fromEntries(emptyConds.map(uid => [uid, true]))
    }

    errors = errs
    return Object.keys(errs).length === 0
  }
</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">mock</span>
  </a>
</header>

<div class="app">
  <div class="page-head row">
    <div>
      <h1>Mock Server</h1>
      <p>Mock endpoints and responses</p>
    </div>
    <div class="head-actions">
      <button class="btn" onclick={handleSaveConfig} disabled={saving}>{saving ? 'Saving...' : 'Save to config'}</button>
      <button class="btn primary" onclick={startAdd}>Add endpoint</button>
    </div>
  </div>

  {#if error}
    <button class="msg error block" onclick={() => error = ''}>{error}</button>
  {/if}
  {#if success}
    <button class="msg success block" onclick={() => success = ''}>{success}</button>
  {/if}

  <div class="tabs">
    <button class="tab" class:active={tab === 'endpoints'} onclick={() => tab = 'endpoints'}>Endpoints</button>
    <button class="tab" class:active={tab === 'logs'} onclick={() => { tab = 'logs'; loadLogsData() }}>Logs</button>
    <button class="tab" class:active={tab === 'help'} onclick={() => tab = 'help'}>Help</button>
  </div>

  {#if tab === 'endpoints'}

    {#if editing === 'new'}
      <div class="card edit">
        <h2>New endpoint</h2>
        <FormFields {form} {addHeader} {removeHeader} {formatBody} />
        <div class="row-actions">
          <button class="btn primary" onclick={handleSave} disabled={saving}>{saving ? 'Saving...' : 'Create'}</button>
          <button class="btn" onclick={cancelEdit}>Cancel</button>
        </div>
      </div>
    {/if}

    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr><th>Method</th><th>Path</th><th>Status</th><th>Delay</th><th></th></tr>
        </thead>
        <tbody>
          {#each endpoints as ep}
            <tr>
              <td data-label="Method"><span class="method">{ep.method}</span></td>
              <td class="path" data-label="Path">{ep.path}</td>
              <td data-label="Status"><span class="status" class:ok={ep.status >= 200 && ep.status < 300} class:warn={ep.status >= 400 && ep.status < 500} class:err={ep.status >= 500}>{ep.status}</span></td>
              <td class="muted" data-label="Delay">{ep.delay || '—'}</td>
              <td class="actions">
                <button class="btn xs" onclick={() => startEdit(ep)} disabled={editing !== null && editing !== ep.id}>Edit</button>
                <button class="btn xs danger" onclick={() => handleDelete(ep.id)} disabled={saving}>Delete</button>
              </td>
            </tr>
            {#if editing === ep.id}
              <tr class="edit-row">
                <td colspan="5">
                  <FormFields {form} {addHeader} {removeHeader} {formatBody} />
                  <div class="row-actions">
                    <button class="btn primary" onclick={handleSave} disabled={saving}>{saving ? 'Saving...' : 'Save'}</button>
                    <button class="btn" onclick={cancelEdit}>Cancel</button>
                  </div>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
      {#if endpoints.length === 0}
        <div class="empty">No endpoints yet. Add one to get started.</div>
      {/if}
    </div>
  {/if}

  {#if tab === 'logs'}
    <div class="toolbar">
      <span class="count">{logs.length} invocations</span>
      <button class="btn xs" onclick={loadLogsData}>Refresh</button>
    </div>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr><th>Time</th><th>Method</th><th>Path</th><th>Status</th><th>Duration</th><th>Remote IP</th></tr>
        </thead>
        <tbody>
          {#each logs as log}
            <tr>
              <td class="log-time" data-label="Time">{log.timestamp}</td>
              <td><span class="method">{log.method}</span></td>
              <td class="path" data-label="Path">{log.path}</td>
              <td data-label="Status"><span class="status" class:ok={log.status >= 200 && log.status < 300} class:warn={log.status >= 400 && log.status < 500} class:err={log.status >= 500}>{log.status}</span></td>
              <td class="mono muted" data-label="Duration">{log.duration}</td>
              <td class="mono muted" data-label="Remote IP">{log.remoteAddr}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      {#if logs.length === 0}
        <div class="empty">No invocations recorded yet</div>
      {/if}
    </div>
  {/if}

  {#if tab === 'help'}
    <div class="help">
      <div class="card">
        <h2>Template variables</h2>
        <p class="help-desc">Use <code>{"{{"}source.key{"}}"}</code> in the response body or headers to inject values from the incoming request.</p>
        <table class="table">
          <thead><tr><th>Source</th><th>Syntax</th><th>Example</th></tr></thead>
          <tbody>
            <tr><td>URL path param</td><td><code>{"{{"}path.id{"}}"}</code></td><td>URL <code>/api/users/:id</code> → <code>42</code></td></tr>
            <tr><td>Query string</td><td><code>{"{{"}query.page{"}}"}</code></td><td><code>?page=3</code> → <code>3</code></td></tr>
            <tr><td>Request header</td><td><code>{"{{"}header.authorization{"}}"}</code></td><td><code>Authorization: Bearer xxx</code> → <code>Bearer xxx</code></td></tr>
            <tr><td>JSON body (top-level)</td><td><code>&#123;&#123;body.name&#125;&#125;</code></td><td><code>&#123;"name":"alice"&#125;</code> → <code>alice</code></td></tr>
            <tr><td>JSON body (nested)</td><td><code>{"{{"}body.user.address.city{"}}"}</code></td><td>Dot notation for deep fields</td></tr>
          </tbody>
        </table>

        <h3>Example endpoint</h3>
        <pre class="help-code">{exampleEndpoint}</pre>
        <p class="help-desc">Request <code>POST /api/users/42?page=3</code> with body <code>&#123;"name":"alice"&#125;</code> → <code>&#123;"userId":"42","name":"alice","page":"3"&#125;</code></p>
      </div>

      <div class="card">
        <h2>Additional options</h2>
        <table class="table">
          <thead><tr><th>Field</th><th>Format</th><th>Description</th></tr></thead>
          <tbody>
            <tr><td><code>delay</code></td><td><code>"500ms"</code>, <code>"2s"</code>, <code>"1.5m"</code></td><td>Simulated response delay</td></tr>
            <tr><td><code>status</code></td><td><code>200</code>, <code>404</code>, <code>500</code></td><td>Custom HTTP status code</td></tr>
            <tr><td><code>headers</code></td><td><code>&#123;"X-Custom": "value"&#125;</code></td><td>Response headers (supports templates)</td></tr>
          </tbody>
        </table>
      </div>

      <div class="card">
        <h2>Admin API</h2>
        <table class="table">
          <thead><tr><th>Endpoint</th><th>Method</th><th>Description</th></tr></thead>
          <tbody>
            <tr><td><code>/__admin/api/endpoints</code></td><td>GET</td><td>List all endpoints</td></tr>
            <tr><td><code>/__admin/api/endpoints</code></td><td>POST</td><td>Create a new endpoint</td></tr>
            <tr><td><code>/__admin/api/endpoints/:id</code></td><td>PUT</td><td>Update an endpoint</td></tr>
            <tr><td><code>/__admin/api/endpoints/:id</code></td><td>DELETE</td><td>Delete an endpoint</td></tr>
            <tr><td><code>/__admin/api/config</code></td><td>POST</td><td>Save all endpoints to config file</td></tr>
            <tr><td><code>/__admin/api/logs</code></td><td>GET</td><td>Recent invocation logs (up to 200)</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>

{#snippet FormFields()}
  <div class="form">
    <div class="field-row">
      <label class="field method-field">
        <span class="field-label">Method</span>
        <select bind:value={form.method}>
          <option>GET</option><option>POST</option><option>PUT</option><option>PATCH</option><option>DELETE</option>
        </select>
      </label>
      <label class="field grow">
        <span class="field-label">Path</span>
        <input type="text" bind:value={form.path} placeholder="/api/example" class:invalid={errors.path} />
        {#if errors.path}<span class="err">{errors.path}</span>{/if}
      </label>
    </div>
    <div class="field-row">
      <label class="field">
        <span class="field-label">Status</span>
        <input type="number" bind:value={form.status} min="100" max="599" class:invalid={errors.status} />
        {#if errors.status}<span class="err">{errors.status}</span>{/if}
      </label>
      <label class="field">
        <span class="field-label">Delay</span>
        <input type="text" bind:value={form.delay} placeholder="e.g. 500ms, 2s" class:invalid={errors.delay} />
        {#if errors.delay}<span class="err">{errors.delay}</span>{/if}
      </label>
    </div>
    <div class="field">
      <span class="field-label">Headers</span>
      <div class="kv-list">
        {#each form.headers as h, i (i)}
          <div class="kv-row">
            <input type="text" bind:value={h.key} placeholder="Key" />
            <input type="text" bind:value={h.value} placeholder="Value" />
            <button class="btn xs icon-btn" onclick={() => removeHeader(i)} aria-label="Remove header">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>
            </button>
          </div>
        {/each}
        <button class="btn xs add" onclick={addHeader}>Add header</button>
      </div>
    </div>
    <div class="conditions">
      <button class="btn xs" onclick={addCondition}>Add condition</button>
      {#each form.conditions as cond (cond._uid)}
        <ConditionTree node={cond} level={0} onRemove={removeCondition} condErrors={errors.condErrors || {}} />
      {/each}
    </div>
    <div class="field">
      <div class="label-row">
        <span class="field-label">Response body</span>
        <button class="btn xs" onclick={formatBody}>Format</button>
      </div>
      <textarea bind:value={form.body} rows="10" placeholder="Response body (JSON or plain text). This is the default fallback."></textarea>
    </div>
  </div>
{/snippet}

<style>
  :global(body) { --page-max: 960px; }
  .block { margin-bottom: 12px; }
  .count { color: var(--text3); font-family: var(--mono); font-size: 12px; }
  .head-actions, .row-actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .row-actions { margin-top: 16px; }

  .table-wrap { border: 1px solid var(--border); border-radius: var(--r-md); overflow: auto; }
  .table .path { font-family: var(--mono); font-size: 12.5px; }
  .table .method { color: var(--text2); font-family: var(--mono); font-size: 11px; }
  .table .mono { font-family: var(--mono); font-size: 12px; white-space: nowrap; }
  .table .muted { color: var(--text3); font-size: 12px; }
  .table .actions { white-space: nowrap; text-align: right; }
  .table .actions .btn + .btn { margin-left: 6px; }
  .table .status { font-family: var(--mono); font-size: 12px; }
  .table .status.ok { color: var(--success); }
  .table .status.warn { color: var(--warning); }
  .table .status.err { color: var(--danger); }

  .edit { margin-bottom: 16px; }
  .edit h2 { margin-bottom: 16px; }
  .edit-row td { padding: 0; background: var(--highlight); }
  .edit-row .form { padding: 16px 20px; }

  .toolbar { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }

  .form { display: flex; flex-direction: column; gap: 16px; }
  .field-label { display: block; margin-bottom: 6px; color: var(--text2); font-size: 12px; font-weight: 500; }
  .label-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 6px; }
  .label-row .field-label { margin-bottom: 0; }
  .field-row .field { margin-bottom: 0; }
  .grow { flex: 2; }
  .method-field { flex: 0 0 140px; }
  .invalid { border-color: var(--danger); }
  .err { display: block; margin-top: 4px; color: var(--danger); font-size: 12px; }

  .kv-list, .conditions { display: flex; flex-direction: column; gap: 8px; }
  .kv-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) auto; gap: 8px; align-items: center; }
  .kv-row input { min-height: 32px; padding: 6px 10px; font-family: var(--mono); font-size: 12.5px; }
  .icon-btn { width: 26px; padding: 0; }
  .add { align-self: flex-start; }

  .help { display: flex; flex-direction: column; gap: 16px; }
  .help h2 { margin-bottom: 8px; }
  .help h3 { margin: 24px 0 8px; }
  .help-desc { margin: 0 0 12px; color: var(--text2); font-size: 13px; }
  .help-desc code, .table code {
    padding: 1px 5px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--surface2);
    font-size: 12px;
  }
  .help-code {
    margin: 0 0 12px;
    padding: 12px 14px;
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--bg);
    font-size: 12.5px;
    line-height: 1.55;
    white-space: pre;
    overflow-x: auto;
  }
  .help .table td:first-child { white-space: nowrap; }

  @media (max-width: 700px) {
    .head-actions { width: 100%; }
    .head-actions .btn { flex: 1; }
    .method-field { flex: 1 1 100%; }
    .edit-row .form { padding: 12px; }

    /* Endpoint and log rows become cards so the row actions stay reachable. */
    .table-wrap { border: 0; overflow: visible; }
    .table-wrap table, .table-wrap thead, .table-wrap tbody,
    .table-wrap th, .table-wrap td, .table-wrap tr { display: block; }
    .table-wrap thead { display: none; }
    .table-wrap tbody tr {
      margin-bottom: 12px;
      padding: 12px 14px;
      border: 1px solid var(--border);
      border-radius: 12px;
      background: var(--surface);
    }
    .table-wrap td { border: 0; padding: 2px 0; }
    .table-wrap td[data-label] { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
    .table-wrap td[data-label]::before { content: attr(data-label); flex: none; color: var(--text2); font-size: 12px; }
    .table-wrap td.path { overflow-wrap: anywhere; text-align: right; }
    .table-wrap .actions { display: flex; gap: 8px; margin-top: 10px; text-align: left; }
    .table-wrap .actions .btn { flex: 1; }
    .table-wrap tr.edit-row { padding: 0; border: 0; background: transparent; }
    .table-wrap tr.edit-row td { padding: 0; }
    .card { overflow-x: auto; }
  }
</style>
