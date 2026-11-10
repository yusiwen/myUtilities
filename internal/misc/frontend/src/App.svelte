<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let tab = $state('json')

  // JSON Tool
  let jsonInput = $state('')
  let jsonOutput = $state('')
  let jsonError = $state('')

  // UUID
  let uuidCount = $state(1)
  let uuids = $state([])

  // Timestamp
  let tsInput = $state('')
  let tsResult = $state('')

  // Hash
  let hashAlg = $state('sha256')
  let hashInput = $state('')
  let hashResult = $state('')

  // Trackers
  let trackers = $state('')
  let trackersLoading = $state(false)
  let trackersError = $state('')

  // Copy feedback
  let copiedText = $state('')
  let copiedTimer

  async function jsonOp(op) {
    if (!jsonInput.trim()) { jsonError = 'Input is required'; return }
    jsonError = ''; jsonOutput = ''
    try {
      const r = await fetch('/api/misc/json/' + op, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ input: jsonInput }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      jsonOutput = (await r.json()).result
    } catch (e) { jsonError = e.message }
  }

  async function genUuid() {
    try {
      const r = await fetch('/api/misc/uuid', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ count: uuidCount }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      uuids = (await r.json()).uuids
    } catch (e) { jsonError = e.message }
  }

  async function doTimestamp() {
    if (!tsInput.trim()) { tsResult = ''; return }
    try {
      const r = await fetch('/api/misc/timestamp', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ input: tsInput }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      tsResult = (await r.json()).result
    } catch (e) { tsResult = 'Error: ' + e.message }
  }

  function loadHashFile(e) {
    const file = e.target.files[0]; if (!file) return
    const reader = new FileReader()
    reader.onload = () => { hashInput = reader.result }
    reader.readAsText(file)
  }

  async function doHash() {
    if (!hashInput.trim()) return
    try {
      const r = await fetch('/api/misc/hash/' + hashAlg, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ input: hashInput }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      hashResult = (await r.json()).result
    } catch (e) { hashResult = 'Error: ' + e.message }
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text) } catch {
      const ta = document.createElement('textarea'); ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0'
      document.body.appendChild(ta); ta.select(); document.execCommand('copy'); document.body.removeChild(ta)
    }
    copiedText = text
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => copiedText = '', 1500)
  }

  async function loadTrackers(force) {
    trackersLoading = true; trackersError = ''
    try {
      const r = await fetch('/api/misc/trackers' + (force ? '?refresh=1' : ''))
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      trackers = (await r.json()).content
    } catch (e) { trackersError = e.message }
    trackersLoading = false
  }

  async function openTrackers() {
    tab = 'trackers'
    if (!trackers) await loadTrackers()
  }
</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">misc</span>
  </a>
</header>

