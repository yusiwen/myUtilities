<script>
  let aliases = $state({})
  let formName = $state('')
  let formMac = $state('')
  let editingName = $state(null)
  let error = $state('')
  let bootTimes = $state({})
  let success = $state('')
  let wolCooldowns = $state({})
  let wolTimers = $state({})
  let successTimer = $state(null)
  let appVersion = $state('1.0.0')
  let tokenInput = $state('')
  let showTokenModal = $state(false)
  let token = $state(localStorage.getItem('wol-token') || '')

  async function apiFetch(url, options = {}) {
    if (token) {
      options.headers = { ...options.headers, 'X-Auth-Token': token }
    }
    const res = await fetch(url, options)
    if (res.status === 401) {
      error = 'Unauthorized — check your token in settings'
    }
    return res
  }

  async function fetchAliases() {
    try {
      const res = await fetch('/api/aliases')
      if (res.ok) aliases = await res.json()
    } catch (e) {
      error = 'Failed to fetch aliases'
    }
  }

  async function fetchBootTime(name) {
    try {
      const res = await fetch(`/api/boot/${encodeURIComponent(name)}`)
      if (res.ok) {
        const data = await res.json()
        bootTimes[name] = data.boot_time || null
      }
    } catch (e) {
      bootTimes[name] = null
    }
  }

  async function handleSubmit() {
    error = ''
    if (!formName || !formMac) {
      error = 'Name and MAC are required'
      return
    }
    try {
      const res = await apiFetch('/api/aliases', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: formName, mac: formMac })
      })
      if (res.ok) {
        formName = ''
        formMac = ''
        editingName = null
        await fetchAliases()
      } else {
        const data = await res.json()
        error = data.error || 'Failed to save alias'
      }
    } catch (e) {
      error = 'Failed to save alias'
    }
  }

  async function handleDelete(name) {
    error = ''
    if (!confirm(`Delete alias "${name}"?`)) return
    try {
      const res = await apiFetch(`/api/aliases/${encodeURIComponent(name)}`, {
        method: 'DELETE'
      })
      if (res.ok) {
        await fetchAliases()
      } else {
        const data = await res.json()
        error = data.error || 'Failed to delete alias'
      }
    } catch (e) {
      error = 'Failed to delete alias'
    }
  }

  async function handleWol(name) {
    // 检查冷却时间
    if (wolCooldowns[name] > 0) return
    
    error = ''
    try {
      const res = await apiFetch(`/api/wake/${encodeURIComponent(name)}`, {
        method: 'POST'
      })
      if (res.ok) {
        showSuccess(`WOL request sent to ${name}`)
        await fetchBootTime(name)
        startCooldown(name)
      } else {
        const data = await res.json()
        error = data.error || 'WOL request failed'
      }
    } catch (e) {
      error = 'WOL request failed'
    }
  }

  function editAlias(name) {
    const entry = aliases[name]
    formName = name
    formMac = entry.Mac
    editingName = name
  }

  function cancelEdit() {
    formName = ''
    formMac = ''
    editingName = null
  }

  function bootStatus(name) {
    const t = bootTimes[name]
    if (!t) return 'unknown'
    const d = new Date(t)
    const now = new Date()
    const diff = (now - d) / 1000 / 60
    if (diff < 5) return 'just booted'
    if (diff < 60) return `${Math.round(diff)}m ago`
    if (diff < 1440) return `${Math.round(diff / 60)}h ago`
    return d.toLocaleString()
  }

  // 启动倒计时（10秒）
  function startCooldown(name) {
    wolCooldowns[name] = 10
    const timerId = setInterval(() => {
      wolCooldowns[name]--
      if (wolCooldowns[name] <= 0) {
        clearInterval(timerId)
        delete wolCooldowns[name]
        delete wolTimers[name]
      }
    }, 1000)
    wolTimers[name] = timerId
  }

  // 显示成功消息（3秒后自动清除）
  function showSuccess(message) {
    success = message
    if (successTimer) clearTimeout(successTimer)
    successTimer = setTimeout(() => {
      success = ''
      successTimer = null
    }, 3000)
  }

  $effect(() => {
    fetchAliases()
    fetchVersion()
  })

  async function fetchVersion() {
    try {
      const res = await fetch('/version.json')
      const data = await res.json()
      appVersion = data.version || '1.0.0'
    } catch (e) {
      appVersion = '1.0.0'
    }
  }

  function openTokenModal() {
    tokenInput = token
    showTokenModal = true
  }

  function saveToken() {
    if (tokenInput) {
      localStorage.setItem('wol-token', tokenInput)
      token = tokenInput
    } else {
      localStorage.removeItem('wol-token')
      token = ''
    }
    showTokenModal = false
  }

  function clearToken() {
    tokenInput = ''
    localStorage.removeItem('wol-token')
    token = ''
    showTokenModal = false
  }

  $effect(() => {
    const names = Object.keys(aliases)
    names.forEach(fetchBootTime)
  })

  $effect(() => {
    return () => {
      // 清理所有定时器
      Object.values(wolTimers).forEach(timerId => clearInterval(timerId))
      if (successTimer) clearTimeout(successTimer)
    }
  })
