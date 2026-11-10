<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let aliases = $state({})
  let formName = $state('')
  let formMac = $state('')
  let editingName = $state(null)
  let error = $state('')
  let bootTimes = $state({})
  let hoveredCell = $state(null)
  let success = $state('')
  let wolCooldowns = $state({})
  let refreshing = $state(false)
  let wolTimers = $state({})
  let successTimer = $state(null)
  let appVersion = $state('1.0.0')
  let tokenInput = $state('')
  let showTokenModal = $state(false)
  let token = $state(localStorage.getItem('wol-token') || '')

  function loadFavorites() {
    try {
      return JSON.parse(localStorage.getItem('wol-favorites') || '[]')
    } catch {
      return []
    }
  }
  let favorites = $state(loadFavorites())

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
      const res = await fetch(`/api/notify/${encodeURIComponent(name)}`)
      if (res.ok) {
        const data = await res.json()
        bootTimes[name] = data
      }
    } catch (e) {
      bootTimes[name] = null
    }
  }

  async function refreshAllBootStatus() {
    refreshing = true
    const names = Object.keys(aliases)
    await Promise.all(names.map(fetchBootTime))
    refreshing = false
  }

  function timeAgo(dateStr) {
    if (!dateStr) return null
    const d = new Date(dateStr)
    const now = new Date()
    const diffMin = (now - d) / 1000 / 60
    if (diffMin < 0) return null
    if (diffMin < 1) return 'just now'
    if (diffMin < 60) return `${Math.round(diffMin)}m ago`
    const h = diffMin / 60
    if (h < 24) return `${Math.round(h)}h ago`
    const days = h / 24
    if (days < 30) return `${Math.round(days)}d ago`
    const mo = days / 30
    if (mo < 12) return `${Math.round(mo)}mo ago`
    return `${Math.round(days / 365)}y ago`
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

  function toggleFavorite(name) {
    const idx = favorites.indexOf(name)
    if (idx >= 0) {
      favorites = [...favorites.slice(0, idx), ...favorites.slice(idx + 1)]
    } else {
      favorites = [...favorites, name]
    }
  }

  let sortedEntries = $derived.by(() => {
    const entries = Object.entries(aliases)
    entries.sort(([a], [b]) => {
      const fa = favorites.includes(a) ? 0 : 1
      const fb = favorites.includes(b) ? 0 : 1
      return fa - fb
    })
    return entries
  })

  function bootStatus(name) {
    const info = bootTimes[name]
    if (!info || !info.events || info.events.length === 0) return { state: 'unknown', ago: '' }
    const latest = info.events[0]
    const ago = timeAgo(latest.timestamp)
    if (latest.type === 'boot') return { state: 'up', ago }
    if (latest.type === 'shutdown') return { state: 'down', ago }
    return { state: 'unknown', ago: '' }
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
      const res = await fetch('./version.json')
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
      Object.values(wolTimers).forEach(timerId => clearInterval(timerId))
      if (successTimer) clearTimeout(successTimer)
    }
  })

  $effect(() => {
    localStorage.setItem('wol-favorites', JSON.stringify(favorites))
  })

  $effect(() => {
    const names = Object.keys(aliases)
    if (names.length === 0) return
    const validNames = new Set(names)
    const toRemove = favorites.filter(n => !validNames.has(n))
    if (toRemove.length > 0) {
      favorites = favorites.filter(n => validNames.has(n))
    }
  })
</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">wol</span>
  </a>
  <div class="spacer"></div>
  <div class="actions">
    <button class="btn xs icon-btn" class:key-on={token} onclick={openTokenModal} aria-label="API token settings">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m15.5 7.5 2.3 2.3a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L19 4"/><path d="m21 2-9.6 9.6"/><circle cx="7.5" cy="15.5" r="5.5"/></svg>
    </button>
  </div>
</header>

