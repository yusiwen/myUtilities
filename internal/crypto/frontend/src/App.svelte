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

<header class="topbar">
  <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
    <span class="mu">mu</span><span class="sep">/</span><span class="app-name">crypto</span>
  </a>
</header>

<div class="app">
  <div class="page-head">
    <h1>Crypto</h1>
    <p>Encrypt, decrypt, passwords</p>
  </div>

  <div class="tabs">
    <button class="tab" class:active={tab === 'passwd'} onclick={() => tab = 'passwd'}>Password</button>
    <button class="tab" class:active={tab === 'cipher'} onclick={() => tab = 'cipher'}>Encrypt / decrypt</button>
    <button class="tab" class:active={tab === 'encode'} onclick={() => tab = 'encode'}>Encode / decode</button>
    <button class="tab" class:active={tab === 'jwt'} onclick={() => tab = 'jwt'}>JWT</button>
  </div>

  {#if tab === 'passwd'}
    <div class="card">
      <div class="field">
        <label for="pw-length">Length</label>
        <input id="pw-length" type="number" bind:value={pwLength} min="8" max="128" />
      </div>
      <div class="checks">
        <label class="check"><input type="checkbox" bind:checked={pwDigits} /><span>Digits</span></label>
        <label class="check"><input type="checkbox" bind:checked={pwSpecial} /><span>Special chars</span></label>
      </div>
      <button class="btn primary" onclick={genPasswd}>Generate</button>

      {#if pwError}<div class="msg error block">{pwError}</div>{/if}
      {#if password}
        <div class="result">
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
        <div class="radios">
          <label class="check"><input type="radio" bind:group={op} value="encrypt" /><span>Encrypt</span></label>
          <label class="check"><input type="radio" bind:group={op} value="decrypt" /><span>Decrypt</span></label>
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

      {#if cipherError}<div class="msg error block">{cipherError}</div>{/if}
      {#if result}
        <div class="result">
          <code class="result-text result-mono">{result}</code>
          <button class="btn xs" onclick={() => copy(result, v => copyLabelResult = v)}>{copyLabelResult}</button>
        </div>
      {/if}
    </div>
  {:else if tab === 'jwt'}
    <div class="card">
      <div class="field">
        <label for="jwt-token">JWT token</label>
        <textarea id="jwt-token" bind:value={jwtToken} rows="5" placeholder="Paste a JWT token here"></textarea>
      </div>

      <div class="row">
        <button class="btn primary" onclick={doJwtDecode}>Decode</button>
        {#if jwtVerified !== null}
          <span class="badge" class:ok={jwtVerified} class:err={!jwtVerified}>{jwtVerified ? 'Valid signature' : 'Invalid signature'}</span>
        {/if}
      </div>

      {#if jwtError}<div class="msg error block">{jwtError}</div>{/if}

      {#if jwtHeader}
        <div class="jwt-section">
          <div class="field-label">Header</div>
          <pre class="json-block">{jwtHeader}</pre>
        </div>
        <div class="jwt-section">
          <div class="field-label">Payload</div>
          <pre class="json-block">{jwtPayload}</pre>
        </div>
      {/if}

      <details class="verify">
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
            <div class="inline">
              <input id="jwt-key" type="text" bind:value={jwtKey} placeholder="HMAC secret key" />
              <label class="check"><input type="checkbox" bind:checked={jwtKeyB64} /><span>Base64</span></label>
            </div>
          </div>
        </div>
        <button class="btn primary" onclick={doJwtVerify}>Verify</button>
      </details>
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
        <div class="radios">
          <label class="check"><input type="radio" name="enc-op" bind:group={encOp} value="encode" onchange={() => switchOp('encode')} /><span>Encode</span></label>
          <label class="check"><input type="radio" name="enc-op" bind:group={encOp} value="decode" onchange={() => switchOp('decode')} /><span>Decode</span></label>
        </div>
      </div>

      <div class="field">
        <label for="enc-input">Input</label>
        <textarea id="enc-input" bind:value={encInput} rows="4" placeholder="Text to encode or decode"></textarea>
      </div>

      <button class="btn primary" onclick={doEnc}>{encOp === 'encode' ? 'Encode' : 'Decode'}</button>

      {#if encError}<div class="msg error block">{encError}</div>{/if}
      {#if encResult}
        <div class="result">
          <code class="result-text result-mono">{encResult}</code>
          <button class="btn xs" onclick={() => copy(encResult, v => encLabelResult = v)}>{encLabelResult}</button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  :global(body) { --page-max: 600px; }
  .block { margin: 12px 0; }
  .row { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 12px 0; }

  .field-label { margin-bottom: 4px; color: var(--text2); font-size: 12px; font-weight: 500; }
  .checks, .radios { display: flex; flex-wrap: wrap; gap: 16px; }
  .check { display: inline-flex; align-items: center; gap: 6px; color: var(--text); font-size: 13px; cursor: pointer; }
  .check input { width: 14px; height: 14px; margin: 0; accent-color: var(--text); }
  .inline { display: flex; align-items: center; gap: 8px; }
  .inline input { flex: 1; }

  .result { display: flex; align-items: flex-start; gap: 8px; margin-top: 12px; padding: 12px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); }
  .result-text { flex: 1; color: var(--text); font-size: 13px; line-height: 1.55; word-break: break-all; }
  .result-mono { font-family: var(--mono); }

  .jwt-section { margin-top: 16px; }
  .json-block { margin: 0; padding: 12px 14px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); font-size: 13px; line-height: 1.55; white-space: pre-wrap; word-break: break-all; }

  .verify { margin-top: 20px; padding-top: 16px; border-top: 1px solid var(--border); }
  .verify summary { margin-bottom: 12px; color: var(--text2); font-size: 13px; font-weight: 500; cursor: pointer; }
  .verify summary:hover { color: var(--text); }
</style>
