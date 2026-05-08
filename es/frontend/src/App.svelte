<script>
  let connected = $state(false)
  let connectionError = $state('')
  let esHost = $state('')
  let clusterName = $state('')
  let esVersion = $state('')

  let settingsHost = $state('')
  let settingsUser = $state('')
  let settingsPass = $state('')
  let showSettings = $state(false)

  let indices = $state([])
  let selectedIndex = $state('')
  let queryText = $state('{"query": {"match_all": {}}}')
  let queryError = $state('')
  let results = $state(null)
  let searching = $state(false)
  let error = $state('')
  let success = $state('')
  let appVersion = $state('1.0.0')

  let successTimer = $state(null)
  let placeholderText = `{"query": {"match_all": {}}}`

  async function fetchVersion() {
    try {
      const res = await fetch('./version.json')
      const data = await res.json()
      appVersion = data.version || '1.0.0'
    } catch (e) {
      appVersion = '1.0.0'
    }
  }

  async function fetchStatus() {
    try {
      const res = await fetch('/api/status')
      const data = await res.json()
      connected = data.connected
      esHost = data.host || ''
      connectionError = data.error || ''
      if (data.info) {
        clusterName = data.info.cluster_name || ''
        esVersion = data.info.version?.number || ''
      }
    } catch (e) {
      connected = false
      connectionError = 'Failed to reach server'
    }
  }

  async function fetchConfig() {
    try {
      const res = await fetch('/api/config')
      const data = await res.json()
      settingsHost = data.host || ''
      settingsUser = data.username || ''
      settingsPass = data.password || ''
    } catch (e) {
      // ignore
    }
  }

  async function fetchIndices() {
    if (!connected) return
    try {
      const res = await fetch('/api/indices')
      const data = await res.json()
      indices = data.indices || []
    } catch (e) {
      indices = []
    }
  }

  async function saveSettings() {
    error = ''
    try {
      const res = await fetch('/api/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          host: settingsHost,
          username: settingsUser,
          password: settingsPass
        })
      })
      if (res.ok) {
        showSettings = false
        showSuccess('Settings saved')
        await fetchStatus()
        await fetchIndices()
      } else {
        const data = await res.json()
        error = data.error || 'Failed to save settings'
      }
    } catch (e) {
      error = 'Failed to save settings'
    }
  }

  async function handleSearch() {
    error = ''
    queryError = ''
    results = null
    searching = true

    if (!selectedIndex) {
      error = 'Please select an index'
      searching = false
      return
    }

    let body
    try {
      body = JSON.parse(queryText)
    } catch (e) {
      queryError = 'Invalid JSON: ' + e.message
      searching = false
      return
    }

    try {
      const res = await fetch('/api/search', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ index: selectedIndex, body })
      })
      if (res.ok) {
        results = await res.json()
      } else {
        const data = await res.json()
        error = data.error || 'Search failed'
      }
    } catch (e) {
      error = 'Search request failed'
    }
    searching = false
  }

  function showSuccess(msg) {
    success = msg
    if (successTimer) clearTimeout(successTimer)
    successTimer = setTimeout(() => {
      success = ''
      successTimer = null
    }, 3000)
  }

  function openSettings() {
    settingsHost = esHost || 'http://localhost:9200'
    fetchConfig()
    showSettings = true
  }

  $effect(() => {
    fetchStatus()
    fetchConfig()
    fetchVersion()
  })

  $effect(() => {
    if (connected) {
      fetchIndices()
    }
  })

  $effect(() => {
    return () => {
      if (successTimer) clearTimeout(successTimer)
    }
  })
</script>

