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
  let resMetrics = $state(false)
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
      const r = await fetch('/api/k8s/resources', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ type: resType, namespace: resNs, metrics: resMetrics }) })
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

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">k8s</span>
  </a>
</header>

<div class="app">
  <div class="page-head">
    <h1>K8s</h1>
    <p>Secret YAML encode and decode</p>
  </div>

  <div class="tabs">
    <button class="tab" class:active={tab === 'secret'} onclick={() => tab = 'secret'}>Secret</button>
    <button class="tab" class:active={tab === 'resources'} onclick={() => tab = 'resources'}>Resources</button>
  </div>

  {#if tab === 'secret'}
    <div class="card">
      <div class="segment">
        <button class="segment-btn" class:active={secretMode === 'encode'} onclick={() => secretMode = 'encode'}>Encode</button>
        <button class="segment-btn" class:active={secretMode === 'decode'} onclick={() => secretMode = 'decode'}>Decode</button>
      </div>

      {#if secretMode === 'encode'}
        <div class="field">
          <label for="secret-name">Secret name</label>
          <input id="secret-name" type="text" bind:value={secretName} placeholder="my-app" />
        </div>
        <div class="field">
          <div class="label-row">
            <span class="field-label">Key-value pairs</span>
            <button class="btn xs" onclick={() => document.getElementById('env-file').click()}>Load .env</button>
          </div>
          <input id="env-file" type="file" accept=".env,.txt" onchange={loadEnvFile} style="display:none" />
          <div class="kv-list">
            {#each secretRows as r, i (i)}
              <div class="kv-row">
                <input type="text" bind:value={r.key} placeholder="Key" />
                <input type="text" bind:value={r.value} placeholder="Value" />
                <button class="btn xs icon-btn" onclick={() => removeRow(i)} disabled={secretRows.length === 1} aria-label="Remove row">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>
                </button>
              </div>
            {/each}
            <button class="btn xs add-row" onclick={addRow}>Add row</button>
          </div>
        </div>
        <button class="btn primary" onclick={doEncode} disabled={secretLoading}>{secretLoading ? 'Generating...' : 'Generate YAML'}</button>
        {#if secretError}<div class="msg error block">{secretError}</div>{/if}
        {#if yamlResult}
          <div class="result">
            <div class="result-actions">
              <button class="btn xs" onclick={() => copy(yamlResult)}>Copy</button>
              <button class="btn xs" onclick={() => download(secretName + '-secret.yaml', yamlResult)}>Download</button>
            </div>
            <pre class="code-block">{yamlResult}</pre>
          </div>
        {/if}
      {:else}
        <div class="field">
          <div class="label-row">
            <span class="field-label">Secret YAML</span>
            <button class="btn xs" onclick={() => document.getElementById('yaml-file').click()}>Upload .yaml</button>
          </div>
          <input id="yaml-file" type="file" accept=".yaml,.yml" onchange={loadYamlFile} style="display:none" />
          <textarea bind:value={decodeInput} rows="12" placeholder="Paste Secret YAML here"></textarea>
        </div>
        <button class="btn primary" onclick={doDecode}>Decode</button>
        {#if secretError}<div class="msg error block">{secretError}</div>{/if}
        {#if decodedData}
          <div class="result">
            <button class="btn xs" onclick={() => copy(entries(decodedData).map(([k,v]) => k+'='+v).join('\n'))}>Copy all</button>
            <div class="decode-list">
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
        <h2>Configure kubeconfig</h2>

        {#if entries(kcSaved).length > 0}
          <div class="field">
            <div class="field-label">Saved configs</div>
            <div class="saved-list">
              {#each entries(kcSaved) as [name, _]}
                <div class="saved-row">
                  <span class="saved-name">{name}</span>
                  <div class="saved-actions">
                    <button class="btn xs" onclick={() => activateConfig(name)}>Connect</button>
                    <button class="btn xs danger" onclick={() => deleteConfig(name)}>Delete</button>
                  </div>
                </div>
              {/each}
            </div>
          </div>
          <div class="divider">or add a new one</div>
        {/if}

        <div class="field">
          <label for="new-name">Name (optional, defaults to current context)</label>
          <input id="new-name" type="text" bind:value={newName} placeholder="my-cluster" />
        </div>
        <div class="field">
          <div class="label-row">
            <span class="field-label">Kubeconfig content</span>
            <button class="btn xs" onclick={() => document.getElementById('kc-file2').click()}>Upload</button>
          </div>
          <input id="kc-file2" type="file" accept=".yaml,.yml,.kubeconfig" onchange={loadNewKcFile} style="display:none" />
          <textarea bind:value={newKc} rows="8" placeholder="Paste your kubeconfig YAML here"></textarea>
        </div>
        <button class="btn primary" onclick={uploadKubeconfig} disabled={!newKc.trim()}>Connect</button>
        {#if resError}<div class="msg error block">{resError}</div>{/if}
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
          <span class="flex1"></span>
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
          <button class="btn" onclick={doQuery} disabled={resLoading}>{resLoading ? 'Querying...' : 'Query'}</button>
        </div>

        <label class="check metrics-toggle">
          <input type="checkbox" bind:checked={resMetrics} />
          <span>Show metrics</span>
        </label>

        {#if resError}<div class="msg error block">{resError}</div>{/if}
        {#if resCols.length > 0}
          <div class="table-wrap">
            <table class="table res-table">
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
        <button class="btn xs icon-btn" onclick={() => { descText = ''; descLoading = false }} aria-label="Close">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>
        </button>
      </div>
      <div class="modal-body">
        {#if descLoading}
          <p class="muted">Loading...</p>
        {:else}
          <pre class="desc-text">{descText}</pre>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  :global(body) { --page-max: 960px; }
  .block { margin: 12px 0; }
  .flex1 { flex: 1; }

  .segment { display: inline-flex; padding: 2px; margin-bottom: 20px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); }
  .segment-btn { height: 28px; padding: 0 12px; border: 0; border-radius: var(--r-sm); background: transparent; color: var(--text2); font-family: var(--sans); font-size: 13px; font-weight: 500; cursor: pointer; }
  .segment-btn:hover { color: var(--text); }
  .segment-btn.active { background: var(--surface2); color: var(--text); }

  .label-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 6px; }
  .field-label { color: var(--text2); font-size: 12px; font-weight: 500; }

  .kv-list { display: flex; flex-direction: column; gap: 6px; }
  .kv-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) auto; gap: 6px; align-items: center; }
  .kv-row input { min-height: 32px; padding: 6px 10px; font-family: var(--mono); font-size: 13px; }
  .icon-btn { width: 26px; padding: 0; }
  .add-row { align-self: flex-start; margin-top: 2px; }

  .result { margin-top: 16px; }
  .result-actions { display: flex; gap: 8px; margin-bottom: 8px; }
  .code-block { margin: 0; padding: 12px 14px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); font-size: 13px; line-height: 1.55; white-space: pre-wrap; word-break: break-all; }

  .decode-list { display: flex; flex-direction: column; gap: 1px; margin-top: 8px; border: 1px solid var(--border); border-radius: var(--r-md); overflow: hidden; background: var(--border); }
  .decode-row { display: flex; gap: 12px; padding: 8px 12px; background: var(--bg); }
  .decode-key { min-width: 140px; color: var(--text2); font-family: var(--mono); font-size: 13px; }
  .decode-val { color: var(--text); font-family: var(--mono); font-size: 13px; word-break: break-all; }

  .saved-list { display: flex; flex-direction: column; gap: 6px; }
  .saved-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 12px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); }
  .saved-name { color: var(--text); font-family: var(--mono); font-size: 13px; }
  .saved-actions { display: flex; gap: 6px; }

  .divider { display: flex; align-items: center; gap: 12px; margin: 20px 0; color: var(--text3); font-size: 12px; }
  .divider::before, .divider::after { content: ''; flex: 1; border-top: 1px solid var(--border); }

  .metrics-toggle { margin: 16px 0; color: var(--text2); }
  .conn-bar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; margin-bottom: 16px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); }
  .conn-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--success); flex: none; }
  .conn-ctx { width: auto; max-width: 320px; min-height: 28px; padding: 2px 8px; font-family: var(--mono); font-size: 12px; }

  .table-wrap { margin-top: 12px; border: 1px solid var(--border); border-radius: var(--r-md); overflow: auto; }
  .res-table { font-family: var(--mono); font-size: 12.5px; }
  .res-table th { font-family: var(--mono); font-size: 11px; }
  .res-table td { padding: 8px 12px; white-space: nowrap; }
  .name-link { padding: 0; border: 0; background: none; color: var(--text); font-family: inherit; font-size: inherit; text-decoration: underline; text-decoration-color: var(--border2); text-underline-offset: 3px; cursor: pointer; }
  .name-link:hover { color: var(--accent); text-decoration-color: var(--accent); }

  .modal-overlay { position: fixed; inset: 0; z-index: 200; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(0, 0, 0, 0.55); }
  .modal { display: flex; flex-direction: column; width: 720px; max-width: 100%; max-height: 80vh; border: 1px solid var(--border2); border-radius: var(--r-lg); background: var(--surface); }
  .modal-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 16px 12px 20px; border-bottom: 1px solid var(--border); }
  .modal-body { flex: 1; padding: 20px; overflow-y: auto; }
  .desc-text { margin: 0; font-family: var(--mono); font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-all; }
</style>
