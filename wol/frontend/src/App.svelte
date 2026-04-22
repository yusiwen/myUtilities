<script>
  let aliases = $state({})
  let formName = $state('')
  let formMac = $state('')
  let editingName = $state(null)
  let error = $state('')
  let bootTimes = $state({})

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
      const res = await fetch('/api/aliases', {
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
      const res = await fetch(`/api/aliases/${encodeURIComponent(name)}`, {
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
    error = ''
    try {
      const res = await fetch(`/api/wake/${encodeURIComponent(name)}`, {
        method: 'POST'
      })
      if (res.ok) {
        await fetchBootTime(name)
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

  $effect(() => {
    fetchAliases()
  })

  $effect(() => {
    const names = Object.keys(aliases)
    names.forEach(fetchBootTime)
  })
</script>

<main>
  <h1>WOL Manager</h1>

  {#if error}
    <div class="error">{error}</div>
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
                <button class="btn-wol" onclick={() => handleWol(name)} title="Wake on LAN">⚡ WOL</button>
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

  main {
    max-width: 900px;
    margin: 0 auto;
    padding: 20px;
  }

  h1 {
    font-size: 1.8em;
    margin-bottom: 20px;
    color: #2c3e50;
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
</style>
