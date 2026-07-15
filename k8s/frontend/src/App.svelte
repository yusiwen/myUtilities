<script>
  import { onMount } from 'svelte'

  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let tab = $state('secret')
  let secretMode = $state('encode')

  // Secret Encode
  let secretName = $state('my-app')
  let secretRows = $state([{ key: '', value: '' }])
  let yamlResult = $state('')
  let secretError = $state('')
  let secretLoading = $state(false)

  // Secret Decode
  let decodeInput = $state('')
  let decodedData = $state(null)

  // Resources / Kubeconfig
  let kcConfig = $state(null)    // full config response from API
  let kcActive = $state(false)
  let kcSaved = $state({})       // {name: yamlContent}
  let resType = $state('pods')
  let currentCtx = $state('')
  let namespaces = $state([])
  let resNs = $state('')
  let resCols = $state([])
  let resRows = $state([])
  let isNamespaced = $derived(resCols.length > 0 && resCols[0] !== 'NAME')
  let resourceHasNs = $derived(resType !== 'nodes' && resType !== 'namespaces')

  $effect(() => { if (!resourceHasNs) resNs = '' })
  let resLoading = $state(false)
  let resError = $state('')

  // Describe modal
  let descText = $state('')
  let descTitle = $state('')
  let descLoading = $state(false)

  // Add new config form
  let newName = $state('')
  let newKc = $state('')
  let newKcFile = $state(null)

  onMount(fetchConfig)

  async function fetchConfig() {
    try {
      const r = await fetch('/api/k8s/config')
      if (r.ok) {
        const d = await r.json()
        kcSaved = d.configs || {}
        kcConfig = d
        kcActive = d.active
      }
    } catch {}
  }

  // === Secret Encode ===
  function addRow() { secretRows = [...secretRows, { key: '', value: '' }] }
  function removeRow(i) { secretRows = secretRows.filter((_, idx) => idx !== i) }

  function loadEnvFile(e) {
    const file = e.target.files[0]; if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const parsed = []
      for (const line of reader.result.split('\n')) {
        const t = line.trim(); if (!t || t.startsWith('#')) continue
        const eq = t.indexOf('='); if (eq === -1) continue
        parsed.push({ key: t.slice(0, eq), value: t.slice(eq + 1) })
      }
      if (parsed.length > 0) secretRows = parsed
    }
    reader.readAsText(file)
  }

  async function doEncode() {
    const data = {}; for (const r of secretRows) { if (r.key.trim()) data[r.key.trim()] = r.value }
    if (!secretName.trim()) { secretError = 'Secret name is required'; return }
    if (Object.keys(data).length === 0) { secretError = 'At least one key=value pair is required'; return }
    secretError = ''; yamlResult = ''; secretLoading = true
    try {
      const r = await fetch('/api/k8s/secret', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: secretName.trim(), data }) })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      yamlResult = (await r.json()).yaml
    } catch (e) { secretError = e.message } finally { secretLoading = false }
  }

  async function doDecode() {
    if (!decodeInput.trim()) { secretError = 'Paste or upload a Secret YAML'; return }
    secretError = ''; decodedData = null
    try {
      const r = await fetch('/api/k8s/secret/decode', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ yaml: decodeInput }) })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      decodedData = (await r.json()).data
    } catch (e) { secretError = e.message }
  }

  function loadYamlFile(e) {
    const file = e.target.files[0]; if (!file) return
    const reader = new FileReader()
    reader.onload = () => { decodeInput = reader.result }
    reader.readAsText(file)
  }

  // === Kubeconfig ===
  function loadNewKcFile(e) {
    const file = e.target.files[0]; if (!file) return
    const reader = new FileReader()
    reader.onload = () => { newKc = reader.result }
    reader.readAsText(file)
  }

  async function uploadKubeconfig() {
    if (!newKc.trim()) return
    resError = ''
    try {
      const r = await fetch('/api/k8s/config', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newName || undefined, kubeconfig: newKc }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      const d = await r.json()
      kcConfig = d; kcActive = true; kcSaved = d.configs || {}; currentCtx = d.currentContext || ''
      newKc = ''; newName = ''; loadNamespaces()
    } catch (e) { resError = e.message }
  }

  async function activateConfig(name) {
    resError = ''
    try {
      const r = await fetch(`/api/k8s/configs/${encodeURIComponent(name)}`, { method: 'POST' })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      const d = await r.json()
      kcConfig = d; kcActive = true; kcSaved = d.configs || {}; currentCtx = d.currentContext || ''
      resCols = []; resRows = []; loadNamespaces()
    } catch (e) { resError = e.message }
  }

  async function deleteConfig(name) {
    resError = ''
    try {
      const r = await fetch(`/api/k8s/configs/${encodeURIComponent(name)}`, { method: 'DELETE' })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      const d = await r.json()
      kcSaved = d.configs || {}
      if (name === kcConfig?.activeName) { kcActive = false; kcConfig = null }
    } catch (e) { resError = e.message }
  }

  async function switchContext() {
    try {
      const r = await fetch('/api/k8s/config', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ switchContext: currentCtx }),
      })
      if (r.ok) {
        const d = await r.json()
        kcConfig = d; kcSaved = d.configs || {}; currentCtx = d.currentContext || ''
        resCols = []; resRows = []; loadNamespaces()
      }
    } catch (e) { resError = e.message }
  }

  async function disconnect() {
    await fetch('/api/k8s/config', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ deactivate: true }),
    })
    kcActive = false; kcConfig = null; resCols = []; resRows = []
  }

  async function doDescribe(name, ns) {
    descTitle = `Describe ${resType}: ${name}`
    descText = ''
    descLoading = true
    try {
      const r = await fetch('/api/k8s/describe', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: resType, name, namespace: ns || '' }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      descText = (await r.json()).describe
    } catch (e) { descText = 'Error: ' + e.message } finally { descLoading = false }
  }

  async function loadNamespaces() {
    try {
      const r = await fetch('/api/k8s/namespaces')
      if (r.ok) namespaces = await r.json()
    } catch {}
  }

  async function doQuery() {
    resLoading = true; resError = ''; resCols = []; resRows = []
    try {
      const r = await fetch('/api/k8s/resources', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ type: resType, namespace: resNs }) })
      if (!r.ok) throw new Error((await r.text()) || 'failed')
      const d = await r.json()
      resCols = d.columns || []; resRows = d.rows || []
    } catch (e) { resError = e.message } finally { resLoading = false }
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text) } catch {
      const ta = document.createElement('textarea'); ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0'
      document.body.appendChild(ta); ta.select(); document.execCommand('copy'); document.body.removeChild(ta)
    }
  }

  function download(filename, content) {
    const blob = new Blob([content], { type: 'text/plain' })
    const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = filename
    a.click(); URL.revokeObjectURL(a.href)
  }

  function entries(o) { return Object.entries(o || {}) }
