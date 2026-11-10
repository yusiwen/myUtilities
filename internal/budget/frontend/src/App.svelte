<script>
  import { onMount } from 'svelte'

  let balances = $state([])
  let loading = $state(true)
  let error = $state('')

  let status = $state(null)
  let statusError = $state('')
  let statusLoading = $state(true)
  let pollAfter = $state(30)

  const inGateway = typeof window !== 'undefined' && window.__MU_GATEWAY__

  async function fetchBalances() {
    loading = true
    error = ''
    try {
      const res = await fetch('/api/budget/balance')
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `HTTP ${res.status}`)
      }
      balances = await res.json()
    } catch (e) {
      error = e.message
      balances = []
    } finally {
      loading = false
    }
  }

  async function fetchStatus() {
    try {
      const res = await fetch('/api/budget/status')
      const data = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error(data?.error || `HTTP ${res.status}`)
      }
      status = data
      statusError = ''
      pollAfter = data?.health?.poll_after_seconds || 30
    } catch (e) {
      statusError = e.message
    } finally {
      statusLoading = false
    }
  }

  function refreshAll() {
    fetchBalances()
    fetchStatus()
  }

  $effect(() => { fetchBalances() })

  // Poll on the interval the status widget itself recommends.
  onMount(() => {
    let cancelled = false
    let timer
    const tick = async () => {
      await fetchStatus()
      if (cancelled) return
      const wait = Math.min(Math.max(pollAfter || 30, 15), 300)
      timer = setTimeout(tick, wait * 1000)
    }
    tick()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  })

  function currencyLabel(code) {
    switch (code) {
      case 'CNY': return '¥'
      case 'USD': return '$'
      default: return ''
    }
  }

  function fmtAge(seconds) {
    if (seconds === null || seconds === undefined || seconds < 0) return ''
    if (seconds < 60) return `${Math.round(seconds)}s`
    if (seconds < 3600) return `${Math.round(seconds / 60)}m`
    if (seconds < 86400) return `${Math.round(seconds / 3600)}h`
    return `${Math.floor(seconds / 86400)}d ${Math.round((seconds % 86400) / 3600)}h`
  }

  function fmtTime(iso) {
    if (!iso) return ''
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso
    return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
  }

  function maintenanceWindow(m) {
    const start = m.scheduled_start_at || m.starts_at
    const end = m.scheduled_end_at || m.ends_at
    if (!start && !end) return ''
    return `${start ? fmtTime(start) : '?'} → ${end ? fmtTime(end) : '?'}`
  }
</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">budget</span>
  </a>
</header>

