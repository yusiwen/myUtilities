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

  const palette = ['#4a9eff', '#e94560', '#4caf50', '#ff9800', '#ab47bc', '#00bcd4', '#fdd835', '#8d6e63']
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

<div class="container">
  <div class="header">
    <div class="header-left">
      {#if inGateway}
        <a href="/" class="home-link">← Home</a>
      {/if}
      <h1>Metrics</h1>
    </div>
    <div class="controls">
      <select class="host-select" bind:value={selectedHost} onchange={load}>
        <option value="all">All hosts</option>
        {#each hosts as h}
          <option value={h}>{h}</option>
        {/each}
      </select>
      <select class="range-select" bind:value={range} onchange={load}>
        {#each ranges as r}
          <option value={r.value}>{r.label}</option>
        {/each}
      </select>
      <label class="auto-refresh">
        <input type="checkbox" bind:checked={autoRefresh} />
        auto
      </label>
      <button class="btn xs" onclick={load} disabled={loading}>
        {loading ? 'Refreshing...' : 'Refresh'}
      </button>
    </div>
  </div>

  {#if admin && admin.state}
    <div class="admin-bar">
      <span class="admin-state admin-{admin.state}">{admin.state}</span>
      {#if admin.state === 'running' || admin.state === 'starting'}
        <span class="admin-meta">pid {admin.pid}</span>
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
      <button class="btn xs" onclick={showStatus} disabled={statusLoading}>
        {statusLoading ? 'Loading...' : 'Status'}
      </button>
      {#if admin.log && admin.log.length}
        <button class="btn xs" onclick={showLog}>Log</button>
      {/if}
    </div>
  {/if}

  <dialog bind:this={statusDialog} class="status-dialog" onclose={() => { statusErr = ''; statusText = '' }}>
    <div class="dialog-head">
      <h3>Server Status</h3>
      <button class="btn xs" onclick={closeStatus} title="Close">✕</button>
    </div>
    {#if statusErr}
      <div class="admin-error">{statusErr}</div>
    {/if}
    <pre>{statusText}</pre>
  </dialog>

  <dialog bind:this={logDialog} class="status-dialog" onclose={() => { logText = '' }}>
    <div class="dialog-head">
      <h3>Server Log</h3>
      <button class="btn xs" onclick={closeLog} title="Close">✕</button>
    </div>
    <pre>{logText}</pre>
  </dialog>

  {#if error}
    <div class="msg error">{error}</div>
  {/if}

  {#if loading && series.length === 0 && !error}
    <p class="loading">Loading metrics...</p>
  {:else if series.length === 0 && !error}
    <div class="empty">
      <p>No metrics collected.</p>
      <p class="hint">Start the collector with <code>mu metrics serve --agent</code></p>
    </div>
  {:else}
    <div class="summary">
      {#each summary as s}
        <div class="card summary-card">
          <div class="summary-label">{s.label}</div>
          <div class="summary-value">{fmt(s.value)}{s.unit}</div>
        </div>
      {/each}
      {#if lastUpdated}
        <div class="updated">updated {lastUpdated}</div>
      {/if}
    </div>

    <div class="cards">
      {#each series as s}
        <div class="card metric-card">
          <div class="metric-head">
            <span class="metric-name">{s.name}</span>
            {#if s.lines.length > 1}
              <span class="host-tag">{s.lines.length} hosts</span>
            {:else if s.lines[0]?.host}
              <span class="host-tag">{s.lines[0].host}</span>
            {/if}
          </div>
          {#if s.error}
            <div class="metric-error">{s.error}</div>
          {:else}
            <Chart lines={s.lines} height={160} from={rangeFrom} to={rangeTo} />
            <Stats card={s} host={selectedHost} />
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .container {
    max-width: 960px;
    margin: 0 auto;
    padding: 40px 20px;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 24px;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  h1 {
    font-size: 1.5rem;
    color: var(--text);
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .range-select {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
  }

  .host-select {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
    max-width: 180px;
  }

  .auto-refresh {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;
    color: var(--text2);
  }

  .admin-bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
    margin-bottom: 18px;
    padding: 10px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    font-size: 13px;
  }

  .admin-state {
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .admin-stopped {
    color: var(--text2);
  }

  .admin-starting {
    color: var(--primary);
  }

  .admin-running {
    color: var(--ok);
  }

  .admin-external {
    color: var(--primary);
  }

  .admin-error {
    color: var(--danger);
  }

  .admin-meta {
    color: var(--text2);
  }

  .status-dialog {
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--surface);
    color: var(--text);
    padding: 16px 20px;
    max-width: min(720px, 90vw);
    width: 100%;
    margin: auto;
  }

  .status-dialog[open] {
    display: flex;
    flex-direction: column;
  }

  .status-dialog::backdrop {
    background: rgba(0, 0, 0, 0.45);
  }

  .dialog-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 10px;
  }

  .dialog-head h3 {
    margin: 0;
    font-size: 1rem;
    color: var(--text);
  }

  .status-dialog pre {
    margin: 0;
    max-width: 100%;
    max-height: 60vh;
    overflow: auto;
    padding: 10px 12px;
    border-radius: 6px;
    background: var(--surface2);
    color: var(--text2);
    font-size: 12px;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .loading {
    color: var(--text2);
    font-size: 14px;
  }

  .empty {
    text-align: center;
    padding: 40px;
    color: var(--text2);
  }

  .hint {
    font-size: 13px;
    margin-top: 8px;
    color: var(--text3);
  }

  .hint code {
    background: var(--surface2);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .summary {
    display: flex;
    align-items: flex-end;
    gap: 12px;
    margin-bottom: 20px;
    flex-wrap: wrap;
  }

  .summary-card {
    min-width: 130px;
    padding: 14px 18px;
  }

  .summary-label {
    font-size: 12px;
    color: var(--text3);
    margin-bottom: 4px;
  }

  .summary-value {
    font-size: 1.4rem;
    font-weight: 600;
    font-family: 'Menlo', 'Consolas', monospace;
    color: var(--text);
  }

  .updated {
    font-size: 12px;
    color: var(--text3);
    margin-left: auto;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(420px, 1fr));
    gap: 16px;
  }

  .metric-card {
    padding: 18px;
  }

  .metric-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
  }

  .metric-name {
    font-family: 'Menlo', 'Consolas', monospace;
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .host-tag {
    font-size: 11px;
    color: var(--text3);
    background: var(--surface2);
    border-radius: 4px;
    padding: 2px 6px;
    white-space: nowrap;
  }

  .metric-error {
    color: #e94560;
    font-size: 13px;
    padding: 10px 0;
  }

  @media (max-width: 720px) {
    .cards {
      grid-template-columns: 1fr;
    }
  }
</style>