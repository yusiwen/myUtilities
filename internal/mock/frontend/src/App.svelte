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

  const methodColors = { GET: '#61affe', POST: '#49cc90', PUT: '#fca130', PATCH: '#50e3c2', DELETE: '#f93e3e' }

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

<div class="app">
  <header class="header">
    <h1>
      {#if inGateway}
        <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
      {/if}
      Dynamic Mock Server
    </h1>
    <div class="header-actions">
      <button class="btn primary" onclick={startAdd}>+ Add Endpoint</button>
      <button class="btn" onclick={handleSaveConfig} disabled={saving}>
        {saving ? 'Saving...' : 'Save to Config'}
      </button>
    </div>
  </header>

  {#if error}
    <button class="toast error" onclick={() => error = ''}>{error}</button>
  {/if}
  {#if success}
    <button class="toast success" onclick={() => success = ''}>{success}</button>
  {/if}

  <div class="tabs">
    <button class="tab" class:active={tab === 'endpoints'} onclick={() => tab = 'endpoints'}>Endpoints</button>
    <button class="tab" class:active={tab === 'logs'} onclick={() => { tab = 'logs'; loadLogsData() }}>Logs</button>
    <button class="tab" class:active={tab === 'help'} onclick={() => tab = 'help'}>Help</button>
  </div>

  {#if tab === 'endpoints'}

  {#if editing === 'new'}
    <div class="edit-panel">
      <h2>New Endpoint</h2>
      <FormFields {form} {addHeader} {removeHeader} {formatBody} />
      <div class="edit-actions">
        <button class="btn primary" onclick={handleSave} disabled={saving}>{saving ? 'Saving...' : 'Create'}</button>
        <button class="btn" onclick={cancelEdit}>Cancel</button>
      </div>
    </div>
  {/if}

  <div class="table-wrap">
    <table>
      <thead>
        <tr><th>Method</th><th>Path</th><th>Status</th><th>Delay</th><th>Actions</th></tr>
      </thead>
      <tbody>
        {#each endpoints as ep}
          <tr>
            <td><span class="badge" style="background: {methodColors[ep.method] || '#999'}">{ep.method}</span></td>
            <td class="path">{ep.path}</td>
            <td>{ep.status}</td>
            <td>{ep.delay || '—'}</td>
            <td class="actions">
              <button class="btn sm" onclick={() => startEdit(ep)} disabled={editing !== null && editing !== ep.id}>Edit</button>
              <button class="btn sm danger" onclick={() => handleDelete(ep.id)} disabled={saving}>Del</button>
            </td>
          </tr>
          {#if editing === ep.id}
            <tr class="edit-row">
              <td colspan="5">
                <div class="edit-panel inline">
                  <FormFields {form} {addHeader} {removeHeader} {formatBody} />
                  <div class="edit-actions">
                    <button class="btn primary" onclick={handleSave} disabled={saving}>{saving ? 'Saving...' : 'Save'}</button>
                    <button class="btn" onclick={cancelEdit}>Cancel</button>
                  </div>
                </div>
              </td>
            </tr>
          {/if}
        {/each}
      </tbody>
    </table>
    {#if endpoints.length === 0}
      <div class="empty">No endpoints defined. Click "+ Add Endpoint" to create one.</div>
    {/if}
  </div>
  {/if}

  {#if tab === 'logs'}
    <div class="table-wrap">
      <div class="log-bar">
        <span class="count">{logs.length} invocation(s)</span>
        <button class="btn sm" onclick={loadLogsData}>🔄</button>
      </div>
      <table>
        <thead>
          <tr><th>Time</th><th>Method</th><th>Path</th><th>Status</th><th>Duration</th><th>Remote IP</th></tr>
        </thead>
        <tbody>
          {#each logs as log}
            <tr>
              <td class="log-time">{log.timestamp}</td>
              <td><span class="badge" style="background: {methodColors[log.method] || '#999'}">{log.method}</span></td>
              <td class="path">{log.path}</td>
              <td>{log.status}</td>
              <td class="log-dur">{log.duration}</td>
              <td class="log-ip">{log.remoteAddr}</td>
            </tr>
          {:else}
            <tr><td colspan="6" class="empty">No invocations recorded yet.</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if tab === 'help'}
    <div class="help-section">
      <div class="help-card">
        <h2>Template Variables</h2>
        <p class="help-desc">Use <code>{"{{"}source.key{"}}"}</code> in the response body or headers to
        dynamically inject values from the incoming request.</p>
        <table class="help-table">
          <thead><tr><th>Source</th><th>Syntax</th><th>Example</th></tr></thead>
          <tbody>
            <tr><td>URL path param</td><td><code>{"{{"}path.id{"}}"}</code></td><td>Url <code>/api/users/:id</code> → <code>42</code></td></tr>
            <tr><td>Query string</td><td><code>{"{{"}query.page{"}}"}</code></td><td><code>?page=3</code> → <code>3</code></td></tr>
            <tr><td>Request header</td><td><code>{"{{"}header.authorization{"}}"}</code></td><td><code>Authorization: Bearer xxx</code> → <code>Bearer xxx</code></td></tr>
            <tr><td>JSON body (top-level)</td><td><code>&#123;&#123;body.name&#125;&#125;</code></td><td><code>&#123;"name":"alice"&#125;</code> → <code>alice</code></td></tr>
            <tr><td>JSON body (nested)</td><td><code>{"{{"}body.user.address.city{"}}"}</code></td><td>Dot notation for deep fields</td></tr>
          </tbody>
        </table>

        <h3>Example Endpoint</h3>
        <div class="help-example"><pre>{exampleEndpoint}</pre></div>
          <p class="help-desc">Request <code>POST /api/users/42?page=3</code> with body
          <code>&#123;"name":"alice"&#125;</code> → <code>&#123;"userId":"42","name":"alice","page":"3"&#125;</code></p>
      </div>

      <div class="help-card">
        <h2>Additional Options</h2>
        <table class="help-table">
          <thead><tr><th>Field</th><th>Format</th><th>Description</th></tr></thead>
          <tbody>
            <tr><td><code>delay</code></td><td><code>"500ms"</code>, <code>"2s"</code>, <code>"1.5m"</code></td><td>Simulated response delay</td></tr>
            <tr><td><code>status</code></td><td><code>200</code>, <code>404</code>, <code>500</code></td><td>Custom HTTP status code</td></tr>
              <tr><td><code>headers</code></td><td><code>&#123;"X-Custom": "value"&#125;</code></td><td>Response headers (supports templates)</td></tr>
          </tbody>
        </table>
      </div>

      <div class="help-card">
        <h2>Admin API</h2>
        <table class="help-table">
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
    <div class="form-row">
      <label>Method
        <select bind:value={form.method}>
          <option>GET</option><option>POST</option><option>PUT</option><option>PATCH</option><option>DELETE</option>
        </select>
      </label>
      <label class="grow">Path
        <input type="text" bind:value={form.path} placeholder="/api/example" class:field-error={errors.path} />
        {#if errors.path}<span class="err-msg">{errors.path}</span>{/if}
      </label>
    </div>
    <div class="form-row">
      <label>Status
        <input type="number" bind:value={form.status} min="100" max="599" class:field-error={errors.status} />
        {#if errors.status}<span class="err-msg">{errors.status}</span>{/if}
      </label>
      <label class="grow">Delay
        <input type="text" bind:value={form.delay} placeholder="e.g. 500ms, 2s" class:field-error={errors.delay} />
        {#if errors.delay}<span class="err-msg">{errors.delay}</span>{/if}
      </label>
    </div>
    <label>Headers
      <div class="headers-list">
        {#each form.headers as h, i (i)}
          <div class="hr">
            <input type="text" bind:value={h.key} placeholder="Key" />
            <span class="sep">:</span>
            <input type="text" bind:value={h.value} placeholder="Value" />
            <button class="btn xs" onclick={() => removeHeader(i)}>x</button>
          </div>
        {/each}
        <button class="btn xs" onclick={addHeader}>+ Add Header</button>
      </div>
    </label>
    <div class="cond-section">
      <button class="btn xs" onclick={addCondition}>+ Add Condition</button>
    </div>
    {#each form.conditions as cond (cond._uid)}
      <ConditionTree node={cond} level={0} onRemove={removeCondition} condErrors={errors.condErrors || {}} />
    {/each}
    <label>Response Body <button class="btn xs" onclick={formatBody} style="float:right">Format</button>
      <div class="body-editor">
        <textarea bind:value={form.body} rows="10" placeholder='response body (JSON or plain text) — this IS the default fallback'></textarea>
      </div>
    </label>
  </div>
{/snippet}

<style>
  .app { max-width: 960px; margin: 0 auto; padding: 24px 16px; }
  .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; flex-wrap: wrap; gap: 12px; }
  .header h1 { font-size: 22px; font-weight: 600; }
  .header-actions { display: flex; gap: 8px; }

  .btn { padding: 8px 16px; }
  .btn.danger { border-color: var(--danger); color: var(--danger); }
  .btn.danger:hover { background: var(--danger); color: #fff; }
  .btn.sm { font-size: 12px; padding: 4px 10px; }

  .toast { padding: 10px 16px; border-radius: 6px; margin-bottom: 12px; font-size: 14px; cursor: pointer; }
  .toast.error { background: #3d1f2a; border: 1px solid var(--danger); color: var(--danger); }
  .toast.success { background: #1a3d2a; border: 1px solid var(--success); color: var(--success); }

  .edit-panel { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 20px; margin-bottom: 16px; }
  .edit-panel.inline { margin: 0; }
  .edit-panel h2 { font-size: 16px; margin-bottom: 12px; }

  .form { display: flex; flex-direction: column; gap: 12px; }
  .form-row { display: flex; gap: 12px; }
  .form-row label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; color: var(--text2); flex: 1; }
  .form-row label.grow { flex: 2; }
  .form-row input, .form-row select { padding: 8px 10px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 14px; }
  .form-row select { cursor: pointer; }

  .headers-list { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
  .hr { display: flex; align-items: center; gap: 6px; }
  .hr input { flex: 1; padding: 6px 8px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 13px; }
  .sep { color: var(--text2); font-size: 13px; }

  .body-editor { margin-top: 4px; }
  .body-editor textarea { width: 100%; padding: 10px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 13px; font-family: 'Menlo', 'Consolas', monospace; resize: vertical; line-height: 1.5; }

  .edit-actions { display: flex; gap: 8px; margin-top: 16px; }

  .table-wrap { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
  table { width: 100%; border-collapse: collapse; }
  th { text-align: left; padding: 12px 16px; font-size: 12px; text-transform: uppercase; letter-spacing: .5px; color: var(--text2); border-bottom: 1px solid var(--border); }
  td { padding: 12px 16px; font-size: 14px; border-bottom: 1px solid var(--border); }
  tr:last-child td { border-bottom: none; }
  .path { font-family: 'Menlo', 'Consolas', monospace; font-size: 13px; }
  .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; color: #fff; font-size: 11px; font-weight: 700; }
  .actions { white-space: nowrap; }
  .actions .btn + .btn { margin-left: 4px; }
  .empty { padding: 32px; text-align: center; color: var(--text2); font-size: 14px; }

  .tabs { display: flex; gap: 0; margin-bottom: 16px; border-bottom: 1px solid var(--border); }
  .tab { padding: 10px 20px; border: none; background: none; color: var(--text2); cursor: pointer; font-size: 14px; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .tab.active { color: var(--text); border-bottom-color: var(--primary); }
  .tab:hover { color: var(--text); }

  .log-bar { display: flex; align-items: center; justify-content: space-between; padding: 8px 16px; border-bottom: 1px solid var(--border); }
  .log-bar .count { font-size: 13px; color: var(--text3); }
  .log-bar .btn.sm { padding: 4px 8px; font-size: 14px; }

  .log-time { font-size: 12px; color: var(--text2); white-space: nowrap; }
  .log-dur { font-family: 'Menlo', 'Consolas', monospace; font-size: 12px; color: var(--text2); white-space: nowrap; }
  .log-ip { font-family: 'Menlo', 'Consolas', monospace; font-size: 12px; color: var(--text3); white-space: nowrap; }

  .help-section { display: flex; flex-direction: column; gap: 16px; }
  .help-card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; }
  .help-card h2 { margin: 0 0 8px; font-size: 1.1rem; color: var(--text); }
  .help-card h3 { margin: 16px 0 6px; font-size: 0.95rem; color: var(--text); }
  .help-desc { font-size: 14px; color: var(--text2); margin: 0 0 12px; }
  .help-desc code { background: var(--surface2); padding: 2px 6px; border-radius: 4px; font-size: 13px; }
  .help-table { width: 100%; border-collapse: collapse; font-size: 14px; }
  .help-table th, .help-table td { text-align: left; padding: 8px 12px; border-bottom: 1px solid var(--border); }
  .help-table th { color: var(--text2); font-weight: 600; font-size: 13px; }
  .help-table td { color: var(--text); }
  .help-table code { background: var(--surface2); padding: 2px 5px; border-radius: 3px; font-family: 'Menlo','Consolas',monospace; font-size: 13px; }
  .help-example { background: var(--surface2); border: 1px solid var(--border); border-radius: 8px; padding: 16px; font-family: 'Menlo','Consolas',monospace; font-size: 13px; line-height: 1.5; white-space: pre; overflow-x: auto; color: var(--text); }

  .cond-section { margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--border); }

  .field-error { border-color: var(--primary) !important; }
  .err-msg { color: var(--primary); font-size: 12px; margin-top: 2px; display: block; }
</style>

