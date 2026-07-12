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

  function handleKeydown(e) {
    if (e.key === 'Enter') generate()
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
      <input id="text-input" type="text" bind:value={text} onkeydown={handleKeydown} placeholder="https://example.com or any text" />
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
  .app { max-width: 500px; margin: 0 auto; padding: 40px 16px; text-align: center; }
  h1 { font-size: 24px; margin-bottom: 24px; }
  .home-link { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px 3px 6px; border: 1px solid var(--border); border-radius: 20px; background: var(--surface); color: var(--text2); text-decoration: none; font-size: 12px; margin-right: 10px; float: left; }
  .home-link:hover { border-color: var(--primary); color: var(--text); }

  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; text-align: left; }
  .field { margin-bottom: 16px; }
  .field label { display: block; font-size: 13px; color: var(--text2); margin-bottom: 6px; font-weight: 500; }
  .field input, .field select { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; outline: none; }
  .field input:focus, .field select:focus { border-color: var(--primary); }

  .btn { display: inline-block; padding: 10px 24px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); cursor: pointer; font-size: 14px; text-decoration: none; }
  .btn:hover { background: var(--surface2); }
  .btn.primary { background: var(--primary); border-color: var(--primary); color: #fff; width: 100%; text-align: center; }
  .btn.primary:hover { opacity: .85; }
  .btn:disabled { opacity: .5; cursor: not-allowed; }

  .msg { padding: 10px 14px; border-radius: 6px; margin-bottom: 16px; font-size: 14px; }
  .msg.error { background: #3d1f2a; border: 1px solid #e94560; color: #e94560; }

  .qr-section { margin-top: 24px; }
  .qr-frame { display: inline-block; background: #fff; padding: 16px; border-radius: 12px; margin-bottom: 16px; }
  .qr-frame img { display: block; width: 256px; height: 256px; image-rendering: pixelated; }
</style>
