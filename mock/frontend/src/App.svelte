<script>
  import { onMount } from 'svelte'
  import { listEndpoints, createEndpoint, updateEndpoint, deleteEndpoint, saveToConfig } from './lib/api.js'
  import JsonEditor from './components/JsonEditor.svelte'

  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let endpoints = $state([])
  let error = $state('')
  let success = $state('')
  let saving = $state(false)
  let editing = $state(null)
  let form = $state({ method: 'GET', path: '', status: 200, delay: '', headers: [], body: '' })
  let editorRef = $state(null)

  function emptyForm() {
    return { method: 'GET', path: '', status: 200, delay: '', headers: [], body: '' }
  }

  const methodColors = { GET: '#61affe', POST: '#49cc90', PUT: '#fca130', PATCH: '#50e3c2', DELETE: '#f93e3e' }

  async function load() {
    try {
      endpoints = await listEndpoints()
    } catch (e) {
      error = e.message
    }
  }

  onMount(load)

  function startAdd() {
    editing = 'new'
    form = emptyForm()
  }

  function startEdit(ep) {
    editing = ep.id
    form = {
      method: ep.method,
      path: ep.path,
      status: ep.status,
      delay: ep.delay || '',
      headers: Object.entries(ep.headers || {}).map(([k, v]) => ({ key: k, value: v })),
      body: ep.body || '',
    }
  }

  function cancelEdit() {
    editing = null
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
    return data
  }

  async function handleSave() {
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
        <input type="text" bind:value={form.path} placeholder="/api/example" />
      </label>
    </div>
    <div class="form-row">
      <label>Status
        <input type="number" bind:value={form.status} min="100" max="599" />
      </label>
      <label class="grow">Delay
        <input type="text" bind:value={form.delay} placeholder="e.g. 500ms, 2s" />
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
    <label>Response Body <button class="btn xs" onclick={formatBody} style="float:right">Format</button>
      <div class="body-editor">
        <textarea bind:value={form.body} rows="10" placeholder='response body (JSON or plain text)'></textarea>
      </div>
    </label>
  </div>
{/snippet}

<style>
  .app { max-width: 960px; margin: 0 auto; padding: 24px 16px; }
  .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; flex-wrap: wrap; gap: 12px; }
  .header h1 { font-size: 22px; font-weight: 600; }
  .home-link { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px 3px 6px; border: 1px solid var(--border); border-radius: 20px; background: var(--surface); color: var(--text2); text-decoration: none; font-size: 12px; margin-right: 10px; }
  .home-link:hover { border-color: var(--primary); color: var(--text); }
  .header-actions { display: flex; gap: 8px; }

  .btn { padding: 8px 16px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); cursor: pointer; font-size: 14px; }
  .btn:hover { background: var(--surface2); }
  .btn.primary { background: var(--primary); border-color: var(--primary); color: #fff; }
  .btn.primary:hover { opacity: .85; }
  .btn.danger { border-color: var(--danger); color: var(--danger); }
  .btn.danger:hover { background: var(--danger); color: #fff; }
  .btn.sm { font-size: 12px; padding: 4px 10px; }
  .btn.xs { font-size: 11px; padding: 2px 8px; }
  .btn:disabled { opacity: .5; cursor: not-allowed; }

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
  .form-row input, .form-row select { padding: 8px 10px; border: 1px solid var(--border); border-radius: 4px; background: #0d1117; color: var(--text); font-size: 14px; }
  .form-row select { cursor: pointer; }

  .headers-list { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
  .hr { display: flex; align-items: center; gap: 6px; }
  .hr input { flex: 1; padding: 6px 8px; border: 1px solid var(--border); border-radius: 4px; background: #0d1117; color: var(--text); font-size: 13px; }
  .sep { color: var(--text2); font-size: 13px; }

  .body-editor { margin-top: 4px; }
  .body-editor textarea { width: 100%; padding: 10px; border: 1px solid var(--border); border-radius: 4px; background: #0d1117; color: var(--text); font-size: 13px; font-family: 'Menlo', 'Consolas', monospace; resize: vertical; line-height: 1.5; }

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
</style>