<div class="app">
  <div class="page-head">
    <h1>Misc tools</h1>
    <p>JSON, UUID, timestamp, hash</p>
  </div>

  <div class="tabs">
    <button class="tab" class:active={tab === 'json'} onclick={() => tab = 'json'}>JSON</button>
    <button class="tab" class:active={tab === 'uuid'} onclick={() => tab = 'uuid'}>UUID</button>
    <button class="tab" class:active={tab === 'timestamp'} onclick={() => tab = 'timestamp'}>Timestamp</button>
    <button class="tab" class:active={tab === 'hash'} onclick={() => tab = 'hash'}>Hash</button>
    <button class="tab" class:active={tab === 'trackers'} onclick={openTrackers}>Trackers</button>
  </div>

  {#if tab === 'json'}
    <div class="card">
      <div class="field">
        <label for="json-input">Input</label>
        <textarea id="json-input" bind:value={jsonInput} rows="8" placeholder="Paste JSON here"></textarea>
      </div>
      <div class="row">
        <button class="btn" onclick={() => jsonOp('format')}>Format</button>
        <button class="btn" onclick={() => jsonOp('validate')}>Validate</button>
        <button class="btn" onclick={() => jsonOp('minify')}>Minify</button>
      </div>
      {#if jsonError}<div class="msg error block">{jsonError}</div>{/if}
      {#if jsonOutput}
        <div class="result">
          <button class="btn xs" onclick={() => copy(jsonOutput)}>{copiedText === jsonOutput ? 'Copied' : 'Copy'}</button>
          <pre class="result-block">{jsonOutput}</pre>
        </div>
      {/if}
    </div>
  {:else if tab === 'uuid'}
    <div class="card">
      <div class="field">
        <label for="uuid-count">Count</label>
        <input id="uuid-count" type="number" bind:value={uuidCount} min="1" max="100" />
      </div>
      <button class="btn primary" onclick={genUuid}>Generate</button>
      {#if uuids.length > 0}
        <div class="result">
          <button class="btn xs" onclick={() => copy(uuids.join('\n'))}>{copiedText === uuids.join('\n') ? 'Copied' : 'Copy all'}</button>
          <div class="uuid-list">{#each uuids as u}<div class="uuid-line">{u}</div>{/each}</div>
        </div>
      {/if}
    </div>
  {:else if tab === 'timestamp'}
    <div class="card">
      <div class="field">
        <label for="ts-input">Input (Unix timestamp, ISO date, or empty for now)</label>
        <input id="ts-input" type="text" bind:value={tsInput} placeholder="1700000000 or 2024-12-01" oninput={doTimestamp} />
      </div>
      {#if tsResult}
        <div class="result">
          <code class="result-text">{tsResult}</code>
          <button class="btn xs" onclick={() => copy(tsResult)}>{copiedText === tsResult ? 'Copied' : 'Copy'}</button>
        </div>
      {/if}
    </div>
  {:else if tab === 'trackers'}
    <div class="card">
      <div class="row">
        <button class="btn" onclick={() => loadTrackers(true)} disabled={trackersLoading}>Refresh</button>
      </div>
      {#if trackersError}<div class="msg error block">{trackersError}</div>{/if}
      {#if trackersLoading}
        <div class="msg">Loading trackers list...</div>
      {:else if trackers}
        <div class="result">
          <div class="row">
            <button class="btn xs" onclick={() => copy(trackers)}>{copiedText === trackers ? 'Copied' : 'Copy all'}</button>
            <a class="btn xs" href="https://github.com/ngosang/trackerslist" target="_blank" rel="noopener noreferrer">GitHub</a>
          </div>
          <pre class="result-block">{trackers}</pre>
        </div>
      {/if}
    </div>
  {:else}
    <div class="card">
      <div class="field-row">
        <div class="field">
          <label for="hash-alg">Algorithm</label>
          <select id="hash-alg" bind:value={hashAlg}>
            <option value="sha256">SHA-256</option>
            <option value="sha512">SHA-512</option>
            <option value="md5">MD5</option>
          </select>
        </div>
        <button class="btn" onclick={() => document.getElementById('hash-file').click()}>Upload</button>
        <input id="hash-file" class="hidden-file" type="file" accept=".txt,.md,.js,.go,.json,.yaml,.yml" onchange={loadHashFile} />
      </div>
      <div class="field">
        <label for="hash-input">Input text</label>
        <textarea id="hash-input" bind:value={hashInput} rows="6" placeholder="Text to hash, or upload a file"></textarea>
      </div>
      <button class="btn primary" onclick={doHash} disabled={!hashInput.trim()}>Hash</button>
      {#if hashResult}
        <div class="result">
          <code class="result-text result-mono">{hashResult}</code>
          <button class="btn xs" onclick={() => copy(hashResult)}>{copiedText === hashResult ? 'Copied' : 'Copy'}</button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  :global(body) { --page-max: 720px; }
  .block { margin-bottom: 16px; }
  .row { display: flex; flex-wrap: wrap; gap: 8px; }
  .hidden-file { display: none; }

  .result { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; margin-top: 16px; }
  .result-block {
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
  .result-text { color: var(--text); font-size: 14px; word-break: break-all; }
  .result-mono { font-size: 13px; }

  .uuid-list { width: 100%; display: flex; flex-direction: column; gap: 2px; }
  .uuid-line { font-family: var(--mono); font-size: 13px; color: var(--text2); }
</style>
