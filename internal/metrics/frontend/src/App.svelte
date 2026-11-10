<script>
  import { onMount } from 'svelte'
  import Chart from './Chart.svelte'
  import Stats from './Stats.svelte'

  let series = $state([])
  let hosts = $state([])
  let selectedHost = $state('all')
  let loading = $state(true)
  let error = $state('')
  let range = $state('15m')
  let autoRefresh = $state(true)
  let lastUpdated = $state('')
  let rangeFrom = $state(0)
  let rangeTo = $state(Date.now())
  let admin = $state(null)
  let statusText = $state('')
  let statusLoading = $state(false)
  let statusErr = $state('')
  let statusDialog = $state(null)
  let logDialog = $state(null)
  let logText = $state('')

  const inGateway = typeof window !== 'undefined' && window.__MU_GATEWAY__

  const ranges = [
    { value: '15m', label: '15m' },
    { value: '1h', label: '1h' },
    { value: '6h', label: '6h' },
    { value: '24h', label: '24h' }
  ]

  const palette = ['#e94560', '#6aa9ff', '#4cc38a', '#d9a441', '#a4aab8', '#8b91a1']
  const hostColor = new Map()
  let colorIdx = 0

  function colorFor(host) {
    if (!hostColor.has(host)) {
      hostColor.set(host, palette[colorIdx % palette.length])
      colorIdx++
    }
    return hostColor.get(host)
  }

  function rangeMs(v) {
    const n = parseFloat(v)
    if (v.endsWith('h')) return n * 3600e3
    if (v.endsWith('d')) return n * 86400e3
    return n * 60e3
  }

  async function load() {
    loading = true
    error = ''
    try {
      const [hostsRes, nameRes] = await Promise.all([fetch('/api/metrics/hosts'), fetch('/api/metrics')])
      if (!hostsRes.ok || !nameRes.ok) {
        throw new Error(`HTTP ${hostsRes.status || nameRes.status}`)
      }
      const hostList = await hostsRes.json()
      const nameList = await nameRes.json()
      hosts = hostList
      if (selectedHost !== 'all' && !hostList.includes(selectedHost)) {
        selectedHost = 'all'
      }

      const toMs = Date.now()
      const fromMs = toMs - rangeMs(range)
      const from = new Date(fromMs).toISOString()
      rangeFrom = fromMs
      rangeTo = toMs

      const results = await Promise.allSettled(
        nameList.map(async (name) => {
          let url = `/api/metrics/${encodeURIComponent(name)}?from=${encodeURIComponent(from)}&to=now&limit=2000`
          if (selectedHost !== 'all') {
            url += `&tags=host=${encodeURIComponent(selectedHost)}`
          }
          const r = await fetch(url)
          if (!r.ok) {
            const text = await r.text()
            throw new Error(text || `HTTP ${r.status}`)
          }
          return r.json()
        })
      )

      const byName = new Map()
      results.forEach((res2, i) => {
        const name = nameList[i]
        if (res2.status !== 'fulfilled') {
          byName.set(name, { name, error: res2.reason?.message || 'failed', lines: [] })
          return
        }
        const metrics = res2.value
        if (!Array.isArray(metrics) || metrics.length === 0) return
        const lines = metrics.map((m) => {
          const host = (m.tags && m.tags.host) || ''
          return {
            host,
            label: host || m.metric,
            points: (m.points || []).map((p) => ({ x: p.t / 1e6, y: p.v })),
            color: colorFor(host)
          }
        })
        byName.set(name, { name, error: '', lines })
      })

      series = [...byName.values()]
      lastUpdated = new Date().toLocaleTimeString()
    } catch (e) {
      error = e.message
      series = []
    } finally {
      loading = false
    }
  }

  function primaryLine(card) {
    if (!card) return null
    if (selectedHost !== 'all') {
      const l = card.lines.find((x) => x.host === selectedHost && x.points.length > 0)
      if (l) return l
    }
    return card.lines.find((x) => x.points.length > 0) || null
  }

  const summary = $derived.by(() => {
    const pick = (key) => {
      const card = series.find((x) => x.name === key)
      const line = primaryLine(card)
      return line ? line.points[line.points.length - 1].y : null
    }
    return [
      { label: 'CPU', value: pick('cpu.used.percent'), unit: '%' },
      { label: 'Memory', value: pick('memory.used.percent'), unit: '%' },
      { label: 'Load 1m', value: pick('load.1m'), unit: '' }
    ]
  })

  $effect(() => {
    if (!autoRefresh) return
    const id = setInterval(() => load(), 30000)
    return () => clearInterval(id)
  })

  onMount(() => {
    load()
    if (inGateway) {
      refreshAdmin()
      adminTimer = setInterval(refreshAdmin, 5000)
    }
  })

  let adminTimer

  async function refreshAdmin() {
    try {
      const r = await fetch('/api/metrics/admin/status')
      if (!r.ok) return
      admin = await r.json()
    } catch (e) {
      /* admin API unavailable */
    }
  }

  async function adminAction(action) {
    try {
      const r = await fetch(`/api/metrics/admin/${action}`, { method: 'POST' })
      if (!r.ok) {
        const text = await r.text()
        admin = { state: 'error', error: text || `HTTP ${r.status}` }
        return
      }
      admin = await r.json()
    } catch (e) {
      admin = { state: 'error', error: e.message }
    }
    load()
  }

  function adminDuration() {
    if (!admin || !admin.since) return ''
    const since = new Date(admin.since).getTime()
    if (!since) return ''
    const s = Math.max(0, Math.floor((Date.now() - since) / 1000))
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return h > 0 ? `${h}h ${m}m` : `${m}m ${s % 60}s`
  }

  async function showStatus() {
    statusLoading = true
    statusErr = ''
    statusText = ''
    try {
      const [infoRes, namesRes, hostsRes] = await Promise.all([
        fetch('/api/metrics/info'),
        fetch('/api/metrics'),
        fetch('/api/metrics/hosts')
      ])
      if (!infoRes.ok || !namesRes.ok || !hostsRes.ok) {
        throw new Error(`HTTP ${infoRes.status || namesRes.status || hostsRes.status}`)
      }
      const info = await infoRes.json()
      const names = await namesRes.json()
      const hosts = await hostsRes.json()
      statusText = buildStatusText(info, names, hosts)
      statusDialog?.showModal()
    } catch (e) {
      statusErr = e.message
      statusDialog?.showModal()
    } finally {
      statusLoading = false
    }
  }

  function closeStatus() {
    statusDialog?.close()
  }

  function showLog() {
    logText = admin.log ? admin.log.join('\n') : ''
    logDialog?.showModal()
  }

  function closeLog() {
    logDialog?.close()
  }

  function buildStatusText(info, names, hosts) {
    const lines = []
    const dash = (v, d = '-') => (v === undefined || v === null || v === '' ? d : String(v))
    const retention = info.retention === '' || info.retention === '0' ? '0 (forever)' : dash(info.retention)

    lines.push('Config:')
    lines.push(`  mode             ${dash(info.mode)}`)
    if (info.pid) lines.push(`  pid              ${info.pid}`)
    if (info.started_at) lines.push(`  started_at       ${info.started_at}`)
    if (info.version) lines.push(`  version          ${info.version}`)
    lines.push(`  config-dir       ${dash(info.config_dir, '(default ~/.config/mu)')}`)
    lines.push(`  config file      ${dash(info.config_file)}`)
    lines.push(`  retention        ${retention}`)
    lines.push(`  compact_interval ${dash(info.compact_interval, '1d')}`)
    lines.push(`  collect_interval ${dash(info.collect_interval, '30s')}`)
    lines.push(`  hostname         ${dash(info.hostname)}`)
    lines.push(`  db-path          ${dash(info.db_path)}`)
    lines.push(`  debug            ${info.debug}`)
    if (info.agent) lines.push(`  agent            running (pid ${dash(info.pid)}, ${dash(info.mode)})`)
    lines.push('')

    lines.push('Running:')
    lines.push(`  server           ${info.port ? `http://localhost:${info.port}` : '-'}`)
    lines.push(`  state            running (${names.length} metrics)`)
    lines.push('')

    lines.push('DB:')
    if (info.db_path) {
      lines.push(`  file             ${info.db_path}`)
      if (info.db_size) lines.push(`  size             ${fmtSize(info.db_size)}`)
      if (info.db_modified) lines.push(`  modified         ${new Date(info.db_modified).toLocaleString()}`)
    }
    if (info.series > 0 || info.points > 0) {
      lines.push(`  series           ${info.series || 0}`)
      lines.push(`  points           ${info.points || 0}`)
    }
    if (Array.isArray(hosts)) lines.push(`  hosts            ${listPreview(hosts)}`)
    if (Array.isArray(names)) lines.push(`  metrics          ${listPreview(names)} (${names.length} total)`)
    return lines.join('\n')
  }

  function listPreview(items) {
    if (!items || items.length === 0) return '-'
    const max = 8
    const parts = items.slice(0, max)
    let s = parts.join(', ')
    if (items.length > max) s += ', ...'
    return s
  }

  function fmtSize(b) {
    if (b === null || b === undefined) return '-'
    const n = Number(b)
    if (n < 1024) return `${n} B`
    const units = ['K', 'M', 'G', 'T', 'P', 'E']
    let div = 1024
    let exp = 0
    for (let x = n / 1024; x >= 1024 && exp < units.length - 1; x /= 1024) {
      div *= 1024
      exp++
    }
    return `${(n / div).toFixed(1)} ${units[exp]}B`
  }

  function fmt(v) {
    if (v === null || v === undefined) return '-'
    return Number(v).toFixed(2)
  }
</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">metrics</span>
  </a>
</header>

<div class="app">
  <div class="page-head row">
    <div>
      <h1>Metrics</h1>
      <p>System time-series charts</p>
    </div>
    <div class="controls">
      <select class="host" bind:value={selectedHost} onchange={load} aria-label="Host">
        <option value="all">All hosts</option>
        {#each hosts as h}
          <option value={h}>{h}</option>
        {/each}
      </select>
      <select bind:value={range} onchange={load} aria-label="Range">
        {#each ranges as r}
          <option value={r.value}>{r.label}</option>
        {/each}
      </select>
      <label class="check"><input type="checkbox" bind:checked={autoRefresh} /><span>Auto</span></label>
      <button class="btn" onclick={load} disabled={loading}>{loading ? 'Refreshing...' : 'Refresh'}</button>
    </div>
  </div>

  {#if admin && admin.state}
    <div class="admin-bar">
      <span class="admin-state admin-{admin.state}"><span class="dot"></span>{admin.state}</span>
      {#if admin.state === 'running' || admin.state === 'starting'}
        <span class="admin-meta">PID {admin.pid}</span>
        {#if adminDuration()}<span class="admin-meta">{adminDuration()}</span>{/if}
      {/if}
      {#if admin.error}
        <span class="admin-error">{admin.error}</span>
      {/if}
      {#if admin.state === 'running' || admin.state === 'starting'}
        <button class="btn xs" onclick={() => adminAction('stop')}>Stop</button>
        <button class="btn xs" onclick={() => adminAction('restart')}>Restart</button>
      {:else}
        <button class="btn xs" onclick={() => adminAction('start')} disabled={admin.state === 'external'}>
          {admin.state === 'external' ? 'External' : 'Start'}
        </button>
      {/if}
      <button class="btn xs" onclick={showStatus} disabled={statusLoading}>{statusLoading ? 'Loading...' : 'Status'}</button>
      {#if admin.log && admin.log.length}
        <button class="btn xs" onclick={showLog}>Log</button>
      {/if}
    </div>
  {/if}

  <dialog bind:this={statusDialog} class="dialog" onclose={() => { statusErr = ''; statusText = '' }}>
    <div class="dialog-head">
      <h3>Server status</h3>
      <button class="btn xs" onclick={closeStatus}>Close</button>
    </div>
    {#if statusErr}
      <div class="msg error">{statusErr}</div>
    {/if}
    <pre>{statusText}</pre>
  </dialog>

  <dialog bind:this={logDialog} class="dialog" onclose={() => { logText = '' }}>
    <div class="dialog-head">
      <h3>Server log</h3>
      <button class="btn xs" onclick={closeLog}>Close</button>
    </div>
    <pre>{logText}</pre>
  </dialog>

  {#if error}
    <div class="msg error block">{error}</div>
  {/if}

  {#if loading && series.length === 0 && !error}
    <p class="muted">Loading metrics...</p>
  {:else if series.length === 0 && !error}
    <div class="card empty-card">
      <p>No metrics collected</p>
      <p class="hint">Start the collector with <code>mu metrics serve --agent</code></p>
    </div>
  {:else}
    <div class="summary">
      {#each summary as s}
        <div class="card stat">
          <div class="stat-label">{s.label}</div>
          <div class="stat-value">{fmt(s.value)}{s.unit}</div>
        </div>
      {/each}
      {#if lastUpdated}
        <div class="updated">updated {lastUpdated}</div>
      {/if}
    </div>

    <div class="charts">
      {#each series as s}
        <div class="card chart-card">
          <div class="chart-head">
            <span class="chart-name">{s.name}</span>
            {#if s.lines.length > 1}
              <span class="badge">{s.lines.length} hosts</span>
            {:else if s.lines[0]?.host}
              <span class="badge">{s.lines[0].host}</span>
            {/if}
          </div>
          {#if s.error}
            <div class="chart-error">{s.error}</div>
          {:else}
            <Chart lines={s.lines} height={150} from={rangeFrom} to={rangeTo} />
            <Stats card={s} host={selectedHost} />
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  :global(body) { --page-max: 1080px; }
  .block { margin-bottom: 16px; }
  .hint { margin-top: 8px; color: var(--text3); font-size: 13px; }
  .hint code { padding: 2px 6px; border-radius: 4px; background: var(--surface2); }

  .controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .controls select { width: auto; min-width: 96px; }
  .controls .host { max-width: 180px; }
  .check { display: inline-flex; align-items: center; gap: 6px; color: var(--text2); font-size: 13px; cursor: pointer; }
  .check input { width: 14px; height: 14px; margin: 0; accent-color: var(--text); }

  .admin-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 16px; padding: 8px 12px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--surface); }
  .admin-state { display: inline-flex; align-items: center; gap: 6px; color: var(--text2); font-family: var(--mono); font-size: 12px; }
  .admin-state .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--text3); }
  .admin-running .dot { background: var(--success); }
  .admin-running { color: var(--success); }
  .admin-starting { color: var(--warning); }
  .admin-starting .dot { background: var(--warning); }
  .admin-external { color: var(--info); }
  .admin-external .dot { background: var(--info); }
  .admin-stopped { color: var(--text3); }
  .admin-error { color: var(--danger); font-size: 12px; }
  .admin-meta { color: var(--text3); font-family: var(--mono); font-size: 12px; }

  .dialog { width: 100%; max-width: min(720px, 92vw); margin: auto; padding: 16px 20px; border: 1px solid var(--border2); border-radius: var(--r-lg); background: var(--surface); color: var(--text); }
  .dialog[open] { display: flex; flex-direction: column; }
  .dialog::backdrop { background: rgba(0, 0, 0, 0.55); }
  .dialog-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
  .dialog pre { max-height: 60vh; margin: 0; padding: 12px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); color: var(--text2); font-size: 12px; line-height: 1.55; white-space: pre-wrap; word-break: break-word; overflow: auto; }

  .summary { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 12px; margin-bottom: 16px; }
  .stat { min-width: 132px; padding: 12px 16px; }
  .stat-label { margin-bottom: 4px; color: var(--text2); font-size: 12px; }
  .stat-value { color: var(--text); font-family: var(--mono); font-size: 18px; font-variant-numeric: tabular-nums; }
  .updated { margin-left: auto; color: var(--text3); font-family: var(--mono); font-size: 12px; }

  .charts { display: grid; grid-template-columns: repeat(auto-fill, minmax(420px, 1fr)); gap: 16px; }
  .chart-card { padding: 16px 18px; }
  .chart-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 12px; }
  .chart-name { overflow: hidden; color: var(--text); font-family: var(--mono); font-size: 12.5px; text-overflow: ellipsis; white-space: nowrap; }
  .chart-error { padding: 8px 0; color: var(--danger); font-size: 13px; }

  @media (max-width: 720px) {
    .charts { grid-template-columns: 1fr; }
    .controls { width: 100%; }
    .controls .btn { flex: 1; }
  }
</style>
