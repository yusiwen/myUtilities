<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
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

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">es</span>
  </a>
  <div class="spacer"></div>
  <div class="actions">
    <button class="btn xs icon-btn" onclick={openSettings} aria-label="Connection settings">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 7h-9"/><path d="M14 17H5"/><circle cx="17" cy="17" r="3"/><circle cx="7" cy="7" r="3"/></svg>
    </button>
  </div>
</header>

<div class="app">
  <div class="page-head">
    <h1>Elasticsearch</h1>
    <p>Indices, documents, connections</p>
    <p class="meta conn">
      <span class="dot" class:on={connected} class:off={!connected}></span>
      {#if connected}
        <span class="conn-text">{clusterName || 'Elasticsearch'} {esVersion} · {esHost}</span>
      {:else if esHost}
        <span class="conn-text">{esHost} · {connectionError}</span>
      {:else}
        <span class="conn-text">Not configured</span>
      {/if}
    </p>
  </div>

  {#if error}
    <div class="msg error block">{error}</div>
  {/if}
  {#if success}
    <div class="msg success block">{success}</div>
  {/if}

  <section class="card search">
    <div class="field">
      <label for="index-select">Index</label>
      <div class="select-row">
        <select id="index-select" bind:value={selectedIndex}>
          <option value="">Select an index</option>
          {#each indices as idx}
            <option value={idx}>{idx}</option>
          {/each}
        </select>
        <button class="btn" onclick={fetchIndices}>Refresh</button>
      </div>
    </div>

    <div class="field">
      <label for="query-input">Query (Elasticsearch Query DSL JSON)</label>
      <textarea id="query-input" bind:value={queryText} rows="8" placeholder={placeholderText}></textarea>
      {#if queryError}
        <div class="err">{queryError}</div>
      {/if}
    </div>

    <button class="btn primary" onclick={handleSearch} disabled={searching || !connected}>
      {searching ? 'Searching...' : 'Search'}
    </button>
  </section>

  {#if results !== null}
    <section class="card results">
      <h2>Results
        {#if results.hits}
          <span class="hits">{results.hits.total?.value || results.hits.total || 0} hits</span>
        {/if}
      </h2>
      <pre class="json">{JSON.stringify(results, null, 2)}</pre>
    </section>
  {/if}
</div>

{#if showSettings}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="modal-overlay" role="presentation" onclick={() => showSettings = false}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()}>
      <h2>Connection settings</h2>
      <div class="field">
        <label for="settings-host">Host</label>
        <input id="settings-host" type="text" placeholder="http://localhost:9200" bind:value={settingsHost} />
      </div>
      <div class="field">
        <label for="settings-user">Username</label>
        <input id="settings-user" type="text" placeholder="elastic" bind:value={settingsUser} />
      </div>
      <div class="field">
        <label for="settings-pass">Password</label>
        <input id="settings-pass" type="password" placeholder="password" bind:value={settingsPass} />
      </div>
      <div class="modal-actions">
        <button class="btn primary" onclick={saveSettings}>Save</button>
        <button class="btn" onclick={() => showSettings = false}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

<style>
  :global(body) { --page-max: 960px; }
  .block { margin-bottom: 16px; }

  .page-head .meta { margin-top: 10px; }
  .conn { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .icon-btn { width: 32px; height: 32px; padding: 0; }
  .icon-btn svg { width: 16px; height: 16px; }
  .conn-text { overflow: hidden; color: var(--text2); font-family: var(--mono); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
  .dot { width: 7px; height: 7px; border-radius: 50%; flex: none; background: var(--text3); }
  .dot.on { background: var(--success); }
  .dot.off { background: var(--danger); }

  .search { margin-bottom: 16px; }
  .select-row { display: flex; gap: 8px; }
  .select-row select { flex: 1; }
  .err { margin-top: 6px; color: var(--danger); font-size: 12px; }

  .results h2 { display: flex; align-items: baseline; gap: 8px; margin-bottom: 12px; }
  .hits { color: var(--text3); font-family: var(--mono); font-size: 12px; font-weight: 400; }
  .json {
    max-height: 600px;
    margin: 0;
    padding: 14px 16px;
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--bg);
    font-size: 12.5px;
    line-height: 1.6;
    overflow: auto;
  }

  .modal-overlay { position: fixed; inset: 0; z-index: 200; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(0, 0, 0, 0.55); }
  .modal { width: 440px; max-width: 100%; padding: 24px; border: 1px solid var(--border2); border-radius: var(--r-lg); background: var(--surface); }
  .modal h2 { margin-bottom: 16px; }
  .modal-actions { display: flex; gap: 8px; margin-top: 20px; }
</style>