<main>
  <header>
    <h1><span>ES Search</span> <span class="version">{appVersion}</span></h1>
    <div class="conn-bar">
      <span class="conn-dot" class:connected class:disconnected={!connected}></span>
      {#if connected}
        <span class="conn-info">{clusterName || 'Elasticsearch'} ({esVersion}) @ {esHost}</span>
      {:else if esHost}
        <span class="conn-info">Disconnected @ {esHost} — {connectionError}</span>
      {:else}
        <span class="conn-info">Not configured</span>
      {/if}
      <button class="btn-settings" onclick={openSettings} title="Settings">&#9881;</button>
    </div>
  </header>

  {#if error}
    <div class="error">{error}</div>
  {/if}
  {#if success}
    <div class="success">{success}</div>
  {/if}

  <section class="search-section">
    <div class="search-controls">
      <div class="field">
        <label for="index-select">Index</label>
        <div class="select-row">
          <select id="index-select" bind:value={selectedIndex}>
            <option value="">-- Select index --</option>
            {#each indices as idx}
              <option value={idx}>{idx}</option>
            {/each}
          </select>
          <button class="btn-refresh" onclick={fetchIndices} title="Refresh indices">&#8635;</button>
        </div>
      </div>
    </div>

    <div class="field">
      <label for="query-input">Query (Elasticsearch Query DSL JSON)</label>
      <textarea
        id="query-input"
        bind:value={queryText}
        rows="8"
        placeholder={placeholderText}
      ></textarea>
      {#if queryError}
        <div class="field-error">{queryError}</div>
      {/if}
    </div>

    <button class="btn-search" onclick={handleSearch} disabled={searching || !connected}>
      {searching ? 'Searching...' : 'Search'}
    </button>
  </section>

  {#if results !== null}
    <section class="results-section">
      <h2>
        Results
        {#if results.hits}
          <span class="hits-count">({results.hits.total?.value || results.hits.total || 0} hits)</span>
        {/if}
      </h2>
      <pre class="results-json">{JSON.stringify(results, null, 2)}</pre>
    </section>
  {/if}
</main>

<footer>
  <p>Created by Siwen Yu (yusiwen@gmail.com)</p>
  <p><a href="https://github.com/yusiwen/myUtilities">https://github.com/yusiwen/myUtilities</a></p>
</footer>

{#if showSettings}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="modal-overlay" role="presentation" onclick={() => showSettings = false}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()}>
      <h2>ES Connection Settings</h2>
      <label for="settings-host">Host</label>
      <input id="settings-host" type="text" placeholder="http://localhost:9200" bind:value={settingsHost} />
      <label for="settings-user">Username</label>
      <input id="settings-user" type="text" placeholder="elastic" bind:value={settingsUser} />
      <label for="settings-pass">Password</label>
      <input id="settings-pass" type="password" placeholder="password" bind:value={settingsPass} />
      <div class="modal-actions">
        <button onclick={saveSettings}>Save</button>
        <button class="btn-cancel" onclick={() => showSettings = false}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

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
    max-width: 960px;
    margin: 0 auto;
    padding: 20px;
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 20px;
    flex-wrap: wrap;
    gap: 10px;
  }

  h1 {
    font-size: 1.6em;
    color: #2c3e50;
    display: flex;
    align-items: flex-end;
    gap: 4px;
  }

  h1 > span {
    display: inline-flex;
    align-items: flex-end;
  }

  h1 .version {
    font-size: 0.5em;
    color: #7f8c8d;
  }

  .conn-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    background: #fff;
    padding: 6px 14px;
    border-radius: 20px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  }

  .conn-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .conn-dot.connected {
    background: #27ae60;
  }

  .conn-dot.disconnected {
    background: #e74c3c;
  }

  .conn-info {
    font-size: 0.85em;
    color: #555;
  }

  .btn-settings {
    background: none;
    border: none;
    font-size: 1.3em;
    cursor: pointer;
    padding: 2px 4px;
    color: #555;
  }

  .btn-settings:hover {
    color: #222;
  }

  .error {
    background: #f8d7da;
    color: #721c24;
    padding: 10px 14px;
    border-radius: 6px;
    margin-bottom: 16px;
    font-size: 0.9em;
  }

  .success {
    background: #d4edda;
    color: #155724;
    padding: 10px 14px;
    border-radius: 6px;
    margin-bottom: 16px;
    font-size: 0.9em;
  }

  .search-section {
    background: #fff;
    padding: 20px;
    border-radius: 8px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
    margin-bottom: 20px;
  }

  .search-controls {
    margin-bottom: 16px;
  }

  .field {
    margin-bottom: 16px;
  }

  .field label {
    display: block;
    font-size: 0.85em;
    color: #555;
    margin-bottom: 6px;
    font-weight: 500;
  }

  .select-row {
    display: flex;
    gap: 6px;
  }

  select {
    flex: 1;
    padding: 10px 12px;
    border: 1px solid #ddd;
    border-radius: 6px;
    font-size: 0.95em;
    outline: none;
    background: #fff;
    transition: border-color 0.2s;
    cursor: pointer;
  }

  select:focus {
    border-color: #3498db;
    box-shadow: 0 0 0 3px rgba(52,152,219,0.1);
  }

  .btn-refresh {
    background: #95a5a6;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 0 12px;
    font-size: 1.1em;
    cursor: pointer;
  }

  .btn-refresh:hover {
    background: #7f8c8d;
  }

  textarea {
    width: 100%;
    padding: 12px;
    border: 1px solid #ddd;
    border-radius: 6px;
    font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
    font-size: 0.9em;
    outline: none;
    resize: vertical;
    transition: border-color 0.2s;
  }

  textarea:focus {
    border-color: #3498db;
    box-shadow: 0 0 0 3px rgba(52,152,219,0.1);
  }

  .field-error {
    color: #e74c3c;
    font-size: 0.85em;
    margin-top: 4px;
  }

  .btn-search {
    background: #3498db;
    color: #fff;
    border: none;
    padding: 10px 24px;
    border-radius: 6px;
    font-size: 0.95em;
    cursor: pointer;
    transition: background 0.2s;
  }

  .btn-search:hover {
    background: #2980b9;
  }

  .btn-search:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .results-section {
    background: #fff;
    padding: 20px;
    border-radius: 8px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  }

  .results-section h2 {
    font-size: 1.2em;
    color: #2c3e50;
    margin-bottom: 12px;
  }

  .hits-count {
    font-size: 0.8em;
    color: #7f8c8d;
    font-weight: normal;
  }

  .results-json {
    background: #2c3e50;
    color: #ecf0f1;
    padding: 16px;
    border-radius: 6px;
    overflow-x: auto;
    font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
    font-size: 0.85em;
    line-height: 1.5;
    max-height: 600px;
    overflow-y: auto;
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
    width: 420px;
    max-width: 90vw;
  }

  .modal h2 {
    margin-bottom: 16px;
    font-size: 1.2em;
    color: #2c3e50;
  }

  .modal label {
    display: block;
    font-size: 0.85em;
    color: #555;
    margin-bottom: 4px;
    margin-top: 10px;
  }

  .modal label:first-of-type {
    margin-top: 0;
  }

  .modal input {
    width: 100%;
    padding: 8px 10px;
    border: 1px solid #ddd;
    border-radius: 6px;
    font-size: 0.9em;
    outline: none;
  }

  .modal input:focus {
    border-color: #3498db;
    box-shadow: 0 0 0 3px rgba(52,152,219,0.1);
  }

  .modal-actions {
    display: flex;
    gap: 8px;
    margin-top: 18px;
  }

  .modal-actions button {
    padding: 8px 16px;
    border: none;
    border-radius: 6px;
    font-size: 0.9em;
    cursor: pointer;
    transition: background 0.2s;
  }

  .modal-actions button:first-child {
    background: #3498db;
    color: #fff;
  }

  .modal-actions button:first-child:hover {
    background: #2980b9;
  }

  .btn-cancel {
    background: #95a5a6;
    color: #fff;
  }

  .btn-cancel:hover {
    background: #7f8c8d;
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
</style>
