<script>
  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let tab = $state('passwd')

  // Password generator
  let pwLength = $state(32)
  let pwDigits = $state(true)
  let pwSpecial = $state(false)
  let password = $state('')
  let pwError = $state('')

  // Copy feedback
  let copyLabel = $state('Copy')
  let copyLabelResult = $state('Copy')

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

  // Encode/Decode
  let encType = $state('base64')
  let encOp = $state('encode')
  let encInput = $state('')
  let encResult = $state('')
  let encError = $state('')
  let encLabelResult = $state('Copy')

  // JWT
  let jwtToken = $state('')
  let jwtAlg = $state('HS256')
  let jwtKey = $state('')
  let jwtKeyB64 = $state(false)
  let jwtHeader = $state('')
  let jwtPayload = $state('')
  let jwtVerified = $state(null)
  let jwtError = $state('')

  async function doJwtDecode() {
    if (!jwtToken.trim()) { jwtError = 'Token is required'; return }
    jwtError = ''
    jwtHeader = ''
    jwtPayload = ''
    jwtVerified = null
    try {
      const r = await fetch('/api/crypto/jwt/decode', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: jwtToken.trim() }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      const d = await r.json()
      jwtHeader = JSON.stringify(d.header, null, 2)
      jwtPayload = JSON.stringify(d.payload, null, 2)
      if (d.header && d.header.alg) jwtAlg = d.header.alg
    } catch (e) {
      jwtError = e.message
    }
  }

  async function doJwtVerify() {
    if (!jwtToken.trim()) { jwtError = 'Token is required'; return }
    if (!jwtKey) { jwtError = 'Secret key is required for verification'; return }
    jwtError = ''
    jwtHeader = ''
    jwtPayload = ''
    jwtVerified = null
    try {
      const r = await fetch('/api/crypto/jwt/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: jwtToken.trim(), key: jwtKey, keyB64: jwtKeyB64, alg: jwtAlg }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      const d = await r.json()
      jwtHeader = JSON.stringify(d.header, null, 2)
      jwtPayload = JSON.stringify(d.payload, null, 2)
      jwtVerified = d.valid
    } catch (e) {
      jwtError = e.message
    }
  }

  async function genPasswd() {
    pwError = ''
    password = ''
    try {
      const r = await fetch('/api/crypto/passwd', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ length: pwLength, digits: pwDigits, special: pwSpecial }),
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

  async function copy(text, setLabel) {
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
    setLabel('Copied!')
    setTimeout(() => setLabel('Copy'), 2000)
  }

  async function doEnc() {
    if (!encInput) { encError = 'Input is required'; return }
    encError = ''
    encResult = ''
    try {
      const r = await fetch(`/api/crypto/${encOp}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: encType, input: encInput }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'request failed')
      const d = await r.json()
      encResult = d.result
    } catch (e) {
      encError = e.message
    }
  }

  function switchOp(op) {
    encOp = op
    encResult = ''
    encError = ''
  }

  const ciphers = [
    { value: 'aes', label: 'AES' },
    { value: 'des', label: 'DES' },
    { value: '3des', label: '3DES' },
    { value: 'sm4', label: 'SM4' },
  ]

  const encTypes = [
    { value: 'base64', label: 'Base64' },
    { value: 'base64url', label: 'Base64 (URL-safe)' },
    { value: 'hex', label: 'Hex' },
    { value: 'url', label: 'URL' },
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
    <button class="tab" class:active={tab === 'encode'} onclick={() => tab = 'encode'}>Encode / Decode</button>
    <button class="tab" class:active={tab === 'jwt'} onclick={() => tab = 'jwt'}>JWT</button>
  </div>

  {#if tab === 'passwd'}
    <div class="card">
      <div class="field">
        <label for="pw-length">Length</label>
        <input id="pw-length" type="number" bind:value={pwLength} min="8" max="128" />
      </div>
      <div class="field-row toggles">
        <label class="toggle">
          <input type="checkbox" bind:checked={pwDigits} />
          <span class="toggle-label">Digits</span>
        </label>
        <label class="toggle">
          <input type="checkbox" bind:checked={pwSpecial} />
          <span class="toggle-label">Special chars</span>
        </label>
      </div>
      <button class="btn primary" onclick={genPasswd}>Generate</button>

      {#if pwError}
        <div class="msg error">{pwError}</div>
      {/if}
      {#if password}
        <div class="result-box">
          <code class="result-text">{password}</code>
          <button class="btn xs" onclick={() => copy(password, v => copyLabel = v)}>{copyLabel}</button>
        </div>
      {/if}
    </div>
  {:else if tab === 'cipher'}
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
          <button class="btn xs" onclick={() => copy(result, v => copyLabelResult = v)}>{copyLabelResult}</button>
        </div>
      {/if}
    </div>
  {:else}
    <div class="card">
      <div class="field">
        <label for="enc-type">Type</label>
        <select id="enc-type" bind:value={encType}>
          {#each encTypes as t}
            <option value={t.value}>{t.label}</option>
          {/each}
        </select>
      </div>

      <div class="field">
        <div class="field-label">Operation</div>
        <div class="radio-group">
          <label><input type="radio" name="enc-op" bind:group={encOp} value="encode" onchange={() => switchOp('encode')} /> Encode</label>
          <label><input type="radio" name="enc-op" bind:group={encOp} value="decode" onchange={() => switchOp('decode')} /> Decode</label>
        </div>
      </div>

      <div class="field">
        <label for="enc-input">Input</label>
        <textarea id="enc-input" bind:value={encInput} rows="4" placeholder="Text to encode or decode"></textarea>
      </div>

      <button class="btn primary" onclick={doEnc}>
        {encOp === 'encode' ? 'Encode' : 'Decode'}
      </button>

      {#if encError}
        <div class="msg error">{encError}</div>
      {/if}
      {#if encResult}
        <div class="result-box">
          <code class="result-text result-mono">{encResult}</code>
          <button class="btn xs" onclick={() => copy(encResult, v => encLabelResult = v)}>{encLabelResult}</button>
        </div>
      {/if}
    </div>
  {:else if tab === 'jwt'}
    <div class="card">
      <div class="field">
        <label for="jwt-token">JWT Token</label>
        <textarea id="jwt-token" bind:value={jwtToken} rows="5" placeholder="Paste a JWT token here"></textarea>
      </div>

      <div class="field-row">
        <button class="btn primary" onclick={doJwtDecode}>Decode</button>
        {#if jwtVerified !== null}
          <span class="jwt-badge" class:jwt-valid={jwtVerified} class:jwt-invalid={!jwtVerified}>
            {jwtVerified ? '✅ Valid signature' : '❌ Invalid signature'}
          </span>
        {/if}
      </div>

      {#if jwtError}
        <div class="msg error">{jwtError}</div>
      {/if}

      {#if jwtHeader}
        <div class="jwt-section">
          <div class="jwt-label">Header</div>
          <pre class="jwt-json">{jwtHeader}</pre>
        </div>
        <div class="jwt-section">
          <div class="jwt-label">Payload</div>
          <pre class="jwt-json">{jwtPayload}</pre>
        </div>
      {/if}

      <details class="verify-section">
        <summary>Verify signature</summary>
        <div class="field-row">
          <div class="field">
            <label for="jwt-alg">Algorithm</label>
            <select id="jwt-alg" bind:value={jwtAlg}>
              <option value="HS256">HS256</option>
              <option value="HS384">HS384</option>
              <option value="HS512">HS512</option>
            </select>
          </div>
          <div class="field" style="flex:2">
            <label for="jwt-key">Secret key</label>
            <div class="field-row" style="align-items:center">
              <input id="jwt-key" type="text" bind:value={jwtKey} placeholder="HMAC secret key" style="flex:1" />
              <label class="toggle" style="white-space:nowrap;flex-shrink:0">
                <input type="checkbox" bind:checked={jwtKeyB64} />
                <span class="toggle-label">Base64</span>
              </label>
            </div>
          </div>
        </div>
        <button class="btn primary" onclick={doJwtVerify}>Verify</button>
      </details>
    </div>
  {/if}
</div>

<style>
  .app { max-width: 600px; margin: 0 auto; padding: 40px 16px; }
  h1 { font-size: 24px; margin-bottom: 16px; }
  .home-link { float: left; }

  .tabs { display: flex; gap: 0; margin-bottom: 16px; border-bottom: 1px solid var(--border); }
  .tab { padding: 10px 20px; border: none; background: none; color: var(--text2); cursor: pointer; font-size: 14px; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .tab.active { color: var(--text); border-bottom-color: var(--primary); }
  .tab:hover { color: var(--text); }

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

  .toggles { display: flex; gap: 20px; margin-bottom: 14px; }
  .toggle { display: inline-flex; align-items: center; gap: 6px; font-size: 14px; color: var(--text); cursor: pointer; }
  .toggle input { width: auto; }

  .btn.primary { width: 100%; text-align: center; }
  .btn.xs { flex-shrink: 0; }

  .msg { margin-bottom: 10px; }

  .result-box { display: flex; align-items: center; gap: 8px; margin-top: 14px; background: var(--bg); padding: 12px; border-radius: 8px; }
  .result-text { flex: 1; font-size: 14px; word-break: break-all; }
  .result-mono { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; }

  .jwt-section { margin-bottom: 14px; }
  .jwt-label { font-size: 13px; color: var(--text2); margin-bottom: 4px; font-weight: 500; }
  .jwt-json { background: var(--bg); padding: 12px; border-radius: 8px; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; white-space: pre-wrap; word-break: break-all; line-height: 1.5; margin: 0; }
  .jwt-badge { display: inline-flex; align-items: center; gap: 4px; font-size: 13px; font-weight: 600; }
  .jwt-valid { color: #4caf50; }
  .jwt-invalid { color: #e94560; }
  .verify-section { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--border); }
  .verify-section summary { cursor: pointer; font-size: 14px; font-weight: 500; color: var(--text2); margin-bottom: 12px; }
  .verify-section summary:hover { color: var(--text); }
</style>
