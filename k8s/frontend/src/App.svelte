<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let tab = $state('encode')

  // Encode tab
  let secretName = $state('my-app')
  let rows = $state([{ key: '', value: '' }])
  let yamlResult = $state('')
  let encodeError = $state('')
  let encodeLoading = $state(false)

  // Decode tab
  let decodeInput = $state('')
  let decodedData = $state(null)
  let decodeError = $state('')
  let decodeLoading = $state(false)

  function addRow() { rows = [...rows, { key: '', value: '' }] }
  function removeRow(i) { rows = rows.filter((_, idx) => idx !== i) }

  function loadEnvFile(e) {
    const file = e.target.files[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const lines = reader.result.split('\n')
      const parsed = []
      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed || trimmed.startsWith('#')) continue
        const eq = trimmed.indexOf('=')
        if (eq === -1) continue
        parsed.push({ key: trimmed.slice(0, eq), value: trimmed.slice(eq + 1) })
      }
      if (parsed.length > 0) rows = parsed
    }
    reader.readAsText(file)
  }

  async function doEncode() {
    const data = {}
    for (const r of rows) {
      if (r.key.trim()) data[r.key.trim()] = r.value
    }
    if (!secretName.trim()) { encodeError = 'Secret name is required'; return }
    if (Object.keys(data).length === 0) { encodeError = 'At least one key=value pair is required'; return }
    encodeError = ''
    yamlResult = ''
    encodeLoading = true
    try {
      const r = await fetch('/api/k8s/secret', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: secretName.trim(), data }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      const d = await r.json()
      yamlResult = d.yaml
    } catch (e) {
      encodeError = e.message
    } finally {
      encodeLoading = false
    }
  }

  async function doDecode() {
    if (!decodeInput.trim()) { decodeError = 'Paste or upload a Secret YAML'; return }
    decodeError = ''
    decodedData = null
    decodeLoading = true
    try {
      const r = await fetch('/api/k8s/secret/decode', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ yaml: decodeInput }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      const d = await r.json()
      decodedData = d.data
    } catch (e) {
      decodeError = e.message
    } finally {
      decodeLoading = false
    }
  }

  function loadDecodeFile(e) {
    const file = e.target.files[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => { decodeInput = reader.result }
    reader.readAsText(file)
  }

  async function copy(text) {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
  }

  function download(filename, content) {
    const blob = new Blob([content], { type: 'application/x-yaml' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = filename
    a.click()
    URL.revokeObjectURL(a.href)
  }

  function entries(o) { return Object.entries(o || {}) }
</script>

<div class="app">
  {#if inGateway}
    <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
  {/if}
  <h1>Kubernetes Tools</h1>

  <div class="tabs">
    <button class="tab" class:active={tab === 'encode'} onclick={() => tab = 'encode'}>Secret Generator</button>
    <button class="tab" class:active={tab === 'decode'} onclick={() => tab = 'decode'}>Decode Secret</button>
  </div>

  {#if tab === 'encode'}
    <div class="card">
      <div class="field">
        <label for="secret-name">Secret Name</label>
        <input id="secret-name" type="text" bind:value={secretName} placeholder="my-app" />
      </div>

      <div class="field">
        <label>Key-Value Pairs
          <button class="btn xs upload-inline" onclick={() => document.getElementById('env-file').click()}>📂 Load .env</button>
        </label>
        <input id="env-file" type="file" accept=".env,.txt" onchange={loadEnvFile} style="display:none" />
        <div class="kv-table">
          {#each rows as r, i (i)}
            <div class="kv-row">
              <input type="text" bind:value={r.key} placeholder="Key" />
              <input type="text" bind:value={r.value} placeholder="Value" />
              <button class="btn xs" onclick={() => removeRow(i)} disabled={rows.length === 1}>x</button>
            </div>
          {/each}
          <button class="btn xs" onclick={addRow}>+ Add Row</button>
        </div>
      </div>

      <button class="btn primary" onclick={doEncode} disabled={encodeLoading}>
        {encodeLoading ? 'Generating...' : 'Generate YAML'}
      </button>

      {#if encodeError}
        <div class="msg error">{encodeError}</div>
      {/if}
      {#if yamlResult}
        <div class="result-area">
          <div class="result-actions">
            <button class="btn xs" onclick={() => copy(yamlResult)}>📋 Copy</button>
            <button class="btn xs" onclick={() => download(secretName + '-secret.yaml', yamlResult)}>💾 Download</button>
          </div>
          <pre class="yaml-block">{yamlResult}</pre>
        </div>
      {/if}
    </div>
  {:else}
    <div class="card">
      <div class="field">
        <label for="decode-yaml">Secret YAML
          <button class="btn xs upload-inline" onclick={() => document.getElementById('yaml-file').click()}>📂 Upload .yaml</button>
        </label>
        <input id="yaml-file" type="file" accept=".yaml,.yml" onchange={loadDecodeFile} style="display:none" />
        <textarea id="decode-yaml" bind:value={decodeInput} rows="12" placeholder="Paste Secret YAML here"></textarea>
      </div>

      <button class="btn primary" onclick={doDecode} disabled={decodeLoading}>
        {decodeLoading ? 'Decoding...' : 'Decode'}
      </button>

      {#if decodeError}
        <div class="msg error">{decodeError}</div>
      {/if}
      {#if decodedData}
        <div class="result-area">
          <button class="btn xs" onclick={() => copy(entries(decodedData).map(([k,v]) => k+'='+v).join('\n'))}>📋 Copy All</button>
          <div class="decode-table">
            {#each entries(decodedData) as [k, v]}
              <div class="decode-row">
                <span class="decode-key">{k}</span>
                <span class="decode-val">{v}</span>
              </div>
            {/each}
          </div>
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

  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; }
  .field { margin-bottom: 16px; }
  .field label { display: block; font-size: 13px; color: var(--text2); margin-bottom: 6px; font-weight: 500; }
  .field input, .field textarea { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; font-family: inherit; outline: none; }
  .field input:focus, .field textarea:focus { border-color: var(--primary); }
  .field textarea { font-family: 'SF Mono', 'Fira Code', monospace; resize: vertical; }

  .upload-inline { float: right; }

  .kv-table { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
  .kv-row { display: flex; gap: 8px; align-items: center; }
  .kv-row input { flex: 1; padding: 8px 10px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 13px; }

  .btn.primary { width: 100%; text-align: center; }

  .msg.error { background: #3d1f2a; border: 1px solid #e94560; color: #e94560; padding: 10px 14px; border-radius: 6px; margin-bottom: 10px; font-size: 14px; }

  .result-area { margin-top: 16px; }
  .result-actions { display: flex; gap: 6px; margin-bottom: 8px; }
  .yaml-block { background: var(--bg); padding: 14px; border-radius: 8px; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; white-space: pre-wrap; word-break: break-all; line-height: 1.5; margin: 0; }
  .decode-table { display: flex; flex-direction: column; gap: 4px; margin-top: 8px; }
  .decode-row { display: flex; gap: 8px; align-items: center; background: var(--bg); padding: 8px 12px; border-radius: 6px; }
  .decode-key { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; font-weight: 600; color: var(--primary); min-width: 140px; }
  .decode-val { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; color: var(--text); word-break: break-all; }
</style>
