<script>
  import { onMount } from 'svelte'
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let tab = $state('dashboard')
  let error = $state('')
  let loading = $state(true)

  let services = $state([])
  let instances = $state([])
  let svcList = $state([])
  let envFilter = $state('')
  let expandedId = $state('')
  let expandedData = $state({instances:[],schemas:[],tags:{}})
  let expandedLoading = $state(false)
  let stats = $state({serviceCount:0,onlineCount:0,instanceCount:0})

  // admin state
  let adminStatus = $state({running:false,pid:0,config:{port:30100,host:'0.0.0.0',dbPath:'~/.config/mu/svcreg.db'},logs:[]})
  let adminPort = $state(30100)
  let adminHost = $state('0.0.0.0')
  let adminDB = $state('~/.config/mu/svcreg.db')
  let adminIndependent = $state(true)
  let adminBusy = $state(false)

  function api(path, opts) {
    const ctrl = new AbortController()
    const timer = setTimeout(() => ctrl.abort(), 3000)
    const merged = { signal: ctrl.signal, ...opts }
    return fetch(path, merged).then(r => {
      clearTimeout(timer)
      if (!r.ok) return r.json().then(e => { throw new Error(e.error || e.message || r.statusText) })
      return r.json()
    })
  }

  async function loadData() {
    loading = true
    error = ''
    try {
      const s = await api('/api/svcreg/status')
      stats = s
      svcList = await api('/api/svcreg/services')
      instances = await api('/api/svcreg/instances')
    } catch (e) {
      error = e.message || 'Failed to load data'
    } finally {
      loading = false
    }
  }

  async function loadAdmin() {
    try {
      adminStatus = await api('/api/svcreg/admin/status')
      adminPort = adminStatus.config.port || 30100
      adminHost = adminStatus.config.host || '0.0.0.0'
      adminDB = adminStatus.config.dbPath || '~/.config/mu/svcreg.db'
      adminIndependent = adminStatus.config.independent !== false
    } catch (e) {
      error = 'Admin API not available'
    }
  }

  async function startServer() {
    adminBusy = true
    error = ''
    try {
      await api('/api/svcreg/admin/start', {
        method: 'POST',
        headers: {'Content-Type':'application/json'},
        body: JSON.stringify({port:adminPort,host:adminHost,dbPath:adminDB,independent:adminIndependent})
      })
      await loadAdmin()
      await loadData()
    } catch (e) {
      error = e.message
    } finally {
      adminBusy = false
    }
  }

  async function stopServer() {
    if (!confirm('Stop the service registry server?')) return
    adminBusy = true
    error = ''
    try {
      await api('/api/svcreg/admin/stop', {method:'POST'})
      await loadAdmin()
      await loadData()
    } catch (e) {
      error = e.message
    } finally {
      adminBusy = false
    }
  }

  onMount(() => { loadData().finally(() => loadAdmin()) })

  let filteredServices = $derived.by(() => {
    if (!envFilter) return svcList
    return svcList.filter(s => (s.environment || '') === envFilter)
  })
  let envs = $derived([...new Set(svcList.map(s => s.environment || '').filter(Boolean))])

  let statCards = $derived([
    { label: 'Status', value: 'UP' },
    { label: 'Listening', value: stats.version?.listen || '-' },
    { label: 'Services', value: `${stats.serviceCount} (${stats.onlineCount} online)` },
    { label: 'Instances', value: `${stats.instanceCount}` },
  ])

  async function deleteService(id) {
    if (!confirm('Delete this service and all its instances?')) return
    try {
      // must use v4 API directly since proxy doesn't expose DELETE
      await fetch('/v4/default/registry/microservices/' + id, { method: 'DELETE' })
      await loadData()
    } catch (e) {
      error = e.message
    }
  }

  function toggleExpand(id) {
    if (expandedId === id) {
      expandedId = ''
      return
    }
    expandedId = id
    expandedLoading = true
    expandedData = {instances:[]}
    api('/api/svcreg/instances?serviceId=' + id)
      .then(d => { expandedData = {instances: d || []} })
      .catch(() => { expandedData = {instances: []} })
      .finally(() => { expandedLoading = false })
  }
</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">svcreg</span>
  </a>
