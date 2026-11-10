<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let text = $state('')
  let level = $state('medium')
  let qrUrl = $state('')
  let error = $state('')
  let loading = $state(false)

  async function generate() {
    if (!text.trim()) {
      error = 'Please enter text to encode'
      return
    }
    error = ''
    loading = true
    qrUrl = `/api/qrcode?text=${encodeURIComponent(text.trim())}&level=${level}`
    loading = false
  }

</script>

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">qrcode</span>
  </a>
</header>

<div class="app">
  <div class="page-head">
    <h1>QR Code</h1>
    <p>Text to QR code</p>
  </div>

  {#if error}
    <div class="msg error block">{error}</div>
  {/if}

  <div class="card">
    <div class="field">
      <label for="text-input">Text to encode</label>
      <textarea id="text-input" bind:value={text} rows="4" placeholder="https://example.com or any text"></textarea>
    </div>

    <div class="field">
      <label for="level-select">Error correction</label>
      <select id="level-select" bind:value={level}>
        <option value="low">Low (7%)</option>
        <option value="medium">Medium (15%)</option>
        <option value="high">High (25%)</option>
      </select>
    </div>

    <button class="btn primary" onclick={generate} disabled={loading}>
      {loading ? 'Generating...' : 'Generate'}
    </button>
  </div>

  {#if qrUrl}
    <div class="qr-section">
      <div class="qr-frame"><img src={qrUrl} alt="QR code" /></div>
      <a href={qrUrl} download="qrcode.png" class="btn">Download PNG</a>
    </div>
  {/if}
</div>

<style>
  :global(body) { --page-max: 640px; }
  .block { margin-bottom: 16px; }

  .qr-section { display: flex; align-items: flex-end; gap: 16px; margin-top: 24px; }
  .qr-frame { padding: 16px; border: 1px solid var(--border); border-radius: var(--r-lg); background: #fff; }
  .qr-frame img { display: block; width: 224px; height: 224px; image-rendering: pixelated; }

  @media (max-width: 700px) {
    .qr-section { flex-direction: column; align-items: flex-start; }
  }
</style>
