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

<div class="app">
  {#if inGateway}
    <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
  {/if}
  <h1>QR Code Generator</h1>

  {#if error}
    <div class="msg error">{error}</div>
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
      <div class="qr-frame">
        <img src={qrUrl} alt="QR Code" />
      </div>
      <a href={qrUrl} download="qrcode.png" class="btn">Download PNG</a>
    </div>
  {/if}
</div>

<style>
  .app { max-width: 640px; margin: 0 auto; padding: 40px 16px; text-align: center; }
  h1 { font-size: 24px; margin-bottom: 24px; }
  .home-link { float: left; }

  .card { text-align: left; }
  .field { margin-bottom: 16px; }
  .field label { display: block; font-size: 13px; color: var(--text2); margin-bottom: 6px; font-weight: 500; }
  .field textarea { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; font-family: inherit; outline: none; resize: vertical; }
  .field textarea:focus { border-color: var(--primary); }
  .field select { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; outline: none; }
  .field select:focus { border-color: var(--primary); }

  .btn { text-decoration: none; }
  .btn.primary { width: 100%; text-align: center; }

  .qr-section { margin-top: 24px; }
  .qr-frame { display: inline-block; background: #fff; padding: 16px; border-radius: 12px; margin-bottom: 16px; }
  .qr-frame img { display: block; width: 256px; height: 256px; image-rendering: pixelated; }
</style>
