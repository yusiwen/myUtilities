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

  // SSL Cert
  let certDomain = $state('')
  let certResult = $state('')
  let certError = $state('')

  async function doCert() {
    if (!certDomain.trim()) { certError = 'Domain is required'; return }
    certError = ''; certResult = ''
    try {
      const r = await fetch('/api/network/cert', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ domain: certDomain.trim() }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      certResult = (await r.json()).cert
    } catch (e) { certError = e.message }
  }

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

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">network</span>
  </a>
</header>

<div class="app">
  <div class="page-head">
    <h1>Network tools</h1>
    <p>DNS lookup and dig queries</p>
  </div>

  <div class="tabs">
    <button class="tab" class:active={tab === 'dns'} onclick={() => tab = 'dns'}>DNS lookup</button>
    <button class="tab" class:active={tab === 'dig'} onclick={() => tab = 'dig'}>DIG</button>
    <button class="tab" class:active={tab === 'whois'} onclick={() => tab = 'whois'}>WHOIS</button>
    <button class="tab" class:active={tab === 'cert'} onclick={() => tab = 'cert'}>SSL cert</button>
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
        <button class="btn" onclick={doDNS}>Query</button>
      </div>
      {#if dnsError}<div class="msg error block">{dnsError}</div>{/if}
      {#if dnsResults.length > 0}
        <div class="result">
          <button class="btn xs" onclick={() => copy(dnsResultsText())}>Copy</button>
          <div class="dns-table">
            {#each dnsResults as r}
              <div class="dns-row"><span class="badge">{r.type}</span><span class="dns-value">{r.value}</span><span class="dns-ttl">{r.ttl}</span></div>
            {/each}
          </div>
          <div class="dns-meta">Query time {dnsTime} ms</div>
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
        <button class="btn" onclick={doDig}>Dig</button>
      </div>
      {#if digError}<div class="msg error block">{digError}</div>{/if}
      {#if digOutput}
        <div class="result">
          <button class="btn xs" onclick={() => copy(digOutput)}>Copy</button>
          <pre class="dig-block">{digOutput}</pre>
        </div>
      {/if}
    </div>
  {:else if tab === 'whois'}
    <div class="card">
      <div class="field">
        <label for="whois-domain">Domain or IP</label>
        <div class="field-row">
          <input id="whois-domain" type="text" bind:value={whoisDomain} placeholder="example.com" style="flex:1" />
          <button class="btn" onclick={doWhois}>Lookup</button>
        </div>
      </div>
      {#if whoisError}<div class="msg error block">{whoisError}</div>{/if}
      {#if whoisResult}
        <div class="result">
          <button class="btn xs" onclick={() => copy(whoisResult)}>Copy</button>
          <pre class="dig-block">{whoisResult}</pre>
        </div>
      {/if}
    </div>
  {:else}
    <div class="card">
      <div class="field">
        <label for="cert-domain">Domain</label>
        <div class="field-row">
          <input id="cert-domain" type="text" bind:value={certDomain} placeholder="example.com" style="flex:1" />
          <button class="btn" onclick={doCert}>Check</button>
        </div>
      </div>
      {#if certError}<div class="msg error block">{certError}</div>{/if}
      {#if certResult}
        <div class="result">
          <button class="btn xs" onclick={() => copy(certResult)}>Copy</button>
          <pre class="dig-block">{certResult}</pre>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  :global(body) { --page-max: 800px; }
  .block { margin-bottom: 16px; }

  .result { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; margin-top: 16px; }
  .result .btn { align-self: flex-start; }

  .dns-table { width: 100%; border: 1px solid var(--border); border-radius: var(--r-md); overflow: hidden; }
  .dns-row { display: flex; align-items: center; gap: 12px; padding: 8px 12px; }
  .dns-row + .dns-row { border-top: 1px solid var(--border); }
  .dns-value { flex: 1; font-family: var(--mono); font-size: 13px; color: var(--text); word-break: break-all; }
  .dns-ttl { color: var(--text3); font-family: var(--mono); font-size: 12px; white-space: nowrap; }
  .dns-meta { color: var(--text3); font-family: var(--mono); font-size: 12px; }

  .dig-block {
    width: 100%;
    margin: 0;
    padding: 12px 14px;
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--bg);
    font-size: 13px;
    line-height: 1.55;
    white-space: pre-wrap;
    word-break: break-all;
  }

  .field-row { margin-bottom: 16px; }
  .field-row:last-child { margin-bottom: 0; }
</style>
