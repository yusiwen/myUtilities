<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let selectedFile = $state(null)
  let loading = $state(false)
  let error = $state('')
  let info = $state(null)

  function onFileSelect(e) {
    selectedFile = e.target.files[0] || null
    error = ''
    info = null
  }

  async function analyze() {
    if (!selectedFile) {
      error = 'Please select a .jar file'
      return
    }
    if (!selectedFile.name.endsWith('.jar')) {
      error = 'Only .jar files are accepted'
      return
    }
    error = ''
    loading = true
    info = null

    const fd = new FormData()
    fd.append('file', selectedFile)

    try {
      const res = await fetch('/api/jarinfo/analyze', { method: 'POST', body: fd })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || 'Analysis failed')
      }
      info = await res.json()
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function fmtSize(bytes) {
    if (!bytes) return '0 B'
    const u = ['B', 'KB', 'MB', 'GB']
    let i = 0
    let n = bytes
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
    return n.toFixed(i === 0 ? 0 : 1) + ' ' + u[i]
  }

  function entries(o) { return Object.entries(o || {}) }

  function copyMaven() {
    if (!info?.maven) return
    const s = `${info.maven.groupId}:${info.maven.artifactId}:${info.maven.version}`
    navigator.clipboard.writeText(s)
  }
</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">jarinfo</span>
  </a>
</header>

<div class="app">
  <div class="page-head">
    <h1>JAR Analyzer</h1>
    <p>JAR structure and metadata</p>
  </div>

  {#if error}
    <div class="msg error block">{error}</div>
  {/if}

  <div class="card">
    <div class="field">
      <span class="field-label">JAR file</span>
      <div class="file-row">
        <label class="btn">
          Choose file
          <input type="file" accept=".jar" onchange={onFileSelect} />
        </label>
        <span class="file-name">{selectedFile ? selectedFile.name : 'No file selected'}</span>
      </div>
    </div>
    <button class="btn primary" onclick={analyze} disabled={loading || !selectedFile}>
      {loading ? 'Analyzing...' : 'Analyze'}
    </button>
  </div>

  {#if info}
    <div class="card result">
      <div class="meta">
        <div class="meta-item"><span class="label">Target JDK</span><span class="value">{info.minJDKVersion}</span></div>
        <div class="meta-item"><span class="label">Classes</span><span class="value">{info.classCount}</span></div>
        <div class="meta-item"><span class="label">Total entries</span><span class="value">{info.totalEntries}</span></div>
        <div class="meta-item">
          <span class="label">Compressed</span>
          <span class="value">{fmtSize(info.compressedSize)} &rarr; {fmtSize(info.uncompressedSize)} ({info.uncompressedSize ? (info.compressedSize * 100 / info.uncompressedSize).toFixed(0) : 0}%)</span>
        </div>
      </div>

      {#if info.manifest}
        <h2>Manifest</h2>
        <table class="table">
          <tbody>
            <tr><td>Main-Class</td><td>{info.manifest.mainClass || '-'}</td></tr>
            <tr><td>Created-By</td><td>{info.manifest.createdBy || '-'}</td></tr>
            <tr><td>Build-Jdk</td><td>{info.manifest.buildJDK || '-'}</td></tr>
            <tr><td>Implementation-Version</td><td>{info.manifest.implVersion || '-'}</td></tr>
            <tr><td>Automatic-Module-Name</td><td>{info.manifest.automaticModuleName || '-'}</td></tr>
          </tbody>
        </table>
      {/if}

      {#if info.maven}
        <h2>Maven <button class="btn xs" onclick={copyMaven} title="Copy coordinates">Copy</button></h2>
        <div class="maven">{info.maven.groupId}:{info.maven.artifactId}:{info.maven.version}</div>
      {/if}

      <div class="tags">
        <span class="badge" class:ok={info.signed} class:err={!info.signed}>{info.signed ? 'Signed' : 'Unsigned'}</span>
        {#if info.versionedClasses && entries(info.versionedClasses).length}
          <span class="badge">Multi-release</span>
        {/if}
      </div>

      {#if info.versionedClasses && entries(info.versionedClasses).length}
        <h2>Multi-release</h2>
        <table class="table">
          <tbody>
            {#each entries(info.versionedClasses) as [v, c]}
              <tr><td>JDK {v}</td><td>{c} classes</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}

      {#if info.versionHistogram}
        <h2>Version breakdown</h2>
        <table class="table">
          <thead><tr><th>JDK</th><th>Major</th><th>Classes</th></tr></thead>
          <tbody>
            {#each entries(info.versionHistogram) as [major, count]}
              <tr><td>{info.minJDKVersion}</td><td>{major}</td><td>{count}</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {/if}
</div>

<style>
  :global(body) { --page-max: 640px; }
  .block { margin-bottom: 16px; }
  .field-label { display: block; margin-bottom: 6px; color: var(--text2); font-size: 12px; font-weight: 500; }
  .file-row { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
  .file-row .btn input { display: none; }
  .file-name { min-width: 0; overflow: hidden; color: var(--text3); font-family: var(--mono); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }

  .result h2 { display: flex; align-items: center; gap: 12px; margin: 24px 0 12px; }
  .result h2:first-of-type { margin-top: 24px; }

  .meta {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1px;
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    overflow: hidden;
    background: var(--border);
  }
  .meta-item { background: var(--surface); padding: 12px 14px; }
  .meta-item .label { display: block; margin-bottom: 4px; color: var(--text2); font-size: 12px; }
  .meta-item .value { color: var(--text); font-family: var(--mono); font-size: 14px; font-variant-numeric: tabular-nums; }

  .maven {
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--bg);
    color: var(--text2);
    font-family: var(--mono);
    font-size: 13px;
    word-break: break-all;
  }

  .tags { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; }
</style>
