<script>
  import ConditionTree from './ConditionTree.svelte'
  let { node = { body: '', headers: [], children: [] }, level = 0, onRemove = null, condErrors = {} } = $props()

  function addChild() {
    if (!node.children) node.children = []
    const child = { condition: '', status: '', delay: '', headers: [], body: '', children: [], _uid: Date.now() + Math.random() }
    node.children = [...node.children, child]
  }

  function removeChild(child) {
    if (!node.children) return
    node.children = node.children.filter(c => c !== child)
  }

  function ensureUid() {
    if (!node._uid) {
      node._uid = Date.now() + Math.random()
    }
  }
  ensureUid()

  function addHeader() {
    node.headers = [...node.headers, { key: '', value: '' }]
  }
  function removeHeader(i) {
    node.headers = node.headers.filter((_, idx) => idx !== i)
  }
  function formatBody() {
    try {
      const parsed = JSON.parse(node.body)
      node.body = JSON.stringify(parsed, null, 2)
    } catch { }
  }
</script>

<div class="condition-node" style="margin-left: {level * 24}px; border-left: {level > 0 ? '2px solid var(--border2)' : 'none'}; padding-left: {level > 0 ? '16px' : '0'}; margin-top: 8px;">
  <div class="cond-row">
    <span class="cond-label">When</span>
      <input type="text" bind:value={node.condition} placeholder='e.g. {"{{"}header.authorization{"}}"} != empty' class="cond-input" class:cond-input-error={condErrors[node._uid]} />
    {#if onRemove}
      <button class="btn xs icon-btn danger" onclick={() => onRemove(node)} aria-label="Remove condition"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button>
    {/if}
  </div>
  {#if condErrors[node._uid]}
    <span class="err-msg">Condition expression is required</span>
  {/if}

  <div class="cond-details">
    <label>Status <input type="number" bind:value={node.status} min="100" max="599" placeholder="inherit" /></label>
    <label>Delay <input type="text" bind:value={node.delay} placeholder="inherit" /></label>
  </div>

  <div class="cond-body-section">
    <label class="cond-headers-label">Headers
      <div class="headers-list">
        {#each node.headers as h, i (i)}
          <div class="hr">
            <input type="text" bind:value={h.key} placeholder="Key" />
            <span class="sep">:</span>
            <input type="text" bind:value={h.value} placeholder="Value" />
            <button class="btn xs icon-btn" onclick={() => removeHeader(i)} aria-label="Remove header"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button>
          </div>
        {/each}
        <button class="btn xs" onclick={addHeader}>Add header</button>
      </div>
    </label>

    <label class="cond-body-label">Response Body <button class="btn xs" onclick={formatBody} style="float:right">Format</button>
      <div class="body-editor">
        <textarea bind:value={node.body} rows="4" placeholder='response body (JSON or plain text)'></textarea>
      </div>
    </label>
  </div>

  <button class="btn xs" onclick={addChild} style="margin: 4px 0 8px;">Add child condition</button>

  {#each (node.children || []) as child (child._uid)}
    <ConditionTree node={child} level={level + 1} onRemove={removeChild} condErrors={condErrors} />
  {/each}
</div>

<style>
  .condition-node { border-radius: var(--r-md); background: var(--surface); padding: 12px; border: 1px solid var(--border); margin-bottom: 8px; }
  .cond-row { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
  .cond-label { font-weight: 500; font-size: 12px; color: var(--text2); white-space: nowrap; }
  .cond-input { flex: 1; padding: 6px 10px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); font-family: var(--mono); font-size: 13px; }
  .cond-details { display: flex; gap: 16px; margin-bottom: 8px; }
  .cond-details label { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--text2); }
  .cond-details input[type="number"] { width: 70px; padding: 4px 8px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); }
  .cond-details input[type="text"] { width: 100px; padding: 4px 8px; border: 1px solid var(--border); border-radius: 4px; background: var(--bg); color: var(--text); }
  .cond-body-section { margin-left: 20px; margin-top: 12px; }
  .cond-headers-label { font-size: 13px; color: var(--text2); display: block; margin-bottom: 12px; }
  .cond-body-label { display: block; margin-top: 12px; font-size: 13px; color: var(--text2); }
  .cond-input-error { border-color: var(--danger) !important; }
  .err-msg { color: var(--danger); font-size: 12px; display: block; margin: -4px 0 6px; }
  .body-editor textarea { width: 100%; padding: 10px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg); color: var(--text); font-size: 13px; font-family: var(--mono); resize: vertical; line-height: 1.5; box-sizing: border-box; }
  .icon-btn { width: 26px; padding: 0; flex: none; }
</style>
