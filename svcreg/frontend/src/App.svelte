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

<div class="app">
  <div class="top-bar">
    <div class="top-left">
      {#if inGateway}
        <a href="/" class="home-link">&larr; Home</a>
      {/if}
      <h1>Service Registry</h1>
    </div>
  </div>

  {#if error}
    <button class="msg error" onclick={() => error = ''}>{error}</button>
  {/if}

  <div class="tabs">
    <button class="tab" class:active={tab === 'dashboard'} onclick={() => tab = 'dashboard'}>Dashboard</button>
    <button class="tab" class:active={tab === 'services'} onclick={() => tab = 'services'}>Services</button>
    <button class="tab" class:active={tab === 'instances'} onclick={() => tab = 'instances'}>Instances</button>
    <button class="tab" class:active={tab === 'admin'} onclick={() => tab = 'admin'}>Admin</button>
  </div>

  {#if loading}
    <div class="loading">Loading...</div>
  {:else}

    {#if tab === 'dashboard'}
      <div class="refresh-bar"><button class="btn xs" onclick={loadData}>↻</button></div>
      <div class="stats">
        {#each statCards as stat}
          <div class="card stat-card">
            <div class="stat-label">{stat.label}</div>
            <div class="stat-value">{stat.value}</div>
          </div>
        {/each}
      </div>
    {/if}

    {#if tab === 'services'}
      <div class="toolbar">
        <div class="field">
          <select bind:value={envFilter}>
            <option value="">All Environments</option>
            {#each envs as env}
              <option value={env}>{env}</option>
            {/each}
          </select>
        </div>
        <span class="count">{filteredServices.length} service(s)</span>
        <button class="btn xs" onclick={loadData}>↻</button>
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
                <td><button class="expand-btn" onclick={() => toggleExpand(svc.serviceId)}>{expandedId === svc.serviceId ? '▾' : '▸'}</button></td>
                <td class="mono">{svc.serviceId ? svc.serviceId.slice(0, 12) + '...' : '-'}</td>
                <td>{svc.appId || '-'}</td>
                <td><strong>{svc.serviceName}</strong></td>
                <td>{svc.version}</td>
                <td><span class="env-tag">{svc.environment || '-'}</span></td>
                <td><span class="status-dot" class:up={svc.status === 'UP'} class:down={svc.status !== 'UP'}></span>{svc.status || '-'}</td>
                <td><button class="btn xs" onclick={() => deleteService(svc.serviceId)}>del</button></td>
              </tr>
              {#if expandedId === svc.serviceId}
                <tr class="detail-row">
                  <td colspan="8">
                    <div class="detail">
                      <div class="detail-section">
                        <div class="section-title">Instances</div>
                        {#if expandedLoading}
                          <div class="loading-mini">loading...</div>
                        {:else if expandedData.instances.length === 0}
                          <div class="text2">No instances</div>
                        {:else}
                          <table class="table mini">
                            <thead><tr><th>Instance ID</th><th>Host</th><th>Status</th><th>Endpoints</th></tr></thead>
                            <tbody>
                              {#each expandedData.instances as inst}
                                <tr>
                                  <td class="mono">{inst.instanceId ? inst.instanceId.slice(0, 12) + '...' : '-'}</td>
                                  <td>{inst.hostName || '-'}</td>
                                  <td><span class="status-dot" class:up={inst.status === 'UP'} class:down={inst.status !== 'UP'}></span>{inst.status || '-'}</td>
                                  <td class="mono">{(inst.endpoints || []).join(', ') || '-'}</td>
                                </tr>
                              {/each}
                            </tbody>
                          </table>
                        {/if}
                      </div>
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
        <span class="count">{instances.length} instance(s)</span>
        <button class="btn xs" onclick={loadData}>↻</button>
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
                <td><span class="env-tag">{inst.environment || '-'}</span></td>
                <td class="mono">{inst.instanceId ? inst.instanceId.slice(0, 12) + '...' : '-'}</td>
                <td>{inst.hostName || '-'}</td>
                <td><span class="status-dot" class:up={inst.status === 'UP'} class:down={inst.status !== 'UP'}></span>{inst.status || '-'}</td>
                <td class="mono">{(inst.endpoints || []).join(', ') || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    {#if tab === 'admin'}
      <div class="card admin-card">
        <div class="section-title">Server Configuration</div>

        <div class="field">
          <label for="admin-port">Port</label>
          <input id="admin-port" type="number" bind:value={adminPort} disabled={adminStatus.running} />
        </div>
        <div class="field">
          <label for="admin-host">Host</label>
          <input id="admin-host" type="text" bind:value={adminHost} disabled={adminStatus.running} />
        </div>
        <div class="field">
          <label for="admin-db">Database Path</label>
          <input id="admin-db" type="text" bind:value={adminDB} disabled={adminStatus.running} />
        </div>

        <div class="field checkbox-field">
          <label>
            <input type="checkbox" bind:checked={adminIndependent} disabled={adminStatus.running} />
            Independent process group
          </label>
        </div>

        <div class="admin-actions">
          {#if adminStatus.running}
            <button class="btn" onclick={stopServer} disabled={adminBusy}>⏹ Stop</button>
            <span class="admin-status running">● Running (PID: {adminStatus.pid})</span>
          {:else}
            <button class="btn primary" onclick={startServer} disabled={adminBusy}>▶ Start</button>
            <span class="admin-status stopped">○ Stopped</span>
          {/if}
        </div>

        <div class="section-title" style="margin-top:20px">Logs</div>
        <div class="log-box">
          {#each adminStatus.logs as line}
            <div class="log-line">{line}</div>
          {:else}
            <div class="log-line muted">No logs yet</div>
          {/each}
        </div>
        <button class="btn xs" onclick={() => { adminStatus.logs = [] }}>Clear Logs</button>
      </div>
    {/if}

  {/if}
</div>

<style>
  .app { max-width: 960px; margin: 0 auto; padding: 40px 16px; }
  h1 { font-size: 24px; display: inline; }
  .top-bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
  .top-left { display: flex; align-items: center; gap: 10px; }
  .home-link { float: none; }

  .tabs { display: flex; gap: 0; margin-bottom: 16px; border-bottom: 1px solid var(--border); }
  .tab { padding: 10px 20px; border: none; background: none; color: var(--text2); cursor: pointer; font-size: 14px; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .tab.active { color: var(--text); border-bottom-color: var(--primary); }
  .tab:hover { color: var(--text); }

  .loading { text-align: center; padding: 60px 0; color: var(--text2); font-size: 16px; }
  .loading-mini { color: var(--text3); font-size: 13px; padding: 4px 0; }

  .refresh-bar { text-align: right; margin-bottom: 8px; }
  .stats { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 16px; }
  .stat-card { padding: 24px; }
  .stat-label { font-size: 12px; color: var(--text2); text-transform: uppercase; letter-spacing: .5px; margin-bottom: 8px; }
  .stat-value { font-size: 20px; font-weight: 600; color: var(--text); }

  .toolbar { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
  .toolbar .field { margin: 0; width: auto; }
  .toolbar .field select { width: auto; min-width: 180px; }
  .count { font-size: 13px; color: var(--text3); }

  .table-wrap { overflow-x: auto; border: 1px solid var(--border); border-radius: 8px; }
  .table { width: 100%; border-collapse: collapse; font-size: 13px; }
  .table th { text-align: left; padding: 10px 12px; background: var(--surface); color: var(--text2); font-size: 11px; text-transform: uppercase; letter-spacing: .5px; border-bottom: 1px solid var(--border); white-space: nowrap; }
  .table td { padding: 8px 12px; border-bottom: 1px solid var(--border); color: var(--text); }
  .table tr:last-child td { border-bottom: none; }
  .table.mini { font-size: 12px; }
  .table.mini td { padding: 4px 8px; }
  .mono { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 12px; }
  .text2 { color: var(--text2); font-size: 13px; }

  .expand-btn { background: none; border: none; color: var(--text2); cursor: pointer; padding: 0 4px; font-size: 14px; }
  .expand-btn:hover { color: var(--text); }

  .detail-row td { padding: 0 !important; background: var(--highlight); }
  .detail { padding: 16px 24px 16px 40px; }
  .detail-section { margin-bottom: 16px; }
  .detail-section:last-child { margin-bottom: 0; }
  .section-title { font-size: 13px; font-weight: 600; margin-bottom: 6px; color: var(--text); }

  .env-tag { font-size: 11px; color: var(--text2); }

  .status-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 6px; background: var(--text3); }
  .status-dot.up { background: #4caf50; }
  .status-dot.down { background: #e94560; }

  .admin-card .field { margin-bottom: 12px; }
  .admin-card .field label { display: block; font-size: 12px; color: var(--text2); margin-bottom: 4px; }
  .admin-card .field input { width: 100%; padding: 8px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; font-family: inherit; outline: none; }
  .admin-card .field input:disabled { opacity: .5; }
  .admin-card .field input:focus { border-color: var(--primary); }
  .admin-card .checkbox-field label { display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px; color: var(--text); }
  .admin-card .checkbox-field input { width: auto; }

  .admin-actions { display: flex; align-items: center; gap: 12px; margin-top: 16px; }
  .admin-status { font-size: 13px; }
  .admin-status.running { color: #4caf50; }
  .admin-status.stopped { color: var(--text3); }

  .log-box { background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: 10px; max-height: 240px; overflow-y: auto; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 11px; margin-bottom: 8px; }
  .log-line { color: var(--text2); padding: 1px 0; line-height: 1.5; white-space: pre-wrap; word-break: break-all; }
  .log-line.muted { color: var(--text3); font-style: italic; }
</style>