</script>

<main>
  <h1>WOL Manager <span class="version">{appVersion}</span>
    <button class="btn-settings" onclick={openTokenModal}>
      <span class="lock-icon">{token ? '\u{1F512}' : '\u{1F513}'}</span>
    </button>
  </h1>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  {#if success}
    <div class="success">{success}</div>
  {/if}

  <section class="form-section">
    <h2>{editingName ? 'Edit Alias' : 'Add Alias'}</h2>
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit() }}>
      <input
        type="text"
        placeholder="Hostname"
        bind:value={formName}
        disabled={editingName !== null}
      />
      <input
        type="text"
        placeholder="MAC Address (e.g. aa:bb:cc:dd:ee:ff)"
        bind:value={formMac}
      />
      <div class="form-actions">
        <button type="submit">{editingName ? 'Update' : 'Add'}</button>
        {#if editingName}
          <button type="button" onclick={cancelEdit}>Cancel</button>
        {/if}
      </div>
    </form>
  </section>

  <section class="list-section">
    <h2>Aliases ({Object.keys(aliases).length})</h2>
    {#if Object.keys(aliases).length === 0}
      <p class="empty">No aliases yet. Add one above.</p>
    {:else}
      <table>
        <thead>
          <tr>
            <th>Hostname</th>
            <th>MAC Address</th>
            <th>Boot Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each Object.entries(aliases) as [name, entry]}
            <tr>
              <td>{name}</td>
              <td class="mono">{entry.Mac}</td>
              <td class="boot-cell">{bootStatus(name)}</td>
              <td class="actions">
                <button 
                  class="btn-wol" 
                  onclick={() => handleWol(name)} 
                  title="Wake on LAN"
                  disabled={wolCooldowns[name] > 0}
                >
                  {#if wolCooldowns[name] > 0}
                    ⏳ WOL ({wolCooldowns[name]}s)
                  {:else}
                    ⚡ WOL
                  {/if}
                </button>
                <button class="btn-edit" onclick={() => editAlias(name)}>✏️ Edit</button>
                <button class="btn-delete" onclick={() => handleDelete(name)}>🗑️ Delete</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
</main>

{#if showTokenModal}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="modal-overlay" role="presentation" onclick={() => showTokenModal = false}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()}>
      <h2>Settings</h2>
      <label for="token-input">API Token</label>
      <input
        id="token-input"
        type="text"
        placeholder="Enter pre-shared token"
        bind:value={tokenInput}
      />
      <div class="modal-actions">
        <button onclick={saveToken}>Save</button>
        <button class="btn-clear" onclick={clearToken}>Clear</button>
        <button class="btn-cancel" onclick={() => showTokenModal = false}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

<footer>
  <p>Created by Siwen Yu (yusiwen@gmail.com)</p>
  <p><a href="https://github.com/yusiwen/myUtilities">https://github.com/yusiwen/myUtilities</a></p>
</footer>

<style>
  :global(*) {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #f5f5f5;
    color: #333;
    line-height: 1.6;
  }

  footer {
    text-align: center;
    color: #bdc3c7;
    padding: 20px;
    font-size: 0.85em;
  }

  footer a {
    color: #bdc3c7;
  }

  footer a:hover {
    color: #95a5a6;
  }

  main {
    max-width: 900px;
    margin: 0 auto;
    padding: 20px;
  }

  h1 {
    font-size: 1.8em;
    color: #2c3e50;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  h1 .version {
    font-size: 0.4em;
    color: #7f8c8d;
    margin-right: auto;
  }

  h2 {
    font-size: 1.2em;
    margin-bottom: 12px;
    color: #34495e;
  }

  .error {
    background: #f8d7da;
    color: #721c24;
    padding: 10px 14px;
    border-radius: 6px;
    margin-bottom: 16px;
    font-size: 0.9em;
  }

  .form-section {
    background: #fff;
    padding: 20px;
    border-radius: 8px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
    margin-bottom: 20px;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  input {
    padding: 10px 12px;
    border: 1px solid #ddd;
    border-radius: 6px;
    font-size: 0.95em;
    outline: none;
    transition: border-color 0.2s;
  }

  input:focus {
    border-color: #3498db;
    box-shadow: 0 0 0 3px rgba(52,152,219,0.1);
  }

  input:disabled {
    background: #f0f0f0;
    color: #666;
  }

  .form-actions {
    display: flex;
    gap: 8px;
  }

  button {
    padding: 8px 16px;
    border: none;
    border-radius: 6px;
    font-size: 0.9em;
    cursor: pointer;
    transition: background 0.2s;
  }

  button[type="submit"] {
    background: #3498db;
    color: #fff;
  }

  button[type="submit"]:hover {
    background: #2980b9;
  }

  button[type="button"] {
    background: #95a5a6;
    color: #fff;
  }

  button[type="button"]:hover {
    background: #7f8c8d;
  }

  .list-section {
    background: #fff;
    padding: 20px;
    border-radius: 8px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  }

  .empty {
    color: #999;
    font-style: italic;
  }

  table {
    width: 100%;
    border-collapse: collapse;
  }

  th {
    text-align: left;
    padding: 10px 8px;
    border-bottom: 2px solid #eee;
    font-size: 0.85em;
    text-transform: uppercase;
    color: #7f8c8d;
  }

  td {
    padding: 10px 8px;
    border-bottom: 1px solid #f0f0f0;
    font-size: 0.95em;
  }

  .mono {
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 0.9em;
  }

  .boot-cell {
    font-size: 0.85em;
    color: #666;
  }

  .actions {
    white-space: nowrap;
  }

  .actions button {
    padding: 4px 8px;
    font-size: 0.85em;
    margin-right: 4px;
  }

  .btn-wol {
    background: #27ae60;
    color: #fff;
  }

  .btn-wol:hover {
    background: #219a52;
  }

  .btn-edit {
    background: #f39c12;
    color: #fff;
  }

  .btn-edit:hover {
    background: #e67e22;
  }

  .btn-delete {
    background: #e74c3c;
    color: #fff;
  }

  .btn-delete:hover {
    background: #c0392b;
  }

  .success {
    background: #d4edda;
    color: #155724;
    padding: 10px 14px;
    border-radius: 6px;
    margin-bottom: 16px;
    font-size: 0.9em;
  }

  .btn-wol:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-settings {
    background: none;
    border: none;
    font-size: 1.2em;
    cursor: pointer;
    padding: 4px;
    line-height: 1;
  }

  .btn-settings:hover {
    opacity: 0.7;
  }

  .lock-icon {
    font-size: 1em;
  }

  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0,0,0,0.4);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .modal {
    background: #fff;
    padding: 24px;
    border-radius: 8px;
    box-shadow: 0 4px 12px rgba(0,0,0,0.15);
    width: 360px;
    max-width: 90vw;
  }

  .modal h2 {
    margin-top: 0;
    margin-bottom: 16px;
  }

  .modal label {
    display: block;
    font-size: 0.85em;
    color: #555;
    margin-bottom: 6px;
  }

  .modal input {
    width: 100%;
    margin-bottom: 16px;
  }

  .modal-actions {
    display: flex;
    gap: 8px;
  }

  .btn-clear {
    background: #e74c3c;
    color: #fff;
  }

  .btn-clear:hover {
    background: #c0392b;
  }

  .btn-cancel {
    background: #95a5a6;
    color: #fff;
  }

  .btn-cancel:hover {
    background: #7f8c8d;
  }
</style>
