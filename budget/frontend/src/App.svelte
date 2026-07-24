<script>
  let balances = $state([])
  let loading = $state(true)
  let error = $state('')

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

  $effect(() => { fetchBalances() })

  function currencyLabel(code) {
    switch (code) {
      case 'CNY': return '¥'
      case 'USD': return '$'
      default: return ''
    }
  }
</script>

<div class="container">
  <div class="header">
    <div class="header-left">
      {#if inGateway}
        <a href="/" class="home-link">← Home</a>
      {/if}
      <h1>API Budget</h1>
    </div>
    <button class="btn" onclick={fetchBalances} disabled={loading}>
      {loading ? 'Refreshing...' : 'Refresh'}
    </button>
  </div>

  {#if error}
    <div class="msg error">{error}</div>
  {/if}

  {#if loading && balances.length === 0}
    <p class="loading">Loading balances...</p>
  {:else if balances.length === 0 && !error}
    <div class="empty">
      <p>No providers configured.</p>
      <p class="hint">Add API keys to <code>~/.config/mu/budget-config.json</code></p>
    </div>
  {:else}
    <div class="cards">
      {#each balances as b}
        <div class="card balance-card">
          <div class="provider-name">
            {b.provider === 'deepseek' ? 'DeepSeek' : b.provider === 'openrouter' ? 'OpenRouter' : b.provider === 'aliyun' ? 'Aliyun' : b.provider}
            <span class="currency">{b.currency}</span>
          </div>

          {#if b.error}
            <div class="provider-error">{b.error}</div>
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
            <div class="balance-row">
              <span class="label">Remaining</span>
              <span class="value highlight">{currencyLabel(b.currency)}{b.remaining.toFixed(2)}</span>
            </div>

            {#if b.provider === 'deepseek' && b.is_available !== undefined}
              <div class="balance-row">
                <span class="label">Available</span>
                <span class="value">{b.is_available ? 'Yes' : 'No'}</span>
              </div>
            {/if}

            {#if b.extra?.granted_balance || b.extra?.topped_up_balance}
              <div class="extra-row">
                topped_up: {b.extra.topped_up_balance}, granted: {b.extra.granted_balance}
              </div>
            {/if}
            {#if b.provider === 'aliyun' && b.extra?.available_cash}
              <div class="extra-row">
                Cash: {currencyLabel(b.currency)}{parseFloat(b.extra.available_cash).toFixed(2)}, Credit: {currencyLabel(b.currency)}{parseFloat(b.extra.credit_amount || '0').toFixed(2)}
              </div>
            {/if}
            {#if b.extra?.packages}
              {@const pkgs = JSON.parse(b.extra.packages)}
              <div class="packages-section">
                {#each pkgs as pkg}
                  <div class="pkg-row">
                    <span class="pkg-name">{pkg.package_type}</span>
                    <span class="pkg-amount">{pkg.remaining_amount}{pkg.remaining_unit} / {pkg.total_amount}{pkg.total_unit}</span>
                  </div>
                  {#if pkg.remark}
                    <div class="pkg-remark">{pkg.remark}</div>
                  {/if}
                  <div class="pkg-expiry">{pkg.expiry_time?.split('T')[0]}</div>
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
  .container {
    max-width: 720px;
    margin: 0 auto;
    padding: 40px 20px;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
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

  .cards {
    display: grid;
    grid-template-columns: 1fr;
    gap: 16px;
  }

  .balance-card {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .provider-name {
    font-size: 1.1rem;
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .currency {
    font-size: 0.8rem;
    color: var(--text3);
    font-weight: 400;
  }

  .balance-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .label {
    color: var(--text2);
    font-size: 14px;
  }

  .value {
    font-family: 'Menlo', 'Consolas', monospace;
    font-size: 14px;
  }

  .value.highlight {
    font-weight: 600;
    color: var(--text);
  }

  .extra-row {
    font-size: 12px;
    color: var(--text3);
    padding-top: 4px;
    border-top: 1px solid var(--border);
  }

  .packages-section {
    padding-top: 8px;
    border-top: 1px solid var(--border);
    margin-top: 4px;
  }

  .pkg-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 3px 0;
  }

  .pkg-name {
    font-size: 12px;
    color: var(--text2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 55%;
  }

  .pkg-amount {
    font-family: 'Menlo', 'Consolas', monospace;
    font-size: 12px;
    color: var(--text3);
    white-space: nowrap;
  }

  .pkg-remark {
    font-size: 11px;
    color: var(--text3);
    padding-left: 4px;
  }

  .pkg-expiry {
    font-size: 11px;
    color: var(--text3);
    padding-left: 4px;
    margin-bottom: 4px;
  }

  .provider-error {
    color: #e94560;
    font-size: 14px;
    padding: 8px 0;
  }
</style>