<div class="app">
  <div class="page-head row">
    <div>
      <h1>API Budget</h1>
      <p>LLM API balance, usage and service status</p>
    </div>
    <button class="btn" onclick={refreshAll} disabled={loading}>{loading ? 'Refreshing...' : 'Refresh'}</button>
  </div>

  {#if error}
    <div class="msg error block">{error}</div>
  {/if}

  <div class="card status-card">
    {#if status?.health}
      {@const h = status.health}
      <div class="status-head">
        <span class="dot {h.level}" aria-hidden="true"></span>
        <span class="status-title">DeepSeek service</span>
        <span class="status-badge {h.level}">{h.overall_status || 'unknown'}</span>
        <span class="status-checked">
          checked {fmtTime(h.checked_at)}{status.cached ? ' · cached' : ''}
        </span>
      </div>
      <p class="status-headline">{h.headline}</p>
      <p class="status-detail">{h.detail}</p>

      {#if h.incident_count > 0 && status.summary?.ongoing_incidents?.length}
        <ul class="events">
          {#each status.summary.ongoing_incidents as inc}
            <li class="event">
              <div class="event-head">
                <span class="event-title">{inc.name || 'Unnamed incident'}</span>
                <span class="event-tags">
                  {[inc.impact, inc.phase].filter(Boolean).join(' · ')}
                </span>
              </div>
              {#if inc.last_update?.message}
                <p class="event-msg">{inc.last_update.message}</p>
              {/if}
              {#if inc.affected_components?.length}
                <p class="event-meta">
                  Affected: {inc.affected_components.map((c) => c.name || c.id).filter(Boolean).join(', ')}
                </p>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if status.summary?.in_progress_maintenances?.length}
        <ul class="events">
          {#each status.summary.in_progress_maintenances as m}
            <li class="event">
              <div class="event-head">
                <span class="event-title">{m.name || 'Unnamed maintenance'}</span>
                <span class="event-tags">in progress</span>
              </div>
              {#if maintenanceWindow(m)}<p class="event-meta">{maintenanceWindow(m)}</p>{/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if status.summary?.scheduled_maintenances?.length}
        <ul class="events">
          {#each status.summary.scheduled_maintenances as m}
            <li class="event">
              <div class="event-head">
                <span class="event-title">{m.name || 'Unnamed maintenance'}</span>
                <span class="event-tags">scheduled</span>
              </div>
              {#if maintenanceWindow(m)}<p class="event-meta">{maintenanceWindow(m)}</p>{/if}
            </li>
          {/each}
        </ul>
      {/if}

      <div class="status-foot">
        <span>snapshot {fmtTime(h.generated_at)}</span>
        {#if h.data_age_seconds >= 0}
          <span>{fmtAge(h.data_age_seconds)} old</span>
        {/if}
        {#if h.stale}
          <span class="stale">older than the {h.max_stale_seconds}s freshness window — upstream may not have refreshed</span>
        {/if}
      </div>
    {:else if statusError}
      <div class="status-head">
        <span class="dot unknown" aria-hidden="true"></span>
        <span class="status-title">DeepSeek service</span>
        <span class="status-checked">unavailable</span>
      </div>
      <p class="status-detail">Status check failed: {statusError}</p>
    {:else}
      <div class="status-head">
        <span class="dot unknown" aria-hidden="true"></span>
        <span class="status-title">DeepSeek service</span>
        <span class="status-checked">{statusLoading ? 'checking…' : 'no data'}</span>
      </div>
    {/if}
  </div>

  {#if loading && balances.length === 0}
    <p class="muted">Loading balances...</p>
  {:else if balances.length === 0 && !error}
    <div class="card empty">
      <p>No providers configured</p>
      <p class="hint">Add API keys to <code>~/.config/mu/budget-config.json</code></p>
    </div>
  {:else}
    <div class="cards">
      {#each balances as b}
        <div class="card balance-card">
          <div class="provider-name">
            <span class="provider-label">{b.provider === 'deepseek' ? 'DeepSeek' : b.provider === 'openrouter' ? 'OpenRouter' : b.provider === 'aliyun' ? 'Aliyun' : b.provider}</span>
            <span class="currency">{b.currency}</span>
            {#if b.extra?.top_up_url}
              <a href={b.extra.top_up_url} target="_blank" rel="noopener noreferrer" class="top-up">Top up</a>
            {/if}
          </div>

          {#if b.error}
            <div class="msg error">{b.error}</div>
          {:else}
            <div class="balance-row">
              <span class="label">Total</span>
              <span class="value">{currencyLabel(b.currency)}{b.total.toFixed(2)}</span>
            </div>
            {#if b.used}
              <div class="balance-row">
                <span class="label">Used</span>
                <span class="value">{currencyLabel(b.currency)}{b.used.toFixed(2)}</span>
              </div>
            {/if}
            <div class="balance-row remaining">
              <span class="label">Remaining</span>
              <span class="value">{currencyLabel(b.currency)}{b.remaining.toFixed(2)}</span>
            </div>

            {#if b.provider === 'deepseek' && b.is_available !== undefined}
              <div class="balance-row">
                <span class="label">Available</span>
                <span class="value">{b.is_available ? 'Yes' : 'No'}</span>
              </div>
            {/if}

            {#if b.extra?.granted_balance || b.extra?.topped_up_balance}
              <div class="extra-row">topped up {b.extra.topped_up_balance}, granted {b.extra.granted_balance}</div>
            {/if}
            {#if b.provider === 'aliyun' && b.extra?.available_cash}
              <div class="extra-row">
                Cash {currencyLabel(b.currency)}{parseFloat(b.extra.available_cash).toFixed(2)}, credit {currencyLabel(b.currency)}{parseFloat(b.extra.credit_amount || '0').toFixed(2)}
              </div>
            {/if}
            {#if b.extra?.packages}
              {@const pkgs = JSON.parse(b.extra.packages)}
              <div class="packages">
                {#each pkgs as pkg}
                  <div class="pkg-row">
                    <span class="pkg-name">{pkg.package_type}</span>
                    <span class="pkg-amount">{pkg.remaining_amount}{pkg.remaining_unit} / {pkg.total_amount}{pkg.total_unit}</span>
                  </div>
                  {#if pkg.remark}<div class="pkg-meta">{pkg.remark}</div>{/if}
                  <div class="pkg-meta">{pkg.expiry_time?.split('T')[0]}</div>
                {/each}
              </div>
            {/if}
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  :global(body) { --page-max: 720px; }
  .block { margin-bottom: 16px; }
  .hint { margin-top: 8px; color: var(--text3); font-size: 13px; }
  .hint code { padding: 2px 6px; border-radius: 4px; background: var(--surface2); word-break: break-all; }

  .status-card { margin-bottom: 16px; display: flex; flex-direction: column; gap: 6px; }
  .status-head { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--text3); }
  .dot.healthy { background: var(--success); }
  .dot.degraded { background: var(--warning); }
  .dot.outage { background: var(--danger); }
  .dot.maintenance { background: var(--info); }
  .status-title { font-size: 14px; font-weight: 500; }
  .status-badge { padding: 0 6px; border: 1px solid var(--border); border-radius: 4px; color: var(--text2); font-family: var(--mono); font-size: 11px; }
  .status-badge.healthy { color: var(--success); }
  .status-badge.degraded, .status-badge.maintenance { color: var(--warning); }
  .status-badge.outage { color: var(--danger); }
  .status-checked { margin-left: auto; color: var(--text3); font-family: var(--mono); font-size: 11px; }
  .status-headline { font-size: 15px; }
  .status-detail { color: var(--text2); font-size: 13px; }

  .events { display: flex; flex-direction: column; gap: 8px; margin-top: 4px; padding-top: 10px; border-top: 1px solid var(--border); list-style: none; }
  .event-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
  .event-title { font-size: 13px; font-weight: 500; }
  .event-tags { color: var(--text3); font-family: var(--mono); font-size: 11px; white-space: nowrap; }
  .event-msg { color: var(--text2); font-size: 12px; }
  .event-meta { color: var(--text3); font-size: 11px; }

  .status-foot { display: flex; flex-wrap: wrap; gap: 4px 10px; margin-top: 4px; padding-top: 10px; border-top: 1px solid var(--border); color: var(--text3); font-family: var(--mono); font-size: 11px; }
  .status-foot .stale { color: var(--warning); }

  .cards { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; }
  .balance-card { display: flex; flex-direction: column; gap: 10px; }

  .provider-name { display: flex; align-items: baseline; gap: 8px; font-size: 15px; font-weight: 500; }
  .currency { color: var(--text3); font-family: var(--mono); font-size: 12px; }
  .top-up { margin-left: auto; color: var(--text2); font-size: 12px; }
  .top-up:hover { color: var(--text); }

  .balance-row { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
  .balance-row .label { color: var(--text2); font-size: 13px; }
  .balance-row .value { color: var(--text); font-family: var(--mono); font-size: 13px; font-variant-numeric: tabular-nums; }
  .balance-row.remaining { padding-top: 8px; border-top: 1px solid var(--border); }
  .balance-row.remaining .value { font-size: 16px; }

  .extra-row { padding-top: 8px; border-top: 1px solid var(--border); color: var(--text3); font-family: var(--mono); font-size: 12px; }

  .packages { padding-top: 8px; border-top: 1px solid var(--border); }
  .pkg-row { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; padding: 2px 0; }
  .pkg-name { min-width: 0; overflow: hidden; color: var(--text2); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
  .pkg-amount { color: var(--text3); font-family: var(--mono); font-size: 12px; white-space: nowrap; }
  .pkg-meta { color: var(--text3); font-size: 11px; }

  @media (max-width: 700px) {
    .page-head.row { flex-wrap: wrap; align-items: flex-start; }
    .page-head.row .btn { width: 100%; }
    .status-checked { margin-left: 0; }
  }
</style>
