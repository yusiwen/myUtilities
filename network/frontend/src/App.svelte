<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let tab = $state('dns')

  // DNS Lookup
  let dnsHost = $state('')
  let dnsType = $state('A')
  let dnsResults = $state([])
  let dnsTime = $state(0)
  let dnsError = $state('')

  // DIG
  let digHost = $state('')
  let digType = $state('A')
  let digNs = $state('')
  let digOutput = $state('')
  let digError = $state('')

  // WHOIS
  let whoisDomain = $state('')
  let whoisResult = $state('')
  let whoisError = $state('')

  async function doWhois() {
    if (!whoisDomain.trim()) { whoisError = 'Domain is required'; return }
    whoisError = ''; whoisResult = ''
    try {
      const r = await fetch('/api/network/whois', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ domain: whoisDomain.trim() }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      whoisResult = (await r.json()).whois
    } catch (e) { whoisError = e.message }
  }

  async function doDNS() {
    if (!dnsHost.trim()) { dnsError = 'Host is required'; return }
    dnsError = ''; dnsResults = []; dnsTime = 0
    try {
      const r = await fetch('/api/network/dns', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ host: dnsHost.trim(), type: dnsType }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      const d = await r.json()
      dnsResults = d.results || []; dnsTime = d.queryTime || 0
    } catch (e) { dnsError = e.message }
  }

  async function doDig() {
    if (!digHost.trim()) { digError = 'Host is required'; return }
    digError = ''; digOutput = ''
    try {
      const r = await fetch('/api/network/dig', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ host: digHost.trim(), type: digType, ns: digNs || undefined }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      digOutput = (await r.json()).dig
    } catch (e) { digError = e.message }
  }

  function dnsResultsText() {
    return dnsResults.map(r => `${r.name} → ${r.value} (${r.type}, TTL: ${r.ttl})`).join('\n')
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text) } catch {
      const ta = document.createElement('textarea'); ta.value = text
      ta.style.position = 'fixed'; ta.style.opacity = '0'
      document.body.appendChild(ta); ta.select(); document.execCommand('copy'); document.body.removeChild(ta)
    }
  }
</script>