<div class="app">
  <div class="page-head">
    <h1>Wake-on-LAN</h1>
    <p>Devices, magic packets, boot history</p>
  </div>

  {#if error}
    <div class="msg error block">{error}</div>
  {/if}
  {#if success}
    <div class="msg success block">{success}</div>
  {/if}

  <section class="card form-section">
    <h2>{editingName ? 'Edit alias' : 'Add alias'}</h2>
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit() }}>
      <div class="field-row">
        <div class="field">
          <label for="wol-name">Hostname</label>
          <input id="wol-name" type="text" placeholder="nas" bind:value={formName} disabled={editingName !== null} />
        </div>
        <div class="field grow">
          <label for="wol-mac">MAC address</label>
          <input id="wol-mac" type="text" placeholder="aa:bb:cc:dd:ee:ff" bind:value={formMac} />
        </div>
      </div>
      <div class="form-actions">
        <button type="submit" class="btn primary">{editingName ? 'Update' : 'Add'}</button>
        {#if editingName}
          <button type="button" class="btn" onclick={cancelEdit}>Cancel</button>
        {/if}
      </div>
    </form>
  </section>

  <section class="card list-section">
    <div class="list-head">
      <h2>Aliases</h2>
      <span class="count">{Object.keys(aliases).length}</span>
    </div>
    {#if Object.keys(aliases).length === 0}
      <p class="empty">No aliases yet. Add one above.</p>
    {:else}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th></th>
              <th>Hostname</th>
              <th>MAC address</th>
              <th>
                <span class="th-row">Boot status
                  <button class="icon-btn refresh" onclick={refreshAllBootStatus} disabled={refreshing} aria-label="Refresh boot status">
                    <svg class:spinning={refreshing} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/></svg>
                  </button>
                </span>
              </th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each sortedEntries as [name, entry]}
              {@const st = bootStatus(name)}
              <tr class={favorites.includes(name) ? 'favorite-row' : ''}>
                <td class="fav-cell">
                  <button
                    class="fav"
                    class:on={favorites.includes(name)}
                    onclick={() => toggleFavorite(name)}
                    aria-label={favorites.includes(name) ? `Remove ${name} from favorites` : `Add ${name} to favorites`}
                    aria-pressed={favorites.includes(name)}>
                    <svg viewBox="0 0 24 24" fill={favorites.includes(name) ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" aria-hidden="true"><path d="M12 3.5 14.6 9l6 .9-4.3 4.2 1 6-5.3-2.8L6.7 20l1-6L3.4 9.9l6-.9z"/></svg>
                  </button>
                </td>
                <td class="host-cell">{name}</td>
                <td class="mono">{entry.Mac}</td>
                <td class="boot-cell"
                  onmouseenter={() => hoveredCell = name}
                  onmouseleave={() => hoveredCell = null}>
                  <span class="dot" class:up={st.state === 'up'} class:down={st.state === 'down'}></span>
                  <span class="boot-text">{st.ago || (st.state === 'unknown' ? 'No events' : '')}</span>
                  {#if hoveredCell === name && bootTimes[name]?.events?.length}
                    <div class="tooltip">
                      <div class="tooltip-header">Recent events</div>
                      <ul class="tooltip-list">
                        {#each bootTimes[name].events as event}
                          <li>
                            <span class="dot" class:up={event.type === 'boot'} class:down={event.type === 'shutdown'}></span>
                            <span class="event-type">{event.type}</span>
                            <span class="event-time">{timeAgo(event.timestamp)}</span>
                          </li>
                        {/each}
                      </ul>
                    </div>
                  {/if}
                </td>
                <td class="actions">
                  <button class="btn xs" onclick={() => handleWol(name)} disabled={wolCooldowns[name] > 0}>
                    {wolCooldowns[name] > 0 ? `${wolCooldowns[name]}s` : 'Wake'}
                  </button>
                  <button class="btn xs" onclick={() => editAlias(name)}>Edit</button>
                  <button class="btn xs danger" onclick={() => handleDelete(name)}>Delete</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</div>

{#if showTokenModal}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="modal-overlay" role="presentation" onclick={() => showTokenModal = false}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="modal" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()}>
      <h2>API token</h2>
      <div class="field">
        <label for="token-input">Pre-shared token</label>
        <input id="token-input" type="text" placeholder="Enter pre-shared token" bind:value={tokenInput} />
      </div>
      <div class="modal-actions">
        <button class="btn primary" onclick={saveToken}>Save</button>
        <button class="btn danger" onclick={clearToken}>Clear</button>
        <button class="btn" onclick={() => showTokenModal = false}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

<style>
  :global(body) { --page-max: 960px; }
  .block { margin-bottom: 16px; }

  .form-section { margin-bottom: 16px; }
  .form-section h2 { margin-bottom: 16px; }
  form { display: flex; flex-direction: column; gap: 16px; }
  .form-row, .form-actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .grow { flex: 2; }

  .list-head { display: flex; align-items: baseline; gap: 8px; margin-bottom: 12px; }
  .count { color: var(--text3); font-family: var(--mono); font-size: 12px; }

  .th-row { display: inline-flex; align-items: center; gap: 6px; }
  .refresh { display: inline-grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 0; border-radius: var(--r-sm); background: transparent; color: var(--text3); cursor: pointer; }
  .refresh:hover:not(:disabled) { background: var(--surface2); color: var(--text); }
  .refresh:disabled { opacity: 0.6; cursor: default; }
  .refresh svg { width: 14px; height: 14px; }
  .spinning { animation: spin 0.9s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .fav-cell { width: 32px; }
  .fav { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; background: none; color: var(--text3); cursor: pointer; }
  .fav svg { width: 15px; height: 15px; }
  .fav:hover { color: var(--text2); }
  .fav.on { color: var(--accent); }
  .favorite-row td { background: var(--highlight); }

  .host-cell { color: var(--text); font-weight: 500; }
  .mono { color: var(--text2); font-family: var(--mono); font-size: 12px; }

  .dot { display: inline-block; width: 7px; height: 7px; margin-right: 6px; border-radius: 50%; background: var(--text3); flex: none; }
  .dot.up { background: var(--success); }
  .dot.down { background: var(--danger); }
  .boot-cell { position: relative; color: var(--text2); font-size: 12px; cursor: default; white-space: nowrap; }
  .boot-text { vertical-align: 1px; }

  .tooltip { position: absolute; bottom: 100%; left: 50%; z-index: 50; transform: translateX(-50%); margin-bottom: 6px; padding: 10px 12px; border: 1px solid var(--border2); border-radius: var(--r-md); background: var(--surface); white-space: nowrap; }
  .tooltip-header { margin-bottom: 6px; color: var(--text3); font-size: 11px; font-weight: 500; }
  .tooltip-list { margin: 0; padding: 0; list-style: none; }
  .tooltip-list li { display: flex; align-items: center; gap: 6px; padding: 2px 0; font-size: 12px; color: var(--text2); }
  .event-type { min-width: 5em; text-transform: capitalize; }
  .event-time { color: var(--text3); }

  .actions { display: flex; gap: 6px; white-space: nowrap; text-align: right; }

  .icon-btn { width: 32px; height: 32px; padding: 0; }
  .icon-btn svg { width: 16px; height: 16px; }
  .key-on { color: var(--accent); border-color: rgba(233, 69, 96, 0.35); }

  .modal-overlay { position: fixed; inset: 0; z-index: 200; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(0, 0, 0, 0.55); }
  .modal { width: 380px; max-width: 100%; padding: 24px; border: 1px solid var(--border2); border-radius: var(--r-lg); background: var(--surface); }
  .modal h2 { margin-bottom: 16px; }
  .modal-actions { display: flex; gap: 8px; margin-top: 20px; }

  @media (max-width: 700px) {
    .table-wrap table, .table-wrap thead, .table-wrap tbody { display: block; }
    .table-wrap thead { display: none; }
    .table-wrap tbody tr {
      display: grid;
      grid-template-columns: 24px minmax(0, 1fr) auto;
      grid-template-areas:
        'fav host boot'
        '.   mac  mac'
        'act act  act';
      align-items: center;
      column-gap: 8px;
      row-gap: 4px;
      margin-bottom: 12px;
      padding: 12px 14px;
      border: 1px solid var(--border);
      border-radius: 12px;
      background: var(--surface);
    }
    .table-wrap td { padding: 0; border: 0; background: transparent; }
    .fav-cell { grid-area: fav; width: auto; align-self: start; }
    .fav { width: 32px; height: 32px; margin: -4px; }
    .host-cell { grid-area: host; min-width: 0; overflow-wrap: anywhere; font-size: 15px; line-height: 1.35; }
    .table-wrap td.mono { grid-area: mac; }
    .boot-cell { grid-area: boot; align-self: start; }
    .boot-cell .tooltip { display: none; }
    .actions { grid-area: act; display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
    .favorite-row td { background: transparent; }
  }

  @media (max-width: 480px) {
    .actions .btn { flex: 1; }
  }
</style>