</header>

<div class="app">
  <div class="page-head row">
    <div>
      <h1>Service Registry</h1>
      <p>Register and discover services</p>
    </div>
    <button class="btn" onclick={loadData} disabled={loading}>Refresh</button>
  </div>

  {#if error}
    <button class="msg error block" onclick={() => error = ''}>{error}</button>
  {/if}

  <div class="tabs">
    <button class="tab" class:active={tab === 'dashboard'} onclick={() => tab = 'dashboard'}>Dashboard</button>
    <button class="tab" class:active={tab === 'services'} onclick={() => tab = 'services'}>Services</button>
    <button class="tab" class:active={tab === 'instances'} onclick={() => tab = 'instances'}>Instances</button>
    <button class="tab" class:active={tab === 'admin'} onclick={() => tab = 'admin'}>Admin</button>
  </div>

  {#if loading}
    <p class="muted">Loading...</p>
  {:else}

    {#if tab === 'dashboard'}
      <div class="stats">
        {#each statCards as stat}
          <div class="card stat">
            <div class="stat-label">{stat.label}</div>
            <div class="stat-value">{stat.value}</div>
          </div>
        {/each}
      </div>
    {/if}

    {#if tab === 'services'}
      <div class="toolbar">
        <select bind:value={envFilter} aria-label="Environment filter">
          <option value="">All environments</option>
          {#each envs as env}
            <option value={env}>{env}</option>
          {/each}
        </select>
        <span class="count">{filteredServices.length} services</span>
      </div>
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th></th>
              <th>Service ID</th>
              <th>App ID</th>
              <th>Name</th>
              <th>Version</th>
              <th>Environment</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each filteredServices as svc}
              <tr>
                <td>
                  <button class="expand" class:open={expandedId === svc.serviceId} onclick={() => toggleExpand(svc.serviceId)} aria-label="Toggle instances">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg>
                  </button>
                </td>
                <td class="mono">{svc.serviceId ? svc.serviceId.slice(0, 12) + '...' : '-'}</td>
                <td>{svc.appId || '-'}</td>
                <td class="strong">{svc.serviceName}</td>
                <td>{svc.version}</td>
                <td>{svc.environment || '-'}</td>
                <td><span class="dot" class:up={svc.status === 'UP'}></span>{svc.status || '-'}</td>
                <td><button class="btn xs danger" onclick={() => deleteService(svc.serviceId)}>Delete</button></td>
              </tr>
              {#if expandedId === svc.serviceId}
                <tr class="detail-row">
                  <td colspan="8">
                    <div class="detail">
                      <div class="field-label">Instances</div>
                      {#if expandedLoading}
                        <p class="muted">Loading...</p>
                      {:else if expandedData.instances.length === 0}
                        <p class="muted">No instances</p>
                      {:else}
                        <table class="table mini">
                          <thead><tr><th>Instance ID</th><th>Host</th><th>Status</th><th>Endpoints</th></tr></thead>
                          <tbody>
                            {#each expandedData.instances as inst}
                              <tr>
                                <td class="mono">{inst.instanceId ? inst.instanceId.slice(0, 12) + '...' : '-'}</td>
                                <td>{inst.hostName || '-'}</td>
                                <td><span class="dot" class:up={inst.status === 'UP'}></span>{inst.status || '-'}</td>
                                <td class="mono">{(inst.endpoints || []).join(', ') || '-'}</td>
                              </tr>
                            {/each}
                          </tbody>
                        </table>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    {#if tab === 'instances'}
      <div class="toolbar">
        <span class="count">{instances.length} instances</span>
      </div>
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>Service</th>
              <th>Version</th>
              <th>Env</th>
              <th>Instance ID</th>
              <th>Host</th>
              <th>Status</th>
              <th>Endpoints</th>
            </tr>
          </thead>
          <tbody>
            {#each instances as inst}
              <tr>
                <td>{inst.serviceName || '-'}</td>
                <td>{inst.version || '-'}</td>
                <td>{inst.environment || '-'}</td>
                <td class="mono">{inst.instanceId ? inst.instanceId.slice(0, 12) + '...' : '-'}</td>
                <td>{inst.hostName || '-'}</td>
                <td><span class="dot" class:up={inst.status === 'UP'}></span>{inst.status || '-'}</td>
                <td class="mono">{(inst.endpoints || []).join(', ') || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    {#if tab === 'admin'}
      <div class="card">
        <h2>Server configuration</h2>

        <div class="field">
          <label for="admin-port">Port</label>
          <input id="admin-port" type="number" bind:value={adminPort} disabled={adminStatus.running} />
        </div>
        <div class="field">
          <label for="admin-host">Host</label>
          <input id="admin-host" type="text" bind:value={adminHost} disabled={adminStatus.running} />
        </div>
        <div class="field">
          <label for="admin-db">Database path</label>
          <input id="admin-db" type="text" bind:value={adminDB} disabled={adminStatus.running} />
        </div>

        <label class="check">
          <input type="checkbox" bind:checked={adminIndependent} disabled={adminStatus.running} />
          <span>Independent process group</span>
        </label>

        <div class="admin-actions">
          {#if adminStatus.running}
            <button class="btn" onclick={stopServer} disabled={adminBusy}>Stop</button>
            <span class="admin-state"><span class="dot up"></span>Running (PID {adminStatus.pid})</span>
          {:else}
            <button class="btn primary" onclick={startServer} disabled={adminBusy}>Start</button>
            <span class="admin-state"><span class="dot"></span>Stopped</span>
          {/if}
        </div>

        <h2 class="logs-title">Logs</h2>
        <div class="log-box">
          {#each adminStatus.logs as line}
            <div class="log-line">{line}</div>
          {:else}
            <div class="log-line muted">No logs yet</div>
          {/each}
        </div>
        <button class="btn xs" onclick={() => { adminStatus.logs = [] }}>Clear logs</button>
      </div>
    {/if}

  {/if}
</div>

<style>
  :global(body) { --page-max: 960px; }
  .block { margin-bottom: 16px; }
  .count { color: var(--text3); font-family: var(--mono); font-size: 12px; }

  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; }
  .stat { padding: 16px 20px; }
  .stat-label { margin-bottom: 6px; color: var(--text2); font-size: 12px; }
  .stat-value { color: var(--text); font-family: var(--mono); font-size: 20px; font-variant-numeric: tabular-nums; }

  .toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin-bottom: 12px; }
  .toolbar select { width: auto; min-width: 180px; }

  .table-wrap { border: 1px solid var(--border); border-radius: var(--r-md); overflow: auto; }
  .table td { white-space: nowrap; }
  .table .strong { color: var(--text); font-weight: 500; }
  .table .mono { color: var(--text2); font-size: 12px; }
  .table.mini { font-size: 12px; }
  .table.mini th, .table.mini td { padding: 6px 10px; }

  .expand { display: grid; place-items: center; width: 20px; height: 20px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--text3); cursor: pointer; transition: transform 140ms var(--ease), color 140ms var(--ease); }
  .expand svg { width: 14px; height: 14px; }
  .expand:hover { color: var(--text); }
  .expand.open { transform: rotate(90deg); }

  .detail-row td { padding: 0; background: var(--highlight); }
  .detail { padding: 12px 20px 12px 40px; }
  .field-label { margin-bottom: 8px; color: var(--text2); font-size: 12px; font-weight: 500; }

  .dot { display: inline-block; width: 7px; height: 7px; margin-right: 6px; border-radius: 50%; background: var(--text3); }
  .dot.up { background: var(--success); }

  .check { display: inline-flex; align-items: center; gap: 8px; margin-top: 4px; color: var(--text); font-size: 13px; cursor: pointer; }
  .check input { width: 14px; height: 14px; margin: 0; accent-color: var(--text); }

  .admin-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin-top: 20px; }
  .admin-state { display: inline-flex; align-items: center; color: var(--text2); font-size: 13px; }

  .logs-title { margin-top: 24px; }
  .log-box { max-height: 220px; margin: 12px 0; padding: 10px 12px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); overflow-y: auto; }
  .log-line { color: var(--text2); font-family: var(--mono); font-size: 11px; line-height: 1.6; white-space: pre-wrap; word-break: break-all; }
  .log-line.muted { color: var(--text3); }
</style>