<div class="app">
  {#if inGateway}
    <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
  {/if}
  <h1>Network Tools</h1>

  <div class="tabs">
    <button class="tab" class:active={tab === 'dns'} onclick={() => tab = 'dns'}>DNS Lookup</button>
    <button class="tab" class:active={tab === 'dig'} onclick={() => tab = 'dig'}>DIG</button>
    <button class="tab" class:active={tab === 'whois'} onclick={() => tab = 'whois'}>WHOIS</button>
  </div>

  {#if tab === 'dns'}
    <div class="card">
      <div class="field-row">
        <div class="field" style="flex:2">
          <label for="dns-host">Host</label>
          <input id="dns-host" type="text" bind:value={dnsHost} placeholder="example.com" />
        </div>
        <div class="field">
          <label for="dns-type">Type</label>
          <select id="dns-type" bind:value={dnsType}>
            <option value="A">A</option>
            <option value="AAAA">AAAA</option>
            <option value="MX">MX</option>
            <option value="NS">NS</option>
            <option value="CNAME">CNAME</option>
            <option value="TXT">TXT</option>
            <option value="SOA">SOA</option>
            <option value="ALL">ALL</option>
          </select>
        </div>
        <div class="field" style="flex:0">
          <div style="height:1.5em"></div>
          <button class="btn" onclick={doDNS}>Query</button>
        </div>
      </div>
      {#if dnsError}<div class="msg error">{dnsError}</div>{/if}
      {#if dnsResults.length > 0}
        <div class="result-area">
          <button class="btn xs" onclick={() => copy(dnsResultsText())}>📋 Copy</button>
          <div class="dns-table">
            {#each dnsResults as r}
              <div class="dns-row"><span class="dns-type-badge">{r.type}</span><span class="dns-value">{r.value}</span><span class="dns-ttl">TTL:{r.ttl}</span></div>
            {/each}
          </div>
          <div class="dns-query-time">Query time: {dnsTime} ms</div>
        </div>
      {/if}
    </div>
  {:else if tab === 'dig'}
    <div class="card">
      <div class="field-row">
        <div class="field" style="flex:2">
          <label for="dig-host">Host</label>
          <input id="dig-host" type="text" bind:value={digHost} placeholder="example.com" />
        </div>
        <div class="field">
          <label for="dig-type">Type</label>
          <select id="dig-type" bind:value={digType}>
            <option value="A">A</option>
            <option value="AAAA">AAAA</option>
            <option value="MX">MX</option>
            <option value="NS">NS</option>
            <option value="CNAME">CNAME</option>
            <option value="TXT">TXT</option>
            <option value="SOA">SOA</option>
          </select>
        </div>
      </div>
      <div class="field-row">
        <div class="field" style="flex:2">
          <label for="dig-ns">Nameserver (optional)</label>
          <input id="dig-ns" type="text" bind:value={digNs} placeholder="8.8.8.8" />
        </div>
        <div class="field" style="flex:0">
          <div style="height:1.5em"></div>
          <button class="btn" onclick={doDig}>Dig</button>
        </div>
      </div>
      {#if digError}<div class="msg error">{digError}</div>{/if}
      {#if digOutput}
        <div class="result-area">
          <button class="btn xs" onclick={() => copy(digOutput)}>📋 Copy</button>
          <pre class="dig-block">{digOutput}</pre>
        </div>
      {/if}
    </div>
  {:else}
    <div class="card">
      <div class="field">
        <label for="whois-domain">Domain or IP</label>
        <div class="field-row">
          <input id="whois-domain" type="text" bind:value={whoisDomain} placeholder="example.com" style="flex:1" />
          <button class="btn" onclick={doWhois}>Lookup</button>
        </div>
      </div>
      {#if whoisError}<div class="msg error">{whoisError}</div>{/if}
      {#if whoisResult}
        <div class="result-area">
          <button class="btn xs" onclick={() => copy(whoisResult)}>📋 Copy</button>
          <pre class="dig-block">{whoisResult}</pre>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .app { max-width: 800px; margin: 0 auto; padding: 40px 16px; }
  h1 { font-size: 24px; margin-bottom: 16px; }
  .home-link { float: left; }

  .tabs { display: flex; gap: 0; margin-bottom: 16px; border-bottom: 1px solid var(--border); }
  .tab { padding: 10px 20px; border: none; background: none; color: var(--text2); cursor: pointer; font-size: 14px; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .tab.active { color: var(--text); border-bottom-color: var(--primary); }
  .tab:hover { color: var(--text); }

  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; }
  .field { margin-bottom: 14px; }
  .field label { display: block; font-size: 13px; color: var(--text2); margin-bottom: 4px; font-weight: 500; }
  .field input, .field select { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; font-family: inherit; outline: none; }
  .field input:focus, .field select:focus { border-color: var(--primary); }
  .field-row { display: flex; gap: 12px; align-items: flex-end; }
  .field-row .field { flex: 1; }

  .btn { display: inline-block; padding: 10px 24px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); cursor: pointer; font-size: 14px; }
  .btn:hover { background: var(--surface2); }
  .btn:disabled { opacity: .5; cursor: not-allowed; }
  .btn.xs { font-size: 11px; padding: 2px 8px; display: inline-flex; align-items: center; gap: 4px; }

  .msg.error { background: #3d1f2a; border: 1px solid #e94560; color: #e94560; padding: 10px 14px; border-radius: 6px; margin-bottom: 10px; font-size: 14px; }
  .result-area { margin-top: 14px; }
  .result-area .btn.xs { margin-bottom: 6px; }

  .dns-table { background: var(--bg); border-radius: 8px; padding: 8px; }
  .dns-row { display: flex; align-items: center; gap: 10px; padding: 6px 8px; border-bottom: 1px solid var(--border); }
  .dns-row:last-child { border-bottom: none; }
  .dns-type-badge { display: inline-block; padding: 1px 6px; border-radius: 4px; background: var(--primary); color: #fff; font-size: 11px; font-weight: 700; min-width: 40px; text-align: center; }
  .dns-value { flex: 1; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; word-break: break-all; }
  .dns-ttl { font-size: 11px; color: var(--text2); white-space: nowrap; }
  .dns-query-time { font-size: 12px; color: var(--text2); margin-top: 8px; }

  .dig-block { background: var(--bg); padding: 16px; border-radius: 8px; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-all; margin: 0; }
</style>