</script>

<div class="app">
  {#if inGateway}
    <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
  {/if}
  <h1>Kubernetes Tools</h1>

  <div class="tabs">
    <button class="tab" class:active={tab === 'secret'} onclick={() => tab = 'secret'}>Secret</button>
    <button class="tab" class:active={tab === 'resources'} onclick={() => tab = 'resources'}>Resources</button>
  </div>

  {#if tab === 'secret'}
    <div class="card">
      <div class="mode-switch">
        <button class="mode-btn" class:active={secretMode === 'encode'} onclick={() => secretMode = 'encode'}>Encode</button>
        <button class="mode-btn" class:active={secretMode === 'decode'} onclick={() => secretMode = 'decode'}>Decode</button>
      </div>

      {#if secretMode === 'encode'}
        <div class="field">
          <label for="secret-name">Secret Name</label>
          <input id="secret-name" type="text" bind:value={secretName} placeholder="my-app" />
        </div>
        <div class="field">
          <label>Key-Value Pairs <button class="btn xs upload-inline" onclick={() => document.getElementById('env-file').click()}>📂 Load .env</button></label>
          <input id="env-file" type="file" accept=".env,.txt" onchange={loadEnvFile} style="display:none" />
          <div class="kv-table">
            {#each secretRows as r, i (i)}
              <div class="kv-row"><input type="text" bind:value={r.key} placeholder="Key" /><input type="text" bind:value={r.value} placeholder="Value" /><button class="btn xs" onclick={() => removeRow(i)} disabled={secretRows.length === 1}>x</button></div>
            {/each}
            <button class="btn xs" onclick={addRow}>+ Add Row</button>
          </div>
        </div>
        <button class="btn primary" onclick={doEncode} disabled={secretLoading}>{secretLoading ? 'Generating...' : 'Generate YAML'}</button>
        {#if secretError}<div class="msg error">{secretError}</div>{/if}
        {#if yamlResult}
          <div class="result-area">
            <div class="result-actions"><button class="btn xs" onclick={() => copy(yamlResult)}>📋 Copy</button><button class="btn xs" onclick={() => download(secretName + '-secret.yaml', yamlResult)}>💾 Download</button></div>
            <pre class="yaml-block">{yamlResult}</pre>
          </div>
        {/if}
      {:else}
        <div class="field">
          <label>Secret YAML <button class="btn xs upload-inline" onclick={() => document.getElementById('yaml-file').click()}>📂 Upload .yaml</button></label>
          <input id="yaml-file" type="file" accept=".yaml,.yml" onchange={loadYamlFile} style="display:none" />
          <textarea bind:value={decodeInput} rows="12" placeholder="Paste Secret YAML here"></textarea>
        </div>
        <button class="btn primary" onclick={doDecode}>Decode</button>
        {#if secretError}<div class="msg error">{secretError}</div>{/if}
        {#if decodedData}
          <div class="result-area">
            <button class="btn xs" onclick={() => copy(entries(decodedData).map(([k,v]) => k+'='+v).join('\n'))}>📋 Copy All</button>
            <div class="decode-table">
              {#each entries(decodedData) as [k, v]}
                <div class="decode-row"><span class="decode-key">{k}</span><span class="decode-val">{v}</span></div>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  {:else}
    {#if !kcActive}
      <div class="card">
        <h2 class="section-title">Configure Kubeconfig</h2>

        {#if entries(kcSaved).length > 0}
          <div class="field">
            <div class="field-label">Saved Configs</div>
            {#each entries(kcSaved) as [name, _]}
              <div class="saved-row">
                <span class="saved-name">{name}</span>
                <div class="saved-actions">
                  <button class="btn xs" onclick={() => activateConfig(name)}>Connect</button>
                  <button class="btn xs" onclick={() => deleteConfig(name)}>Delete</button>
                </div>
              </div>
            {/each}
          </div>
          <div class="divider"><span>or add a new one</span></div>
        {/if}

        <div class="field">
          <label for="new-name">Name (optional, defaults to current context)</label>
          <input id="new-name" type="text" bind:value={newName} placeholder="my-cluster" />
        </div>
        <div class="field">
          <label>Kubeconfig Content <button class="btn xs upload-inline" onclick={() => document.getElementById('kc-file2').click()}>📂 Upload</button></label>
          <input id="kc-file2" type="file" accept=".yaml,.yml,.kubeconfig" onchange={loadNewKcFile} style="display:none" />
          <textarea bind:value={newKc} rows="8" placeholder="Paste your kubeconfig YAML here"></textarea>
        </div>
        <button class="btn primary" onclick={uploadKubeconfig} disabled={!newKc.trim()}>Connect</button>
        {#if resError}<div class="msg error">{resError}</div>{/if}
      </div>
    {:else}
      <div class="card">
        <div class="conn-bar">
          <span class="conn-dot"></span>
          <select class="conn-ctx" bind:value={currentCtx} onchange={switchContext}>
            {#each kcConfig.contexts || [] as ctx}
              <option value={ctx}>{ctx}</option>
            {/each}
          </select>
          <span class="spacer"></span>
          <button class="btn xs" onclick={disconnect}>Disconnect</button>
        </div>

        <div class="field-row">
          <div class="field">
            <label for="res-type">Resource</label>
            <select id="res-type" bind:value={resType}>
              <option value="pods">Pods</option>
              <option value="nodes">Nodes</option>
              <option value="deployments">Deployments</option>
              <option value="services">Services</option>
              <option value="configmaps">ConfigMaps</option>
              <option value="namespaces">Namespaces</option>
              <option value="statefulsets">StatefulSets</option>
              <option value="daemonsets">DaemonSets</option>
              <option value="ingresses">Ingresses</option>
              <option value="secrets">Secrets</option>
            </select>
          </div>
          <div class="field">
            <label for="res-ns">Namespace</label>
            <select id="res-ns" bind:value={resNs} disabled={!resourceHasNs}>
              <option value="">All namespaces</option>
              {#each namespaces as ns}
                <option value={ns}>{ns}</option>
              {/each}
            </select>
          </div>
          <div class="field" style="flex:0">
            <div style="height:1.5em"></div>
            <button class="btn" onclick={doQuery} disabled={resLoading}>{resLoading ? 'Querying...' : 'Query'}</button>
          </div>
        </div>

        {#if resError}<div class="msg error">{resError}</div>{/if}
        {#if resCols.length > 0}
          <div class="res-table-wrap">
            <table class="res-table">
              <thead><tr>{#each resCols as col}<th>{col}</th>{/each}</tr></thead>
              <tbody>
                {#each resRows as row}
                  <tr>
                    {#each row as cell, i}
                      {#if (isNamespaced && i === 1) || (!isNamespaced && i === 0)}
                        <td><button class="name-link" onclick={() => doDescribe(cell, isNamespaced ? row[0] : '')}>{cell}</button></td>
                      {:else}
                        <td>{cell}</td>
                      {/if}
                    {/each}
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

{#if descText || descLoading}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="modal-overlay" role="presentation" onclick={() => { descText = ''; descLoading = false }}>
    <!-- svelte-ignore a11y_click_events_have_key_events,a11y_no_static_element_interactions -->
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <div class="modal-header">
        <h3>{descTitle}</h3>
        <button class="btn xs" onclick={() => { descText = ''; descLoading = false }}>✕</button>
      </div>
      <div class="modal-body">
        {#if descLoading}
          <p>Loading...</p>
        {:else}
          <pre class="desc-text">{descText}</pre>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .app { max-width: 960px; margin: 0 auto; padding: 40px 16px; }
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
  .field select:disabled { opacity: .4; cursor: not-allowed; }
  .field input:focus, .field select:focus, .field textarea:focus { border-color: var(--primary); }
  .field textarea { font-family: 'SF Mono', 'Fira Code', monospace; resize: vertical; }
  .field-row { display: flex; gap: 12px; align-items: flex-end; }
  .field-row .field { flex: 1; }

  .mode-switch { display: flex; gap: 0; margin-bottom: 16px; border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
  .mode-btn { flex: 1; padding: 8px; border: none; background: var(--surface); color: var(--text2); cursor: pointer; font-size: 14px; }
  .mode-btn.active { background: var(--primary); color: #fff; }
  .mode-btn:hover:not(.active) { background: var(--surface2); }

  .upload-inline { float: right; }
  .kv-table { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
  .kv-row { display: flex; gap: 6px; align-items: center; }
  .kv-row input { flex: 1; padding: 8px 10px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-size: 13px; }

  .btn { display: inline-block; padding: 10px 24px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); cursor: pointer; font-size: 14px; }
  .btn:hover { background: var(--surface2); }
  .btn.primary { background: var(--primary); border-color: var(--primary); color: #fff; width: 100%; text-align: center; }
  .btn.primary:hover { opacity: .85; }
  .btn:disabled { opacity: .5; cursor: not-allowed; }
  .btn.xs { font-size: 11px; padding: 2px 8px; display: inline-flex; align-items: center; gap: 4px; }

  .msg.error { background: #3d1f2a; border: 1px solid #e94560; color: #e94560; padding: 10px 14px; border-radius: 6px; margin-bottom: 10px; font-size: 14px; }
  .result-area { margin-top: 14px; }
  .result-actions { display: flex; gap: 6px; margin-bottom: 6px; }
  .yaml-block { background: var(--bg); padding: 14px; border-radius: 8px; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; white-space: pre-wrap; word-break: break-all; line-height: 1.5; margin: 0; }
  .decode-table { display: flex; flex-direction: column; gap: 4px; margin-top: 8px; }
  .decode-row { display: flex; gap: 8px; align-items: center; background: var(--bg); padding: 8px 12px; border-radius: 6px; }
  .decode-key { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; font-weight: 600; color: var(--primary); min-width: 140px; }
  .decode-val { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; color: var(--text); word-break: break-all; }

  .section-title { font-size: 18px; margin-bottom: 8px; }

  .saved-row { display: flex; align-items: center; justify-content: space-between; padding: 10px 12px; background: var(--bg); border-radius: 6px; margin-bottom: 6px; }
  .saved-name { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; font-weight: 600; }
  .saved-actions { display: flex; gap: 6px; }

  .divider { display: flex; align-items: center; gap: 12px; margin: 16px 0; color: var(--text2); font-size: 12px; }
  .divider::before, .divider::after { content: ''; flex: 1; border-top: 1px solid var(--border); }

  .conn-bar { display: flex; align-items: center; gap: 8px; padding: 10px 14px; background: var(--bg); border-radius: 8px; margin-bottom: 16px; }
  .conn-dot { width: 10px; height: 10px; border-radius: 50%; background: #4caf50; flex-shrink: 0; }
  .conn-ctx { font-size: 12px; padding: 2px 6px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); max-width: 300px; }
  .spacer { flex: 1; }

  .res-table-wrap { overflow-x: auto; margin-top: 12px; border: 1px solid var(--border); border-radius: 8px; }
  .res-table { width: 100%; border-collapse: collapse; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; }
  .res-table th { text-align: left; padding: 10px 12px; background: var(--surface); color: var(--text2); font-size: 11px; text-transform: uppercase; letter-spacing: .5px; border-bottom: 1px solid var(--border); white-space: nowrap; }
  .res-table td { padding: 8px 12px; border-bottom: 1px solid var(--border); color: var(--text); white-space: nowrap; }
  .res-table tr:last-child td { border-bottom: none; }

  .name-link { background: none; border: none; color: var(--primary); cursor: pointer; font-family: inherit; font-size: inherit; padding: 0; text-decoration: underline; text-underline-offset: 2px; }
  .name-link:hover { color: var(--text); }

  .modal-overlay { position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 200; }
  .modal { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; width: 700px; max-width: 90vw; max-height: 80vh; display: flex; flex-direction: column; }
  .modal-header { display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-bottom: 1px solid var(--border); flex-shrink: 0; }
  .modal-header h3 { font-size: 16px; }
  .modal-body { padding: 20px; overflow-y: auto; flex: 1; }
  .desc-text { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-all; margin: 0; }
</style>
