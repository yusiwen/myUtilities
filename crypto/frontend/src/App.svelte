<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let tab = $state('passwd')

  // Password generator
  let pwLength = $state(32)
  let password = $state('')
  let pwError = $state('')

  // Cipher
  let cipher = $state('aes')
  let mode = $state('ecb')
  let op = $state('encrypt')
  let key = $state('')
  let iv = $state('')
  let input = $state('')
  let inputHex = $state(false)
  let outputHex = $state(true)
  let result = $state('')
  let cipherError = $state('')
  let loading = $state(false)

  async function genPasswd() {
    pwError = ''
    password = ''
    try {
      const r = await fetch('/api/crypto/passwd', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ length: pwLength }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      const d = await r.json()
      password = d.password
    } catch (e) {
      pwError = e.message
    }
  }

  async function doCipher() {
    if (!key) { cipherError = 'Key is required'; return }
    if (!input) { cipherError = 'Input is required'; return }
    if (mode === 'cbc' && !iv) { cipherError = 'IV is required for CBC mode'; return }
    cipherError = ''
    result = ''
    loading = true
    try {
      const r = await fetch('/api/crypto/cipher', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ cipher, mode, op, key, iv, input, inputHex, outputHex }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      const d = await r.json()
      result = d.result
    } catch (e) {
      cipherError = e.message
    } finally {
      loading = false
    }
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text) } catch {}
  }

  const ciphers = [
    { value: 'aes', label: 'AES' },
    { value: 'des', label: 'DES' },
    { value: '3des', label: '3DES' },
    { value: 'sm4', label: 'SM4' },
  ]
</script>

<div class="app">
  {#if inGateway}
    <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
  {/if}
  <h1>Crypto Toolkit</h1>

  <div class="tabs">
    <button class="tab" class:active={tab === 'passwd'} onclick={() => tab = 'passwd'}>Password Generator</button>
    <button class="tab" class:active={tab === 'cipher'} onclick={() => tab = 'cipher'}>Encrypt / Decrypt</button>
  </div>

  {#if tab === 'passwd'}
    <div class="card">
      <div class="field">
        <label for="pw-length">Length</label>
        <input id="pw-length" type="number" bind:value={pwLength} min="8" max="128" />
      </div>
      <button class="btn primary" onclick={genPasswd}>Generate</button>

      {#if pwError}
        <div class="msg error">{pwError}</div>
      {/if}
      {#if password}
        <div class="result-box">
          <code class="result-text">{password}</code>
          <button class="btn xs" onclick={() => copy(password)}>Copy</button>
        </div>
      {/if}
    </div>
  {:else}
    <div class="card">
      <div class="field-row">
        <div class="field">
          <label for="cipher-select">Cipher</label>
          <select id="cipher-select" bind:value={cipher}>
            {#each ciphers as c}
              <option value={c.value}>{c.label}</option>
            {/each}
          </select>
        </div>
        <div class="field">
          <label for="mode-select">Mode</label>
          <select id="mode-select" bind:value={mode}>
            <option value="ecb">ECB</option>
            <option value="cbc">CBC</option>
          </select>
        </div>
      </div>

      <div class="field">
        <div class="field-label">Operation</div>
        <div class="radio-group">
          <label><input type="radio" bind:group={op} value="encrypt" /> Encrypt</label>
          <label><input type="radio" bind:group={op} value="decrypt" /> Decrypt</label>
        </div>
      </div>

      <div class="field">
        <label for="cipher-key">Key</label>
        <input id="cipher-key" type="text" bind:value={key} placeholder="Secret key" />
      </div>

      {#if mode === 'cbc'}
        <div class="field">
          <label for="cipher-iv">IV</label>
          <input id="cipher-iv" type="text" bind:value={iv} placeholder="Initialization vector (CBC)" />
        </div>
      {/if}

      <div class="field-row">
        <div class="field">
          <label for="input-format">Input format</label>
          <select id="input-format" bind:value={inputHex}>
            <option value={false}>Raw text</option>
            <option value={true}>Hex</option>
          </select>
        </div>
        <div class="field">
          <label for="output-format">Output format</label>
          <select id="output-format" bind:value={outputHex}>
            <option value={true}>Hex</option>
            <option value={false}>Raw text</option>
          </select>
        </div>
      </div>

      <div class="field">
        <label for="cipher-input">Input</label>
        <textarea id="cipher-input" bind:value={input} rows="4" placeholder="Text to encrypt or decrypt"></textarea>
      </div>

      <button class="btn primary" onclick={doCipher} disabled={loading}>
        {loading ? 'Processing...' : (op === 'encrypt' ? 'Encrypt' : 'Decrypt')}
      </button>

      {#if cipherError}
        <div class="msg error">{cipherError}</div>
      {/if}
      {#if result}
        <div class="result-box">
          <code class="result-text result-mono">{result}</code>
          <button class="btn xs" onclick={() => copy(result)}>Copy</button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .app { max-width: 600px; margin: 0 auto; padding: 40px 16px; }
  h1 { font-size: 24px; margin-bottom: 16px; }
  .home-link { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px 3px 6px; border: 1px solid var(--border); border-radius: 20px; background: var(--surface); color: var(--text2); text-decoration: none; font-size: 12px; margin-right: 10px; float: left; }
  .home-link:hover { border-color: var(--primary); color: var(--text); }

  .tabs { display: flex; gap: 0; margin-bottom: 16px; border-bottom: 1px solid var(--border); }
  .tab { padding: 10px 20px; border: none; background: none; color: var(--text2); cursor: pointer; font-size: 14px; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .tab.active { color: var(--text); border-bottom-color: var(--primary); }
  .tab:hover { color: var(--text); }

  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; }
  .field { margin-bottom: 14px; }
  .field label, .field-label { display: block; font-size: 13px; color: var(--text2); margin-bottom: 4px; font-weight: 500; }
  .field input, .field select, .field textarea { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; font-family: inherit; outline: none; }
  .field input:focus, .field select:focus, .field textarea:focus { border-color: var(--primary); }
  .field textarea { font-family: 'SF Mono', 'Fira Code', monospace; resize: vertical; }
  select option { background: var(--bg); color: var(--text); }
  .field-row { display: flex; gap: 12px; }
  .field-row .field { flex: 1; }

  .radio-group { display: flex; gap: 16px; }
  .radio-group label { display: inline-flex; align-items: center; gap: 6px; font-size: 14px; color: var(--text); cursor: pointer; }
  .radio-group input { width: auto; }

  .btn { display: inline-block; padding: 10px 24px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); cursor: pointer; font-size: 14px; }
  .btn:hover { background: var(--surface2); }
  .btn.primary { background: var(--primary); border-color: var(--primary); color: #fff; width: 100%; text-align: center; }
  .btn.primary:hover { opacity: .85; }
  .btn:disabled { opacity: .5; cursor: not-allowed; }
  .btn.xs { font-size: 11px; padding: 2px 8px; flex-shrink: 0; }

  .msg { padding: 10px 14px; border-radius: 6px; margin-bottom: 10px; font-size: 14px; }
  .msg.error { background: #3d1f2a; border: 1px solid #e94560; color: #e94560; }

  .result-box { display: flex; align-items: center; gap: 8px; margin-top: 14px; background: var(--bg); padding: 12px; border-radius: 8px; }
  .result-text { flex: 1; font-size: 14px; word-break: break-all; }
  .result-mono { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; }
</style>
