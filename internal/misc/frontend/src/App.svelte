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
  }
</script>

<div class="app">
  {#if inGateway}
    <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
  {/if}
  <h1>Misc Tools</h1>

  <div class="tabs">
    <button class="tab" class:active={tab === 'json'} onclick={() => tab = 'json'}>JSON</button>
    <button class="tab" class:active={tab === 'uuid'} onclick={() => tab = 'uuid'}>UUID</button>
    <button class="tab" class:active={tab === 'timestamp'} onclick={() => tab = 'timestamp'}>Timestamp</button>
    <button class="tab" class:active={tab === 'hash'} onclick={() => tab = 'hash'}>Hash</button>
  </div>

  {#if tab === 'json'}
    <div class="card">
      <div class="field">
        <label for="json-input">Input</label>
        <textarea id="json-input" bind:value={jsonInput} rows="8" placeholder="Paste JSON here"></textarea>
      </div>
      <div class="field-row btn-row">
        <button class="btn" onclick={() => jsonOp('format')}>Format</button>
        <button class="btn" onclick={() => jsonOp('validate')}>Validate</button>
        <button class="btn" onclick={() => jsonOp('minify')}>Minify</button>
      </div>
      {#if jsonError}<div class="msg error">{jsonError}</div>{/if}
      {#if jsonOutput}
        <div class="result-area">
          <button class="btn xs" onclick={() => copy(jsonOutput)}>📋 Copy</button>
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
        <div class="result-area">
          <button class="btn xs" onclick={() => copy(uuids.join('\n'))}>📋 Copy All</button>
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
        <div class="result-area">
          <code class="result-text">{tsResult}</code>
          <button class="btn xs" onclick={() => copy(tsResult)}>📋 Copy</button>
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
        <div class="field" style="flex:0">
          <div style="height:1.5em"></div>
          <button class="btn xs" onclick={() => document.getElementById('hash-file').click()}>📂 Upload</button>
          <input id="hash-file" type="file" accept=".txt,.md,.js,.go,.json,.yaml,.yml" onchange={loadHashFile} style="display:none" />
        </div>
      </div>
      <div class="field">
        <label for="hash-input">Input text</label>
        <textarea id="hash-input" bind:value={hashInput} rows="6" placeholder="Text to hash, or upload a file"></textarea>
      </div>
      <button class="btn primary" onclick={doHash} disabled={!hashInput.trim()}>Hash</button>
      {#if hashResult}
        <div class="result-area">
          <code class="result-text result-mono">{hashResult}</code>
          <button class="btn xs" onclick={() => copy(hashResult)}>📋 Copy</button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .app { max-width: 720px; margin: 0 auto; padding: 40px 16px; }
  h1 { font-size: 24px; margin-bottom: 16px; }
  .home-link { float: left; }

  .tabs { display: flex; gap: 0; margin-bottom: 16px; border-bottom: 1px solid var(--border); }
  .tab { padding: 10px 20px; border: none; background: none; color: var(--text2); cursor: pointer; font-size: 14px; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .tab.active { color: var(--text); border-bottom-color: var(--primary); }
  .tab:hover { color: var(--text); }

  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; margin-bottom: 16px; }
  .field { margin-bottom: 14px; }
  .field label { display: block; font-size: 13px; color: var(--text2); margin-bottom: 4px; font-weight: 500; }
  .field input, .field select, .field textarea { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; font-family: inherit; outline: none; }
  .field input:focus, .field select:focus, .field textarea:focus { border-color: var(--primary); }
  .field textarea { font-family: 'SF Mono', 'Fira Code', monospace; resize: vertical; }
  .field-row { display: flex; gap: 12px; align-items: flex-end; }
  .field-row .field { flex: 1; }
  .btn-row { margin-bottom: 10px; }
  .btn-row .btn { flex: 1; text-align: center; }

  .btn { display: inline-block; padding: 10px 24px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); cursor: pointer; font-size: 14px; }
  .btn:hover { background: var(--surface2); }
  .btn.primary { background: var(--primary); border-color: var(--primary); color: #fff; width: 100%; text-align: center; }
  .btn.primary:hover { opacity: .85; }
  .btn:disabled { opacity: .5; cursor: not-allowed; }
  .btn.xs { font-size: 11px; padding: 2px 8px; display: inline-flex; align-items: center; gap: 4px; }

  .msg.error { background: #3d1f2a; border: 1px solid #e94560; color: #e94560; padding: 10px 14px; border-radius: 6px; margin-bottom: 10px; font-size: 14px; }
  .result-area { margin-top: 14px; }
  .result-area .btn.xs { margin-bottom: 6px; }
  .result-block { background: var(--bg); padding: 14px; border-radius: 8px; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; white-space: pre-wrap; word-break: break-all; line-height: 1.5; margin: 0; }
  .result-text { font-size: 14px; word-break: break-all; }
  .result-mono { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; }
</style>
