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

<div class="app">
  {#if inGateway}
    <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
  {/if}
  <h1>JAR Analyzer</h1>

  {#if error}
    <div class="msg error">{error}</div>
  {/if}

  <div class="card">
    <div class="field">
      <label for="file-input">Select a JAR file</label>
      <input id="file-input" type="file" accept=".jar" onchange={onFileSelect} />
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
          <span class="value">{fmtSize(info.compressedSize)} &rarr; {fmtSize(info.uncompressedSize)}
            ({info.uncompressedSize ? (info.compressedSize * 100 / info.uncompressedSize).toFixed(0) : 0}%)
          </span>
        </div>
      </div>

      {#if info.manifest}
        <h2>Manifest</h2>
        <table>
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
        <h2>Maven
          <button class="btn xs" onclick={copyMaven} title="Copy coordinates">Copy</button>
        </h2>
        <div class="maven">{info.maven.groupId}:{info.maven.artifactId}:{info.maven.version}</div>
      {/if}

      <div class="tags">
        <span class="tag" class:tag-green={info.signed} class:tag-gray={!info.signed}>
          {info.signed ? 'Signed' : 'Unsigned'}
        </span>
        {#if info.versionedClasses && entries(info.versionedClasses).length}
          <span class="tag tag-blue">Multi-release</span>
        {/if}
      </div>

      {#if info.versionedClasses && entries(info.versionedClasses).length}
        <h2>Multi-release</h2>
        <table>
          <tbody>
            {#each entries(info.versionedClasses) as [v, c]}
              <tr><td>JDK {v}</td><td>{c} classes</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}

      {#if info.versionHistogram}
        <h2>Version breakdown</h2>
        <table>
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
  .app { max-width: 640px; margin: 0 auto; padding: 40px 16px; }
  h1 { font-size: 24px; margin-bottom: 24px; }
  .home-link { float: left; }

  .card { margin-bottom: 16px; }
  .field { margin-bottom: 16px; }
  .field label { display: block; font-size: 13px; color: var(--text2); margin-bottom: 6px; font-weight: 500; }
  .field input { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; outline: none; }
  .field input:focus { border-color: var(--primary); }

  .btn.primary { width: 100%; text-align: center; }

  .result h2 { font-size: 16px; margin: 20px 0 10px; display: flex; align-items: center; gap: 8px; }
  .result h2:first-of-type { margin-top: 0; }

  .meta { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .meta-item { background: var(--bg); padding: 12px; border-radius: 8px; }
  .meta-item .label { display: block; font-size: 11px; text-transform: uppercase; letter-spacing: .5px; color: var(--text2); margin-bottom: 4px; }
  .meta-item .value { font-size: 16px; font-weight: 600; }

  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  td, th { padding: 6px 4px; border-bottom: 1px solid var(--border); text-align: left; }
  th { font-size: 12px; text-transform: uppercase; color: var(--text2); letter-spacing: .5px; }
  td:last-child, th:last-child { text-align: right; }

  .maven { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; background: var(--bg); padding: 10px; border-radius: 6px; }

  .tags { display: flex; gap: 8px; margin: 16px 0; }
  .tag { display: inline-block; padding: 3px 10px; border-radius: 20px; font-size: 12px; font-weight: 600; }
  .tag-green { background: #1a3d2a; color: #4caf50; border: 1px solid #4caf50; }
  .tag-blue { background: #1a2a3d; color: #61affe; border: 1px solid #61affe; }
  .tag-gray { background: #2a2a3a; color: #888; border: 1px solid #555; }
</style>
